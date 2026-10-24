package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type InventorySource struct {
	ReadOnly bool   `json:"read_only,omitempty"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Path     string `json:"path"`
}

type Connection struct {
	SourceHost             *ProfileReference `json:"source_host,omitempty"`
	DestinationHost        *ProfileReference `json:"destination_host,omitempty"`
	OriginalProfile        *Connection       `json:"-"`
	ProfileID              string            `json:"profile_id,omitempty"`
	HostID                 string            `json:"host_id,omitempty"`
	ProfileName            string            `json:"profile_name,omitempty"`
	Default                bool              `json:"default,omitempty"`
	ExtraHostKeyAlgorithms []string          `json:"extra_host_key_algorithms,omitempty"`
	ExtraPubkeyAlgorithms  []string          `json:"extra_pubkey_algorithms,omitempty"`
	IdentitiesOnly         *bool             `json:"identities_only,omitempty"`
	ExtraKexAlgorithms     []string          `json:"extra_kex_algorithms,omitempty"`
	SSHAgent               string            `json:"ssh_agent,omitempty"`
	Type                   string            `json:"type,omitempty"`
	Domain                 *string           `json:"domain,omitempty"`
	Account                string            `json:"account,omitempty"`
	TunnelType             string            `json:"tunnel_type,omitempty"`
	Bind                   string            `json:"local_bind,omitempty"`
	LocalPort              int               `json:"local_port,omitempty"`
	RemoteHost             string            `json:"remote_host,omitempty"`
	RemoteHostID           string            `json:"remote_host_id,omitempty"`
	RemotePort             int               `json:"remote_port,omitempty"`
	SSHID                  string            `json:"ssh_id,omitempty"`
	Resource               string            `json:"resource,omitempty"`
	Namespace              string            `json:"namespace,omitempty"`
	Context                string            `json:"context,omitempty"`
	ID                     string            `json:"id"`
	Alias                  string            `json:"alias"`
	Address                string            `json:"address,omitempty"`
	Port                   int               `json:"port"`
	User                   string            `json:"user,omitempty"`
	IdentityFile           string            `json:"identity_file,omitempty"`
	JumpID                 string            `json:"jump_id,omitempty"`
	Tags                   []string          `json:"tags,omitempty"`
	Notes                  string            `json:"notes,omitempty"`
}

func (r Connection) kind() string {
	if r.Type == "" {
		return "ssh"
	}
	return r.Type
}

type ConnectionOverride struct {
	Account                *string           `json:"account,omitempty"`
	Domain                 *string           `json:"domain,omitempty"`
	ExtraHostKeyAlgorithms *[]string         `json:"extra_host_key_algorithms,omitempty"`
	ExtraPubkeyAlgorithms  *[]string         `json:"extra_pubkey_algorithms,omitempty"`
	IdentitiesOnly         *bool             `json:"identities_only,omitempty"`
	ExtraKexAlgorithms     *[]string         `json:"extra_kex_algorithms,omitempty"`
	SSHAgent               *string           `json:"ssh_agent,omitempty"`
	Variables              map[string]string `json:"variables,omitempty"`
	User                   *string           `json:"user,omitempty"`
	IdentityFile           *string           `json:"identity_file,omitempty"`
	Address                *string           `json:"address,omitempty"`
	Port                   *int              `json:"port,omitempty"`
	JumpID                 *string           `json:"jump_id,omitempty"`
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
	if s.ReadOnly {
		q.Set("mode", "ro")
	}
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
	if version < 1 || version > 7 {
		return fail(fmt.Errorf("unsupported inventory schema %d (expected 1 through 7)", version))
	}
	if s.ReadOnly && version < 7 {
		return fail(fmt.Errorf("read-only source needs schema 7; migrate using a writable copy"))
	}
	var id string
	if err = db.QueryRow("SELECT id FROM source").Scan(&id); err != nil {
		return fail(err)
	}
	if id != s.ID {
		return fail(fmt.Errorf("database identity changed; expected %s, found %s", s.ID, id))
	}
	if version < 6 {
		if err = migrateInventoryHosts(db, version); err != nil {
			return fail(err)
		}
	}
	if version < 7 {
		if err = migrateReusableProfiles(db); err != nil {
			return fail(err)
		}
	}
	return db, nil
}

func migrateInventoryV2(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	records, err := readConnections(tx)
	if err != nil {
		return err
	}
	if err = validateConnections(records); err != nil {
		return err
	}
	for _, r := range records {
		if r.kind() != "ssh" {
			return fmt.Errorf("schema 1 contains a non-SSH record; refusing migration")
		}
		r.Type = "ssh"
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE connections SET data=? WHERE id=?", string(data), r.ID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("PRAGMA user_version=2"); err != nil {
		return err
	}
	return tx.Commit()
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
		hostTableSQL,
		profileTableSQL,
		"PRAGMA user_version=7",
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
	records, err := readHostConnections(db)
	if err != nil {
		return nil, err
	}
	if err = validateHostProfiles(db, records); err != nil {
		return nil, err
	}
	return records, nil
}

func validateConnections(records []Connection) error {
	byID := map[string]Connection{}
	aliases := map[string]bool{}
	for _, c := range records {
		if !inventoryName(c.ID) || !inventoryName(c.Alias) {
			return fmt.Errorf("%s: ID, valid alias, address and port (1–65535) required", c.Alias)
		}
		if !validAgentChoice(c.SSHAgent) {
			return fmt.Errorf("%s: invalid SSH agent selection", c.Alias)
		}
		if err := validateExtraKex(c.ExtraKexAlgorithms); err != nil {
			return fmt.Errorf("%s: %w", c.Alias, err)
		}
		for directive, names := range map[string][]string{"HostKeyAlgorithms": c.ExtraHostKeyAlgorithms, "PubkeyAcceptedAlgorithms": c.ExtraPubkeyAlgorithms} {
			if err := validateExtraAlgorithms(directive, names); err != nil {
				return fmt.Errorf("%s: %w", c.Alias, err)
			}
		}
		switch c.kind() {
		case "ssh", "rdp":
			if c.Address == "" || c.Port < 1 || c.Port > 65535 {
				return fmt.Errorf("%s: address and valid port required", c.Alias)
			}
		case "tunnel":
			if c.LocalPort < 1 || c.LocalPort > 65535 || c.RemotePort < 1 || c.RemotePort > 65535 {
				return fmt.Errorf("%s: valid local and remote ports required", c.Alias)
			}
			if c.TunnelType != "ssh" && c.TunnelType != "kubectl" && c.TunnelType != "bridge" {
				return fmt.Errorf("%s: tunnel type must be ssh or kubectl", c.Alias)
			}
			if (c.TunnelType == "ssh" || c.TunnelType == "bridge") && ((c.SSHID == "" && c.SourceHost == nil) || (c.RemoteHost == "" && c.RemoteHostID == "")) {
				return fmt.Errorf("%s: SSH connection and remote host required", c.Alias)
			}
			if c.TunnelType == "bridge" && c.DestinationHost == nil {
				return fmt.Errorf("bridge requires a destination host")
			}
			for _, ref := range []*ProfileReference{c.SourceHost, c.DestinationHost} {
				if ref != nil && (!inventoryName(ref.Source) || !inventoryName(ref.ID)) {
					return fmt.Errorf("invalid tunnel host reference")
				}
			}
			if c.TunnelType == "kubectl" && c.Resource == "" {
				return fmt.Errorf("%s: Kubernetes resource required", c.Alias)
			}
			if c.Bind != "" && !strings.Contains(c.Bind, "${") {
				ip := net.ParseIP(c.Bind)
				if ip == nil || !ip.IsLoopback() {
					return fmt.Errorf("%s: bind must be a loopback IP", c.Alias)
				}
			}
		default:
			return fmt.Errorf("%s: unknown connection type %s", c.Alias, c.Type)
		}
		if _, ok := byID[c.ID]; ok {
			return fmt.Errorf("duplicate connection ID %s", c.ID)
		}
		if aliases[c.Alias] {
			return fmt.Errorf("duplicate alias %s", c.Alias)
		}
		domain := ""
		if c.Domain != nil {
			domain = *c.Domain
		}
		for _, v := range []string{c.Address, c.User, c.IdentityFile, c.Notes, strings.Join(c.Tags, ""), domain, c.Account, c.Bind, c.RemoteHost, c.Resource, c.Namespace, c.Context} {
			if clean(v) != v {
				return fmt.Errorf("%s: multiline/control values are not supported", c.Alias)
			}
		}
		byID[c.ID] = c
		aliases[c.Alias] = true
	}
	for _, c := range records {
		for _, ref := range []string{c.JumpID, c.SSHID} {
			if ref != "" {
				r, ok := byID[ref]
				if !ok || r.kind() != "ssh" {
					return fmt.Errorf("%s: referenced connection must be an existing SSH record", c.Alias)
				}
			}
		}
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
		if c.kind() != "tunnel" {
			if c.HostID == "" {
				c.HostID, c.ProfileName, c.Default = c.ID, "Default", true
				if err = writeInventoryHost(tx, InventoryHost{ID: c.HostID, Alias: c.Alias, Address: c.Address, Tags: c.Tags, Notes: c.Notes}); err != nil {
					return err
				}
			}
			c.Address, c.Tags, c.Notes = "", nil, ""
			if err = persistProfileEdit(tx, &c); err != nil {
				return err
			}
			c = profileAttachment(c)
		}
		data, e := json.Marshal(c)
		if e != nil {
			return e
		}
		_, err = tx.Exec("INSERT INTO connections(id,alias,data) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET alias=excluded.alias,data=excluded.data", c.ID, c.Alias, string(data))
	}
	if err != nil {
		return err
	}
	records, err := readHostConnections(tx)
	if err != nil {
		return err
	}
	if err = validateConnections(records); err != nil {
		return err
	}
	if err = validateHostProfiles(tx, records); err != nil {
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
		if !sourceEnabled(c, s.ID) {
			continue
		}
		inventory, err := sourceInventoryHosts(s)
		if err != nil {
			return nil, err
		}
		byID := map[string]InventoryHost{}
		for _, host := range inventory {
			byID[host.ID] = host
		}
		records, err := effectiveSourceConnections(c, s)
		if err != nil {
			return nil, err
		}
		if err = validateConnections(records); err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
		configured := map[string]bool{}
		for _, r := range records {
			configured[r.HostID] = true
			if r.kind() != "tunnel" && !r.Default {
				continue
			}
			alias := r.Alias
			if r.HostID != "" {
				alias = byID[r.HostID].Alias
			}
			color := byID[r.HostID].Color
			bracketColor := byID[r.HostID].BracketColor
			if r.kind() == "tunnel" && r.SSHID != "" {
				for _, p := range records {
					if p.ID == r.SSHID {
						color = byID[p.HostID].Color
						bracketColor = byID[p.HostID].BracketColor
						break
					}
				}
			}
			hosts = append(hosts, Host{BracketColor: bracketColor, Color: color, Alias: alias, Hostname: r.Address, Comment: r.Notes, Tags: r.Tags, SourceID: s.ID, SourceName: s.Name, ConnectionID: r.ID, InventoryHostID: r.HostID, ProfileName: r.ProfileName, Type: r.kind()})
		}
		for _, host := range inventory {
			if !configured[host.ID] {
				hosts = append(hosts, Host{BracketColor: host.BracketColor, Color: host.Color, Alias: host.Alias, Hostname: host.Address, Tags: host.Tags, Comment: host.Notes, SourceID: s.ID, SourceName: s.Name, ConnectionID: host.ID, InventoryHostID: host.ID, Type: "host"})
			}
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

func managedHostMode(h Host) string {
	switch h.Type {
	case "rdp":
		return "rdp"
	case "tunnel":
		return "tunnels"
	default:
		return "ssh"
	}
}

func isTransferMode(mode string) bool {
	return mode == "scp" || mode == "legacy-scp" || mode == "rsync"
}

func managedModeEligible(h Host, mode string) bool {
	if isTransferMode(mode) {
		return h.Type == "ssh" || h.Type == ""
	}
	return managedHostMode(h) == mode
}
