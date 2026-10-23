package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

var popupFrameEnabled bool

var runTUI = func(m tea.Model) (tea.Model, error) {
	if popupFrameEnabled {
		m = &popupFrame{model: m}
	}
	return tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
}

var (
	tuiAccent   = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	tuiDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tuiSelected = lipgloss.NewStyle().Reverse(true).Bold(true)
	tuiError    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

func fit(s string, width int) string { return ansi.Truncate(s, max(0, width), "…") }

func detailsPanel(v viewport.Model) string {
	pw := v.Width
	panel := tuiDim.Render("╭─ Details "+strings.Repeat("─", max(0, pw-8))+"╮") + "\n"
	total := v.TotalLineCount()
	thumb, start := 0, 0
	if total > v.Height && v.Height > 0 {
		thumb = max(1, v.Height*v.Height/total)
		start = int(v.ScrollPercent() * float64(v.Height-thumb))
	}
	for row, line := range strings.Split(v.View(), "\n") {
		indicator := " "
		if thumb > 0 {
			indicator = tuiDim.Render("░")
			if row >= start && row < start+thumb {
				indicator = tuiAccent.Render("█")
			}
		}
		panel += tuiDim.Render("│") + " " + lipgloss.NewStyle().Width(pw).Render(line) + indicator + tuiDim.Render("│") + "\n"
	}
	return panel + tuiDim.Render("╰"+strings.Repeat("─", pw+2)+"╯")
}

func cycleCursor(cursor, delta, count int) int {
	if count <= 0 {
		return 0
	}
	return ((cursor+delta)%count + count) % count
}

func menuSearch() textinput.Model {
	input := textinput.New()
	input.Prompt = "⌕ Search: "
	input.PromptStyle = tuiAccent
	input.Placeholder = "filter…"
	input.Focus()
	return input
}

type menuPosition struct {
	layout          menuLayout
	query, selected string
	cursor, preview int
}

var menuPositions map[string]menuPosition
var menuDepth int
var menuTrail []string

// Keep navigation only for this menu visit, never across unrelated launches.
func menuSession() func() {
	if menuDepth == 0 {
		menuPositions = map[string]menuPosition{}
		menuTrail = nil
	}
	menuDepth++
	return func() {
		menuDepth--
		if menuDepth == 0 {
			menuPositions = nil
			menuTrail = nil
		}
	}
}

func menuKey(title string) string {
	_, file, line, _ := runtime.Caller(2)
	return fmt.Sprintf("%s:%d:%s", file, line, title)
}

type dialogModel struct {
	footerActions  []choice
	optionDetails  map[string]string
	saveTunnel     bool
	layout         menuLayout
	resizeKind     string
	resizeStart    int
	resizeLayout   menuLayout
	resizeHits     []hitRegion
	sourceActions  bool
	deleteHost     bool
	saveHost       bool
	colorChoices   bool
	saveMessage    string
	swatch         string
	breadcrumb     string
	singleChoice   bool
	current        string
	title          string
	options        []choice
	filtered       []int
	cursor         int
	query          textinput.Model
	preview        viewport.Model
	text           string
	width, height  int
	result         string
	restorePreview *int
	done, back     bool
}

func newDialog(title string, options []choice, text string) *dialogModel {
	input := menuSearch()
	m := &dialogModel{title: title, options: options, text: text, query: input, preview: viewport.New(1, 1)}
	m.refilter()
	return m
}

func (m *dialogModel) refilter() {
	m.filtered = nil
	if m.query.Value() == "" {
		for i := range m.options {
			if m.title == "Settings" && (m.options[i].ID == "save" || m.options[i].ID == "cancel") {
				continue
			}
			m.filtered = append(m.filtered, i)
		}
	} else {
		labels := make([]string, len(m.options))
		for i, o := range m.options {
			labels[i] = o.Label
		}
		for _, match := range fuzzy.Find(m.query.Value(), labels) {
			if m.title == "Settings" && (m.options[match.Index].ID == "save" || m.options[match.Index].ID == "cancel") {
				continue
			}
			m.filtered = append(m.filtered, match.Index)
		}
	}
	m.cursor = min(max(0, m.cursor), max(0, len(m.filtered)-1))
}

func (m *dialogModel) Init() tea.Cmd { return textinput.Blink }
func (m *dialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer func() {
		if len(m.optionDetails) > 0 && len(m.filtered) > 0 {
			index := m.filtered[min(m.cursor, len(m.filtered)-1)]
			text := m.text
			if detail := m.optionDetails[m.options[index].ID]; detail != "" {
				text += "\n\n" + detail
			}
			m.preview.SetContent(ansi.Wrap(menuExplanation(text), max(1, m.preview.Width), ""))
		}
	}()
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.query.Width = max(1, msg.Width-ansi.StringWidth(m.query.Prompt)-1)
		m.preview.Width = max(1, min(64, msg.Width-4))
		if msg.Width >= 90 {
			_, pw := m.menuPaneWidths()
			m.preview.Width = pw
		}
		content := ansi.Wrap(menuExplanation(m.text), m.preview.Width, "")
		m.preview.Height = min(lipgloss.Height(content), max(1, (msg.Height-7)/2))
		if msg.Width >= 90 {
			m.preview.Height = max(1, min(20, msg.Height-5-footerExtra(msg.Height))-2)
		}
		m.preview.SetContent(content)
		if m.restorePreview != nil {
			m.preview.SetYOffset(*m.restorePreview)
			m.restorePreview = nil
		}
	case tea.KeyMsg:
		scope := "menu"
		if m.sourceActions {
			scope = "sources"
		}
		key := canonicalKey(menuHotkeys, msg.String(), scope)
		if m.width > 0 && (m.width < 24 || m.height < 9) && key != "esc" {
			return m, nil
		}
		switch key {
		case "ctrl+n", "ctrl+o", "ctrl+alt+d":
			if m.sourceActions {
				id := "create"
				if key == "ctrl+o" {
					id = "attach"
				}
				if key == "ctrl+alt+d" {
					if len(m.filtered) == 0 {
						return m, nil
					}
					source := m.options[m.filtered[m.cursor]].ID
					if source == "" || source == "ssh-config" {
						return m, nil
					}
					id = "remove:" + source
				}
				m.result, m.done = id, true
				return m, tea.Quit
			}
		case "ctrl+z":
			m.resetMenuLayout()
			return m, nil
		case "delete":
			if m.deleteHost {
				m.result, m.done = "delete", true
				return m, tea.Quit
			}
		case "ctrl+s":
			if len(m.footerActions) > 0 {
				for _, a := range m.footerActions {
					if a.ID == "save-host" || a.ID == "save" {
						m.result, m.done = a.ID, true
						return m, tea.Quit
					}
				}
				return m, nil
			}
			if m.saveTunnel {
				m.result, m.done = "__save_tunnel", true
				return m, tea.Quit
			}
			if m.sourceActions {
				m.result, m.done = "settings", true
				return m, tea.Quit
			}
			if m.saveHost {
				m.result, m.done = "__save_host", true
				return m, tea.Quit
			}
			if saveActiveSettings != nil {
				m.result, m.done = "__save_settings", true
				return m, tea.Quit
			}
			if m.title == "Settings" {
				m.result, m.done = "save", true
				return m, tea.Quit
			}
		case "esc", "ctrl+c", "ctrl+h", "left":
			m.done, m.back = true, true
			return m, tea.Quit
		case "up", "ctrl+k":
			m.cursor = cycleCursor(m.cursor, 1, len(m.filtered))
			return m, nil
		case "down", "ctrl+j":
			m.cursor = cycleCursor(m.cursor, -1, len(m.filtered))
			return m, nil
		case "ctrl+u":
			m.preview.HalfPageUp()
			return m, nil
		case "ctrl+d":
			m.preview.HalfPageDown()
			return m, nil
		case "enter", "ctrl+l":
			if len(m.filtered) > 0 {
				m.result = m.options[m.filtered[m.cursor]].ID
				m.done = true
				return m, tea.Quit
			}
			return m, nil
		}
		if m.saveHost && saveHostLocalSettings != nil && msg.String() == "ctrl+alt+s" {
			m.result, m.done = "__save_host_local", true
			return m, tea.Quit
		}
	case tea.MouseMsg:
		if m.resizeMenuMouse(msg) {
			return m, nil
		}
		if m.width < 24 || m.height < 9 {
			return m, nil
		}
		if len(m.footerActions) > 0 && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.footerY() {
			x := 8
			for _, a := range m.footerActions {
				width := ansi.StringWidth(a.Label) + 3
				if msg.X >= x && msg.X < x+width {
					if a.ID == "select" {
						if len(m.filtered) == 0 {
							return m, nil
						}
						m.result = m.options[m.filtered[m.cursor]].ID
					} else {
						m.result = a.ID
					}
					m.done = true
					return m, tea.Quit
				}
				x += width
			}
			if msg.X >= 8 {
				return m, nil
			}
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.footerY() && msg.X >= 0 && msg.X < 8 {
			m.done, m.back = true, true
			return m, tea.Quit
		}
		if m.title == "Settings" && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.footerY() {
			if msg.X >= 8 && msg.X < 15 {
				if saveActiveSettings != nil {
					m.result, m.done = "__save_settings", true
					return m, tea.Quit
				}
				m.result, m.done = "save", true
				return m, tea.Quit
			}
			if msg.X >= 15 && msg.X < 24 {
				m.result, m.done = "cancel", true
				return m, tea.Quit
			}
		}
		if m.title != "Settings" && saveActiveSettings != nil && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.footerY() && msg.X >= 8 && msg.X < 24 {
			m.result, m.done = "__save_settings", true
			return m, tea.Quit
		}
		if m.saveHost && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.footerY() && msg.X >= 8 && msg.X < 20 {
			m.result, m.done = "__save_host", true
			return m, tea.Quit
		}
		if m.saveTunnel && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.footerY() && msg.X >= 8 && msg.X < 22 {
			m.result, m.done = "__save_tunnel", true
			return m, tea.Quit
		}
		if m.deleteHost && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.footerY() && msg.X >= 20 && msg.X < 34 {
			m.result, m.done = "delete", true
			return m, tea.Quit
		}
		if m.saveHost && saveHostLocalSettings != nil && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.footerY() {
			x := 20
			if m.deleteHost {
				x = 34
			}
			if msg.X >= x && msg.X < x+22 {
				m.result, m.done = "__save_host_local", true
				return m, tea.Quit
			}
		}
		if m.sourceActions && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.footerY() {
			settingsX := 39
			if len(m.filtered) > 0 {
				id := m.options[m.filtered[m.cursor]].ID
				if id != "" && id != "ssh-config" {
					settingsX = 55
				}
			}
			if msg.X >= settingsX && msg.X < settingsX+11 {
				m.result, m.done = "settings", true
				return m, tea.Quit
			}
			if msg.X >= 39 && msg.X < 55 && len(m.filtered) > 0 {
				option := m.options[m.filtered[m.cursor]]
				if option.ID != "" && option.ID != "ssh-config" {
					m.result, m.done = "remove:"+option.ID, true
					return m, tea.Quit
				}
			}
			if msg.X >= 8 && msg.X < 24 {
				m.result, m.done = "create", true
				return m, tea.Quit
			}
			if msg.X >= 24 && msg.X < 39 {
				m.result, m.done = "attach", true
				return m, tea.Quit
			}
		}
		if msg.Button == tea.MouseButtonWheelUp {
			if msg.Y >= m.listStart() && msg.Y < m.listStart()+m.optionRows() && m.inList(msg.X) {
				m.cursor = min(max(0, len(m.filtered)-1), m.cursor+3)
				return m, nil
			}
			m.preview.ScrollUp(3)
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			if msg.Y >= m.listStart() && msg.Y < m.listStart()+m.optionRows() && m.inList(msg.X) {
				m.cursor = max(0, m.cursor-3)
				return m, nil
			}
			m.preview.ScrollDown(3)
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			row := msg.Y - m.listStart()
			i := -1
			if row >= 0 && row < m.optionRows() {
				i = m.visibleSlots()[row]
			}
			if i >= 0 && i < len(m.filtered) && m.inList(msg.X) {
				m.cursor = i
				m.result = m.options[m.filtered[m.cursor]].ID
				m.done = true
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	old := m.query.Value()
	m.query, cmd = m.query.Update(msg)
	if old != m.query.Value() {
		m.cursor = 0
	}
	m.refilter()
	return m, cmd
}

func (m *dialogModel) inList(x int) bool {
	width := m.width
	if m.width >= 90 {
		width, _ = m.menuPaneWidths()
	}
	return x >= 0 && x < width
}
func (m *dialogModel) optionRows() int {
	rows := m.height - 5 - footerExtra(m.height)
	if m.text != "" && m.width < 90 {
		rows -= m.preview.Height + 1
	}
	return max(1, min(20, rows))
}

func (m *dialogModel) listStart() int {
	return m.height - 3 - footerExtra(m.height) - m.optionRows()
}

func (m *dialogModel) footerY() int { return footerButtonY(m.height) }
func (m *dialogModel) View() string {
	m.resizeHits = nil
	if m.width == 0 {
		return "Loading…"
	}
	if m.width < 24 || m.height < 9 {
		return fit("Resize terminal · Esc back", m.width)
	}
	kind := "Menu"
	if m.singleChoice {
		kind = "Choose one"
	}
	header := m.breadcrumb
	if header == "" {
		header = m.title
	}
	if m.singleChoice {
		header += " — " + kind
	}
	if settingsSaveState != nil {
		header += " · " + settingsSaveState()
	}
	if m.saveHost && hostSaveState != nil {
		header += " · " + hostSaveState()
	}
	if m.saveHost && hostLocalSettingsState != nil {
		header += " · " + hostLocalSettingsState()
	}
	if m.saveTunnel && tunnelSaveState != nil {
		header += " · " + tunnelSaveState()
	}
	if m.saveMessage != "" {
		header += " · " + m.saveMessage
	}
	lines := []string{tuiAccent.Render(fit(header, m.width))}
	if m.swatch != "" {
		lines[0] += " " + colorSwatch(m.swatch)
	}
	if m.text != "" && m.width < 90 {
		lines = append(lines, m.preview.View(), "")
	}
	for lipgloss.Height(strings.Join(lines, "\n")) < m.listStart() {
		lines = append(lines, "")
	}
	var rows []string
	listWidth := m.width - 2
	if m.width >= 90 {
		listWidth, _ = m.menuPaneWidths()
	}
	for row, i := range m.visibleSlots() {
		if i < 0 {
			if len(m.filtered) == 0 && row == m.optionRows()-1 {
				rows = append(rows, tuiDim.Render("No matches"))
			} else {
				rows = append(rows, "")
			}
			continue
		}
		option := m.options[m.filtered[i]]
		value := clean(option.Label)
		if m.formRows() {
			value = m.formLabel(option, listWidth-2)
		}
		if m.sourceActions {
			value = m.sourceLabel(option, listWidth-2)
		}
		if m.singleChoice {
			marker := "○ "
			if option.ID == m.current {
				marker = "● "
			}
			value = marker + value
		} else if !m.formRows() && (strings.HasSuffix(value, "…") || strings.HasSuffix(value, "...") || strings.Contains(value, ": ") || strings.Contains(value, " — ")) {
			value += "  ›"
		}
		label := "  " + value
		if i == m.cursor {
			label = tuiSelected.Render(fit("› "+value, listWidth))
		}
		if m.colorChoices && hostHexColor.MatchString(option.ID) {
			// The radio marker is two cells; put the swatch after it, outside
			// the selected row's reverse video so its color remains visible.
			label = ansi.Cut(label, 0, 4) + colorSwatch(option.ID) + " " + ansi.Cut(label, 4, listWidth-2)
		}
		if option.ID == "color" && m.formRows() {
			_, hex, _ := strings.Cut(option.Label, ": ")
			if fields := strings.Fields(hex); len(fields) > 0 {
				hex = fields[0]
			}
			if hostHexColor.MatchString(hex) {
				x := 2 + m.formWidth(listWidth-2) + lipgloss.Width(menuColumnSeparator())
				label = ansi.Cut(label, 0, x) + colorSwatch(hex) + " " + ansi.Cut(label, x, listWidth-2)
			}
		}
		rows = append(rows, fit(label, listWidth))
		if m.sourceActions || m.formRows() {
			bounds := m.menuColumnBounds(listWidth - 2)
			for index, x := range bounds {
				m.resizeHits = append(m.resizeHits, hitRegion{x + 2, m.listStart() + row, 2, "column", index})
			}
		}
	}
	body := strings.Join(rows, "\n")
	if m.width >= 90 {
		panel := detailsPanel(m.preview)
		divider := strings.TrimSuffix(strings.Repeat(tuiDim.Render("│")+"\n", m.optionRows()), "\n")
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listWidth).Render(body), divider, panel)
		for row := 0; row < m.optionRows(); row++ {
			m.resizeHits = append(m.resizeHits, hitRegion{listWidth, m.listStart() + row, 1, "pane", 0})
		}
	}
	lines = append(lines, body)
	labels := []string{"Back"}
	hint := "Ctrl+H/← back · Ctrl+L/Enter select · ↑/↓ move · Ctrl+U/D details"
	if len(menuHotkeys.Hotkeys) > 0 {
		hint = keyHint(menuHotkeys, "back") + " back · " + keyHint(menuHotkeys, "select") + " select"
	}
	if m.title == "Settings" {
		labels = []string{"Back", "Save", "Cancel"}
		hint = "Ctrl+S save · Esc cancel · Ctrl+L/Enter open"
		if len(menuHotkeys.Hotkeys) > 0 {
			hint = keyHint(menuHotkeys, "menu_save") + " save · " + keyHint(menuHotkeys, "back") + " back"
		}
	}
	if saveActiveSettings != nil && m.title != "Settings" {
		labels = []string{"Back", "Save settings"}
		hint = keyHint(menuHotkeys, "menu_save") + " save · " + keyHint(menuHotkeys, "back") + " back"
	}
	if m.saveHost {
		labels = []string{"Back", "Save host"}
		hint = keyHint(menuHotkeys, "menu_save") + " save · " + keyHint(menuHotkeys, "back") + " back"
	}
	if m.saveTunnel {
		labels = []string{"Back", "Save tunnel"}
		hint = keyHint(menuHotkeys, "menu_save") + " save | " + keyHint(menuHotkeys, "back") + " back"
	}
	if m.deleteHost {
		labels = append(labels, "Delete host")
		hint += " · " + keyHint(menuHotkeys, "delete_host") + " delete"
	}
	if m.saveHost && saveHostLocalSettings != nil {
		labels = append(labels, "Save local settings")
		hint += " | ctrl+alt+s Local save"
	}
	if m.sourceActions {
		labels = []string{"Back", "Create source", "Add existing"}
		if len(m.filtered) > 0 {
			id := m.options[m.filtered[m.cursor]].ID
			if id != "" && id != "ssh-config" {
				labels = append(labels, "Remove source")
			}
		}
		hint = keyHint(menuHotkeys, "back") + " Back | " + keyHint(menuHotkeys, "source_create") + " Create | " + keyHint(menuHotkeys, "source_attach") + " Add existing"
		if len(labels) > 3 {
			hint += " | " + keyHint(menuHotkeys, "source_remove") + " Remove"
		}
		labels = append(labels, "Settings")
		hint += " | " + keyHint(menuHotkeys, "source_settings") + " Settings"
	}
	if len(m.footerActions) > 0 {
		labels = []string{"Back"}
		for _, a := range m.footerActions {
			labels = append(labels, a.Label)
		}
		hint = keyHint(menuHotkeys, "back") + " back | " + keyHint(menuHotkeys, "select") + " select"
		for _, a := range m.footerActions {
			if a.ID == "save-host" || a.ID == "save" {
				hint += " | " + keyHint(menuHotkeys, "menu_save") + " " + a.Label
				break
			}
		}
	}
	if m.layout != (menuLayout{}) {
		x := 1
		for _, label := range labels {
			x += lipgloss.Width(label) + 3
		}
		m.resizeHits = append(m.resizeHits, hitRegion{x - 1, m.footerY(), 14, "reset", 0})
		labels = append(labels, "Reset layout")
	}
	hint = strings.ReplaceAll(hint, " · ", " | ")
	lines = append(lines, strings.Repeat("─", m.width), m.query.View(), menuFooter(m.height, m.width, labels, hint))
	return strings.Join(lines, "\n")
}

func menuExplanation(text string) string {
	// Separate prose sentences; preserve explicitly authored lists and paragraphs.
	if !strings.Contains(text, "\n") {
		text = strings.ReplaceAll(text, ". ", ".\n\n")
	}
	return cleanLines(text)
}

func cleanLines(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = clean(lines[i])
	}
	return strings.Join(lines, "\n")
}

type pathModel struct {
	label                 string
	input                 textinput.Model
	local                 bool
	items                 []string
	cursor, width, height int
	done, back            bool
	err                   error
}

func newPathModel(label, value string, local bool) *pathModel {
	input := textinput.New()
	input.Prompt = "› "
	input.PromptStyle = tuiAccent
	input.SetValue(value)
	input.Focus()
	m := &pathModel{label: label, input: input, local: local}
	m.complete()
	return m
}
func (m *pathModel) complete() {
	if m.local {
		m.items, m.err = completeLocal(m.input.Value())
	}
	m.cursor = min(max(0, len(m.items)-1), max(0, m.cursor))
}
func (m *pathModel) Init() tea.Cmd { return textinput.Blink }
func (m *pathModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = max(1, msg.Width-6)
	case tea.KeyMsg:
		if m.width > 0 && (m.width < 24 || m.height < 9) && msg.String() != "esc" && msg.String() != "ctrl+c" {
			return m, nil
		}
		switch msg.String() {
		case "esc", "ctrl+c":
			m.done, m.back = true, true
			return m, tea.Quit
		case "enter":
			m.done = true
			return m, tea.Quit
		case "down", "ctrl+j":
			m.cursor = cycleCursor(m.cursor, 1, len(m.items))
			return m, nil
		case "up", "ctrl+k":
			m.cursor = cycleCursor(m.cursor, -1, len(m.items))
			return m, nil
		case "tab":
			if len(m.items) > 0 {
				m.input.SetValue(m.items[m.cursor])
				m.input.CursorEnd()
				m.cursor = 0
				m.complete()
			}
			return m, nil
		}
	case tea.MouseMsg:
		if m.width < 24 || m.height < 9 {
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelUp {
			m.cursor = max(0, m.cursor-3)
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			m.cursor = min(max(0, len(m.items)-1), m.cursor+3)
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			if msg.Y == footerButtonY(m.height) && msg.X >= 0 && msg.X < 25 {
				m.done = true
				m.back = msg.X >= 17
				return m, tea.Quit
			}
			i := msg.Y - 1 + max(0, m.cursor-m.inputRows()+1)
			if msg.Y >= 1 && msg.Y < 1+m.inputRows() && i >= 0 && i < len(m.items) {
				m.input.SetValue(m.items[i])
				m.input.CursorEnd()
				m.cursor = 0
				m.complete()
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.cursor = 0
	m.complete()
	return m, cmd
}
func (m *pathModel) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	if m.width < 24 || m.height < 9 {
		return fit("Resize terminal · Esc back", m.width)
	}
	header := "Edit value"
	if len(menuTrail) > 0 {
		header = strings.Join(menuTrail, " › ") + " › " + m.label
	}
	lines := []string{tuiAccent.Render(fit(header, m.width))}
	rows := m.inputRows()
	offset := max(0, m.cursor-rows+1)
	for row := 0; row < rows; row++ {
		i := offset + row
		if i >= len(m.items) {
			lines = append(lines, "")
			continue
		}
		line := fit("  "+m.items[i], m.width)
		if i == m.cursor {
			line = tuiSelected.Render(fit("› "+m.items[i], m.width))
		}
		lines = append(lines, line)
	}
	if m.err != nil {
		lines = append(lines, tuiError.Render(fit(fmt.Sprint(m.err), m.width)))
	} else {
		lines = append(lines, "")
	}
	lines = append(lines, strings.Repeat("─", m.width), tuiAccent.Render(fit(m.label, m.width)), m.input.View(), menuFooter(m.height, m.width, []string{"Use typed path", "Back"}, "Enter accept · Tab complete · Esc back"))
	return strings.Join(lines, "\n")
}

func (m *pathModel) inputRows() int { return max(1, m.height-6-footerExtra(m.height)) }
