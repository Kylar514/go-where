package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

type menuLayout struct{ listPercent, labelWidth, typeWidth int }

func (m *dialogModel) menuPaneWidths() (int, int) {
	if m.width < 90 || m.layout.listPercent == 0 {
		return viewSplit(m.width)
	}
	total := m.width - 2
	list := min(total-25, max(30, total*m.layout.listPercent/100))
	return list, max(18, total-list-5)
}

func (m *dialogModel) menuColumnBounds(width int) []int {
	if m.sourceActions {
		a, b := m.sourceWidths(width)
		return []int{a, a + 4 + b}
	}
	return []int{m.formWidth(width)}
}

func (m *dialogModel) refreshMenuLayout() {
	offset := m.preview.YOffset
	m.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.preview.SetYOffset(offset)
}

func (m *dialogModel) resetMenuLayout() {
	m.layout = menuLayout{}
	if m.readingScreen {
		m.layout.listPercent = 20
	}
	m.resizeKind = ""
	m.refreshMenuLayout()
}

func (m *dialogModel) resizeMenuMouse(msg tea.MouseMsg) bool {
	if m.resizeKind != "" {
		if msg.Action == tea.MouseActionRelease {
			m.resizeKind = ""
			return true
		}
		if msg.Action != tea.MouseActionMotion {
			return true
		}
		width := m.width - 2
		if m.width >= 90 {
			width, _ = m.menuPaneWidths()
		}
		width -= 2
		switch m.resizeKind {
		case "pane":
			total := m.width - 2
			list := min(total-25, max(30, msg.X))
			m.layout.listPercent = (list*100 + total/2) / total
		case "column":
			m.layout.labelWidth = min(max(4, msg.X-2), max(4, width-12))
			if m.palette {
				m.layout.labelWidth = min(max(4, msg.X-4), max(4, width-14))
			}
		case "type":
			a, _ := m.sourceWidths(width)
			if m.palette {
				a = m.paletteWidths(width - 2)[0]
			}
			m.layout.typeWidth = min(max(4, msg.X-2-a-4), max(4, width-a-12))
			if m.palette {
				m.layout.typeWidth = min(max(4, msg.X-4-a-4), max(4, width-a-14))
			}
		}
		m.refreshMenuLayout()
		return true
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return false
	}
	for _, hit := range m.resizeHits {
		if msg.Y == hit.y && msg.X >= hit.x && msg.X < hit.x+hit.width {
			if hit.action == "palette-sort" {
				selected := ""
				if len(m.filtered) > 0 {
					selected = m.options[m.filtered[m.cursor]].ID
				}
				if m.paletteSort.Column == hit.index {
					m.paletteSort.Descending = !m.paletteSort.Descending
				} else {
					m.paletteSort = pickerSort{Column: hit.index}
				}
				m.sortPalette()
				for i, index := range m.filtered {
					if m.options[index].ID == selected {
						m.cursor = i
						break
					}
				}
				return true
			}
			if hit.action == "reset" {
				m.resetMenuLayout()
				return true
			}
			m.resizeKind = hit.action
			if hit.action == "column" && hit.index == 1 {
				m.resizeKind = "type"
			}
			return true
		}
	}
	return false
}

// Keep a visible separator in the existing four-cell gutter, without widening rows.
func menuColumnSeparator() string { return " │  " }
