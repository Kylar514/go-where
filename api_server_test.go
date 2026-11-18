package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteInventoryPermissionsConflictsAndTokens(t *testing.T) {
	s, err := openInventoryServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	_, err = s.db.Exec("INSERT INTO users(id,name) VALUES('alice','alice'),('bob','bob')")
	if err != nil {
		t.Fatal(err)
	}
	inv := testInventory(t, "Shared")
	if _, err = s.db.Exec("INSERT INTO inventories(id,name,path) VALUES(?,?,?)", inv.ID, inv.Name, inv.Path); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("INSERT INTO grants(user_id,inventory_id,access) VALUES('alice',?,'read-write'),('bob',?,'read-only')", inv.ID, inv.ID); err != nil {
		t.Fatal(err)
	}
	a, _ := s.issueToken("alice", "laptop", 90)
	b, _ := s.issueToken("bob", "laptop", 90)
	server := httptest.NewTLSServer(s)
	defer server.Close()
	oldClient, oldGet := apiHTTPClient, apiTokenGet
	apiHTTPClient = server.Client()
	apiTokenGet = func(service, id string) (string, error) {
		if id == "bob" {
			return b.Secret, nil
		}
		return a.Secret, nil
	}
	defer func() { apiHTTPClient = oldClient; apiTokenGet = oldGet }()
	source := InventorySource{ID: inv.ID, Name: inv.Name, Type: "api", URL: server.URL, CredentialID: "alice"}
	refreshAPISources(Config{Sources: []InventorySource{source}})
	v, err := readAPISnapshot(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = saveInventoryHost(source, InventoryHost{ID: "host", Alias: "host", Address: "192.0.2.1"}, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := sourceInventoryHosts(source)
	if err != nil || len(loaded) != 1 {
		t.Fatal(loaded, err)
	}
	if err = changeConnection(source, Connection{ID: "ssh", Alias: "host-ssh", HostID: "host", ProfileName: "Default", Default: true, Type: "ssh", Port: 22, User: "operator"}, false); err != nil {
		t.Fatal(err)
	}
	connections, err := sourceConnections(source)
	if err != nil || len(connections) != 1 || connections[0].OriginalProfile == nil {
		t.Fatal("remote connection reconstruction", connections, err)
	}
	connections[0].User = "mutated draft"
	again, _ := sourceConnections(source)
	if again[0].User == "mutated draft" {
		t.Fatal("draft modified cached snapshot")
	}
	c := defaultConfig()
	c.Sources = []InventorySource{source}
	hosts, err := inventoryHosts(c)
	if err != nil || len(hosts) != 1 {
		t.Fatal(hosts, err)
	}
	job, err := managedJob(hosts[0], c)
	if err != nil || !strings.Contains(job.Configuration, "operator") {
		t.Fatal("remote launch resolution", err)
	}
	baseline, err := sourceInventoryHosts(source)
	if err != nil {
		t.Fatal(err)
	}
	if err = saveHostFields(source, baseline, map[string]string{"notes": "edited note"}); err != nil {
		t.Fatal("ordinary host edit failed", err)
	}
	baseline, _ = sourceInventoryHosts(source)
	baseline[0].Tags = []string{}
	if err = saveHostFields(source, baseline, map[string]string{"color": "#ef4444"}); err != nil {
		t.Fatal("empty tags edit failed", err)
	}
	profiles, err := sourceProfiles(source)
	fresh, _ := sourceInventoryHosts(source)
	if e := saveHostFields(source, fresh, map[string]string{"address": ""}); e == nil || strings.Contains(e.Error(), "inventory changed") || !strings.Contains(e.Error(), "valid host name and address required") {
		t.Fatal("validation error misreported", e)
	}
	if err != nil {
		t.Fatal(err)
	}
	profiles[0].User = "changed-user"
	if err = saveReusableProfile(source, profiles[0]); err != nil {
		t.Fatal("ordinary profile edit failed", err)
	}
	path := filepath.Join(t.TempDir(), "profile.db")
	original, before, settings, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	settings.Sources = []InventorySource{source}
	if err = saveSettingsDraft(path, original, before, settings); err != nil {
		t.Fatal(err)
	}
	scriptTUI(t, "notes", "enter\nUI edit", "save", "back")
	if err = editPersonalHosts(settings, path, hosts); err != nil {
		t.Fatal("unified remote edit failed", err)
	}
	if err = apiRequest(source, "POST", "inventories/"+inv.ID, apiMutation{Action: "host-delete", ID: "host"}, nil, v.Revision); err == nil {
		t.Fatal("stale revision accepted")
	}
	readonly := source
	readonly.CredentialID = "bob"
	if err = saveInventoryHost(readonly, InventoryHost{ID: "bad", Alias: "bad", Address: "192.0.2.2"}, nil); err == nil {
		t.Fatal("read-only mutation accepted")
	}
	if err = apiRequest(readonly, "POST", "inventories/"+inv.ID, apiMutation{Action: "host-delete", ID: "host"}, nil, v.Revision); err == nil {
		t.Fatal("server ignored permission")
	}
	if err = apiRequest(readonly, "DELETE", "tokens/"+a.ID, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err = apiRequest(source, "GET", "inventories", nil, &[]InventorySource{}, ""); err != nil {
		t.Fatal("another user revoked token")
	}
	var tokens []serverToken
	if err = apiRequest(source, "GET", "tokens", nil, &tokens, ""); err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		if token.Secret != "" {
			t.Fatal("token secret listed")
		}
	}
	if _, err = s.db.Exec("UPDATE users SET disabled=1 WHERE id='alice'"); err != nil {
		t.Fatal(err)
	}
	if err = apiRequest(source, "GET", "tokens", nil, &tokens, ""); err == nil {
		t.Fatal("disabled user authenticated")
	}
	if validateAPIURL("http://example.com") == nil || validateAPIURL("https://user:secret@example.com") == nil {
		t.Fatal("unsafe URL accepted")
	}
}

func TestServerSnapshotMutationValidation(t *testing.T) {
	s, err := openInventoryServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	_, _ = s.db.Exec("INSERT INTO users(id,name) VALUES('alice','alice')")
	inv := testInventory(t, "Validation")
	_, _ = s.db.Exec("INSERT INTO inventories(id,name,path) VALUES(?,?,?)", inv.ID, inv.Name, inv.Path)
	_, _ = s.db.Exec("INSERT INTO grants VALUES('alice',?,'read-write')", inv.ID)
	token, _ := s.issueToken("alice", "test", 90)
	v, err := localAPISnapshot(inv)
	if err != nil {
		t.Fatal(err)
	}
	request := func(body string, secret string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/v1/inventories/"+inv.ID, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+secret)
		r.Header.Set("If-Match", v.Revision)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	if w := request(`{"action":"host-save","unknown":true}`, token.Secret); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request(`{"action":"host-save","host":{"id":"a","alias":"a","address":"bad\naddress"}}`, token.Secret); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request(`{}`, strings.Repeat("0", 64)); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(`{"action":"bulk","before":[{}],"after":[]}`, token.Secret); w.Code != 409 {
		t.Fatal(w.Code)
	}
	var secret string
	if err = s.db.QueryRow("SELECT hash FROM tokens WHERE id=?", token.ID).Scan(&secret); err != nil || secret == token.Secret {
		t.Fatal("plaintext stored")
	}
	if err = s.serve("0.0.0.0:8080", "", ""); err == nil {
		t.Fatal("plaintext public bind allowed")
	}
}

func TestAPISourceRegistrationNoPlaintextToken(t *testing.T) {
	// Token-bearing request serialization is limited to headers, not source data.
	s := InventorySource{ID: "inventory", Type: "api", Name: "Remote", URL: "https://example.com", CredentialID: "credential"}
	data, err := json.Marshal(s)
	if err != nil || bytes.Contains(data, []byte("Bearer")) {
		t.Fatal(string(data), err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	if err = c.validate(); err != nil {
		t.Fatal(err)
	}
}
