//go:build linux

package tunnelcore

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRegistryIsolationAndPIDReuseProtection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	if err := os.MkdirAll(filepath.Join(dir, "diglet"), 0700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, "diglet", "state.json")
	if err := os.WriteFile(legacy, []byte("legacy untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sleep", "30")
	prepareCommand(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	identity, err := processIdentity(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	err = withState(func(s *stateFile) (bool, error) {
		s.Tunnels["stale"] = stateEntry{PID: cmd.Process.Pid, Identity: identity + 1}
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = Stop(Connection{Name: "stale"}); err != nil {
		t.Fatal(err)
	}
	if _, err = processIdentity(cmd.Process.Pid); err != nil {
		t.Fatal("stale identity killed unrelated process", err)
	}
	data, err := os.ReadFile(legacy)
	if err != nil || string(data) != "legacy untouched" {
		t.Fatal("legacy registry changed", err)
	}
}
