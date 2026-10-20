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
	// Second result sits one row above the bottom of the reverse list.
	m.Update(tea.MouseMsg{X: 2, Y: 3 + m.rows() - 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if h, _ := m.activeHost(); h.Alias != "b" {
		t.Fatal("mouse did not select second row")
	}
	// Bottom-up rendering: Up moves the highlight to the row above.
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if h, _ := m.activeHost(); h.Alias != "b" {
		t.Fatal("up moved away from the clicked row above")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if h, _ := m.activeHost(); h.Alias != "b" {
		t.Fatal("up did not reach the visually higher row")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if h, _ := m.activeHost(); h.Alias != "a" {
		t.Fatal("down did not move the highlight visually down")
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

func TestModeKeysAndFramedLayout(t *testing.T) {
	c := defaultConfig()
	c.TunnelsConfig = t.TempDir() + "/missing"
	m := newPickerModel([]Host{{Alias: "demo"}}, "", "terminal", Origin{}, PickerState{Mode: "ssh"}, c)
	m.Update(tea.WindowSizeMsg{Width: 148, Height: 28})
	for _, step := range []struct {
		key  tea.KeyType
		mode string
	}{
		{tea.KeyRight, "rdp"}, {tea.KeyCtrlL, "tunnels"}, {tea.KeyRight, "scp"},
		{tea.KeyRight, "ssh"}, {tea.KeyLeft, "scp"}, {tea.KeyCtrlH, "tunnels"},
	} {
		m.Update(tea.KeyMsg{Type: step.key})
		if m.state.Mode != step.mode {
			t.Fatalf("key %v: mode %s", step.key, m.state.Mode)
		}
	}
	m.search.SetValue("abc")
	m.search.CursorEnd()
	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if m.search.Value() != "ab" || m.state.Mode != "tunnels" {
		t.Fatal("Backspace must edit search, not switch modes")
	}
	view := m.View()
	lines := strings.Split(view, "\n")
	searchRow := 3 + m.rows() + 1
	if !strings.Contains(lines[searchRow], "⌕ Search:") || !strings.Contains(lines[searchRow-1], "────") || !strings.Contains(lines[searchRow+1], "Toggle") {
		t.Fatal("search must sit between separator and action buttons")
	}
	if strings.Contains(view, "connections ·") || strings.Count(view, "Ctrl+R") != 1 || !strings.Contains(view, "╭─ Details") || !strings.Contains(view, "╰") {
		t.Fatalf("unexpected layout: %s", view)
	}
	if lipgloss.Width(view) > 148 || lipgloss.Height(view) != 28 {
		t.Fatalf("layout size %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
}

func TestPopupFrameLayoutAndMouse(t *testing.T) {
	c := defaultConfig()
	c.TunnelsConfig = t.TempDir() + "/missing"
	picker := newPickerModel([]Host{{Alias: "demo"}}, "", "terminal", Origin{}, PickerState{Mode: "ssh"}, c)
	frame := &popupFrame{model: picker}
	frame.Update(tea.WindowSizeMsg{Width: 150, Height: 30})
	view := frame.View()
	if picker.width != 148 || picker.height != 28 || lipgloss.Width(view) != 150 || lipgloss.Height(view) != 30 {
		t.Fatalf("frame/child dimensions wrong: %dx%d / %dx%d", lipgloss.Width(view), lipgloss.Height(view), picker.width, picker.height)
	}
	if !strings.Contains(view, "╭─ go-where ") || !strings.Contains(view, "╰") {
		t.Fatal("missing frame")
	}
	for _, hit := range picker.hits {
		if hit.action == "mode:tunnels" {
			frame.Update(tea.MouseMsg{X: hit.x + 1, Y: hit.y + 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
			break
		}
	}
	if picker.state.Mode != "tunnels" {
		t.Fatal("frame did not translate mouse coordinates")
	}
	frame.Update(tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if picker.state.Mode != "tunnels" {
		t.Fatal("border click reached content")
	}
	picker.action("help")
	if !strings.Contains(frame.View(), "╭─ go-where ") {
		t.Fatal("dialog lost outer frame")
	}
	frame.Update(tea.WindowSizeMsg{Width: 2, Height: 2})
	if frame.View() != "" {
		t.Fatal("tiny frame overflowed")
	}
}
