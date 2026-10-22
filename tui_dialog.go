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
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.query.Width = max(1, msg.Width-ansi.StringWidth(m.query.Prompt)-1)
		m.preview.Width = max(1, min(64, msg.Width-4))
		if msg.Width >= 90 {
			_, pw := viewSplit(msg.Width)
			m.preview.Width = pw
		}
		content := ansi.Wrap(menuExplanation(m.text), m.preview.Width, "")
		m.preview.Height = min(lipgloss.Height(content), max(1, (msg.Height-7)/2))
		if msg.Width >= 90 {
			m.preview.Height = max(1, min(20, msg.Height-5)-2)
		}
		m.preview.SetContent(content)
		if m.restorePreview != nil {
			m.preview.SetYOffset(*m.restorePreview)
			m.restorePreview = nil
		}
	case tea.KeyMsg:
		if m.width > 0 && (m.width < 24 || m.height < 9) && msg.String() != "esc" && msg.String() != "ctrl+c" && msg.String() != "ctrl+h" && msg.String() != "left" {
			return m, nil
		}
		switch msg.String() {
		case "ctrl+s":
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
	case tea.MouseMsg:
		if m.width < 24 || m.height < 9 {
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.height-1 && msg.X >= 0 && msg.X < 8 {
			m.done, m.back = true, true
			return m, tea.Quit
		}
		if m.title == "Settings" && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.height-1 {
			if msg.X >= 8 && msg.X < 15 {
				m.result, m.done = "save", true
				return m, tea.Quit
			}
			if msg.X >= 15 && msg.X < 24 {
				m.result, m.done = "cancel", true
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
			offset := max(0, m.cursor-m.optionRows()+1)
			i := offset + m.optionRows() - 1 - row
			if row >= 0 && row < m.optionRows() && i < len(m.filtered) && m.inList(msg.X) {
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
		width, _ = viewSplit(m.width)
	}
	return x >= 0 && x < width
}
func (m *dialogModel) optionRows() int {
	rows := m.height - 5
	if m.text != "" && m.width < 90 {
		rows -= m.preview.Height + 1
	}
	return max(1, min(20, rows))
}

func (m *dialogModel) listStart() int {
	return m.height - 3 - m.optionRows()
}
func (m *dialogModel) View() string {
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
	lines := []string{tuiAccent.Render(fit(header, m.width))}
	if m.text != "" && m.width < 90 {
		lines = append(lines, m.preview.View(), "")
	}
	for lipgloss.Height(strings.Join(lines, "\n")) < m.listStart() {
		lines = append(lines, "")
	}
	offset := max(0, m.cursor-m.optionRows()+1)
	var rows []string
	listWidth := m.width - 2
	if m.width >= 90 {
		listWidth, _ = viewSplit(m.width)
	}
	for row := m.optionRows() - 1; row >= 0; row-- {
		i := offset + row
		if i >= len(m.filtered) {
			if len(m.filtered) == 0 && row == 0 {
				rows = append(rows, tuiDim.Render("No matches"))
			} else {
				rows = append(rows, "")
			}
			continue
		}
		option := m.options[m.filtered[i]]
		value := clean(option.Label)
		if m.singleChoice {
			marker := "○ "
			if option.ID == m.current {
				marker = "● "
			}
			value = marker + value
		} else if strings.HasSuffix(value, "…") || strings.HasSuffix(value, "...") || strings.Contains(value, ": ") || strings.Contains(value, " — ") {
			value += "  ›"
		}
		label := "  " + value
		if i == m.cursor {
			label = tuiSelected.Render(fit("› "+value, listWidth))
		}
		rows = append(rows, fit(label, listWidth))
	}
	body := strings.Join(rows, "\n")
	if m.width >= 90 {
		panel := detailsPanel(m.preview)
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(listWidth).Render(body), " ", panel)
	}
	lines = append(lines, body)
	footer := compactControl("Back") + "  Ctrl+H/← back · Ctrl+L/Enter select · ↑/↓ move · Ctrl+U/D details"
	if m.title == "Settings" {
		footer = compactControl("Back", "Save", "Cancel") + "  Ctrl+S save · Esc cancel · Ctrl+L/Enter open"
	}
	lines = append(lines, strings.Repeat("─", m.width), m.query.View(), fit(footer, m.width))
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
			if msg.Y == m.height-1 && msg.X >= 0 && msg.X < 25 {
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
	lines = append(lines, strings.Repeat("─", m.width), tuiAccent.Render(fit(m.label, m.width)), m.input.View(), fit(compactControl("Use typed path", "Back")+"  Enter accept · Tab complete · Esc back", m.width))
	return strings.Join(lines, "\n")
}

func (m *pathModel) inputRows() int { return max(1, m.height-6) }
