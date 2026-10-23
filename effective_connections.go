package main

import "fmt"

// Overlay protocol availability without writing attachments into shared sources.
func effectiveSourceConnections(c Config, s InventorySource) ([]Connection, error) {
	rows, err := sourceConnections(s)
	if err != nil {
		return nil, err
	}
	hosts, err := sourceInventoryHosts(s)
	if err != nil {
		return nil, err
	}
	var result []Connection
	exists := map[string]bool{}
	for _, r := range rows {
		selection := c.PersonalConnections[s.ID][personalKey(r)]
		if r.kind() != "tunnel" && selection.Disabled {
			continue
		}
		result = append(result, r)
		exists[personalKey(r)] = true
	}
	for _, h := range hosts {
		for _, kind := range []string{"ssh", "rdp"} {
			key := h.ID + ":" + kind
			selection := c.PersonalConnections[s.ID][key]
			if exists[key] || selection.Disabled || selection.Profile == nil {
				continue
			}
			p, err := personalProfile(c, *selection.Profile)
			if err != nil {
				return nil, err
			}
			if p.kind() != kind {
				return nil, fmt.Errorf("personal profile protocol mismatch")
			}
			p.ID = "local-" + h.ID + "-" + kind
			p.Alias = p.ID
			p.HostID = h.ID
			p.Default = true
			p.Address = h.Address
			p.Tags = h.Tags
			p.Notes = h.Notes
			result = append(result, p)
		}
	}
	return result, nil
}
