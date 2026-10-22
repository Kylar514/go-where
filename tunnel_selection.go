package main

import tea "github.com/charmbracelet/bubbletea"

func (m *pickerModel) markedTunnels() []int {
	var ids []int
	for i, c := range m.tunnels {
		if !m.marked[c.Name] {
			continue
		}
		h := m.tunnelHosts[c.Name]
		if m.state.Source != "" && m.state.Source != h.SourceID {
			continue
		}
		ids = append(ids, i)
	}
	return ids
}

func (m *pickerModel) bulkTunnels(stop bool) tea.Cmd {
	var commands []tea.Cmd
	for _, i := range m.markedTunnels() {
		c := m.tunnels[i]
		if m.pending[c.Name] {
			continue
		}
		if err := m.tunnelErrors[c.Name]; err != nil {
			m.status = err.Error()
			m.statusError = true
			continue
		}
		info := m.tunnelStatus[c.Name]
		if stop && info.pid == 0 {
			continue
		}
		if !stop && (info.pid > 0 || info.status == 3) {
			continue
		}
		m.pending[c.Name] = true
		commands = append(commands, func() tea.Msg { return tunnelActionMsg{c.Name, toggleManagedTunnel(c, stop)} })
	}
	return tea.Batch(commands...)
}
