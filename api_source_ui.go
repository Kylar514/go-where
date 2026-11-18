package main

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
)

func addAPISource(path string, state *PickerState) error {
	raw, back, err := inventoryPrompt("API HTTPS URL", "https://")
	if err != nil || back {
		return err
	}
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if err = validateAPIURL(raw); err != nil {
		return err
	}
	m := newPathModel("API token (stored in OS credential store)", "", false)
	m.input.EchoMode = textinput.EchoPassword
	if _, err = runTUI(m); err != nil || m.back {
		return err
	}
	token := strings.TrimSpace(m.input.Value())
	m.input.SetValue("")
	_, decodeErr := hex.DecodeString(token)
	if len(token) != 64 || decodeErr != nil {
		return fmt.Errorf("API token must contain 64 hexadecimal characters")
	}
	credential := inventoryID()
	if err = apiTokenSet(apiCredentialService, credential, token); err != nil {
		return fmt.Errorf("store API credential: %w", err)
	}
	s := InventorySource{URL: raw, CredentialID: credential, Type: "api"}
	var sources []InventorySource
	if err = apiRequest(s, "GET", "inventories", nil, &sources, ""); err != nil {
		_ = apiTokenDelete(apiCredentialService, credential)
		return err
	}
	var options []choice
	for _, source := range sources {
		label := source.Name
		if source.ReadOnly {
			label += " (read-only)"
		}
		options = append(options, choice{source.ID, label})
	}
	id, b, e := chooseOne("Remote inventories", options, "Choose an authorized inventory to register. No passwords, identities, or keys are fetched.", "")
	if e != nil || b {
		_ = apiTokenDelete(apiCredentialService, credential)
		return e
	}
	for _, source := range sources {
		if source.ID == id {
			source.URL, source.CredentialID = raw, credential
			if err = registerSource(path, source); err != nil {
				_ = apiTokenDelete(apiCredentialService, credential)
				return err
			}
			state.Source = source.ID
			return nil
		}
	}
	_ = apiTokenDelete(apiCredentialService, credential)
	return fmt.Errorf("no authorized inventory selected")
}

func manageAPITokens(c Config) error {
	defer menuSession()()
	var options []choice
	for _, s := range c.Sources {
		if s.Type == "api" {
			options = append(options, choice{s.ID, s.Name + " — " + s.URL})
		}
	}
	id, back, err := chooseOne("API server tokens", options, "Manage only your own device tokens. Inventory permissions are assigned by the administrator.", "")
	if err != nil || back {
		return err
	}
	s, err := findSource(c, id)
	if err != nil {
		return err
	}
	for {
		var tokens []serverToken
		if err = apiRequest(s, "GET", "tokens", nil, &tokens, ""); err != nil {
			return err
		}
		options = []choice{{"create", "Create device token…"}, {"rotate", "Rotate this device token…"}}
		for _, t := range tokens {
			options = append(options, choice{t.ID, fmt.Sprintf("%s — expires %s", t.Name, time.Unix(t.Expires, 0).Format("2006-01-02"))})
		}
		id, b, e := choose("Device tokens", options, "Token secrets are never listed. Creating a token displays it once. Revocation is permanent.")
		if e != nil || b {
			return e
		}
		if id == "create" || id == "rotate" {
			name, b, e := inventoryPrompt("Device name", "work-laptop")
			if e != nil || b {
				return e
			}
			_, b, e = choose("Create token?", []choice{{"confirm", "Create token"}}, "Creates an authentication credential with your current user permissions, expiring in 90 days.")
			if e != nil {
				return e
			}
			if b {
				continue
			}
			var replacement serverToken
			if e = apiRequest(s, "POST", "tokens", map[string]any{"name": name, "days": 90}, &replacement, ""); e != nil {
				return e
			}
			if id == "create" {
				if e = notice("Copy this token now; it cannot be retrieved again:\n" + replacement.Secret); e != nil {
					return e
				}
				continue
			}
			if e = rotateAPIToken(s, replacement); e != nil {
				return e
			}
			continue
		}
		_, b, e = choose("Revoke token?", []choice{{"confirm", "Revoke permanently"}}, "Revoking the current token disconnects this device. Other users' tokens cannot be revoked.")
		if e != nil {
			return e
		}
		if !b {
			if e = apiRequest(s, "DELETE", "tokens/"+id, nil, nil, ""); e != nil {
				return e
			}
		}
	}
}
