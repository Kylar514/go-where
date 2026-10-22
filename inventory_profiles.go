package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
)

const profileTableSQL = "CREATE TABLE profiles (id TEXT PRIMARY KEY NOT NULL, data TEXT NOT NULL)"

// Reuse Connection's protocol fields; host/address/routing/defaults belong to
// attachments, never the shared profile.
func profileSettings(r Connection) Connection {
	r.OriginalProfile = nil
	r.HostID, r.ProfileID, r.JumpID, r.Address, r.Alias = "", "", "", "", ""
	r.Default = false
	r.Tags = nil
	r.Notes = ""
	return r
}

func applyProfile(a, p Connection) Connection {
	original := p
	p.OriginalProfile = &original
	p.ID, p.Alias, p.HostID, p.ProfileID, p.Default, p.JumpID = a.ID, a.Alias, a.HostID, a.ProfileID, a.Default, a.JumpID
	p.Address, p.Tags, p.Notes = a.Address, a.Tags, a.Notes
	return p
}

func persistProfileEdit(tx *sql.Tx, r *Connection) error {
	if r.ProfileID == "" {
		r.ProfileID = inventoryID()
		p := profileSettings(*r)
		p.ID = r.ProfileID
		return writeReusableProfile(tx, p)
	}
	p := profileSettings(*r)
	p.ID = r.ProfileID
	if r.OriginalProfile != nil {
		if reflect.DeepEqual(p, *r.OriginalProfile) {
			return nil
		}
		profiles, err := readReusableProfiles(tx)
		if err != nil {
			return err
		}
		found := false
		for _, current := range profiles {
			if current.ID == r.ProfileID {
				found = true
				if !reflect.DeepEqual(current, *r.OriginalProfile) {
					return fmt.Errorf("shared profile changed while editing; reopen host and retry")
				}
			}
		}
		if !found {
			return fmt.Errorf("shared profile no longer exists")
		}
	}
	return writeReusableProfile(tx, p)
}

func readReusableProfiles(db inventoryReader) ([]Connection, error) {
	rows, err := db.Query("SELECT id,data FROM profiles ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Connection
	for rows.Next() {
		var id, data string
		var p Connection
		if err = rows.Scan(&id, &data); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(data), &p); err != nil {
			return nil, err
		}
		if p.ID != id || !inventoryName(id) || !inventoryName(p.ProfileName) || (p.kind() != "ssh" && p.kind() != "rdp") || p.HostID != "" || p.JumpID != "" {
			return nil, fmt.Errorf("invalid shared profile %s", id)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func writeReusableProfile(tx *sql.Tx, p Connection) error {
	p = profileSettings(p)
	if !inventoryName(p.ID) || !inventoryName(p.ProfileName) || (p.kind() != "ssh" && p.kind() != "rdp") {
		return fmt.Errorf("valid profile name and protocol required")
	}
	check := p
	check.Alias = p.ID
	check.Address = "192.0.2.1"
	if err := validateConnections([]Connection{check}); err != nil {
		return err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO profiles(id,data) VALUES(?,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", p.ID, string(data))
	return err
}

func profileAttachment(r Connection) Connection {
	return Connection{ID: r.ID, Alias: r.Alias, HostID: r.HostID, ProfileID: r.ProfileID, Type: r.kind(), ProfileName: r.ProfileName, Default: r.Default, JumpID: r.JumpID}
}

func sourceProfiles(s InventorySource) ([]Connection, error) {
	db, err := openInventory(s)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return readReusableProfiles(db)
}

func migrateReusableProfiles(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(profileTableSQL); err != nil {
		return err
	}
	records, err := readConnections(tx)
	if err != nil {
		return err
	}
	for _, r := range records {
		if r.kind() == "tunnel" {
			continue
		}
		p := profileSettings(r)
		p.ID = r.ID
		// Existing Default profiles retain distinct names across hosts; no merging.
		if err = writeReusableProfile(tx, p); err != nil {
			return err
		}
		r.ProfileID = p.ID
		r = profileAttachment(r)
		data, e := json.Marshal(r)
		if e != nil {
			return e
		}
		if _, err = tx.Exec("UPDATE connections SET data=? WHERE id=?", string(data), r.ID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("PRAGMA user_version=7"); err != nil {
		return err
	}
	records, err = readHostConnections(tx)
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

func saveReusableProfile(s InventorySource, p Connection) error {
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
	if err = writeReusableProfile(tx, p); err != nil {
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
