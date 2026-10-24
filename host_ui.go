package main

import (
	"fmt"
	"reflect"
	"strings"
)

var saveActiveHost func() error
var hostSaveState func() string
var saveHostLocalSettings func() error
var hostLocalSettingsState func() string

func editInventoryHost(c Config, state *PickerState, selected *Host) error {
	previousKeys := menuHotkeys
	menuHotkeys = c
	defer func() { menuHotkeys = previousKeys }()
	defer menuSession()()
	preferred := state.Source
	if selected != nil {
		preferred = selected.SourceID
	}
	if selected == nil {
		preferred = ""
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
	localOriginal, localBefore, localDraft, localErr := settingsDraft(state.ConfigPath)
	if state.ConfigPath == "" {
		localBefore = c
		localDraft = cloneAgentSettings(c)
		localErr = nil
	}
	previousLocalSave, previousLocalState := saveHostLocalSettings, hostLocalSettingsState
	defer func() { saveHostLocalSettings, hostLocalSettingsState = previousLocalSave, previousLocalState }()
	saveHostLocalSettings = func() error {
		if localErr != nil {
			return localErr
		}
		if state.ConfigPath == "" {
			return fmt.Errorf("settings path required")
		}
		if err := saveSettingsDraft(state.ConfigPath, localOriginal, localBefore, localDraft); err != nil {
			return err
		}
		data, err := readSettingsData(state.ConfigPath)
		if err != nil {
			return err
		}
		localOriginal = data
		localBefore = cloneAgentSettings(localDraft)
		c = localDraft
		return nil
	}
	hostLocalSettingsState = func() string {
		if !reflect.DeepEqual(localBefore, localDraft) {
			return "Local changes unsaved"
		}
		return "Local settings saved"
	}
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
		// Direct fields replace the old profile -> identity -> profile menu loop.
		options = options[:4]
		localAddress := "Inherit host address"
		if o := localDraft.HostOverrides[s.ID][h.ID]; o.Address != nil {
			localAddress = *o.Address
		}
		options = append(options, choice{"local-address", "Local address: " + localAddress})
		options = append(options, directConnectionRows(localDraft, s, profiles)...)
		options = append(options, choice{"tunnels", "Tunnels…"}, choice{"save", "Save host and profiles"})
		options = append(options, choice{"color", "Color: " + h.Color + " …"})
		if selected != nil {
			options = append(options, choice{"delete", "Delete host and profiles…"})
		}
		menu := newDialog("Host — "+s.Name, options, "Connection selects assigned or available shared profiles. Identity selects your device-local login. Jump selects this host's route.\n\nSave host writes shared fields and routes. Save local settings writes identities and local exceptions separately. Back discards only unsaved changes.")
		menu.colorBrackets = map[string]string{"color": h.BracketColor}
		id, b, e := runChoice(menu, menuKey(menu.title))
		if e != nil || b {
			return e
		}
		switch id {
		case "local-address":
			mode, back, err := chooseOne("Local address", []choice{{"inherit", "Inherit host address"}, {"set", "Set device-local address"}}, "Local settings only; shared host address is unchanged.", "inherit")
			if err != nil {
				return err
			}
			if back {
				continue
			}
			var address *string
			if mode == "set" {
				value, b, e := inventoryPrompt("Local address", h.Address)
				if e != nil {
					return e
				}
				if b {
					continue
				}
				address = &value
			}
			if localDraft.HostOverrides == nil {
				localDraft.HostOverrides = map[string]map[string]HostOverride{}
			}
			if localDraft.HostOverrides[s.ID] == nil {
				localDraft.HostOverrides[s.ID] = map[string]HostOverride{}
			}
			localDraft.HostOverrides[s.ID][h.ID] = HostOverride{Address: address}
		case "ssh-profile", "rdp-profile":
			kind := strings.Split(id, "-")[0]
			if e = editHostProfiles(localDraft, s, h, &profiles, kind, state.ConfigPath, &localDraft); e != nil {
				return e
			}
			continue
		case "ssh-identity", "rdp-identity":
			kind := strings.Split(id, "-")[0]
			i := defaultConnectionIndex(profiles, kind)
			if i < 0 {
				if e = notice("Select a connection profile first."); e != nil {
					return e
				}
				continue
			}
			if e = assignConnectionIdentity(&localDraft, s, profiles[i]); e != nil {
				return e
			}
		case "ssh-jump", "rdp-jump":
			kind := strings.Split(id, "-")[0]
			i := defaultConnectionIndex(profiles, kind)
			if i < 0 {
				if e = notice("Select a connection profile first."); e != nil {
					return e
				}
				continue
			}
			jump, back, err := chooseJump(c, s, profiles[i].JumpID, profiles[i].ID)
			if err != nil {
				return err
			}
			if !back {
				profiles[i].JumpID = jump
			}
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
			value, bracket, back, err := editHostColors(h.Color, h.BracketColor, func(block, bracket string) error {
				if selected == nil {
					return fmt.Errorf("Save the new host's name and address before saving colors.")
				}
				if err := saveHostFields(s, []InventoryHost{beforeHost}, map[string]string{"color": block, "bracket_color": bracket}); err != nil {
					return err
				}
				beforeHost.Color, beforeHost.BracketColor = block, bracket
				saved = true
				return nil
			})
			if err != nil {
				return err
			}
			if !back {
				h.Color = value
				h.BracketColor = bracket
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

func editHostProfiles(c Config, s InventorySource, h InventoryHost, profiles *[]Connection, kind, configPath string, localDrafts ...*Config) error {
	for {
		options := []choice{{"add", "Add " + strings.ToUpper(kind) + " profile…"}}
		options = append(options, choice{"none", "None — disable " + strings.ToUpper(kind) + " for this host"})
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
			options = append(options, choice{"unassign:" + p.ID, "Unassign: " + p.ProfileName})
		}
		var actions, list []choice
		for _, option := range options {
			if option.ID == "add" || option.ID == "none" || strings.HasPrefix(option.ID, "unassign:") {
				actions = append(actions, option)
			} else {
				list = append(list, option)
			}
		}
		options = append(actions, list...)
		id, back, err := choose(strings.ToUpper(kind)+" profiles", options, "Select an available profile to assign it. Select an assigned profile to configure it.\n\nNone clears all assignments for this protocol; Unassign removes only the named connection. Reusable policies remain.\n\nSave host applies the draft and rejects removal of connections referenced by jumps or tunnels.")
		if err != nil || back {
			return err
		}
		if id == "none" || strings.HasPrefix(id, "unassign:") {
			var kept []Connection
			for _, p := range *profiles {
				if id == "none" && p.kind() == kind {
					continue
				}
				if id == "unassign:"+p.ID {
					continue
				}
				kept = append(kept, p)
			}
			defaultFound := false
			for _, p := range kept {
				if p.kind() == kind && p.Default {
					defaultFound = true
				}
			}
			if !defaultFound {
				for i := range kept {
					if kept[i].kind() == kind {
						kept[i].Default = true
						break
					}
				}
			}
			*profiles = kept
			continue
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
			options := []choice{{"default", "Default connection: " + fmt.Sprint(p.Default)}, {"exceptions", "Local field exceptions…"}}
			if p.ProfileID == "" {
				options = append(options, choice{"settings", "New policy settings…"})
			}
			action, b, e := choose("Connection — "+p.ProfileName, options, fmt.Sprintf("Policy settings are shared by %d saved attachments. Manage them in Settings → Connection policies.\n\nJump and default apply only to this host. Local identity belongs to this device. Remove detaches the connection, not the reusable policy.", uses))
			if e != nil {
				return e
			}
			if b {
				break
			}
			switch action {
			case "exceptions":
				if len(localDrafts) == 0 {
					if e = notice("Open the host editor to change local exceptions."); e != nil {
						return e
					}
					continue
				}
				if e = editLocalProfileFields(localDrafts[0], s, p, h.Alias); e != nil {
					return e
				}
			case "policy":
				shared, e := sourceProfiles(s)
				if e != nil {
					return e
				}
				options := []choice{}
				for _, policy := range shared {
					if policy.kind() == kind {
						options = append(options, choice{policy.ID, policy.ProfileName})
					}
				}
				pid, b, e := chooseOne("Connection policy", options, "Select shared policy settings for this host connection.", p.ProfileID)
				if e != nil {
					return e
				}
				if !b {
					for _, policy := range shared {
						if policy.ID == pid {
							a := p
							a.ProfileID = pid
							(*profiles)[index] = applyProfile(a, policy)
						}
					}
				}
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
				if kind == "ssh" {
					if p.ProfileID == "" {
						if e = notice("Save the host first."); e != nil {
							return e
						}
						continue
					}
					original, before, draft, e := settingsDraft(configPath)
					if e != nil {
						return e
					}
					if e = assignConnectionIdentity(&draft, s, p); e != nil {
						return e
					}
					_, back, e := choose("Save local identity?", []choice{{"save", "Save local assignment separately"}}, "Shared host/profile data is unchanged.")
					if e != nil {
						return e
					}
					if !back {
						if e = saveSettingsDraft(configPath, original, before, draft); e != nil {
							return e
						}
						c = draft
					}
					continue
				}
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
