package main

import "fmt"

// An explicit one-launch choice wins over the record's saved profile.
func connectionAccount(c Config, r Connection) (RDPAccount, error) {
	if c.activeRDPAccount.User != "" {
		return c.activeRDPAccount, nil
	}
	if r.Account == "" {
		return RDPAccount{}, nil
	}
	for _, a := range c.RDP.Accounts {
		if a.Name == r.Account {
			return a, nil
		}
	}
	return RDPAccount{}, fmt.Errorf("%s: local RDP account %q is not configured", r.Alias, r.Account)
}

func hostRDPJob(c Config, h Host, sshConfig string) (rdpJob, error) {
	if h.SourceID == "" {
		return newRDPJob(c.RDP, h, sshConfig)
	}
	_, r, err := selectedRecord(h, c)
	if err != nil {
		return rdpJob{}, err
	}
	if r.kind() != "rdp" {
		return rdpJob{}, fmt.Errorf("%s is not an RDP connection", r.Alias)
	}
	settings := c.RDP
	account, err := connectionAccount(c, r)
	if err != nil {
		return rdpJob{}, err
	}
	// Custom uses only the record's login and the client's password prompt.
	// A saved profile supplies its own credentials, never the global secret.
	settings.PasswordCommand = nil
	if r.User != "" {
		settings.User = r.User
	}
	settings.Port = r.Port
	if r.Domain != nil {
		settings.DefaultDomain = *r.Domain
		settings.DomainRules = nil
	}
	settings = settings.withAccount(account)
	if account.Password != nil && !c.debugLaunch {
		settings.credentialPath = c.settingsPath
		settings.credentialIdentity = account.ID
	}
	h.Hostname = r.Address
	h.JumpHost = ""
	var snapshot string
	var target string
	if r.JumpID != "" {
		jump := h
		jump.ConnectionID = r.JumpID
		j, e := managedJob(jump, c)
		if e != nil {
			return rdpJob{}, e
		}
		snapshot = j.Configuration
		target = j.Target
		_, ref, e := selectedRecord(jump, c)
		if e != nil {
			return rdpJob{}, e
		}
		h.JumpHost = ref.Alias
	}
	j, err := newRDPJob(settings, h, "")
	j.SSHSnapshot = snapshot
	if target != "" {
		j.Jump = target
	}
	return j, err
}
