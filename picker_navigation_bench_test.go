package main

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func BenchmarkPickerNavigation(b *testing.B) {
	for _, width := range []int{80, 148} {
		for _, render := range []bool{false, true} {
			b.Run(fmt.Sprintf("width=%d/render=%t", width, render), func(b *testing.B) {
				hosts := make([]Host, 150)
				for i := range hosts {
					hosts[i] = Host{Alias: fmt.Sprintf("server-%03d", i), Hostname: "192.0.2.1", Comment: "Synthetic host for sustained-navigation profiling"}
				}
				m := newPickerModel(hosts, "", "current", Origin{}, PickerState{Mode: "ssh"}, defaultConfig())
				m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
					if render {
						m.View()
					}
				}
			})
		}
	}
}

func TestControlMeasurementMatchesRendering(t *testing.T) {
	for _, width := range []int{30, 40, 60, 80, 100, 148, 200} {
		for _, height := range []int{12, 16, 20, 30, 40} {
			for _, mode := range []string{"ssh", "rdp", "tunnels", "transfers"} {
				m := newPickerModel(nil, "", "current", Origin{}, PickerState{Mode: mode}, defaultConfig())
				m.width, m.height = width, height
				for _, buttons := range [][]pickerButton{m.tabs(), m.buttons()} {
					_, _, rendered := m.controls(buttons, 0)
					if measured := m.controlHeight(buttons); measured != rendered {
						t.Fatalf("%s %dx%d: measured %d, rendered %d", mode, width, height, measured, rendered)
					}
					text, hits, _ := m.layoutControls(buttons, 0, true)
					if len(hits) != 0 || len(text) > rendered {
						t.Fatal("measurement generated styled content or mouse regions")
					}
				}
			}
		}
	}
}
