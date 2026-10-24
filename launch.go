package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Origin struct {
	Pane    string
	Window  string
	Session string
	Client  string
}

func captureOrigin(client string) (Origin, error) {
	pane := os.Getenv("TMUX_PANE")
	if pane == "" || os.Getenv("TMUX") == "" {
		return Origin{}, nil
	}
	out, err := exec.Command("tmux", "list-clients", "-F",
		"#{pane_id}\t#{window_id}\t#{session_id}\t#{client_name}").Output()
	if err != nil {
		return Origin{}, fmt.Errorf("capture tmux origin: %w", err)
	}
	var matches []Origin
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, "\t")
		if len(f) == 4 && f[0] == pane && f[2] != "" && f[3] != "" && (client == "" || f[3] == client) {
			matches = append(matches, Origin{f[0], f[1], f[2], f[3]})
		}
	}
	if len(matches) != 1 {
		return Origin{}, fmt.Errorf("cannot uniquely identify invoking tmux client (%d matches); use --client NAME from the launch binding", len(matches))
	}
	return matches[0], nil
}

func encodeOrigin(o Origin) string {
	b, _ := json.Marshal(o)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeOrigin(s string) (Origin, error) {
	var o Origin
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &o)
	}
	return o, err
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func shellCommand(argv []string) string {
	parts := make([]string, len(argv))
	for i, s := range argv {
		parts[i] = shellQuote(s)
	}
	return strings.Join(parts, " ")
}

func run(argv []string) error {
	c := exec.Command(argv[0], argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func output(argv ...string) (string, error) {
	c := exec.Command(argv[0], argv[1:]...)
	c.Stderr = os.Stderr
	b, err := c.Output()
	return strings.TrimSpace(string(b)), err
}

func destination(dest string, o Origin, c Config) string {
	if strings.HasPrefix(dest, "tmux-") && o.Pane == "" {
		return c.Fallback
	}
	return dest
}

func paneTarget(o Origin) string {
	// Session qualification prevents a linked window selecting another session.
	return o.Session + ":" + o.Window + "." + o.Pane
}

func launchCommand(dest string, argv []string, o Origin, c Config, floating bool) error {
	if floating {
		return spawnTerminal(c, argv, true)
	}
	switch dest {
	case "current":
		return run(argv)
	case "terminal":
		return spawnTerminal(c, argv, false)
	case "tmux-pane":
		return run([]string{"tmux", "split-window", "-h", "-t", paneTarget(o), shellCommand(argv)})
	case "tmux-pane-below":
		return run([]string{"tmux", "split-window", "-v", "-t", paneTarget(o), shellCommand(argv)})
	case "tmux-window":
		return run([]string{"tmux", "new-window", "-t", o.Session + ":", shellCommand(argv)})
	default:
		return fmt.Errorf("cannot launch a single command with destination %q", dest)
	}
}

func connect(hosts []Host, dest string, o Origin, c Config, sshConfig string) error {
	if len(hosts) == 0 {
		return nil
	}
	if strings.HasPrefix(dest, "tmux-") && o.Pane == "" {
		return fmt.Errorf("%s requires tmux; use Enter for your configured fallback or Ctrl+Alt+T", dest)
	}
	commands := map[string][]string{}
	for _, h := range hosts {
		argv := []string{"ssh", "--", h.Alias}
		if h.SourceID != "" {
			var err error
			argv, err = managedSSHCommand(h, c)
			if err != nil {
				return err
			}
		} else if sshConfig != "" {
			argv = []string{"ssh", "-F", sshConfig, "--", h.Alias}
		}
		commands[hostIdentity(h)] = argv
		if c.debugLaunch && h.SourceID == "" {
			argv = append([]string{"ssh", "-vv", "-o", "ConnectTimeout=15"}, argv[1:]...)
			job := transferJob{Label: "Debug SSH", Argv: argv}
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			commands[hostIdentity(h)] = []string{exe, "__debug", encodeJob(job)}
		}
	}
	ssh := func(h Host) []string { return commands[hostIdentity(h)] }
	if dest == "current" && len(hosts) > 1 {
		return fmt.Errorf("current destination accepts one host; choose panes, windows or terminals for multiple hosts")
	}
	if dest == "tmux-session" {
		name := "ssh-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		id, err := output("tmux", "new-session", "-d", "-P", "-F", "#{session_id}", "-s", name, shellCommand(ssh(hosts[0])))
		if err != nil {
			return fmt.Errorf("create session: %w", err)
		}
		for _, h := range hosts[1:] {
			if err := run([]string{"tmux", "new-window", "-d", "-t", id + ":", shellCommand(ssh(h))}); err != nil {
				return fmt.Errorf("session %s created; opening %s failed: %w", name, h.Alias, err)
			}
		}
		if o.Client != "" {
			return run([]string{"tmux", "switch-client", "-c", o.Client, "-t", id})
		}
		return nil
	}
	for i, h := range hosts {
		if err := launchCommand(dest, ssh(h), o, c, false); err != nil {
			return fmt.Errorf("opened %d/%d hosts; %s failed: %w (no automatic rerouting)", i, len(hosts), h.Alias, err)
		}
	}
	return nil
}

func editFile(file string, line int, dest string, floating bool, o Origin, c Config) error {
	argv := append([]string{}, c.Editor...)
	if line > 0 {
		argv = append(argv, "+"+strconv.Itoa(line))
	}
	argv = append(argv, "--", file)
	dest = destination(dest, o, c)
	if dest == "tmux-session" && !floating {
		return fmt.Errorf("editing supports tmux-pane, tmux-window, terminal or current, not tmux-session")
	}
	return launchCommand(dest, argv, o, c, floating)
}
