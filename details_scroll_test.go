package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestDetailsScrollIndicator(t *testing.T) {
	v := viewport.New(24, 5)
	v.SetContent("short")
	if strings.Contains(detailsPanel(v), "█") || strings.Contains(detailsPanel(v), "░") {
		t.Fatal("indicator shown for fitting content")
	}
	v.SetContent(strings.Repeat("line\n", 30))
	top := strings.Split(ansi.Strip(detailsPanel(v)), "\n")
	if !strings.Contains(top[1], "█") || !strings.Contains(top[5], "░") {
		t.Fatal("incorrect top position", top)
	}
	v.GotoBottom()
	bottom := strings.Split(ansi.Strip(detailsPanel(v)), "\n")
	if !strings.Contains(bottom[1], "░") || !strings.Contains(bottom[5], "█") {
		t.Fatal("incorrect bottom position", bottom)
	}
	if lipgloss.Width(detailsPanel(v)) != 28 || lipgloss.Height(detailsPanel(v)) != 7 {
		t.Fatal("indicator changed panel size")
	}
}
