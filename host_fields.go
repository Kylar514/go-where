package main

import (
	"fmt"
	"reflect"
	"strings"
)

func inventoryHostField(h InventoryHost, field string) string {
	switch field {
	case "name":
		return h.Alias
	case "address":
		return h.Address
	case "tags":
		return strings.Join(h.Tags, ", ")
	case "notes":
		return h.Notes
	case "color":
		return h.Color
	case "bracket_color":
		return h.BracketColor
	}
	return ""
}

func setInventoryHostField(h *InventoryHost, field, value string) {
	switch field {
	case "name":
		h.Alias = value
	case "address":
		h.Address = value
	case "notes":
		h.Notes = value
	case "color":
		h.Color = value
	case "bracket_color":
		h.BracketColor = value
	case "tags":
		h.Tags = nil
		for _, tag := range strings.Split(value, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				h.Tags = append(h.Tags, tag)
			}
		}
	}
}

// Only host metadata is written; attachments, routes and reusable profiles are
// never rewritten by the combined editor's shared save.
func saveHostFields(s InventorySource, before []InventoryHost, edits map[string]string) error {
	if s.ReadOnly {
		return fmt.Errorf("source is read-only")
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
	current, err := readInventoryHosts(tx)
	if err != nil {
		return err
	}
	byID := map[string]InventoryHost{}
	for _, h := range current {
		byID[h.ID] = h
	}
	for _, h := range before {
		if !reflect.DeepEqual(h, byID[h.ID]) {
			return fmt.Errorf("host changed while editing; reopen and retry")
		}
		for field, value := range edits {
			setInventoryHostField(&h, field, value)
		}
		if err = writeInventoryHost(tx, h); err != nil {
			return err
		}
	}
	if _, err = readInventoryHosts(tx); err != nil {
		return err
	}
	rows, err := readHostConnections(tx)
	if err != nil {
		return err
	}
	if err = validateConnections(rows); err != nil {
		return err
	}
	if err = validateHostProfiles(tx, rows); err != nil {
		return err
	}
	return tx.Commit()
}
