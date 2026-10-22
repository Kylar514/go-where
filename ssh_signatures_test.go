package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func fakeSignatureSupport(t *testing.T) *int {
	t.Helper()
	old := sshOutput
	calls := 0
	sshOutput = func(args ...string) ([]byte, error) {
		calls++
		if args[0] == "-Q" && (args[1] == "HostKeyAlgorithms" || args[1] == "PubkeyAcceptedAlgorithms") {
			return []byte("ssh-ed25519\nrsa-sha2-256\nssh-rsa\n"), nil
		}
		if args[0] == "-G" {
			return []byte("hostkeyalgorithms ssh-ed25519,rsa-sha2-256\npubkeyacceptedalgorithms ssh-ed25519,rsa-sha2-256\n"), nil
		}
		return nil, fmt.Errorf("unexpected query %v", args)
	}
	resetKexCache()
	t.Cleanup(func() { sshOutput = old; resetKexCache() })
	return &calls
}

func TestSignatureCatalogAndValidation(t *testing.T) {
	calls := fakeSignatureSupport(t)
	for _, directive := range []string{"HostKeyAlgorithms", "PubkeyAcceptedAlgorithms"} {
		entries, note := algorithmCandidates(directive, []string{"unsupported", "rsa-sha2-256"})
		if note != "" || !reflect.DeepEqual(entryNames(entries), []string{"rsa-sha2-256", "ssh-rsa", "unsupported"}) || !entries[1].Weak || !entries[2].Unsupported {
			t.Fatalf("%s candidates: %+v, %q", directive, entries, note)
		}
		if err := checkAlgorithmsSupported(directive, []string{"unsupported"}); err == nil || !strings.Contains(err.Error(), directive) {
			t.Fatalf("missing support error: %v", err)
		}
		for _, bad := range []string{"+ssh-rsa", "ssh-rsa\nHost *", "ssh-rsa,ssh-dss", "*", ""} {
			if validateExtraAlgorithms(directive, []string{bad}) == nil {
				t.Fatalf("accepted %q", bad)
			}
		}
		if validateExtraAlgorithms(directive, []string{"ssh-rsa", "ssh-rsa"}) == nil {
			t.Fatal("accepted duplicate")
		}
	}
	if *calls != 4 {
		t.Fatalf("expected cached query pair per directive, got %d", *calls)
	}
	for _, field := range []string{"extra_host_key_algorithms", "extra_pubkey_algorithms"} {
		if _, err := decodeConfig([]byte(`{"overrides":{"s":{"c":{"` + field + `":["+ssh-rsa"]}}}}`)); err == nil {
			t.Fatal("invalid override accepted")
		}
	}
	old := sshOutput
	sshOutput = func(...string) ([]byte, error) { return nil, fmt.Errorf("no ssh") }
	resetKexCache()
	for _, directive := range []string{"HostKeyAlgorithms", "PubkeyAcceptedAlgorithms"} {
		entries, note := algorithmCandidates(directive, nil)
		if len(entries) != 1 || entries[0].Name != "ssh-rsa" || !entries[0].Weak || entries[0].Unsupported || !strings.Contains(note, "unverified") {
			t.Fatalf("fallback: %+v %q", entries, note)
		}
		if err := checkAlgorithmsSupported(directive, []string{"custom"}); err != nil {
			t.Fatal(err)
		}
	}
	sshOutput = old
	resetKexCache()
}

func TestSignatureEditorPersistenceOverridesAndHops(t *testing.T) {
	fakeSignatureSupport(t)
	s := testInventory(t, "Signatures")
	jump := Connection{ID: "jump", Alias: "jump", Address: "192.0.2.1", Port: 22}
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.2", Port: 22, JumpID: jump.ID}
	scriptTUI(t, "hostkey", "toggle:ssh-rsa,done", "pubkey", "toggle:ssh-rsa,done", "save")
	if err := editSSHCompatibility(&r); err != nil {
		t.Fatal(err)
	}
	for _, record := range []Connection{jump, r} {
		if err := changeConnection(s, record, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	h := Host{SourceID: s.ID, ConnectionID: r.ID, Alias: r.Alias, Type: "ssh"}
	_, saved, err := storedRecord(h, c)
	if err != nil || !reflect.DeepEqual(saved.ExtraHostKeyAlgorithms, []string{"ssh-rsa"}) || !reflect.DeepEqual(saved.ExtraPubkeyAlgorithms, []string{"ssh-rsa"}) {
		t.Fatalf("save/read: %+v %v", saved, err)
	}
	job, err := managedJob(h, c)
	if err != nil {
		t.Fatal(err)
	}
	blocks := strings.Split(job.Configuration, "Host gw-1")
	for _, directive := range []string{"HostKeyAlgorithms", "PubkeyAcceptedAlgorithms"} {
		if !strings.Contains(blocks[0], directive+" +ssh-rsa") || strings.Contains(blocks[1], directive) {
			t.Fatalf("exception leaked to jump: %s", job.Configuration)
		}
	}
	if !strings.Contains(managedDetails(h, c), "Additional public-key signature algorithms: ssh-rsa (connection default)") {
		t.Fatal("Details missing signature settings")
	}
	// Cancel must discard checklist edits without changing the connection.
	scriptTUI(t, "hostkey", "toggle:ssh-rsa,back", "save")
	if err := editSSHCompatibility(&r); err != nil || len(r.ExtraHostKeyAlgorithms) != 1 {
		t.Fatal("cancel altered record", err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	original := []byte("{ // preserve\n}\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	before.Sources = []InventorySource{s}
	draft := before
	scriptTUI(t, "hostkey", "custom", "toggle:ssh-rsa,done", "pubkey", "custom", "toggle:ssh-rsa,done", "save")
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
	if o.ExtraHostKeyAlgorithms == nil || len(*o.ExtraHostKeyAlgorithms) != 0 || o.ExtraPubkeyAlgorithms == nil || len(*o.ExtraPubkeyAlgorithms) != 0 {
		t.Fatalf("explicit empty lost: %+v", o)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "// preserve") || !strings.Contains(string(data), `"extra_host_key_algorithms":[]`) || !strings.Contains(string(data), `"extra_pubkey_algorithms":[]`) {
		t.Fatalf("settings save: %s", data)
	}
	// Empty local additions on target; independent legacy additions on jump.
	legacy := []string{"ssh-rsa"}
	draft.Overrides[s.ID][jump.ID] = ConnectionOverride{ExtraHostKeyAlgorithms: &legacy, ExtraPubkeyAlgorithms: &legacy}
	job, err = managedJob(h, draft)
	if err != nil {
		t.Fatal(err)
	}
	blocks = strings.Split(job.Configuration, "Host gw-1")
	if strings.Contains(blocks[0], "Algorithms +ssh-rsa") || !strings.Contains(blocks[1], "HostKeyAlgorithms +ssh-rsa") || !strings.Contains(blocks[1], "PubkeyAcceptedAlgorithms +ssh-rsa") {
		t.Fatalf("per-hop overrides: %s", job.Configuration)
	}
	// Tunnels and RDP jumps use this same snapshot generation path.
	if err = changeConnection(s, Connection{ID: "rdp", Alias: "desktop", Type: "rdp", Address: "192.0.2.3", Port: 3389, JumpID: r.ID}, false); err != nil {
		t.Fatal(err)
	}
	rdp, err := hostRDPJob(draft, Host{SourceID: s.ID, ConnectionID: "rdp", Type: "rdp"}, "")
	if err != nil || rdp.SSHSnapshot != job.Configuration {
		t.Fatalf("RDP snapshot mismatch: %+v %v", rdp, err)
	}
	tunnel := Connection{ID: "tunnel", Alias: "forward", Type: "tunnel", TunnelType: "ssh", Bind: "127.0.0.1", LocalPort: 8080, RemoteHost: "localhost", RemotePort: 80, SSHID: r.ID}
	if err = changeConnection(s, tunnel, false); err != nil {
		t.Fatal(err)
	}
	conn, err := inventoryTunnel(Host{SourceID: s.ID, ConnectionID: tunnel.ID, Type: "tunnel"}, draft)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := base64.RawURLEncoding.DecodeString(conn.Resource)
	var spec inventoryTunnelSpec
	if err = json.Unmarshal(b, &spec); err != nil || spec.Configuration != job.Configuration {
		t.Fatal("tunnel snapshot mismatch", err)
	}
	scriptTUI(t, "hostkey", "inherit", "pubkey", "inherit", "save")
	if err = editCompatibilityOverrideDraft(&draft, h); err != nil {
		t.Fatal(err)
	}
	o = draft.Overrides[s.ID][r.ID]
	if o.ExtraHostKeyAlgorithms != nil || o.ExtraPubkeyAlgorithms != nil {
		t.Fatal("inherit failed")
	}
	for _, field := range []string{"hostkey", "pubkey"} {
		bad := []string{"unsupported"}
		o = ConnectionOverride{}
		if field == "hostkey" {
			o.ExtraHostKeyAlgorithms = &bad
		} else {
			o.ExtraPubkeyAlgorithms = &bad
		}
		draft.Overrides[s.ID][r.ID] = o
		if _, err = managedJob(h, draft); err == nil {
			t.Fatal("unsupported launch accepted")
		}
		bad[0] = "ssh-rsa\nHost *"
		if _, err = managedJob(h, draft); err == nil {
			t.Fatal("unsafe constructed override accepted")
		}
	}
	badRecord := r
	badRecord.ExtraHostKeyAlgorithms = []string{"+ssh-rsa"}
	if changeConnection(s, badRecord, false) == nil {
		t.Fatal("unsafe database field accepted")
	}
	badRecord = r
	badRecord.ExtraPubkeyAlgorithms = []string{"ssh-rsa", "ssh-rsa"}
	if changeConnection(s, badRecord, false) == nil {
		t.Fatal("duplicate database field accepted")
	}
}

func TestSignatureMigrationPreservesSettings(t *testing.T) {
	s := testInventory(t, "Marker")
	db, err := openInventory(s)
	if err != nil {
		t.Fatal(err)
	}
	const data = `{"id":"server","alias":"server","address":"192.0.2.1","port":22,"notes":"unchanged"}`
	if _, err = db.Exec("INSERT INTO connections(id,alias,data) VALUES('server','server',?)", data); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DROP TABLE hosts"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("DROP TABLE profiles"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("PRAGMA user_version=4"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = openInventory(s)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	var got string
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 7 {
		t.Fatal("marker not advanced", version, err)
	}
	if err = db.QueryRow("SELECT data FROM connections").Scan(&got); err != nil {
		t.Fatal(err)
	}
	var profile Connection
	if err = json.Unmarshal([]byte(got), &profile); err != nil || profile.HostID != "server" || profile.ID != "server" || !profile.Default {
		t.Fatal("profile migration", got, err)
	}
	if err = db.QueryRow("SELECT data FROM hosts").Scan(&got); err != nil || !strings.Contains(got, `"notes":"unchanged"`) || !strings.Contains(got, `"address":"192.0.2.1"`) {
		t.Fatal("host settings lost", got, err)
	}
}

func TestSignatureSnapshotOpenSSH(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("OpenSSH not installed")
	}
	resetKexCache()
	t.Cleanup(resetKexCache)
	for _, directive := range []string{"HostKeyAlgorithms", "PubkeyAcceptedAlgorithms"} {
		caps := algorithmSupport(directive)
		if !caps.known || !caps.supported["ssh-rsa"] {
			t.Skip("OpenSSH does not support ssh-rsa for " + directive)
		}
	}
	s := testInventory(t, "OpenSSH")
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.1", Port: 22, ExtraHostKeyAlgorithms: []string{"ssh-rsa"}, ExtraPubkeyAlgorithms: []string{"ssh-rsa"}}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	job, err := managedJob(Host{SourceID: s.ID, ConnectionID: r.ID}, c)
	if err != nil {
		t.Fatal(err)
	}
	path, cleanup, err := sshSnapshot(job.Configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	out, err := exec.Command("ssh", "-G", "-F", path, job.Target).CombinedOutput()
	if err != nil {
		t.Fatalf("OpenSSH rejected snapshot: %s %v", out, err)
	}
	for _, directive := range []string{"hostkeyalgorithms", "pubkeyacceptedalgorithms"} {
		found := false
		for _, line := range strings.Split(string(out), "\n") {
			key, value, _ := strings.Cut(line, " ")
			if key == directive {
				for _, name := range strings.Split(value, ",") {
					found = found || name == "ssh-rsa"
				}
				if !strings.Contains(value, "rsa-sha2-") {
					t.Fatal("addition replaced defaults", line)
				}
			}
		}
		if !found {
			t.Fatal("ssh-rsa missing from effective " + directive)
		}
	}
}
