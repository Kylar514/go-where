package main

import "testing"

func TestSharedFolderBackDiscardsChildAndApplyCommits(t *testing.T) {
	s := RDPSessionSettings{Folders: []RDPFolder{{Name: "Files", Path: "/example", Enabled: false}}}
	scriptTUI(t, "toggle", "back")
	if err := editSharedFolder(&s, 0); err != nil {
		t.Fatal(err)
	}
	if s.Folders[0].Enabled {
		t.Fatal("Back changed parent draft")
	}
	scriptTUI(t, "toggle", "save")
	if err := editSharedFolder(&s, 0); err != nil {
		t.Fatal(err)
	}
	if !s.Folders[0].Enabled {
		t.Fatal("Apply ignored change")
	}
}
