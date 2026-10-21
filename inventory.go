package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type InventorySource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
}

type Connection struct {
	ID           string   `json:"id"`
	Alias        string   `json:"alias"`
	Address      string   `json:"address"`
	Port         int      `json:"port"`
	User         string   `json:"user,omitempty"`
	IdentityFile string   `json:"identity_file,omitempty"`
	JumpID       string   `json:"jump_id,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Notes        string   `json:"notes,omitempty"`
}

type ConnectionOverride struct {
	Variables    map[string]string `json:"variables,omitempty"`
	User         *string           `json:"user,omitempty"`
	IdentityFile *string           `json:"identity_file,omitempty"`
	Address      *string           `json:"address,omitempty"`
	Port         *int              `json:"port,omitempty"`
	JumpID       *string           `json:"jump_id,omitempty"`
}

func inventoryID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func sourcePath(s InventorySource) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(expandHome(s.Path, home))
}

// Opening a configured source never creates it. New databases require an explicit action.
func openInventory(s InventorySource) (*sql.DB, error) {
	path, err := sourcePath(s)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := url.Values{"mode": {"rw"}, "_pragma": {"busy_timeout(5000)"}}
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*sql.DB, error) { db.Close(); return nil, fmt.Errorf("source %s: %w", s.Name, err) }
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version != 1 {
		return fail(fmt.Errorf("unsupported inventory schema %d (expected 1)", version))
	}
	var id string
	if err = db.QueryRow("SELECT id FROM source").Scan(&id); err != nil {
		return fail(err)
	}
	if id != s.ID {
		return fail(fmt.Errorf("database identity changed; expected %s, found %s", s.ID, id))
	}
	return db, nil
}

func createInventory(name, path string) (InventorySource, error) {
	s := InventorySource{ID: inventoryID(), Name: strings.TrimSpace(name), Type: "sqlite", Path: path}
	if s.Name == "" || path == "" {
		return s, fmt.Errorf("source name and path are required")
	}
	p, err := sourcePath(s)
	if err != nil {
		return s, err
	}
	s.Path = p
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return s, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return s, err
	}
	if err = f.Close(); err != nil {
		return s, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p), RawQuery: "mode=rw"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return s, err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return s, err
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		"CREATE TABLE source (id TEXT PRIMARY KEY NOT NULL)",
		"CREATE TABLE connections (id TEXT PRIMARY KEY NOT NULL, alias TEXT UNIQUE NOT NULL, data TEXT NOT NULL)",
		"PRAGMA user_version=1",
	} {
		if _, err = tx.Exec(stmt); err != nil {
			return s, err
		}
	}
	if _, err = tx.Exec("INSERT INTO source(id) VALUES(?)", s.ID); err != nil {
		return s, err
	}
	return s, tx.Commit()
}

type inventoryReader interface {
	Query(string, ...any) (*sql.Rows, error)
}

func readConnections(db inventoryReader) ([]Connection, error) {
	rows, err := db.Query("SELECT id,alias,data FROM connections ORDER BY alias COLLATE NOCASE,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Connection
	for rows.Next() {
		var id, alias, data string
		var c Connection
		if err = rows.Scan(&id, &alias, &data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(data), &c); err != nil {
			return nil, err
		}
		if c.ID != id || c.Alias != alias {
			return nil, fmt.Errorf("inconsistent connection %s", id)
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func sourceConnections(s InventorySource) ([]Connection, error) {
	db, err := openInventory(s)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return readConnections(db)
}

func validateConnections(records []Connection) error {
	byID := map[string]Connection{}
	aliases := map[string]bool{}
	for _, c := range records {
		if !inventoryName(c.ID) || !inventoryName(c.Alias) || c.Address == "" || c.Port < 1 || c.Port > 65535 {
			return fmt.Errorf("%s: ID, valid alias, address and port (1–65535) required", c.Alias)
		}
		if _, ok := byID[c.ID]; ok {
			return fmt.Errorf("duplicate connection ID %s", c.ID)
		}
		if aliases[c.Alias] {
			return fmt.Errorf("duplicate alias %s", c.Alias)
		}
		for _, v := range []string{c.Address, c.User, c.IdentityFile, c.Notes, strings.Join(c.Tags, "")} {
			if strings.ContainsAny(v, "\x00\r\n") {
				return fmt.Errorf("%s: multiline/control values are not supported", c.Alias)
			}
		}
		byID[c.ID] = c
		aliases[c.Alias] = true
	}
	for _, c := range records {
		seen := map[string]bool{}
		for id := c.ID; id != ""; {
			if seen[id] {
				return fmt.Errorf("%s: jump cycle", c.Alias)
			}
			seen[id] = true
			next, ok := byID[id]
			if !ok {
				return fmt.Errorf("%s: missing jump %s", c.Alias, id)
			}
			id = next.JumpID
		}
	}
	return nil
}

func inventoryName(v string) bool {
	if v == "" || strings.HasPrefix(v, "-") {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)) {
			return false
		}
	}
	return true
}

// Validate the whole graph inside the write transaction, including references on delete.
func changeConnection(s InventorySource, c Connection, remove bool) error {
	db, err := openInventory(s)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if remove {
		_, err = tx.Exec("DELETE FROM connections WHERE id=?", c.ID)
	} else {
		data, e := json.Marshal(c)
		if e != nil {
			return e
		}
		_, err = tx.Exec("INSERT INTO connections(id,alias,data) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET alias=excluded.alias,data=excluded.data", c.ID, c.Alias, string(data))
	}
	if err != nil {
		return err
	}
	records, err := readConnections(tx)
	if err != nil {
		return err
	}
	if err = validateConnections(records); err != nil {
		return err
	}
	return tx.Commit()
}

func findSource(c Config, id string) (InventorySource, error) {
	for _, s := range c.Sources {
		if s.ID == id {
			return s, nil
		}
	}
	return InventorySource{}, fmt.Errorf("source %s is not configured", id)
}

func inventoryHosts(c Config) ([]Host, error) {
	var hosts []Host
	for _, s := range c.Sources {
		records, err := sourceConnections(s)
		if err != nil {
			return nil, err
		}
		if err = validateConnections(records); err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		for _, r := range records {
			hosts = append(hosts, Host{Alias: r.Alias, Hostname: r.Address, Comment: r.Notes, SourceID: s.ID, SourceName: s.Name, ConnectionID: r.ID})
		}
	}
	return hosts, nil
}

func hostIdentity(h Host) string {
	if h.SourceID != "" {
		return h.SourceID + ":" + h.ConnectionID
	}
	return h.Alias // Preserve existing SSH-config picker state.
}
