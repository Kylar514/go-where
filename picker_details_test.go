package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLatestOnlyDetailsRefresh(t *testing.T) {
	hosts := []Host{
		{Alias: "one", SourceID: "test", ConnectionID: "one", Type: "ssh"},
		{Alias: "two", SourceID: "test", ConnectionID: "two", Type: "ssh"},
		{Alias: "three", SourceID: "test", ConnectionID: "three", Type: "ssh"},
	}
	m := newPickerModel(hosts, "", "current", Origin{}, PickerState{Mode: "ssh"}, defaultConfig())
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	first := m.detailsRevision
	if m.cursor != 1 || m.detailsBusy || m.detailsText != "Loading Details…" {
		t.Fatal("navigation waited for Details")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	second := m.detailsRevision
	if _, cmd := m.Update(detailsSettled{first}); cmd != nil || m.detailsBusy {
		t.Fatal("stale timer started work")
	}
	_, work := m.Update(detailsSettled{second})
	if work == nil || !m.detailsBusy {
		t.Fatal("latest timer did not start work")
	}
	if _, cmd := m.Update(detailsSettled{second}); cmd != nil {
		t.Fatal("two refreshes in flight")
	}

	// More navigation must work while the first resolver is still running.
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	latest := m.detailsRevision
	m.Update(detailsSettled{latest})
	_, next := m.Update(detailsResult{second, "stale host"})
	if strings.Contains(m.detailsText, "stale") || next == nil || !m.detailsBusy || m.cursor != 1 {
		t.Fatal("stale result applied or newest request lost")
	}
	m.Update(detailsResult{latest, "current host details"})
	if m.detailsText != "current host details" || m.detailsBusy {
		t.Fatal("latest result missing")
	}

	// Launch/edit uses the selected host, even before display generation completes.
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.key != "ctrl-e" || len(m.selected) != 1 || m.selected[0].Alias != "three" {
		t.Fatal("edit waited for or used stale Details")
	}
}

func TestAsyncDetailsSnapshotAndResize(t *testing.T) {
	m := newPickerModel([]Host{{Alias: "one", SourceID: "missing", Type: "ssh"}}, "", "current", Origin{}, PickerState{Mode: "ssh"}, defaultConfig())
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	_, work := m.Update(detailsSettled{m.detailsRevision})
	if work == nil {
		t.Fatal("missing worker")
	}
	result := work().(detailsResult)
	if m.detailsText != "Loading Details…" {
		t.Fatal("worker mutated UI")
	}
	m.Update(result)
	if !strings.Contains(m.detailsText, "source missing is not configured") {
		t.Fatal(result.text)
	}
	revision := m.detailsRevision
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	if m.detailsRevision != revision || m.detailsBusy {
		t.Fatal("resize reloaded inventory")
	}
}
