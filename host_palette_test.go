package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestTwoColorDraftAndPersistence(t *testing.T) {
	scriptTUI(t, "block", "#ef4444", "bracket", "#3b82f6", "save")
	block, bracket, back, err := editHostColors("#22c55e", "")
	if err != nil || back || block != "#ef4444" || bracket != "#3b82f6" {
		t.Fatal(block, bracket, back, err)
	}
	scriptTUI(t, "clear", "back")
	b, r, back, err := editHostColors(block, bracket)
	if err != nil || !back || b != block || r != bracket {
		t.Fatal("Back lost original colors")
	}
	scriptTUI(t, "clear", "save")
	b, r, back, err = editHostColors(block, bracket)
	if err != nil || back || b != "" || r != bracket {
		t.Fatal("Clear changed the other field")
	}
	scriptTUI(t, "block", "#ef4444", "save")
	b, r, back, err = editHostColors("Mixed", "Mixed")
	if err != nil || back || b != "#ef4444" || r != "Mixed" {
		t.Fatal("untouched bulk brackets changed")
	}
	scriptTUI(t, "bracket", "custom", "enter\n#ABCDEF", "apply", "save")
	b, r, back, err = editHostColors(block, bracket)
	if err != nil || back || b != block || r != "#abcdef" {
		t.Fatal("custom bracket", b, r, back, err)
	}
	s := testInventory(t, "Colors")
	h := InventoryHost{ID: "host", Alias: "host", Address: "192.0.2.1", Color: block}
	if err = saveInventoryHost(s, h, nil); err != nil {
		t.Fatal(err)
	}
	before, err := sourceInventoryHosts(s)
	if err != nil {
		t.Fatal(err)
	}
	if before[0].BracketColor != "" {
		t.Fatal("legacy default changed")
	}
	if err = saveHostFields(s, before, map[string]string{"bracket_color": bracket}); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	hosts, err := inventoryHosts(c)
	if err != nil || hosts[0].BracketColor != bracket || hosts[0].Color != block {
		t.Fatal(hosts, err)
	}
	s.ReadOnly = true
	before, _ = sourceInventoryHosts(s)
	if saveHostFields(s, before, map[string]string{"bracket_color": "#ffffff"}) == nil {
		t.Fatal("read-only write accepted")
	}
	s.ReadOnly = false
	h.BracketColor = "blue"
	if saveInventoryHost(s, h, nil) == nil {
		t.Fatal("invalid bracket accepted")
	}
	profile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(profile)
	lipgloss.SetColorProfile(termenv.TrueColor)
	marker := hostColorMarker(block, bracket)
	if ansi.Strip(marker) != "[■]" || !strings.Contains(marker, "38;2;59;130;246") || !strings.Contains(marker, "38;2;239;68;68") {
		t.Fatal(marker)
	}
}

func TestColorEditorStandardControls(t *testing.T) {
	old := runTUI
	defer func() { runTUI = old }()
	step := 0
	runTUI = func(model tea.Model) (tea.Model, error) {
		m := model.(*dialogModel)
		if len(m.options) != 2 || m.options[0].ID != "block" || m.options[1].ID != "bracket" {
			t.Fatal("actions must not be list entries", m.options)
		}
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "ctrl+s Save") || !strings.Contains(view, "delete Clear") || strings.Contains(view, "Apply") {
			t.Fatal("missing standard hints", view)
		}
		if step == 0 {
			m.Update(tea.KeyMsg{Type: tea.KeyDelete})
			if m.result != "clear" {
				t.Fatal("Delete did not clear")
			}
		} else {
			if m.swatch != "" || m.swatchBracket != "#3b82f6" {
				t.Fatal("Clear did not update preview")
			}
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
			if m.result != "save" {
				t.Fatal("Ctrl+S did not save")
			}
		}
		step++
		return m, nil
	}
	block, bracket, back, err := editHostColors("#ef4444", "#3b82f6")
	if err != nil || back || block != "" || bracket != "#3b82f6" || step != 2 {
		t.Fatal(block, bracket, back, err)
	}
	for _, action := range []struct {
		x  int
		id string
	}{{9, "save"}, {17, "clear"}} {
		m := newDialog("Host color", nil, "")
		m.footerActions = []choice{{"save", "Save"}, {"clear", "Clear"}}
		m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		m.Update(tea.MouseMsg{X: action.x, Y: m.footerY(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if m.result != action.id {
			t.Fatal("footer click", action.id, m.result)
		}
	}
}

func TestColorSaveIsIndependentOfHostSave(t *testing.T) {
	for _, saveParent := range []bool{false, true} {
		s := testInventory(t, "Color save")
		if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
			t.Fatal(err)
		}
		rows, _ := sourceConnections(s)
		path := filepath.Join(t.TempDir(), "profile.db")
		original, before, c, err := settingsDraft(path)
		if err != nil {
			t.Fatal(err)
		}
		c.Sources = []InventorySource{s}
		if err = saveSettingsDraft(path, original, before, c); err != nil {
			t.Fatal(err)
		}
		steps := []string{"notes", "enter\nunsaved note", "color", "block", "#ef4444", "bracket", "#3b82f6", "save"}
		if saveParent {
			steps = append(steps, "save")
		}
		steps = append(steps, "back")
		scriptTUI(t, steps...)
		if err = editPersonalHosts(c, path, []Host{{SourceID: s.ID, InventoryHostID: rows[0].HostID}}); err != nil {
			t.Fatal(err)
		}
		hosts, err := sourceInventoryHosts(s)
		if err != nil {
			t.Fatal(err)
		}
		if hosts[0].Color != "#ef4444" || hosts[0].BracketColor != "#3b82f6" {
			t.Fatal("colors needed second Save", hosts)
		}
		if saveParent && hosts[0].Notes != "unsaved note" || !saveParent && hosts[0].Notes != "" {
			t.Fatal("color Save affected unrelated draft", hosts)
		}
	}
}

func TestLegacyHostColorSavePreservesOtherDrafts(t *testing.T) {
	s := testInventory(t, "Legacy color save")
	if err := changeConnection(s, Connection{ID: "c", Alias: "host", Address: "192.0.2.1", Port: 22}, false); err != nil {
		t.Fatal(err)
	}
	rows, _ := sourceConnections(s)
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	scriptTUI(t, "notes", "enter\nunsaved", "color", "block", "#ef4444", "bracket", "#3b82f6", "save", "back")
	state := PickerState{Mode: "ssh", Source: s.ID}
	if err := editInventoryHost(c, &state, &Host{SourceID: s.ID, InventoryHostID: rows[0].HostID}); err != nil {
		t.Fatal(err)
	}
	hosts, err := sourceInventoryHosts(s)
	if err != nil || hosts[0].Color != "#ef4444" || hosts[0].BracketColor != "#3b82f6" || hosts[0].Notes != "" {
		t.Fatal(hosts, err)
	}
}

func TestClearBracketOnly(t *testing.T) {
	old := runTUI
	defer func() { runTUI = old }()
	step := 0
	runTUI = func(model tea.Model) (tea.Model, error) {
		m := model.(*dialogModel)
		if step == 0 {
			m.cursor = 1
			m.Update(tea.KeyMsg{Type: tea.KeyDelete})
		} else {
			m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
		}
		step++
		return m, nil
	}
	b, r, back, err := editHostColors("#ef4444", "#3b82f6")
	if err != nil || back || b != "#ef4444" || r != "none" {
		t.Fatal(b, r, back, err)
	}
}

func TestBracketModesRenderAndPersist(t *testing.T) {
	s := testInventory(t, "Bracket modes")
	for _, mode := range []string{"", "same", "none", "#3b82f6"} {
		h := InventoryHost{ID: "host", Alias: "host", Address: "192.0.2.1", Color: "#ef4444", BracketColor: mode}
		if err := saveInventoryHost(s, h, nil); err != nil {
			t.Fatal(mode, err)
		}
		rows, err := sourceInventoryHosts(s)
		if err != nil || rows[0].BracketColor != mode {
			t.Fatal(mode, rows, err)
		}
		marker := hostColorMarker(h.Color, mode)
		want := "[■]"
		if mode == "none" {
			want = " ■ "
		}
		if ansi.StringWidth(marker) != 3 || ansi.Strip(marker) != want {
			t.Fatal(mode, marker)
		}
	}
	if validateBracketColor("contrast") == nil || validateHostColor("same") == nil {
		t.Fatal("invalid modes accepted outside bracket field")
	}
	profile := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(profile)
	lipgloss.SetColorProfile(termenv.TrueColor)
	if hostColorMarker("#ef4444", "same") != hostColorMarker("#ef4444", "#ef4444") {
		t.Fatal("same mode does not follow block")
	}
	for _, mode := range []string{"same", "none", ""} {
		scriptTUI(t, mode)
		value, back, err := choosePaletteColor("#3b82f6", true)
		if err != nil || back || value != mode {
			t.Fatal("mode selection", mode, value, back, err)
		}
	}
}

func TestPaletteColumnsSortFilterAndResize(t *testing.T) {
	if len(paletteOptions(false)) != 27 || len(paletteOptions(true)) != 15 {
		t.Fatal("wrong palette size")
	}
	for _, width := range []int{40, 80, 120} {
		m := newDialog("Block color", paletteOptions(false), "")
		m.palette, m.colorChoices, m.singleChoice = true, true, true
		m.current = "#ef4444"
		m.refilter()
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		for _, column := range []int{1, 2, 0} {
			m.View()
			found := false
			for _, hit := range m.resizeHits {
				if hit.action == "palette-sort" && hit.index == column {
					selected := m.options[m.filtered[m.cursor]].ID
					m.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
					if m.options[m.filtered[m.cursor]].ID != selected || m.done || m.resizeKind != "" {
						t.Fatal("sort lost selection or triggered action")
					}
					found = true
					break
				}
			}
			if !found || m.paletteSort.Column != column {
				t.Fatal("missing header", column, width)
			}
		}
		if lipgloss.Width(m.View()) > width {
			t.Fatal("narrow overflow", width, m.View())
		}
		m.paletteSort = pickerSort{Column: 0}
		m.refilter()
		m.View()
		for _, hit := range m.resizeHits {
			if hit.action == "palette-sort" && hit.index == 0 {
				m.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				if !m.paletteSort.Descending {
					t.Fatal("second click did not reverse")
				}
				break
			}
		}
		m.paletteSort = pickerSort{Column: 1}
		m.refilter()
		if m.options[m.filtered[0]].ID != "custom" || m.options[m.filtered[1]].ID != "" || m.options[m.filtered[2]].Label != "Amber" {
			t.Fatal("name sorting/special actions")
		}
		m.query.SetValue("Blue")
		m.refilter()
		if len(m.filtered) != 1 {
			t.Fatal("filter")
		}
		m.query.SetValue("")
		m.refilter()
		m.View()
		for _, hit := range m.resizeHits {
			if hit.action == "column" {
				before := m.paletteSort
				m.Update(tea.MouseMsg{X: hit.x, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				m.Update(tea.MouseMsg{X: hit.x + 2, Y: hit.y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
				m.Update(tea.MouseMsg{Action: tea.MouseActionRelease})
				if m.paletteSort != before || m.layout.labelWidth == 0 {
					t.Fatal("divider sorting/resize")
				}
				break
			}
		}
	}
	if paletteHue("#ef4444") >= paletteHue("#3b82f6") || paletteHue("#ffffff") < 360 {
		t.Fatal("hue ordering")
	}
}
