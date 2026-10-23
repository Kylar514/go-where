package main

import (
	"fmt"
	"strings"
)

func hostTunnelEntries(c Config, selected []Host) ([]Host, map[string]string, error) {
	wanted := map[string]bool{}
	for _, h := range selected {
		hostID := h.InventoryHostID
		if hostID == "" {
			_, r, err := storedRecord(h, c)
			if err != nil {
				return nil, nil, err
			}
			hostID = r.HostID
		}
		wanted[h.SourceID+":"+hostID] = true
	}
	var result []Host
	roles := map[string]string{}
	for _, s := range c.Sources {
		if !sourceEnabled(c, s.ID) {
			continue
		}
		rows, err := sourceConnections(s)
		if err != nil {
			return nil, nil, err
		}
		inventory, err := sourceInventoryHosts(s)
		if err != nil {
			return nil, nil, err
		}
		byHost := map[string]InventoryHost{}
		for _, h := range inventory {
			byHost[h.ID] = h
		}
		byID := map[string]Connection{}
		for _, r := range rows {
			byID[r.ID] = r
		}
		for _, r := range rows {
			if r.kind() != "tunnel" {
				continue
			}
			var role []string
			transport := byID[r.SSHID]
			if wanted[s.ID+":"+transport.HostID] && transport.HostID != "" {
				role = append(role, "SSH transport: "+byHost[transport.HostID].Alias)
			}
			if r.RemoteHostID != "" && wanted[s.ID+":"+r.RemoteHostID] {
				role = append(role, "Remote target: "+byHost[r.RemoteHostID].Alias)
			}
			if len(role) == 0 {
				continue
			}
			h := Host{SourceID: s.ID, SourceName: s.Name, ConnectionID: r.ID, Alias: r.Alias, Type: "tunnel", Color: byHost[transport.HostID].Color, Comment: r.Notes, Tags: r.Tags}
			result = append(result, h)
			roles[hostIdentity(h)] = strings.Join(role, "; ")
		}
	}
	return result, roles, nil
}

func editHostTunnels(c Config, path string, hosts []Host) error {
	previousSettings, previousState := saveActiveSettings, settingsSaveState
	saveActiveSettings, settingsSaveState = nil, nil
	defer func() { saveActiveSettings, settingsSaveState = previousSettings, previousState }()
	query := ""
	for {
		entries, roles, err := hostTunnelEntries(c, hosts)
		if err != nil {
			return err
		}
		m := newPickerModel(entries, query, "current", Origin{}, PickerState{Mode: "tunnels", ConfigPath: path}, c)
		m.hostTunnelTitle = fmt.Sprintf("Tunnels — %d hosts", len(hosts))
		if len(hosts) == 1 {
			m.hostTunnelTitle = "Tunnels — " + hosts[0].Alias
		}
		m.hostTunnelRoles = roles
		if _, err = runTUI(m); err != nil {
			return err
		}
		query = m.search.Value()
		switch m.key {
		case "cancel", "":
			return nil
		case "new-connection":
			if err = newHostTunnel(c, path, hosts); err != nil {
				if err = notice(err.Error()); err != nil {
					return err
				}
			}
		case "ctrl-e", "ctrl-alt-e":
			if len(m.selected) == 0 {
				continue
			}
			h := m.selected[0]
			s, err := findSource(c, h.SourceID)
			if err != nil {
				return err
			}
			if s.ReadOnly {
				if err = notice("This tunnel source is read-only. Start/Stop remains available."); err != nil {
					return err
				}
				continue
			}
			state := PickerState{Mode: "tunnels", Source: s.ID, ConfigPath: path}
			if err = editConnection(c, &state, &h); err != nil {
				if err = notice(err.Error()); err != nil {
					return err
				}
			}
		}
	}
}

func newHostTunnel(c Config, path string, hosts []Host) error {
	wanted := map[string]bool{}
	for _, h := range hosts {
		id := h.InventoryHostID
		if id == "" {
			_, r, err := storedRecord(h, c)
			if err != nil {
				return err
			}
			id = r.HostID
		}
		wanted[h.SourceID+":"+id] = true
	}
	type candidate struct {
		s InventorySource
		r Connection
	}
	choices := []choice{}
	candidates := map[string]candidate{}
	for _, s := range c.Sources {
		if s.ReadOnly || !sourceEnabled(c, s.ID) {
			continue
		}
		rows, err := sourceConnections(s)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if r.kind() == "ssh" && wanted[s.ID+":"+r.HostID] {
				key := s.ID + ":" + r.ID
				candidates[key] = candidate{s, r}
				choices = append(choices, choice{key, sshReferenceLabel(c, s, r.ID) + " — " + s.Name})
			}
		}
	}
	if len(choices) == 0 {
		return fmt.Errorf("a selected host needs a saved SSH connection in a writable source to create a tunnel")
	}
	id := choices[0].ID
	if len(choices) > 1 {
		value, back, err := chooseOne("Tunnel SSH transport", choices, "Choose one selected host connection. A new tunnel is not duplicated across the selection.", id)
		if err != nil || back {
			return err
		}
		id = value
	}
	chosen := candidates[id]
	name, back, err := inventoryPrompt("Tunnel name", "")
	if err != nil || back {
		return err
	}
	r := Connection{ID: inventoryID(), Alias: name, Type: "tunnel", TunnelType: "ssh", SSHID: chosen.r.ID, Bind: "127.0.0.1", LocalPort: 8080, RemoteHost: "localhost", RemotePort: 80}
	previousSave, previousState := saveActiveTunnel, tunnelSaveState
	defer func() { saveActiveTunnel, tunnelSaveState = previousSave, previousState }()
	saved := false
	saveActiveTunnel = func() error {
		if saved {
			if err := ensureInventoryTunnelStopped(Host{SourceID: chosen.s.ID, ConnectionID: r.ID, Type: "tunnel"}); err != nil {
				return err
			}
		}
		if err := changeConnection(chosen.s, r, false); err != nil {
			return err
		}
		saved = true
		return nil
	}
	tunnelSaveState = func() string { return "Tunnel saves separately" }
	state := PickerState{Mode: "tunnels", Source: chosen.s.ID, ConfigPath: path}
	return editConnectionDraft(c, &state, nil, &r)
}
