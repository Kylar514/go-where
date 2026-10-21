package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func entryNames(entries []kexEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

func TestKexCandidatesComeFromLocalOpenSSH(t *testing.T) {
	calls := fakeKexSupport(t, testKexSupported, testKexDefaults)
	entries, note := kexCandidates(nil)
	// Defaults are already on, so only the supported-but-disabled names remain.
	want := []string{"diffie-hellman-group-exchange-sha1", "diffie-hellman-group1-sha1", "diffie-hellman-group14-sha1"}
	if !reflect.DeepEqual(entryNames(entries), want) || note != "" {
		t.Fatalf("candidates %v note %q", entryNames(entries), note)
	}
	for _, e := range entries {
		if !e.Weak || !strings.Contains(e.Note, "weak") || e.Unsupported {
			t.Fatalf("SHA-1 exchange not flagged weak: %+v", e)
		}
	}
	// A stored default and a stored name this machine lacks both stay visible.
	entries, _ = kexCandidates([]string{"made-up-kex", "diffie-hellman-group14-sha256", "diffie-hellman-group1-sha1"})
	want = []string{"diffie-hellman-group14-sha256", "diffie-hellman-group-exchange-sha1", "diffie-hellman-group1-sha1", "diffie-hellman-group14-sha1", "made-up-kex"}
	if !reflect.DeepEqual(entryNames(entries), want) {
		t.Fatalf("order must be strong, weak, then unusable: %v", entryNames(entries))
	}
	if !strings.Contains(entries[0].Note, "already enabled") || !entries[4].Unsupported || !strings.Contains(entries[4].Note, "not supported") {
		t.Fatalf("notes wrong: %+v %+v", entries[0], entries[4])
	}
	// One query pair per process, however often Details or the picker ask.
	for i := 0; i < 5; i++ {
		kexCandidates(nil)
		if err := checkKexSupported([]string{"diffie-hellman-group1-sha1"}); err != nil {
			t.Fatal(err)
		}
	}
	if *calls != 2 {
		t.Fatalf("ssh queried %d times; want one -Q and one -G", *calls)
	}
	resetKexCache()
	kexCandidates(nil)
	if *calls != 4 {
		t.Fatal("reset did not re-query")
	}
}

func TestKexFallbacksWhenSSHCannotBeQueried(t *testing.T) {
	old := sshOutput
	t.Cleanup(func() { sshOutput = old; resetKexCache() })
	sshOutput = func(...string) ([]byte, error) { return nil, fmt.Errorf("ssh: not found") }
	resetKexCache()
	entries, note := kexCandidates([]string{"custom-kex"})
	if !reflect.DeepEqual(entryNames(entries), []string{"custom-kex", "diffie-hellman-group-exchange-sha1", "diffie-hellman-group1-sha1", "diffie-hellman-group14-sha1"}) || !strings.Contains(note, "could not be queried") {
		t.Fatalf("built-in fallback wrong: %v %q", entryNames(entries), note)
	}
	for _, e := range entries {
		if e.Unsupported {
			t.Fatal("cannot call a name unsupported without a successful query")
		}
	}
	if err := checkKexSupported([]string{"anything"}); err != nil {
		t.Fatal("an unqueryable ssh must never block a launch")
	}
	// -Q works but -G does not: all supported names are offered, with an explanation.
	sshOutput = func(args ...string) ([]byte, error) {
		if args[0] == "-Q" {
			return []byte("a-sha256\nb-sha1\n"), nil
		}
		return nil, fmt.Errorf("no -G")
	}
	resetKexCache()
	entries, note = kexCandidates(nil)
	if !reflect.DeepEqual(entryNames(entries), []string{"a-sha256", "b-sha1"}) || !strings.Contains(note, "default list could not be read") {
		t.Fatalf("unreadable defaults: %v %q", entryNames(entries), note)
	}
}

func TestLaunchNamesUnsupportedKexBeforeOpenSSHFails(t *testing.T) {
	fakeKexSupport(t, testKexSupported, testKexDefaults)
	s := testInventory(t, "Unsupported")
	r := Connection{ID: "server", Alias: "server", Address: "192.0.2.1", Port: 22, ExtraKexAlgorithms: []string{"diffie-hellman-group1-sha1", "made-up-kex"}}
	if err := changeConnection(s, r, false); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	c.Sources = []InventorySource{s}
	h := Host{Alias: r.Alias, Type: "ssh", SourceID: s.ID, ConnectionID: r.ID}
	_, err := managedJob(h, c)
	if err == nil || !strings.Contains(err.Error(), `"made-up-kex"`) || !strings.Contains(err.Error(), "ssh -Q kex") {
		t.Fatalf("launch error must name the algorithm: %v", err)
	}
	if details := managedDetails(h, c); !strings.Contains(details, "made-up-kex") {
		t.Fatalf("Details must show the same error: %s", details)
	}
	// A local override replaces the unusable database entry.
	only := []string{"diffie-hellman-group1-sha1"}
	c.Overrides = map[string]map[string]ConnectionOverride{s.ID: {r.ID: {ExtraKexAlgorithms: &only}}}
	if _, err = managedJob(h, c); err != nil {
		t.Fatalf("override did not resolve the unsupported name: %v", err)
	}
}

func TestChooseKexChecklistFlow(t *testing.T) {
	fakeKexSupport(t, testKexSupported, testKexDefaults)
	// Toggle, type a name manually, then drop a stored name. Result follows list order.
	seen := scriptTUI(t, "toggle:diffie-hellman-group1-sha1,manual", "enter\nx-custom-kex", "toggle:made-up-kex,done")
	got, back, err := chooseKex("Test", "intro", []string{"made-up-kex"})
	if err != nil || back || !reflect.DeepEqual(got, []string{"diffie-hellman-group1-sha1", "x-custom-kex"}) {
		t.Fatalf("flow result %v back=%v err=%v", got, back, err)
	}
	if !reflect.DeepEqual(*seen, []string{"multi:Test", "path:", "multi:Test"}) {
		t.Fatalf("dialog sequence: %v", *seen)
	}
	// Invalid manual names are refused with a notice and change nothing.
	scriptTUI(t, "manual", "enter\n+diffie-hellman-group1-sha1", "back", "manual", "enter\nspaces not allowed", "back", "done")
	got, back, err = chooseKex("Test", "", []string{"diffie-hellman-group1-sha1"})
	if err != nil || back || !reflect.DeepEqual(got, []string{"diffie-hellman-group1-sha1"}) {
		t.Fatalf("invalid manual entry altered the selection: %v %v", got, err)
	}
	// Backing out of the name prompt returns to the list with state intact.
	scriptTUI(t, "toggle:diffie-hellman-group14-sha1,manual", "back\nignored", "done")
	got, _, err = chooseKex("Test", "", nil)
	if err != nil || !reflect.DeepEqual(got, []string{"diffie-hellman-group14-sha1"}) {
		t.Fatalf("toggle lost after prompt back-out: %v %v", got, err)
	}
	// Esc discards every toggle made in the dialog.
	scriptTUI(t, "toggle:diffie-hellman-group1-sha1,back")
	got, back, err = chooseKex("Test", "", []string{"diffie-hellman-group14-sha1"})
	if err != nil || !back || got != nil {
		t.Fatalf("cancel must discard: %v %v %v", got, back, err)
	}
	// Unchecking everything is an explicit empty list, never nil.
	scriptTUI(t, "toggle:diffie-hellman-group14-sha1,done")
	got, back, err = chooseKex("Test", "", []string{"diffie-hellman-group14-sha1"})
	if err != nil || back || got == nil || len(got) != 0 {
		t.Fatalf("empty selection must be a non-nil empty list: %#v", got)
	}
}

func TestMultiDialogKeysFilterAndPinnedRows(t *testing.T) {
	items := []multiItem{{ID: "a", Label: "alpha"}, {ID: "b", Label: "beta", Note: "⚠ weak (SHA-1)"}, {ID: "c", Label: "gamma", Checked: true}}
	m := newMultiDialog("Pick", items, "details")
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	view := m.View()
	for _, want := range []string{"[ ] alpha", "[ ] beta", "⚠ weak (SHA-1)", "[✓] gamma", "Done — apply 1 selected", "Type a name manually"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in:\n%s", want, view)
		}
	}
	// Space toggles and must not be typed into the search box.
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.items[0].Checked || m.query.Value() != "" || m.done {
		t.Fatal("space must toggle the highlighted row only")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.items[0].Checked {
		t.Fatal("enter must toggle too")
	}
	// Filtering keeps pinned rows reachable and acts on the filtered row.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("beta")})
	if len(m.filtered) != 1 || m.items[m.filtered[0]].ID != "b" {
		t.Fatalf("filter: %v", m.filtered)
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !m.items[1].Checked || m.items[0].Checked {
		t.Fatal("toggle hit the wrong row under a filter")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Fatalf("cursor must stop on the last pinned row, got %d", m.cursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.done || m.action != "manual" {
		t.Fatal("last pinned row is manual entry")
	}
	m = newMultiDialog("Pick", items, "")
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != len(m.filtered) {
		t.Fatal("down should reach the pinned Done row")
	}
	m.cursor = len(m.filtered)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.done || m.action != "done" || m.back {
		t.Fatal("first pinned row applies")
	}
	m = newMultiDialog("Pick", items, "")
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 20})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.done || !m.back {
		t.Fatal("Esc must cancel")
	}
}

func TestMultiDialogMouseAndSmallTerminals(t *testing.T) {
	var items []multiItem
	for i := 0; i < 40; i++ {
		items = append(items, multiItem{ID: fmt.Sprint("kex-", i), Label: fmt.Sprintf("kex-%02d", i)})
	}
	text := strings.Repeat("long explanatory line\n", 30)
	m := newMultiDialog("Many", items, text)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	view := m.View()
	press := func(row int) {
		m.Update(tea.MouseMsg{X: 4, Y: m.listStart() + row, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	}
	press(m.itemRows() - 2)
	if !m.items[1].Checked || m.done {
		t.Fatal("clicking a row must toggle it")
	}
	press(m.itemRows())
	if !m.done || m.action != "done" {
		t.Fatal("clicking the Done row must apply")
	}
	m = newMultiDialog("Many", items, text)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	press(m.itemRows() + 1)
	if m.action != "manual" {
		t.Fatal("clicking the manual row must open entry")
	}
	m = newMultiDialog("Many", items, text)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	for i := 0; i < 60; i++ { // wheel scrolls the list, clamped to the pinned rows
		m.Update(tea.MouseMsg{X: 4, Y: m.listStart() + 1, Button: tea.MouseButtonWheelDown})
	}
	if m.cursor != len(m.filtered)+1 {
		t.Fatalf("wheel cursor %d", m.cursor)
	}
	if v := m.View(); !strings.Contains(v, "Done — apply 0 selected") {
		t.Fatalf("pinned rows must stay visible after scrolling:\n%s", v)
	}
	m.Update(tea.MouseMsg{X: 2, Y: m.height - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !m.back {
		t.Fatal("clicking Back must cancel")
	}
	_ = view
	for _, size := range [][2]int{{80, 40}, {60, 24}, {40, 14}, {24, 12}, {24, 10}} {
		m = newMultiDialog("Many", items, text)
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		v := m.View()
		if lipgloss.Height(v) > size[1] || lipgloss.Width(v) > size[0] {
			t.Fatalf("dialog overflows %v: %dx%d", size, lipgloss.Width(v), lipgloss.Height(v))
		}
		if !strings.Contains(v, "Done — apply") || !strings.Contains(v, "Back") {
			t.Fatalf("pinned rows or Back clipped at %v:\n%s", size, v)
		}
	}
	// Too small to show the pinned rows: refuse input rather than act unseen.
	m = newMultiDialog("Many", items, text)
	m.Update(tea.WindowSizeMsg{Width: 24, Height: 9})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.done || m.items[0].Checked || !strings.Contains(m.View(), "Resize") {
		t.Fatal("hidden actions must not run in a tiny terminal")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.back {
		t.Fatal("Esc must still work when tiny")
	}
}
