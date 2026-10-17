package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type PickerState struct {
	ConfigPath     string
	Account        RDPAccount
	Mode           string
	Focus          string
	Marked         []string
	RDPTags        []string
	Debug          bool
	RDPDestination string
}

func pickerHeader(mode, dest, rdpDest string) string {
	if mode == "rdp" {
		dest = rdpDest
		if dest == "" {
			dest = "background"
		}
	}
	return "Enter → " + strings.ReplaceAll(dest, "-", " ") + "    │    Ctrl+O → mode    │    ? → help\n" + strings.Repeat("─", 64)
}

func pickerHelp(mode string) string {
	launch := "Enter          Launch using the default destination\nCtrl+Alt+P     New tmux pane\nCtrl+Alt+W     New tmux window\nCtrl+Alt+N     New tmux session\nCtrl+Alt+T     New terminal window"
	if mode == "rdp" {
		launch = "Enter          Launch using rdp.destination (background by default)\nAlt+A          Choose an account and launch selected hosts\nCtrl+Alt+T     Launch in a visible terminal for debugging"
	}
	return "LAUNCH\n" + launch + `

NAVIGATE & SELECT
↑/↓, Ctrl+J/K  Move through hosts
Ctrl+U/D       Move half a page
Tab            Mark and move up
Shift+Tab      Mark and move down
Shift+↑/↓      Scroll preview

EDIT
Ctrl+S         Settings in a floating terminal
Ctrl+E         Highlighted host config in a floating terminal
Ctrl+Alt+S     Settings in a tmux pane
Ctrl+Alt+E     Highlighted host config in a tmux pane

OTHER
Ctrl+O         Choose mode
Ctrl+R         Reload hosts and settings
Esc / Ctrl+C   Cancel picker
?              Open help (reserved; not a search character)
`
}

func validMode(s string) bool {
	return s == "ssh" || s == "scp" || s == "legacy-scp" || s == "rsync" || s == "rdp"
}

func splitTags(csv string) []string {
	if csv == "" {
		return nil
	}
	return strings.Split(csv, ",")
}

func listKind(mode string) string {
	if mode == "rdp" {
		return "rdp"
	}
	return "all"
}

// IDs stay the host's index in the full list, so selections never depend on filtering.
func rowsForMode(hosts []Host, mode string, tags []string) string {
	if mode != "rdp" {
		return hostRows(hosts)
	}
	return hostRowsFor(hosts, func(h Host) bool { return h.isRDP(tags) })
}

func readHostsFile(path string) ([]Host, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var hosts []Host
	return hosts, json.Unmarshal(b, &hosts)
}

// Only RDP mode shows a different host list, so only crossing it reloads fzf.
func modeReload(w io.Writer, modeFile, listedFile, hostsFile, tags string) error {
	mode, err := readMode(modeFile)
	if err != nil {
		return err
	}
	listed, _ := os.ReadFile(listedFile)
	if string(listed) == listKind(mode) {
		return nil
	}
	if err := os.WriteFile(listedFile, []byte(listKind(mode)), 0600); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(w, "reload("+shellCommand([]string{exe, "__rows", hostsFile, modeFile, tags})+")")
	return err
}

func modeWarning(mode string) string {
	switch mode {
	case "scp":
		return "SCP: requires remote SFTP. Recursive copy; existing files may be overwritten."
	case "legacy-scp":
		return "LEGACY SCP (-O): remote scp + POSIX shell required. Windows shells unsupported."
	case "rsync":
		return "RSYNC: rsync 3.1+ required on both ends. Native Windows OpenSSH alone is insufficient. No --delete."
	case "rdp":
		return "RDP: FreeRDP through this host's ProxyJump (SSH tunnel). Domain/user come from your rdp settings; the client prompts for the password."
	}
	return ""
}

// Restore by filtered rank, not source-file index: fuzzy search reorders hosts.
func restorePicker(rows string, hosts []Host, query string, state *PickerState) (string, error) {
	_, input, _ := strings.Cut(rows, "\n")
	c := exec.Command("fzf", "--filter="+query, "--delimiter=\t", "--with-nth=2..")
	c.Env, c.Stdin = withoutFZFOptions(os.Environ()), strings.NewReader(input)
	out, err := c.Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return "", err
	}
	marked := map[string]bool{}
	for _, id := range state.Marked {
		marked[id] = true
	}
	var actions []string
	focus := 0
	for rank, row := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		id, _, _ := strings.Cut(row, "\t")
		i, err := strconv.Atoi(id)
		if err != nil || i < 0 || i >= len(hosts) {
			continue
		}
		if marked[hosts[i].Alias] {
			actions = append(actions, fmt.Sprintf("pos(%d)+select", rank+1))
		}
		if hosts[i].Alias == state.Focus {
			focus = rank + 1
		}
	}
	if focus > 0 {
		actions = append(actions, fmt.Sprintf("pos(%d)", focus))
	}
	return strings.Join(actions, "+"), nil
}

type choice struct{ ID, Label string }

func choose(title string, options []choice, text string) (string, bool, error) {
	var input strings.Builder
	for _, option := range options {
		fmt.Fprintf(&input, "%s\t%s\n", option.ID, clean(option.Label))
	}
	// Matches are ranked (no --no-sort) so typing "rdp" prefers RDP over rsync's "dry-run".
	args := []string{"--height=90%", "--layout=default", "--border", "--header-first", "--delimiter=\t", "--with-nth=2..",
		"--prompt=" + title + " > ", "--header=Ctrl+J/K or arrows · Enter select · Esc back", "--bind=ctrl-j:down,ctrl-k:up"}
	if text != "" {
		f, err := os.CreateTemp("", "go-where-review-*.txt")
		if err != nil {
			return "", false, err
		}
		defer os.Remove(f.Name())
		_, writeErr := f.WriteString(text)
		closeErr := f.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return "", false, err
		}
		exe, err := os.Executable()
		if err != nil {
			return "", false, err
		}
		args = append(args, "--preview="+shellCommand([]string{exe, "__text", f.Name()}), "--preview-window=up:70%:wrap", "--bind=ctrl-u:preview-page-up,ctrl-d:preview-page-down")
	}
	c := exec.Command("fzf", args...)
	c.Env = withoutFZFOptions(os.Environ())
	c.Stdin, c.Stderr = strings.NewReader(input.String()), os.Stderr
	out, err := c.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && (exit.ExitCode() == 130 || exit.ExitCode() == 1) {
			return "", true, nil
		}
		return "", false, err
	}
	id, _, _ := strings.Cut(strings.TrimSuffix(string(out), "\n"), "\t")
	for _, option := range options {
		if option.ID == id {
			return id, false, nil
		}
	}
	return "", false, fmt.Errorf("invalid menu selection")
}

func chooseMode(current string) (string, bool, error) {
	return choose("Mode ("+current+")", []choice{
		{"ssh", "SSH — interactive shell"}, {"scp", "SCP — SFTP-based file copy"},
		{"legacy-scp", "Legacy SCP — scp -O, POSIX remote shell only"}, {"rsync", "rsync — efficient transfers with dry-run preview"},
		{"rdp", "RDP — Windows hosts tagged in your SSH config, via FreeRDP"},
	}, "")
}

func chooseRDPAccount(modeFile, configFile, selectionFile string) error {
	if err := os.WriteFile(selectionFile, nil, 0600); err != nil {
		return err
	}
	mode, err := readMode(modeFile)
	if err != nil || mode != "rdp" {
		return err
	}
	c, err := loadConfig(configFile)
	if err != nil {
		return err
	}
	options := []choice{{"0", "Default — configured user and domain rules"}}
	for i, account := range c.RDP.Accounts {
		options = append(options, choice{strconv.Itoa(i + 1), account.Name + " — " + account.User})
	}
	id, back, err := choose("RDP account", options, "")
	if err != nil || back {
		return err
	}
	i, err := strconv.Atoi(id)
	if err != nil || i < 0 || i > len(c.RDP.Accounts) {
		return fmt.Errorf("invalid RDP account selection")
	}
	account := RDPAccount{}
	if i > 0 {
		account = c.RDP.Accounts[i-1]
	}
	b, err := json.Marshal(account)
	if err != nil {
		return err
	}
	return os.WriteFile(selectionFile, b, 0600)
}

func readMode(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	mode := string(b)
	if !validMode(mode) {
		return "", fmt.Errorf("invalid picker mode")
	}
	return mode, nil
}

func writeMode(path, mode string) error {
	if !validMode(mode) {
		return fmt.Errorf("invalid picker mode")
	}
	f, err := os.CreateTemp(filepath.Dir(path), "go-where-mode-next-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(mode)
	if err := errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func notice(message string) error {
	_, _, err := choose("Notice", []choice{{"back", "Back"}}, message)
	return err
}

func completeLocal(value string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if value == "~" {
		return []string{"~/"}, nil
	}
	typedDir, base := filepath.Split(value)
	dir := expandHome(typedDir, home)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil // New destination names are valid input.
	}
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), base) || clean(entry.Name()) != entry.Name() {
			continue
		}
		candidate := typedDir + entry.Name()
		if entry.IsDir() {
			candidate += string(filepath.Separator)
		}
		matches = append(matches, candidate)
	}
	return matches, nil
}

func pathPrompt(label, value string, local bool, exe string) (string, bool, error) {
	for {
		var candidates []string
		var err error
		if local {
			candidates, err = completeLocal(value)
		}
		header := "Type a path · Enter accepts typed path · Tab completes local path · Esc back"
		if err != nil {
			header += "\nCannot list this path: " + clean(err.Error())
		}
		args := []string{"--height=16", "--layout=default", "--border", "--header-first", "--disabled", "--no-sort", "--query=" + value,
			"--prompt=" + label + " > ", "--header=" + header,
			"--bind=ctrl-j:down,ctrl-k:up,enter:become(printf '%s\\n' enter {q}),esc:become(printf '%s\\n' back {q})"}
		if local {
			args = append(args, "--bind=change:reload("+shellCommand([]string{exe, "__complete"})+" {q}),tab:become(printf '%s\\n' tab {q} {})")
		}
		c := exec.Command("fzf", args...)
		c.Env = withoutFZFOptions(os.Environ())
		c.Stdin, c.Stderr = strings.NewReader(strings.Join(candidates, "\n")), os.Stderr
		out, err := c.Output()
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && exit.ExitCode() == 130 {
				return value, true, nil
			}
			return value, false, err
		}
		parts := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		if len(parts) < 2 {
			return value, false, fmt.Errorf("invalid path prompt output")
		}
		value = parts[1]
		switch parts[0] {
		case "back":
			return value, true, nil
		case "enter":
			return value, false, nil
		case "tab":
			if len(parts) != 3 {
				return value, false, fmt.Errorf("invalid completion selection")
			}
			value = parts[2]
		default:
			return value, false, fmt.Errorf("unknown path action")
		}
	}
}
