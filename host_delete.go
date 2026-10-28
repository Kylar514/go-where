package main

import "fmt"

func reviewDeleteHost(path string, source InventorySource, host InventoryHost) (bool, error) {
	if source.ReadOnly {
		return false, fmt.Errorf("source is read-only")
	}
	id, back, err := choose("Delete host?", []choice{{"cancel", "Cancel"}, {"confirm", "Delete host permanently"}}, fmt.Sprintf("Host: %s\nSource: %s\n\nDeletes the saved host and its SSH/RDP profile assignments. Removes this device's selections and overrides for those entries. Reusable profiles and identities are preserved. Referencing routes or tunnels must be removed or reassigned first. Unsaved editor changes are not saved.", host.Alias, source.Name))
	if err != nil || back || id != "confirm" {
		return false, err
	}
	var original []byte
	var before, draft Config
	if path != "" {
		original, before, draft, err = settingsDraft(path)
		if err != nil {
			return false, err
		}
		for sid, values := range draft.PersonalConnections {
			for key, value := range values {
				own := sid == source.ID && (key == host.ID+":ssh" || key == host.ID+":rdp" || key == host.ID+":route")
				if !own && value.Jump != nil && value.Jump.Source == source.ID && value.Jump.ID == host.ID {
					return false, fmt.Errorf("host is referenced by a personal jump route; reassign it first")
				}
			}
		}
		for _, kind := range []string{"ssh", "rdp", "route"} {
			delete(draft.PersonalConnections[source.ID], host.ID+":"+kind)
		}
		delete(draft.HostOverrides[source.ID], host.ID)
		rows, e := sourceConnections(source)
		if e != nil {
			return false, e
		}
		for _, r := range rows {
			if r.HostID == host.ID {
				delete(draft.Overrides[source.ID], r.ID)
				a := draft.IdentityAssignments[source.ID]
				delete(a.Connections, r.ID)
				if draft.IdentityAssignments != nil {
					draft.IdentityAssignments[source.ID] = a
				}
			}
		}
		if err = draft.validate(); err != nil {
			return false, err
		}
	}
	if err = deleteInventoryHost(source, host.ID); err != nil {
		return false, err
	}
	if path != "" {
		if err = saveSettingsDraft(path, original, before, draft); err != nil {
			return true, fmt.Errorf("host deleted, but local selection cleanup failed: %w. Reopen Settings to remove stale selections", err)
		}
	}
	return true, nil
}
