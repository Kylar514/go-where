package main

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// sshOutput runs the local OpenSSH client for capability queries. Tests replace it.
var sshOutput = func(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "ssh", args...).Output()
}

// builtinLegacyKex is offered only when the local ssh cannot be queried.
var builtinLegacyKex = []string{"diffie-hellman-group1-sha1", "diffie-hellman-group14-sha1", "diffie-hellman-group-exchange-sha1"}

type kexCapabilities struct {
	known     bool            // ssh -Q kex succeeded
	supported map[string]bool // everything this OpenSSH can negotiate
	defaults  map[string]bool // enabled without "+"; empty when it could not be read
}

var (
	kexMu    sync.Mutex
	kexCache = map[string]kexCapabilities{}
)

func resetKexCache() {
	kexMu.Lock()
	kexCache = map[string]kexCapabilities{}
	kexMu.Unlock()
}

// kexSupport queries OpenSSH once per process: the answer does not change while
// the TUI runs, and Details refreshes on every cursor move.
func kexSupport() kexCapabilities {
	return algorithmSupport("KexAlgorithms")
}

func algorithmSupport(directive string) kexCapabilities {
	kexMu.Lock()
	defer kexMu.Unlock()
	if caps, ok := kexCache[directive]; ok {
		return caps
	}
	query := directive
	if directive == "KexAlgorithms" {
		query = "kex"
	}
	caps := kexCapabilities{supported: map[string]bool{}, defaults: map[string]bool{}}
	if out, err := sshOutput("-Q", query); err == nil {
		for _, name := range strings.Fields(string(out)) {
			caps.supported[name] = true
		}
		caps.known = len(caps.supported) > 0
	}
	if caps.known {
		// -F none skips every config file, so this is OpenSSH's own default list.
		if out, err := sshOutput("-G", "-F", "none", "go-where-capability-probe"); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				key, value, _ := strings.Cut(strings.TrimSpace(line), " ")
				if strings.EqualFold(key, directive) {
					for _, name := range strings.Split(strings.TrimSpace(value), ",") {
						if name != "" {
							caps.defaults[name] = true
						}
					}
				}
			}
		}
	}
	kexCache[directive] = caps
	return caps
}

// A name OpenSSH cannot negotiate makes ssh abort ("Bad SSH2 KexAlgorithms"),
// so say which one before launching. An unqueryable ssh never blocks a launch.
func checkKexSupported(names []string) error {
	return checkAlgorithmsSupported("KexAlgorithms", names)
}

func checkAlgorithmsSupported(directive string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	caps := algorithmSupport(directive)
	if !caps.known {
		return nil
	}
	for _, name := range names {
		if !caps.supported[name] {
			query, label := directive, directive
			if directive == "KexAlgorithms" {
				query, label = "kex", "KEX"
			}
			return fmt.Errorf("additional %s %q is not supported by this machine's OpenSSH (see ssh -Q %s); remove it in SSH compatibility or override it locally", label, name, query)
		}
	}
	return nil
}

type kexEntry struct {
	Name        string
	Note        string
	Weak        bool
	Unsupported bool
}

func weakKex(name string) bool { return strings.Contains(name, "sha1") }

// kexCandidates lists what "+" can usefully add: supported, but not enabled by
// default. Names already stored stay listed even when this machine cannot use
// them, so opening the picker never silently drops a shared record's setting.
func kexCandidates(current []string) ([]kexEntry, string) {
	return algorithmCandidates("KexAlgorithms", current)
}

func algorithmCandidates(directive string, current []string) ([]kexEntry, string) {
	caps := algorithmSupport(directive)
	var names []string
	note := ""
	switch {
	case !caps.known:
		if directive == "KexAlgorithms" {
			names = append(names, builtinLegacyKex...)
			note = "ssh -Q kex could not be queried, so a built-in list of common legacy exchanges is shown. Use \"Type a name manually\" for others."
		} else {
			names = append(names, "ssh-rsa")
			note = "ssh -Q " + directive + " could not be queried; ssh-rsa is shown as a legacy fallback, with support unverified. Use manual entry for other names."
		}
	default:
		for name := range caps.supported {
			if !caps.defaults[name] {
				names = append(names, name)
			}
		}
		if len(caps.defaults) == 0 {
			note = "OpenSSH's default list could not be read, so default-enabled algorithms are shown too; adding them is harmless."
		}
	}
	have := map[string]bool{}
	for _, name := range names {
		have[name] = true
	}
	for _, name := range current {
		if !have[name] {
			names = append(names, name)
			have[name] = true
		}
	}
	entries := make([]kexEntry, 0, len(names))
	for _, name := range names {
		weak := weakKex(name)
		if directive != "KexAlgorithms" {
			weak = name == "ssh-rsa" || name == "ssh-dss" || strings.HasPrefix(name, "ssh-rsa-cert-") || strings.HasPrefix(name, "ssh-dss-cert-")
		}
		e := kexEntry{Name: name, Weak: weak}
		var notes []string
		if e.Weak {
			notes = append(notes, "⚠ weak (SHA-1)")
		}
		switch {
		case caps.known && !caps.supported[name]:
			e.Unsupported = true
			notes = append(notes, "not supported by this machine's OpenSSH")
		case caps.defaults[name]:
			notes = append(notes, "already enabled by default")
		}
		e.Note = strings.Join(notes, " · ")
		entries = append(entries, e)
	}
	// Supported strong first, then weak, then unusable names; alphabetical within.
	rank := func(e kexEntry) int {
		switch {
		case e.Unsupported:
			return 2
		case e.Weak:
			return 1
		}
		return 0
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if ri, rj := rank(entries[i]), rank(entries[j]); ri != rj {
			return ri < rj
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, note
}

// chooseKex shows a searchable checklist. Esc discards every change made in it.
func chooseKex(title, intro string, current []string) ([]string, bool, error) {
	return chooseAlgorithms("KexAlgorithms", title, intro, current)
}

func chooseAlgorithms(directive, title, intro string, current []string) ([]string, bool, error) {
	selected := map[string]bool{}
	for _, name := range current {
		selected[name] = true
	}
	listed := append([]string(nil), current...)
	position := menuPosition{}
	for {
		entries, note := algorithmCandidates(directive, listed)
		items := make([]multiItem, len(entries))
		for i, e := range entries {
			items[i] = multiItem{ID: e.Name, Label: e.Name, Note: e.Note, Checked: selected[e.Name]}
		}
		text := intro
		if note != "" {
			text += "\n\n" + note
		}
		m := newMultiDialog(title, items, text)
		m.identitySelection = true
		m.query.SetValue(position.query)
		m.refilter()
		m.cursor = min(position.cursor, len(m.filtered)+1)
		m.restorePreview = &position.preview
		if _, err := runTUI(m); err != nil {
			return nil, false, err
		}
		if m.back {
			return nil, true, nil
		}
		position = menuPosition{query: m.query.Value(), cursor: m.cursor, preview: m.preview.YOffset}
		for _, item := range m.items {
			selected[item.ID] = item.Checked
		}
		switch m.action {
		case "done":
			result := make([]string, 0) // explicit empty, never nil
			for _, e := range entries {
				if selected[e.Name] {
					result = append(result, e.Name)
				}
			}
			return result, false, nil
		case "manual":
			example := "ssh-rsa"
			if directive == "KexAlgorithms" {
				example = "diffie-hellman-group1-sha1"
			}
			text, back, err := inventoryPrompt("Algorithm name (bare name, e.g. "+example+")", "")
			if err != nil {
				return nil, false, err
			}
			if back {
				continue
			}
			name := strings.TrimSpace(text)
			if err = validateExtraAlgorithms(directive, []string{name}); err != nil {
				if err = notice(err.Error()); err != nil {
					return nil, false, err
				}
				continue
			}
			listed = append(listed, name)
			selected[name] = true
		}
	}
}
