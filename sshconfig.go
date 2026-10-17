package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type Host struct {
	Alias    string
	Hostname string
	JumpHost string
	Comment  string
	Notes    []string
	File     string
	Line     int
	Block    string
}

type hostBlock struct {
	hosts    []Host
	comment  []string
	hostname string
	jumpHost string
}

type configReader struct {
	home    string
	base    string
	active  map[string]bool
	blocks  []*hostBlock
	current *hostBlock
	inMatch bool
}

func readHosts(path string) ([]Host, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(expandHome(path, home))
	if err != nil {
		return nil, err
	}
	r := configReader{home: home, base: filepath.Join(home, ".ssh"), active: map[string]bool{}}
	if err := r.read(path); err != nil {
		return nil, err
	}
	var hosts []Host
	seen := map[string]bool{}
	for _, b := range r.blocks {
		for _, h := range b.hosts {
			if !seen[h.Alias] {
				h.Hostname = b.hostname
				h.JumpHost = b.jumpHost
				if h.Hostname == "" {
					h.Hostname = h.Alias
				}
				h.Comment = strings.Join(b.comment, " · ")
				h.Notes = b.comment
				hosts = append(hosts, h)
				seen[h.Alias] = true
			}
		}
	}
	return hosts, nil
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func (r *configReader) read(path string) error {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if r.active[canonical] || len(r.active) >= 32 {
		return fmt.Errorf("SSH Include cycle or nesting limit at %s", path)
	}
	r.active[canonical] = true
	defer delete(r.active, canonical)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var lines []string
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 4096), 1024*1024)
	for s.Scan() {
		lines = append(lines, s.Text())
	}
	if err := s.Err(); err != nil {
		return err
	}
	for i, line := range lines {
		fields, comment, err := sshFields(line)
		if err != nil {
			return fmt.Errorf("%s:%d: %w", path, i+1, err)
		}
		if len(fields) == 0 {
			if text := description(comment); text != "" {
				if r.current != nil && len(line) > 0 && (line[0] == ' ' || line[0] == '\t') {
					r.current.comment = append(r.current.comment, text)
				}
			}
			continue
		}
		key := strings.ToLower(fields[0])
		if key == "host" {
			b := &hostBlock{}
			end := i + 1
			for end < len(lines) {
				f, _, _ := sshFields(lines[end])
				if len(f) > 0 && (strings.EqualFold(f[0], "host") || strings.EqualFold(f[0], "match")) {
					break
				}
				end++
			}
			for _, alias := range fields[1:] {
				if validAlias(alias) {
					b.hosts = append(b.hosts, Host{Alias: alias, File: path, Line: i + 1, Block: strings.Join(lines[i:end], "\n")})
				}
			}
			r.blocks = append(r.blocks, b)
			r.current, r.inMatch = b, false
		}
		if key == "match" {
			r.current, r.inMatch = nil, true
		}
		if r.current != nil {
			if text := description(comment); text != "" {
				r.current.comment = append(r.current.comment, text)
			}
			if key == "hostname" && len(fields) > 1 && r.current.hostname == "" {
				r.current.hostname = fields[1]
			}
			if len(fields) > 1 && r.current.jumpHost == "" {
				switch key {
				case "proxyjump":
					r.current.jumpHost = fields[1]
				case "proxycommand":
					r.current.jumpHost = "ProxyCommand"
					if strings.EqualFold(fields[1], "none") {
						r.current.jumpHost = "none"
					}
				}
			}
		}
		if key == "include" {
			// ponytail: don't evaluate Match conditions or Match exec; conditional
			// includes need an explicit discovery policy before being supported.
			if r.inMatch {
				return fmt.Errorf("%s:%d: Include inside Match is not supported for discovery", path, i+1)
			}
			for _, pattern := range fields[1:] {
				pattern = expandHome(pattern, r.home)
				if !filepath.IsAbs(pattern) {
					pattern = filepath.Join(r.base, pattern)
				}
				matches, err := filepath.Glob(pattern)
				if err != nil {
					return fmt.Errorf("%s:%d: %w", path, i+1, err)
				}
				for _, match := range matches {
					if err := r.read(match); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func validAlias(s string) bool {
	return s != "" && !strings.ContainsAny(s, "*?![]\t\r\n ") && !strings.HasPrefix(s, "-") && !strings.ContainsFunc(s, unicode.IsControl)
}

// A commented SSH directive is configuration, not a descriptive label.
func description(s string) string {
	s = strings.TrimSpace(s)
	f, _, _ := sshFields(s)
	if len(f) > 0 {
		switch strings.ToLower(f[0]) {
		case "host", "hostname", "user", "port", "identityfile", "identitiesonly", "proxyjump", "proxycommand", "include", "match", "forwardagent", "localforward", "remoteforward", "dynamicforward", "addkeystoagent", "serveraliveinterval", "serveralivecountmax", "stricthostkeychecking", "userknownhostsfile", "preferredauthentications", "pubkeyauthentication", "passwordauthentication", "requesttty", "remotecommand", "controlmaster", "controlpath", "controlpersist", "kexalgorithms", "hostkeyalgorithms", "pubkeyacceptedalgorithms", "ciphers", "macs", "loglevel", "setenv", "sendenv":
			return ""
		}
	}
	if strings.Trim(s, " -=_#─═") == "" {
		return ""
	}
	return s
}

// Tokenize the SSH config's quoting, escapes, optional '=' and inline comments.
// This discovers hosts; OpenSSH remains responsible for evaluating connections.
func sshFields(line string) ([]string, string, error) {
	var fields []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	comment := ""
	flush := func() {
		if started {
			fields = append(fields, word.String())
			word.Reset()
			started = false
		}
	}
	for i, ch := range line {
		if escaped {
			word.WriteRune(ch)
			escaped = false
		} else if ch == '\\' {
			escaped, started = true, true
		} else if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				word.WriteRune(ch)
			}
		} else if ch == '"' || ch == '\'' {
			quote, started = ch, true
		} else if ch == '#' {
			comment = line[i+1:]
			break
		} else if unicode.IsSpace(ch) || (ch == '=' && len(fields) == 0) {
			flush()
		} else if ch == '=' && len(fields) == 1 && !started {
			continue
		} else {
			word.WriteRune(ch)
			started = true
		}
	}
	if quote != 0 || escaped {
		return nil, "", fmt.Errorf("unfinished quote or escape")
	}
	flush()
	return fields, comment, nil
}
