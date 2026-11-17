package main

import (
	"maps"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Adjust this settling delay if Details feels too eager or too slow.
const detailsRefreshDelay = 0 * time.Millisecond

type detailsSettled struct{ revision uint64 }
type detailsResult struct {
	revision uint64
	text     string
}

func (m *pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case detailsSettled:
		if msg.revision != m.detailsRevision {
			return m, nil
		}
		m.detailsReady = true
		return m, m.startDetailsRefresh()
	case detailsResult:
		m.detailsBusy = false
		if msg.revision == m.detailsRevision {
			m.detailsText = msg.text
			m.preview.SetContent(ansi.Wrap(msg.text, max(1, m.preview.Width), ""))
			if m.dialog != nil && m.dialogKind == "details" {
				m.dialog.detailsText = msg.text
				m.dialog.refreshDetailsLayout()
			}
		}
		return m, m.startDetailsRefresh()
	}
	// Window-size changes rewrap existing content; they need no database read.
	if size, ok := msg.(tea.WindowSizeMsg); ok && m.width != 0 {
		m.width, m.height = size.Width, size.Height
		m.search.Width = max(1, size.Width-4-ansi.StringWidth(m.search.Prompt))
		_, w := m.paneWidths()
		if w == 0 {
			w = max(1, size.Width-4)
		}
		m.preview.Width, m.preview.Height = w, max(1, m.rows()-1)
		m.preview.SetContent(ansi.Wrap(m.detailsText, w, ""))
		m.scrollToCursor()
		if m.dialog != nil {
			_, cmd := m.dialog.Update(size)
			return m, cmd
		}
		return m, nil
	}
	_, initialSize := msg.(tea.WindowSizeMsg)
	m.deferDetails = !initialSize
	model, cmd := m.update(msg)
	m.deferDetails = false
	if !m.detailsDirty {
		return model, cmd
	}
	m.detailsDirty = false
	revision := m.detailsRevision
	return model, tea.Batch(cmd, tea.Tick(detailsRefreshDelay, func(time.Time) tea.Msg { return detailsSettled{revision} }))
}

func (m *pickerModel) startDetailsRefresh() tea.Cmd {
	if m.detailsBusy || !m.detailsReady {
		return nil
	}
	m.detailsReady = false
	m.detailsBusy = true
	revision := m.detailsRevision
	snapshot := *m
	snapshot.deferDetails = false
	snapshot.visible = append([]int(nil), m.visible...)
	snapshot.tunnelStatus = maps.Clone(m.tunnelStatus)
	snapshot.pending = maps.Clone(m.pending)
	return func() tea.Msg {
		snapshot.updatePreview()
		return detailsResult{revision, snapshot.detailsText}
	}
}
