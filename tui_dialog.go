package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

var runTUI = func(m tea.Model) (tea.Model, error) {
	return tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
}

var (
	tuiAccent   = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
	tuiDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	tuiSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24"))
	tuiError    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

func fit(s string, width int) string { return ansi.Truncate(s, max(0, width), "…") }

type dialogModel struct {
	title         string
	options       []choice
	filtered      []int
	cursor        int
	query         textinput.Model
	preview       viewport.Model
	text          string
	width, height int
	result        string
	done, back    bool
}

func newDialog(title string, options []choice, text string) *dialogModel {
	input := textinput.New()
	input.Prompt = "/ "
	input.Focus()
	m := &dialogModel{title: title, options: options, text: text, query: input, preview: viewport.New(1, 1)}
	m.refilter()
	return m
}

func (m *dialogModel) refilter() {
	m.filtered = nil
	if m.query.Value() == "" {
		for i := range m.options {
			m.filtered = append(m.filtered, i)
		}
	} else {
		labels := make([]string, len(m.options))
		for i, o := range m.options {
			labels[i] = o.Label
		}
		for _, match := range fuzzy.Find(m.query.Value(), labels) {
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
		m.query.Width = max(1, msg.Width-6)
		m.preview.Width = max(1, msg.Width-4)
		content:=ansi.Hardwrap(cleanLines(m.text),m.preview.Width,true)
		m.preview.Height=min(lipgloss.Height(content),max(1,min(msg.Height/2,msg.Height-8)))
		m.preview.SetContent(content)
	case tea.KeyMsg:
		if m.width > 0 && (m.width < 24 || m.height < 9) && msg.String() != "esc" && msg.String() != "ctrl+c" {
			return m, nil
		}
		switch msg.String() {
		case "esc", "ctrl+c":
			m.done, m.back = true, true
			return m, tea.Quit
		case "up", "ctrl+k":
			m.cursor = max(0, m.cursor-1)
			return m, nil
		case "down", "ctrl+j":
			m.cursor = min(max(0, len(m.filtered)-1), m.cursor+1)
			return m, nil
		case "ctrl+u":
			m.preview.HalfPageUp()
			return m, nil
		case "ctrl+d":
			m.preview.HalfPageDown()
			return m, nil
		case "enter":
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
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.height-1 && msg.X < 8 {
			m.done, m.back = true, true
			return m, tea.Quit
		}
		if msg.Button == tea.MouseButtonWheelUp {
			start := 4
			if m.text != "" {
				start += m.preview.Height + 1
			}
			if msg.Y >= start {
				m.cursor = max(0, m.cursor-3)
				return m, nil
			}
			m.preview.ScrollUp(3)
			return m, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			start := 4
			if m.text != "" {
				start += m.preview.Height + 1
			}
			if msg.Y >= start {
				m.cursor = min(max(0, len(m.filtered)-1), m.cursor+3)
				return m, nil
			}
			m.preview.ScrollDown(3)
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
			start := 4
			if m.text != "" {
				start += m.preview.Height + 1
			}
			i := msg.Y - start
			offset := max(0, m.cursor-m.optionRows()+1)
			if i >= 0 && i < m.optionRows() && i+offset < len(m.filtered) {
				m.cursor = i + offset
				m.result = m.options[m.filtered[m.cursor]].ID
				m.done = true
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	m.refilter()
	return m, cmd
}
func (m *dialogModel) optionRows() int {
	rows := m.height - 7
	if m.text != "" {
		rows -= m.preview.Height + 1
	}
	return max(1, rows)
}
func (m *dialogModel) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	if m.width < 24 || m.height < 9 {
		return fit("Resize terminal · Esc back", m.width)
	}
	lines := []string{tuiAccent.Render(fit(m.title, m.width-2)), tuiDim.Render(fit("Enter / click select    Esc back    Ctrl+U/D scroll details", m.width-2)), "", m.query.View()}
	if m.text != "" {
		lines = append(lines, m.preview.View(), "")
	}
	offset := max(0, m.cursor-m.optionRows()+1)
	for i := offset; i < min(len(m.filtered), offset+m.optionRows()); i++ {
		label := "  " + clean(m.options[m.filtered[i]].Label)
		if i == m.cursor {
			label = tuiSelected.Render(fit("› "+clean(m.options[m.filtered[i]].Label), m.width-2))
		}
		lines = append(lines, fit(label, m.width-2))
	}
	if len(m.filtered) == 0 {
		lines = append(lines, tuiDim.Render("No matches"))
	}
	for used := lipgloss.Height(strings.Join(lines, "\n")); used < m.height-1; used++ {
		lines = append(lines, "")
	}
	lines = append(lines, tuiAccent.Render("[ Back ]"))
	return strings.Join(lines, "\n")
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
			m.cursor = min(max(0, len(m.items)-1), m.cursor+1)
			return m, nil
		case "up", "ctrl+k":
			m.cursor = max(0, m.cursor-1)
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
			if msg.Y == m.height-2 && (msg.X < 18 || msg.X >= 20 && msg.X < 28) {
				m.done = true
				m.back = msg.X >= 20
				return m, tea.Quit
			}
			i := msg.Y - 5 + max(0, m.cursor-max(1, m.height-8)+1)
			if i >= 0 && i < len(m.items) {
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
	lines := []string{tuiAccent.Render(fit(m.label, m.width)), fit("Enter accepts typed path · Tab completes · Esc back", m.width), "", m.input.View(), ""}
	rows := max(1, m.height-8)
	offset := max(0, m.cursor-rows+1)
	for i := offset; i < min(len(m.items), offset+rows); i++ {
		line := fit("  "+m.items[i], m.width)
		if i == m.cursor {
			line = tuiSelected.Render(fit("› "+m.items[i], m.width))
		}
		lines = append(lines, line)
	}
	if m.err != nil {
		lines = append(lines, tuiError.Render(fit(fmt.Sprint(m.err), m.width)))
	}
	for used := lipgloss.Height(strings.Join(lines, "\n")); used < m.height-2; used++ {
		lines = append(lines, "")
	}
	lines = append(lines, fit(tuiAccent.Render("[ Use typed path ]  [ Back ]"), m.width))
	return strings.Join(lines, "\n")
}
