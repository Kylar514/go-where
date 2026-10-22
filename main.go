package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

func main() {
	if err := app(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "go-where:", err)
		os.Exit(1)
	}
}

func app(args []string) error {
	if len(args) > 0 && args[0] == "probe" {
		if len(args) != 4 {
			return fmt.Errorf("invalid tunnel probe arguments")
		}
		port, e1 := strconv.Atoi(args[2])
		pid, e2 := strconv.Atoi(args[3])
		if e1 != nil || e2 != nil || port < 1 || port > 65535 || pid < 1 {
			return fmt.Errorf("invalid tunnel probe arguments")
		}
		runTunnelProbe(args[1], port, pid)
		return nil
	}
	if len(args) > 0 && strings.HasPrefix(args[0], "__") {
		return transferInternal(args)
	}
	if len(args) > 0 && args[0] == "rdm-report" {
		return rdmReportCommand(args[1:])
	}
	settings, err := configPath()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("go-where", flag.ContinueOnError)
	sshConfig := fs.String("ssh-config", filepath.Join(home, ".ssh", "config"), "SSH config to browse and connect with")
	config := fs.String("config", settings, "settings (JSON with comments / JSONC)")
	dest := fs.String("destination", "", "override default: tmux-pane, tmux-window, tmux-session, terminal, current")
	noPopup := fs.Bool("no-popup", false, "run inline instead of opening a tmux popup")
	popup := fs.Bool("popup", false, "open inside a tmux popup (default: standalone TUI)")
	tunnelsConfig := fs.String("tunnels-config", "", "Diglet connections.yaml (default: existing Diglet user config)")
	inPopup := fs.Bool("in-popup", false, "internal: running inside a tmux popup that closes after a launch")
	client := fs.String("client", "", "invoking tmux client (for bindings or shared panes)")
	context := fs.String("origin", "", "internal: captured tmux origin")
	legacy := fs.Bool("legacy", false, "use the legacy SCP protocol (scp mode only)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: go-where [ssh|scp|rsync|rdp|tunnels] [options]\n       go-where scp --legacy [options]\n       go-where config [options]\n       go-where rdm-report EXPORT.json [--ssh-config path]\n\nCtrl+O changes mode. Transfer modes always review before execution.")
		fs.PrintDefaults()
	}
	editSettings := len(args) > 0 && args[0] == "config"
	if editSettings {
		args = args[1:]
	}
	mode := "ssh"
	explicitMode := false
	if !editSettings && len(args) > 0 && validMode(args[0]) {
		explicitMode = true
		mode, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *legacy {
		if mode != "scp" || editSettings {
			return fmt.Errorf("--legacy requires scp mode")
		}
		mode = "legacy-scp"
	}
	customSSHConfig := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "ssh-config" {
			customSSHConfig = true
		}
	})
	*sshConfig, err = filepath.Abs(expandHome(*sshConfig, home))
	if err != nil {
		return err
	}
	*config, err = filepath.Abs(expandHome(*config, home))
	if err != nil {
		return err
	}
	c, err := loadConfig(*config)
	if err != nil {
		if !editSettings {
			return err
		}
		// A broken settings file must remain editable using safe defaults.
		c = defaultConfig()
	}
	if !explicitMode && c.StartupMode != "" {
		mode = c.StartupMode
	}
	if *dest != "" {
		c.Destination = *dest
	}
	if err := c.validate(); err != nil {
		return err
	}
	var o Origin
	if *context != "" {
		o, err = decodeOrigin(*context)
	} else {
		o, err = captureOrigin(*client)
	}
	if err != nil {
		return err
	}
	if editSettings {
		if err := ensureConfig(*config); err != nil {
			return err
		}
		return editFile(*config, 0, c.EditDestination, false, o, c)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if o.Pane != "" && *popup && !*noPopup {
		argv := []string{exe, mode, "--no-popup", "--in-popup", "--origin", encodeOrigin(o), "--config", *config}
		if customSSHConfig {
			argv = append(argv, "--ssh-config", *sshConfig)
		}
		if *dest != "" {
			argv = append(argv, "--destination", *dest)
		}
		if *tunnelsConfig != "" {
			argv = append(argv, "--tunnels-config", *tunnelsConfig)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		// Fit the popup to the TUI's content, not a screen-filling 90%.
		// The TUI inside derives its layout from the popup size, so the
		// height here equals the list rows plus the fixed chrome.
		w, h := "90%", "90%"
		sizeOK := false
		if cw, ch, ok := tmuxClientSize(o.Client); ok {
			rows := max(1, min(20, ch-12))
			// Reserve two cells for go-where's frame inside the borderless popup.
			w, h = strconv.Itoa(min(cw-2, 150)), strconv.Itoa(rows+10)
			sizeOK = true
		}
		if os.Getenv("GO_WHERE_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "go-where: popup size client=%q ok=%t w=%s h=%s\n", o.Client, sizeOK, w, h)
		}
		return run([]string{"tmux", "display-popup", "-EE", "-B", "-s", "fg=default,bg=default", "-c", o.Client, "-d", cwd, "-w", w, "-h", h, shellCommand(argv)})
	}
	previousFrame := popupFrameEnabled
	popupFrameEnabled = *inPopup
	defer func() { popupFrameEnabled = previousFrame }()
	query := ""
	state := PickerState{Mode: mode}
	draft := Transfer{Direction: "upload"}
	sessionDest := ""
	sessionRDPDest := ""
	for {
		c, err = loadConfig(*config)
		if err != nil {
			return err
		}
		menuHotkeys = c
		if *dest != "" {
			c.Destination = *dest
		}
		if sessionDest != "" {
			c.Destination = sessionDest
		}
		if sessionRDPDest != "" {
			c.RDP.Destination = sessionRDPDest
		}
		if *tunnelsConfig != "" {
			c.TunnelsConfig = *tunnelsConfig
		}
		hosts, err := readHosts(*sshConfig)
		if err != nil && !(os.IsNotExist(err) && !customSSHConfig) {
			return err
		}
		managed, err := inventoryHosts(c)
		if err != nil {
			return err
		}
		hosts = append(hosts, managed...)
		state.RDPTags = c.RDP.Tags
		state.ConfigPath = *config
		state.RDPDestination = rdpDestination(c, false)
		state.TunnelsConfig = c.TunnelsConfig
		pickDest := destination(c.Destination, o, c)
		var key string
		var selected []Host
		key, query, selected, err = pick(hosts, exe, query, pickDest, o, &state)
		if err != nil || key == "cancel" {
			return err
		}
		switch key {
		case "settings", "ctrl-s":
			raw, e := settingsMenu(*config)
			if e != nil {
				if e = notice(e.Error()); e != nil {
					return e
				}
				continue
			}
			if raw {
				if err = ensureConfig(*config); err != nil {
					return err
				}
				return editFile(*config, 0, "tmux-pane", true, o, c)
			}
			continue
		case "sources":
			if err = sourceMenu(*config, c, &state); err != nil {
				if err = notice(err.Error()); err != nil {
					return err
				}
			}
			continue
		case "new-connection":
			previousFocus := state.Focus
			if state.Mode == "tunnels" {
				err = editConnection(c, &state, nil)
			} else {
				err = editInventoryHost(c, &state, nil)
			}
			if err != nil {
				if err = notice(err.Error()); err != nil {
					return err
				}
			}
			if state.Focus != previousFocus {
				query = ""
			}
			continue
		case "target":
			current := c.Destination
			if state.Mode == "rdp" {
				current = rdpDestination(c, false)
			}
			chosen, nextOrigin, back, targetErr := chooseTarget(o, current, state.Mode == "rdp")
			err = targetErr
			if err != nil {
				if showErr := notice(err.Error()); showErr != nil {
					return showErr
				}
			}
			if !back && err == nil {
				if state.Mode == "rdp" {
					sessionRDPDest = chosen
				} else {
					sessionDest = chosen
				}
				o = nextOrigin
			}
			continue
		case "tunnel-edit":
			path := c.TunnelsConfig
			if path == "" {
				path = defaultTunnelPath()
			}
			path = expandHome(path, home)
			if err := ensureTunnelConfig(path); err != nil {
				return err
			}
			return editFile(path, 0, c.EditDestination, false, o, c)
		case "ctrl-r":
			continue
		case "ctrl-alt-s":
			if key == "ctrl-alt-s" && o.Pane == "" {
				return fmt.Errorf("editing in a pane requires tmux; use Ctrl+S for a floating editor")
			}
			if err := ensureConfig(*config); err != nil {
				return err
			}
			return editFile(*config, 0, "tmux-pane", false, o, c)
		case "ctrl-e", "ctrl-alt-e":
			if len(selected) == 0 {
				continue
			}
			h := selected[0]
			if h.SourceID != "" {
				if h.Type == "tunnel" {
					err = editConnection(c, &state, &h)
				} else {
					err = editInventoryHost(c, &state, &h)
				}
				if err != nil {
					if err = notice(err.Error()); err != nil {
						return err
					}
				}
				continue
			}
			if key == "ctrl-alt-e" && o.Pane == "" {
				return fmt.Errorf("editing in a pane requires tmux; use Ctrl+E for a floating editor")
			}
			return editFile(h.File, h.Line, "tmux-pane", key == "ctrl-e", o, c)
		}
		if len(selected) == 0 {
			continue
		}
		if selected[0].Type == "host" {
			if err = notice("Configure an SSH or RDP profile first with Ctrl+E."); err != nil {
				return err
			}
			continue
		}
		if key == "connect-as" {
			var back bool
			selected, back, err = chooseHostProfiles(c, selected)
			if err != nil {
				if err = notice(err.Error()); err != nil {
					return err
				}
				continue
			}
			if back {
				continue
			}
		}
		target := destination(c.Destination, o, c)
		switch key {
		case "ctrl-alt-p":
			target = "tmux-pane"
		case "ctrl-alt-w":
			target = "tmux-window"
		case "ctrl-alt-n":
			target = "tmux-session"
		case "ctrl-alt-t":
			target = "terminal"
		}
		connectionConfig := ""
		if customSSHConfig {
			connectionConfig = *sshConfig
		}
		if state.Mode == "rdp" {
			c.activeRDPAccount = state.Account
			c.RDP = c.RDP.withAccount(state.Account)
			err = launchRDP(selected, o, c, connectionConfig, exe, state.Debug)
		} else if state.Mode != "ssh" {
			draft.Mode = state.Mode
			var launched bool
			launched, err = transferWizard(&draft, selected, target, o, c, connectionConfig, exe)
			// Without a launch (backed out of the wizard) the popup must remain open.
			if *inPopup && launched {
				return nil
			}
		} else {
			err = connect(selected, target, o, c, connectionConfig)
		}
		if err != nil {
			if showErr := notice(err.Error()); showErr != nil {
				return errors.Join(err, showErr)
			}
			continue
		}
		// Inside a tmux popup, an explicit launch closes the popup. Failed
		// launches and non-launch actions (target, edit, reload) keep it open.
		if *inPopup {
			return nil
		}
	}
}

func tmuxClientSize(client string) (int, int, bool) {
	// list-clients is the only tmux query that reliably reports sizes for a
	// specific client from a spawned process; display-message's client
	// context proved unreliable (probed on real tmux). Match by name.
	out, err := output("tmux", "list-clients", "-F", "#{client_name}\t#{client_width}\t#{client_height}")
	if err != nil {
		return 0, 0, false
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || (client != "" && fields[0] != client) {
			continue
		}
		w, err1 := strconv.Atoi(fields[1])
		h, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil || w < 40 || h < 26 {
			continue
		}
		return w, h, true
	}
	return 0, 0, false
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
