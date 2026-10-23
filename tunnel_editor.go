package main

import (
	"fmt"
	"strings"
)

var saveActiveTunnel func() error
var tunnelSaveState func() string

func defaultTunnelHost(c Config, ref ProfileReference) (Host, error) {
	s, err := findSource(c, ref.Source)
	if err != nil {
		return Host{}, err
	}
	if !sourceEnabled(c, s.ID) {
		return Host{}, fmt.Errorf("host source is disabled")
	}
	rows, err := sourceConnections(s)
	if err != nil {
		return Host{}, err
	}
	hosts, err := sourceInventoryHosts(s)
	if err != nil {
		return Host{}, err
	}
	for _, r := range rows {
		if r.HostID == ref.ID && r.kind() == "ssh" && r.Default {
			for _, h := range hosts {
				if h.ID == ref.ID {
					return Host{SourceID: s.ID, SourceName: s.Name, InventoryHostID: h.ID, ConnectionID: r.ID, Alias: h.Alias, Hostname: h.Address, Type: "ssh"}, nil
				}
			}
		}
	}
	return Host{}, fmt.Errorf("host has no default SSH connection; configure it in Hosts")
}

func chooseTunnelHost(c Config, current *ProfileReference) (*ProfileReference, bool, error) {
	options := []choice{}
	refs := map[string]ProfileReference{}
	selected := ""
	if current != nil {
		selected = current.Source + ":" + current.ID
	}
	for _, s := range c.Sources {
		if !sourceEnabled(c, s.ID) {
			continue
		}
		hosts, err := sourceInventoryHosts(s)
		if err != nil {
			return nil, false, err
		}
		for _, h := range hosts {
			ref := ProfileReference{s.ID, h.ID}
			key := s.ID + ":" + h.ID
			refs[key] = ref
			options = append(options, choice{key, h.Alias + " — " + s.Name})
		}
	}
	id, back, err := chooseOne("SSH host", options, "Select a saved host. Its default SSH connection and your local identity are used.", selected)
	if err != nil || back {
		return nil, back, err
	}
	ref := refs[id]
	if _, err = defaultTunnelHost(c, ref); err != nil {
		return nil, false, err
	}
	return &ref, false, nil
}

func editTunnelDefinition(c Config, state *PickerState, h *Host, source *ProfileReference) error {
	var s InventorySource
	var err error
	var r Connection
	if h != nil {
		s, r, err = storedRecord(*h, c)
	} else {
		s, _, err = writableSource(c, state.Source, state.ConfigPath)
		r = Connection{ID: inventoryID(), Type: "tunnel", TunnelType: "ssh", SourceHost: source, Bind: "127.0.0.1", RemoteHost: "localhost", LocalPort: 8080, RemotePort: 80}
	}
	if err != nil {
		return err
	}
	previousSave, previousState := saveActiveSettings, settingsSaveState
	saveActiveSettings, settingsSaveState = nil, nil
	defer func() { saveActiveSettings, settingsSaveState = previousSave, previousState }()
	for {
		name := func(ref *ProfileReference, fallback string) string {
			if ref == nil {
				return fallback
			}
			host, e := defaultTunnelHost(c, *ref)
			if e != nil {
				return "Unavailable: " + e.Error()
			}
			return host.Alias + " — " + host.SourceName
		}
		typeLabel := "Local forward"
		if r.TunnelType == "bridge" {
			typeLabel = "Bridge"
		}
		if r.TunnelType == "kubectl" {
			return editConnectionDraft(c, state, h, nil)
		}
		options := []choice{{"type", "Tunnel type: " + typeLabel}, {"name", "Tunnel name: " + r.Alias}, {"source-host", "Source host: " + name(r.SourceHost, sshReferenceLabel(c, s, r.SSHID))}, {"source-address", "Source address: " + r.RemoteHost}, {"source-port", fmt.Sprintf("Source port: %d", r.RemotePort)}}
		if r.TunnelType == "bridge" {
			options = append(options, choice{"destination-host", "Destination host: " + name(r.DestinationHost, "Not selected")})
		}
		options = append(options, choice{"destination-address", "Destination address: " + r.Bind}, choice{"destination-port", fmt.Sprintf("Destination port: %d", r.LocalPort)}, choice{"save-to", "Save to: " + s.Name})
		m := newDialog("Tunnel — "+s.Name, options, "")
		m.optionDetails = tunnelEditorDetails(options, r, s)
		m.footerActions = []choice{{"save", "Save tunnel"}}
		if s.ReadOnly {
			m.footerActions = []choice{{"close", "Close"}}
			m.text = "Read-only tunnel definition. Viewing does not change its source."
		}
		id, back, e := runChoice(m, menuKey(m.title))
		if e != nil || back {
			return e
		}
		if id == "close" {
			return nil
		}
		if s.ReadOnly {
			continue
		}
		switch id {
		case "type":
			value, b, e := chooseOne("Tunnel type", []choice{{"ssh", "Local forward"}, {"bridge", "Bridge"}}, "Bridge needs SSH forwarding permission on both servers.", r.TunnelType)
			if e != nil {
				return e
			}
			if !b {
				r.TunnelType = value
			}
		case "source-host", "destination-host":
			current := r.SourceHost
			if id == "destination-host" {
				current = r.DestinationHost
			}
			ref, b, e := chooseTunnelHost(c, current)
			if e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
				continue
			}
			if !b {
				if id == "source-host" {
					r.SourceHost = ref
					r.SSHID = ""
				} else {
					r.DestinationHost = ref
				}
			}
		case "save-to":
			next, b, e := writableSource(c, "", state.ConfigPath)
			if e != nil {
				return e
			}
			if !b {
				if h != nil && next.ID != s.ID {
					if e = notice("Changing source requires copying this existing tunnel; create a new definition instead."); e != nil {
						return e
					}
					continue
				}
				s = next
			}
		case "save":
			if r.SourceHost != nil {
				if _, e = defaultTunnelHost(c, *r.SourceHost); e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
			}
			if r.TunnelType == "bridge" {
				if r.DestinationHost == nil {
					if e = notice("Select a destination host."); e != nil {
						return e
					}
					continue
				}
				if _, e = defaultTunnelHost(c, *r.DestinationHost); e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
			}
			if h != nil {
				if e = ensureInventoryTunnelStopped(*h); e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
			}
			if e = changeConnection(s, r, false); e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
				continue
			}
			state.Source = s.ID
			state.Focus = s.ID + ":" + r.ID
			return nil
		default:
			value := r.Alias
			switch id {
			case "source-address":
				value = r.RemoteHost
			case "destination-address":
				value = r.Bind
			case "source-port":
				value = fmt.Sprint(r.RemotePort)
			case "destination-port":
				value = fmt.Sprint(r.LocalPort)
			}
			v, b, e := inventoryPrompt(id, value)
			if e != nil {
				return e
			}
			if b {
				continue
			}
			switch id {
			case "name":
				r.Alias = v
			case "source-address":
				r.RemoteHost = v
				r.RemoteHostID = ""
			case "destination-address":
				r.Bind = v
			case "source-port", "destination-port":
				p, e := connectionPort(v)
				if e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
				if id == "source-port" {
					r.RemotePort = p
				} else {
					r.LocalPort = p
				}
			}
		}
	}
}

func tunnelEditorDetails(options []choice, r Connection, s InventorySource) map[string]string {
	listener := "this computer"
	if r.TunnelType == "bridge" {
		listener = "the destination SSH host"
	}
	details := map[string]string{
		"type":                "Tunnel type\nLocal forward makes the service available on this computer. Bridge makes it available on another SSH host through this computer. Bridge needs forwarding permission on both SSH servers.",
		"name":                "Tunnel name\nA display name for this tunnel. It does not select or link a host.",
		"source-host":         "Source host\nSelect the saved SSH host providing access to the service. Its default SSH connection and your effective local identity are used.",
		"source-address":      "Source address\nAddress reached from the source SSH host. localhost means the source host itself, not your computer. Another address can identify a service reachable from that host.",
		"source-port":         "Source port\nThe actual service port at the source address, reached from the source SSH host.",
		"destination-host":    "Destination host\nBridge only: select the saved SSH host where the loopback listener opens. Its default SSH connection and your local identity are used.",
		"destination-address": "Destination address\nThe listener address on " + listener + ". Only loopback addresses are allowed; it is not exposed to other computers.",
		"destination-port":    "Destination port\nThe port used to access the forwarded service on " + listener + ". Choose a port not already in use there.",
		"save-to":             "Save to\nThe inventory database receiving this tunnel definition. Tunnel saves are separate from host and personal-selection saves.\n\nDatabase: " + s.Path,
	}
	values := map[string]string{}
	for _, o := range options {
		_, v, _ := strings.Cut(o.Label, ": ")
		values[o.ID] = v
	}
	destination := "This computer"
	if r.TunnelType == "bridge" {
		destination = values["destination-host"]
	}
	route := fmt.Sprintf("Service: %s → %s:%d\nAvailable at: %s → %s:%d", values["source-host"], r.RemoteHost, r.RemotePort, destination, r.Bind, r.LocalPort)
	permission := "Writable"
	if s.ReadOnly {
		permission = "Read-only"
	}
	details["save-to"] += "\nPermissions: " + permission
	for _, o := range options {
		details[o.ID] += "\n\nCurrent: " + values[o.ID] + "\n\n" + route
	}
	return details
}
