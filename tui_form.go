package main

import (
	"github.com/charmbracelet/lipgloss"
	"strings"
)

func (m *dialogModel) formRows() bool {
	if m.singleChoice || m.colorChoices {
		return false
	}
	if m.title == "Settings" {
		return true
	}
	for _, option := range m.options {
		if strings.Contains(option.Label, ": ") || strings.Contains(option.Label, " — ") || strings.Contains(option.Label, " – ") {
			return true
		}
	}
	return false
}

func (m *dialogModel) formLabel(option choice, width int) string {
	label, value, hasValue := strings.Cut(option.Label, ": ")
	if dash := strings.Index(option.Label, " — "); dash >= 0 && (!hasValue || dash < strings.Index(option.Label, ": ")) {
		label, value, hasValue = strings.Cut(option.Label, " — ")
	}
	if !hasValue {
		label, value, hasValue = strings.Cut(option.Label, " — ")
	}
	if !hasValue {
		label, value, hasValue = strings.Cut(option.Label, " – ")
	}
	label = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(label), "…"), "...")
	if !hasValue {
		value = ""
	}
	value = strings.TrimSpace(strings.TrimSuffix(value, "…"))
	if hasValue && value == "" {
		value = "—"
	}
	labelWidth := m.formWidth(width)
	label = fit(clean(label), labelWidth)
	return label + strings.Repeat(" ", max(0, labelWidth-lipgloss.Width(label))) + menuColumnSeparator() + fit(clean(value), max(0, width-labelWidth-6)) + " ›"
}

func (m *dialogModel) formWidth(width int) int {
	labelWidth := 0
	for _, o := range m.options {
		l, _, ok := strings.Cut(o.Label, ": ")
		if !ok {
			l, _, _ = strings.Cut(o.Label, " — ")
		}
		l, _, _ = strings.Cut(l, " – ")
		l = strings.TrimSuffix(l, "…")
		labelWidth = max(labelWidth, lipgloss.Width(l))
	}
	labelWidth = min(26, min(labelWidth, max(8, width-18)))
	if m.layout.labelWidth > 0 {
		labelWidth = min(m.layout.labelWidth, max(4, width-12))
	}
	return labelWidth
}

func (m *dialogModel) sourceLabel(option choice, width int) string {
	name, kind, path := option.Label, "Combined", "—"
	if option.ID == "ssh-config" {
		kind, path = "OpenSSH", "~/.ssh/config"
	} else if option.ID != "" {
		var ok bool
		name, path, ok = strings.Cut(option.Label, " — SQLite: ")
		if !ok {
			name, path, _ = strings.Cut(option.Label, " – SQLite: ")
		}
		kind = "SQLite"
	}
	nameWidth, kindWidth := m.sourceWidths(width)
	cell := func(value string, w int) string {
		value = fit(clean(value), w)
		return value + strings.Repeat(" ", max(0, w-lipgloss.Width(value)))
	}
	return fit(cell(name, nameWidth)+menuColumnSeparator()+cell(kind, kindWidth)+menuColumnSeparator()+path, width)
}

func (m *dialogModel) sourceWidths(width int) (int, int) {
	a := min(20, max(8, width/3))
	b := min(9, max(5, width/4))
	if m.layout.labelWidth > 0 {
		a = min(m.layout.labelWidth, max(4, width-16))
	}
	if m.layout.typeWidth > 0 {
		b = min(m.layout.typeWidth, max(4, width-a-12))
	}
	return a, b
}

func (m *dialogModel) anchorColumns(values []string, width int) string {
	widths := []int{10, 16, 10, 8}
	for _, row := range m.columnRows {
		for i, v := range row {
			widths[i] = max(widths[i], lipgloss.Width(v))
		}
	}
	widths[0] = min(widths[0], max(4, width/4))
	widths[1] = min(widths[1], max(5, width/3))
	widths[2] = min(widths[2], max(4, width/5))
	widths[3] = max(0, width-widths[0]-widths[1]-widths[2]-6)
	var cells []string
	for i, v := range values {
		v = fit(clean(v), widths[i])
		cells = append(cells, v+strings.Repeat(" ", max(0, widths[i]-lipgloss.Width(v))))
	}
	return strings.Join(cells, " │ ")
}

func hostFormGroup(id string) int {
	if strings.HasPrefix(id, "ssh-") {
		return 1
	}
	if strings.HasPrefix(id, "rdp-") {
		return 2
	}
	switch id {
	case "alias", "address", "tags", "notes":
		return 0
	case "ssh", "rdp", "tunnels":
		return 1
	default:
		return 2
	}
}

// Visual slots may include gaps; filtered indices remain the selectable rows.
func (m *dialogModel) visualSlots() []int {
	var slots []int
	lastGroup := -1
	for i := range m.filtered {
		group := 0
		if m.optionGroups != nil && m.query.Value() == "" {
			group = m.optionGroups[m.options[m.filtered[i]].ID]
		}
		if m.colorChoices && m.query.Value() == "" {
			if i >= 2 {
				group = 1 + (i-2)/4
			}
		}
		if (strings.HasPrefix(m.title, "Edit host — ") || strings.HasPrefix(m.title, "Edit hosts — ")) && m.query.Value() == "" {
			id := m.options[m.filtered[i]].ID
			if strings.HasPrefix(id, "ssh-") || strings.HasPrefix(id, "rdp-") {
				group = 1
			}
		}
		if strings.HasPrefix(m.title, "Host — ") && m.query.Value() == "" {
			group = hostFormGroup(m.options[m.filtered[i]].ID)
		}
		if (m.title == "SSH profiles" || m.title == "RDP profiles") && m.query.Value() == "" {
			id := m.options[m.filtered[i]].ID
			if id == "add" || id == "none" || strings.HasPrefix(id, "unassign:") {
				group = 1
			}
		}
		if strings.HasPrefix(m.title, "Tunnel — ") && m.query.Value() == "" {
			switch m.options[m.filtered[i]].ID {
			case "alias", "source":
				group = 0
			case "tunnel_type", "ssh_id", "resource", "namespace", "context":
				group = 1
			case "bind", "local_port":
				group = 2
			case "remote_target", "remote_host", "remote_port":
				group = 3
			default:
				group = 4
			}
		}
		if lastGroup >= 0 && group != lastGroup {
			slots = append(slots, -1)
		}
		slots = append(slots, i)
		lastGroup = group
	}
	return slots
}

func (m *dialogModel) visibleSlots() []int {
	slots := m.visualSlots()
	cursorSlot := 0
	for i, index := range slots {
		if index == m.cursor {
			cursorSlot = i
			break
		}
	}
	offset := max(0, cursorSlot-m.optionRows()+1)
	result := make([]int, m.optionRows())
	for row := range result {
		slot := offset + m.optionRows() - 1 - row
		result[row] = -1
		if slot < len(slots) {
			result[row] = slots[slot]
		}
	}
	return result
}
