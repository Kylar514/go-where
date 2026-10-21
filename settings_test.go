package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestJSONCSettingsPreserveCommentsAndRejectStaleDraft(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	raw := []byte(`// local preferences
{
  "startup_mode": "ssh", // keep startup comment
  "ssh_agents": {
    "legacy": { "socket_env": "OLD_AGENT", /* keep profile comment */ },
  },
  "overrides": {
    "source": { "connection": { "user": "operator", /* keep override comment */ } },
  },
  "rdp": { "client_args": ["+clipboard", /* keep args comment */], },
}
// keep footer
`)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, raw) {
		t.Fatal("JSONC decode modified original bytes")
	}
	draft.StartupMode = "rdp"
	draft.SSHAgents["legacy"] = SSHAgentProfile{SocketEnv: "SSH_AUTH_SOCK_LEGACY"}
	o := draft.Overrides["source"]["connection"]
	agent := "legacy"
	o.SSHAgent = &agent
	draft.Overrides["source"]["connection"] = o
	if err = saveSettingsDraft(path, original, before, draft); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"// local preferences", "// keep startup comment", "/* keep profile comment */", "/* keep override comment */", `"rdp": { "client_args": ["+clipboard", /* keep args comment */], },`, "// keep footer"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("lost unrelated bytes/comments %q: %s", want, data)
		}
	}
	c, err := loadConfig(path)
	if err != nil || c.StartupMode != "rdp" || c.SSHAgents["legacy"].SocketEnv != "SSH_AUTH_SOCK_LEGACY" || c.Overrides["source"]["connection"].User == nil || *c.Overrides["source"]["connection"].User != "operator" {
		t.Fatalf("bad round trip: %+v %v", c, err)
	}
	current := append([]byte("// external change\n"), data...)
	os.WriteFile(path, current, 0600)
	if err = saveSettingsDraft(path, original, before, draft); err == nil {
		t.Fatal("stale draft overwritten")
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, current) {
		t.Fatal("failed save changed file")
	}
	for _, bad := range []string{`null`, `[]`, `{"startup_mode":"ssh","startup_mode":"rdp"}`, `{"ssh_agents":{"x":{"socket_env":"A","socket_env":"B"}}}`, `{"unknown":true}`} {
		if _, err = decodeConfig([]byte(bad)); err == nil {
			t.Fatalf("accepted ambiguous/invalid settings %s", bad)
		}
	}
	beforeData := append([]byte(nil), current...)
	original, before, draft, err = settingsDraft(path)
	if err != nil {
		t.Fatal(err)
	}
	draft.SSHAgents["broken"] = SSHAgentProfile{Socket: "/a", SocketEnv: "B"}
	if err = saveSettingsDraft(path, original, before, draft); err == nil {
		t.Fatal("invalid profile saved")
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, beforeData) {
		t.Fatal("validation failure changed file")
	}
}

func TestSourceRegistrationPreservesJSONC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.jsonc")
	s := testInventory(t, "First")
	other := testInventory(t, "Other")
	raw := "// user config\n{\n  /* preserve me */ \"startup_mode\": \"tunnels\",\n  \"sources\": [ /* source list */ ],\n}\n"
	writeTestFile(t, path, raw)
	if err := registerSource(path, s); err != nil {
		t.Fatal(err)
	}
	if err := registerSource(path, other); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, comment := range []string{"// user config", "/* preserve me */", "/* source list */"} {
		if !strings.Contains(string(data), comment) {
			t.Fatalf("source registration lost comments: %s", data)
		}
	}
	c, err := loadConfig(path)
	if err != nil || len(c.Sources) != 2 || c.StartupMode != "tunnels" {
		t.Fatalf("bad registration: %v", err)
	}
}

func TestSettingsMenuDraftSaveCancelAndStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	scriptTUI(t, "general", "rdp", "agents", "add", "enter\nlegacy", "save", "back", "save")
	if raw, err := settingsMenu(path); err != nil || raw {
		t.Fatalf("settings menu: %v", err)
	}
	c, err := loadConfig(path)
	if err != nil || c.StartupMode != "rdp" || c.SSHAgents["legacy"].SocketEnv != "SSH_AUTH_SOCK_LEGACY" {
		t.Fatalf("settings not saved: %+v %v", c, err)
	}
	data, _ := os.ReadFile(path)
	scriptTUI(t, "general", "tunnels", "agents", "profile:legacy", "delete", "delete", "back", "cancel")
	if _, err = settingsMenu(path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, data) {
		t.Fatal("Cancel changed settings")
	}
	// Missing file + Cancel must not create a file or directory.
	missing := filepath.Join(t.TempDir(), "new", "config.json")
	scriptTUI(t, "back")
	if _, err = settingsMenu(missing); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("Cancel created settings")
	}
	// Explicit CLI mode overrides configured startup mode; no arguments use it.
	dir := t.TempDir()
	ssh := filepath.Join(dir, "ssh-config")
	writeTestFile(t, ssh, "Host demo\n HostName 192.0.2.1\n")
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	old := runTUI
	defer func() { runTUI = old }()
	for _, tc := range []struct{ mode, want string }{{"", "rdp"}, {"ssh", "ssh"}, {"tunnels", "tunnels"}} {
		runTUI = func(model tea.Model) (tea.Model, error) {
			m := model.(*pickerModel)
			if m.state.Mode != tc.want {
				t.Fatalf("startup got %s want %s", m.state.Mode, tc.want)
			}
			m.key = "cancel"
			return m, nil
		}
		args := []string{"--config", path, "--ssh-config", ssh, "--no-popup"}
		if tc.mode != "" {
			args = append([]string{tc.mode}, args...)
		}
		if err = app(args); err != nil {
			t.Fatal(err)
		}
	}
}
