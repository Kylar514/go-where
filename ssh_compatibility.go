package main

import (
	"fmt"
	"regexp"
	"strings"
)

var kexName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._@-]*$`)

func validateExtraKex(names []string) error {
	return validateExtraAlgorithms("KEX", names)
}

func validateExtraAlgorithms(directive string, names []string) error {
	seen := map[string]bool{}
	for _, name := range names {
		if !kexName.MatchString(name) || seen[name] {
			return fmt.Errorf("invalid or duplicate additional %s name %q; enter bare algorithm names, not SSH directives or +/-/^ prefixes", directive, name)
		}
		seen[name] = true
	}
	return nil
}

func effectiveSSHCompatibility(c Config, sourceID string, r Connection) (*bool, []string) {
	only, kex := r.IdentitiesOnly, r.ExtraKexAlgorithms
	o := c.Overrides[sourceID][r.ID]
	if o.IdentitiesOnly != nil {
		only = o.IdentitiesOnly
	}
	if o.ExtraKexAlgorithms != nil {
		kex = *o.ExtraKexAlgorithms
	}
	return only, kex
}

func effectiveSignatureAlgorithms(c Config, sourceID string, r Connection) ([]string, []string) {
	host, pubkey := r.ExtraHostKeyAlgorithms, r.ExtraPubkeyAlgorithms
	o := c.Overrides[sourceID][r.ID]
	if o.ExtraHostKeyAlgorithms != nil {
		host = *o.ExtraHostKeyAlgorithms
	}
	if o.ExtraPubkeyAlgorithms != nil {
		pubkey = *o.ExtraPubkeyAlgorithms
	}
	return host, pubkey
}

func identitiesOnlyLabel(v *bool) string {
	if v == nil {
		return "default"
	}
	if *v {
		return "yes"
	}
	return "no"
}

func kexLabel(names []string) string {
	if len(names) == 0 {
		return "none (OpenSSH defaults)"
	}
	return strings.Join(names, ", ")
}

func compatibilityDetails(c Config, sourceID string, r Connection) string {
	only, kex := effectiveSSHCompatibility(c, sourceID, r)
	iOrigin, kOrigin := "connection default", "connection default"
	if r.IdentitiesOnly == nil {
		iOrigin = "system default"
	}
	if len(r.ExtraKexAlgorithms) == 0 {
		kOrigin = "system default"
	}
	o := c.Overrides[sourceID][r.ID]
	if o.IdentitiesOnly != nil {
		iOrigin = "local override"
	}
	if o.ExtraKexAlgorithms != nil {
		kOrigin = "local override"
	}
	details := fmt.Sprintf("Identities only: %s (%s)\nAdditional KEX: %s (%s)\n", identitiesOnlyLabel(only), iOrigin, kexLabel(kex), kOrigin)
	host, pubkey := effectiveSignatureAlgorithms(c, sourceID, r)
	origin := func(stored []string, override *[]string) string {
		if override != nil {
			return "local override"
		}
		if len(stored) == 0 {
			return "system default"
		}
		return "connection default"
	}
	return details + fmt.Sprintf("Additional host-key algorithms: %s (%s)\nAdditional public-key signature algorithms: %s (%s)\n", kexLabel(host), origin(r.ExtraHostKeyAlgorithms, o.ExtraHostKeyAlgorithms), kexLabel(pubkey), origin(r.ExtraPubkeyAlgorithms, o.ExtraPubkeyAlgorithms))
}

func chooseIdentitiesOnly(inherit bool, current *bool) (*bool, bool, error) {
	first, label := "default", "Default — OpenSSH behavior"
	if inherit {
		first, label = "inherit", "Inherit database setting"
	}
	selected := first
	if current != nil {
		selected = identitiesOnlyLabel(current)
	}
	id, back, err := chooseOne("Identities only", []choice{{first, label}, {"yes", "Yes — restrict offered keys to configured identities"}, {"no", "No — permit other available identities"}}, "", selected)
	if err != nil || back {
		return nil, back, err
	}
	if id == first {
		return nil, false, nil
	}
	v := id == "yes"
	return &v, false, nil
}

func editSSHCompatibility(r *Connection) error {
	defer menuSession()()
	draft := *r
	for {
		fields := []choice{
			{"identities", "Identities only: " + identitiesOnlyLabel(draft.IdentitiesOnly)},
			{"kex", "Additional key exchanges: " + kexLabel(draft.ExtraKexAlgorithms)},
			{"hostkey", "Additional host-key algorithms: " + kexLabel(draft.ExtraHostKeyAlgorithms)},
			{"pubkey", "Additional public-key signature algorithms: " + kexLabel(draft.ExtraPubkeyAlgorithms)},
		}
		m := newDialog("SSH compatibility", fields, "Apply updates the profile draft; save the profile afterward. Enable legacy algorithms only when required.")
		m.footerActions = []choice{{"select", "Edit field"}, {"save", "Apply"}}
		m.optionDetails = map[string]string{"identities": "Identities only\nRestricts the identities offered by SSH to configured keys. Default retains OpenSSH behavior.", "kex": "Key exchanges\nAlgorithms negotiating session encryption keys. Additions preserve OpenSSH defaults.", "hostkey": "Host-key algorithms\nSignatures verifying server identity. Host-key verification stays enabled.", "pubkey": "Public-key signatures\nAlgorithms authenticating your login. Enable legacy SHA-1 signatures only when required."}
		id, back, err := runChoice(m, menuKey(m.title))
		if err != nil || back {
			return err
		}
		switch id {
		case "identities":
			v, b, e := chooseIdentitiesOnly(false, draft.IdentitiesOnly)
			if e != nil {
				return e
			}
			if !b {
				draft.IdentitiesOnly = v
			}
		case "kex":
			v, b, e := chooseKex("Additional key exchanges", "Checked algorithms are added to OpenSSH's defaults (KexAlgorithms +…) for this connection only. Weak legacy exchanges are listed last; check only what the server requires. Leave everything unchecked for none.", draft.ExtraKexAlgorithms)
			if e != nil {
				return e
			}
			if !b {
				draft.ExtraKexAlgorithms = v
			}
		case "hostkey", "pubkey":
			directive, title, purpose := "HostKeyAlgorithms", "Additional host-key algorithms", "server identity verification"
			field := &draft.ExtraHostKeyAlgorithms
			if id == "pubkey" {
				directive, title, purpose = "PubkeyAcceptedAlgorithms", "Additional public-key signature algorithms", "user authentication"
				field = &draft.ExtraPubkeyAlgorithms
			}
			v, b, e := chooseAlgorithms(directive, title, "Adds to OpenSSH defaults for "+purpose+" on this connection only ("+directive+" +…). ssh-rsa uses legacy SHA-1 signatures; enable only if required. Checking nothing leaves defaults unchanged.", *field)
			if e != nil {
				return e
			}
			if !b {
				*field = v
			}
		case "save":
			*r = draft
			return nil
		}
	}
}

func editCompatibilityOverrideDraft(c *Config, h Host) error {
	defer menuSession()()
	s, r, err := storedRecord(h, *c)
	if err != nil {
		return err
	}
	if r.kind() != "ssh" {
		return fmt.Errorf("compatibility overrides belong to the referenced SSH record")
	}
	o := c.Overrides[s.ID][r.ID]
	for {
		iLabel := "inherit"
		if o.IdentitiesOnly != nil {
			iLabel = identitiesOnlyLabel(o.IdentitiesOnly)
		}
		kLabel := "inherit"
		if o.ExtraKexAlgorithms != nil {
			kLabel = kexLabel(*o.ExtraKexAlgorithms)
		}
		overrideLabel := func(v *[]string) string {
			if v == nil {
				return "inherit"
			}
			return kexLabel(*v)
		}
		id, back, e := choose("Local SSH compatibility — "+r.Alias, []choice{
			{"identities", "Identities only: " + iLabel}, {"kex", "Additional KEX: " + kLabel},
			{"hostkey", "Additional host-key algorithms: " + overrideLabel(o.ExtraHostKeyAlgorithms)},
			{"pubkey", "Additional public-key signature algorithms: " + overrideLabel(o.ExtraPubkeyAlgorithms)},
			{"save", "Apply compatibility override to settings draft"},
		}, "Currently applied:\n"+compatibilityDetails(*c, s.ID, r)+"\nChanges affect local settings only. Inherit follows the database; an explicit empty list disables its additions. OpenSSH global defaults and host-key verification remain unchanged.")
		if e != nil || back {
			return e
		}
		switch id {
		case "identities":
			v, b, e := chooseIdentitiesOnly(true, o.IdentitiesOnly)
			if e != nil {
				return e
			}
			if !b {
				o.IdentitiesOnly = v
			}
		case "kex":
			mode, b, e := choose("Additional KEX override", []choice{{"inherit", "Inherit database additions"}, {"custom", "Choose local additions… (none checked disables database additions)"}}, "")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if mode == "inherit" {
				o.ExtraKexAlgorithms = nil
				continue
			}
			// Start from what applies now, so unchecking one entry is one click.
			start := r.ExtraKexAlgorithms
			if o.ExtraKexAlgorithms != nil {
				start = *o.ExtraKexAlgorithms
			}
			v, b, e := chooseKex("Local additional key exchanges", "Replaces the database's additions for you only. Checking nothing is an explicit local override that disables them. Choose \"Inherit\" in the previous menu to follow the database again.", start)
			if e != nil {
				return e
			}
			if !b {
				o.ExtraKexAlgorithms = &v
			}
		case "hostkey", "pubkey":
			directive, title := "HostKeyAlgorithms", "Additional host-key algorithms"
			stored, field := r.ExtraHostKeyAlgorithms, &o.ExtraHostKeyAlgorithms
			if id == "pubkey" {
				directive, title = "PubkeyAcceptedAlgorithms", "Additional public-key signature algorithms"
				stored, field = r.ExtraPubkeyAlgorithms, &o.ExtraPubkeyAlgorithms
			}
			mode, b, e := choose(title+" override", []choice{{"inherit", "Inherit database additions"}, {"custom", "Choose local additions… (none checked disables database additions)"}}, "")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			if mode == "inherit" {
				*field = nil
				continue
			}
			if *field != nil {
				stored = **field
			}
			v, b, e := chooseAlgorithms(directive, "Local "+title, "Replaces database additions for you only. ssh-rsa uses weak SHA-1 signatures; enable only if required. Checking nothing disables database additions. Inherit follows the database again.", stored)
			if e != nil {
				return e
			}
			if !b {
				*field = &v
			}
		case "save":
			if c.Overrides == nil {
				c.Overrides = map[string]map[string]ConnectionOverride{}
			}
			if c.Overrides[s.ID] == nil {
				c.Overrides[s.ID] = map[string]ConnectionOverride{}
			}
			c.Overrides[s.ID][r.ID] = o
			return nil
		}
	}
}
