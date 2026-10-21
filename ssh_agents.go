package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type SSHAgentProfile struct {
	SocketEnv string `json:"socket_env,omitempty"`
	Socket    string `json:"socket,omitempty"`
}

func validAgentChoice(name string) bool {
	if name == "" || name == "default" || name == "none" {
		return true
	}
	return inventoryName(name) && !strings.EqualFold(name, "default") && !strings.EqualFold(name, "none") && !strings.EqualFold(name, "inherit")
}

func validateAgentProfile(name string, p SSHAgentProfile) error {
	if !inventoryName(name) || strings.EqualFold(name, "default") || strings.EqualFold(name, "none") || strings.EqualFold(name, "inherit") {
		return fmt.Errorf("SSH agent name %q is invalid or reserved", name)
	}
	if (p.SocketEnv == "") == (p.Socket == "") {
		return fmt.Errorf("SSH agent %s needs exactly one of socket_env or socket", name)
	}
	if p.SocketEnv != "" && !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(p.SocketEnv) {
		return fmt.Errorf("SSH agent %s: invalid environment variable name", name)
	}
	if !safeValue(p.Socket) || strings.Contains(p.Socket, "$") {
		return fmt.Errorf("SSH agent %s: use a literal endpoint or socket_env, not shell expressions", name)
	}
	return nil
}

func effectiveAgent(c Config, sourceID string, r Connection) (name, origin string) {
	name, origin = r.SSHAgent, "connection default"
	if o := c.Overrides[sourceID][r.ID]; o.SSHAgent != nil {
		name, origin = *o.SSHAgent, "local override"
	}
	if name == "" {
		name = "default"
		if origin != "local override" {
			origin = "system default"
		}
	}
	return name, origin
}

func resolveSSHAgent(c Config, sourceID string, r Connection) (endpoint, description string, err error) {
	name, origin := effectiveAgent(c, sourceID, r)
	description = agentChoiceLabel(name) + " (" + origin + ")"
	switch name {
	case "default":
		return "", description, nil
	case "none":
		return "none", description, nil
	}
	p, ok := c.SSHAgents[name]
	if !ok {
		return "", description, fmt.Errorf("SSH agent %q is not configured locally", name)
	}
	if err = validateAgentProfile(name, p); err != nil {
		return "", description, err
	}
	endpoint = p.Socket
	if p.SocketEnv != "" {
		endpoint = os.Getenv(p.SocketEnv)
		if endpoint == "" {
			return "", description, fmt.Errorf("SSH agent %s: environment variable %s is unset or empty", name, p.SocketEnv)
		}
	}
	if !safeValue(endpoint) || strings.Contains(endpoint, "$") {
		return "", description, fmt.Errorf("SSH agent %s: endpoint must be literal text without control characters", name)
	}
	home, e := os.UserHomeDir()
	if e != nil {
		return "", description, e
	}
	endpoint = expandHome(endpoint, home)
	if !filepath.IsAbs(endpoint) {
		return "", description, fmt.Errorf("SSH agent %s: endpoint must be an absolute socket/pipe path", name)
	}
	if runtime.GOOS != "windows" {
		info, e := os.Stat(endpoint)
		if e != nil {
			return "", description, fmt.Errorf("SSH agent %s: %w", name, e)
		}
		if info.Mode()&os.ModeSocket == 0 {
			return "", description, fmt.Errorf("SSH agent %s: %s is not a Unix socket", name, endpoint)
		}
	}
	return endpoint, description, nil
}

func agentChoiceLabel(name string) string {
	switch name {
	case "", "default":
		return "System default"
	case "none":
		return "Disabled"
	default:
		return "Profile: " + name
	}
}
