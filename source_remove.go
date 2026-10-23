package main

import (
	"fmt"
	"strings"
)

func removeSourceRegistration(path string, c Config, state *PickerState, id string) error {
	s, err := findSource(c, id)
	if err != nil {
		return err
	}
	_, back, err := choose("Remove source — "+s.Name, []choice{{"remove", "Remove from go-where; keep database file"}}, s.Name+"\n"+s.Path+"\n\nThe SQLite file will NOT be deleted.")
	if err != nil || back {
		return err
	}
	original, before, draft, err := settingsDraft(path)
	if err != nil {
		return err
	}
	identities := before.IdentityAssignments[id]
	if len(before.Overrides[id]) > 0 || len(before.HostOverrides[id]) > 0 || identities.Default != "" || len(identities.Profiles) > 0 || len(identities.Connections) > 0 {
		_, back, err = choose("Discard source-local overrides?", []choice{{"discard", "Remove registration and its local overrides"}}, "Local profile and host overrides for "+s.Name+" will be removed. Shared inventory remains in the database.")
		if err != nil || back {
			return err
		}
	}
	// Confirmed draft save uses stale-write protection and preserves JSONC comments.
	var sources []InventorySource
	for _, source := range draft.Sources {
		if source.ID != id {
			sources = append(sources, source)
		}
	}
	draft.Sources = sources
	var disabled []string
	for _, source := range draft.DisabledSources {
		if source != id {
			disabled = append(disabled, source)
		}
	}
	draft.DisabledSources = disabled
	if draft.StartupSource == id {
		draft.StartupSource = ""
	}
	delete(draft.Overrides, id)
	delete(draft.HostOverrides, id)
	delete(draft.IdentityAssignments, id)
	if err = saveSettingsDraft(path, original, before, draft); err != nil {
		return fmt.Errorf("remove source: %w", err)
	}
	if state.Source == id {
		state.Source = ""
		state.Focus = ""
	}
	var marked []string
	for _, mark := range state.Marked {
		if !strings.HasPrefix(mark, id+":") {
			marked = append(marked, mark)
		}
	}
	state.Marked = marked
	return nil
}
