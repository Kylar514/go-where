//go:build linux

package tunnelcore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestStartCapturesEarlyFailureAndAllowsSeparateLoopbacks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "ss"), []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	Register("test-failure", func(Connection) (*exec.Cmd, error) {
		return exec.Command("sh", "-c", "echo 'host key changed' >&2; exit 1"), nil
	})
	if _, err := Start(Connection{Name: "failure", TunnelType: "test-failure", LocalPort: 12345}); err == nil {
		t.Fatal("early exit not reported")
	}
	path, _ := LogPath("failure")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "host key changed") {
		t.Fatal("stderr missing", err)
	}
	Register("test-sleep", func(Connection) (*exec.Cmd, error) { return exec.Command("sleep", "30"), nil })
	for _, c := range []Connection{{Name: "one", TunnelType: "test-sleep", Bind: "127.0.10.1", LocalPort: 27017}, {Name: "two", TunnelType: "test-sleep", Bind: "127.0.20.1", LocalPort: 27017}} {
		if _, err := Start(c); err != nil {
			t.Fatal("different bind blocked", err)
		}
		defer Stop(c)
	}
}
