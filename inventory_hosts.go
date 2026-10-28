package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

const hostTableSQL = "CREATE TABLE hosts (id TEXT PRIMARY KEY NOT NULL, alias TEXT UNIQUE NOT NULL, data TEXT NOT NULL)"

type InventoryHost struct {
	BracketColor string   `json:"bracket_color,omitempty"`
	Color        string   `json:"color,omitempty"`
	ID           string   `json:"id"`
	Alias        string   `json:"alias"`
	Address      string   `json:"address"`
	Tags         []string `json:"tags,omitempty"`
	Notes        string   `json:"notes,omitempty"`
}

type HostOverride struct {
	Address *string `json:"address,omitempty"`
}

func hostAddress(c Config, sourceID string, h InventoryHost) (string, error) {
	address := h.Address
	if o := c.HostOverrides[sourceID][h.ID]; o.Address != nil {
		address = *o.Address
	}
	return resolveValue(address, c.Variables)
}

func readInventoryHosts(db inventoryReader) ([]InventoryHost, error) {
	rows, err := db.Query("SELECT id,alias,data FROM hosts ORDER BY alias COLLATE NOCASE,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []InventoryHost
	for rows.Next() {
		var id, alias, data string
		var h InventoryHost
		if err = rows.Scan(&id, &alias, &data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(data), &h); err != nil {
			return nil, err
		}
		if err = validateHostColor(h.Color); err != nil {
			return nil, err
		}
		if err = validateBracketColor(h.BracketColor); err != nil {
			return nil, err
		}
		if h.ID != id || h.Alias != alias || !inventoryName(id) || !inventoryName(alias) || h.Address == "" || clean(h.Address) != h.Address || clean(h.Notes) != h.Notes {
			return nil, fmt.Errorf("invalid host %s", id)
		}
		for _, tag := range h.Tags {
			if clean(tag) != tag {
				return nil, fmt.Errorf("invalid host tag")
			}
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func writeInventoryHost(tx *sql.Tx, h InventoryHost) error {
	if err := validateHostColor(h.Color); err != nil {
		return err
	}
	if err := validateBracketColor(h.BracketColor); err != nil {
		return err
	}
	if !inventoryName(h.ID) || !inventoryName(h.Alias) || h.Address == "" || clean(h.Address) != h.Address || clean(h.Notes) != h.Notes {
		return fmt.Errorf("valid host name and address required")
	}
	data, err := json.Marshal(h)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO hosts(id,alias,data) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET alias=excluded.alias,data=excluded.data", h.ID, h.Alias, string(data))
	return err
}

func readHostConnections(db inventoryReader) ([]Connection, error) {
	records, err := readConnections(db)
	if err != nil {
		return nil, err
	}
	hosts, err := readInventoryHosts(db)
	if err != nil {
		return nil, err
	}
	byID := map[string]InventoryHost{}
	for _, h := range hosts {
		byID[h.ID] = h
	}
	var version int
	// Legacy host migration runs inside a transaction before profiles exists.
	vr, err := db.Query("PRAGMA user_version")
	if err != nil {
		return nil, err
	}
	if vr.Next() {
		err = vr.Scan(&version)
	}
	vr.Close()
	if err != nil {
		return nil, err
	}
	shared := map[string]Connection{}
	if version >= 7 {
		profiles, e := readReusableProfiles(db)
		if e != nil {
			return nil, e
		}
		for _, p := range profiles {
			shared[p.ID] = p
		}
	}
	for i := range records {
		r := &records[i]
		if r.kind() == "tunnel" {
			if r.RemoteHostID != "" {
				if _, ok := byID[r.RemoteHostID]; !ok {
					return nil, fmt.Errorf("%s: missing remote target host", r.Alias)
				}
			}
			continue
		}
		if version >= 7 {
			p, ok := shared[r.ProfileID]
			if !ok {
				return nil, fmt.Errorf("%s: missing reusable profile", r.Alias)
			}
			*r = applyProfile(*r, p)
		}
		h, ok := byID[r.HostID]
		if !ok {
			return nil, fmt.Errorf("%s: missing host %s", r.Alias, r.HostID)
		}
		r.Address, r.Tags, r.Notes = h.Address, h.Tags, h.Notes
	}
	return records, nil
}

func validateHostProfiles(db inventoryReader, records []Connection) error {
	_, err := readInventoryHosts(db)
	if err != nil {
		return err
	}
	groups := map[string]int{}
	defaults := map[string]int{}
	names := map[string]bool{}
	for _, r := range records {
		if r.kind() == "tunnel" {
			continue
		}
		if r.HostID == "" || !inventoryName(r.ProfileName) {
			return fmt.Errorf("%s: valid host and profile name required", r.Alias)
		}
		group := r.HostID + ":" + r.kind()
		name := group + ":" + r.ProfileName
		if names[name] {
			return fmt.Errorf("duplicate profile name %s", r.ProfileName)
		}
		names[name] = true
		groups[group]++
		if r.Default {
			defaults[group]++
		}
	}
	for group := range groups {
		if defaults[group] != 1 {
			return fmt.Errorf("%s: exactly one default profile required", group)
		}
	}
	return nil
}

// Preserve connection IDs so existing jumps, tunnels and local overrides remain
// pinned to the original profile. Equal addresses never imply the same host.
func migrateInventoryHosts(db *sql.DB, version int) error {
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
	if _, err = tx.Exec(hostTableSQL); err != nil {
		return err
	}
	for _, r := range records {
		if version == 1 && r.kind() != "ssh" {
			return fmt.Errorf("schema 1 contains a non-SSH record")
		}
		if r.kind() == "tunnel" {
			continue
		}
		if err = writeInventoryHost(tx, InventoryHost{ID: r.ID, Alias: r.Alias, Address: r.Address, Tags: r.Tags, Notes: r.Notes}); err != nil {
			return err
		}
		r.HostID, r.ProfileName, r.Default, r.Type = r.ID, "Default", true, r.kind()
		r.Address, r.Tags, r.Notes = "", nil, ""
		data, e := json.Marshal(r)
		if e != nil {
			return e
		}
		if _, err = tx.Exec("UPDATE connections SET data=? WHERE id=?", string(data), r.ID); err != nil {
			return err
		}
	}
	records, err = readHostConnections(tx)
	if err != nil {
		return err
	}
	if err = validateHostProfiles(tx, records); err != nil {
		return err
	}
	if _, err = tx.Exec("PRAGMA user_version=6"); err != nil {
		return err
	}
	return tx.Commit()
}

func sourceInventoryHosts(s InventorySource) ([]InventoryHost, error) {
	db, err := openInventory(s)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return readInventoryHosts(db)
}

// Host and every profile are committed together, with whole-graph validation.
func saveInventoryHost(s InventorySource, h InventoryHost, profiles []Connection) error {
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
	if err = writeInventoryHost(tx, h); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM connections WHERE json_extract(data,'$.host_id')=?", h.ID); err != nil {
		return err
	}
	for _, r := range profiles {
		if r.HostID != h.ID || (r.kind() != "ssh" && r.kind() != "rdp") {
			return fmt.Errorf("invalid host profile")
		}
		if err = persistProfileEdit(tx, &r); err != nil {
			return err
		}
		r = profileAttachment(r)
		r.Address, r.Tags, r.Notes = "", nil, ""
		data, e := json.Marshal(r)
		if e != nil {
			return e
		}
		if _, err = tx.Exec("INSERT INTO connections(id,alias,data) VALUES(?,?,?)", r.ID, r.Alias, string(data)); err != nil {
			return err
		}
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

func deleteInventoryHost(s InventorySource, id string) error {
	if s.ReadOnly {
		return fmt.Errorf("source is read-only")
	}
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
	if _, err = tx.Exec("DELETE FROM connections WHERE json_extract(data,'$.host_id')=?", id); err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM hosts WHERE id=?", id); err != nil {
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
