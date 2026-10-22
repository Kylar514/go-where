package main

import (
	"fmt"
	"reflect"
	"strings"
)

var saveActiveHost func() error
var hostSaveState func() string

func editInventoryHost(c Config, state *PickerState, selected *Host) error {
	previousKeys := menuHotkeys
	menuHotkeys = c
	defer func() { menuHotkeys = previousKeys }()
	defer menuSession()()
	preferred := state.Source
	if selected != nil {
		preferred = selected.SourceID
	}
	s, back, err := writableSource(c, preferred, state.ConfigPath)
	if err != nil || back {
		return err
	}
	if _, e := findSource(c, s.ID); e != nil {
		c.Sources = append(c.Sources, s)
	}
	h := InventoryHost{ID: inventoryID()}
	var profiles []Connection
	if selected != nil {
		hostID := selected.InventoryHostID
		if hostID == "" {
			_, r, e := storedRecord(*selected, c)
			if e != nil {
				return e
			}
			hostID = r.HostID
		}
		hosts, e := sourceInventoryHosts(s)
		if e != nil {
			return e
		}
		found := false
		for _, candidate := range hosts {
			if candidate.ID == hostID {
				h = candidate
				found = true
			}
		}
		if !found {
			return fmt.Errorf("host no longer exists")
		}
		records, e := sourceConnections(s)
		if e != nil {
			return e
		}
		for _, record := range records {
			if record.HostID == h.ID {
				profiles = append(profiles, record)
			}
		}
	}
	previousSave, previousState := saveActiveHost, hostSaveState
	defer func() { saveActiveHost, hostSaveState = previousSave, previousState }()
	beforeHost := h
	beforeProfiles := append([]Connection(nil), profiles...)
	saved := false
	saveActiveHost = func() error {
		if err := saveInventoryHost(s, h, profiles); err != nil {
			return err
		}
		rows, err := sourceConnections(s)
		if err != nil {
			return err
		}
		for i := range profiles {
			for _, row := range rows {
				if row.ID == profiles[i].ID {
					profiles[i] = row
				}
			}
		}
		beforeHost = h
		beforeProfiles = append([]Connection(nil), profiles...)
		saved = true
		if selected == nil {
			selected = &Host{SourceID: s.ID, InventoryHostID: h.ID}
		}
		state.Source = s.ID
		state.Focus = s.ID + ":" + h.ID
		for _, p := range profiles {
			if p.Default && p.kind() == state.Mode {
				state.Focus = s.ID + ":" + p.ID
			}
		}
		return nil
	}
	hostSaveState = func() string {
		if !reflect.DeepEqual(h, beforeHost) || !reflect.DeepEqual(profiles, beforeProfiles) {
			return "Unsaved changes"
		}
		if saved {
			return "Saved"
		}
		return "No unsaved changes"
	}
	for {
		options := []choice{{"alias", "Name: " + h.Alias}, {"address", "Address: " + h.Address}, {"tags", "Tags: " + strings.Join(h.Tags, ", ")}, {"notes", "Notes: " + h.Notes}, {"ssh", "SSH profiles: " + hostProfileSummary(profiles, "ssh") + " …"}, {"rdp", "RDP profiles: " + hostProfileSummary(profiles, "rdp") + " …"}, {"tunnels", "Tunnels…"}, {"save", "Save host and profiles"}}
		options = append(options, choice{"color", "Color: " + h.Color + " …"})
		if selected != nil {
			options = append(options, choice{"local-overrides", "Local overrides… (this device only)"})
		}
		if selected != nil {
			options = append(options, choice{"delete", "Delete host and profiles…"})
		}
		id, b, e := choose("Host — "+s.Name, options, "Address, tags and notes are shared. SSH/RDP profiles keep independent ports and accounts. Jumps and tunnels reference a specific SSH profile. Changes are saved together; Esc discards this draft.")
		if e != nil || b {
			return e
		}
		switch id {
		case "local-overrides":
			if e = editHostLocalOverrides(state.ConfigPath, s, h); e != nil {
				return e
			}
			c, e = loadConfig(state.ConfigPath)
			if e != nil {
				return e
			}
			continue
		case "color":
			value, back, err := chooseHostColor(h.Color)
			if err != nil {
				return err
			}
			if !back {
				h.Color = value
			}
		case "ssh", "rdp":
			if e = editHostProfiles(c, s, h, &profiles, id, state.ConfigPath); e != nil {
				return e
			}
		case "tunnels":
			if selected == nil {
				if e = notice("Save this host and an SSH profile first."); e != nil {
					return e
				}
				continue
			}
			if e = hostTunnels(c, s, h, state); e != nil {
				return e
			}
		case "save":
			if e = saveInventoryHost(s, h, profiles); e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
				continue
			}
			state.Source = s.ID
			state.Focus = s.ID + ":" + h.ID
			for _, p := range profiles {
				if p.Default && p.kind() == state.Mode {
					state.Focus = s.ID + ":" + p.ID
				}
			}
			return nil
		case "delete":
			_, b, e = choose("Delete "+h.Alias+"?", []choice{{"delete", "Delete host and all attachments"}}, "Host: "+h.Alias+"\n\nShared source profiles remain available. Referenced attachments cannot be deleted. Tunnels must be removed or reassigned first.")
			if e != nil {
				return e
			}
			if !b {
				return deleteInventoryHost(s, h.ID)
			}
		default:
			value := ""
			switch id {
			case "alias":
				value = h.Alias
			case "address":
				value = h.Address
			case "tags":
				value = strings.Join(h.Tags, ", ")
			case "notes":
				value = h.Notes
			}
			value, b, e = inventoryPrompt(id, value)
			if e != nil {
				return e
			}
			if b {
				continue
			}
			switch id {
			case "alias":
				h.Alias = value
			case "address":
				h.Address = value
			case "notes":
				h.Notes = value
			case "tags":
				h.Tags = nil
				for _, tag := range strings.Split(value, ",") {
					if tag = strings.TrimSpace(tag); tag != "" {
						h.Tags = append(h.Tags, tag)
					}
				}
			}
		}
	}
}

func hostProfileSummary(profiles []Connection, kind string) string {
	var names []string
	for _, profile := range profiles {
		if profile.kind() != kind {
			continue
		}
		name := profile.ProfileName
		if profile.Default {
			name += " ★"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return "Not configured"
	}
	return strings.Join(names, ", ")
}

func editHostProfiles(c Config, s InventorySource, h InventoryHost, profiles *[]Connection, kind, configPath string) error {
	for {
		options := []choice{{"add", "Add " + strings.ToUpper(kind) + " profile…"}}
		shared, err := sourceProfiles(s)
		if err != nil {
			return err
		}
		for _, p := range shared {
			if p.kind() != kind {
				continue
			}
			attached := false
			for _, a := range *profiles {
				if a.ProfileID == p.ID {
					attached = true
					break
				}
			}
			if !attached {
				options = append(options, choice{"available:" + p.ID, "○ " + p.ProfileName + " — available [" + p.ID + "]"})
			}
		}
		for _, p := range *profiles {
			if p.kind() != kind {
				continue
			}
			label := "✓ " + p.ProfileName + " — assigned"
			if p.Default {
				label += " ★ default"
			}
			options = append(options, choice{p.ID, label})
		}
		id, back, err := choose(strings.ToUpper(kind)+" profiles", options, "All profiles in this source are available here.\n\nSelect an available profile to assign it to this host's draft. Select an assigned profile to edit it or choose the default. Save host persists assignments.\n\nShared edits affect every host using that profile.")
		if err != nil || back {
			return err
		}
		if strings.HasPrefix(id, "available:") {
			pid := strings.TrimPrefix(id, "available:")
			for _, p := range shared {
				if p.ID == pid {
					a := p
					original := p
					a.OriginalProfile = &original
					a.ProfileID = p.ID
					a.ID = inventoryID()
					a.Alias = "profile-" + a.ID
					a.HostID = h.ID
					a.Default = true
					for _, old := range *profiles {
						if old.kind() == kind {
							a.Default = false
						}
					}
					*profiles = append(*profiles, a)
				}
			}
			continue
		}
		if id == "add" {
			name, b, e := inventoryPrompt("Profile name", "Default")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if !inventoryName(name) {
				if e = notice("Use letters, digits, dots, underscores or hyphens for profile names."); e != nil {
					return e
				}
				continue
			}
			p := Connection{ID: inventoryID(), HostID: h.ID, ProfileName: name, Type: kind, Default: true, Port: 22}
			p.Alias = "profile-" + p.ID
			if kind == "rdp" {
				p.Port = 3389
			}
			for _, existing := range *profiles {
				if existing.kind() == kind {
					p.Default = false
				}
			}
			*profiles = append(*profiles, p)
			id = p.ID
		}
		index := -1
		for i, p := range *profiles {
			if p.ID == id {
				index = i
			}
		}
		if index < 0 {
			continue
		}
		for {
			p := (*profiles)[index]
			uses := 0
			if rows, e := sourceConnections(s); e == nil {
				for _, a := range rows {
					if a.ProfileID == p.ProfileID && p.ProfileID != "" {
						uses++
					}
				}
			}
			action, b, e := choose("Profile — "+p.ProfileName, []choice{{"settings", "Shared connection settings…"}, {"name", "Name: " + p.ProfileName}, {"default", "Make default for this host"}, {"jump", "Host-specific SSH jump…"}, {"duplicate", "Duplicate as independent profile"}, {"remove", "Detach profile from host draft"}}, fmt.Sprintf("Shared profile used by %d saved host attachments. Editing settings updates all users of this profile on Save host. Address, default and jumps remain host-specific. Local overrides belong in the host editor.", uses))
			if e != nil {
				return e
			}
			if b {
				break
			}
			switch action {
			case "duplicate":
				(*profiles)[index].ProfileID = ""
				(*profiles)[index].OriginalProfile = nil
				(*profiles)[index].ProfileName += "-copy"
			case "jump":
				rows, e := sourceConnections(s)
				if e != nil {
					return e
				}
				choices := []choice{{"", "None (direct)"}}
				for _, a := range rows {
					if a.kind() == "ssh" && a.ID != p.ID {
						choices = append(choices, choice{a.ID, a.Alias + " / " + a.ProfileName})
					}
				}
				id, b, e := chooseOne("Host jump", choices, "Pinned to a host/profile attachment.", p.JumpID)
				if e != nil {
					return e
				}
				if !b {
					(*profiles)[index].JumpID = id
				}
			case "settings":
				p.Address, p.Tags, p.Notes = h.Address, h.Tags, h.Notes
				ps := PickerState{Source: s.ID, ConfigPath: configPath}
				if e = editConnectionDraft(c, &ps, nil, &(*profiles)[index]); e != nil {
					return e
				}
			case "name":
				v, b, e := inventoryPrompt("Profile name", p.ProfileName)
				if e != nil {
					return e
				}
				if !b {
					(*profiles)[index].ProfileName = v
				}
			case "default":
				for i := range *profiles {
					if (*profiles)[i].kind() == kind {
						(*profiles)[i].Default = (*profiles)[i].ID == id
					}
				}
			case "local":
				if kind != "ssh" || configPath == "" {
					if e = notice("Local SSH overrides require a saved SSH profile and settings path."); e != nil {
						return e
					}
					continue
				}
				ref := Host{SourceID: s.ID, ConnectionID: p.ID, Alias: h.Alias}
				if _, _, e = storedRecord(ref, c); e != nil {
					if e = notice("Save the host first."); e != nil {
						return e
					}
					continue
				}
				a, b, e := choose("Local profile overrides", []choice{{"agent", "SSH agent"}, {"compatibility", "SSH compatibility"}}, "Saved separately from the host draft.")
				if e != nil {
					return e
				}
				if b {
					continue
				}
				editor := editAgentOverrideDraft
				if a == "compatibility" {
					editor = editCompatibilityOverrideDraft
				}
				if e = editConnectionOverride(configPath, ref, editor); e != nil {
					return e
				}
				c, e = loadConfig(configPath)
				if e != nil {
					return e
				}
			case "remove":
				*profiles = append((*profiles)[:index], (*profiles)[index+1:]...)
				if p.Default {
					for i := range *profiles {
						if (*profiles)[i].kind() == kind {
							(*profiles)[i].Default = true
							break
						}
					}
				}
			}
			if action == "remove" {
				break
			}
		}
	}
}

func hostTunnels(c Config, s InventorySource, h InventoryHost, state *PickerState) error {
	for {
		records, err := sourceConnections(s)
		if err != nil {
			return err
		}
		sshIDs := map[string]bool{}
		for _, r := range records {
			if r.HostID == h.ID && r.kind() == "ssh" {
				sshIDs[r.ID] = true
			}
		}
		options := []choice{{"add", "Add SSH tunnel…"}}
		for _, r := range records {
			if r.kind() == "tunnel" && sshIDs[r.SSHID] {
				options = append(options, choice{r.ID, r.Alias})
			}
		}
		id, back, err := choose("Tunnels — "+h.Alias, options, "Tunnel edits are saved separately. A tunnel uses this host's SSH profile; its remote target can be a different machine.")
		if err != nil || back {
			return err
		}
		if id == "add" {
			if len(sshIDs) == 0 {
				return notice("Save an SSH profile on this host first.")
			}
			choices := []choice{}
			for _, r := range records {
				if sshIDs[r.ID] {
					choices = append(choices, choice{r.ID, r.ProfileName})
				}
			}
			profile, b, e := choose("SSH profile", choices, "")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			r := Connection{ID: inventoryID(), Type: "tunnel", TunnelType: "ssh", SSHID: profile, Bind: "127.0.0.1", LocalPort: 8080, RemoteHost: "localhost", RemotePort: 80}
			// Use existing tunnel editor, then explicitly persist this standalone definition.
			ps := PickerState{Source: s.ID, ConfigPath: state.ConfigPath}
			if e = editConnectionDraft(c, &ps, nil, &r); e != nil {
				return e
			}
			name, b, e := inventoryPrompt("Tunnel name", h.Alias+"-forward")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			r.Alias = name
			_, b, e = choose("Save tunnel?", []choice{{"save", "Save tunnel separately"}}, "Cancelling the host editor will not undo this save.")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if e = changeConnection(s, r, false); e != nil {
				return e
			}
		} else {
			ref := Host{SourceID: s.ID, ConnectionID: id, Type: "tunnel"}
			ps := PickerState{Source: s.ID, ConfigPath: state.ConfigPath}
			if err = editConnection(c, &ps, &ref); err != nil {
				return err
			}
		}
	}
}

func chooseHostProfiles(c Config, selected []Host) ([]Host, bool, error) {
	result := append([]Host(nil), selected...)
	for i, h := range result {
		if h.SourceID == "" {
			continue
		}
		s, r, err := storedRecord(h, c)
		if err != nil {
			return nil, false, err
		}
		records, err := sourceConnections(s)
		if err != nil {
			return nil, false, err
		}
		options := []choice{}
		for _, p := range records {
			if p.HostID == r.HostID && p.kind() == r.kind() {
				label := p.ProfileName
				if p.Default {
					label += " ★ default"
				}
				options = append(options, choice{p.ID, label})
			}
		}
		id, back, err := chooseOne("Connect as — "+h.Alias, options, "One-launch profile choice; saved defaults are unchanged.", h.ConnectionID)
		if err != nil || back {
			return nil, back, err
		}
		result[i].ConnectionID = id
	}
	return result, false, nil
}
