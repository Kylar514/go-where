package main

import "fmt"

// Save checks reference structure, never launch readiness: no key expansion,
// agent access, algorithm probes or credential retrieval.
func validateEditedHostRoutes(c Config, hosts []Host) error {
	cache := map[string][]Connection{}
	rowsFor := func(source string) ([]Connection, error) {
		if rows, ok := cache[source]; ok {
			return rows, nil
		}
		s, err := findSource(c, source)
		if err != nil {
			return nil, err
		}
		rows, err := effectiveSourceConnections(c, s)
		if err == nil {
			cache[source] = rows
		}
		return rows, err
	}
	var walk func(string, Connection, map[string]bool) error
	walk = func(source string, r Connection, seen map[string]bool) error {
		key := source + ":" + r.ID
		if seen[key] {
			return fmt.Errorf("jump cycle for connection %s", r.ID)
		}
		seen[key] = true
		defer delete(seen, key)
		nextSource, nextID := source, r.JumpID
		if jump := c.PersonalConnections[source][r.HostID+":route"].Jump; jump != nil {
			if jump.Source == "direct" {
				return nil
			}
			nextSource = jump.Source
			rows, err := rowsFor(nextSource)
			if err != nil {
				return err
			}
			nextID = ""
			for _, candidate := range rows {
				if candidate.HostID == jump.ID && candidate.kind() == "ssh" && candidate.Default {
					nextID = candidate.ID
					break
				}
			}
			if nextID == "" {
				return fmt.Errorf("jump host has no default SSH connection")
			}
		}
		if nextID == "" {
			return nil
		}
		rows, err := rowsFor(nextSource)
		if err != nil {
			return err
		}
		for _, candidate := range rows {
			if candidate.ID == nextID {
				if candidate.kind() != "ssh" {
					return fmt.Errorf("jump must reference an SSH connection")
				}
				return walk(nextSource, candidate, seen)
			}
		}
		return fmt.Errorf("jump connection %s no longer exists", nextID)
	}
	for _, h := range hosts {
		rows, err := rowsFor(h.SourceID)
		if err != nil {
			return err
		}
		hostID := h.InventoryHostID
		if hostID == "" {
			for _, r := range rows {
				if r.ID == h.ConnectionID {
					hostID = r.HostID
					break
				}
			}
		}
		for _, r := range rows {
			if r.HostID == hostID && r.kind() != "tunnel" {
				if err = walk(h.SourceID, r, map[string]bool{}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
