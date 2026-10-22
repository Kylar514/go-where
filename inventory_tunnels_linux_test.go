//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	digletconfig "github.com/kylar514/go-where/internal/tunnelcore"
)

func TestInventoryTunnelUsesLocalLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("PIDFILE", filepath.Join(dir, "pid"))
	fakeCommand(t, dir, "ssh", `printf '%s\n' "$$" > "$PIDFILE"; exec sleep 30`)
	fakeCommand(t, dir, "ss", `if [ -f "$PIDFILE" ]; then read -r pid < "$PIDFILE"; if kill -0 "$pid" 2>/dev/null; then printf 'LISTEN 0 128 127.0.0.10:8089 0.0.0.0:* users:(("ssh",pid=%s,fd=3))\n' "$pid"; fi; fi`)
	s := testInventory(t, "Lifecycle")
	for _, r := range []Connection{
		{ID: "gateway", Alias: "gateway", Address: "192.0.2.1", Port: 22},
		{ID: "web", Type: "tunnel", Alias: "web", TunnelType: "ssh", SSHID: "gateway", Bind: "127.0.0.10", LocalPort: 8089, RemoteHost: "192.0.2.2", RemotePort: 80},
	} {
		if err := changeConnection(s, r, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	h := Host{Type: "tunnel", SourceID: s.ID, ConnectionID: "web"}
	conn, err := inventoryTunnel(h, c)
	if err != nil {
		t.Fatal(err)
	}
	if err = toggleManagedTunnel(conn, false); err != nil {
		t.Fatal(err)
	}
	defer toggleManagedTunnel(conn, true)
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, e := tunnelStatuses([]digletconfig.Connection{conn})
		if e != nil {
			t.Fatal(e)
		}
		if status[conn.Name].status == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no active status: %v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = ensureInventoryTunnelStopped(h); err == nil {
		t.Fatal("editing an active tunnel was allowed")
	}
	path, err := tunnelSnapshotPath(conn.Name)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "192.0.2.1") {
		t.Fatal("SSH snapshot missing")
	}
	if err = toggleManagedTunnel(conn, true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("snapshot not removed on stop")
	}
	if err = ensureInventoryTunnelStopped(h); err != nil {
		t.Fatal(err)
	}
}
