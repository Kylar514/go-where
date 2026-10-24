package main

import "fmt"

func confirmSettingsDelete(name, source string) (bool, error) {
	_, back, err := choose("Delete "+name+"?", []choice{{"confirm", "Delete permanently"}}, "Delete "+name+" from "+source+". Referenced items cannot be deleted; nothing is reassigned automatically.")
	return !back && err == nil, err
}

func identityReferenced(c Config, kind, name string) bool {
	for _, values := range c.PersonalConnections {
		for key, v := range values {
			if len(key) < len(kind)+1 || key[len(key)-len(kind):] != kind {
				continue
			}
			if v.DefaultIdentity == name {
				return true
			}
			for _, id := range v.Identities {
				if id == name {
					return true
				}
			}
		}
	}
	if kind == "ssh" {
		for _, a := range c.IdentityAssignments {
			if a.Default == name {
				return true
			}
			for _, id := range a.Profiles {
				if id == name {
					return true
				}
			}
			for _, id := range a.Connections {
				if id == name {
					return true
				}
			}
		}
	}
	if kind == "rdp" {
		for _, values := range c.Overrides {
			for _, o := range values {
				if o.Account != nil && *o.Account == name {
					return true
				}
			}
		}
		for _, s := range c.Sources {
			rows, err := sourceConnections(s)
			if err != nil {
				return true
			}
			for _, r := range rows {
				if r.Account == name {
					return true
				}
			}
		}
	}
	return false
}

func deleteConnectionProfile(c Config, path string, s InventorySource, id string) error {
	if s.ReadOnly {
		return fmt.Errorf("source is read-only")
	}
	for _, values := range c.PersonalConnections {
		for _, v := range values {
			if v.Profile != nil && v.Profile.Source == s.ID && v.Profile.ID == id {
				return fmt.Errorf("profile is referenced by personal host selections")
			}
		}
	}
	if s.ID == "local" {
		original, before, draft, err := settingsDraft(path)
		if err != nil {
			return err
		}
		for _, values := range draft.PersonalConnections {
			for _, v := range values {
				if v.Profile != nil && v.Profile.Source == "local" && v.Profile.ID == id {
					return fmt.Errorf("profile is referenced")
				}
			}
		}
		delete(draft.LocalProfiles, id)
		return saveSettingsDraft(path, original, before, draft)
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
	rows, err := readHostConnections(tx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ProfileID == id {
			return fmt.Errorf("profile is used by host connections; reassign them first")
		}
	}
	if _, err = tx.Exec("DELETE FROM profiles WHERE id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}
