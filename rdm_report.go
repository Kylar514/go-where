package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type rdmEntry struct{ Name, Address, Group string }

// Entries are found by shape (ConnectionType 1 = RDP, Url = address), so the
// report does not depend on how the export nests folders.
func collectRDM(node any, out *[]rdmEntry) {
	switch v := node.(type) {
	case map[string]any:
		if t, ok := v["ConnectionType"].(float64); ok && t == 1 {
			if addr, _ := v["Url"].(string); strings.TrimSpace(addr) != "" {
				name, _ := v["Name"].(string)
				group, _ := v["Group"].(string)
				*out = append(*out, rdmEntry{name, addr, group})
			}
		}
		for _, child := range v {
			collectRDM(child, out)
		}
	case []any:
		for _, child := range v {
			collectRDM(child, out)
		}
	}
}

func normalizeAddress(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if host, port, ok := strings.Cut(s, ":"); ok && !strings.Contains(port, ":") {
		if _, err := strconv.Atoi(port); err == nil {
			return host
		}
	}
	return s
}

// The report only reads: it proposes tags and never edits the SSH config.
func rdmReport(data []byte, hosts []Host, tags []string, out io.Writer) error {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("not valid JSON")
	}
	var entries []rdmEntry
	collectRDM(doc, &entries)
	if len(entries) == 0 {
		return fmt.Errorf("no RDP entries found (ConnectionType 1 with a Url); is this an RDM JSON export?")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	byAddress := map[string][]rdmEntry{}
	for _, e := range entries {
		byAddress[normalizeAddress(e.Address)] = append(byAddress[normalizeAddress(e.Address)], e)
	}
	known := map[string]bool{}
	var missing, orphans []string
	tagged := 0
	for _, h := range hosts {
		address := normalizeAddress(h.Hostname)
		known[address] = true
		matches := byAddress[address]
		switch {
		case h.isRDP(tags):
			tagged++
			if len(matches) == 0 {
				orphans = append(orphans, fmt.Sprintf("  %s  %s", clean(h.Alias), clean(h.Hostname)))
			}
		case len(matches) > 0:
			missing = append(missing, fmt.Sprintf("  %s  %s  %s:%d  (RDM: %s)", clean(h.Alias), clean(h.Hostname), clean(filepath.Base(h.File)), h.Line, clean(matches[0].Name)))
		}
	}
	fmt.Fprintf(out, "RDM RDP entries: %d · SSH hosts: %d · tagged for RDP: %d\n", len(entries), len(hosts), tagged)
	section := func(title string, lines []string, limit int) {
		fmt.Fprintf(out, "\n%s: %d\n", title, len(lines))
		for i, line := range lines {
			if i == limit {
				fmt.Fprintf(out, "  … and %d more\n", len(lines)-limit)
				break
			}
			fmt.Fprintln(out, line)
		}
	}
	section("Add a '# "+strings.ToUpper(tags[0][:1])+tags[0][1:]+"' comment (address matches an RDM RDP entry)", missing, -1)
	section("Tagged, but no RDM RDP entry has that address", orphans, -1)
	var absent []string
	for _, e := range entries {
		if !known[normalizeAddress(e.Address)] {
			absent = append(absent, fmt.Sprintf("  %s  %s  %s", clean(e.Name), clean(e.Address), clean(e.Group)))
		}
	}
	section("RDM RDP entries with no SSH host", absent, 100)
	fmt.Fprintln(out, "\nNothing was changed. Edit your SSH config yourself, then rerun to confirm.")
	return nil
}

func rdmReportCommand(args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	settings, err := configPath()
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("ssh-pick rdm-report", flag.ContinueOnError)
	sshConfig := fs.String("ssh-config", filepath.Join(home, ".ssh", "config"), "SSH config to compare against")
	config := fs.String("config", settings, "picker settings (JSON)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: ssh-pick rdm-report [--ssh-config path] EXPORT.json")
	}
	c, err := loadConfig(expandHome(*config, home))
	if err != nil {
		return err
	}
	hosts, err := readHosts(*sshConfig)
	if err != nil {
		return err
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		return err
	}
	defer f.Close()
	const limit = 64 << 20
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return err
	}
	if len(data) > limit {
		return fmt.Errorf("export exceeds the 64 MiB limit")
	}
	return rdmReport(data, hosts, c.RDP.Tags, os.Stdout)
}
