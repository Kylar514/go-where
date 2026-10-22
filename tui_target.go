package main

import (
	"fmt"
	"os"
	"strings"

	digletconfig "github.com/kylar514/diglet/config"
)

func defaultTunnelPath() string { return digletconfig.DefaultConfigPath() }
func ensureTunnelConfig(path string) error {
	if home, err := os.UserHomeDir(); err == nil {
		path = expandHome(path, home)
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return digletconfig.Scaffold(path)
}
func chooseTarget(current Origin, dest string, rdp ...bool) (string, Origin, bool, error) {
	options := []choice{{"terminal", "New terminal"}, {"current", "Current terminal (one host)"}}
	if len(rdp) > 0 && rdp[0] {
		options = append([]choice{{"background", "Background — open RDP client without a terminal"}}, options...)
	}
	origins := map[string]Origin{}
	// Outside tmux there is no implicit "current session". Never guess.
	if rows, err := output("tmux", "list-panes", "-a", "-F", "#{session_id}\t#{window_id}\t#{pane_id}\t#{session_name}:#{window_index}.#{pane_index}"); err == nil {
		for _, line := range strings.Split(rows, "\n") {
			f := strings.Split(line, "\t")
			if len(f) != 4 {
				continue
			}
			key := f[0] + ":" + f[1] + "." + f[2]
			origins[key] = Origin{Pane: f[2], Window: f[1], Session: f[0], Client: current.Client}
			options = append(options, choice{key, "tmux pane → " + f[3]})
		}
	}
	id, back, err := chooseOne("Launch location", options, "Choose where this mode launches for the current session. Settings are not saved. Select a tmux pane explicitly when launching from a desktop or another terminal.", dest)
	if err != nil || back {
		return dest, current, true, err
	}
	if id == "terminal" || id == "current" || (id == "background" && len(rdp) > 0 && rdp[0]) {
		return id, current, false, nil
	}
	if o, ok := origins[id]; ok {
		return "tmux-pane", o, false, nil
	}
	return dest, current, true, fmt.Errorf("invalid tmux target")
}
