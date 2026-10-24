package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	digletconfig "github.com/kylar514/go-where/internal/tunnelcore"
	"github.com/sahilm/fuzzy"
)

type tunnelInfo struct {
	status, pid int
	owner       string
}
type tunnelPoll struct{}
type tunnelStatusMsg struct {
	statuses map[string]tunnelInfo
	err      error
}
type tunnelActionMsg struct {
	name string
	err  error
}
type hitRegion struct {
	x, y, width int
	action      string
	index       int
}
type pickerModel struct {
	hostTunnelTitle               string
	hostTunnelRoles               map[string]string
	hosts                         []Host
	state                         PickerState
	config                        Config
	dest                          string
	origin                        Origin
	search                        textinput.Model
	preview                       viewport.Model
	visible                       []int
	marked                        map[string]bool
	cursor, offset, width, height int
	transferMode                  string
	dialog                        *dialogModel
	dialogKind                    string
	hits                          []hitRegion
	key, status                   string
	statusError                   bool
	positions                     map[string]string
	selected                      []Host
	tunnels                       []digletconfig.Connection
	tunnelHosts                   map[string]Host
	tunnelErrors                  map[string]error
	tunnelPath                    string
	tunnelLoadErr                 error
	tunnelStatus                  map[string]tunnelInfo
	pending                       map[string]bool
	lastClick                     string
	lastClickAt                   time.Time
	dragKind                      string
	dragColumn, dragX             int
	dragWidths                    []int
	dragHostWidths                []int
}

func newPickerModel(hosts []Host, query, dest string, o Origin, state PickerState, c Config) *pickerModel {
	input := textinput.New()
	input.Prompt = "⌕ Search: "
	input.PromptStyle = tuiAccent
	input.Placeholder = "host, address, source, tags, notes…"
	input.SetValue(query)
	input.Focus()
	m := &pickerModel{hosts: hosts, state: state, config: c, dest: dest, origin: o, search: input, preview: viewport.New(1, 1), marked: map[string]bool{}, transferMode: "scp", tunnelStatus: map[string]tunnelInfo{}, pending: map[string]bool{}, positions: map[string]string{}}
	m.state.Account = RDPAccount{}
	if state.Mode == "" {
		m.state.Mode = "ssh"
	}
	if state.Mode == "scp" || state.Mode == "legacy-scp" || state.Mode == "rsync" {
		m.transferMode = state.Mode
	}
	for _, alias := range state.Marked {
		m.marked[alias] = true
	}
	m.loadTunnels()
	m.refilter()
	for i, id := range m.visible {
		identity := ""
		if m.state.Mode == "tunnels" {
			if h, ok := m.tunnelHosts[m.tunnels[id].Name]; ok {
				identity = hostIdentity(h)
			}
		} else if m.state.Mode == "transfers" {
			identity = transferMethods[id].ID
		} else {
			identity = hostIdentity(hosts[id])
		}
		if identity != "" && identity == state.Focus {
			m.cursor = i
			break
		}
	}
	m.updatePreview()
	return m
}

func (m *pickerModel) loadTunnels() {
	m.tunnelHosts = map[string]Host{}
	m.tunnelErrors = map[string]error{}
	m.tunnels = nil
	for _, h := range m.hosts {
		if h.SourceID != "" && h.Type == "tunnel" {
			conn, err := inventoryTunnel(h, m.config)
			m.tunnels = append(m.tunnels, conn)
			m.tunnelHosts[conn.Name] = h
			if err != nil {
				m.tunnelErrors[conn.Name] = err
			}
		}
	}
}

func (m *pickerModel) tunnelDisplayName(c digletconfig.Connection) string {
	if h, ok := m.tunnelHosts[c.Name]; ok {
		return h.Alias + " [" + h.SourceName + "]"
	}
	return c.Name
}

func (m *pickerModel) Init() tea.Cmd { return tea.Batch(textinput.Blink, tunnelTick()) }
func tunnelTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tunnelPoll{} })
}

func (m *pickerModel) refilter() {
	if m.state.Mode == "transfers" {
		m.visible = nil
		for i, method := range transferMethods {
			if strings.Contains(strings.ToLower(method.Label), strings.ToLower(m.search.Value())) {
				m.visible = append(m.visible, i)
			}
		}
		m.cursor = min(m.cursor, max(0, len(m.visible)-1))
		return
	}
	var ids []int
	var labels []string
	query := m.search.Value()
	var requiredTags []string
	var text []string
	for _, token := range strings.Fields(query) {
		if strings.HasPrefix(token, "#") && len(token) > 1 {
			requiredTags = append(requiredTags, token[1:])
		} else {
			text = append(text, token)
		}
	}
	query = strings.Join(text, " ")
	if m.state.Mode == "tunnels" {
		for i, c := range m.tunnels {
			if h, ok := m.tunnelHosts[c.Name]; ok {
				if !matchesTags(h.Tags, requiredTags) {
					continue
				}
				if m.state.Source == "ssh-config" || (m.state.Source != "" && m.state.Source != h.SourceID) {
					continue
				}
			} else if len(requiredTags) > 0 || m.state.Source != "" && m.state.Source != "ssh-config" {
				continue
			}
			ids = append(ids, i)
			if h, ok := m.tunnelHosts[c.Name]; ok {
				labels = append(labels, m.tunnelDisplayName(c)+" "+h.Comment+" "+strings.Join(h.Tags, ", "))
			} else {
				labels = append(labels, c.Name+" "+c.TunnelType+" "+c.SSHHost+" "+c.Resource+" "+c.Namespace)
			}
		}
	} else {
		for i, h := range m.hosts {
			if !matchesTags(h.Tags, requiredTags) {
				continue
			}
			if m.state.Source != "" && ((m.state.Source == "ssh-config" && h.SourceID != "") || (m.state.Source != "ssh-config" && h.SourceID != m.state.Source)) {
				continue
			}
			if h.SourceID != "" && !managedModeEligible(h, m.state.Mode) {
				continue
			}
			if m.state.Mode == "rdp" && !h.isRDP(m.config.RDP.Tags) {
				continue
			}
			ids = append(ids, i)
			labels = append(labels, h.Alias+" "+h.Hostname+" "+h.JumpHost+" "+h.Comment+" "+hostSourceLabel(h)+" "+strings.Join(h.Tags, ", "))
		}
	}
	m.visible = nil
	if query == "" {
		m.visible = ids
	} else {
		for _, match := range fuzzy.Find(query, labels) {
			m.visible = append(m.visible, ids[match.Index])
		}
	}
	m.sortVisible()
	m.cursor = min(max(0, m.cursor), max(0, len(m.visible)-1))
	m.scrollToCursor()
}

func matchesTags(tags, required []string) bool {
	for _, wanted := range required {
		found := false
		for _, tag := range tags {
			if strings.EqualFold(tag, wanted) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (m *pickerModel) rows() int {
	_, _, tabs := m.controls(m.tabs(), 0)
	_, _, buttons := m.controls(m.buttons(), 0)
	return max(1, min(20, m.height-tabs-buttons-5-2*m.toolbarGap()))
}

func (m *pickerModel) toolbarGap() int {
	return 0
}

// viewSplit: 70% list / 30% details on wide terminals; single column otherwise.
func viewSplit(width int) (listWidth, previewWidth int) {
	total := width - 2
	if width >= 90 {
		lw := max(40, total*7/10)
		return lw, max(18, total-lw-5)
	}
	return total, 0
}

func hostSourceLabel(h Host) string {
	if h.SourceID == "" {
		return "SSH config"
	}
	return h.SourceName
}

// Headings and rows share cell widths; hide Notes, then Tags as space shrinks.
func hostColumnLine(h *Host, width int, mark string) string {
	return renderHostColumns(h, width, mark, nil)
}

func renderHostColumns(h *Host, width int, mark string, custom []int) string {
	return renderHostColumnValues(h, width, mark, custom, hostColumnValues(h))
}

func hostColumnValues(h *Host) []string {
	if h == nil {
		return []string{"HOST", "HOSTNAME / IP", "SOURCE", "TAGS", "NOTES"}
	}
	return []string{h.Alias, h.Hostname, hostSourceLabel(*h), strings.Join(h.Tags, ", "), h.Comment}
}

func renderHostColumnValues(h *Host, width int, mark string, custom []int, values []string) string {
	values[0] = "    " + values[0]
	prefix := "     "
	if h != nil {
		prefix = " " + mark + " "
	}
	var widths []int
	switch {
	case width >= 88:
		widths = []int{18, 16, 12, 18, width - 77}
	case width >= 68:
		widths = []int{18, 16, 12, width - 57}
	case width >= 48:
		widths = []int{width - 35, 16, 10}
	case width >= 30:
		widths = []int{width - 19, 12}
	default:
		widths = []int{max(1, width-5)}
	}
	columns := make([]string, len(widths))
	if len(custom) == len(widths) {
		widths = custom
	}
	for i, w := range widths {
		v := fit(clean(values[i]), w)
		columns[i] = v + strings.Repeat(" ", max(0, w-lipgloss.Width(v)))
	}
	separator := "  "
	if h == nil {
		separator = " │"
	}
	return fit(prefix+strings.Join(columns, separator), width)
}
func (m *pickerModel) scrollToCursor() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.rows() {
		m.offset = m.cursor - m.rows() + 1
	}
	m.offset = min(max(0, m.offset), max(0, len(m.visible)-1))
}
func (m *pickerModel) switchMode(mode string) {
	if m.hostTunnelTitle != "" {
		return
	}
	previous := ""
	if len(m.visible) > 0 {
		if m.state.Mode == "transfers" {
			previous = transferMethods[m.visible[m.cursor]].ID
		} else if m.state.Mode == "tunnels" {
			previous = m.tunnels[m.visible[m.cursor]].Name
		} else {
			previous = hostIdentity(m.hosts[m.visible[m.cursor]])
		}
		m.positions[m.state.Mode] = previous
	}
	m.state.Mode = mode
	if mode == "scp" || mode == "legacy-scp" || mode == "rsync" {
		m.transferMode = mode
	}
	m.cursor, m.offset = 0, 0
	m.refilter()
	wanted := m.positions[mode]
	if wanted == "" && mode != "tunnels" {
		wanted = previous
	}
	for i, id := range m.visible {
		name := ""
		if mode == "transfers" {
			name = transferMethods[id].ID
		} else if mode == "tunnels" {
			name = m.tunnels[id].Name
		} else {
			name = hostIdentity(m.hosts[id])
		}
		if name == wanted {
			m.cursor = i
			break
		}
	}
	m.scrollToCursor()
	m.preview.GotoTop()
	m.updatePreview()
}
func (m *pickerModel) activeHost() (Host, bool) {
	if m.state.Mode == "tunnels" || m.state.Mode == "transfers" || len(m.visible) == 0 {
		return Host{}, false
	}
	return m.hosts[m.visible[m.cursor]], true
}
func (m *pickerModel) selection(highlightOnly bool) []Host {
	if m.state.Mode == "tunnels" || m.state.Mode == "transfers" {
		return nil
	}
	if !highlightOnly {
		var hosts []Host
		for _, h := range m.hosts {
			if m.state.Source != "" && ((m.state.Source == "ssh-config" && h.SourceID != "") || (m.state.Source != "ssh-config" && h.SourceID != m.state.Source)) {
				continue
			}
			if h.SourceID != "" && !managedModeEligible(h, m.state.Mode) {
				continue
			}
			if m.marked[hostIdentity(h)] && (m.state.Mode != "rdp" || h.isRDP(m.config.RDP.Tags)) {
				hosts = append(hosts, h)
			}
		}
		if len(hosts) > 0 {
			return hosts
		}
	}
	h, ok := m.activeHost()
	if !ok {
		return nil
	}
	return []Host{h}
}
func (m *pickerModel) finish(key string) tea.Cmd {
	if m.state.Mode == "transfers" && (key == "enter" || key == "debug") {
		if len(m.visible) == 0 {
			return nil
		}
		m.key = "transfer-method:" + transferMethods[m.visible[m.cursor]].ID
		return tea.Quit
	}
	if key == "transfers" {
		m.key = key
		return tea.Quit
	}
	if m.hostTunnelTitle != "" && (key == "settings" || key == "sources" || key == "mode") {
		return nil
	}
	if key == "debug" && m.state.Mode == "tunnels" {
		return m.debugTunnel()
	}
	if m.state.Mode == "tunnels" && (key == "ctrl-e" || key == "ctrl-alt-e") {
		if len(m.visible) > 0 {
			if h, ok := m.tunnelHosts[m.tunnels[m.visible[m.cursor]].Name]; ok {
				m.selected = []Host{h}
				m.key = "ctrl-e"
				return tea.Quit
			}
		}
		m.key = "new-connection"
		return tea.Quit
	}
	if key == "enter" && m.state.Mode == "tunnels" {
		return m.toggleTunnel()
	}
	if key == "enter" || key == "debug" || key == "connect-as" || strings.HasPrefix(key, "ctrl-alt-") || key == "ctrl-e" {
		m.selected = m.selection(key == "ctrl-alt-e")
		if len(m.selected) == 0 && key != "ctrl-alt-s" {
			return nil
		}
	}
	if key == "bulk-edit" {
		m.selected = m.selection(false)
		if len(m.selected) == 0 {
			return nil
		}
	}
	m.key = key
	m.state.Debug = key == "debug" || key == "ctrl-alt-t" && m.state.Mode == "rdp"
	return tea.Quit
}
func (m *pickerModel) openDialog(kind, title string, options []choice, text string) {
	m.dialogKind = kind
	m.dialog = newDialog(title, options, text)
	m.dialog.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
}
func (m *pickerModel) accountChoices() []choice {
	options := []choice{{"0", "Default"}}
	h, ok := m.activeHost()
	if !ok {
		return options
	}
	for i := 0; i <= len(m.config.RDP.Accounts); i++ {
		r := m.config.RDP
		label := "Default"
		if i > 0 {
			account := r.Accounts[i-1]
			label = account.Name
			r = r.withAccount(account)
		}
		previewConfig := m.config
		previewConfig.RDP = r
		if i > 0 {
			previewConfig.activeRDPAccount = r.Accounts[i-1]
		}
		job, err := hostRDPJob(previewConfig, h, "")
		if err == nil {
			login := job.User
			if job.Domain != "" {
				login = job.Domain + `\` + login
			}
			label += " — " + login
			if len(m.selection(false)) > 1 {
				label += " (domain resolved per host)"
			}
		}
		if i == 0 {
			options[0].Label = label
		} else {
			options = append(options, choice{strconv.Itoa(i), label})
		}
	}
	return options
}
func (m *pickerModel) action(action string) tea.Cmd {
	if m.hostTunnelTitle != "" && (action == "mode" || action == "target" || action == "sources" || action == "settings") {
		return nil
	}
	switch action {
	case "select-all":
		m.toggleAllMarks()
	case "mode:transfers":
		return m.finish("transfers")
	case "back":
		return m.finish("cancel")
	case "new-tunnel":
		return m.finish("new-connection")
	case "edit-tunnel":
		return m.finish("ctrl-e")
	case "help":
		text := configuredHelp(m.config) + "\n\n" + modeWarning(m.state.Mode) + "\n\nMouse: click to select, double-click to activate. Tab / Shift+Tab mark and move. Marks survive filtering. Edit uses marked hosts or the highlighted host.\n\nNormal keys navigate the list; Shift+Up/Down scroll Details and Shift+PgUp/PgDn scroll Details by a page. Help also accepts plain arrows and page keys. Drag the pane divider to resize."
		m.openDialog("help", "Keyboard & mouse", nil, text)
		m.dialog.readingScreen = true
		m.dialog.layout.listPercent = 20
		m.dialog.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	case "mode":
		m.openDialog("mode", "Choose mode", []choice{{"ssh", "SSH"}, {"rdp", "RDP"}, {"tunnels", "Tunnels"}, {"transfers", "Transfers"}}, "")
	case "target":
		return m.finish("target")
	case "sources":
		return m.finish("sources")
	case "settings":
		return m.finish("settings")
	case "connect":
		return m.finish("enter")
	case "bulk-edit":
		return m.finish("bulk-edit")
	case "start-selected":
		return m.bulkTunnels(false)
	case "stop-selected":
		return m.bulkTunnels(true)
	case "debug":
		return m.finish("debug")
	case "connect-as":
		return m.finish("connect-as")
	case "protocol":
		m.openDialog("mode", "Transfer protocol", []choice{{"scp", "SCP (SFTP)"}, {"legacy-scp", "Legacy SCP (-O)"}, {"rsync", "rsync"}}, "")
	}
	return nil
}
func (m *pickerModel) toggleTunnel() tea.Cmd {
	if len(m.visible) == 0 {
		return nil
	}
	c := m.tunnels[m.visible[m.cursor]]
	if err := m.tunnelErrors[c.Name]; err != nil {
		m.status = err.Error()
		m.statusError = true
		return nil
	}
	if m.pending[c.Name] {
		return nil
	}
	info := m.tunnelStatus[c.Name]
	if info.status == 3 && info.pid == 0 {
		m.status = fmt.Sprintf("Port %d is occupied; no unrelated process will be stopped", c.LocalPort)
		m.statusError = true
		return nil
	}
	m.pending[c.Name] = true
	return func() tea.Msg { return tunnelActionMsg{c.Name, toggleManagedTunnel(c, info.pid > 0)} }
}
func (m *pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tunnelPoll:
		if m.state.Mode != "tunnels" || len(m.tunnels) == 0 {
			return m, tunnelTick()
		}
		connections := append([]digletconfig.Connection(nil), m.tunnels...)
		return m, func() tea.Msg { s, e := tunnelStatuses(connections); return tunnelStatusMsg{s, e} }
	case tunnelStatusMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			m.statusError = true
		} else {
			m.tunnelStatus = msg.statuses
			if s, ok := m.state.Sorts["tunnels"]; m.state.Mode == "tunnels" && ok && s.Column == 5 {
				m.sortVisible()
				m.scrollToCursor()
			}
			if m.statusError {
				m.status = ""
				m.statusError = false
			}
		}
		m.updatePreview()
		return m, tunnelTick()
	case tunnelActionMsg:
		if result, ok := msg.err.(debugTunnelResult); ok {
			msg.err = result.err
			text := result.text + "\n\nTracked tunnel startup completed."
			if result.err != nil {
				text = result.text + "\n\nFAILED: " + result.err.Error()
			}
			m.openDialog("debug-tunnel", "Tunnel diagnostics", []choice{{"back", "Back"}}, text)
		}
		delete(m.pending, msg.name)
		if msg.err != nil {
			m.status = msg.err.Error()
			m.statusError = true
		} else {
			name := msg.name
			if h, ok := m.tunnelHosts[msg.name]; ok {
				name = h.Alias
			}
			m.status = name + ": tunnel updated"
			m.statusError = false
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.search.Width = max(1, msg.Width-4-lipgloss.Width(m.search.Prompt))
		_, previewWidth := m.paneWidths()
		m.preview.Width = max(1, previewWidth)
		m.preview.Height = max(1, m.rows()-1)
		m.scrollToCursor()
		m.updatePreview()
	}
	if m.dialog != nil {
		_, cmd := m.dialog.Update(msg)
		if !m.dialog.done {
			return m, cmd
		}
		d, kind := m.dialog, m.dialogKind
		m.dialog = nil
		if d.back {
			return m, nil
		}
		switch kind {
		case "mode":
			m.switchMode(d.result)
		}
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := canonicalKey(m.config, msg.String(), "picker")
		if m.width > 0 && (m.width < 30 || m.height < 12) && key != "esc" && key != "ctrl+c" {
			return m, nil
		}
		switch key {
		case "alt+d":
			return m, m.finish("debug")
		case "ctrl+b":
			return m, m.finish("sources")
		case "ctrl+n":
			return m, m.finish("new-connection")
		case "left", "ctrl+h", "right", "ctrl+l":
			if m.hostTunnelTitle != "" {
				if key == "left" || key == "ctrl+h" {
					return m, m.finish("cancel")
				}
				return m, m.finish("enter")
			}
			modes := []string{"ssh", "rdp", "tunnels", "transfers"}
			for i, mode := range modes {
				if mode == m.state.Mode {
					delta := 1
					if key == "left" || key == "ctrl+h" {
						delta = -1
					}
					next := modes[(i+delta+len(modes))%len(modes)]
					m.switchMode(next)
					break
				}
			}
			return m, nil
		case "ctrl+c", "esc":
			return m, m.finish("cancel")
		case "?", "f1":
			return m, m.action("help")
		case "ctrl+o":
			return m, m.action("mode")
		case "ctrl+g":
			return m, m.action("target")
		case "ctrl+z":
			m.resetLayout()
			return m, nil
		case "alt+a":
			if m.state.Mode != "tunnels" && m.state.Mode != "transfers" {
				return m, m.finish("connect-as")
			}
			return m, nil
		case "alt+1":
			m.switchMode("ssh")
			return m, nil
		case "alt+2":
			m.switchMode("rdp")
			return m, nil
		case "alt+3":
			m.switchMode("tunnels")
			return m, nil
		case "alt+4":
			m.switchMode("transfers")
			return m, nil
		case "alt+5":
			m.switchMode("legacy-scp")
			return m, nil
		case "alt+6":
			m.switchMode("rsync")
			return m, nil
		// The list renders bottom-up: up moves the highlight visually up
		// (a higher entry index), down moves visually down.
		case "up", "ctrl+k":
			m.cursor = cycleCursor(m.cursor, 1, len(m.visible))
		case "down", "ctrl+j":
			m.cursor = cycleCursor(m.cursor, -1, len(m.visible))
		case "ctrl+u", "pgup":
			m.cursor = min(max(0, len(m.visible)-1), m.cursor+max(1, m.rows()/2))
		case "ctrl+d", "pgdown":
			m.cursor = max(0, m.cursor-max(1, m.rows()/2))
		case "shift+up":
			m.preview.ScrollUp(1)
			return m, nil
		case "shift+down":
			m.preview.ScrollDown(1)
			return m, nil
		case "shift+pgup":
			m.preview.HalfPageUp()
			return m, nil
		case "shift+pgdown":
			m.preview.HalfPageDown()
			return m, nil
		case "ctrl+a":
			m.toggleAllMarks()
			return m, nil
		case "tab", "shift+tab":
			if m.state.Mode == "tunnels" && len(m.visible) > 0 {
				name := m.tunnels[m.visible[m.cursor]].Name
				m.marked[name] = !m.marked[name]
				delta := 1
				if key == "shift+tab" {
					delta = -1
				}
				m.cursor = cycleCursor(m.cursor, delta, len(m.visible))
				break
			}
			if h, ok := m.activeHost(); ok {
				m.marked[hostIdentity(h)] = !m.marked[hostIdentity(h)]
				delta := 1
				if key == "shift+tab" {
					delta = -1
				}
				m.cursor = max(0, min(len(m.visible)-1, m.cursor+delta))
			}
		case "ctrl+e":
			return m, m.finish("ctrl-e")
		case "enter", "ctrl+r", "ctrl+s", "ctrl+alt+p", "ctrl+alt+w", "ctrl+alt+n", "ctrl+alt+t", "ctrl+alt+s", "ctrl+alt+e":
			return m, m.finish(strings.ReplaceAll(key, "+", "-"))
		default:
			old := m.search.Value()
			var cmd tea.Cmd
			m.search, cmd = m.search.Update(msg)
			if m.search.Value() != old {
				m.cursor, m.offset = 0, 0
				m.refilter()
				m.preview.GotoTop()
				m.updatePreview()
			}
			return m, cmd
		}
		m.scrollToCursor()
		m.preview.GotoTop()
		m.updatePreview()
	case tea.MouseMsg:
		if m.resizeMouse(msg) {
			return m, nil
		}
		if m.width < 30 || m.height < 12 {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
			delta := 3
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -3
			}
			listWidth, _ := m.paneWidths()
			if m.width >= 90 && msg.X > listWidth {
				if delta < 0 {
					m.preview.ScrollUp(-delta)
				} else {
					m.preview.ScrollDown(delta)
				}
			} else {
				m.cursor = max(0, min(max(0, len(m.visible)-1), m.cursor-delta))
				m.scrollToCursor()
				m.updatePreview()
			}
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			for _, hit := range m.hits {
				if msg.Y == hit.y && msg.X >= hit.x && msg.X < hit.x+hit.width {
					if hit.action == "row" {
						m.cursor = hit.index
						if m.state.Mode != "transfers" && msg.X >= 1 && msg.X <= 3 {
							if m.state.Mode == "tunnels" || m.state.Mode == "transfers" {
								name := m.tunnels[m.visible[m.cursor]].Name
								m.marked[name] = !m.marked[name]
							}
							if h, ok := m.activeHost(); ok {
								m.marked[hostIdentity(h)] = !m.marked[hostIdentity(h)]
							}
							m.lastClick = ""
							m.updatePreview()
							return m, nil
						}
						m.updatePreview()
						identity := m.state.Mode + ":" + strconv.Itoa(m.visible[hit.index])
						now := time.Now()
						if m.lastClick == identity && now.Sub(m.lastClickAt) < 400*time.Millisecond {
							m.lastClick = ""
							if m.state.Mode == "tunnels" {
								return m, m.finish("enter")
							}
							h, _ := m.activeHost()
							cmd := m.finish("enter")
							m.selected = []Host{h}
							return m, cmd
						}
						m.lastClick, m.lastClickAt = identity, now
						return m, nil
					}
					m.lastClick = ""
					if hit.action == "sort-column" {
						m.toggleSort(hit.index)
						return m, nil
					}
					if hit.action == "reset-layout" {
						m.resetLayout()
						return m, nil
					}
					if strings.HasPrefix(hit.action, "mode:") {
						m.switchMode(strings.TrimPrefix(hit.action, "mode:"))
						return m, nil
					}
					return m, m.action(hit.action)
				}
			}
		}
	}
	return m, nil
}

func (m *pickerModel) updatePreview() {
	if m.state.Mode == "transfers" {
		text := "Choose a transfer method."
		if len(m.visible) > 0 {
			text = transferMethodDetails[transferMethods[m.visible[m.cursor]].ID]
		}
		m.preview.SetContent(ansi.Wrap(text, max(1, m.preview.Width), ""))
		return
	}
	var text string
	if m.state.Mode == "tunnels" {
		if len(m.visible) > 0 {
			c := m.tunnels[m.visible[m.cursor]]
			if h, ok := m.tunnelHosts[c.Name]; ok {
				text = managedDetails(h, m.config) + "\n\n" + m.tunnelLabel(c.Name)
				if role := m.hostTunnelRoles[hostIdentity(h)]; role != "" {
					text += "\n\nSelected host role: " + role
				}
				if err := m.tunnelErrors[c.Name]; err != nil {
					text += "\n" + err.Error()
				}
				m.setHostPreview(text, h)
				return
			}
			text = fmt.Sprintf("%s\n\nType       %s\nLocal      127.0.0.1:%d\nRemote     %d\nSSH host   %s\nResource   %s\nNamespace  %s\n\n%s\n\n%s", c.Name, c.TunnelType, c.LocalPort, c.RemotePort, c.SSHHost, c.Resource, c.Namespace, m.tunnelLabel(c.Name), tunnelCommandPreview(c))
		} else {
			text = "No database tunnels configured.\n\nAdd a tunnel with Ctrl+N in an enabled SQLite source. SSH tunnels reference saved SSH profiles; Kubernetes tunnels use saved forwarding settings."
			if m.tunnelLoadErr != nil {
				text += "\n\n" + m.tunnelLoadErr.Error()
			}
		}
	} else if h, ok := m.activeHost(); ok {
		if h.SourceID != "" {
			details := managedDetails(h, m.config)
			if isTransferMode(m.state.Mode) {
				details = "Transfer method: " + strings.ToUpper(m.state.Mode) + "\n" + modeWarning(m.state.Mode) + "\n\n" + details
			}
			m.setHostPreview(details, h)
			return
		}
		text = fmt.Sprintf("%s\n\nAddress   %s\nJump      %s\nTags      %s\nNotes     %s\nSource    %s\nFile      %s:%d\n\n", h.Alias, h.Hostname, h.JumpHost, strings.Join(h.Tags, ", "), h.Comment, hostSourceLabel(h), h.File, h.Line)
		if m.state.Mode == "rdp" {
			job, err := newRDPJob(m.config.RDP, h, "")
			if err != nil {
				text += err.Error()
			} else {
				text += "Default account\n" + job.Domain + `\` + job.User + "\n\nWindow title\n" + job.Title + "\n\nAlt+A chooses another account.\n"
			}
		}
		text += modeWarning(m.state.Mode) + "\n\n" + h.Block
	} else {
		text = "No matching hosts.\n\nRDP lists hosts with an indented # Windows or # RDP tag."
		if isTransferMode(m.state.Mode) {
			text = "No SSH-capable hosts found in the selected sources.\n\nAdd an SSH profile to a host, or enable another source.\n\nTransfer method: " + strings.ToUpper(m.state.Mode) + "\n" + modeWarning(m.state.Mode)
		} else if m.state.Mode == "ssh" {
			text = "No matching hosts.\n\nAdd a host with an SSH profile, enable another source, or clear the search filter."
		}
	}
	m.preview.SetContent(ansi.Wrap(cleanLines(text), max(1, m.preview.Width), ""))
}
func tunnelCommandPreview(c digletconfig.Connection) string {
	if c.TunnelType == "ssh" {
		return shellCommand([]string{"ssh", "-o", "ExitOnForwardFailure=yes", "-L", fmt.Sprintf("%d:localhost:%d", c.LocalPort, c.RemotePort), "-N", c.SSHHost})
	}
	if c.TunnelType == "kubectl" {
		args := []string{"kubectl", "port-forward", c.Resource, fmt.Sprintf("%d:%d", c.LocalPort, c.RemotePort)}
		if c.Namespace != "" {
			args = append(args, "-n", c.Namespace)
		}
		return shellCommand(args)
	}
	return "Docker tunnels are not implemented."
}
func (m *pickerModel) tunnelLabel(name string) string {
	if m.pending[name] {
		return "◌ working"
	}
	info := m.tunnelStatus[name]
	switch info.status {
	case 1:
		return "◌ connecting"
	case 2:
		return "● active"
	case 3:
		return "! occupied"
	}
	return "○ inactive"
}
func (m *pickerModel) View() string {
	if m.dialog != nil {
		return m.dialog.View()
	}
	if m.width == 0 {
		return "Loading go-where…"
	}
	m.hits = nil
	if m.width < 30 || m.height < 12 || m.width < 55 && m.height < 16 {
		return fit("Resize terminal (55×12 or 30×16) · Esc exits", m.width)
	}
	width := m.width - 2
	tabs, tabHits, tabHeight := m.controls(m.tabs(), 0)
	m.hits = append(m.hits, tabHits...)
	lines := []string{tabs}
	if m.toolbarGap() > 0 {
		lines = append(lines, "")
	}
	listWidth, _ := m.paneWidths()
	columns := m.columnWidths(listWidth)
	var left []string
	// Fixed column titles; the list scrolls beneath them.
	if m.state.Mode == "transfers" {
		left = append(left, "Transfer method")
	} else if m.state.Mode == "tunnels" {
		left = append(left, tuiDim.Render(m.tunnelColumns(-1, listWidth, columns)))
		x := 9
		for i, w := range columns {
			m.hits = append(m.hits, hitRegion{x, tabHeight, w, "sort-column", m.sortColumn(i, len(columns))})
			x += w
			if i < len(columns)-1 {
				m.hits = append(m.hits, hitRegion{x, tabHeight, 2, "resize-column", i})
				x += 2
			}
		}
	} else {
		values := hostColumnValues(nil)
		m.sortHeading(values, columns)
		heading := renderHostColumnValues(nil, listWidth, "", columns, values)
		left = append(left, tuiDim.Render(heading))
		x := 5
		for i, w := range columns {
			if m.state.Mode == "ssh" || m.state.Mode == "rdp" {
				m.hits = append(m.hits, hitRegion{x, tabHeight, w, "sort-column", i})
			}
			x += w
			if i < len(columns)-1 {
				m.hits = append(m.hits, hitRegion{x, tabHeight, 2, "resize-column", i})
				x += 2
			}
		}
	}
	// Rows anchor to the bottom and fill upward (fzf --layout=reverse): the
	// first result sits on the last slot, older entries stack above it.
	pad := max(0, m.rows()-max(1, len(m.visible)))
	occupied := m.rows() - pad
	for row := 0; row < m.rows(); row++ {
		line := ""
		if row < pad {
			left = append(left, "")
			continue
		}
		i := m.offset + occupied - 1 - (row - pad)
		if i < len(m.visible) {
			id := m.visible[i]
			if m.state.Mode == "transfers" {
				line = "  " + transferMethods[id].Label
			} else if m.state.Mode == "tunnels" {
				line = m.tunnelColumns(id, listWidth, columns)
			} else {
				h := m.hosts[id]
				mark := "[ ]"
				if m.marked[hostIdentity(h)] {
					mark = "[✓]"
				}
				line = renderHostColumns(&h, listWidth, mark, columns)
			}
			if i == m.cursor {
				line = tuiSelected.Render(fit("›"+strings.TrimPrefix(line, " "), listWidth))
			}
			if m.state.Mode == "transfers" {
			} else if m.state.Mode != "tunnels" {
				h := m.hosts[id]
				if h.Color != "" {
					line = ansi.Cut(line, 0, 5) + hostColorMarker(h.Color, h.BracketColor) + ansi.Cut(line, 8, listWidth)
				}
			} else if h, ok := m.tunnelHosts[m.tunnels[id].Name]; ok && h.Color != "" {
				line = ansi.Cut(line, 0, 5) + hostColorMarker(h.Color, h.BracketColor) + ansi.Cut(line, 8, listWidth)
			}
			m.hits = append(m.hits, hitRegion{0, tabHeight + 1 + m.toolbarGap() + row, listWidth, "row", i})
		} else if row == m.rows()-1 {
			line = tuiDim.Render("No matches")
		}
		left = append(left, fit(line, listWidth))
	}
	body := strings.Join(left, "\n")
	if m.width >= 90 {
		// The frame shares the list's height, including its column heading.
		panel := detailsPanel(m.preview)
		divider := strings.TrimSuffix(strings.Repeat(tuiDim.Render("│")+"\n", m.rows()+1), "\n")
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listWidth).Render(body), divider, panel)
		for row := 0; row < m.rows()+1; row++ {
			m.hits = append(m.hits, hitRegion{listWidth, tabHeight + row, 1, "resize-pane", 0})
		}
	}
	// Separate the search input from results without adding another row.
	lines = append(lines, body, strings.Repeat("─", width), m.search.View())
	if m.toolbarGap() > 0 {
		lines = append(lines, "")
	}
	buttons, buttonHits, _ := m.controls(m.buttons(), tabHeight+m.rows()+3+2*m.toolbarGap())
	m.hits = append(m.hits, buttonHits...)
	status := tuiDim.Render(clean(m.status))
	if m.statusError {
		status = tuiError.Render(clean(m.status))
	}
	lines = append(lines, buttons, fit(tuiDim.Render(fmt.Sprintf("%d results · %d marked", len(m.visible), m.markCount())), width))
	if m.height >= 14 || m.status != "" {
		lines = append(lines, fit(status, width))
	}
	return strings.Join(lines, "\n")
}
func (m *pickerModel) markCount() int {
	n := 0
	for _, marked := range m.marked {
		if marked {
			n++
		}
	}
	return n
}

func pick(hosts []Host, exe, query, dest string, o Origin, state *PickerState) (string, string, []Host, error) {
	if state == nil {
		state = &PickerState{Mode: "ssh"}
	}
	c := defaultConfig()
	if state.ConfigPath != "" {
		var err error
		c, err = loadConfig(state.ConfigPath)
		if err != nil {
			return "", query, nil, err
		}
	}
	if len(state.RDPTags) > 0 {
		c.RDP.Tags = state.RDPTags
	}
	if state.RDPDestination != "" {
		c.RDP.Destination = state.RDPDestination
	}
	if state.TunnelsConfig != "" {
		c.TunnelsConfig = state.TunnelsConfig
	}
	m := newPickerModel(hosts, query, dest, o, *state, c)
	previousKeys := menuHotkeys
	menuHotkeys = c
	defer func() { menuHotkeys = previousKeys }()
	if _, err := runTUI(m); err != nil {
		return "", query, nil, err
	}
	*state = m.state
	state.Marked = nil
	for _, h := range hosts {
		if m.marked[hostIdentity(h)] {
			state.Marked = append(state.Marked, hostIdentity(h))
		}
	}
	for _, tunnel := range m.tunnels {
		if m.marked[tunnel.Name] {
			state.Marked = append(state.Marked, tunnel.Name)
		}
	}
	if h, ok := m.activeHost(); ok {
		state.Focus = hostIdentity(h)
	}
	return m.key, m.search.Value(), m.selected, nil
}
