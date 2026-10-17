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
		fmt.Fprintln(os.Stderr, "go-where:", err)
		os.Exit(1)
	}
}

func app(args []string) error {
	if len(args) > 0 && args[0] == "__preview" {
		return preview(args[1:])
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
	config := fs.String("config", settings, "picker settings (JSON)")
	dest := fs.String("destination", "", "override default: tmux-pane, tmux-window, tmux-session, terminal, current")
	noPopup := fs.Bool("no-popup", false, "run inline instead of opening a tmux popup")
	client := fs.String("client", "", "invoking tmux client (for bindings or shared panes)")
	context := fs.String("origin", "", "internal: captured tmux origin")
	legacy := fs.Bool("legacy", false, "use the legacy SCP protocol (scp mode only)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "Usage: go-where [ssh|scp|rsync|rdp] [options]\n       go-where scp --legacy [options]\n       go-where config [options]\n       go-where rdm-report EXPORT.json [--ssh-config path]\n\nCtrl+O changes mode. Transfer modes always review before execution.")
		fs.PrintDefaults()
	}
	editSettings := len(args) > 0 && args[0] == "config"
	if editSettings {
		args = args[1:]
	}
	mode := "ssh"
	if !editSettings && len(args) > 0 && validMode(args[0]) {
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
		argv := []string{exe, mode, "--no-popup", "--origin", encodeOrigin(o), "--config", *config}
		if customSSHConfig {
			argv = append(argv, "--ssh-config", *sshConfig)
		}
		if *dest != "" {
			argv = append(argv, "--destination", *dest)
		}
		// 45 fzf rows plus the popup's two border rows.
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return run([]string{"tmux", "display-popup", "-EE", "-c", o.Client, "-d", cwd, "-w", "90%", "-h", "47", shellCommand(argv)})
	}
	query := ""
	state := PickerState{Mode: mode}
	draft := Transfer{Direction: "upload"}
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
		state.RDPTags = c.RDP.Tags
		state.RDPDestination = rdpDestination(c, false)
		pickDest := destination(c.Destination, o, c)
		var key string
		var selected []Host
		key, query, selected, err = pick(hosts, exe, query, pickDest, o, &state)
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
		if state.Mode == "rdp" {
			return launchRDP(selected, o, c, connectionConfig, exe, state.Debug)
		}
		if state.Mode != "ssh" {
			draft.Mode = state.Mode
			launched, err := transferWizard(&draft, selected, target, o, c, connectionConfig, exe)
			if err != nil || launched {
				return err
			}
			continue
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

func pick(hosts []Host, exe, query, dest string, o Origin, state *PickerState) (key, nextQuery string, selected []Host, err error) {
	if state == nil {
		state = &PickerState{Mode: "ssh"}
	}
	modeFile, err := os.CreateTemp("", "go-where-mode-*")
	if err != nil {
		return "", query, nil, err
	}
	defer os.Remove(modeFile.Name())
	_, err = modeFile.WriteString(state.Mode)
	closeErr := modeFile.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return "", query, nil, err
	}
	listedFile, err := os.CreateTemp("", "go-where-listed-*")
	if err != nil {
		return "", query, nil, err
	}
	defer os.Remove(listedFile.Name())
	_, err = listedFile.WriteString(listKind(state.Mode))
	closeErr = listedFile.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return "", query, nil, err
	}
	f, err := os.CreateTemp("", "go-where-*.json")
	if err != nil {
		return "", query, nil, err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(hosts)
	closeErr = f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", query, nil, err
	}
	input := rowsForMode(hosts, state.Mode, state.RDPTags)
	header := pickerHeader(state.Mode, dest, state.RDPDestination)
	expect := []string{"enter", "ctrl-alt-p", "ctrl-alt-w", "ctrl-alt-n", "ctrl-alt-t", "ctrl-s", "ctrl-alt-s", "ctrl-r"}
	args := []string{
		// 25 results + 10 preview rows + 10 rows of borders, labels and controls.
		"--height=45",
		"--multi", "--sync", "--layout=default", "--header-first", "--header-lines=1", "--border", "--delimiter=\t", "--with-nth=2..",
		"--prompt=" + strings.ToUpper(state.Mode) + " > ", "--header=" + header, "--query=" + query, "--print-query",
		"--expect=" + strings.Join(expect, ","),
		// Editing always uses the highlighted row, not the multiselection.
		"--bind=ctrl-j:down,ctrl-k:up,tab:toggle-up,shift-tab:toggle-down,ctrl-u:half-page-up,ctrl-d:half-page-down,ctrl-e:become(printf '%s\\n' {q} ctrl-e {}),ctrl-alt-e:become(printf '%s\\n' {q} ctrl-alt-e {})",
		// Ctrl+O picks a mode; the list reloads only when it crosses RDP mode's filtered view.
		"--id-nth=1",
		"--bind=ctrl-o:execute(" + shellCommand([]string{exe, "__mode", modeFile.Name()}) + ")+transform-prompt(" + shellCommand([]string{exe, "__mode-prompt", modeFile.Name()}) + ")+transform-header(" + shellCommand([]string{exe, "__picker-header", modeFile.Name(), dest, state.RDPDestination}) + ")+transform(" + shellCommand([]string{exe, "__mode-reload", modeFile.Name(), listedFile.Name(), f.Name(), strings.Join(state.RDPTags, ",")}) + ")+refresh-preview",
		"--bind=?:execute(" + shellCommand([]string{exe, "__help", modeFile.Name()}) + ")",
		"--preview=" + shellCommand([]string{exe, "__preview", f.Name(), modeFile.Name()}) + " {1}",
		// ponytail: cap preview at 10 rows to preserve list space; dynamic sizing if needed.
		"--preview-window=up:10:wrap",
	}
	if state.Focus != "" || len(state.Marked) > 0 {
		restore, err := restorePicker(input, hosts, query, state)
		if err != nil {
			return "", query, nil, err
		}
		if restore != "" {
			args = append(args, "--bind=result:"+restore+"+unbind(result)")
		}
	}
	// become returns only the highlighted row for editing, even with marks.
	// printf receives fzf's shell-quoted placeholders, not executable host data.
	c := exec.Command("fzf", args...)
	c.Env = withoutFZFOptions(os.Environ())
	c.Stdin, c.Stderr = strings.NewReader(input), os.Stderr
	out, runErr := c.Output()
	state.Mode, err = readMode(modeFile.Name())
	if err != nil {
		return "", query, nil, err
	}
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
	// In RDP mode Ctrl+Alt+T reuses the terminal key from the other modes as the
	// debug trigger: a visible terminal instead of the usual detached launch.
	state.Debug = key == "ctrl-alt-t" && state.Mode == "rdp"
	rows := lines[2:]
	state.Marked = nil
	for _, row := range rows {
		id, _, _ := strings.Cut(row, "\t")
		i, parseErr := strconv.Atoi(id)
		if i == emptyRowID {
			continue
		}
		if parseErr != nil || i < 0 || i >= len(hosts) {
			return "", query, nil, fmt.Errorf("invalid picker selection")
		}
		selected = append(selected, hosts[i])
		state.Marked = append(state.Marked, hosts[i].Alias)
	}
	if len(state.Marked) > 0 {
		state.Focus = state.Marked[0]
	}
	return key, nextQuery, selected, nil
}

func hostRows(hosts []Host) string { return hostRowsFor(hosts, nil) }

// emptyRowID marks a non-selectable placeholder, shown when a filter hides every host.
const emptyRowID = -2

func hostRowsFor(hosts []Host, include func(Host) bool) string {
	rows := [][4]string{{"HOST", "HOSTNAME / IP", "JUMPHOST", "COMMENT"}}
	ids := []int{-1}
	var widths [3]int
	for i, h := range hosts {
		if include != nil && !include(h) {
			continue
		}
		jump := h.JumpHost
		if jump == "" || strings.EqualFold(jump, "none") {
			jump = "—"
		}
		rows = append(rows, [4]string{clean(h.Alias), clean(h.Hostname), clean(jump), clean(h.Comment)})
		ids = append(ids, i)
	}
	for _, row := range rows {
		for col := range widths {
			widths[col] = max(widths[col], len([]rune(row[col])))
		}
	}
	var input strings.Builder
	for i, row := range rows {
		// The first row is a non-selectable fzf header; IDs still index hosts.
		fmt.Fprintf(&input, "%d\t%-*s  %-*s  %-*s  %s\n", ids[i], widths[0], row[0], widths[1], row[1], widths[2], row[2], row[3])
	}
	if include != nil && len(rows) == 1 {
		fmt.Fprintf(&input, "%d\t(no hosts are tagged; add an indented '# Windows' or '# RDP' comment inside a Host block)\n", emptyRowID)
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
	if len(args) != 3 {
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
	i, err := strconv.Atoi(args[2])
	if i == emptyRowID {
		return nil
	}
	if err != nil || i < 0 || i >= len(hosts) {
		return fmt.Errorf("invalid preview selection")
	}
	h := hosts[i]
	mode, err := readMode(args[1])
	if err != nil {
		return err
	}
	if warning := modeWarning(mode); warning != "" {
		fmt.Printf("%s\n\n", warning)
	}
	fmt.Printf("%s\n%s:%d\n\n%s\n\n", clean(h.Alias), clean(h.File), h.Line, clean(h.Comment))
	for n, line := range strings.Split(h.Block, "\n") {
		fmt.Printf("%5d  %s\n", h.Line+n, clean(line))
	}
	return nil
}
