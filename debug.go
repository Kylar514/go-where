package main

import (
	"bufio"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"os"
)

func debugDestination(o Origin) string {
	if o.Pane != "" {
		return "tmux-pane"
	}
	return "terminal"
}
func runDebugCommand(argv []string) error {
	err := run(argv)
	if err != nil {
		fmt.Printf("\nFAILED: %s\n", clean(err.Error()))
	}
	fmt.Print("\nDebug session ended. Press Enter to close… ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return err
}
func verboseTransfer(argv []string) []string {
	result := append([]string(nil), argv...)
	if result[0] == "scp" {
		return append([]string{"scp", "-vv"}, result[1:]...)
	}
	result = append([]string{"rsync", "-vv"}, result[1:]...)
	for i := range result {
		if result[i] == "-e" && i+1 < len(result) {
			result[i+1] += " -vv"
			return result
		}
	}
	return append(result[:1], append([]string{"-e", "ssh -vv"}, result[1:]...)...)
}

func (m *pickerModel) debugTunnel() tea.Cmd {
	if len(m.visible) == 0 {
		return nil
	}
	c := m.tunnels[m.visible[m.cursor]]
	text := m.tunnelDisplayName(c) + "\n" + m.tunnelLabel(c.Name) + "\n\n" + tunnelCommandPreview(c)
	if h, ok := m.tunnelHosts[c.Name]; ok {
		text = managedDetails(h, m.config) + "\n\n" + m.tunnelLabel(c.Name)
	}
	if m.pending[c.Name] || m.tunnelStatus[c.Name].pid > 0 {
		m.openDialog("debug-tunnel", "Tunnel diagnostics", []choice{{"back", "Back"}}, text+"\n\nAlready running: no second tunnel started.")
		return nil
	}
	// Use the ordinary tracked lifecycle; report preflight and startup errors.
	cmd := m.toggleTunnel()
	if cmd == nil {
		m.openDialog("debug-tunnel", "Tunnel diagnostics", []choice{{"back", "Back"}}, text+"\n\n"+m.status)
		return nil
	}
	return func() tea.Msg {
		message := cmd()
		if result, ok := message.(tunnelActionMsg); ok {
			result.err = debugTunnelResult{text: text, err: result.err}
			return result
		}
		return message
	}
}

type debugTunnelResult struct {
	text string
	err  error
}

func (r debugTunnelResult) Error() string {
	if r.err != nil {
		return r.err.Error()
	}
	return ""
}
