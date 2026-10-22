package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestMenuNavigationAndBottomSearch(t *testing.T) {
	options := []choice{{"a", "Alpha"}, {"b", "Beta"}, {"c", "Gamma"}}
	d := newDialog("Menu", options, "Details")
	d.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	d.Update(tea.KeyMsg{Type: tea.KeyUp})
	if d.cursor != 1 {
		t.Fatal("up must move visually upward")
	}
	d.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if d.result != "b" {
		t.Fatal("Ctrl+L must select")
	}
	for _, key := range []tea.KeyType{tea.KeyCtrlH, tea.KeyLeft} {
		d = newDialog("Menu", options, "")
		d.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
		d.Update(tea.KeyMsg{Type: key})
		if !d.back {
			t.Fatal("back key failed", key)
		}
	}
	for _, size := range [][2]int{{80, 30}, {40, 14}, {24, 9}} {
		d = newDialog("Menu", options, strings.Repeat("detail\n", 30))
		d.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		view := ansi.Strip(d.View())
		lines := strings.Split(view, "\n")
		search := size[1] - 2 - footerExtra(size[1])
		if len(lines) != size[1] || !strings.HasPrefix(lines[search], "⌕ Search: ") || !strings.HasPrefix(lines[search-1], "─") || lipgloss.Width(d.View()) > size[0] {
			t.Fatalf("layout %v:\n%s", size, view)
		}
		if d.query.PromptStyle.String() != tuiAccent.String() {
			t.Fatal("search color differs from picker")
		}
	}
	d = newDialog("Menu", options, "")
	d.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	d.Update(tea.MouseMsg{X: 3, Y: d.listStart() + d.optionRows() - 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if d.result != "b" {
		t.Fatal("reverse row mouse selection failed")
	}
	m := newMultiDialog("Checklist", []multiItem{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta"}}, "")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if !m.items[1].Checked || m.items[0].Checked {
		t.Fatal("Ctrl+L toggled wrong reversed row")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if !m.back {
		t.Fatal("checklist back failed")
	}
	// Text editing keeps its native cursor behavior, not menu navigation.
	p := newPathModel("Value", "abc", false)
	p.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	p.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if p.back {
		t.Fatal("left backed out of text entry")
	}
}

func TestParentMenuRestoresPositionWithinVisit(t *testing.T) {
	old := runTUI
	t.Cleanup(func() { runTUI = old })
	options := make([]choice, 40)
	for i := range options {
		options[i] = choice{fmt.Sprint(i), fmt.Sprintf("Item %02d", i)}
	}
	visit := func() {
		defer menuSession()()
		for step := 0; step < 2; step++ {
			runTUI = func(model tea.Model) (tea.Model, error) {
				d := model.(*dialogModel)
				d.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
				if step == 0 {
					d.query.SetValue("Item")
					d.refilter()
					d.cursor = 25
					d.preview.SetYOffset(5)
					d.result = "25"
				} else {
					if d.query.Value() != "Item" || d.cursor != 25 || d.preview.YOffset != 5 {
						t.Fatalf("parent position lost: query=%q cursor=%d preview=%d", d.query.Value(), d.cursor, d.preview.YOffset)
					}
					d.back = true
				}
				return d, nil
			}
			if _, _, err := choose("Parent", options, strings.Repeat("detail\n", 40)); err != nil {
				t.Fatal(err)
			}
		}
	}
	visit()
	if menuPositions != nil || menuDepth != 0 {
		t.Fatal("menu visit leaked navigation state")
	}
}

func TestMenuKindsAndBottomInput(t *testing.T) {
	d := newDialog("Settings", []choice{{"submenu", "SSH settings…"}, {"save", "Save"}}, "")
	d.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	v := ansi.Strip(d.View())
	if !strings.HasPrefix(v, "Settings\n") || !strings.Contains(v, "SSH settings…  ›") || strings.Contains(v, "Save  ›") {
		t.Fatal(v)
	}
	d.singleChoice, d.current = true, "save"
	v = ansi.Strip(d.View())
	if !strings.Contains(v, "Choose one") || !strings.Contains(v, "○ SSH settings…") {
		t.Fatal(v)
	}
	m := newMultiDialog("Algorithms", []multiItem{{ID: "a", Label: "Alpha", Checked: true}}, "")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if v = ansi.Strip(m.View()); !strings.Contains(v, "Select several") || !strings.Contains(v, "[✓] Alpha") {
		t.Fatal(v)
	}
	for _, size := range [][2]int{{80, 20}, {24, 9}} {
		p := newPathModel("Address", "192.0.2.1", false)
		p.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		v = ansi.Strip(p.View())
		lines := strings.Split(v, "\n")
		input := size[1] - 2 - footerExtra(size[1])
		if len(lines) != size[1] || lines[input-1] != "Address" || !strings.HasPrefix(lines[input], "› 192.0.2.1") || lipgloss.Width(p.View()) > size[0] {
			t.Fatal(v)
		}
		p.Update(tea.MouseMsg{X: 18, Y: size[1] - 1 - footerExtra(size[1])/2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if !p.back {
			t.Fatal("bottom Back button failed")
		}
	}
}

func TestListsCycleAtBothEnds(t *testing.T) {
	for _, count := range []int{0, 1, 3} {
		for _, cursor := range []int{0, max(0, count-1)} {
			for _, delta := range []int{-1, 1} {
				got := cycleCursor(cursor, delta, count)
				if count == 0 {
					if got != 0 {
						t.Fatal("empty list moved")
					}
					continue
				}
				if got < 0 || got >= count || cycleCursor(got, -delta, count) != cursor {
					t.Fatal("invalid cycle", cursor, delta, count, got)
				}
			}
		}
	}
	options := []choice{{"a", "Alpha"}, {"b", "Beta"}, {"c", "Gamma"}}
	for _, keys := range [][2]tea.KeyType{{tea.KeyDown, tea.KeyUp}, {tea.KeyCtrlJ, tea.KeyCtrlK}} {
		d := newDialog("Menu", options, "")
		d.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
		d.Update(tea.KeyMsg{Type: keys[0]})
		if d.cursor != 2 {
			t.Fatal("menu down failed to wrap")
		}
		d.Update(tea.KeyMsg{Type: keys[1]})
		if d.cursor != 0 {
			t.Fatal("menu up failed to wrap")
		}
		c := defaultConfig()
		c.TunnelsConfig = t.TempDir() + "/missing"
		m := newPickerModel([]Host{{Alias: "a"}, {Alias: "b"}, {Alias: "c"}}, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
		m.Update(tea.KeyMsg{Type: keys[0]})
		if m.cursor != 2 {
			t.Fatal("picker down failed to wrap")
		}
		m.Update(tea.KeyMsg{Type: keys[1]})
		if m.cursor != 0 {
			t.Fatal("picker up failed to wrap")
		}
	}
	m := newMultiDialog("Checklist", []multiItem{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta"}}, "")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	// Top algorithm -> last pinned action -> top algorithm.
	m.cursor = 1
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 3 {
		t.Fatal("checklist top did not wrap to manual")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatal("manual did not wrap to top")
	}
	m.query.SetValue("no match")
	m.refilter()
	m.cursor = 0
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 1 {
		t.Fatal("empty checklist pinned actions do not cycle")
	}
	p := newPathModel("Path", "", false)
	p.items = []string{"a", "b"}
	p.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	p.Update(tea.KeyMsg{Type: tea.KeyUp})
	if p.cursor != 1 {
		t.Fatal("completion list did not wrap")
	}
	p.Update(tea.KeyMsg{Type: tea.KeyDown})
	if p.cursor != 0 {
		t.Fatal("completion reverse wrap failed")
	}
}

func TestSettingsBreadcrumbDetailsAndActions(t *testing.T) {
	old := runTUI
	t.Cleanup(func() { runTUI = old })
	defer menuSession()()
	for _, title := range []string{"Settings", "SSH agents", "legacy"} {
		runTUI = func(model tea.Model) (tea.Model, error) {
			m := model.(*dialogModel)
			m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
			view := ansi.Strip(m.View())
			if !strings.Contains(view, "╭─ Details") || !strings.HasPrefix(view, strings.Join(menuTrail, " › ")) {
				t.Fatal(view)
			}
			if title == "Settings" {
				for _, index := range m.filtered {
					if m.options[index].ID == "save" || m.options[index].ID == "cancel" {
						t.Fatal("action still in list")
					}
				}
				m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
				if m.result != "save" {
					t.Fatal("save shortcut failed")
				}
			}
			return m, nil
		}
		if _, _, err := choose(title, []choice{{"open", "Open…"}, {"save", "Save"}, {"cancel", "Cancel"}}, "First sentence. Second sentence."); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(menuTrail, " › ") != "Settings › SSH agents › legacy" {
		t.Fatal(menuTrail)
	}
	if menuExplanation("First sentence. Second sentence.") != "First sentence.\n\nSecond sentence." {
		t.Fatal("prose not separated")
	}
}

func TestDetailsWrapWholeWordsAndLongValues(t *testing.T) {
	text := "alpha beta elephant"
	want := "alpha beta\nelephant"
	d := newDialog("Details", nil, text)
	d.Update(tea.WindowSizeMsg{Width: 24, Height: 20})
	d.preview.Width = 12
	d.preview.Height = 2
	d.preview.SetContent(ansi.Wrap(text, 12, ""))
	lines := strings.Split(d.preview.View(), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	if strings.Join(lines, "\n") != want {
		t.Fatal("word split", d.preview.View())
	}
	for _, value := range []string{text, "averylongunbrokenvalue", "日本語の長い文章"} {
		wrapped := ansi.Wrap(value, 12, "")
		if lipgloss.Width(wrapped) > 12 {
			t.Fatal("long value overflows", wrapped)
		}
	}
	c := defaultConfig()
	c.TunnelsConfig = t.TempDir() + "/missing"
	m := newPickerModel([]Host{{Alias: "alpha", Hostname: "beta elephant"}}, "", "current", Origin{}, PickerState{Mode: "ssh"}, c)
	m.preview.Width = 12
	m.preview.Height = 30
	m.updatePreview()
	if !strings.Contains(m.preview.View(), "elephant") {
		t.Fatal("picker splits word", m.preview.View())
	}
	check := newMultiDialog("Details", nil, "alpha beta elephant")
	check.Update(tea.WindowSizeMsg{Width: 24, Height: 20})
	if !strings.Contains(check.preview.View(), "elephant") {
		t.Fatal("checklist splits word")
	}
}
