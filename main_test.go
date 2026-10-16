package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func fakeCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	writeTestFile(t, path, "#!/bin/sh\n"+body+"\n")
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
}

func TestSSHDiscovery(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".ssh", "config")
	writeTestFile(t, root, `# Personal servers
Host home home-alias *.invalid !disabled # living room
    HostName = 192.0.2.10
    # IdentityFile ~/.ssh/testing
    # Main machine
    User zeal

# Work servers
Include "conf.d/*.conf"
Host other
    HostName other.example.org
Match host other
    HostName ignored.example.org
`)
	writeTestFile(t, filepath.Join(home, ".ssh/conf.d/a.conf"), `# Production
Host work # Office
    HostName "work.example.org"
    ProxyJump bastion
    # ProxyCommand disabled-test
Host home
    HostName duplicate.example.org
`)
	hosts, err := readHosts(root)
	if err != nil {
		t.Fatal(err)
	}
	var aliases []string
	for _, h := range hosts {
		aliases = append(aliases, h.Alias)
	}
	if !reflect.DeepEqual(aliases, []string{"home", "home-alias", "work", "other"}) {
		t.Fatalf("aliases: %v", aliases)
	}
	if hosts[0].Hostname != "192.0.2.10" || hosts[0].Line != 2 || hosts[0].File != root {
		t.Fatalf("source tracking: %+v", hosts[0])
	}
	if hosts[0].Comment != "Personal servers · living room · Main machine" {
		t.Fatalf("comments: %q", hosts[0].Comment)
	}
	if hosts[2].Hostname != "work.example.org" || hosts[2].Line != 2 || !strings.Contains(hosts[2].Block, "ProxyJump bastion") {
		t.Fatalf("included block: %+v", hosts[2])
	}
	if hosts[3].Hostname != "other.example.org" || strings.Contains(hosts[3].Block, "Match") {
		t.Fatalf("Match boundary: %+v", hosts[3])
	}
}

func TestSSHParsingAndBoundaries(t *testing.T) {
	for _, tc := range []struct {
		line string
		want []string
	}{
		{`Host=one two`, []string{"Host", "one", "two"}},
		{`Host = one`, []string{"Host", "one"}},
		{`HostName="foo#bar" # label`, []string{"HostName", "foo#bar"}},
		{`Include "a b/*"`, []string{"Include", "a b/*"}},
		{`IdentityFile ~/.ssh/key\ name`, []string{"IdentityFile", "~/.ssh/key name"}},
	} {
		got, _, err := sshFields(tc.line)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: %v, %v", tc.line, got, err)
		}
	}
	for _, alias := range []string{"*", "foo?", "!foo", "-oProxyCommand=bad", "bad\x1b", "bad name"} {
		if validAlias(alias) {
			t.Errorf("unsafe/pattern alias allowed: %q", alias)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	writeTestFile(t, path, "Include \""+path+"\"\n")
	if _, err := readHosts(path); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
	writeTestFile(t, path, "Host \"unfinished\n")
	if _, err := readHosts(path); err == nil {
		t.Fatal("accepted invalid SSH quoting")
	}
	writeTestFile(t, path, "Match exec touch-never-run\n Include conditional/*\n")
	if _, err := readHosts(path); err == nil || !strings.Contains(err.Error(), "inside Match") {
		t.Fatalf("conditional include must not be silently misread: %v", err)
	}
}

func TestConfigAndQuoting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "config.json")
	if err := ensureConfig(path); err != nil {
		t.Fatal(err)
	}
	if c, err := loadConfig(path); err != nil || c.Destination != "tmux-pane" {
		t.Fatalf("defaults: %+v, %v", c, err)
	}
	writeTestFile(t, path, `{"destination":"tmux-window","editor":["nvim","-n"]}`)
	if err := ensureConfig(path); err != nil {
		t.Fatal(err)
	}
	if c, err := loadConfig(path); err != nil || c.Destination != "tmux-window" || len(c.Editor) != 2 {
		t.Fatalf("partial config / non-overwrite: %+v, %v", c, err)
	}
	for _, text := range []string{`{"destinaton":"terminal"}`, `{"editor":[]}`, `{"fallback":"tmux-pane"}`, `{"edit_destination":"tmux-session"}`, `{} {}`} {
		writeTestFile(t, path, text)
		if _, err := loadConfig(path); err == nil {
			t.Errorf("accepted invalid settings %s", text)
		}
	}
	argv := []string{"ssh", "--", `host'; touch /never; echo '`, "a b", "$(false)"}
	out, err := exec.Command("/bin/sh", "-c", "printf '%s\\n' "+shellCommand(argv)).Output()
	if err != nil || string(out) != strings.Join(argv, "\n")+"\n" {
		t.Fatalf("shell quoting: %q, %v", out, err)
	}
	if clean("a\x1b[31m\tb\n") != "a [31m b " {
		t.Fatal("preview control characters not sanitized")
	}
}

func TestTmuxOriginAndDestinations(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("CALLS", log)
	t.Setenv("TMUX", "/tmp/fake,1,0")
	t.Setenv("TMUX_PANE", "%7")
	t.Setenv("CLIENTS", "%7\t@8\t$9\t/dev/pts/2\n%1\t@2\t$3\t/dev/pts/4\n")
	fakeCommand(t, dir, "tmux", `if [ "$1" = list-clients ]; then printf '%s' "$CLIENTS"; exit; fi
printf 'CALL\n' >> "$CALLS"
printf '%s\n' "$@" >> "$CALLS"
if [ "$1" = new-session ]; then printf '$42\n'; fi`)
	o, err := captureOrigin("")
	if err != nil || o != (Origin{"%7", "@8", "$9", "/dev/pts/2"}) {
		t.Fatalf("origin: %+v, %v", o, err)
	}
	if decoded, err := decodeOrigin(encodeOrigin(o)); err != nil || decoded != o {
		t.Fatal("origin roundtrip failed")
	}
	t.Setenv("CLIENTS", "%7\t@8\t$9\t/dev/pts/2\n%7\t@8\t$9\t/dev/pts/4\n")
	if _, err := captureOrigin(""); err == nil {
		t.Fatal("ambiguous clients must not silently choose a destination")
	}
	if _, err := captureOrigin("/dev/pts/2"); err != nil {
		t.Fatal(err)
	}
	c := defaultConfig()
	hosts := []Host{{Alias: "web"}, {Alias: "db"}}
	for _, dest := range []string{"tmux-pane", "tmux-window", "tmux-session"} {
		if err := connect(hosts, dest, o, c, ""); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(log)
	text := string(b)
	for _, fragment := range []string{"split-window\n-h\n-t\n$9:@8.%7\n'ssh' '--' 'web'", "new-window\n-t\n$9:", "new-window\n-d\n-t\n$42:", "switch-client\n-c\n/dev/pts/2\n-t\n$42"} {
		if !strings.Contains(text, fragment) {
			t.Errorf("missing %q in calls:\n%s", fragment, text)
		}
	}
	if err := connect(hosts, "current", o, c, ""); err == nil {
		t.Fatal("multiple hosts cannot replace one current terminal")
	}
	if err := connect(hosts, "tmux-pane", Origin{}, c, ""); err == nil {
		t.Fatal("explicit tmux action must fail outside tmux")
	}
	fakeCommand(t, dir, "tmux", "exit 1")
	if err := connect(hosts, "tmux-pane", o, c, ""); err == nil || !strings.Contains(err.Error(), "opened 0/2") {
		t.Fatalf("partial failure context missing: %v", err)
	}
}

func TestPickerProtocol(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("FZF_DEFAULT_OPTS", "--filter=broken")
	fakeCommand(t, dir, "fzf", `if [ -n "$FZF_DEFAULT_OPTS" ]; then exit 99; fi
case "$*" in
  *ctrl-u:half-page-up,ctrl-d:half-page-down*) ;;
  *) exit 98 ;;
esac
IFS= read -r row
printf 'query\nalt-w\n%s\n' "$row"`)
	hosts := []Host{{Alias: "web", Hostname: "192.0.2.1", Comment: "Production"}}
	key, query, selected, err := pick(hosts, "/fake/ssh-pick", "", "tmux-pane", Origin{})
	if err != nil || key != "alt-w" || query != "query" || len(selected) != 1 || selected[0].Alias != "web" {
		t.Fatalf("picker: %q %q %+v %v", key, query, selected, err)
	}
	fakeCommand(t, dir, "fzf", "printf 'notfound\\nctrl-r\\n'; exit 1")
	key, _, _, err = pick(hosts, "/fake/ssh-pick", "", "terminal", Origin{})
	if err != nil || key != "ctrl-r" {
		t.Fatalf("reload with no matches: %q, %v", key, err)
	}
	fakeCommand(t, dir, "fzf", "exit 130")
	key, _, _, err = pick(hosts, "/fake/ssh-pick", "", "terminal", Origin{})
	if err != nil || key != "cancel" {
		t.Fatalf("cancel: %q, %v", key, err)
	}
}

func TestAppCustomConfigAndEditor(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "settings"))
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	log := filepath.Join(dir, "calls")
	t.Setenv("CALLS", log)
	root := filepath.Join(dir, ".ssh", "config")
	writeTestFile(t, root, "Host web\n HostName 192.0.2.1\n")
	fakeCommand(t, dir, "fzf", "IFS= read -r row; printf '\\nenter\\n%s\\n' \"$row\"")
	fakeCommand(t, dir, "ssh", "printf '%s\\n' \"$@\" > \"$CALLS\"")
	if err := app([]string{"--destination", "current"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	if string(b) != "--\nweb\n" {
		t.Fatalf("default config must not override system SSH settings: %q", b)
	}
	if err := app([]string{"--ssh-config", root, "--destination", "current"}); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(log)
	if string(b) != "-F\n"+root+"\n--\nweb\n" {
		t.Fatalf("custom SSH config not passed through: %q", b)
	}
	c := defaultConfig()
	c.Editor, c.EditDestination = []string{"test-editor", "-n"}, "current"
	fakeCommand(t, dir, "test-editor", "printf '%s\\n' \"$@\" > \"$CALLS\"")
	if err := editFile(root, 12, "current", false, Origin{}, c); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(log)
	if string(b) != "-n\n+12\n--\n"+root+"\n" {
		t.Fatalf("editor position: %q", b)
	}
	settings, _ := configPath()
	data, _ := json.Marshal(c)
	writeTestFile(t, settings, string(data))
	if err := app([]string{"config"}); err != nil {
		t.Fatal(err)
	}
}
