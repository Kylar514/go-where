package main

import (
	"sort"
	"strings"
)

type pickerSort struct {
	Column     int
	Descending bool
}

// Narrow tunnel layouts hide columns, rather than changing their identity.
func (m *pickerModel) sortColumn(column, count int) int {
	if m.state.Mode == "tunnels" {
		if count == 4 {
			return []int{0, 3, 4, 5}[column]
		}
		if count == 2 {
			return []int{0, 5}[column]
		}
	}
	return column
}

func (m *pickerModel) sortHeading(values []string, widths []int) {
	s, ok := m.state.Sorts[m.state.Mode]
	if !ok {
		return
	}
	for i, w := range widths {
		if m.sortColumn(i, len(widths)) == s.Column {
			arrow := " ▲"
			if s.Descending {
				arrow = " ▼"
			}
			padding := 0
			if m.state.Mode != "tunnels" && i == 0 {
				padding = 2
			}
			values[i] = fit(values[i], max(0, w-padding-2)) + arrow
		}
	}
}

func (m *pickerModel) sortVisible() {
	s, ok := m.state.Sorts[m.state.Mode]
	if !ok || (m.state.Mode != "ssh" && m.state.Mode != "rdp" && m.state.Mode != "tunnels") {
		return
	}
	values := make(map[int]string, len(m.visible))
	selected := -1
	if m.cursor >= 0 && m.cursor < len(m.visible) {
		selected = m.visible[m.cursor]
	}
	for _, id := range m.visible {
		var row []string
		if m.state.Mode == "tunnels" {
			row = m.tunnelColumnValues(id)
		} else {
			row = hostColumnValues(&m.hosts[id])
		}
		if s.Column < 0 || s.Column >= len(row) {
			return
		}
		values[id] = strings.ToLower(clean(row[s.Column]))
		if m.state.Mode == "tunnels" && s.Column == 5 {
			_, values[id], _ = strings.Cut(values[id], " ")
		}
	}
	sort.SliceStable(m.visible, func(i, j int) bool {
		a, b := m.visible[i], m.visible[j]
		if values[a] == values[b] {
			return a < b
		}
		if s.Descending {
			return values[a] > values[b]
		}
		return values[a] < values[b]
	})
	for i, id := range m.visible {
		if id == selected {
			m.cursor = i
			break
		}
	}
}

func (m *pickerModel) toggleSort(column int) {
	if m.state.Sorts == nil {
		m.state.Sorts = map[string]pickerSort{}
	}
	s, ok := m.state.Sorts[m.state.Mode]
	if ok && s.Column == column {
		s.Descending = !s.Descending
	} else {
		s = pickerSort{Column: column}
	}
	m.state.Sorts[m.state.Mode] = s
	m.sortVisible()
	m.scrollToCursor()
	m.updatePreview()
}
