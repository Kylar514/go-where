package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPersistentOfflineInventory(t *testing.T) {
	dir := t.TempDir()
	oldDir := apiDiskCacheDir
	apiDiskCacheDir = func() (string, error) { return dir, nil }
	defer func() { apiDiskCacheDir = oldDir }()
	s, err := openInventoryServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	_, _ = s.db.Exec("INSERT INTO users(id,name) VALUES('alice','alice')")
	inv := testInventory(t, "Cached")
	if err = saveInventoryHost(inv, InventoryHost{ID: "host", Alias: "host", Address: "192.0.2.1"}, nil); err != nil {
		t.Fatal(err)
	}
	_, _ = s.db.Exec("INSERT INTO inventories(id,name,path) VALUES(?,?,?)", inv.ID, inv.Name, inv.Path)
	_, _ = s.db.Exec("INSERT INTO grants VALUES('alice',?,'read-write')", inv.ID)
	token, _ := s.issueToken("alice", "device", 90)
	server := httptest.NewTLSServer(s)
	client, get := apiHTTPClient, apiTokenGet
	apiHTTPClient = server.Client()
	apiTokenGet = func(string, string) (string, error) { return token.Secret, nil }
	defer func() { apiHTTPClient = client; apiTokenGet = get }()
	source := InventorySource{ID: inv.ID, Name: "Cached", Type: "api", URL: server.URL, CredentialID: "device"}
	v, err := readAPISnapshot(source)
	if err != nil || v.Offline || v.FetchedAt.IsZero() {
		t.Fatal(v, err)
	}
	server.Close()
	refreshAPISources(Config{Sources: []InventorySource{source}})
	v, err = readAPISnapshot(source)
	if err != nil || !v.Offline || !v.ReadOnly || len(v.Hosts) != 1 {
		t.Fatal(v, err)
	}
	if !strings.Contains(apiSourceAvailability(source), "Cached") {
		t.Fatal("missing cache status")
	}
	if saveHostFields(source, v.Hosts, map[string]string{"notes": "bad"}) == nil {
		t.Fatal("offline write accepted")
	}
	if err = clearAPIDiskCache(source); err != nil {
		t.Fatal(err)
	}
	if _, err = readAPISnapshot(source); err == nil {
		t.Fatal("cleared cache still usable")
	}
}

func TestAuthFailureDoesNotUseDiskCache(t *testing.T) {
	dir := t.TempDir()
	old := apiDiskCacheDir
	apiDiskCacheDir = func() (string, error) { return dir, nil }
	defer func() { apiDiskCacheDir = old }()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(403) }))
	defer server.Close()
	client, get := apiHTTPClient, apiTokenGet
	apiHTTPClient = server.Client()
	apiTokenGet = func(string, string) (string, error) { return "token", nil }
	defer func() { apiHTTPClient = client; apiTokenGet = get }()
	s := InventorySource{ID: "cache", URL: server.URL, CredentialID: "device", Type: "api"}
	if err := saveAPIDiskCache(s, apiSnapshot{Revision: "rev", FetchedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := readAPISnapshot(s); err == nil {
		t.Fatal("permission failure used cache")
	}
	if _, err := loadAPIDiskCache(s); err == nil {
		t.Fatal("denied snapshot retained")
	}
}
