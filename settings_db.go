package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/tailscale/hujson"
)

func databaseSettings(path string) bool { return strings.EqualFold(filepath.Ext(path), ".db") }

// Settings retain the existing validated Config contract. Inventory tables and
// private settings have independent schema versions and are never interchangeable.
func openSettingsDB(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	u.RawQuery = url.Values{"mode": {"rw"}, "_pragma": {"busy_timeout(5000)"}}.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*sql.DB, error) { db.Close(); return nil, fmt.Errorf("local settings: %w", err) }
	tx, err := db.Begin()
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()
	var app, version int
	if err = tx.QueryRow("PRAGMA application_id").Scan(&app); err != nil {
		return fail(err)
	}
	if err = tx.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	const settingsAppID = 0x47575354 // GWST
	if app != settingsAppID {
		var tables int
		if err = tx.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&tables); err != nil {
			return fail(err)
		}
		if app != 0 || version != 0 || tables != 0 {
			return fail(fmt.Errorf("not a go-where local settings database"))
		}
		data := []byte("{}")
		// Only the default profile.db imports its sibling legacy config. Never
		// rewrite or remove that file; it is the original migration backup.
		if filepath.Base(path) == "profile.db" {
			legacy := filepath.Join(filepath.Dir(path), "config.json")
			if b, e := os.ReadFile(legacy); e == nil {
				data = b
			} else if !os.IsNotExist(e) {
				return fail(e)
			}
		}
		if _, err = decodeConfig(data); err != nil {
			return fail(fmt.Errorf("legacy settings import: %w", err))
		}
		if _, err = tx.Exec("CREATE TABLE app_settings (id INTEGER PRIMARY KEY CHECK(id=1), data TEXT NOT NULL)"); err != nil {
			return fail(err)
		}
		if _, err = tx.Exec("INSERT INTO app_settings(id,data) VALUES(1,?)", string(data)); err != nil {
			return fail(err)
		}
		if _, err = tx.Exec(fmt.Sprintf("PRAGMA application_id=%d", settingsAppID)); err != nil {
			return fail(err)
		}
		if _, err = tx.Exec("PRAGMA user_version=1"); err != nil {
			return fail(err)
		}
	} else if version != 1 {
		return fail(fmt.Errorf("unsupported local settings schema %d", version))
	}
	if err = tx.Commit(); err != nil {
		return fail(err)
	}
	return db, nil
}

func readSettingsData(path string) ([]byte, error) {
	if !databaseSettings(path) {
		return os.ReadFile(path)
	}
	db, err := openSettingsDB(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var data string
	err = db.QueryRow("SELECT data FROM app_settings WHERE id=1").Scan(&data)
	return []byte(data), err
}

func modifyDatabaseSettings(path string, expected []byte, edit func(*hujson.Value, Config) error) error {
	db, err := openSettingsDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var stored string
	if err = tx.QueryRow("SELECT data FROM app_settings WHERE id=1").Scan(&stored); err != nil {
		return err
	}
	if expected != nil && !bytes.Equal([]byte(stored), expected) {
		return fmt.Errorf("settings changed while editing; reopen Settings and retry")
	}
	c, err := decodeConfig([]byte(stored))
	if err != nil {
		return err
	}
	v, err := parseSettings([]byte(stored))
	if err != nil {
		return err
	}
	if err = edit(&v, c); err != nil {
		return err
	}
	result := v.Pack()
	if _, err = decodeConfig(result); err != nil {
		return err
	}
	// The conditional update also protects drafts from other processes. SQLite
	// refuses a stale read-to-write transaction rather than losing their changes.
	if _, err = tx.Exec("UPDATE app_settings SET data=? WHERE id=1 AND data=?", string(result), stored); err != nil {
		return err
	}
	return tx.Commit()
}
