package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

func main() {
	if err := app(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ssh-pick:", err)
		os.Exit(1)
	}
}

func app(args []string) error {
	if len(args) > 0 && args[0] == "__preview" {
		return preview(args[1:])
	}
	settings, err := configPath()
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("ssh-pick", flag.ContinueOnError)
	sshConfig := fs.String("ssh-config", filepath.Join(home, ".ssh", "config"), "SSH config to browse and connect with")
	config := fs.String("config", settings, "picker settings (JSON)")
	dest := fs.String("destination", "", "override default: tmux-pane, tmux-window, tmux-session, terminal, current")
	noPopup := fs.Bool("no-popup", false, "run inline instead of opening a tmux popup")
	client := fs.String("client", "", "invoking tmux client (for bindings or shared panes)")
	context := fs.String("origin", "", "internal: captured tmux origin")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: ssh-pick [options]\n       ssh-pick config [options]\n\nSearch SSH hosts with fzf. Enter connects using your configured destination.")
		fs.PrintDefaults()
	}
	editSettings := len(args) > 0 && args[0] == "config"
	if editSettings {
		args = args[1:]
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
	if o.Pane != "" && !*noPopup {
		argv := []string{exe, "--no-popup", "--origin", encodeOrigin(o), "--config", *config}
		if customSSHConfig {
			argv = append(argv, "--ssh-config", *sshConfig)
		}
		if *dest != "" {
			argv = append(argv, "--destination", *dest)
		}
		return run([]string{"tmux", "display-popup", "-EE", "-c", o.Client, "-w", "90%", "-h", "85%", shellCommand(argv)})
	}
	query := ""
	for {
		c, err = loadConfig(*config)
		if err != nil {
			return err
		}
		if *dest != "" {
			c.Destination = *dest
		}
		hosts, err := readHosts(*sshConfig)
		if err != nil {
			return err
		}
		if len(hosts) == 0 {
			return fmt.Errorf("no explicit host aliases found in %s", *sshConfig)
		}
		var key string
		var selected []Host
		key, query, selected, err = pick(hosts, exe, query, destination(c.Destination, o, c), o)
		if err != nil || key == "cancel" {
			return err
		}
		switch key {
		case "ctrl-r":
			continue
		case "alt-c":
			if err := ensureConfig(*config); err != nil {
				return err
			}
			if err := editFile(*config, 0, c.EditDestination, false, o, c); err != nil {
				return err
			}
			if destination(c.EditDestination, o, c) != "current" {
				return nil
			}
			continue
		case "ctrl-e", "alt-e":
			if len(selected) == 0 {
				continue
			}
			h := selected[0]
			if err := editFile(h.File, h.Line, c.EditDestination, key == "alt-e", o, c); err != nil {
				return err
			}
			if key == "alt-e" || destination(c.EditDestination, o, c) != "current" {
				return nil
			}
			continue
		}
		if len(selected) == 0 {
			continue
		}
		target := destination(c.Destination, o, c)
		switch key {
		case "alt-p":
			target = "tmux-pane"
		case "alt-w":
			target = "tmux-window"
		case "alt-s":
			target = "tmux-session"
		case "alt-t":
			target = "terminal"
		}
		connectionConfig := ""
		if customSSHConfig {
			connectionConfig = *sshConfig
		}
		return connect(selected, target, o, c, connectionConfig)
	}
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}

func pick(hosts []Host, exe, query, dest string, o Origin) (key, nextQuery string, selected []Host, err error) {
	f, err := os.CreateTemp("", "ssh-pick-*.json")
	if err != nil {
		return "", query, nil, err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(hosts)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", query, nil, err
	}
	var input strings.Builder
	for i, h := range hosts {
		fmt.Fprintf(&input, "%d\t%-28s  %-30s  %s\n", i, clean(h.Alias), clean(h.Hostname), clean(h.Comment))
	}
	header := "Enter: " + dest
	if o.Pane != "" {
		header += " | origin " + o.Session + ":" + o.Window + "." + o.Pane
	}
	header += "\n^J/K move · ^U/D half-page · Tab mark · Alt+P/W/S/T panes/windows/session/terminals\n^E edit host · Alt+E floating editor · Alt+C settings · ^R reload"
	args := []string{
		"--multi", "--layout=default", "--header-first", "--border", "--delimiter=\t", "--with-nth=2..",
		"--prompt=SSH > ", "--header=" + header, "--query=" + query, "--print-query",
		"--expect=enter,alt-p,alt-w,alt-s,alt-t,alt-c,ctrl-r",
		// Editing always uses the highlighted row, not the multiselection.
		"--bind=ctrl-j:down,ctrl-k:up,tab:toggle+down,shift-tab:toggle+up,ctrl-u:half-page-up,ctrl-d:half-page-down,ctrl-e:become(printf '%s\\n' {q} ctrl-e {}),alt-e:become(printf '%s\\n' {q} alt-e {})",
		"--preview=" + shellCommand([]string{exe, "__preview", f.Name()}) + " {1}",
		// ponytail: cap preview at 10 rows to preserve list space; dynamic sizing if needed.
		"--preview-window=up:10:wrap",
	}
	// become returns only the highlighted row for editing, even with marks.
	// printf receives fzf's shell-quoted placeholders, not executable host data.
	c := exec.Command("fzf", args...)
	c.Env = withoutFZFOptions(os.Environ())
	c.Stdin, c.Stderr = strings.NewReader(input.String()), os.Stderr
	out, runErr := c.Output()
	if runErr != nil {
		var exit *exec.ExitError
		if errors.As(runErr, &exit) {
			if exit.ExitCode() == 130 || (exit.ExitCode() == 1 && len(out) == 0) {
				return "cancel", query, nil, nil
			}
			if exit.ExitCode() != 1 {
				return "", query, nil, fmt.Errorf("fzf: %w", runErr)
			}
		} else {
			return "", query, nil, fmt.Errorf("fzf: %w", runErr)
		}
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) < 2 {
		return "", query, nil, fmt.Errorf("unexpected fzf output")
	}
	nextQuery, key = lines[0], lines[1]
	for _, row := range lines[2:] {
		id, _, _ := strings.Cut(row, "\t")
		i, parseErr := strconv.Atoi(id)
		if parseErr != nil || i < 0 || i >= len(hosts) {
			return "", query, nil, fmt.Errorf("invalid picker selection")
		}
		selected = append(selected, hosts[i])
	}
	return key, nextQuery, selected, nil
}

func withoutFZFOptions(env []string) []string {
	var result []string
	for _, v := range env {
		if !strings.HasPrefix(v, "FZF_DEFAULT_OPTS=") && !strings.HasPrefix(v, "FZF_DEFAULT_OPTS_FILE=") && !strings.HasPrefix(v, "FZF_DEFAULT_COMMAND=") {
			result = append(result, v)
		}
	}
	return result
}

func preview(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("invalid preview arguments")
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	var hosts []Host
	if err := json.Unmarshal(b, &hosts); err != nil {
		return err
	}
	i, err := strconv.Atoi(args[1])
	if err != nil || i < 0 || i >= len(hosts) {
		return fmt.Errorf("invalid preview selection")
	}
	h := hosts[i]
	fmt.Printf("%s\n%s:%d\n\n%s\n\n", clean(h.Alias), clean(h.File), h.Line, clean(h.Comment))
	for n, line := range strings.Split(h.Block, "\n") {
		fmt.Printf("%5d  %s\n", h.Line+n, clean(line))
	}
	return nil
}
