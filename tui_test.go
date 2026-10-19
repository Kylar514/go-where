package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Exercise native launch/workflow code without requiring an interactive terminal.
func scriptTUI(t *testing.T, steps ...string) *[]string {
	t.Helper()
	old := runTUI
	t.Cleanup(func() { runTUI = old })
	seen := []string{}
	runTUI = func(model tea.Model) (tea.Model, error) {
		step := "cancel"
		if len(steps) > 0 {
			step = steps[0]
			steps = steps[1:]
		} else if _, ok := model.(*pickerModel); !ok {
			return model, fmt.Errorf("unexpected extra dialog")
		}
		switch m := model.(type) {
		case *pickerModel:
			if step == "cancel" {
				m.finish("cancel")
				return m, nil
			}
			parts := strings.Split(strings.TrimSuffix(step, "\n"), "\n")
			m.search.SetValue(parts[0])
			m.refilter()
			for _, row := range parts[2:] {
				id, _, _ := strings.Cut(row, "\t")
				i, _ := strconv.Atoi(id)
				if i >= 0 && i < len(m.hosts) {
					m.marked[m.hosts[i].Alias] = true
					m.search.SetValue("")
					m.refilter()
					m.cursor = i
				}
			}
			m.finish(parts[1])
			m.search.SetValue(parts[0])
		case *dialogModel:
			seen = append(seen, "dialog:"+m.title)
			if step == "back" {
				m.back = true
				return m, nil
			}
			id, _, _ := strings.Cut(step, "\t")
			id = strings.TrimSpace(id)
			for _, o := range m.options {
				if o.ID == id {
					m.result = id
					return m, nil
				}
			}
			return m, fmt.Errorf("invalid scripted choice %q for %s", id, m.title)
		case *pathModel:
			seen = append(seen, "path:"+m.input.Value())
			parts := strings.SplitN(strings.TrimSuffix(step, "\n"), "\n", 2)
			if len(parts) != 2 {
				return m, fmt.Errorf("invalid scripted path")
			}
			m.input.SetValue(parts[1])
			m.back = parts[0] == "back"
		}
		return model, nil
	}
	return &seen
}

func TestUnifiedPicker(t *testing.T) {
	c := defaultConfig()
	c.TunnelsConfig = t.TempDir() + "/missing.yaml"
	hosts := []Host{{Alias: "web", Hostname: "192.0.2.1", Notes: []string{"Windows"}}, {Alias: "db", Hostname: "192.0.2.2"}}
	m := newPickerModel(hosts, "", "tmux-pane", Origin{}, PickerState{Mode: "ssh"}, c)
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2"), Alt: true})
	if m.state.Mode != "rdp" || len(m.visible) != 1 || !m.marked["web"] {
		t.Fatal("mode switch lost marks or RDP filtering")
	}
	m.action("account")
	if m.dialog == nil || !strings.Contains(m.dialog.options[0].Label, "Default") {
		t.Fatal("missing native account dialog")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.dialog != nil || m.key != "" || !m.marked["web"] {
		t.Fatal("cancel must preserve picker state without launching")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlT, Alt: true})
	if m.key != "ctrl-alt-t" || !m.state.Debug || len(m.selected) != 1 {
		t.Fatalf("debug shortcut: %+v", m)
	}
	for _, size := range [][2]int{{110, 30}, {40, 15}, {30, 12}, {20, 6}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := m.View()
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("layout overflows %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}

func TestUnifiedMouseAndDialogs(t *testing.T) {
	c := defaultConfig()
	c.TunnelsConfig = t.TempDir() + "/missing.yaml"
	m := newPickerModel([]Host{{Alias: "a"}, {Alias: "b"}}, "", "terminal", Origin{}, PickerState{Mode: "ssh"}, c)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	m.View()
	m.Update(tea.MouseMsg{X: 2, Y: 7, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if h, _ := m.activeHost(); h.Alias != "b" {
		t.Fatal("mouse did not select second row")
	}
	m.View()
	for _, hit := range m.hits {
		if hit.action == "mode:tunnels" {
			m.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			break
		}
	}
	if m.state.Mode != "tunnels" {
		t.Fatal("mouse tab selection failed")
	}
	d := newDialog("Choose", []choice{{"ssh", "SSH"}, {"rdp", "RDP"}, {"rsync", "rsync"}}, "")
	d.Update(tea.WindowSizeMsg{Width: 50, Height: 16})
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("rdp")})
	d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.result != "rdp" {
		t.Fatal("native fuzzy dialog selected wrong mode")
	}
}

func TestUnifiedSelectionAndSmallDialogs(t *testing.T) {
	c := defaultConfig()
	c.TunnelsConfig = t.TempDir() + "/missing"
	hosts := []Host{{Alias: "one", Notes: []string{"RDP"}}, {Alias: "two"}, {Alias: "three", Notes: []string{"RDP"}}}
	m := newPickerModel(hosts, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 25})
	m.cursor = 1
	m.marked["one"], m.marked["two"] = true, true
	m.switchMode("rdp")
	m.switchMode("ssh")
	if h, _ := m.activeHost(); h.Alias != "two" {
		t.Fatal("mode switch lost highlighted host")
	}
	m.search.SetValue("three")
	m.refilter()
	if len(m.selection(false)) != 2 {
		t.Fatal("search must not discard hidden marked hosts")
	}
	m.switchMode("rdp")
	selected := m.selection(false)
	if len(selected) != 1 || selected[0].Alias != "one" {
		t.Fatal("RDP must exclude non-RDP marks without deleting them")
	}
	m.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.key != "" {
		t.Fatal("small terminal launched an invisible action")
	}
	d := newDialog("Confirm", []choice{{"run", "Run"}}, strings.Repeat("review\n", 20))
	d.Update(tea.WindowSizeMsg{Width: 20, Height: 5})
	d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.done {
		t.Fatal("small terminal accepted an invisible confirmation")
	}
	d.Update(tea.WindowSizeMsg{Width: 50, Height: 18})
	d.query.SetValue("no match")
	d.refilter()
	d.Update(tea.KeyMsg{Type: tea.KeyDown})
	d.query.SetValue("")
	d.refilter()
	d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if d.result != "run" {
		t.Fatal("empty search left an invalid cursor")
	}
	for _, size := range [][2]int{{50, 18}, {24, 9}, {20, 5}} {
		d.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if lipgloss.Height(d.View()) > size[1] || lipgloss.Width(d.View()) > size[0] {
			t.Fatalf("dialog overflows %v: %dx%d %q", size, lipgloss.Width(d.View()), lipgloss.Height(d.View()), d.View())
		}
	}
}
