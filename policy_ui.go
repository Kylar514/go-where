package main

import "fmt"

func editSourcePolicies(c Config, path string) error {
	s, back, err := writableSource(c, "")
	if err != nil || back {
		return err
	}
	for {
		profiles, err := sourceProfiles(s)
		if err != nil {
			return err
		}
		options := []choice{}
		for _, p := range profiles {
			options = append(options, choice{p.ID, p.ProfileName + ": " + p.kind()})
		}
		id, b, err := choose("Connection policies", options, "Shared database settings. Saves are separate from local Settings. Editing affects every host using the policy.")
		if err != nil || b {
			return err
		}
		var p Connection
		for _, candidate := range profiles {
			if candidate.ID == id {
				p = candidate
			}
		}
		for {
			rows, err := sourceConnections(s)
			if err != nil {
				return err
			}
			uses := 0
			for _, r := range rows {
				if r.ProfileID == p.ID {
					uses++
				}
			}
			action, b, err := choose("Policy — "+p.ProfileName, []choice{{"settings", "Connection settings…"}, {"name", "Name: " + p.ProfileName}, {"duplicate", "Duplicate policy"}, {"save", "Save shared policy"}}, fmt.Sprintf("Used by %d connections. Save writes shared inventory separately; it is not part of the local Settings draft.", uses))
			if err != nil {
				return err
			}
			if b {
				break
			}
			switch action {
			case "settings":
				ps := PickerState{Source: s.ID, ConfigPath: path}
				if err = editConnectionDraft(c, &ps, nil, &p); err != nil {
					return err
				}
			case "name":
				name, b, e := inventoryPrompt("Policy name", p.ProfileName)
				if e != nil {
					return e
				}
				if !b {
					p.ProfileName = name
				}
			case "duplicate":
				p.ID = inventoryID()
				p.ProfileName += "-copy"
			case "save":
				_, b, e := choose("Save shared policy?", []choice{{"confirm", "Save changes to shared inventory"}}, fmt.Sprintf("Policy %s affects %d connections. This save is separate from local Settings.", p.ProfileName, uses))
				if e != nil {
					return e
				}
				if b {
					continue
				}
				if e = saveReusableProfile(s, p); e != nil {
					return e
				}
			}
		}
	}
}
