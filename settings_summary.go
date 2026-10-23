package main

import (
	"fmt"
	"strings"
)

func startupTabLabel(mode string) string {
	if mode == "" {
		mode = "ssh"
	}
	if mode == "tunnels" {
		return "Tunnels"
	}
	if mode == "legacy-scp" {
		return "Legacy SCP"
	}
	if mode == "rsync" {
		return "Rsync"
	}
	return strings.ToUpper(mode)
}

func settingsSummaries(c Config) (map[string]string, map[string]string) {
	values := map[string]string{"general": startupTabLabel(c.StartupMode), "agents": fmt.Sprintf("%d configured", len(c.SSHAgents)), "identities": fmt.Sprintf("%d configured", len(c.Identities)), "rdp-identities": fmt.Sprintf("%d configured", len(c.RDP.Accounts)), "hotkeys": "Defaults"}
	if len(c.Hotkeys) > 0 {
		values["hotkeys"] = fmt.Sprintf("%d customized actions", len(c.Hotkeys))
	}
	var enabled []string
	hosts, ssh, rdp := 0, 0, 0
	unavailable := false
	for _, p := range c.LocalProfiles {
		if p.kind() == "ssh" {
			ssh++
		} else if p.kind() == "rdp" {
			rdp++
		}
	}
	for _, s := range c.Sources {
		if !sourceEnabled(c, s.ID) {
			continue
		}
		enabled = append(enabled, s.Name)
		h, e := sourceInventoryHosts(s)
		if e != nil {
			unavailable = true
		} else {
			hosts += len(h)
		}
		profiles, e := sourceProfiles(s)
		if e != nil {
			unavailable = true
			continue
		}
		for _, p := range profiles {
			if p.kind() == "ssh" {
				ssh++
			} else if p.kind() == "rdp" {
				rdp++
			}
		}
	}
	values["sources"] = strings.Join(enabled, ", ")
	if len(enabled) == 0 {
		values["sources"] = "None enabled"
	}
	values["hosts"] = fmt.Sprintf("%d hosts", hosts)
	values["ssh-profiles"] = fmt.Sprintf("%d available", ssh)
	values["rdp-profiles"] = fmt.Sprintf("%d available", rdp)
	if unavailable {
		for _, id := range []string{"hosts", "ssh-profiles", "rdp-profiles"} {
			values[id] += " · source unavailable"
		}
	}
	folders := 0
	for _, f := range c.RDP.Session.Folders {
		if f.Enabled {
			folders++
		}
	}
	display := displayModeLabel(c.RDP.Session.Fullscreen)
	clipboard := "Clipboard " + strings.ToLower(boolSettingLabel(c.RDP.Session.Clipboard))
	values["rdp-session"] = fmt.Sprintf("%s · %s · %d shared folders", display, clipboard, folders)
	details := map[string]string{
		"general":        "Startup tab\nChoose the tab shown when go-where opens. An explicit command such as go-where rdp takes precedence.",
		"hosts":          "Hosts\nManage every inventory host, including hosts without SSH or RDP connections. Host fields save to their source; your profile and identity selections save locally. Read-only sources remain protected.",
		"ssh-profiles":   "SSH Profiles\nManage reusable port, agent and compatibility settings from enabled sources and Local. Editing a shared profile affects every host using it. Existing attachment jump routes are retained during migration.",
		"rdp-profiles":   "RDP Profiles\nManage reusable RDP port and domain settings. Editing a shared profile affects every host using it. Existing jump routes are retained; session preferences are configured separately.",
		"identities":     "SSH Identities\nManage device-local usernames and keys. Select available identities and a default in the host editor. These credentials never modify shared inventory.",
		"rdp-identities": "RDP Identities\nManage your device-local RDP logins and existing credential providers. Select available identities and a default per host. New encrypted password storage is deferred.",
		"rdp-session":    "RDP Session Settings\nConfigure display, clipboard and folder sharing for all RDP sessions on this device. Local folder paths never enter shared inventory. Unavailable enabled folders stop launch.",
		"agents":         "SSH Agents\nDefine device-local signing-agent sockets or Windows pipes selected by SSH profiles. This does not enable agent forwarding. Missing named endpoints fail rather than silently falling back.",
		"sources":        "Enabled Sources\nChoose which registered inventories load at launch and appear in host and profile lists. Disabling a source does not delete its database.",
		"hotkeys":        "Hotkeys\nCustomize keyboard actions and restore defaults. Conflicting bindings are validated before saving.",
		"policies":       "Connection profiles\nEdit reusable shared SSH and RDP settings. Inventory saves are separate from your personal settings draft.",
		"overrides":      "Legacy connection overrides\nEdit existing device-local SSH agent and compatibility exceptions without modifying inventory.",
		"raw":            "Raw settings\nOpen the legacy JSONC configuration in a text editor. Unsaved settings changes must be discarded first.",
	}
	for id, value := range values {
		details[id] += "\n\nCurrent: " + value
	}
	return values, details
}
