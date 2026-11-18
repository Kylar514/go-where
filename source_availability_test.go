package main

import (
	"testing"
)

func TestUnavailableSourceDoesNotHideHealthyInventory(t *testing.T) {
	s := testInventory(t, "Healthy")
	if err := saveInventoryHost(s, InventoryHost{ID: "host", Alias: "host", Address: "192.0.2.1"}, nil); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	remote := InventorySource{ID: "remote", Name: "Offline API", Type: "api", URL: "https://127.0.0.1:1", CredentialID: "missing"}
	c.Sources = []InventorySource{remote, s}
	get := apiTokenGet
	defer func() { apiTokenGet = get }()
	apiTokenGet = func(string, string) (string, error) { return "test", nil }
	hosts, failures := availableInventoryHosts(c)
	if len(hosts) != 1 || hosts[0].Alias != "host" || failures[remote.ID] == "" {
		t.Fatal(hosts, failures)
	}
	if _, err := sourceInventoryHosts(remote); err == nil {
		t.Fatal("strict read disguised unavailable inventory as empty")
	}
	c.DisabledSources = []string{remote.ID}
	_, failures = availableInventoryHosts(c)
	if len(failures) != 0 {
		t.Fatal("disabled source queried")
	}
}
