package main

import (
	"fmt"
	"strings"

	digletconfig "github.com/kylar514/go-where/internal/tunnelcore"
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
			for _, endpoint := range []struct {
				ref   *ProfileReference
				label string
			}{{r.SourceHost, "Source host"}, {r.DestinationHost, "Destination host"}} {
				if endpoint.ref != nil && wanted[endpoint.ref.Source+":"+endpoint.ref.ID] {
					role = append(role, endpoint.label)
				}
			}
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
			h := Host{SourceID: s.ID, SourceName: s.Name, ConnectionID: r.ID, Alias: r.Alias, Type: "tunnel", Color: byHost[transport.HostID].Color, BracketColor: byHost[transport.HostID].BracketColor, Comment: r.Notes, Tags: r.Tags}
			result = append(result, h)
			roles[hostIdentity(h)] = strings.Join(role, "; ")
		}
	}
	return result, roles, nil
}

func editHostTunnels(c Config, path string, hosts []Host) error {
	if len(hosts) != 1 {
		return fmt.Errorf("tunnel management requires exactly one host")
	}
	host := hosts[0]
	if host.InventoryHostID == "" {
		_, r, err := storedRecord(host, c)
		if err != nil {
			return err
		}
		host.InventoryHostID = r.HostID
	}
	s, err := findSource(c, host.SourceID)
	if err != nil {
		return err
	}
	inventory, err := sourceInventoryHosts(s)
	if err != nil {
		return err
	}
	for _, h := range inventory {
		if h.ID == host.InventoryHostID {
			host.Alias = h.Alias
		}
	}
	hosts = []Host{host}
	previousSettings, previousState := saveActiveSettings, settingsSaveState
	saveActiveSettings, settingsSaveState = nil, nil
	defer func() { saveActiveSettings, settingsSaveState = previousSettings, previousState }()
	for {
		entries, roles, err := hostTunnelEntries(c, hosts)
		if err != nil {
			return err
		}
		options := []choice{{"new", "New tunnel…"}}
		byID := map[string]Host{}
		for _, h := range entries {
			key := hostIdentity(h)
			byID[key] = h
			status := "Stopped"
			conn, e := inventoryTunnel(h, c)
			if e != nil {
				status = "Unavailable"
			} else if statuses, e := tunnelStatuses([]digletconfig.Connection{conn}); e != nil {
				status = "Status unavailable"
			} else {
				info := statuses[conn.Name]
				if info.pid != 0 {
					status = "Running"
				}
				if info.status == 3 {
					status = "Port conflict"
				}
			}
			options = append(options, choice{key, h.Alias + " — " + status + " · " + h.SourceName})
		}
		m := newDialog("Tunnels — "+host.Alias, options, "Select a tunnel to open its editor. Tunnel saves are separate from your host draft.")
		m.footerActions = []choice{{"start", "Start"}, {"stop", "Stop"}}
		m.optionDetails = map[string]string{"new": "Create a tunnel with " + host.Alias + " prefilled as Source host. Its default SSH connection and your local identity are used. Choose a writable database in Save to."}
		if len(entries) == 0 {
			m.optionDetails["new"] = "No tunnels configured for " + host.Alias + ".\n\n" + m.optionDetails["new"]
		}
		for key, h := range byID {
			m.optionDetails[key] = roles[key] + "\n\n" + managedDetails(h, c)
		}
		id, back, err := runChoice(m, menuKey(m.title))
		if err != nil || back {
			return err
		}
		if id == "new" {
			if err = newHostTunnel(c, path, hosts); err != nil {
				if err = notice(err.Error()); err != nil {
					return err
				}
			}
			continue
		}
		if id == "start" || id == "stop" {
			if len(m.filtered) == 0 {
				continue
			}
			h, ok := byID[m.options[m.filtered[m.cursor]].ID]
			if !ok {
				continue
			}
			conn, e := inventoryTunnel(h, c)
			if e == nil || id == "stop" {
				e = toggleManagedTunnel(conn, id == "stop")
			}
			if e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
			}
			continue
		}
		if h, ok := byID[id]; ok {
			s, err := findSource(c, h.SourceID)
			if err != nil {
				return err
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
	if databaseSettings(path) {
		var ref *ProfileReference
		if len(hosts) == 1 {
			id := hosts[0].InventoryHostID
			if id != "" {
				value := ProfileReference{hosts[0].SourceID, id}
				ref = &value
			}
		}
		return editTunnelDefinition(c, &PickerState{Mode: "tunnels", ConfigPath: path}, nil, ref)
	}
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
