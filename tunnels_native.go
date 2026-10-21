//go:build linux || windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/kylar514/diglet/config"
	"github.com/kylar514/diglet/tunnel"
	"os"
	"os/exec"
	"path/filepath"
)

func init() {
	// Reuse Diglet's PID-identity checks, detached processes and ownership registry.
	// ponytail: registry is port-based; identical ports on different loopback IPs
	// need an address-aware Diglet registry before they can run simultaneously.
	tunnel.Register("go-where", func(c config.Connection) (*exec.Cmd, error) {
		data, err := base64.RawURLEncoding.DecodeString(c.Resource)
		if err != nil {
			return nil, err
		}
		var spec inventoryTunnelSpec
		if err = json.Unmarshal(data, &spec); err != nil {
			return nil, err
		}
		if len(spec.Argv) == 0 || (spec.Argv[0] != "ssh" && spec.Argv[0] != "kubectl") {
			return nil, fmt.Errorf("invalid tunnel command")
		}
		args := spec.Argv[1:]
		if spec.Configuration != "" {
			path, err := tunnelSnapshotPath(c.Name)
			if err != nil {
				return nil, err
			}
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return nil, err
			}
			f, err := os.CreateTemp(filepath.Dir(path), ".snapshot-*")
			if err != nil {
				return nil, err
			}
			defer os.Remove(f.Name())
			_, writeErr := f.WriteString(spec.Configuration)
			closeErr := f.Close()
			if writeErr != nil {
				return nil, writeErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
			if err = os.Rename(f.Name(), path); err != nil {
				return nil, err
			}
			args = append([]string{"-F", path}, args...)
		}
		return exec.Command(spec.Argv[0], args...), nil
	})
}

func ensureInventoryTunnelStopped(h Host) error {
	statuses, err := tunnel.Statuses([]config.Connection{{Name: inventoryTunnelName(h)}})
	if err != nil {
		return err
	}
	if statuses[inventoryTunnelName(h)].PID != 0 {
		return fmt.Errorf("stop this tunnel before editing or deleting it")
	}
	return nil
}

func tunnelStatuses(connections []config.Connection) (map[string]tunnelInfo, error) {
	statuses, err := tunnel.Statuses(connections)
	result := map[string]tunnelInfo{}
	for name, info := range statuses {
		result[name] = tunnelInfo{int(info.Status), info.PID, info.Owner}
	}
	return result, err
}
func toggleManagedTunnel(c config.Connection, stop bool) error {
	if c.TunnelType == "go-where" {
		path, err := tunnelSnapshotPath(c.Name)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		// Serialize snapshot changes with start/stop across go-where instances.
		// ponytail: a crash can leave .lock; recover it manually once no toggle runs.
		lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("tunnel operation is locked (%s): %w", path+".lock", err)
		}
		lock.Close()
		defer os.Remove(path + ".lock")
	}
	if stop {
		err := tunnel.Stop(c)
		if err == nil && c.TunnelType == "go-where" {
			cleanupTunnelSnapshot(c.Name)
		}
		return err
	}
	pid, err := tunnel.Start(c)
	if err != nil {
		return err
	}
	// Diglet's active probe assumes 127.0.0.1. Inventory tunnels can bind
	// other loopback addresses; PID/listener status polling handles those.
	if c.TunnelType == "go-where" {
		return nil
	}
	if err = tunnel.StartProbe(c.Name, c.LocalPort, pid); err != nil {
		_ = tunnel.Stop(c)
		if c.TunnelType == "go-where" {
			cleanupTunnelSnapshot(c.Name)
		}
		return err
	}
	return nil
}
func runTunnelProbe(name string, port, pid int) { tunnel.RunProbe(name, port, pid) }
