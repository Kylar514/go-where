package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var inventoryVariable = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func resolveValue(value string, vars map[string]string) (string, error) {
	var missing string
	value = inventoryVariable.ReplaceAllStringFunc(value, func(token string) string {
		name := token[2 : len(token)-1]
		v, ok := vars[name]
		if !ok {
			missing = name
		}
		return v
	})
	if missing != "" {
		return "", fmt.Errorf("missing local variable %s", missing)
	}
	if strings.Contains(value, "$") {
		return "", fmt.Errorf("use ${NAME} variables with literal, non-recursive values")
	}
	if clean(value) != value {
		return "", fmt.Errorf("control characters are not allowed")
	}
	return value, nil
}

func resolvedConnection(c Config, s InventorySource, r Connection) (Connection, error) {
	o := c.Overrides[s.ID][r.ID]
	if o.User != nil {
		r.User = *o.User
	}
	if o.IdentityFile != nil {
		r.IdentityFile = *o.IdentityFile
	}
	if o.Address != nil {
		r.Address = *o.Address
	}
	if o.Port != nil {
		r.Port = *o.Port
	}
	if o.JumpID != nil {
		r.JumpID = *o.JumpID
	}
	vars := map[string]string{}
	for k, v := range c.Variables {
		vars[k] = v
	}
	for k, v := range o.Variables {
		vars[k] = v
	}
	fields := []*string{&r.Address, &r.User, &r.IdentityFile}
	resolveDomain := r.kind() == "rdp"
	if r.kind() == "rdp" {
		a, err := connectionAccount(c, r)
		if err != nil {
			return r, err
		}
		fields = []*string{&r.Address}
		if a.User == "" {
			fields = append(fields, &r.User)
		}
		resolveDomain = a.Domain == nil
	}
	if r.kind() == "tunnel" {
		fields = []*string{&r.Bind}
		if r.TunnelType == "ssh" {
			fields = append(fields, &r.RemoteHost)
		} else {
			fields = append(fields, &r.Resource, &r.Namespace, &r.Context)
		}
	}
	if resolveDomain && r.Domain != nil {
		domain := *r.Domain
		r.Domain = &domain
		fields = append(fields, r.Domain)
	}
	for _, field := range fields {
		v, err := resolveValue(*field, vars)
		if err != nil {
			return r, fmt.Errorf("%s / %s: %w", s.Name, r.Alias, err)
		}
		*field = v
	}
	if r.kind() != "tunnel" && (r.Port < 1 || r.Port > 65535) {
		return r, fmt.Errorf("%s: invalid port", r.Alias)
	}
	if r.kind() != "tunnel" && !regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`).MatchString(r.Address) {
		return r, fmt.Errorf("%s: address must be an IP or DNS name", r.Alias)
	}
	if r.kind() == "ssh" && r.User != "" && (!regexp.MustCompile(`^[A-Za-z0-9_.@\\-]+$`).MatchString(r.User) || strings.HasPrefix(r.User, "-")) {
		return r, fmt.Errorf("%s: invalid SSH username", r.Alias)
	}
	if r.kind() != "tunnel" && r.Address == "" {
		return r, fmt.Errorf("%s: address required", r.Alias)
	}
	if r.kind() == "ssh" && r.IdentityFile != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return r, err
		}
		r.IdentityFile = expandHome(r.IdentityFile, home)
		if !filepath.IsAbs(r.IdentityFile) {
			return r, fmt.Errorf("%s: key path must be absolute or start with ~/", r.Alias)
		}
	}
	return r, nil
}

type managedSSHJob struct {
	Configuration string
	Target        string
}

// OpenSSH propagates -F to ProxyJump's SSH process. This snapshot gives each
// hop its own user/key, without reading the personal or system SSH configs.
func managedJob(h Host, c Config) (managedSSHJob, error) {
	s, err := findSource(c, h.SourceID)
	if err != nil {
		return managedSSHJob{}, err
	}
	records, err := sourceConnections(s)
	if err != nil {
		return managedSSHJob{}, err
	}
	byID := map[string]Connection{}
	for _, r := range records {
		byID[r.ID] = r
	}
	seen := map[string]bool{}
	var chain []Connection
	for id := h.ConnectionID; id != ""; {
		if seen[id] {
			return managedSSHJob{}, fmt.Errorf("%s: jump cycle", h.Alias)
		}
		seen[id] = true
		r, ok := byID[id]
		if !ok {
			return managedSSHJob{}, fmt.Errorf("%s: connection/jump %s no longer exists", h.Alias, id)
		}
		if r.kind() != "ssh" {
			return managedSSHJob{}, fmt.Errorf("%s: jump must reference an SSH connection", r.Alias)
		}
		r, err = resolvedConnection(c, s, r)
		if err != nil {
			return managedSSHJob{}, err
		}
		chain = append(chain, r)
		id = r.JumpID
	}
	if len(chain) == 0 {
		return managedSSHJob{}, fmt.Errorf("no managed connection selected")
	}
	var text strings.Builder
	quote := func(v string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"` }
	for i, r := range chain {
		fmt.Fprintf(&text, "Host gw-%d\n    HostName %s\n    Port %d\n", i, quote(r.Address), r.Port)
		if r.User != "" {
			fmt.Fprintf(&text, "    User %s\n", quote(r.User))
		}
		if r.IdentityFile != "" {
			fmt.Fprintf(&text, "    IdentityFile %s\n", quote(strings.ReplaceAll(r.IdentityFile, "%", "%%")))
		}
		if i+1 < len(chain) {
			fmt.Fprintf(&text, "    ProxyJump gw-%d\n", i+1)
		}
		text.WriteString("\n")
	}
	return managedSSHJob{Configuration: text.String(), Target: "gw-0"}, nil
}

func managedSSHCommand(h Host, c Config) ([]string, error) {
	j, err := managedJob(h, c)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return []string{exe, "__managed-ssh", base64.RawURLEncoding.EncodeToString(data)}, nil
}

func runManagedSSH(encoded string) error {
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	var j managedSSHJob
	if err = json.Unmarshal(data, &j); err != nil {
		return err
	}
	if j.Target != "gw-0" || j.Configuration == "" {
		return fmt.Errorf("invalid managed SSH job")
	}
	path, cleanup, err := sshSnapshot(j.Configuration)
	if err != nil {
		return err
	}
	defer cleanup()
	// Keep the snapshot until SSH (including its jump children) exits.
	// As in the transfer runner, let SSH receive terminal Ctrl+C while the
	// waiting parent survives to clean up its snapshot.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	return run([]string{"ssh", "-F", path, "--", j.Target})
}

func sshSnapshot(configuration string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "go-where-ssh-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "config")
	if err = os.WriteFile(path, []byte(configuration), 0600); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}

func selectedRecord(h Host, c Config) (InventorySource, Connection, error) {
	s, err := findSource(c, h.SourceID)
	if err != nil {
		return s, Connection{}, err
	}
	records, err := sourceConnections(s)
	if err != nil {
		return s, Connection{}, err
	}
	for _, r := range records {
		if r.ID == h.ConnectionID {
			r, err = resolvedConnection(c, s, r)
			return s, r, err
		}
	}
	return s, Connection{}, fmt.Errorf("connection no longer exists")
}

func managedDetails(h Host, c Config) string {
	metadata := "Source: " + h.SourceName + "\nConnection ID: " + h.ConnectionID + "\nTags: " + strings.Join(h.Tags, ", ") + "\nNotes: " + h.Comment + "\n\n"
	if h.Type == "rdp" {
		j, err := hostRDPJob(c, h, "")
		if err != nil {
			return metadata + err.Error()
		}
		return metadata + fmt.Sprintf("RDP\nAddress: %s:%d\nUser: %s\nDomain: %s\nTitle: %s\nSSH jump: %s\n", j.Host, j.Port, j.User, j.Domain, j.Title, j.Jump)
	}
	if h.Type == "tunnel" {
		_, r, err := selectedRecord(h, c)
		if err != nil {
			return metadata + err.Error()
		}
		conn, err := inventoryTunnel(h, c)
		if err != nil {
			return metadata + err.Error()
		}
		data, _ := base64.RawURLEncoding.DecodeString(conn.Resource)
		var spec inventoryTunnelSpec
		_ = json.Unmarshal(data, &spec)
		return metadata + fmt.Sprintf("%s tunnel\nLocal: %s:%d\nRemote: %s:%d\nResource: %s\nNamespace: %s\nContext: %s\n\n%s", r.TunnelType, r.Bind, r.LocalPort, r.RemoteHost, r.RemotePort, r.Resource, r.Namespace, r.Context, shellCommand(spec.Argv))
	}
	j, err := managedJob(h, c)
	if err != nil {
		return metadata + "Managed SSH: " + err.Error()
	}
	return metadata + "Resolved SSH settings (personal SSH config is not read):\n\n" + j.Configuration
}

func connectionPort(v string) (int, error) {
	p, err := strconv.Atoi(v)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535")
	}
	return p, nil
}
