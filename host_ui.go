package main

import (
	"fmt"
	"strings"
)

func editInventoryHost(c Config, state *PickerState, selected *Host) error {
	defer menuSession()()
	preferred := state.Source
	if selected != nil {
		preferred = selected.SourceID
	}
	s, back, err := writableSource(c, preferred)
	if err != nil || back {
		return err
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
	for {
		options := []choice{{"alias", "Name: " + h.Alias}, {"address", "Address: " + h.Address}, {"tags", "Tags: " + strings.Join(h.Tags, ", ")}, {"notes", "Notes: " + h.Notes}, {"ssh", "SSH profiles…"}, {"rdp", "RDP profiles…"}, {"tunnels", "Tunnels…"}, {"save", "Save host and profiles"}}
		if selected != nil {
			options = append(options, choice{"local-address", "Local host address override…"})
		}
		if selected != nil {
			options = append(options, choice{"delete", "Delete host and profiles…"})
		}
		id, b, e := choose("Host — "+s.Name, options, "Address, tags and notes are shared. SSH/RDP profiles keep independent ports and accounts. Jumps and tunnels reference a specific SSH profile. Changes are saved together; Esc discards this draft.")
		if e != nil || b {
			return e
		}
		switch id {
		case "local-address":
			if state.ConfigPath == "" {
				return fmt.Errorf("settings path required")
			}
			original, before, draft, e := settingsDraft(state.ConfigPath)
			if e != nil {
				return e
			}
			mode, b, e := choose("Local host address", []choice{{"inherit", "Inherit shared host address"}, {"custom", "Set local address…"}}, "Applies to every SSH/RDP profile and saved-host tunnel target. Existing profile-specific address overrides still take precedence. Saved separately.")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			var address *string
			if mode == "custom" {
				v, b, e := inventoryPrompt("Local host address", h.Address)
				if e != nil {
					return e
				}
				if b {
					continue
				}
				address = &v
			}
			if draft.HostOverrides == nil {
				draft.HostOverrides = map[string]map[string]HostOverride{}
			}
			if draft.HostOverrides[s.ID] == nil {
				draft.HostOverrides[s.ID] = map[string]HostOverride{}
			}
			draft.HostOverrides[s.ID][h.ID] = HostOverride{Address: address}
			_, b, e = choose("Save local host override?", []choice{{"save", "Save local settings separately"}}, "Cancelling the host editor does not undo this save.")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if e = saveSettingsDraft(state.ConfigPath, original, before, draft); e != nil {
				return e
			}
			c, e = loadConfig(state.ConfigPath)
			if e != nil {
				return e
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
			_, b, e = choose("Delete host?", []choice{{"delete", "Delete host and all profiles"}}, "Referenced profiles cannot be deleted. Tunnels must be removed or reassigned first.")
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

func editHostProfiles(c Config, s InventorySource, h InventoryHost, profiles *[]Connection, kind, configPath string) error {
	for {
		options := []choice{{"add", "Add " + strings.ToUpper(kind) + " profile…"}}
		for _, p := range *profiles {
			if p.kind() != kind {
				continue
			}
			label := p.ProfileName
			if p.Default {
				label += " ★ default"
			}
			options = append(options, choice{p.ID, label})
		}
		id, back, err := choose(strings.ToUpper(kind)+" profiles", options, "Enter uses the default profile. Connect as chooses another. Referenced profiles cannot be removed when the host is saved.")
		if err != nil || back {
			return err
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
			action, b, e := choose("Profile — "+p.ProfileName, []choice{{"settings", "Connection settings…"}, {"name", "Name: " + p.ProfileName}, {"default", "Make default"}, {"local", "Local settings overrides…"}, {"remove", "Remove profile from draft"}}, "Profile settings belong to this host; address, tags and notes are shared. Local overrides save separately and require a previously saved profile.")
			if e != nil {
				return e
			}
			if b {
				break
			}
			switch action {
			case "settings":
				p.Address, p.Tags, p.Notes = h.Address, h.Tags, h.Notes
				ps := PickerState{Source: s.ID, ConfigPath: configPath}
				if e = editConnectionDraft(c, &ps, nil, &p); e != nil {
					return e
				}
				(*profiles)[index] = p
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
