package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// tmux supplies a borderless terminal. Drawing the frame here keeps its
// background identical to the content, including terminal transparency.
type popupFrame struct {
	model         tea.Model
	width, height int
}

func (m *popupFrame) Init() tea.Cmd { return m.model.Init() }

func (m *popupFrame) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch event := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = event.Width, event.Height
		msg = tea.WindowSizeMsg{Width: max(1, event.Width-2), Height: max(1, event.Height-2)}
	case tea.MouseMsg:
		if event.X < 1 || event.Y < 1 || event.X >= m.width-1 || event.Y >= m.height-1 {
			return m, nil
		}
		event.X--
		event.Y--
		msg = event
	}
	var cmd tea.Cmd
	m.model, cmd = m.model.Update(msg)
	return m, cmd
}

func (m *popupFrame) View() string {
	if m.width < 3 || m.height < 3 {
		return ""
	}
	w, h := m.width-2, m.height-2
	lines := strings.Split(m.model.View(), "\n")
	top := "╭" + ansi.Truncate("─ go-where "+strings.Repeat("─", w), w, "") + "╮"
	result := []string{tuiDim.Render(top)}
	for i := 0; i < h; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], w, "")
		}
		result = append(result, tuiDim.Render("│")+line+strings.Repeat(" ", max(0, w-lipgloss.Width(line)))+tuiDim.Render("│"))
	}
	result = append(result, tuiDim.Render("╰"+strings.Repeat("─", w)+"╯"))
	return strings.Join(result, "\n")
}
