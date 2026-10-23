package main

import (
	"fmt"
	"strings"
)

func hostEditorDetails(options []choice, count int, writable bool) map[string]string {
	details := map[string]string{
		"tunnels":     "Tunnels\nManage tunnels where a selected host is the SSH transport or saved remote target. Start and stop operate locally; tunnel definitions save separately from this host draft. Read-only definitions cannot be edited.",
		"name":        "Name\nDisplay name used to identify the host. Changing it does not change the connection address.",
		"address":     "Address\nIP address or DNS name used to connect to this host.",
		"tags":        "Tags\nComma-separated labels for organizing, filtering and selecting groups of hosts.",
		"notes":       "Notes\nDescriptive information about the host. Notes do not affect connection setup.",
		"color":       "Color\nVisual identifier shown in host lists and Details. Color has no effect on connectivity.",
		"rdp-session": "RDP session\nChange display, clipboard and shared folders for all RDP sessions on this device—not just the selected hosts. Global session settings save separately; your host draft stays unchanged.",
	}
	for _, kind := range []string{"ssh", "rdp"} {
		label := strings.ToUpper(kind)
		details[kind+"-profile"] = label + " profile\nSelect reusable connection settings from enabled sources or Local. Host default follows the inventory assignment; an explicit choice stays pinned to that profile. Selection does not edit the profile definition.\n\nSave my selections stores this choice only on this device."
		details[kind+"-identities"] = label + " identities\nChoose the logins offered when connecting and one default login among them. Existing identity settings remain in effect until you make a selection.\n\nSave my selections stores these choices only on this device; shared inventory is unchanged."
	}
	for _, o := range options {
		if strings.HasPrefix(o.ID, "ssh-") || strings.HasPrefix(o.ID, "rdp-") {
			if count > 1 && o.ID != "rdp-session" {
				details[o.ID] += fmt.Sprintf("\n\nChanging this field applies to all %d selected hosts.", count)
			}
		} else if o.ID != "tunnels" {
			details[o.ID] += "\n\nSave host writes this field to inventory."
			if !writable {
				details[o.ID] += " This field cannot be edited because a selected source is read-only."
			} else if count > 1 {
				details[o.ID] += fmt.Sprintf(" Changing it applies to all %d selected hosts.", count)
			}
		}
		_, value, _ := strings.Cut(o.Label, ": ")
		details[o.ID] += "\n\nCurrent: " + value
	}
	return details
}
