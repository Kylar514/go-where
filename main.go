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
		// 45 fzf rows plus the popup's two border rows.
		return run([]string{"tmux", "display-popup", "-EE", "-c", o.Client, "-w", "90%", "-h", "47", shellCommand(argv)})
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
		case "ctrl-s", "ctrl-alt-s":
			if key == "ctrl-alt-s" && o.Pane == "" {
				return fmt.Errorf("editing in a pane requires tmux; use Ctrl+S for a floating editor")
			}
			if err := ensureConfig(*config); err != nil {
				return err
			}
			return editFile(*config, 0, "tmux-pane", key == "ctrl-s", o, c)
		case "ctrl-e", "ctrl-alt-e":
			if len(selected) == 0 {
				continue
			}
			if key == "ctrl-alt-e" && o.Pane == "" {
				return fmt.Errorf("editing in a pane requires tmux; use Ctrl+E for a floating editor")
			}
			h := selected[0]
			return editFile(h.File, h.Line, "tmux-pane", key == "ctrl-e", o, c)
		}
		if len(selected) == 0 {
			continue
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
	input := hostRows(hosts)
	header := "Enter: " + dest
	if o.Pane != "" {
		header += " | origin " + o.Session + ":" + o.Window + "." + o.Pane
	}
	header += "\n^J/K move · ^U/D half-page · Tab mark · Ctrl+Alt+P/W/N/T panes/windows/session/terminals\nCtrl+S/E floating settings/host · Ctrl+Alt+S/E pane settings/host · ^R reload"
	args := []string{
		// 25 results + 10 preview rows + 10 rows of borders, labels and controls.
		"--height=45",
		"--multi", "--layout=default", "--header-first", "--header-lines=1", "--border", "--delimiter=\t", "--with-nth=2..",
		"--prompt=SSH > ", "--header=" + header, "--query=" + query, "--print-query",
		"--expect=enter,ctrl-alt-p,ctrl-alt-w,ctrl-alt-n,ctrl-alt-t,ctrl-s,ctrl-alt-s,ctrl-r",
		// Editing always uses the highlighted row, not the multiselection.
		"--bind=ctrl-j:down,ctrl-k:up,tab:toggle+down,shift-tab:toggle+up,ctrl-u:half-page-up,ctrl-d:half-page-down,ctrl-e:become(printf '%s\\n' {q} ctrl-e {}),ctrl-alt-e:become(printf '%s\\n' {q} ctrl-alt-e {})",
		"--preview=" + shellCommand([]string{exe, "__preview", f.Name()}) + " {1}",
		// ponytail: cap preview at 10 rows to preserve list space; dynamic sizing if needed.
		"--preview-window=up:10:wrap",
	}
	// become returns only the highlighted row for editing, even with marks.
	// printf receives fzf's shell-quoted placeholders, not executable host data.
	c := exec.Command("fzf", args...)
	c.Env = withoutFZFOptions(os.Environ())
	c.Stdin, c.Stderr = strings.NewReader(input), os.Stderr
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

func hostRows(hosts []Host) string {
	rows := [][4]string{{"HOST", "HOSTNAME / IP", "JUMPHOST", "COMMENT"}}
	var widths [3]int
	for _, h := range hosts {
		jump := h.JumpHost
		if jump == "" || strings.EqualFold(jump, "none") {
			jump = "—"
		}
		rows = append(rows, [4]string{clean(h.Alias), clean(h.Hostname), clean(jump), clean(h.Comment)})
	}
	for _, row := range rows {
		for col := range widths {
			widths[col] = max(widths[col], len([]rune(row[col])))
		}
	}
	var input strings.Builder
	for i, row := range rows {
		// The first row is a non-selectable fzf header; IDs still index hosts.
		fmt.Fprintf(&input, "%d\t%-*s  %-*s  %-*s  %s\n", i-1, widths[0], row[0], widths[1], row[1], widths[2], row[2], row[3])
	}
	return input.String()
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
