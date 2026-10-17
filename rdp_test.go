package main

import (
	"bytes"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestRDPTagsAndRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	config := filepath.Join(home, ".ssh", "config")
	writeTestFile(t, config, `# Windows
Host above
    HostName 192.0.2.1
    ProxyJump jump-a

Host inside
    HostName 192.0.2.2
    # RDP
    ProxyJump jump-b

Host inline # windows
    HostName 192.0.2.3

# Windows server for billing
Host notatag
    HostName 192.0.2.4

Host linux
    HostName 192.0.2.5
`)
	hosts, err := readHosts(config)
	if err != nil {
		t.Fatal(err)
	}
	var tagged []string
	for _, h := range hosts {
		if h.isRDP([]string{"windows", "rdp"}) {
			tagged = append(tagged, h.Alias)
		}
	}
	if !reflect.DeepEqual(tagged, []string{"above", "inside", "inline"}) {
		t.Fatalf("tagged hosts: %v", tagged)
	}
	rows := strings.Split(strings.TrimSuffix(rowsForMode(hosts, "rdp", []string{"windows", "rdp"}), "\n"), "\n")
	if len(rows) != 4 || !strings.HasPrefix(rows[0], "-1\tHOST") || !strings.HasPrefix(rows[1], "0\tabove") || !strings.HasPrefix(rows[2], "1\tinside") || !strings.HasPrefix(rows[3], "2\tinline") {
		t.Fatalf("filtered rows must keep full-list IDs: %q", rows)
	}
	if rowsForMode(hosts, "ssh", nil) != hostRows(hosts) {
		t.Fatal("non-RDP modes must list every host")
	}
	empty := rowsForMode(hosts, "rdp", []string{"nothing"})
	if !strings.Contains(empty, "-2\t(no hosts are tagged") || strings.Count(empty, "\n") != 2 {
		t.Fatalf("empty RDP list needs a placeholder: %q", empty)
	}
}

func TestRDPHopsAndDomainRules(t *testing.T) {
	for raw, want := range map[string]hop{
		"bastion":                   {Host: "bastion"},
		"user@bastion:2222":         {User: "user", Host: "bastion", Port: "2222"},
		"ssh://user@bastion":        {User: "user", Host: "bastion"},
		"[2001:db8::1]:2200":        {Host: "2001:db8::1", Port: "2200"},
		"[2001:db8::1]":             {Host: "2001:db8::1"},
		"first.example.org":         {Host: "first.example.org"},
		"user@with@at@example:2222": {User: "user@with@at", Host: "example", Port: "2222"},
	} {
		got, err := parseHop(raw)
		if err != nil || got != want {
			t.Errorf("%q: %+v, %v", raw, got, err)
		}
	}
	for _, bad := range []string{"", "-oProxyCommand=x", "a b", "host:99999", "2001:db8::1", "a/b", "bad\x1b"} {
		if _, err := parseHop(bad); err == nil {
			t.Errorf("accepted hop %q", bad)
		}
	}
	if hops, _, err := parseJumps("none"); err != nil || hops != nil {
		t.Fatal("none must mean a direct connection")
	}
	if _, _, err := parseJumps("ProxyCommand"); err == nil || !strings.Contains(err.Error(), "ProxyJump only") {
		t.Fatalf("ProxyCommand route must be rejected: %v", err)
	}
	if got := (hop{User: "u", Host: "2001:db8::1", Port: "22"}).destination(); got != "ssh://u@[2001:db8::1]:22" {
		t.Fatalf("destination: %s", got)
	}
	r := defaultRDPConfig()
	r.DefaultDomain = "default.example"
	r.DomainRules = []DomainRule{{"qa-*", "qa.example"}, {"*", "catchall.example"}}
	for jump, want := range map[string]string{"qa-jump": "qa.example", "QA-Jump": "qa.example", "other,qa-late": "qa.example", "other": "catchall.example", "": "default.example"} {
		hops, _, _ := parseJumps(jump)
		if got := r.domainFor(hops); got != want {
			t.Errorf("domain for %q: %q, want %q", jump, got, want)
		}
	}
}

func TestRDPSettingsValidation(t *testing.T) {
	if err := defaultRDPConfig().validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*RDPConfig){
		"control characters": func(r *RDPConfig) { r.User = "user\nname" },
		"port":               func(r *RDPConfig) { r.Port = 70000 },
		"tunnel":             func(r *RDPConfig) { r.Tunnel = "tor" },
		"rule pattern":       func(r *RDPConfig) { r.DomainRules = []DomainRule{{"[", "x.example"}} },
		"rule domain":        func(r *RDPConfig) { r.DomainRules = []DomainRule{{"qa-*", ""}} },
		"tag":                func(r *RDPConfig) { r.Tags = []string{"a,b"} },
		"no tags":            func(r *RDPConfig) { r.Tags = nil },
		"client":             func(r *RDPConfig) { r.Client = []string{""} },
		"password argument":  func(r *RDPConfig) { r.ClientArgs = []string{"/P:hunter2"} },
		"args-from":          func(r *RDPConfig) { r.ClientArgs = []string{"/args-from:file:x"} },
		"password command":   func(r *RDPConfig) { r.PasswordCommand = []string{""} },
	} {
		r := defaultRDPConfig()
		change(&r)
		if err := r.validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path, `{"rdp":{"user":"someone","default_domain":"example.test","domain_rules":[{"jump":"qa*","domain":"qa.example"}]}}`)
	c, err := loadConfig(path)
	if err != nil || c.RDP.User != "someone" || c.RDP.Port != 3389 || c.RDP.Tunnel != "auto" || len(c.RDP.Tags) != 2 {
		t.Fatalf("partial rdp settings must keep defaults: %+v, %v", c.RDP, err)
	}
	writeTestFile(t, path, `{"rdp":{"passwrd":"x"}}`)
	if _, err := loadConfig(path); err == nil {
		t.Fatal("unknown rdp setting accepted")
	}
}

func TestRDPCommands(t *testing.T) {
	r := defaultRDPConfig()
	r.User, r.DefaultDomain = "someone", "default.example"
	r.DomainRules = []DomainRule{{"qa*", "qa.example"}}
	job, err := newRDPJob(r, Host{Alias: "web", Hostname: "192.0.2.10", JumpHost: "user@qa-jump:2222,inner"}, "/custom config")
	if err != nil || job.Domain != "qa.example" {
		t.Fatalf("job: %+v, %v", job, err)
	}
	argv, err := job.tunnelArgv("socks", 41000)
	want := []string{"ssh", "-N", "-o", "ExitOnForwardFailure=yes", "-F", "/custom config", "-J", "user@qa-jump:2222", "-D", "127.0.0.1:41000", "--", "ssh://inner"}
	if err != nil || !reflect.DeepEqual(argv, want) {
		t.Fatalf("socks tunnel: %q, %v", argv, err)
	}
	argv, _ = job.tunnelArgv("forward", 41000)
	if !hasArg(argv, "127.0.0.1:41000:192.0.2.10:3389") || hasArg(argv, "-D") || hasArg(argv, "0.0.0.0") {
		t.Fatalf("forward tunnel must bind loopback only: %q", argv)
	}
	v6, _ := newRDPJob(r, Host{Alias: "v6", Hostname: "2001:db8::10", JumpHost: "bastion"}, "")
	if argv, _ = v6.tunnelArgv("forward", 41000); !hasArg(argv, "127.0.0.1:41000:[2001:db8::10]:3389") {
		t.Fatalf("IPv6 target must be bracketed: %q", argv)
	}
	if got := job.clientArgs("socks", 41000); !reflect.DeepEqual(got, []string{"/v:192.0.2.10:3389", "/u:someone", "/d:qa.example", "/proxy:socks5://127.0.0.1:41000", "/dynamic-resolution"}) {
		t.Fatalf("socks client args: %q", got)
	}
	if got := job.clientArgs("forward", 41000); got[0] != "/v:127.0.0.1:41000" || hasArg(got, "/proxy:socks5://127.0.0.1:41000") {
		t.Fatalf("forward client args: %q", got)
	}
	if got := v6.clientArgs("direct", 0); got[0] != "/v:[2001:db8::10]:3389" {
		t.Fatalf("direct IPv6 client args: %q", got)
	}
	direct, _ := newRDPJob(r, Host{Alias: "plain", Hostname: "192.0.2.11", JumpHost: "none"}, "")
	if argv, err = direct.tunnelArgv("direct", 0); argv != nil || err != nil {
		t.Fatal("a host without ProxyJump needs no tunnel")
	}
	for _, h := range []Host{
		{Alias: "pc", Hostname: "192.0.2.12", JumpHost: "ProxyCommand"},
		{Alias: "token", Hostname: "%h.example", JumpHost: ""},
		{Alias: "option", Hostname: "-oProxyCommand=x", JumpHost: ""},
		{Alias: "hop", Hostname: "192.0.2.13", JumpHost: "bad hop"},
	} {
		if _, err := newRDPJob(r, h, ""); err == nil {
			t.Errorf("accepted unroutable host %+v", h)
		}
	}
	if sessionEnded(131) || !sessionEnded(0) || !sessionEnded(11) {
		t.Fatal("session-ended classification")
	}
}

func TestFakeSSHHelper(t *testing.T) {
	if os.Getenv("SSH_PICK_FAKE_SSH") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	appendTestFile(os.Getenv("FAKE_SSH_LOG"), strings.Join(args, " ")+"\n")
	if os.Getenv("FAKE_SSH_FAIL") == "1" {
		os.Exit(255)
	}
	for i, a := range args {
		if (a == "-D" || a == "-L") && i+1 < len(args) {
			parts := strings.Split(args[i+1], ":")
			if l, err := net.Listen("tcp", "127.0.0.1:"+parts[1]); err == nil {
				defer l.Close()
			}
		}
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig
	appendTestFile(os.Getenv("FAKE_SSH_LOG"), "TERMINATED\n")
	os.Exit(0)
}

func appendTestFile(path, text string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err == nil {
		_, _ = f.WriteString(text)
		f.Close()
	}
}

// fakeRDPEnv installs fake ssh and FreeRDP-like clients and returns the log paths.
func fakeRDPEnv(t *testing.T, clientHelp string, clientExit int) (sshLog, clientLog, fdLog string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	sshLog, clientLog, fdLog = filepath.Join(dir, "ssh.log"), filepath.Join(dir, "client.log"), filepath.Join(dir, "fd3.log")
	t.Setenv("FAKE_SSH_LOG", sshLog)
	fakeCommand(t, dir, "ssh", "SSH_PICK_FAKE_SSH=1 exec '"+os.Args[0]+"' -test.run='^TestFakeSSHHelper$' -- \"$@\"")
	fakeCommand(t, dir, "xfreerdp3", `if [ "$1" = /help ]; then echo '`+clientHelp+`'; exit 0; fi
printf '%s\n' "$@" > '`+clientLog+`'
case "$1" in /args-from:fd:3) cat <&3 > '`+fdLog+`' ;; esac
exit `+strconv.Itoa(clientExit))
	return
}

func rdpTestJob(jump string) rdpJob {
	r := defaultRDPConfig()
	r.User, r.DefaultDomain = "someone", "example.test"
	job, err := newRDPJob(r, Host{Alias: "web", Hostname: "192.0.2.20", JumpHost: jump}, "")
	if err != nil {
		panic(err)
	}
	return job
}

func TestRDPSessionLifecycle(t *testing.T) {
	sshLog, clientLog, _ := fakeRDPEnv(t, "usage: /proxy:[<proto>://]<host>:<port> (socks5)", 11)
	if err := runRDPSession(rdpTestJob("bastion")); err != nil {
		t.Fatalf("a normal session end must not be an error: %v", err)
	}
	ssh, _ := os.ReadFile(sshLog)
	client, _ := os.ReadFile(clientLog)
	if !strings.Contains(string(ssh), "-D 127.0.0.1:") || !strings.Contains(string(ssh), "ssh://bastion") || !strings.HasSuffix(string(ssh), "TERMINATED\n") {
		t.Fatalf("tunnel must start, then be stopped on exit:\n%s", ssh)
	}
	if !strings.Contains(string(client), "/proxy:socks5://127.0.0.1:") || !strings.Contains(string(client), "/v:192.0.2.20:3389") || !strings.Contains(string(client), "/d:example.test") {
		t.Fatalf("client args:\n%s", client)
	}
	// A client without SOCKS5 support falls back to a loopback port forward.
	sshLog, clientLog, _ = fakeRDPEnv(t, "usage: /v:<server>[:port]", 0)
	if err := runRDPSession(rdpTestJob("bastion")); err != nil {
		t.Fatal(err)
	}
	ssh, _ = os.ReadFile(sshLog)
	client, _ = os.ReadFile(clientLog)
	if !strings.Contains(string(ssh), "-L 127.0.0.1:") || !strings.Contains(string(ssh), ":192.0.2.20:3389") || !strings.Contains(string(client), "/v:127.0.0.1:") || strings.Contains(string(client), "/proxy:") {
		t.Fatalf("forward fallback:\nssh: %s\nclient: %s", ssh, client)
	}
	// No ProxyJump: connect directly, with no tunnel process.
	sshLog, clientLog, _ = fakeRDPEnv(t, "socks5", 0)
	if err := runRDPSession(rdpTestJob("")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sshLog); !os.IsNotExist(err) {
		t.Fatal("direct connections must not start ssh")
	}
	client, _ = os.ReadFile(clientLog)
	if strings.Contains(string(client), "/proxy:") || !strings.Contains(string(client), "/v:192.0.2.20:3389") {
		t.Fatalf("direct client args:\n%s", client)
	}
}

func TestRDPSessionFailures(t *testing.T) {
	sshLog, clientLog, _ := fakeRDPEnv(t, "socks5", 131)
	if err := runRDPSession(rdpTestJob("bastion")); err == nil || !strings.Contains(err.Error(), "RDP client") {
		t.Fatalf("a failed connection must be reported: %v", err)
	}
	if ssh, _ := os.ReadFile(sshLog); !strings.HasSuffix(string(ssh), "TERMINATED\n") {
		t.Fatal("tunnel left running after a client failure")
	}
	t.Setenv("FAKE_SSH_FAIL", "1")
	_ = os.Remove(clientLog)
	if err := runRDPSession(rdpTestJob("bastion")); err == nil || !strings.Contains(err.Error(), "tunnel exited") {
		t.Fatalf("tunnel failure must be reported: %v", err)
	}
	if _, err := os.Stat(clientLog); !os.IsNotExist(err) {
		t.Fatal("client started although its tunnel failed")
	}
	t.Setenv("PATH", t.TempDir())
	if err := runRDPSession(rdpTestJob("")); err == nil || !strings.Contains(err.Error(), "no FreeRDP client found") {
		t.Fatalf("missing client: %v", err)
	}
}

func TestRDPPasswordNeverOnCommandLine(t *testing.T) {
	_, clientLog, fdLog := fakeRDPEnv(t, "socks5", 0)
	dir := filepath.Dir(clientLog)
	fakeCommand(t, dir, "fetch-password", "printf 'sec ret-pw\\n'")
	job := rdpTestJob("")
	job.PasswordCommand = []string{"fetch-password"}
	if err := runRDPSession(job); err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(clientLog)
	fd, _ := os.ReadFile(fdLog)
	if strings.Contains(string(argv), "ret-pw") || strings.TrimSpace(string(argv)) != "/args-from:fd:3" {
		t.Fatalf("password or options leaked into argv: %q", argv)
	}
	if !strings.Contains(string(fd), "/p:sec ret-pw\n") || !strings.Contains(string(fd), "/v:192.0.2.20:3389\n") {
		t.Fatalf("client did not receive its options on the pipe: %q", fd)
	}
	for _, body := range []string{"printf ''", "printf 'a\\nb\\n'", "exit 3"} {
		fakeCommand(t, dir, "fetch-password", body)
		if err := runRDPSession(job); err == nil {
			t.Errorf("password command %q accepted", body)
		}
	}
}

func TestLaunchRDPPlacementAndValidation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("CALLS", filepath.Join(dir, "calls"))
	fakeCommand(t, dir, "tmux", `printf 'CALL\n' >> "$CALLS"; printf '%s\n' "$@" >> "$CALLS"`)
	c := defaultConfig()
	c.RDP.User = "someone"
	c.RDP.Destination = "tmux-pane"
	o := Origin{"%1", "@1", "$1", "client"}
	hosts := []Host{
		{Alias: "a", Hostname: "192.0.2.31", JumpHost: "bastion", Notes: []string{"Windows"}},
		{Alias: "b", Hostname: "192.0.2.32", Notes: []string{"RDP"}},
	}
	if err := launchRDP(hosts, o, c, "", "/fake/ssh-pick", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(os.Getenv("CALLS"))
	if strings.Count(string(b), "split-window") != 2 || strings.Count(string(b), "__run-rdp") != 2 {
		t.Fatalf("one job per host expected:\n%s", b)
	}
	if err := launchRDP(append(hosts, Host{Alias: "linux", Hostname: "192.0.2.33"}), o, c, "", "/fake/ssh-pick", false); err == nil || !strings.Contains(err.Error(), "not tagged") {
		t.Fatalf("untagged host launched: %v", err)
	}
	// The picker destination is ignored; only rdp settings decide where a session lands.
	c.RDP.Destination = "current"
	if err := launchRDP(hosts, o, c, "", "/fake/ssh-pick", false); err == nil {
		t.Fatal("several RDP sessions cannot share the current terminal")
	}
	if err := launchRDP(hosts, Origin{}, c, "", "/fake/ssh-pick", false); err != nil {
		t.Fatalf("current is not a tmux target, so no tmux is required: %v", err)
	}
	c.RDP.Destination = "tmux-pane"
	if err := launchRDP(hosts, Origin{}, c, "", "/fake/ssh-pick", false); err == nil {
		t.Fatal("tmux placement must fail outside tmux")
	}
	if got := rdpDestination(c, false); got != "tmux-pane" {
		t.Fatalf("config destination: %q", got)
	}
	if got := rdpDestination(c, true); got != "terminal" {
		t.Fatalf("Ctrl+T must force a terminal: %q", got)
	}
	if got := rdpDestination(Config{}, false); got != "background" {
		t.Fatalf("default must be detached: %q", got)
	}
	job, _ := newRDPJob(c.RDP, hosts[0], "")
	decoded, err := decodeRDPJob(encodeRDPJob(job))
	if err != nil || !reflect.DeepEqual(decoded, job) {
		t.Fatalf("job changed in transit: %+v, %v", decoded, err)
	}
}

func TestModeReloadOnlyWhenListChanges(t *testing.T) {
	dir := t.TempDir()
	modeFile, listedFile, hostsFile := filepath.Join(dir, "mode"), filepath.Join(dir, "listed"), filepath.Join(dir, "hosts.json")
	writeTestFile(t, listedFile, "all")
	var out bytes.Buffer
	for _, step := range []struct {
		mode   string
		reload bool
	}{{"scp", false}, {"rdp", true}, {"rdp", false}, {"ssh", true}, {"rsync", false}} {
		writeTestFile(t, modeFile, step.mode)
		out.Reset()
		if err := modeReload(&out, modeFile, listedFile, hostsFile, "windows,rdp"); err != nil {
			t.Fatal(err)
		}
		if got := strings.HasPrefix(out.String(), "reload('"); got != step.reload || (got && !strings.Contains(out.String(), "__rows")) {
			t.Fatalf("mode %s: reload=%v output=%q", step.mode, got, out.String())
		}
	}
}

func TestRDMReport(t *testing.T) {
	export := []byte(`{"Connections":[
	  {"Name":"QAT Web 01","ConnectionType":1,"Url":"192.0.2.41","Group":"Lab\\QA\\Windows"},
	  {"Name":"QAT Web 02","ConnectionType":1,"Url":"192.0.2.42:3390","Group":"Lab\\QA\\Windows"},
	  {"Name":"Only In RDM","ConnectionType":1,"Url":"192.0.2.99","Group":"Lab\\Other"},
	  {"Name":"Linux Box","ConnectionType":77,"Terminal":{"Host":"192.0.2.43"}},
	  {"Children":[{"Name":"Nested","ConnectionType":1,"Url":"192.0.2.44"}]}
	]}`)
	hosts := []Host{
		{Alias: "web1", Hostname: "192.0.2.41", File: "/home/u/.ssh/config", Line: 7},
		{Alias: "web2", Hostname: "192.0.2.42", Notes: []string{"Windows"}},
		{Alias: "stale", Hostname: "192.0.2.50", Notes: []string{"RDP"}},
		{Alias: "linux", Hostname: "192.0.2.43"},
	}
	var out bytes.Buffer
	if err := rdmReport(export, hosts, []string{"windows", "rdp"}, &out); err != nil {
		t.Fatal(err)
	}
	report := out.String()
	for _, want := range []string{
		"RDM RDP entries: 4 · SSH hosts: 4 · tagged for RDP: 2",
		"Add a '# Windows' comment (address matches an RDM RDP entry): 1\n  web1  192.0.2.41  config:7  (RDM: QAT Web 01)",
		"Tagged, but no RDM RDP entry has that address: 1\n  stale  192.0.2.50",
		"RDM RDP entries with no SSH host: 2",
		"Only In RDM  192.0.2.99", "Nested  192.0.2.44",
		"Nothing was changed.",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "linux") || strings.Contains(report, "Linux Box") {
		t.Fatalf("non-RDP entries must not appear:\n%s", report)
	}
	if err := rdmReport([]byte(`{"Connections":[{"ConnectionType":77}]}`), hosts, []string{"windows"}, &out); err == nil {
		t.Fatal("an export without RDP entries must be rejected")
	}
	if err := rdmReport([]byte(`not json`), hosts, []string{"windows"}, &out); err == nil {
		t.Fatal("invalid JSON accepted")
	}
}
