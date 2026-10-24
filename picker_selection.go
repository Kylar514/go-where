package main

func (m *pickerModel) modeMarkKeys() []string {
	var keys []string
	if m.state.Mode == "tunnels" {
		for _, tunnel := range m.tunnels {
			keys = append(keys, tunnel.Name)
		}
	} else if m.state.Mode == "ssh" || m.state.Mode == "rdp" {
		for _, h := range m.hosts {
			if h.SourceID != "" && !managedModeEligible(h, m.state.Mode) {
				continue
			}
			if m.state.Mode == "rdp" && !h.isRDP(m.config.RDP.Tags) {
				continue
			}
			keys = append(keys, hostIdentity(h))
		}
	}
	return keys
}

func (m *pickerModel) hasModeMarks() bool {
	for _, key := range m.modeMarkKeys() {
		if m.marked[key] {
			return true
		}
	}
	return false
}

func (m *pickerModel) toggleAllMarks() {
	if m.state.Mode != "ssh" && m.state.Mode != "rdp" && m.state.Mode != "tunnels" {
		return
	}
	if m.hasModeMarks() {
		for _, key := range m.modeMarkKeys() {
			delete(m.marked, key)
		}
	} else {
		for _, id := range m.visible {
			key := ""
			if m.state.Mode == "tunnels" {
				key = m.tunnels[id].Name
			} else {
				key = hostIdentity(m.hosts[id])
			}
			m.marked[key] = true
		}
	}
	m.lastClick = ""
	// The changing label/toolbar can wrap differently, reducing available rows.
	m.preview.Height = max(1, m.rows()-1)
	m.scrollToCursor()
	m.updatePreview()
}
