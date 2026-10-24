package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func defaultTunnelPath() string { return "" }
func ensureTunnelConfig(path string) error {
	if home, err := os.UserHomeDir(); err == nil {
		path = expandHome(path, home)
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return fmt.Errorf("legacy tunnel files are no longer used; create a database tunnel")
}
func chooseTarget(current Origin, dest string, rdp ...bool) (string, Origin, bool, error) {
	defer menuSession()()
	options := []choice{{"terminal", "New terminal"}, {"current", "Current terminal (one host)"}}
	if len(rdp) > 0 && rdp[0] {
		options = append([]choice{{"background", "Background — open RDP client without a terminal"}}, options...)
	}
	origins := map[string]Origin{}
	// Outside tmux there is no implicit "current session". Never guess.
	anchors := []choice{}
	anchorDetails := map[string]string{}
	anchorRows := map[string][]string{}
	if rows, err := output("tmux", "list-panes", "-a", "-F", "#{session_id}\t#{window_id}\t#{pane_id}\t#{session_name}\t#{window_name}\t#{pane_current_command}\t#{pane_current_path}\t#{pane_title}\t#{window_index}"); err == nil {
		for _, line := range strings.Split(rows, "\n") {
			f := strings.Split(line, "\t")
			if len(f) != 9 {
				continue
			}
			key := f[0] + ":" + f[1] + "." + f[2]
			origins[key] = Origin{Pane: f[2], Window: f[1], Session: f[0], Client: current.Client}
			window := f[8] + ":" + f[4]
			label := f[3] + " " + window + " " + f[5] + " " + filepath.Base(f[6])
			anchorRows[key] = []string{f[3], window, f[5], filepath.Base(f[6])}
			anchors = append(anchors, choice{key, label})
			path := f[6]
			if home, e := os.UserHomeDir(); e == nil {
				if path == home {
					path = "~"
				} else if strings.HasPrefix(path, home+string(filepath.Separator)) {
					path = "~" + strings.TrimPrefix(path, home)
				}
			}
			anchorDetails[key] = "Anchor\n  Session: " + f[3] + "\n  Window: " + window + "\n  Command: " + f[5] + "\n\nLocation\n  Directory: " + path + "\n  Title: " + f[7]
		}
	}
	if len(anchors) > 0 {
		options = append(options, choice{"tmux-pane", "New pane beside…"}, choice{"tmux-pane-below", "New pane below…"}, choice{"tmux-window", "New tmux window…"}, choice{"tmux-session", "New tmux session…"})
	}
	m := newDialog("Launch location", options, "Placement applies to this app session, not inventory.")
	m.singleChoice = true
	m.current = dest
	m.footerActions = []choice{{"select", "Select"}}
	m.screenID = "launch-location"
	for i, o := range m.options {
		if o.ID == dest {
			m.cursor = i
			break
		}
	}
	m.optionDetails = map[string]string{"terminal": "Open the connection client in a new terminal window.", "current": "Run in the terminal hosting go-where. Only one host can use this location.", "background": "Launch the graphical RDP client without opening a terminal.", "tmux-pane": "Create a pane to the right of an anchor pane. Its existing process remains untouched.", "tmux-pane-below": "Create a pane below an anchor pane. Its existing process remains untouched.", "tmux-window": "Create a new window in the selected anchor's tmux session.", "tmux-session": "Create a new tmux session for the connection client."}
	id, back, err := runChoice(m, m.screenID)
	if err != nil || back {
		return dest, current, true, err
	}
	if id == "terminal" || id == "current" || (id == "background" && len(rdp) > 0 && rdp[0]) {
		return id, current, false, nil
	}
	if strings.HasPrefix(id, "tmux-") {
		if id == "tmux-session" && current.Pane != "" {
			return id, current, false, nil
		}
		m := newDialog("Placement anchor", anchors, "")
		m.singleChoice = true
		m.footerActions = []choice{{"select", "Select"}}
		m.optionDetails = anchorDetails
		m.columnRows = anchorRows
		placement := map[string]string{"tmux-pane": "New pane to the right of this pane.", "tmux-pane-below": "New pane below this pane.", "tmux-window": "New window in this session.", "tmux-session": "New tmux session."}[id]
		for key, text := range m.optionDetails {
			m.optionDetails[key] = text + "\n\nPlacement\n  " + placement + "\n  Existing process remains untouched."
		}
		m.screenID = "placement-anchor"
		for key, o := range origins {
			if o.Pane == current.Pane {
				m.current = key
			}
		}
		key, b, e := runChoice(m, m.screenID)
		if e != nil || b {
			return dest, current, true, e
		}
		if o, ok := origins[key]; ok {
			return id, o, false, nil
		}
	}
	return dest, current, true, fmt.Errorf("invalid tmux target")
}
