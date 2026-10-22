package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedTransfersReuseSnapshotAndCleanUp(t *testing.T) {
	s := testInventory(t, "Transfer")
	jump := Connection{ID: "jump", Alias: "jump", Address: "192.0.2.1", Port: 2222, User: "gateway"}
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.2", Port: 22, User: "operator", JumpID: jump.ID}
	for _, p := range []Connection{jump, r} {
		if err := changeConnection(s, p, false); err != nil {
			t.Fatal(err)
		}
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	h := Host{SourceID: s.ID, ConnectionID: r.ID, Alias: r.Alias}
	ssh, err := managedJob(h, c)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err = os.WriteFile(file, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"scp", "legacy-scp", "rsync"} {
		for _, debug := range []bool{false, true} {
			c.debugLaunch = debug
			tx := Transfer{Mode: mode, Direction: "upload", Local: file, Remote: "/tmp/a file"}
			job, err := buildTransferJob(tx, h, "/must-not-use-personal-config", mode == "rsync", c)
			if err != nil {
				t.Fatal(err)
			}
			if job.Configuration != ssh.Configuration || !strings.Contains(strings.Join(job.Argv, " "), "gw-0:") {
				t.Fatal("wrong managed route", job)
			}
			argv, cleanup, err := prepareTransferJob(job)
			if err != nil {
				t.Fatal(err)
			}
			var path string
			if mode == "rsync" {
				if strings.Contains(strings.Join(argv, " "), transferSnapshotMarker) {
					t.Fatal("marker not replaced")
				} // snapshot path is safely quoted inside -e
				for i, arg := range argv {
					if arg == "-e" {
						for _, part := range strings.Split(argv[i+1], "'") {
							if strings.HasSuffix(part, "/config") {
								path = part
							}
						}
					}
				}
			} else {
				for i, arg := range argv {
					if arg == "-F" {
						path = argv[i+1]
					}
				}
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != ssh.Configuration {
				t.Fatal("snapshot mismatch", path, err)
			}
			cleanup()
			if _, err = os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("snapshot leaked")
			}
			if debug && !strings.Contains(strings.Join(argv, " "), "-vv") {
				t.Fatal("debug missing")
			}
			if mode == "legacy-scp" && !strings.Contains(strings.Join(argv, " "), "-O") {
				t.Fatal("legacy flag missing")
			}
		}
	}
	review, err := transferReview(Transfer{Mode: "scp", Direction: "upload", Local: file, Remote: "/tmp/file"}, []Host{h}, "current", "", c)
	if err != nil || !strings.Contains(review, "personal SSH config is not read") {
		t.Fatal(review, err)
	}
}
