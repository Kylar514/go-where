//go:build linux || windows

package main

import (
	"github.com/kylar514/diglet/config"
	"github.com/kylar514/diglet/tunnel"
)

func tunnelStatuses(connections []config.Connection) (map[string]tunnelInfo, error) {
	statuses, err := tunnel.Statuses(connections)
	result := map[string]tunnelInfo{}
	for name, info := range statuses {
		result[name] = tunnelInfo{int(info.Status), info.PID, info.Owner}
	}
	return result, err
}
func toggleManagedTunnel(c config.Connection, stop bool) error {
	if stop {
		return tunnel.Stop(c)
	}
	pid, err := tunnel.Start(c)
	if err != nil {
		return err
	}
	if err = tunnel.StartProbe(c.Name, c.LocalPort, pid); err != nil {
		_ = tunnel.Stop(c)
		return err
	}
	return nil
}
func runTunnelProbe(name string, port, pid int) { tunnel.RunProbe(name, port, pid) }
