package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSSHCompatibilityValidation(t *testing.T) {
	for _, bad := range []string{"+diffie-hellman-group1-sha1", "-foo", "^foo", "*", "foo bar", "foo\nProxyCommand sh", "foo,bar", "foo\x00bar", "foo;bar", ""} {
		if validateExtraKex([]string{bad}) == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if validateExtraKex([]string{"foo", "foo"}) == nil {
		t.Fatal("duplicate algorithm accepted")
	}
	if validateExtraKex([]string{"diffie-hellman-group1-sha1", "sntrup761x25519-sha512@openssh.com"}) != nil {
		t.Fatal("valid names rejected")
	}
	if _, err := decodeConfig([]byte(`{"overrides":{"s":{"c":{"extra_kex_algorithms":["bad\nHost *"]}}}}`)); err == nil {
		t.Fatal("invalid local KEX accepted")
	}
}

// fakeKexSupport stands in for the local OpenSSH and counts its invocations.
func fakeKexSupport(t *testing.T, supported, defaults []string) *int {
	t.Helper()
	old := sshOutput
	calls := 0
	sshOutput = func(args ...string) ([]byte, error) {
		calls++
		switch args[0] {
		case "-Q":
			return []byte(strings.Join(supported, "\n") + "\n"), nil
		case "-G":
			return []byte("hostname probe\nkexalgorithms " + strings.Join(defaults, ",") + "\n"), nil
		}
		return nil, fmt.Errorf("unexpected ssh query %v", args)
	}
	resetKexCache()
	t.Cleanup(func() { sshOutput = old; resetKexCache() })
	return &calls
}

var (
	testKexDefaults  = []string{"curve25519-sha256", "diffie-hellman-group14-sha256"}
	testKexSupported = append([]string{"diffie-hellman-group-exchange-sha1", "diffie-hellman-group1-sha1", "diffie-hellman-group14-sha1"}, testKexDefaults...)
)

func TestSSHCompatibilityDefaultsOverridesAndHops(t *testing.T) {
	fakeKexSupport(t, testKexSupported, testKexDefaults)
	s := testInventory(t, "Compatibility")
	yes, no := true, false
	jump := Connection{ID: "gateway", Alias: "gateway", Address: "192.0.2.1", Port: 22}
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.2", Port: 22, JumpID: jump.ID, IdentitiesOnly: &yes, ExtraKexAlgorithms: []string{"diffie-hellman-group1-sha1"}}
	for _, record := range []Connection{jump, r} {
		if err := changeConnection(s, record, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	h := Host{SourceID: s.ID, ConnectionID: r.ID}
	j, err := managedJob(h, c)
	if err != nil {
		t.Fatal(err)
	}
	first, second, _ := strings.Cut(j.Configuration, "Host gw-1")
	if !strings.Contains(first, "IdentitiesOnly yes") || !strings.Contains(first, "KexAlgorithms +diffie-hellman-group1-sha1") || strings.Contains(second, "IdentitiesOnly") || strings.Contains(second, "KexAlgorithms") {
		t.Fatalf("per-hop defaults wrong: %s", j.Configuration)
	}
	// Check the actual OpenSSH parser when the local build supports this legacy KEX.
	if ssh, e := exec.LookPath("ssh"); e == nil {
		supported, _ := exec.Command(ssh, "-Q", "kex").Output()
		if strings.Contains(string(supported), "diffie-hellman-group1-sha1") {
			path := filepath.Join(t.TempDir(), "config")
			os.WriteFile(path, []byte(j.Configuration), 0600)
			out, e := exec.Command(ssh, "-G", "-F", path, "gw-0").CombinedOutput()
			if e != nil || !strings.Contains(string(out), "identitiesonly yes") || !strings.Contains(string(out), "diffie-hellman-group1-sha1") {
				t.Fatalf("OpenSSH rejected scoped settings: %s %v", out, e)
			}
		}
	}
	empty := []string{}
	jumpKex := []string{"diffie-hellman-group14-sha256"}
	c.Overrides = map[string]map[string]ConnectionOverride{s.ID: {
		r.ID:    {IdentitiesOnly: &no, ExtraKexAlgorithms: &empty},
		jump.ID: {IdentitiesOnly: &yes, ExtraKexAlgorithms: &jumpKex},
	}}
	j, err = managedJob(h, c)
	if err != nil {
		t.Fatal(err)
	}
	first, second, _ = strings.Cut(j.Configuration, "Host gw-1")
	if !strings.Contains(first, "IdentitiesOnly no") || strings.Contains(first, "KexAlgorithms") || !strings.Contains(second, "IdentitiesOnly yes") || !strings.Contains(second, "KexAlgorithms +diffie-hellman-group14-sha256") {
		t.Fatalf("local overrides not independent: %s", j.Configuration)
	}
	for _, record := range []Connection{
		{ID: "rdp", Type: "rdp", Alias: "desktop", Address: "192.0.2.3", Port: 3389, JumpID: r.ID},
		{ID: "tunnel", Type: "tunnel", Alias: "forward", TunnelType: "ssh", SSHID: r.ID, Bind: "127.0.0.1", LocalPort: 8089, RemoteHost: "localhost", RemotePort: 80},
	} {
		if err = changeConnection(s, record, false); err != nil {
			t.Fatal(err)
		}
	}
	rdp, err := hostRDPJob(c, Host{SourceID: s.ID, ConnectionID: "rdp", Type: "rdp"}, "")
	if err != nil || rdp.SSHSnapshot != j.Configuration {
		t.Fatal("RDP jump did not reuse compatibility settings")
	}
	tun, err := inventoryTunnel(Host{SourceID: s.ID, ConnectionID: "tunnel", Type: "tunnel"}, c)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := base64.RawURLEncoding.DecodeString(tun.Resource)
	var spec inventoryTunnelSpec
	json.Unmarshal(b, &spec)
	if spec.Configuration != j.Configuration {
		t.Fatal("tunnel did not reuse per-hop settings")
	}
	bad := []string{"bad\nHost *"}
	c.Overrides[s.ID][r.ID] = ConnectionOverride{ExtraKexAlgorithms: &bad}
	if _, err = managedJob(h, c); err == nil {
		t.Fatal("invalid constructed override bypassed launch validation")
	}
}

func TestSSHCompatibilityEditorAndLocalSettings(t *testing.T) {
	fakeKexSupport(t, testKexSupported, testKexDefaults)
	s := testInventory(t, "Editor")
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.1", Port: 22, User: "${UNCONFIGURED}"}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	h := Host{Alias: r.Alias, SourceID: s.ID, ConnectionID: r.ID, Type: "ssh"}
	state := PickerState{Mode: "ssh", Source: s.ID}
	scriptTUI(t, "compatibility", "identities", "yes", "kex", "toggle:diffie-hellman-group1-sha1,done", "save", "save")
	if err := editConnection(c, &state, &h); err != nil {
		t.Fatal(err)
	}
	rows, _ := sourceConnections(s)
	if rows[0].IdentitiesOnly == nil || !*rows[0].IdentitiesOnly || len(rows[0].ExtraKexAlgorithms) != 1 {
		t.Fatal("database compatibility not saved")
	}
	// Cancelling the connection discards a changed compatibility draft.
	scriptTUI(t, "compatibility", "identities", "no", "save", "back")
	if err := editConnection(c, &state, &h); err != nil {
		t.Fatal(err)
	}
	rows, _ = sourceConnections(s)
	if !*rows[0].IdentitiesOnly {
		t.Fatal("Cancel saved database change")
	}
	path := filepath.Join(t.TempDir(), "config.jsonc")
	writeTestFile(t, path, "// keep me\n{\"overrides\":{\""+s.ID+"\":{\"server\":{\"ssh_agent\":\"none\"}}}}")
	if err := registerSource(path, s); err != nil {
		t.Fatal(err)
	}
	// Override editing must not require resolving unrelated user/key variables.
	scriptTUI(t, "identities", "no", "kex", "custom", "toggle:diffie-hellman-group1-sha1,done", "save")
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = editCompatibilityOverrideDraft(&draft, h); err != nil {
		t.Fatal(err)
	}
	if err = saveSettingsDraft(path, original, before, draft); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	o := loaded.Overrides[s.ID][r.ID]
	if o.IdentitiesOnly == nil || *o.IdentitiesOnly || o.ExtraKexAlgorithms == nil || len(*o.ExtraKexAlgorithms) != 0 || o.SSHAgent == nil || *o.SSHAgent != "none" {
		t.Fatalf("bad override round trip: %+v", o)
	}
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte("// keep me")) || !bytes.Contains(data, []byte(`"extra_kex_algorithms":[]`)) {
		t.Fatalf("lost comments/explicit empty list: %s", data)
	}
	scriptTUI(t, "identities", "inherit", "kex", "inherit", "back")
	_, _, draft, err = settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = editCompatibilityOverrideDraft(&draft, h); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(data, after) {
		t.Fatal("Cancel saved local override changes")
	}
	scriptTUI(t, "identities", "inherit", "kex", "inherit", "save", "save")
	if err = editConnectionOverride(path, h, editCompatibilityOverrideDraft); err != nil {
		t.Fatal(err)
	}
	loaded, err = loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	o = loaded.Overrides[s.ID][r.ID]
	if o.IdentitiesOnly != nil || o.ExtraKexAlgorithms != nil || o.SSHAgent == nil {
		t.Fatal("inherit failed or removed unrelated agent override")
	}
	rows, _ = sourceConnections(s)
	if !*rows[0].IdentitiesOnly || len(rows[0].ExtraKexAlgorithms) != 1 {
		t.Fatal("local override modified database defaults")
	}
}
