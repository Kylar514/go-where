package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *pickerModel) paneWidths() (int, int) {
	if m.width < 90 || m.state.ListPercent == 0 {
		return viewSplit(m.width)
	}
	total := m.width - 2
	list := min(total-25, max(40, total*m.state.ListPercent/100))
	return list, max(18, total-list-5)
}

func (m *pickerModel) columnWidths(width int) []int {
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
	if len(m.state.ColumnWidths) >= len(widths) {
		budget := 0
		for _, w := range widths {
			budget += w
		}
		remaining := budget
		for i := 0; i < len(widths)-1; i++ {
			widths[i] = min(max(4, m.state.ColumnWidths[i]), max(4, remaining-4*(len(widths)-i-1)))
			remaining -= widths[i]
		}
		widths[len(widths)-1] = max(1, remaining)
	}
	return widths
}

func (m *pickerModel) resetLayout() {
	m.state.ListPercent = 0
	m.state.ColumnWidths = nil
	m.dragKind = ""
	m.lastClick = ""
	_, m.preview.Width = m.paneWidths()
	m.updatePreview()
}

func (m *pickerModel) resizeMouse(msg tea.MouseMsg) bool {
	if m.dragKind != "" {
		if msg.Action == tea.MouseActionRelease {
			m.dragKind = ""
			return true
		}
		if msg.Action != tea.MouseActionMotion {
			return true
		}
		if m.dragKind == "resize-pane" {
			total := m.width - 2
			list := min(total-25, max(40, msg.X))
			m.state.ListPercent = (list*100 + total/2) / total
		} else {
			i := m.dragColumn
			delta := min(m.dragWidths[i+1]-4, max(4-m.dragWidths[i], msg.X-m.dragX))
			m.state.ColumnWidths = append([]int(nil), m.dragWidths...)
			m.state.ColumnWidths[i] += delta
			m.state.ColumnWidths[i+1] -= delta
		}
		_, m.preview.Width = m.paneWidths()
		m.preview.Height = max(1, m.rows()-1)
		m.scrollToCursor()
		m.lastClick = ""
		m.updatePreview()
		return true
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false
	}
	for _, hit := range m.hits {
		if (hit.action == "resize-pane" || hit.action == "resize-column") && msg.Y == hit.y && msg.X >= hit.x && msg.X < hit.x+hit.width {
			m.dragKind, m.dragColumn, m.dragX = hit.action, hit.index, msg.X
			width, _ := m.paneWidths()
			m.dragWidths = m.columnWidths(width)
			m.lastClick = ""
			return true
		}
	}
	return false
}
