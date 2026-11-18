package main

import (
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestTokenRotationPreservesAccess(t *testing.T) {
	s, e := openInventoryServer(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	defer s.db.Close()
	_, _ = s.db.Exec("INSERT INTO users(id,name) VALUES('alice','alice')")
	server := httptest.NewTLSServer(s)
	defer server.Close()
	client, get, set, remove := apiHTTPClient, apiTokenGet, apiTokenSet, apiTokenDelete
	defer func() { apiHTTPClient, apiTokenGet, apiTokenSet, apiTokenDelete = client, get, set, remove }()
	apiHTTPClient = server.Client()
	keys := map[string]string{}
	apiTokenGet = func(service, id string) (string, error) {
		v, ok := keys[id]
		if !ok {
			return "", fmt.Errorf("missing")
		}
		return v, nil
	}
	apiTokenSet = func(service, id, value string) error { keys[id] = value; return nil }
	apiTokenDelete = func(service, id string) error { delete(keys, id); return nil }
	source := InventorySource{Type: "api", URL: server.URL, CredentialID: "device"}
	old, _ := s.issueToken("alice", "old", 90)
	newToken, _ := s.issueToken("alice", "new", 90)
	keys["device"] = old.Secret
	if e = rotateAPIToken(source, newToken); e != nil {
		t.Fatal(e)
	}
	var revoked int
	_ = s.db.QueryRow("SELECT revoked FROM tokens WHERE id=?", old.ID).Scan(&revoked)
	if revoked != 1 || keys["device"] != newToken.Secret || len(keys) != 1 {
		t.Fatal("rotation state")
	}
	active, _ := s.issueToken("alice", "active", 90)
	keys["device"] = active.Secret
	bad := serverToken{ID: "invalid", Secret: "invalid"}
	if rotateAPIToken(source, bad) == nil {
		t.Fatal("invalid replacement accepted")
	}
	_ = s.db.QueryRow("SELECT revoked FROM tokens WHERE id=?", active.ID).Scan(&revoked)
	if revoked != 0 || keys["device"] != active.Secret {
		t.Fatal("verification failure locked out user")
	}
}
