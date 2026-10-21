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

type multiItem struct {
	ID, Label, Note string
	Checked         bool
}

// multiDialog is a searchable checklist. Two pinned rows sit under the list so
// "Done" and "Type a name manually" stay reachable however long it is.
type multiDialog struct {
	title         string
	items         []multiItem
	filtered      []int
	cursor        int // over filtered items, then Done, then manual entry
	query         textinput.Model
	preview       viewport.Model
	text          string
	width, height int
	done, back    bool
	action        string // "done" or "manual"
}

func newMultiDialog(title string, items []multiItem, text string) *multiDialog {
	input := textinput.New()
	input.Prompt = "/ "
	input.Focus()
	m := &multiDialog{title: title, items: append([]multiItem(nil), items...), text: text, query: input, preview: viewport.New(1, 1)}
	m.refilter()
	return m
}

func (m *multiDialog) refilter() {
	m.filtered = nil
	if m.query.Value() == "" {
		for i := range m.items {
			m.filtered = append(m.filtered, i)
		}
	} else {
		labels := make([]string, len(m.items))
		for i, item := range m.items {
			labels[i] = item.Label
		}
		for _, match := range fuzzy.Find(m.query.Value(), labels) {
			m.filtered = append(m.filtered, match.Index)
		}
	}
	m.cursor = min(max(0, m.cursor), len(m.filtered)+1)
}

func (m *multiDialog) toggleID(id string) bool {
	for i := range m.items {
		if m.items[i].ID == id {
			m.items[i].Checked = !m.items[i].Checked
			return true
		}
	}
	return false
}

func (m *multiDialog) checkedCount() int {
	n := 0
	for _, item := range m.items {
		if item.Checked {
			n++
		}
	}
	return n
}

func (m *multiDialog) activate(i int) tea.Cmd {
	if i < len(m.filtered) {
		m.items[m.filtered[i]].Checked = !m.items[m.filtered[i]].Checked
		return nil
	}
	m.action = "done"
	if i > len(m.filtered) {
		m.action = "manual"
	}
	m.done = true
	return tea.Quit
}

func (m *multiDialog) tooSmall() bool { return m.width < 24 || m.height < 10 }

func (m *multiDialog) Init() tea.Cmd { return textinput.Blink }

func (m *multiDialog) listStart() int {
	if m.text != "" {
		return 4 + m.preview.Height + 1
	}
	return 4
}

func (m *multiDialog) itemRows() int {
	rows := m.height - 7
	if m.text != "" {
		rows -= m.preview.Height + 1
	}
	return max(1, rows)
}

func (m *multiDialog) offset() int {
	return max(0, min(m.cursor, len(m.filtered)-1)-m.itemRows()+1)
}

func (m *multiDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.query.Width = max(1, msg.Width-6)
		m.preview.Width = max(1, msg.Width-4)
		content := ansi.Hardwrap(cleanLines(m.text), m.preview.Width, true)
		m.preview.Height = min(lipgloss.Height(content), max(1, (msg.Height-8)/2))
		m.preview.SetContent(content)
	case tea.KeyMsg:
		if m.width > 0 && m.tooSmall() && msg.String() != "esc" && msg.String() != "ctrl+c" {
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
			m.cursor = min(len(m.filtered)+1, m.cursor+1)
			return m, nil
		case " ", "enter":
			return m, m.activate(m.cursor)
		case "ctrl+u":
			m.preview.HalfPageUp()
			return m, nil
		case "ctrl+d":
			m.preview.HalfPageDown()
			return m, nil
		}
	case tea.MouseMsg:
		if m.tooSmall() {
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == m.height-1 && msg.X < 8 {
			m.done, m.back = true, true
			return m, tea.Quit
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			delta := 3
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -3
			}
			if msg.Y >= m.listStart() {
				m.cursor = min(len(m.filtered)+1, max(0, m.cursor+delta))
			} else if delta < 0 {
				m.preview.ScrollUp(3)
			} else {
				m.preview.ScrollDown(3)
			}
			return m, nil
		case tea.MouseButtonLeft:
			if msg.Action != tea.MouseActionPress {
				return m, nil
			}
			row := msg.Y - m.listStart()
			switch {
			case row >= 0 && row < m.itemRows() && m.offset()+row < len(m.filtered):
				m.cursor = m.offset() + row
				return m, m.activate(m.cursor)
			case row == m.itemRows():
				m.cursor = len(m.filtered)
				return m, m.activate(m.cursor)
			case row == m.itemRows()+1:
				m.cursor = len(m.filtered) + 1
				return m, m.activate(m.cursor)
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.query, cmd = m.query.Update(msg)
	m.refilter()
	return m, cmd
}

func (m *multiDialog) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	if m.tooSmall() {
		return fit("Resize terminal · Esc back", m.width)
	}
	w := m.width - 2
	lines := []string{tuiAccent.Render(fit(m.title, w)), tuiDim.Render(fit("Space / Enter / click toggle    Esc discard    Ctrl+U/D scroll details", w)), "", m.query.View()}
	if m.text != "" {
		lines = append(lines, m.preview.View(), "")
	}
	row := func(index int, plain, styled string) string {
		if index == m.cursor {
			return tuiSelected.Render(fit("› "+plain, w))
		}
		return fit("  "+styled, w)
	}
	rows, offset := m.itemRows(), m.offset()
	shown := 0
	for i := offset; i < min(len(m.filtered), offset+rows); i++ {
		item := m.items[m.filtered[i]]
		box := "[ ]"
		if item.Checked {
			box = "[✓]"
		}
		plain := box + " " + clean(item.Label)
		styled := plain
		if item.Note != "" {
			plain += "  — " + clean(item.Note)
			styled += tuiDim.Render("  — " + clean(item.Note))
		}
		lines = append(lines, row(i, plain, styled))
		shown++
	}
	if len(m.filtered) == 0 {
		lines = append(lines, tuiDim.Render("No matches"))
		shown++
	}
	for ; shown < rows; shown++ {
		lines = append(lines, "")
	}
	done := fmt.Sprintf("[ Done — apply %d selected ]", m.checkedCount())
	manual := "[ Type a name manually… ]"
	lines = append(lines, row(len(m.filtered), done, tuiAccent.Render(done)), row(len(m.filtered)+1, manual, tuiAccent.Render(manual)))
	for used := lipgloss.Height(strings.Join(lines, "\n")); used < m.height-1; used++ {
		lines = append(lines, "")
	}
	lines = append(lines, tuiAccent.Render("[ Back ]"))
	return strings.Join(lines, "\n")
}
