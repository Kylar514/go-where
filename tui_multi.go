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
	sourceManagement  bool
	identitySelection bool
	itemDetails       map[string]string
	hostRows          map[string]Host
	columnLayout      pickerModel
	columnHits        []hitRegion
	columnDrag        int
	columnDragging    bool
	resize            dialogModel
	sourceSelection   bool
	allowManual       bool
	title             string
	items             []multiItem
	filtered          []int
	cursor            int // over filtered items, then Done, then manual entry
	query             textinput.Model
	preview           viewport.Model
	text              string
	width, height     int
	done, back        bool
	action            string // "done" or "manual"
	restorePreview    *int
}

func newMultiDialog(title string, items []multiItem, text string) *multiDialog {
	input := menuSearch()
	m := &multiDialog{title: title, items: append([]multiItem(nil), items...), text: text, query: input, preview: viewport.New(1, 1)}
	m.allowManual = true
	m.refilter()
	return m
}

func runIdentityChecklist(m *multiDialog) (tea.Model, error) {
	key := "checklist:" + m.title
	if p, ok := menuPositions[key]; ok {
		m.query.SetValue(p.query)
		m.refilter()
		m.cursor = min(p.cursor, max(0, len(m.filtered)-1))
		for i, index := range m.filtered {
			if m.items[index].ID == p.selected {
				m.cursor = i
				break
			}
		}
		m.restorePreview = &p.preview
	}
	model, err := runTUI(m)
	if menuPositions != nil {
		p := menuPosition{query: m.query.Value(), cursor: m.cursor, preview: m.preview.YOffset}
		if m.cursor < len(m.filtered) {
			p.selected = m.items[m.filtered[m.cursor]].ID
		}
		menuPositions[key] = p
	}
	return model, err
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
	limit := len(m.filtered) + m.manualCount()
	if m.identitySelection {
		limit = max(0, len(m.filtered)-1)
	}
	m.cursor = min(max(0, m.cursor), limit)
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
	if m.sourceSelection {
		for index := range m.items {
			m.items[index].Checked = i == len(m.filtered)
		}
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
	gap := 0
	if m.sourceSelection {
		gap = 1
	}
	return m.height - 5 - footerExtra(m.height) - m.itemRows() - gap
}

func (m *multiDialog) itemRows() int {
	rows := m.height - 7 - footerExtra(m.height)
	if m.sourceSelection {
		rows--
	}
	if m.text != "" && m.width < 90 {
		rows -= m.preview.Height + 1
	}
	return max(1, min(20, rows))
}

func (m *multiDialog) move(delta int) {
	if m.identitySelection {
		m.cursor = cycleCursor(m.cursor, -delta, len(m.filtered))
		return
	}
	n := len(m.filtered)
	position := m.cursor
	if position < n {
		position = n - 1 - position
	}
	if delta == -1 || delta == 1 {
		position = cycleCursor(position, delta, n+1+m.manualCount())
	} else {
		position = min(n+m.manualCount(), max(0, position+delta))
	}
	if position < n {
		position = n - 1 - position
	}
	m.cursor = position
}

func (m *multiDialog) offset() int {
	return max(0, min(m.cursor, len(m.filtered)-1)-m.itemRows()+1)
}

func (m *multiDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.columnLayout.width = msg.Width
		m.resize.width, m.resize.height = msg.Width, msg.Height
		m.query.Width = max(1, msg.Width-ansi.StringWidth(m.query.Prompt)-1)
		m.preview.Width = max(1, msg.Width-4)
		if msg.Width >= 90 {
			_, m.preview.Width = m.resize.menuPaneWidths()
		}
		content := ansi.Wrap(cleanLines(m.text), m.preview.Width, "")
		m.preview.Height = min(lipgloss.Height(content), max(1, (msg.Height-8)/2))
		if msg.Width >= 90 {
			gap := 0
			if m.sourceSelection {
				gap = 1
			}
			if m.hostRows != nil && m.cursor < len(m.filtered) {
				item := m.items[m.filtered[m.cursor]]
				host := m.hostRows[item.ID]
				text := host.Alias + "\n\nAddress: " + host.Hostname + "\nSource: " + host.SourceName + "\nTags: " + strings.Join(host.Tags, ", ") + "\nNotes: " + host.Comment + "\nConnection: " + item.Label + "\n\n" + m.text
				m.preview.SetContent(ansi.Wrap(cleanLines(text), max(1, m.preview.Width), ""))
			}
			m.preview.Height = m.itemRows() + gap
		}
		m.preview.SetContent(content)
		if m.restorePreview != nil {
			m.preview.SetYOffset(*m.restorePreview)
			m.restorePreview = nil
		}
	case tea.KeyMsg:
		scope := "menu"
		if m.sourceManagement {
			scope = "sources"
		}
		key := canonicalKey(menuHotkeys, msg.String(), scope)
		if m.width > 0 && m.tooSmall() && key != "esc" {
			return m, nil
		}
		switch key {
		case "ctrl+n", "ctrl+o", "ctrl+alt+d":
			if m.sourceManagement {
				m.action = map[string]string{"ctrl+n": "create", "ctrl+o": "attach", "ctrl+alt+d": "remove"}[key]
				m.done = true
				return m, tea.Quit
			}
		case "tab", "shift+tab":
			if m.cursor < len(m.filtered) {
				m.activate(m.cursor)
				delta := -1
				if key == "shift+tab" {
					delta = 1
				}
				m.move(delta)
			}
			return m, nil
		case "ctrl+s":
			if m.identitySelection {
				m.action = "done"
				m.done = true
				return m, tea.Quit
			}
			if m.sourceSelection {
				m.action = "save"
				m.done = true
				return m, tea.Quit
			}
		case "esc", "ctrl+c", "ctrl+h", "left":
			m.done, m.back = true, true
			return m, tea.Quit
		case "up", "ctrl+k":
			m.move(-1)
			return m, nil
		case "down", "ctrl+j":
			m.move(1)
			return m, nil
		case " ", "enter", "ctrl+l":
			return m, m.activate(m.cursor)
		case "ctrl+u", "pgup":
			m.cursor = min(len(m.filtered), m.cursor+max(1, (m.height-8)/2))
			return m, nil
		case "ctrl+d", "pgdown":
			m.cursor = max(0, m.cursor-max(1, (m.height-8)/2))
			return m, nil
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
		}
	case tea.MouseMsg:
		if m.sourceManagement && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == footerButtonY(m.height) {
			x := 17
			for _, a := range []string{"create", "attach", "remove", "save"} {
				width := map[string]int{"create": 9, "attach": 15, "remove": 9, "save": 7}[a]
				if msg.X >= x && msg.X < x+width {
					m.action = a
					m.done = true
					return m, tea.Quit
				}
				x += width
			}
		}
		if m.identitySelection && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == footerButtonY(m.height) {
			if msg.X >= 8 && msg.X < 17 {
				return m, m.activate(m.cursor)
			}
			if msg.X >= 17 && msg.X < 25 {
				m.action = "done"
				m.done = true
				return m, tea.Quit
			}
		}
		if m.columnDragging {
			if msg.Action == tea.MouseActionRelease {
				m.columnDragging = false
				return m, nil
			}
			if msg.Action == tea.MouseActionMotion {
				width, _ := m.resize.menuPaneWidths()
				widths := m.columnLayout.columnWidths(width)
				i := m.columnDrag
				start := 5
				for j := 0; j < i; j++ {
					start += widths[j] + 2
				}
				delta := msg.X - start - widths[i]
				delta = min(widths[i+1]-4, max(4-widths[i], delta))
				widths[i] += delta
				widths[i+1] -= delta
				m.columnLayout.state.ColumnWidths = widths
			}
			return m, nil
		}
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			for _, hit := range m.columnHits {
				if msg.Y == hit.y && msg.X >= hit.x && msg.X < hit.x+hit.width {
					m.columnDragging = true
					m.columnDrag = hit.index
					return m, nil
				}
			}
		}
		if m.tooSmall() {
			return m, nil
		}
		m.resize.preview = m.preview
		if m.resize.resizeMenuMouse(msg) {
			if m.resize.resizeKind != "" || msg.Action == tea.MouseActionRelease {
				offset := m.preview.YOffset
				m.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
				m.preview.SetYOffset(offset)
			}
			return m, nil
		}
		if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == footerButtonY(m.height) && msg.X >= 0 && msg.X < 8 {
			m.done, m.back = true, true
			return m, tea.Quit
		}
		if m.sourceSelection && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.Y == footerButtonY(m.height) && msg.X >= 8 && msg.X < 24 {
			if msg.X < 17 {
				return m, m.activate(m.cursor)
			}
			m.action = "save"
			m.done = true
			return m, tea.Quit
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
			delta := 3
			if msg.Button == tea.MouseButtonWheelUp {
				delta = -3
			}
			listWidth, _ := m.resize.menuPaneWidths()
			if msg.Y >= m.listStart() && msg.Y < m.listStart()+m.itemRows()+3 && (m.width < 90 || msg.X < listWidth) {
				m.move(delta)
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
			listWidth, _ := m.resize.menuPaneWidths()
			if m.width >= 90 && msg.X >= listWidth {
				return m, nil
			}
			row := msg.Y - m.listStart()
			if m.sourceSelection && row >= m.itemRows() {
				if row == m.itemRows() {
					return m, nil
				}
				row--
			}
			index := m.offset() + m.itemRows() - 1 - row
			switch {
			case row >= 0 && row < m.itemRows() && index < len(m.filtered):
				m.cursor = index
				return m, m.activate(m.cursor)
			case row == m.itemRows():
				m.cursor = len(m.filtered)
				return m, m.activate(m.cursor)
			case row == m.itemRows()+1 && m.allowManual:
				m.cursor = len(m.filtered) + 1
				return m, m.activate(m.cursor)
			}
			return m, nil
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

func (m *multiDialog) View() string {
	if m.width == 0 {
		return "Loading…"
	}
	if m.tooSmall() {
		return fit("Resize terminal · Esc back", m.width)
	}
	w := m.width - 2
	if m.width >= 90 {
		w, _ = m.resize.menuPaneWidths()
	}
	m.resize.resizeHits = nil
	m.columnHits = nil
	header := m.title
	if len(menuTrail) > 0 {
		header = strings.Join(menuTrail, " › ") + " › " + m.title
	}
	lines := []string{tuiAccent.Render(fit(header+" — Select several", w))}
	if m.itemDetails != nil {
		text := "Select all\nEnable every registered source, including sources hidden by the filter."
		if m.cursor == len(m.filtered)+1 {
			text = "Deselect all\nDisable every registered source, including hidden sources."
		} else if m.cursor < len(m.filtered) {
			item := m.items[m.filtered[m.cursor]]
			text = m.itemDetails[item.ID] + "\nEnabled: " + boolSettingLabel(&item.Checked)
		}
		m.preview.SetContent(ansi.Wrap(text+"\n\n"+m.text, max(1, m.preview.Width), ""))
	}
	if m.itemDetails == nil && m.hostRows == nil && m.cursor < len(m.filtered) {
		item := m.items[m.filtered[m.cursor]]
		m.preview.SetContent(ansi.Wrap(item.Label+"\n\n"+item.Note+"\n\n"+m.text, max(1, m.preview.Width), ""))
	}
	if m.text != "" && m.width < 90 {
		lines = append(lines, m.preview.View(), "")
	}
	for lipgloss.Height(strings.Join(lines, "\n")) < m.listStart() {
		lines = append(lines, "")
	}
	bodyStart := len(lines)
	row := func(index int, plain, styled string) string {
		if index == m.cursor {
			return tuiSelected.Render(fit("› "+plain, w))
		}
		return fit("  "+styled, w)
	}
	if m.hostRows != nil {
		if m.cursor < len(m.filtered) {
			item := m.items[m.filtered[m.cursor]]
			host := m.hostRows[item.ID]
			text := host.Alias + "\n\nAddress: " + host.Hostname + "\nSource: " + host.SourceName + "\nTags: " + strings.Join(host.Tags, ", ") + "\nNotes: " + host.Comment + "\nConnection: " + item.Label + "\n\n" + m.text
			m.preview.SetContent(ansi.Wrap(cleanLines(text), max(1, m.preview.Width), ""))
		}
		widths := m.columnLayout.columnWidths(w)
		heading := renderHostColumns(nil, w, "", widths)
		// Reuse the spare row immediately above the bottom-anchored list.
		if len(lines) > 1 {
			lines[len(lines)-1] = tuiDim.Render(heading)
		}
		x := 5
		for i, width := range widths {
			x += width
			if i < len(widths)-1 {
				m.columnHits = append(m.columnHits, hitRegion{x, m.listStart() - 1, 2, "column", i})
				x += 2
			}
		}
	}
	rows, offset := m.itemRows(), m.offset()
	for slot := rows - 1; slot >= 0; slot-- {
		i := offset + slot
		if i >= len(m.filtered) {
			if len(m.filtered) == 0 && slot == 0 {
				lines = append(lines, tuiDim.Render("No matches"))
			} else {
				lines = append(lines, "")
			}
			continue
		}
		item := m.items[m.filtered[i]]
		box := "[ ]"
		if item.Checked {
			box = "[✓]"
		}
		plain := box + " " + clean(item.Label)
		if host, ok := m.hostRows[item.ID]; ok {
			plain = renderHostColumns(&host, w, box, m.columnLayout.columnWidths(w))
		}
		styled := plain
		if item.Note != "" && m.hostRows == nil && !m.identitySelection {
			plain += "  — " + clean(item.Note)
			styled += tuiDim.Render("  — " + clean(item.Note))
		}
		if m.hostRows != nil {
			line := plain
			if i == m.cursor {
				line = tuiSelected.Render(fit("›"+strings.TrimPrefix(plain, " "), w))
			}
			host := m.hostRows[item.ID]
			if host.Color != "" {
				line = ansi.Cut(line, 0, 5) + colorSwatch(host.Color) + ansi.Cut(line, 6, w)
			}
			lines = append(lines, line)
		} else {
			lines = append(lines, row(i, plain, styled))
		}
	}
	done := fmt.Sprintf("│ Done — apply %d selected │", m.checkedCount())
	manual := "│ Type a name manually… │"
	if m.sourceSelection {
		done = "Select all"
		manual = "Deselect all"
	}
	if m.sourceSelection {
		lines = append(lines, "")
	}
	if !m.identitySelection {
		lines = append(lines, row(len(m.filtered), done, done))
	} else {
		lines = append(lines, "")
	}
	if m.allowManual {
		lines = append(lines, row(len(m.filtered)+1, manual, manual))
	} else {
		lines = append(lines, "")
	}
	if m.width >= 90 {
		body := strings.Join(lines[bodyStart:], "\n")
		bodyHeight := lipgloss.Height(body)
		divider := strings.TrimSuffix(strings.Repeat(tuiDim.Render("│")+"\n", bodyHeight), "\n")
		panel := detailsPanel(m.preview)
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(w).Render(body), divider, panel)
		lines = append(lines[:bodyStart], body)
		for row := 0; row < bodyHeight; row++ {
			m.resize.resizeHits = append(m.resize.resizeHits, hitRegion{w, m.listStart() + row, 1, "pane", 0})
		}
	}
	footer := menuFooter(m.height, m.width, []string{"Back"}, keyHint(menuHotkeys, "back")+" discard · "+keyHint(menuHotkeys, "select")+"/Space toggle")
	if m.identitySelection {
		footer = menuFooter(m.height, m.width, []string{"Back", "Toggle", "Apply"}, primaryMenuHint(menuHotkeys, "back")+" Back │ "+primaryMenuHint(menuHotkeys, "select")+" Toggle │ "+primaryMenuHint(menuHotkeys, "menu_save")+" Apply")
	}
	if m.hostRows != nil {
		footer = menuFooter(m.height, m.width, []string{"Back"}, keyHint(menuHotkeys, "back")+" Back | Tab/Shift+Tab Mark/move | Space Toggle")
	}
	if m.sourceSelection {
		footer = menuFooter(m.height, m.width, []string{"Back", "Toggle", "Save"}, primaryMenuHint(menuHotkeys, "back")+" Back │ "+primaryMenuHint(menuHotkeys, "select")+" Toggle │ "+primaryMenuHint(menuHotkeys, "menu_save")+" Save")
		if m.sourceManagement {
			footer = menuFooter(m.height, m.width, []string{"Back", "Toggle", "Create", "Add existing", "Remove", "Save"}, primaryMenuHint(menuHotkeys, "back")+" Back │ "+primaryMenuHint(menuHotkeys, "select")+" Toggle │ "+primaryMenuHint(menuHotkeys, "source_create")+" Create │ "+primaryMenuHint(menuHotkeys, "source_attach")+" Add existing │ "+primaryMenuHint(menuHotkeys, "source_remove")+" Remove │ "+primaryMenuHint(menuHotkeys, "menu_save")+" Save")
		}
	}
	lines = append(lines, strings.Repeat("─", m.width), m.query.View(), footer)
	return strings.Join(lines, "\n")
}

func (m *multiDialog) manualCount() int {
	if m.allowManual {
		return 1
	}
	return 0
}
