package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	digletconfig "github.com/kylar514/go-where/internal/tunnelcore"
)

type inventoryTunnelSpec struct {
	Configuration string
	Argv          []string
}

func inventoryTunnelName(h Host) string { return "gw-" + h.SourceID + "-" + h.ConnectionID }

func inventoryTunnel(h Host, c Config) (digletconfig.Connection, error) {
	_, r, err := selectedRecord(h, c)
	conn := digletconfig.Connection{Name: inventoryTunnelName(h), TunnelType: "go-where", LocalPort: r.LocalPort, RemotePort: r.RemotePort}
	if err != nil {
		return conn, err
	}
	if r.kind() != "tunnel" {
		return conn, fmt.Errorf("not a tunnel")
	}
	if r.Bind == "" {
		r.Bind = "127.0.0.1"
	}
	conn.Bind = r.Bind
	ip := net.ParseIP(r.Bind)
	if ip == nil || !ip.IsLoopback() {
		return conn, fmt.Errorf("%s: local bind must be a loopback IP", r.Alias)
	}
	if r.LocalPort < 1 || r.LocalPort > 65535 || r.RemotePort < 1 || r.RemotePort > 65535 {
		return conn, fmt.Errorf("invalid tunnel ports")
	}
	spec := inventoryTunnelSpec{}
	switch r.TunnelType {
	case "ssh":
		if !regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`).MatchString(r.RemoteHost) || strings.HasPrefix(r.RemoteHost, "-") {
			return conn, fmt.Errorf("invalid remote host")
		}
		jump := h
		jump.ConnectionID = r.SSHID
		j, e := managedJob(jump, c)
		if e != nil {
			return conn, e
		}
		spec.Configuration = j.Configuration
		target := net.JoinHostPort(r.RemoteHost, strconv.Itoa(r.RemotePort))
		spec.Argv = []string{"ssh", "-N", "-o", "ExitOnForwardFailure=yes", "-L", net.JoinHostPort(r.Bind, strconv.Itoa(r.LocalPort)) + ":" + target, "--", j.Target}
	case "kubectl":
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`).MatchString(r.Resource) {
			return conn, fmt.Errorf("invalid Kubernetes resource")
		}
		for _, v := range []string{r.Namespace, r.Context} {
			if strings.HasPrefix(v, "-") || strings.ContainsAny(v, "\r\n\x00") {
				return conn, fmt.Errorf("invalid Kubernetes namespace/context")
			}
		}
		spec.Argv = []string{"kubectl", "port-forward", "--address", r.Bind}
		if r.Namespace != "" {
			spec.Argv = append(spec.Argv, "--namespace", r.Namespace)
		}
		if r.Context != "" {
			spec.Argv = append(spec.Argv, "--context", r.Context)
		}
		spec.Argv = append(spec.Argv, r.Resource, fmt.Sprintf("%d:%d", r.LocalPort, r.RemotePort))
	default:
		return conn, fmt.Errorf("unknown tunnel backend")
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return conn, err
	}
	conn.Resource = base64.RawURLEncoding.EncodeToString(data)
	return conn, nil
}

func tunnelSnapshotPath(name string) (string, error) {
	if !inventoryName(name) {
		return "", fmt.Errorf("invalid managed tunnel name")
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "go-where", "managed-tunnels", name+".ssh"), nil
}

func cleanupTunnelSnapshot(name string) {
	if p, err := tunnelSnapshotPath(name); err == nil {
		_ = os.Remove(p)
	}
}
