package main

import (
	"fmt"
	"sort"
	"strings"
)

type ProfileReference struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

type PersonalConnection struct {
	Profile         *ProfileReference `json:"profile,omitempty"`
	Identities      []string          `json:"identities,omitempty"`
	DefaultIdentity string            `json:"default_identity,omitempty"`
}

func personalKey(r Connection) string { return r.HostID + ":" + r.kind() }

func validatePersonalConnections(c Config) error {
	for id, p := range c.LocalProfiles {
		if id != p.ID || !inventoryName(id) || !inventoryName(p.ProfileName) || (p.kind() != "ssh" && p.kind() != "rdp") {
			return fmt.Errorf("invalid local connection profile")
		}
		check := p
		check.Alias = p.ID
		check.Address = "192.0.2.1"
		check.JumpID = ""
		if err := validateConnections([]Connection{check}); err != nil {
			return err
		}
	}
	for _, byHost := range c.PersonalConnections {
		for _, selection := range byHost {
			if selection.Profile != nil && (!inventoryName(selection.Profile.Source) || !inventoryName(selection.Profile.ID)) {
				return fmt.Errorf("invalid personal profile reference")
			}
			seen := map[string]bool{}
			for _, id := range selection.Identities {
				if id == "" || seen[id] {
					return fmt.Errorf("invalid identity selection")
				}
				seen[id] = true
			}
			if selection.DefaultIdentity != "" && !seen[selection.DefaultIdentity] {
				return fmt.Errorf("default identity must be selected")
			}
		}
	}
	return nil
}

func personalProfile(c Config, ref ProfileReference) (Connection, error) {
	if ref.Source == "local" {
		p, ok := c.LocalProfiles[ref.ID]
		if !ok {
			return Connection{}, fmt.Errorf("local profile unavailable")
		}
		return p, nil
	}
	if !sourceEnabled(c, ref.Source) {
		return Connection{}, fmt.Errorf("selected profile source is disabled")
	}
	s, err := findSource(c, ref.Source)
	if err != nil {
		return Connection{}, err
	}
	profiles, err := sourceProfiles(s)
	if err != nil {
		return Connection{}, err
	}
	for _, p := range profiles {
		if p.ID == ref.ID {
			return p, nil
		}
	}
	return Connection{}, fmt.Errorf("selected connection profile unavailable")
}

func applyPersonalConnection(c Config, source string, r Connection) (Connection, error) {
	selection, ok := c.PersonalConnections[source][personalKey(r)]
	if !ok {
		return r, nil
	}
	if selection.Profile != nil {
		p, err := personalProfile(c, *selection.Profile)
		if err != nil {
			return r, err
		}
		if p.kind() != r.kind() {
			return r, fmt.Errorf("selected profile has wrong protocol")
		}
		// Legacy attachment routing remains intact until profile routing migration.
		r = applyProfile(r, p)
	}
	if selection.DefaultIdentity == "" {
		return r, nil
	}
	if r.kind() == "rdp" {
		if selection.DefaultIdentity == "default" {
			r.Account = ""
			r.User = ""
		} else {
			r.Account = selection.DefaultIdentity
		}
		return r, nil
	}
	if selection.DefaultIdentity == "default" {
		r.User = ""
		r.IdentityFile = ""
		return r, nil
	}
	p, ok := c.Identities[selection.DefaultIdentity]
	if !ok {
		return r, fmt.Errorf("selected SSH identity unavailable")
	}
	r.User, r.IdentityFile = p.User, p.IdentityFile
	return r, nil
}

func profileChoices(c Config, kind string) ([]choice, map[string]ProfileReference, error) {
	options := []choice{{"existing", "Use host default"}}
	refs := map[string]ProfileReference{}
	var ids []string
	for id := range c.LocalProfiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := c.LocalProfiles[id]
		if p.kind() != kind {
			continue
		}
		key := "local:" + id
		refs[key] = ProfileReference{"local", id}
		options = append(options, choice{key, p.ProfileName + " — Local"})
	}
	for _, s := range c.Sources {
		if !sourceEnabled(c, s.ID) {
			continue
		}
		profiles, err := sourceProfiles(s)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range profiles {
			if p.kind() != kind {
				continue
			}
			key := s.ID + ":" + p.ID
			refs[key] = ProfileReference{s.ID, p.ID}
			options = append(options, choice{key, p.ProfileName + " — " + s.Name})
		}
	}
	return options, refs, nil
}

func selectPersonalIdentities(c Config, kind string, current PersonalConnection) (PersonalConnection, bool, error) {
	checked := map[string]bool{}
	for _, id := range current.Identities {
		checked[id] = true
	}
	items := []multiItem{{ID: "default", Label: "System / client default", Checked: checked["default"]}}
	if kind == "ssh" {
		var names []string
		for name := range c.Identities {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, id := range names {
			items = append(items, multiItem{ID: id, Label: id, Checked: checked[id]})
		}
	} else {
		for _, a := range c.RDP.Accounts {
			items = append(items, multiItem{ID: a.Name, Label: a.Name, Checked: checked[a.Name]})
		}
	}
	m := newMultiDialog(strings.ToUpper(kind)+" Identities", items, "Tab / Shift+Tab toggle and move. Choose the identities offered at launch; then choose one default. These choices are personal and never edit the source.")
	m.allowManual = false
	if _, err := runTUI(m); err != nil {
		return current, false, err
	}
	if m.back {
		return current, true, nil
	}
	var selected []string
	var options []choice
	for _, item := range m.items {
		if item.Checked {
			selected = append(selected, item.ID)
			options = append(options, choice{item.ID, item.Label})
		}
	}
	if len(selected) == 0 {
		return current, false, fmt.Errorf("select at least one identity")
	}
	mDefault := newDialog("Default "+strings.ToUpper(kind)+" identity", options, "Default must be one of the selected identities.")
	mDefault.singleChoice = true
	mDefault.current = current.DefaultIdentity
	mDefault.footerActions = []choice{{"select", "Select"}}
	id, back, err := runChoice(mDefault, menuKey(mDefault.title))
	if err != nil || back {
		return current, back, err
	}
	current.Identities, current.DefaultIdentity = selected, id
	return current, false, nil
}

// Single and bulk edits share this form. Host fields and personal selections
// have separate saves; changing a profile never edits its shared definition.
func editPersonalHosts(c Config, path string, hosts []Host) error {
	if len(hosts) == 0 {
		return nil
	}
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		return err
	}
	type target struct{ source, key, kind string }
	var targets []target
	seen := map[string]bool{}
	sources := map[string]InventorySource{}
	storedHosts := map[string]InventoryHost{}
	defaultProfiles := map[string]string{}
	for _, h := range hosts {
		s, err := findSource(c, h.SourceID)
		if err != nil {
			return err
		}
		hostID := h.InventoryHostID
		if hostID == "" {
			_, r, e := storedRecord(h, c)
			if e != nil {
				return e
			}
			hostID = r.HostID
		}
		if _, ok := sources[s.ID]; !ok {
			sources[s.ID] = s
			inventory, e := sourceInventoryHosts(s)
			if e != nil {
				return e
			}
			for _, host := range inventory {
				storedHosts[s.ID+":"+host.ID] = host
			}
			connections, e := sourceConnections(s)
			if e != nil {
				return e
			}
			for _, r := range connections {
				if r.Default {
					defaultProfiles[s.ID+":"+personalKey(r)] = r.ProfileName + " (" + s.Name + ")"
				}
			}
		}
		if _, ok := storedHosts[s.ID+":"+hostID]; !ok {
			return fmt.Errorf("host no longer exists")
		}
		for _, kind := range []string{"ssh", "rdp"} {
			key := hostID + ":" + kind
			id := s.ID + ":" + key
			if !seen[id] {
				seen[id] = true
				targets = append(targets, target{s.ID, key, kind})
			}
		}
	}
	fieldEdits := map[string]string{}
	allWritable := true
	for _, s := range sources {
		if s.ReadOnly {
			allWritable = false
		}
	}
	previousSave, previousState := saveActiveSettings, settingsSaveState
	saveActiveSettings, settingsSaveState = nil, nil
	defer func() { saveActiveSettings, settingsSaveState = previousSave, previousState }()
	hostDefault := func(t target) string {
		if label := defaultProfiles[t.source+":"+t.key]; label != "" {
			return "Host default — " + label
		}
		return "No host default configured"
	}
	for {
		var options []choice
		for _, field := range []string{"name", "address", "tags", "notes", "color"} {
			value := ""
			first := true
			for _, t := range targets {
				if t.kind != "ssh" {
					continue
				}
				h := storedHosts[t.source+":"+strings.TrimSuffix(t.key, ":ssh")]
				next := inventoryHostField(h, field)
				if first {
					value = next
					first = false
				} else if value != next {
					value = "Mixed"
				}
			}
			if edit, ok := fieldEdits[field]; ok {
				value = edit
			}
			label := strings.ToUpper(field[:1]) + field[1:]
			if !allWritable {
				value += " (read-only)"
			}
			options = append(options, choice{field, label + ": " + value})
		}
		for _, kind := range []string{"ssh", "rdp"} {
			profile, identity := "Inventory profile", "System / client default"
			first := true
			for _, t := range targets {
				if t.kind != kind {
					continue
				}
				value := draft.PersonalConnections[t.source][t.key]
				p := hostDefault(t)
				if value.Profile != nil {
					r, e := personalProfile(draft, *value.Profile)
					if e != nil {
						p = "Unavailable"
					} else {
						name := "Local"
						if value.Profile.Source != "local" {
							if s, e := findSource(draft, value.Profile.Source); e == nil {
								name = s.Name
							} else {
								name = value.Profile.Source
							}
						}
						p = r.ProfileName + " — " + name
					}
				}
				i := strings.Join(value.Identities, ", ")
				if i == "" {
					i = "Existing identity settings"
				}
				if value.DefaultIdentity != "" {
					i += " (default: " + value.DefaultIdentity + ")"
				}
				if first {
					profile, identity = p, i
					first = false
				} else {
					if profile != p {
						profile = "Mixed"
					}
					if identity != i {
						identity = "Mixed"
					}
				}
			}
			options = append(options, choice{kind + "-profile", strings.ToUpper(kind) + " profile: " + profile}, choice{kind + "-identities", strings.ToUpper(kind) + " identities: " + identity})
		}
		options = append(options, choice{"rdp-session", "RDP session: Global"})
		if len(hosts) == 1 {
			tunnels, _, e := hostTunnelEntries(draft, hosts)
			if e != nil {
				return e
			}
			options = append(options, choice{"tunnels", fmt.Sprintf("Tunnels: %d configured", len(tunnels))})
		}
		title := fmt.Sprintf("Edit hosts — %d selected", len(hosts))
		if len(hosts) == 1 {
			title = "Edit host — " + storedHosts[targets[0].source+":"+strings.TrimSuffix(targets[0].key, ":"+targets[0].kind)].Alias
		}
		mEditor := newDialog(title, options, "Untouched fields stay unchanged.")
		mEditor.optionDetails = hostEditorDetails(options, len(hosts), allWritable)
		if allWritable {
			mEditor.footerActions = append(mEditor.footerActions, choice{"save-host", "Save host"})
		}
		mEditor.footerActions = append(mEditor.footerActions, choice{"save", "Save my selections"})
		id, back, err := runChoice(mEditor, menuKey(mEditor.title))
		if err != nil || back {
			return err
		}
		if id == "tunnels" {
			if err = editHostTunnels(draft, path, hosts); err != nil {
				return err
			}
			continue
		}
		if id == "rdp-session" {
			if err = editGlobalRDPSession(path); err != nil {
				return err
			}
			data, loaded, _, e := settingsDraft(path)
			if e != nil {
				return e
			}
			original = data
			before.RDP.Session = loaded.RDP.Session
			draft.RDP.Session = loaded.RDP.Session
			continue
		}
		if id == "save" {
			mReview := newDialog("Review personal selections", []choice{{"save", "Save local selections"}}, fmt.Sprintf("Save personal profile and identity changes for %d hosts. No shared database is modified.", len(hosts)))
			mReview.footerActions = []choice{{"select", "Select"}}
			_, back, err = runChoice(mReview, menuKey(mReview.title))
			if err != nil || back {
				return err
			}
			if err = saveSettingsDraft(path, original, before, draft); err != nil {
				return err
			}
			original, before, draft, err = settingsDraft(path)
			if err != nil {
				return err
			}
			continue
		}
		if id == "save-host" {
			if !allWritable {
				return fmt.Errorf("selected sources include read-only hosts")
			}
			if len(fieldEdits) == 0 {
				continue
			}
			for source, s := range sources {
				var batch []InventoryHost
				for _, t := range targets {
					if t.source == source && t.kind == "ssh" {
						batch = append(batch, storedHosts[source+":"+strings.TrimSuffix(t.key, ":ssh")])
					}
				}
				if err = saveHostFields(s, batch, fieldEdits); err != nil {
					return fmt.Errorf("save %s: %w (earlier sources may already be saved)", s.Name, err)
				}
				for _, h := range batch {
					for field, value := range fieldEdits {
						setInventoryHostField(&h, field, value)
					}
					storedHosts[source+":"+h.ID] = h
				}
			}
			fieldEdits = map[string]string{}
			continue
		}
		if id == "name" || id == "address" || id == "tags" || id == "notes" || id == "color" {
			if !allWritable {
				if err = notice("Host fields are read-only. Your profile and identity selections remain editable."); err != nil {
					return err
				}
				continue
			}
			value := ""
			for _, o := range options {
				if o.ID == id {
					_, value, _ = strings.Cut(o.Label, ": ")
					break
				}
			}
			if value == "Mixed" {
				value = ""
			}
			var v string
			var b bool
			var e error
			if id == "color" {
				v, b, e = chooseHostColor(value)
			} else {
				v, b, e = inventoryPrompt(id, value)
			}
			if e != nil {
				return e
			}
			if !b {
				fieldEdits[id] = v
			}
			continue
		}
		kind := strings.Split(id, "-")[0]
		var ref *ProfileReference
		var identities PersonalConnection
		if strings.HasSuffix(id, "-profile") {
			options, refs, e := profileChoices(draft, kind)
			if e != nil {
				return e
			}
			m := newDialog(strings.ToUpper(kind)+" Profiles", options, "Profiles from all enabled sources and your local database. Selection does not edit the profile.")
			m.singleChoice = true
			m.footerActions = []choice{{"select", "Select"}}
			m.current = "existing"
			defaultLabel := ""
			first := true
			for _, t := range targets {
				if t.kind != kind {
					continue
				}
				label := hostDefault(t)
				value := draft.PersonalConnections[t.source][t.key]
				current := "existing"
				if value.Profile != nil {
					current = value.Profile.Source + ":" + value.Profile.ID
				}
				if first {
					defaultLabel = label
					m.current = current
					first = false
				} else {
					if defaultLabel != label {
						defaultLabel = "Host default — varies by host"
					}
					if m.current != current {
						m.current = ""
					}
				}
			}
			m.options[0].Label = "Use " + strings.ToLower(defaultLabel[:1]) + defaultLabel[1:]
			if defaultLabel == "No host default configured" {
				m.options[0].Label = defaultLabel
			}
			for i, o := range m.options {
				if o.ID == m.current {
					m.cursor = i
					break
				}
			}
			m.optionDetails = map[string]string{}
			m.optionDetails["existing"] = defaultLabel + "\nFollows the inventory's default assignment. An explicit profile selection stays pinned to that profile."
			for key, ref := range refs {
				p, e := personalProfile(draft, ref)
				if e != nil {
					m.optionDetails[key] = e.Error()
					continue
				}
				domain := ""
				if p.Domain != nil {
					domain = *p.Domain
				}
				m.optionDetails[key] = fmt.Sprintf("Profile: %s\nSource: %s\nPort: %d\nDomain: %s\nAgent: %s\nCompatibility: KEX %s; host keys %s; public keys %s\nRouting: existing host route retained", p.ProfileName, ref.Source, p.Port, domain, agentChoiceLabel(p.SSHAgent), strings.Join(p.ExtraKexAlgorithms, ", "), strings.Join(p.ExtraHostKeyAlgorithms, ", "), strings.Join(p.ExtraPubkeyAlgorithms, ", "))
			}
			selected, b, e := runChoice(m, menuKey(m.title))
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if selected != "existing" {
				value := refs[selected]
				ref = &value
			}
		} else {
			var b bool
			current := PersonalConnection{}
			for _, t := range targets {
				if t.kind == kind {
					current = draft.PersonalConnections[t.source][t.key]
					break
				}
			}
			identities, b, err = selectPersonalIdentities(draft, kind, current)
			if err != nil {
				return err
			}
			if b {
				continue
			}
		}
		if draft.PersonalConnections == nil {
			draft.PersonalConnections = map[string]map[string]PersonalConnection{}
		}
		for _, t := range targets {
			if t.kind != kind {
				continue
			}
			if draft.PersonalConnections[t.source] == nil {
				draft.PersonalConnections[t.source] = map[string]PersonalConnection{}
			}
			value := draft.PersonalConnections[t.source][t.key]
			if strings.HasSuffix(id, "-profile") {
				value.Profile = ref
			} else {
				value.Identities, value.DefaultIdentity = identities.Identities, identities.DefaultIdentity
			}
			draft.PersonalConnections[t.source][t.key] = value
		}
	}
}

func chooseLaunchIdentity(c *Config, hosts []Host) error {
	if len(hosts) == 0 {
		return nil
	}
	_, r, err := storedRecord(hosts[0], *c)
	if err != nil {
		return err
	}
	selection := c.PersonalConnections[hosts[0].SourceID][personalKey(r)]
	options := []choice{}
	for _, id := range selection.Identities {
		options = append(options, choice{id, id})
	}
	if len(options) == 0 {
		options = append(options, choice{"default", "System / client default"})
	}
	id, back, err := chooseOne(strings.ToUpper(r.kind())+" Identity", options, "This launch only; saved defaults are unchanged.", selection.DefaultIdentity)
	if err != nil {
		return err
	}
	if back {
		return fmt.Errorf("identity selection cancelled")
	}
	*c = cloneAgentSettings(*c)
	if c.PersonalConnections == nil {
		c.PersonalConnections = map[string]map[string]PersonalConnection{}
	}
	for _, h := range hosts {
		_, r, err := storedRecord(h, *c)
		if err != nil {
			return err
		}
		if c.PersonalConnections[h.SourceID] == nil {
			c.PersonalConnections[h.SourceID] = map[string]PersonalConnection{}
		}
		value := c.PersonalConnections[h.SourceID][personalKey(r)]
		value.DefaultIdentity = id
		value.Identities = []string{id}
		c.PersonalConnections[h.SourceID][personalKey(r)] = value
	}
	return nil
}
