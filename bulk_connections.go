package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// All shared changes for a source commit together, including graph validation.
func applyBulkConnections(s InventorySource, before, after []Connection) error {
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
	current, err := readHostConnections(tx)
	if err != nil {
		return err
	}
	byID := map[string]Connection{}
	for _, r := range current {
		byID[r.ID] = r
	}
	for i, old := range before {
		if !reflect.DeepEqual(byID[old.ID], old) {
			return fmt.Errorf("connection changed while reviewing; reopen bulk edit")
		}
		r := profileAttachment(after[i])
		data, e := json.Marshal(r)
		if e != nil {
			return e
		}
		if _, err = tx.Exec("UPDATE connections SET data=? WHERE id=?", string(data), r.ID); err != nil {
			return err
		}
	}
	current, err = readHostConnections(tx)
	if err != nil {
		return err
	}
	if err = validateConnections(current); err != nil {
		return err
	}
	if err = validateHostProfiles(tx, current); err != nil {
		return err
	}
	return tx.Commit()
}

func bulkEditConnections(c Config, state PickerState, selected []Host) error {
	if databaseSettings(state.ConfigPath) {
		return editPersonalHosts(c, state.ConfigPath, selected)
	}
	if len(selected) == 0 {
		return nil
	}
	source := selected[0].SourceID
	if source == "" {
		return fmt.Errorf("bulk edit requires database hosts")
	}
	hostIDs := map[string]bool{}
	for _, h := range selected {
		if h.SourceID != source {
			return fmt.Errorf("bulk edit one source at a time for atomic changes")
		}
		_, r, err := storedRecord(h, c)
		if err != nil {
			return err
		}
		hostIDs[r.HostID] = true
	}
	s, err := findSource(c, source)
	if err != nil {
		return err
	}
	kind, back, err := chooseOne("Bulk protocol", []choice{{"ssh", "SSH"}, {"rdp", "RDP"}}, "Choose the protocol to change.", "ssh")
	if err != nil || back {
		return err
	}
	scope, back, err := chooseOne("Bulk scope", []choice{{"default", "Default connections"}, {"all", "All connections of this protocol"}}, "Only selected hosts in this source are affected.", "default")
	if err != nil || back {
		return err
	}
	rows, err := sourceConnections(s)
	if err != nil {
		return err
	}
	var before []Connection
	for _, r := range rows {
		if hostIDs[r.HostID] && r.kind() == kind && (scope == "all" || r.Default) {
			before = append(before, r)
		}
	}
	if len(before) == 0 {
		return fmt.Errorf("selected hosts have no matching connections")
	}
	action, back, err := choose("Bulk edit", []choice{{"profile", "Connection profile"}, {"identity", "Local identity"}, {"jump", "Jump host"}}, "Profiles and routes save to shared inventory. Identities save only to local settings. Each operation previews its complete batch.")
	if err != nil || back {
		return err
	}
	after := append([]Connection(nil), before...)
	var localOriginal []byte
	var localBefore, localDraft Config
	value := ""
	switch action {
	case "profile":
		p, b, e := chooseConnectionProfile(s, kind, "")
		if e != nil || b {
			return e
		}
		if p.ID == "" {
			return fmt.Errorf("bulk profile assignment requires a profile; detach individually")
		}
		if p.OriginalProfile == nil {
			profiles, e := sourceProfiles(s)
			if e != nil {
				return e
			}
			found := false
			for _, candidate := range profiles {
				if candidate.ID == p.ID {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("create the shared profile before bulk assignment")
			}
		}
		for i, r := range after {
			r.ProfileID = p.ID
			after[i] = applyProfile(r, p)
		}
		value = p.ProfileName
	case "jump":
		jump, b, e := chooseJump(c, s, "", "")
		if e != nil || b {
			return e
		}
		for i := range after {
			after[i].JumpID = jump
		}
		value = sshReferenceLabel(c, s, jump)
	case "identity":
		localOriginal, localBefore, localDraft, err = settingsDraft(state.ConfigPath)
		if err != nil {
			return err
		}
		// One selector for the batch; no per-host menu repetitions.
		if kind == "ssh" {
			name, b, e := chooseIdentity(c, "inherit")
			if e != nil || b {
				return e
			}
			value = name
			a := localDraft.IdentityAssignments[source]
			a.Connections = cloneStrings(a.Connections)
			if a.Connections == nil {
				a.Connections = map[string]string{}
			}
			for _, r := range before {
				if name == "inherit" {
					delete(a.Connections, r.ID)
				} else {
					a.Connections[r.ID] = name
				}
			}
			if localDraft.IdentityAssignments == nil {
				localDraft.IdentityAssignments = map[string]SourceIdentities{}
			}
			localDraft.IdentityAssignments[source] = a
		} else {
			first := before[0]
			if localDraft.Overrides == nil {
				localDraft.Overrides = map[string]map[string]ConnectionOverride{}
			}
			if localDraft.Overrides[source] == nil {
				localDraft.Overrides[source] = map[string]ConnectionOverride{}
			}
			if err = assignConnectionIdentity(&localDraft, s, first); err != nil {
				return err
			}
			o := localDraft.Overrides[source][first.ID]
			value = "Selected local RDP identity"
			for _, r := range before {
				entry := localDraft.Overrides[source][r.ID]
				entry.Account = o.Account
				localDraft.Overrides[source][r.ID] = entry
			}
		}
	}
	var summary strings.Builder
	fmt.Fprintf(&summary, "%d %s connections in %s\nChange: %s → %s\n\n", len(before), strings.ToUpper(kind), s.Name, action, value)
	hosts, err := sourceInventoryHosts(s)
	if err != nil {
		return err
	}
	names := map[string]string{}
	for _, h := range hosts {
		names[h.ID] = h.Alias
	}
	for _, r := range before {
		fmt.Fprintf(&summary, "• %s / %s\n", names[r.HostID], r.ProfileName)
	}
	_, back, err = choose("Review bulk changes", []choice{{"apply", "Apply batch"}}, summary.String())
	if err != nil || back {
		return err
	}
	if action == "identity" {
		return saveSettingsDraft(state.ConfigPath, localOriginal, localBefore, localDraft)
	}
	return applyBulkConnections(s, before, after)
}

func cloneStrings(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := map[string]string{}
	for k, v := range values {
		result[k] = v
	}
	return result
}
