package main

import (
	"fmt"
	"strings"
)

func connectionWithType(old Connection, kind string) Connection {
	r := Connection{ID: old.ID, Type: kind, Alias: old.Alias, Tags: old.Tags, Notes: old.Notes}
	if old.kind() != "tunnel" && kind != "tunnel" {
		r.Address, r.User, r.JumpID = old.Address, old.User, old.JumpID
	}
	switch kind {
	case "ssh":
		r.Port = 22
	case "rdp":
		r.Port = 3389
	case "tunnel":
		r.TunnelType = "ssh"
		r.Bind = "127.0.0.1"
		r.LocalPort = 8080
		r.RemoteHost = "localhost"
		r.RemotePort = 80
	}
	return r
}

func connectionFields(r Connection, s InventorySource, c Config) []choice {
	fields := []choice{{"type", "Type: " + r.kind()}, {"source", "Source: " + s.Name}, {"alias", "Alias: " + r.Alias}}
	if r.kind() != "tunnel" {
		fields = append(fields, choice{"address", "Address: " + r.Address}, choice{"port", fmt.Sprintf("Port: %d", r.Port)})
	}
	switch r.kind() {
	case "ssh":
		fields = append(fields, choice{"user", "User: " + r.User}, choice{"key", "Identity file: " + r.IdentityFile}, choice{"jump", "Jump SSH profile: " + sshReferenceLabel(c, s, r.JumpID)})
		label := agentChoiceLabel(r.SSHAgent)
		if r.SSHAgent == "" {
			label = "Unspecified (system default)"
		}
		name, origin := effectiveAgent(c, s.ID, r)
		fields = append(fields, choice{"ssh_agent", "Agent: " + label}, choice{"local_agent", "Local agent override… (separate settings)"}, choice{"agent_info", "Effective agent: " + agentChoiceLabel(name) + " — " + origin})
		fields = append(fields, choice{"compatibility", "SSH compatibility… (identities / algorithms)"}, choice{"local_compatibility", "Local compatibility override… (separate settings)"})
	case "rdp":
		domain := "(local default)"
		if r.Domain != nil {
			domain = *r.Domain
		}
		label := "Custom"
		if r.Account != "" {
			label = "Saved local — " + r.Account
		}
		fields = append(fields, choice{"account", "Local RDP identity: " + label})
		if r.Account == "" {
			fields = append(fields, choice{"user", "Username: " + r.User}, choice{"domain", "Domain: " + domain})
		} else {
			a, err := connectionAccount(c, r)
			if err != nil {
				fields = append(fields, choice{"account_info", "Saved account unavailable — choose another account or Custom"})
			} else {
				fields = append(fields, choice{"account_info", "Username: " + a.User + " (saved, read-only)"})
				if a.Domain != nil {
					fields = append(fields, choice{"account_info", "Domain: " + *a.Domain + " (saved, read-only)"})
				} else {
					fields = append(fields, choice{"domain", "Domain: " + domain})
				}
			}
		}
		fields = append(fields, choice{"jump", "SSH jump profile: " + sshReferenceLabel(c, s, r.JumpID)})
	case "tunnel":
		fields = append(fields, choice{"tunnel_type", "Tunnel backend: " + r.TunnelType}, choice{"bind", "Local bind: " + r.Bind}, choice{"local_port", fmt.Sprintf("Local port: %d", r.LocalPort)}, choice{"remote_port", fmt.Sprintf("Remote port: %d", r.RemotePort)})
		if r.TunnelType == "ssh" {
			target := r.RemoteHost
			if r.RemoteHostID != "" {
				target = "Saved host: " + r.RemoteHostID
			}
			fields = append(fields, choice{"ssh_id", "SSH profile: " + sshReferenceLabel(c, s, r.SSHID)}, choice{"remote_target", "Remote target: " + target})
			if r.RemoteHostID == "" {
				fields = append(fields, choice{"remote_host", "Remote address: " + r.RemoteHost})
			}
		} else {
			fields = append(fields, choice{"resource", "Resource: " + r.Resource}, choice{"namespace", "Namespace: " + r.Namespace}, choice{"context", "Context: " + r.Context})
		}
	}
	if r.kind() == "tunnel" {
		order := []string{"type", "alias", "source", "tunnel_type", "ssh_id", "resource", "namespace", "context", "bind", "local_port", "remote_target", "remote_host", "remote_port"}
		var sorted []choice
		for _, id := range order {
			for _, field := range fields {
				if field.ID == id {
					sorted = append(sorted, field)
				}
			}
		}
		fields = sorted
	}
	return append(fields, choice{"tags", "Tags: " + strings.Join(r.Tags, ", ")}, choice{"notes", "Notes: " + r.Notes}, choice{"save", "Save connection"})
}

func sshReferenceLabel(c Config, s InventorySource, id string) string {
	if id == "" {
		return "None"
	}
	_, profile, err := storedRecord(Host{SourceID: s.ID, ConnectionID: id}, c)
	if err != nil {
		return id + " (unavailable)"
	}
	hosts, err := sourceInventoryHosts(s)
	if err != nil {
		return id + " (unavailable)"
	}
	for _, host := range hosts {
		if host.ID == profile.HostID {
			return host.Alias + " / " + profile.ProfileName
		}
	}
	return id
}
