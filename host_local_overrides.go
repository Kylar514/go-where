package main

import (
	"reflect"
	"strings"
)

func editHostLocalOverrides(path string, s InventorySource, h InventoryHost) error {
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		return err
	}
	previousSave, previousState := saveActiveSettings, settingsSaveState
	defer func() { saveActiveSettings, settingsSaveState = previousSave, previousState }()
	saved := false
	saveActiveSettings = func() error {
		if err := saveSettingsDraft(path, original, before, draft); err != nil {
			return err
		}
		data, err := readSettingsData(path)
		if err != nil {
			return err
		}
		original = data
		before = cloneAgentSettings(draft)
		saved = true
		return nil
	}
	settingsSaveState = func() string {
		if !reflect.DeepEqual(before, draft) {
			return "Unsaved changes"
		}
		if saved {
			return "Saved"
		}
		return "No unsaved changes"
	}
	records, err := sourceConnections(s)
	if err != nil {
		return err
	}
	for {
		address := "Inherit host address"
		if o := draft.HostOverrides[s.ID][h.ID]; o.Address != nil {
			address = *o.Address
		}
		options := []choice{{"address", "Address override: " + address}}
		for _, p := range records {
			if p.HostID == h.ID && (p.kind() == "ssh" || p.kind() == "rdp") {
				options = append(options, choice{p.ID, strings.ToUpper(p.kind()) + " profile: " + p.ProfileName})
			}
		}
		id, back, err := choose("Local overrides — "+h.Alias, options, "This device only — shared inventory is unchanged.\n\nAddress applies to the host. SSH overrides apply only to the selected host/profile attachment.\n\nSave settings persists this local draft separately from host edits.")
		if err != nil || back {
			return err
		}
		if id == "address" {
			current := "inherit"
			if draft.HostOverrides[s.ID][h.ID].Address != nil {
				current = "custom"
			}
			mode, b, e := chooseOne("Address override", []choice{{"inherit", "Inherit host address"}, {"custom", "Set local address…"}}, "Existing profile-specific address overrides take precedence.", current)
			if e != nil {
				return e
			}
			if b {
				continue
			}
			var address *string
			if mode == "custom" {
				v, b, e := inventoryPrompt("Local address", h.Address)
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
			continue
		}
		for _, profile := range records {
			if profile.ID == id {
				if err = editLocalProfileFields(&draft, s, profile, h.Alias); err != nil {
					return err
				}
				break
			}
		}
	}
}
