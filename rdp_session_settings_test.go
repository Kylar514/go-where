package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRDPSessionFoldersOptInAndValidation(t *testing.T) {
	on := true
	s := RDPSessionSettings{Clipboard: &on, Folders: []RDPFolder{{Name: "Files", Path: filepath.Join(t.TempDir(), "missing")}}}
	args, err := s.clientArgs()
	if err != nil || len(args) != 1 || args[0] != "+clipboard" {
		t.Fatal(args, err)
	}
	s.Folders[0].Enabled = true
	if _, err = s.clientArgs(); err == nil {
		t.Fatal("missing enabled folder accepted")
	}
	s.Folders[0].Path = t.TempDir()
	args, err = s.clientArgs()
	if err != nil || !strings.HasPrefix(args[1], "/drive:Files,") {
		t.Fatal(args, err)
	}
	s.Folders = append(s.Folders, s.Folders[0])
	if s.validate() == nil {
		t.Fatal("duplicate remote names accepted")
	}
}

func TestExplicitRDPSessionLegacyFlags(t *testing.T) {
	r := defaultRDPConfig()
	r.ClientArgs = []string{"/f", "+clipboard", "/dynamic-resolution", "/clipboard:use-selection"}
	got := explicitRDPSession(r)
	if !*got.Session.Clipboard || !*got.Session.Fullscreen {
		t.Fatal("legacy flags lost")
	}
	if !reflect.DeepEqual(got.ClientArgs, []string{"/dynamic-resolution", "/clipboard:use-selection"}) {
		t.Fatal("unrecognized arguments lost", got.ClientArgs)
	}
	off := false
	r.Session.Clipboard = &off
	r.Session.Fullscreen = &off
	got = explicitRDPSession(r)
	if *got.Session.Clipboard || *got.Session.Fullscreen {
		t.Fatal("explicit settings overridden")
	}
	if !reflect.DeepEqual(got, explicitRDPSession(got)) {
		t.Fatal("migration not idempotent")
	}
	got = explicitRDPSession(defaultRDPConfig())
	if *got.Session.Clipboard || *got.Session.Fullscreen {
		t.Fatal("fresh settings should be clipboard disabled and windowed")
	}
}

func TestRDPSessionDatabaseMigrationPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.db")
	db, err := openSettingsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("UPDATE app_settings SET data=? WHERE id=1", `{"rdp":{"client_args":["/f","+clipboard","/dynamic-resolution"]}}`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.RDP.Session.Clipboard == nil || !*c.RDP.Session.Clipboard || c.RDP.Session.Fullscreen == nil || !*c.RDP.Session.Fullscreen {
		t.Fatal(c.RDP.Session)
	}
	if !reflect.DeepEqual(c.RDP.ClientArgs, []string{"/dynamic-resolution"}) {
		t.Fatal(c.RDP.ClientArgs)
	}
	first, err := readSettingsData(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := readSettingsData(path)
	if err != nil || string(first) != string(second) {
		t.Fatal("reopening changed migrated settings", err)
	}
}

func TestExplicitSessionLaunchRemovesCompetingFlags(t *testing.T) {
	r := defaultRDPConfig()
	off := false
	r.Session.Clipboard = &off
	r.Session.Fullscreen = &off
	r.ClientArgs = []string{"/f", "+clipboard", "/dynamic-resolution"}
	job, err := newRDPJob(r, Host{Alias: "host", Hostname: "192.0.2.1"}, "")
	if err != nil {
		t.Fatal(err)
	}
	args := job.ClientArgs
	if !reflect.DeepEqual(args, []string{"/dynamic-resolution", "-clipboard"}) {
		t.Fatal("competing flags reached client", args)
	}
}
