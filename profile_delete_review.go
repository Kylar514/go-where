package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

func profileUsage(c Config, s InventorySource, id string) ([]string, error) {
	uses := map[string]bool{}
	for _, source := range c.Sources {
		hosts, err := sourceInventoryHosts(source)
		if err != nil {
			return nil, err
		}
		names := map[string]string{}
		for _, h := range hosts {
			names[h.ID] = h.Alias
		}
		if source.ID == s.ID {
			rows, err := sourceConnections(source)
			if err != nil {
				return nil, err
			}
			for _, r := range rows {
				if r.ProfileID == id {
					uses[names[r.HostID]+" — "+source.Name+" · "+r.ProfileName] = true
				}
			}
		}
		for key, v := range c.PersonalConnections[source.ID] {
			if v.Profile != nil && v.Profile.Source == s.ID && v.Profile.ID == id {
				hostID, _, _ := strings.Cut(key, ":")
				name := names[hostID]
				if name == "" {
					name = "Unavailable host " + hostID
				}
				uses[name+" — "+source.Name+" · personal selection"] = true
			}
		}
	}
	// Include references whose source registration no longer exists.
	for source, values := range c.PersonalConnections {
		if _, err := findSource(c, source); err == nil {
			continue
		}
		for key, v := range values {
			if v.Profile != nil && v.Profile.Source == s.ID && v.Profile.ID == id {
				uses["Unavailable host "+key+" — unregistered source"] = true
			}
		}
	}
	list := []string{}
	for label := range uses {
		list = append(list, label)
	}
	sort.Strings(list)
	return list, nil
}

func reviewDeleteProfile(c Config, path string, s InventorySource, p Connection) (bool, error) {
	if s.ReadOnly {
		return false, fmt.Errorf("source %s is read-only", s.Name)
	}
	// Include saved selections as well as unsaved parent Settings selections.
	loaded, err := loadConfig(path)
	if err != nil {
		return false, err
	}
	uses, err := profileUsage(c, s, p.ID)
	if err != nil {
		return false, err
	}
	saved, err := profileUsage(loaded, s, p.ID)
	if err != nil {
		return false, err
	}
	seen := map[string]bool{}
	for _, u := range append(uses, saved...) {
		seen[u] = true
	}
	uses = nil
	for u := range seen {
		uses = append(uses, u)
	}
	sort.Strings(uses)
	text := "Profile: " + p.ProfileName + "\nSource: " + s.Name
	if len(uses) == 0 {
		text += "\n\nThis profile is not in use."
	} else {
		text += "\n\nThis profile is in use by:\n" + strings.Join(uses, "\n")
	}
	options := []choice{{"no", "No — keep profile"}, {"yes", "Yes — delete profile"}}
	if len(uses) > 0 {
		options = []choice{{"no", "Cancel"}, {"replace", "Reassign and delete…"}}
	}
	id, back, err := choose("Delete profile?", options, text)
	if err != nil || back || id == "no" {
		return false, err
	}
	if id == "yes" {
		return true, deleteConnectionProfile(c, path, s, p.ID)
	}
	var profiles []Connection
	if s.ID == "local" {
		for _, candidate := range c.LocalProfiles {
			profiles = append(profiles, candidate)
		}
	} else {
		profiles, err = sourceProfiles(s)
		if err != nil {
			return false, err
		}
	}
	choices := []choice{}
	byID := map[string]Connection{}
	for _, candidate := range profiles {
		if candidate.ID != p.ID && candidate.kind() == p.kind() {
			choices = append(choices, choice{candidate.ID, candidate.ProfileName})
			byID[candidate.ID] = candidate
		}
	}
	if len(choices) == 0 {
		return false, fmt.Errorf("create another %s profile in %s before reassigning", p.kind(), s.Name)
	}
	replacement, back, err := chooseOne("Replacement profile", choices, "Replacement must belong to the same source and protocol so shared hosts never reference a private or foreign profile.", "")
	if err != nil || back {
		return false, err
	}
	action, back, err := choose("Confirm reassignment and deletion", []choice{{"confirm", "Reassign and delete"}, {"no", "Cancel"}}, text+"\n\nReplacement: "+byID[replacement].ProfileName+"\nExisting connection IDs, defaults, jumps and identities remain unchanged.")
	if err != nil || back || action == "no" {
		return false, err
	}
	return true, reassignDeleteProfile(path, s, p, byID[replacement])
}

func reassignDeleteProfile(path string, s InventorySource, old, replacement Connection) error {
	if s.ReadOnly || old.ID == replacement.ID || old.kind() != replacement.kind() {
		return fmt.Errorf("invalid replacement")
	}
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		return err
	}
	if assignments, ok := draft.IdentityAssignments[s.ID]; ok {
		if identity, ok := assignments.Profiles[old.ID]; ok {
			if existing, assigned := assignments.Profiles[replacement.ID]; assigned && existing != identity {
				return fmt.Errorf("replacement profile has a different identity assignment; reconcile assignments before deleting")
			}
			assignments.Profiles[replacement.ID] = identity
			delete(assignments.Profiles, old.ID)
			draft.IdentityAssignments[s.ID] = assignments
		}
	}
	for source, values := range draft.PersonalConnections {
		for key, v := range values {
			if v.Profile != nil && v.Profile.Source == s.ID && v.Profile.ID == old.ID {
				v.Profile = &ProfileReference{s.ID, replacement.ID}
				draft.PersonalConnections[source][key] = v
			}
		}
	}
	if s.ID == "local" {
		current, ok := draft.LocalProfiles[old.ID]
		if !ok || !reflect.DeepEqual(current, old) {
			return fmt.Errorf("profile changed; reopen deletion review")
		}
		if current, ok := draft.LocalProfiles[replacement.ID]; !ok || !reflect.DeepEqual(current, replacement) {
			return fmt.Errorf("replacement changed")
		}
		delete(draft.LocalProfiles, old.ID)
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
	profiles, err := readReusableProfiles(tx)
	if err != nil {
		return err
	}
	foundOld, foundNew := false, false
	for _, p := range profiles {
		if p.ID == old.ID {
			foundOld = reflect.DeepEqual(p, old)
		}
		if p.ID == replacement.ID {
			foundNew = reflect.DeepEqual(p, replacement)
		}
	}
	if !foundOld || !foundNew {
		return fmt.Errorf("profile changed; reopen deletion review")
	}
	rows, err := readHostConnections(tx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ProfileID != old.ID {
			continue
		}
		r.ProfileID = replacement.ID
		r = applyProfile(r, replacement)
		data, err := json.Marshal(profileAttachment(r))
		if err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE connections SET data=? WHERE id=?", string(data), r.ID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("DELETE FROM profiles WHERE id=?", old.ID); err != nil {
		return err
	}
	rows, err = readHostConnections(tx)
	if err != nil {
		return err
	}
	if err = validateConnections(rows); err != nil {
		return err
	}
	if err = validateHostProfiles(tx, rows); err != nil {
		return err
	}
	// Separate stores cannot share a transaction: local changes first remain
	// valid even if inventory commit fails, because replacement already exists.
	if err = saveSettingsDraft(path, original, before, draft); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("personal selections reassigned, inventory deletion failed: %w", err)
	}
	return nil
}
