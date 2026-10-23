package main

import (
	"path/filepath"
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
