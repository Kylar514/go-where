package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	core "github.com/kylar514/go-where/internal/tunnelcore"
)

type bridgeTunnelSpec struct {
	Source, Destination         managedSSHJob
	Address, Bind               string
	SourcePort, DestinationPort int
}

func inventoryBridgeTunnel(h Host, c Config, r Connection) (core.Connection, error) {
	conn := core.Connection{Name: inventoryTunnelName(h), TunnelType: "go-where", Bind: r.Bind, RemotePort: r.RemotePort, Supervised: true}
	if r.SourceHost == nil || r.DestinationHost == nil {
		return conn, fmt.Errorf("bridge needs both host selections")
	}
	source, err := defaultTunnelHost(c, *r.SourceHost)
	if err != nil {
		return conn, err
	}
	destination, err := defaultTunnelHost(c, *r.DestinationHost)
	if err != nil {
		return conn, err
	}
	a, err := managedJob(source, c)
	if err != nil {
		return conn, err
	}
	b, err := managedJob(destination, c)
	if err != nil {
		return conn, err
	}
	ip := net.ParseIP(r.Bind)
	if ip == nil || !ip.IsLoopback() {
		return conn, fmt.Errorf("bridge listener must be loopback")
	}
	if net.ParseIP(r.RemoteHost) == nil && !validSourceAddress(r.RemoteHost) {
		return conn, fmt.Errorf("invalid source address")
	}
	spec := inventoryTunnelSpec{Bridge: &bridgeTunnelSpec{a, b, r.RemoteHost, r.Bind, r.RemotePort, r.LocalPort}}
	data, err := json.Marshal(spec)
	conn.Resource = base64.RawURLEncoding.EncodeToString(data)
	return conn, err
}

func validSourceAddress(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func runBridgeSupervisor(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("invalid bridge arguments")
	}
	data, err := base64.RawURLEncoding.DecodeString(args[0])
	if err != nil {
		return err
	}
	var spec inventoryTunnelSpec
	if err = json.Unmarshal(data, &spec); err != nil {
		return err
	}
	if spec.Bridge == nil {
		return fmt.Errorf("missing bridge specification")
	}
	b := spec.Bridge
	if b.SourcePort < 1 || b.SourcePort > 65535 || b.DestinationPort < 1 || b.DestinationPort > 65535 || net.ParseIP(b.Bind) == nil || !net.ParseIP(b.Bind).IsLoopback() || !validSourceAddress(b.Address) && net.ParseIP(b.Address) == nil {
		return fmt.Errorf("invalid bridge endpoints")
	}
	dir, err := os.MkdirTemp("", "go-where-bridge-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for name, text := range map[string]string{"source": b.Source.Configuration, "destination": b.Destination.Configuration} {
		if err = os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			return err
		}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	// ponytail: OS-selected intermediate port has a bind race; SSH fails closed,
	// retry with a new port if another process wins it.
	start := func(name string, job managedSSHJob, flag, value string) (*exec.Cmd, chan error, error) {
		cmd := exec.Command("ssh", "-F", filepath.Join(dir, name), "-N", "-o", "BatchMode=yes", "-o", "ExitOnForwardFailure=yes", flag, value, "--", job.Target)
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return nil, nil, err
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		return cmd, done, nil
	}
	a, adone, err := start("source", b.Source, "-L", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))+":"+net.JoinHostPort(b.Address, strconv.Itoa(b.SourcePort)))
	if err != nil {
		return err
	}
	defer a.Process.Kill()
	deadline := time.Now().Add(15 * time.Second)
	for {
		select {
		case err := <-adone:
			return fmt.Errorf("source tunnel exited: %v", err)
		default:
		}
		probe, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond)
		if e == nil {
			probe.Close()
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("source tunnel startup timed out")
		}
		time.Sleep(100 * time.Millisecond)
	}
	d, ddone, err := start("destination", b.Destination, "-R", net.JoinHostPort(b.Bind, strconv.Itoa(b.DestinationPort))+":"+net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer d.Process.Kill()
	select {
	case err := <-adone:
		return fmt.Errorf("source tunnel exited: %v", err)
	case err := <-ddone:
		return fmt.Errorf("destination tunnel exited: %v", err)
	}
}
