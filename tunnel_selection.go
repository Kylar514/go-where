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

func (m *pickerModel) bulkTunnels(stop bool, toggle ...bool) tea.Cmd {
	var commands []tea.Cmd
	ids := m.markedTunnels()
	if m.hostTunnelTitle != "" && len(ids) == 0 && len(m.visible) > 0 {
		ids = []int{m.visible[m.cursor]}
	}
	for _, i := range ids {
		c := m.tunnels[i]
		if m.pending[c.Name] {
			continue
		}
		info := m.tunnelStatus[c.Name]
		stopThis := stop
		if len(toggle) > 0 && toggle[0] {
			stopThis = info.pid > 0
		}
		if err := m.tunnelErrors[c.Name]; err != nil && !stopThis {
			m.status = err.Error()
			m.statusError = true
			continue
		}
		if stopThis && info.pid == 0 {
			continue
		}
		if !stopThis && (info.pid > 0 || info.status == 3) {
			continue
		}
		m.pending[c.Name] = true
		commands = append(commands, func() tea.Msg { return tunnelActionMsg{c.Name, toggleManagedTunnel(c, stopThis)} })
	}
	return tea.Batch(commands...)
}
