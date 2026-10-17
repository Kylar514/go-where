package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type DomainRule struct {
	Jump   string `json:"jump"`
	Domain string `json:"domain"`
}

type RDPConfig struct {
	User            string       `json:"user"`
	DefaultDomain   string       `json:"default_domain"`
	DomainRules     []DomainRule `json:"domain_rules"`
	Tags            []string     `json:"tags"`
	Port            int          `json:"port"`
	Tunnel          string       `json:"tunnel"`
	Client          []string     `json:"client"`
	ClientArgs      []string     `json:"client_args"`
	PasswordCommand []string     `json:"password_command"`
}

func defaultRDPConfig() RDPConfig {
	return RDPConfig{
		DomainRules: []DomainRule{}, Tags: []string{"windows", "rdp"}, Port: 3389, Tunnel: "auto",
		Client: []string{}, ClientArgs: []string{"/dynamic-resolution"}, PasswordCommand: []string{},
	}
}

func safeValue(s string) bool { return clean(s) == s && !strings.ContainsAny(s, "\r\n") }

func (r RDPConfig) validate() error {
	if !safeValue(r.User) || !safeValue(r.DefaultDomain) {
		return fmt.Errorf("rdp user and default_domain must not contain control characters")
	}
	for _, rule := range r.DomainRules {
		if _, err := path.Match(rule.Jump, ""); err != nil || rule.Jump == "" {
			return fmt.Errorf("rdp domain_rules: invalid jump pattern %q", rule.Jump)
		}
		if rule.Domain == "" || !safeValue(rule.Domain) {
			return fmt.Errorf("rdp domain_rules: domain must be nonempty text for jump pattern %q", rule.Jump)
		}
	}
	if len(r.Tags) == 0 {
		return fmt.Errorf("rdp tags must name at least one tag")
	}
	for _, tag := range r.Tags {
		if tag == "" || strings.ContainsAny(tag, ", \t") || !safeValue(tag) {
			return fmt.Errorf("rdp tags must be single words without commas")
		}
	}
	if r.Port < 1 || r.Port > 65535 {
		return fmt.Errorf("rdp port must be between 1 and 65535")
	}
	switch r.Tunnel {
	case "auto", "socks", "forward":
	default:
		return fmt.Errorf("rdp tunnel must be auto, socks, or forward")
	}
	if len(r.Client) > 0 && r.Client[0] == "" {
		return fmt.Errorf("rdp client must be an argument array, or empty to auto-detect")
	}
	if len(r.PasswordCommand) > 0 && r.PasswordCommand[0] == "" {
		return fmt.Errorf("rdp password_command must be an argument array, or empty")
	}
	for _, arg := range append(append([]string{}, r.Client...), r.ClientArgs...) {
		if !safeValue(arg) {
			return fmt.Errorf("rdp client arguments must not contain control characters")
		}
	}
	for _, arg := range r.ClientArgs {
		lower := strings.ToLower(arg)
		if strings.HasPrefix(lower, "/p:") || strings.HasPrefix(lower, "/password:") || strings.HasPrefix(lower, "/args-from") {
			return fmt.Errorf("rdp client_args must not carry passwords; use password_command or the client's prompt")
		}
	}
	return nil
}

// A host is an RDP target when a comment on its block is exactly a tag.
func (h Host) isRDP(tags []string) bool {
	for _, note := range h.Notes {
		for _, tag := range tags {
			if strings.EqualFold(strings.TrimSpace(note), tag) {
				return true
			}
		}
	}
	return false
}

type hop struct{ User, Host, Port string }

func parseHop(s string) (hop, error) {
	var h hop
	s = strings.TrimPrefix(strings.TrimSpace(s), "ssh://")
	if s == "" || !safeValue(s) || strings.ContainsAny(s, " \t/") || strings.HasPrefix(s, "-") {
		return h, fmt.Errorf("unsupported ProxyJump hop")
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		h.User, s = s[:i], s[i+1:]
	}
	if host, port, err := net.SplitHostPort(s); err == nil {
		if n, convErr := strconv.Atoi(port); convErr != nil || n < 1 || n > 65535 {
			return h, fmt.Errorf("unsupported ProxyJump port")
		}
		h.Host, h.Port = host, port
	} else if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		h.Host = s[1 : len(s)-1]
	} else if !strings.Contains(s, ":") {
		h.Host = s
	}
	if h.Host == "" {
		return h, fmt.Errorf("unsupported ProxyJump hop (bracket IPv6 addresses)")
	}
	return h, nil
}

func (h hop) destination() string {
	host := h.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	dest := "ssh://"
	if h.User != "" {
		dest += h.User + "@"
	}
	dest += host
	if h.Port != "" {
		dest += ":" + h.Port
	}
	return dest
}

// An empty chain means a direct connection. Only ProxyJump chains are routable.
func parseJumps(raw string) ([]hop, []string, error) {
	if raw == "" || strings.EqualFold(raw, "none") {
		return nil, nil, nil
	}
	if strings.EqualFold(raw, "ProxyCommand") {
		return nil, nil, fmt.Errorf("uses ProxyCommand; RDP mode supports ProxyJump only")
	}
	var hops []hop
	var parts []string
	for _, part := range strings.Split(raw, ",") {
		h, err := parseHop(part)
		if err != nil {
			return nil, nil, err
		}
		hops, parts = append(hops, h), append(parts, strings.TrimSpace(part))
	}
	return hops, parts, nil
}

// Rules are ordered; a rule matches when its pattern matches any hop's host.
func (r RDPConfig) domainFor(hops []hop) string {
	for _, rule := range r.DomainRules {
		for _, h := range hops {
			if ok, _ := path.Match(strings.ToLower(rule.Jump), strings.ToLower(h.Host)); ok {
				return rule.Domain
			}
		}
	}
	return r.DefaultDomain
}

type rdpJob struct {
	Label           string
	Host            string
	Port            int
	Jump            string
	SSHConfig       string
	Tunnel          string
	Client          []string
	ClientArgs      []string
	User            string
	Domain          string
	PasswordCommand []string
}

func (j rdpJob) validate() error {
	if j.Host == "" || !safeValue(j.Host) || strings.ContainsAny(j.Host, " \t/%") || strings.HasPrefix(j.Host, "-") {
		return fmt.Errorf("host address is empty or unsupported (SSH tokens are not expanded)")
	}
	if j.Port < 1 || j.Port > 65535 {
		return fmt.Errorf("invalid RDP port")
	}
	if _, _, err := parseJumps(j.Jump); err != nil {
		return err
	}
	r := RDPConfig{User: j.User, DefaultDomain: j.Domain, Tags: []string{"rdp"}, Port: j.Port, Tunnel: j.Tunnel, Client: j.Client, ClientArgs: j.ClientArgs, PasswordCommand: j.PasswordCommand}
	return r.validate()
}

func newRDPJob(r RDPConfig, h Host, sshConfig string) (rdpJob, error) {
	jump := h.JumpHost
	if strings.EqualFold(jump, "none") {
		jump = ""
	}
	hops, _, err := parseJumps(jump)
	if err != nil {
		return rdpJob{}, fmt.Errorf("%s: %w", h.Alias, err)
	}
	job := rdpJob{Label: "rdp · " + h.Alias, Host: h.Hostname, Port: r.Port, Jump: jump, SSHConfig: sshConfig, Tunnel: r.Tunnel,
		Client: r.Client, ClientArgs: r.ClientArgs, User: r.User, Domain: r.domainFor(hops), PasswordCommand: r.PasswordCommand}
	if err := job.validate(); err != nil {
		return rdpJob{}, fmt.Errorf("%s: %w", h.Alias, err)
	}
	return job, nil
}

func (j rdpJob) tunnelArgv(mode string, port int) ([]string, error) {
	if mode == "direct" {
		return nil, nil
	}
	hops, parts, err := parseJumps(j.Jump)
	if err != nil || len(hops) == 0 {
		return nil, fmt.Errorf("no ProxyJump to tunnel through")
	}
	argv := []string{"ssh", "-N", "-o", "ExitOnForwardFailure=yes"}
	if j.SSHConfig != "" {
		argv = append(argv, "-F", j.SSHConfig)
	}
	if len(parts) > 1 {
		argv = append(argv, "-J", strings.Join(parts[:len(parts)-1], ","))
	}
	switch mode {
	case "socks":
		argv = append(argv, "-D", fmt.Sprintf("127.0.0.1:%d", port))
	case "forward":
		target := j.Host
		if strings.Contains(target, ":") {
			target = "[" + target + "]"
		}
		argv = append(argv, "-L", fmt.Sprintf("127.0.0.1:%d:%s:%d", port, target, j.Port))
	default:
		return nil, fmt.Errorf("unknown tunnel mode")
	}
	return append(argv, "--", hops[len(hops)-1].destination()), nil
}

func (j rdpJob) clientArgs(mode string, port int) []string {
	target := net.JoinHostPort(j.Host, strconv.Itoa(j.Port))
	if mode == "forward" {
		target = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	args := []string{"/v:" + target}
	if j.User != "" {
		args = append(args, "/u:"+j.User)
	}
	if j.Domain != "" {
		args = append(args, "/d:"+j.Domain)
	}
	if mode == "socks" {
		args = append(args, "/proxy:socks5://127.0.0.1:"+strconv.Itoa(port))
	}
	return append(args, j.ClientArgs...)
}

var freeRDPClients = []string{"xfreerdp3", "xfreerdp", "sdl-freerdp3", "sdl-freerdp", "wlfreerdp3", "wlfreerdp"}

func resolveClient(configured []string) ([]string, error) {
	if len(configured) > 0 {
		if _, err := exec.LookPath(configured[0]); err != nil {
			return nil, fmt.Errorf("configured rdp client %q not found", configured[0])
		}
		return configured, nil
	}
	for _, name := range freeRDPClients {
		if p, err := exec.LookPath(name); err == nil {
			return []string{p}, nil
		}
	}
	return nil, fmt.Errorf("no FreeRDP client found (looked for %s); install FreeRDP or set rdp.client", strings.Join(freeRDPClients, ", "))
}

func supportsSOCKS(client []string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, client[0], append(append([]string{}, client[1:]...), "/help")...).CombinedOutput()
	return strings.Contains(strings.ToLower(string(out)), "socks5")
}

func (j rdpJob) mode(client []string) (string, string) {
	hops, _, _ := parseJumps(j.Jump)
	switch {
	case len(hops) == 0:
		return "direct", ""
	case j.Tunnel == "socks", j.Tunnel == "auto" && supportsSOCKS(client):
		return "socks", ""
	}
	return "forward", "client lacks SOCKS5 proxy support: using a local port forward, so expect a certificate-name warning"
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

type sshTunnel struct {
	cmd  *exec.Cmd
	done chan struct{}
	err  error
}

func startTunnel(argv []string) (*sshTunnel, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	// ssh keeps the terminal so passphrase and password prompts still work.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start SSH tunnel: %w", err)
	}
	t := &sshTunnel{cmd: cmd, done: make(chan struct{})}
	go func() { t.err = cmd.Wait(); close(t.done) }()
	return t, nil
}

func (t *sshTunnel) stop() {
	select {
	case <-t.done:
		return
	default:
	}
	_ = t.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-t.done:
	case <-time.After(3 * time.Second):
		_ = t.cmd.Process.Kill()
		<-t.done
	}
}

// The wait is long because ssh may be prompting for a passphrase or password.
func (t *sshTunnel) waitReady(port int, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		select {
		case <-t.done:
			return fmt.Errorf("SSH tunnel exited before it was ready: %v", t.err)
		default:
		}
		if conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond); err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("SSH tunnel was not ready within %s", limit)
}

func passwordFrom(argv []string) (string, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("password_command failed: %w", err)
	}
	pw := strings.TrimSuffix(strings.TrimSuffix(string(out), "\n"), "\r")
	if pw == "" || len(pw) > 1024 || !safeValue(pw) {
		return "", fmt.Errorf("password_command must print one nonempty line of text")
	}
	return pw, nil
}

// FreeRDP exit codes meaning an established session ended normally.
// ponytail: codes from the FreeRDP client sources; confirm on the host and extend if a
// normal close still holds the pane open.
func sessionEnded(code int) bool {
	return code == 0 || code == 1 || code == 2 || code == 3 || code == 11
}

func runRDPSession(j rdpJob) error {
	if err := j.validate(); err != nil {
		return err
	}
	client, err := resolveClient(j.Client)
	if err != nil {
		return err
	}
	mode, warning := j.mode(client)
	if warning != "" {
		fmt.Println(warning)
	}
	port := 0
	if mode != "direct" {
		if port, err = freePort(); err != nil {
			return err
		}
		argv, err := j.tunnelArgv(mode, port)
		if err != nil {
			return err
		}
		fmt.Println("Opening SSH tunnel…")
		tunnel, err := startTunnel(argv)
		if err != nil {
			return err
		}
		defer tunnel.stop()
		if err := tunnel.waitReady(port, 2*time.Minute); err != nil {
			return err
		}
	}
	args := j.clientArgs(mode, port)
	cmd := exec.Command(client[0], client[1:]...)
	if len(j.PasswordCommand) > 0 {
		pw, err := passwordFrom(j.PasswordCommand)
		if err != nil {
			return err
		}
		// The password reaches the client on a pipe, never on a command line or in a file.
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		defer r.Close()
		_, err = w.WriteString(strings.Join(append(args, "/p:"+pw), "\n") + "\n")
		if closeErr := w.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		cmd.ExtraFiles = []*os.File{r}
		args = []string{"/args-from:fd:3"}
	}
	cmd.Args = append(cmd.Args, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	fmt.Println("Starting RDP session…")
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) && sessionEnded(exit.ExitCode()) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("RDP client: %w", err)
	}
	return nil
}

func runRDPJob(j rdpJob) error {
	fmt.Println(clean(j.Label))
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	defer signal.Stop(interrupts)
	err := runRDPSession(j)
	if err != nil {
		fmt.Printf("\nFAILED: %s\nPress Enter to close… ", clean(err.Error()))
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return err
}

func encodeRDPJob(j rdpJob) string {
	b, _ := json.Marshal(j)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeRDPJob(s string) (rdpJob, error) {
	var j rdpJob
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		err = json.Unmarshal(b, &j)
	}
	return j, err
}

func launchRDP(hosts []Host, dest string, o Origin, c Config, sshConfig, exe string) error {
	if len(hosts) == 0 {
		return nil
	}
	if err := checkPlacement(len(hosts), dest, o); err != nil {
		return err
	}
	var labels []string
	var argvs [][]string
	for _, h := range hosts {
		if !h.isRDP(c.RDP.Tags) {
			return fmt.Errorf("%s is not tagged for RDP (add a '# Windows' comment to its Host block)", h.Alias)
		}
		job, err := newRDPJob(c.RDP, h, sshConfig)
		if err != nil {
			return err // Validate every job before launching any of them.
		}
		labels = append(labels, job.Label)
		argvs = append(argvs, []string{exe, "__run-rdp", encodeRDPJob(job)})
	}
	return placeJobs(labels, argvs, dest, o, c)
}
