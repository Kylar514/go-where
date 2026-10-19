package main

import (
	"os"
	"path/filepath"
	"strings"
)

type PickerState struct {
	TunnelsConfig  string
	ConfigPath     string
	Account        RDPAccount
	Mode           string
	Focus          string
	Marked         []string
	RDPTags        []string
	Debug          bool
	RDPDestination string
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
Ctrl+G         Choose tmux target or terminal
Alt+1/2/3/4    SSH / RDP / Tunnels / Transfers
Ctrl+R         Reload hosts and settings
Esc / Ctrl+C   Cancel picker
? / F1         Open help
Click          Select rows, tabs and action buttons
Mouse wheel    Scroll list or preview
`
}

func validMode(s string) bool {
	return s == "ssh" || s == "scp" || s == "legacy-scp" || s == "rsync" || s == "rdp" || s == "tunnels"
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
		return "RDP: FreeRDP via ProxyJump; credentials stay local."
	}
	return ""
}

type choice struct{ ID, Label string }

func choose(title string, options []choice, text string) (string, bool, error) {
	m := newDialog(title, options, text)
	_, err := runTUI(m)
	return m.result, m.back, err
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
		return nil, nil
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
	m := newPathModel(label, value, local)
	_, err := runTUI(m)
	return m.input.Value(), m.back, err
}
