package main

import (
	"os"
	"path/filepath"
	"strings"
)

type PickerState struct {
	TunnelColumnWidths []int
	ListPercent        int
	ColumnWidths       []int
	Source             string
	TunnelsConfig      string
	ConfigPath         string
	Account            RDPAccount
	Mode               string
	Focus              string
	Marked             []string
	RDPTags            []string
	Debug              bool
	RDPDestination     string
}

func pickerHelp(mode string) string {
	launch := "Enter          Launch using the default destination\nCtrl+Alt+P     New tmux pane\nCtrl+Alt+W     New tmux window\nCtrl+Alt+N     New tmux session\nCtrl+Alt+T     New terminal window"
	if mode == "rdp" {
		launch = "Enter          Launch using rdp.destination (background by default)\nAlt+A          Choose an account and launch selected hosts\nCtrl+Alt+T     Launch in a visible terminal for debugging"
	}
	return "LAUNCH\n" + launch + `

NAVIGATE & SELECT
Search         Host, address, source, tags, notes (fuzzy)
#tag           Exact tag filter; multiple #tags require all
Ctrl+B         Choose / create / attach an inventory source
Ctrl+N         New connection (Type field: SSH / RDP / Tunnel)
Ctrl+E         Edit managed connection (or existing SSH block)
←/→, Ctrl+H/L  Previous / next mode (wraps around)
Backspace      Delete search text (terminals sending Ctrl+H switch mode)
↑/↓, Ctrl+J/K  Move through hosts
Ctrl+U/D       Move half a page
Tab            Mark and move up
Shift+Tab      Mark and move down
Shift+↑/↓      Scroll preview

EDIT
Ctrl+S         Settings menu (raw editor available inside)
Ctrl+E         Database connection form / legacy host config editor
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
	key := menuKey(title)
	return runChoice(m, key)
}

func chooseOne(title string, options []choice, text, current string) (string, bool, error) {
	m := newDialog(title, options, text)
	m.singleChoice, m.current = true, current
	for i, option := range options {
		if option.ID == current {
			m.cursor = i
			break
		}
	}
	return runChoice(m, menuKey(title))
}

func runChoice(m *dialogModel, key string) (string, bool, error) {
	m.saveTunnel = saveActiveTunnel != nil
	if m.title == "Sources" {
		m.sourceActions = true
		var options []choice
		for _, option := range m.options {
			if option.ID != "create" && option.ID != "attach" {
				options = append(options, option)
			}
		}
		m.options = options
		m.refilter()
	}
	m.saveHost = saveActiveHost != nil && saveActiveSettings == nil
	if strings.Contains(m.title, "tunnel") || strings.Contains(m.title, "Tunnel") || strings.Contains(m.title, "Local") || strings.Contains(m.title, "local") || m.title == "Notice" {
		m.saveHost = false
	}
	if m.saveHost && strings.HasPrefix(m.title, "Host — ") {
		var options []choice
		for _, option := range m.options {
			if option.ID == "delete" {
				m.deleteHost = true
			}
			if option.ID != "save" && option.ID != "delete" {
				options = append(options, option)
			}
		}
		m.options = options
		m.refilter()
	}
	if menuPositions != nil {
		found := -1
		for i, title := range menuTrail {
			if title == m.title {
				found = i
				break
			}
		}
		if found >= 0 {
			menuTrail = menuTrail[:found+1]
		} else {
			menuTrail = append(menuTrail, m.title)
		}
		m.breadcrumb = strings.Join(menuTrail, " › ")
	}
	if position, ok := menuPositions[key]; ok {
		m.layout = position.layout
		m.query.SetValue(position.query)
		m.refilter()
		m.cursor = min(position.cursor, max(0, len(m.filtered)-1))
		for i, index := range m.filtered {
			if m.options[index].ID == position.selected {
				m.cursor = i
				break
			}
		}
		m.restorePreview = &position.preview
	}
	_, err := runTUI(m)
	for err == nil && (m.result == "__save_settings" || m.result == "__save_host" || m.result == "__save_tunnel") {
		if m.result == "__save_tunnel" && saveActiveTunnel != nil {
			if e := saveActiveTunnel(); e != nil {
				m.saveMessage = e.Error()
			} else {
				m.saveMessage = ""
			}
		}
		if m.result == "__save_host" && saveActiveHost != nil {
			if e := saveActiveHost(); e != nil {
				m.saveMessage = e.Error()
			} else {
				m.saveMessage = ""
			}
		}
		if saveActiveSettings != nil {
			if e := saveActiveSettings(); e != nil {
				m.saveMessage = e.Error()
			} else {
				m.saveMessage = "Saved"
			}
		}
		m.result = ""
		m.done = false
		m.back = false
		_, err = runTUI(m)
	}
	if m.back && len(menuTrail) > 0 {
		menuTrail = menuTrail[:len(menuTrail)-1]
	}
	if menuPositions != nil {
		position := menuPosition{layout: m.layout, query: m.query.Value(), cursor: m.cursor, preview: m.preview.YOffset}
		if len(m.filtered) > 0 {
			position.selected = m.options[m.filtered[m.cursor]].ID
		}
		menuPositions[key] = position
	}
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
