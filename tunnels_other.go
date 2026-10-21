//go:build !linux && !windows

package main

import (
	"fmt"
	"github.com/kylar514/diglet/config"
)

func tunnelStatuses([]config.Connection) (map[string]tunnelInfo, error) {
	return nil, fmt.Errorf("managed tunnel monitoring is currently supported on Linux and Windows; macOS support is pending")
}
func toggleManagedTunnel(config.Connection, bool) error { _, err := tunnelStatuses(nil); return err }
func runTunnelProbe(string, int, int)                   {}

func ensureInventoryTunnelStopped(Host) error { return nil }
