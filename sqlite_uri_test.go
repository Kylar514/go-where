package main

import (
	"net/url"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSQLiteFileURL(t *testing.T) {
	for _, path := range []string{"C:/Users/example/AppData/Roaming/go-where/profile.db", "d:/folder with spaces/a#b?c%.db", "/tmp/folder with spaces/a#b?c%.db"} {
		u := sqliteFileURL(path)
		u.RawQuery = url.Values{"mode": {"ro"}, "_pragma": {"busy_timeout(5000)"}}.Encode()
		parsed, err := url.Parse(u.String())
		want := path
		if path[1] == ':' {
			want = "/" + path
		}
		if err != nil || parsed.Host != "" || parsed.Path != want || parsed.Fragment != "" || parsed.Query().Get("mode") != "ro" || parsed.Query().Get("_pragma") != "busy_timeout(5000)" {
			t.Fatal(path, u.String(), parsed, err)
		}
	}
	if runtime.GOOS == "windows" {
		u := sqliteFileURL(`C:\Users\example\profile.db`)
		if u.String() != "file:///C:/Users/example/profile.db" {
			t.Fatal(u.String())
		}
	}
}

func TestDatabasePathsWithReservedCharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile # percent%.db")
	db, err := openSettingsDB(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := createInventory("URI test", filepath.Join(t.TempDir(), "inventory # percent%.db"))
	if err != nil {
		t.Fatal(err)
	}
	attached, err := attachInventory(s.Name, s.Path)
	if err != nil || attached.ID != s.ID {
		t.Fatal(attached, err)
	}
	s.ReadOnly = true
	db, err = openInventory(s)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("DELETE FROM source"); err == nil {
		t.Fatal("read-only URI allowed writes")
	}
}
