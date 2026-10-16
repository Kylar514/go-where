package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

type Transfer struct {
	Mode      string
	Direction string
	Local     string
	Remote    string
}

type transferJob struct {
	Label string
	Argv  []string
}

func localTransferPath(value string) (string, error) {
	if err := validateTransferPath(value); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := value
	if value == "~" {
		path = home
	} else if strings.HasPrefix(value, "~/") {
		path = strings.TrimSuffix(home, "/") + value[1:]
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = strings.TrimSuffix(cwd, "/") + "/" + path
	}
	// Do not clean: trailing /, /. and symlink/.. affect native copy semantics.
	return path, nil
}

func validateTransferPath(value string) error {
	if value == "" || clean(value) != value {
		return fmt.Errorf("path must be nonempty and contain no control characters")
	}
	return nil
}

func validateLocalSource(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return fmt.Errorf("source must be a regular file or directory")
	}
	return nil
}

func legacyRemotePath(path string) string {
	if path == "~" {
		return "~"
	}
	if strings.HasPrefix(path, "~/") {
		return "~/" + shellQuote(path[2:])
	}
	if strings.HasPrefix(path, "-") {
		path = "./" + path
	}
	return shellQuote(path)
}

func transferCommand(t Transfer, h Host, sshConfig string, dry bool) ([]string, error) {
	if !validAlias(h.Alias) {
		return nil, fmt.Errorf("invalid SSH alias")
	}
	if t.Mode != "scp" && t.Mode != "legacy-scp" && t.Mode != "rsync" {
		return nil, fmt.Errorf("invalid transfer mode")
	}
	if t.Direction != "upload" && t.Direction != "download" {
		return nil, fmt.Errorf("invalid transfer direction")
	}
	if dry && t.Mode != "rsync" {
		return nil, fmt.Errorf("dry run is only supported by rsync")
	}
	local, err := localTransferPath(t.Local)
	if err != nil {
		return nil, err
	}
	if err := validateTransferPath(t.Remote); err != nil {
		return nil, err
	}
	if t.Direction == "upload" {
		if err := validateLocalSource(local); err != nil {
			return nil, fmt.Errorf("local source: %w", err)
		}
	}
	alias, remote := h.Alias, t.Remote
	if strings.Contains(alias, ":") {
		alias = "[" + alias + "]"
	}
	var argv []string
	if t.Mode == "rsync" {
		ssh := []string{"ssh"}
		if sshConfig != "" {
			ssh = append(ssh, "-F", sshConfig)
		}
		argv = []string{"rsync", "-rt", "--protect-args", "--info=progress2", "-e", shellCommand(ssh)}
		if dry {
			argv = append(argv, "--dry-run", "--itemize-changes")
		}
	} else {
		argv = []string{"scp", "-r"}
		if t.Mode == "legacy-scp" {
			argv = append(argv, "-O")
			remote = legacyRemotePath(remote)
		} else {
			argv = append(argv, "-s") // Force SFTP; never inherit a legacy default.
		}
		if sshConfig != "" {
			argv = append(argv, "-F", sshConfig)
		}
	}
	from, to := local, alias+":"+remote
	if t.Direction == "download" {
		from, to = to, from
	}
	return append(argv, "--", from, to), nil
}

func transferReview(t Transfer, hosts []Host, target, sshConfig string) (string, error) {
	var text strings.Builder
	fmt.Fprintf(&text, "%s · %s · %d host(s) · execution: %s\n\n%s\n", strings.ToUpper(t.Mode), strings.ToUpper(t.Direction), len(hosts), target, modeWarning(t.Mode))
	text.WriteString("Existing destination files may be overwritten.\nRemote paths are passed as entered; use a destination valid on every selected host.\n")
	if t.Mode == "rsync" {
		text.WriteString("Recursive copy with timestamps (-rt), not ownership/permissions/archive mode. Source dir/ copies contents; dir copies the directory. No deletion.\n")
	}
	for _, h := range hosts {
		argv, err := transferCommand(t, h, sshConfig, false)
		if err != nil {
			return "", err
		}
		local, _ := localTransferPath(t.Local)
		from, to := local, h.Alias+":"+t.Remote
		if t.Direction == "download" {
			from, to = to, from
		}
		fmt.Fprintf(&text, "\nHOST %s\nFROM %s\nTO   %s\n%s\n", h.Alias, from, to, shellCommand(argv))
	}
	return text.String(), nil
}

func transferWizard(t *Transfer, hosts []Host, target string, o Origin, c Config, sshConfig, exe string) (bool, error) {
	for step := 0; ; {
		back := false
		var err error
		switch step {
		case 0:
			var direction string
			direction, back, err = choose("Transfer direction", []choice{{"upload", "Upload — local source → remote destination(s)"}, {"download", "Download — one remote source → local destination"}}, modeWarning(t.Mode))
			if err == nil && !back {
				t.Direction = direction
				if direction == "download" && len(hosts) != 1 {
					err = notice("Downloads require exactly one host to avoid destination collisions.\nChoose Upload, or go Back and select one host.")
					if err != nil {
						return false, err
					}
					continue
				}
			}
		case 1:
			label := "Local source"
			if t.Direction == "download" {
				label = "Local destination (new names allowed)"
			}
			t.Local, back, err = pathPrompt(label, t.Local, true, exe)
			if err == nil && !back {
				_, err = localTransferPath(t.Local)
				if err == nil && t.Direction == "upload" {
					path, _ := localTransferPath(t.Local)
					err = validateLocalSource(path)
				}
			}
		case 2:
			label := "Remote destination (same path on all hosts)"
			if t.Direction == "download" {
				label = "Remote source"
			}
			t.Remote, back, err = pathPrompt(label, t.Remote, false, exe)
			if err == nil && !back {
				err = validateTransferPath(t.Remote)
			}
		case 3:
			var text, action string
			text, err = transferReview(*t, hosts, target, sshConfig)
			if err != nil {
				if showErr := notice(err.Error()); showErr != nil {
					return false, errors.Join(err, showErr)
				}
				step = 1
				continue
			}
			if err == nil {
				options := []choice{{"run", "Run transfers — confirm writes"}, {"back", "Back — edit paths"}}
				if t.Mode == "rsync" {
					options = append([]choice{{"preview", "Preview changes — dry run on every host"}}, options...)
				}
				action, back, err = choose("Review "+t.Mode, options, text)
				back = back || action == "back"
			}
			if err == nil && !back {
				tool := "scp"
				if t.Mode == "rsync" {
					tool = "rsync"
				}
				_, err = exec.LookPath(tool)
				if err == nil && action == "preview" {
					err = previewTransfers(*t, hosts, sshConfig)
					if err == nil {
						continue // Preview never authorizes execution.
					}
				} else if err == nil && action == "run" {
					return true, launchTransfers(*t, hosts, target, o, c, sshConfig, exe)
				}
			}
		}
		if err != nil {
			if showErr := notice(err.Error()); showErr != nil {
				return false, errors.Join(err, showErr)
			}
			continue
		}
		if back {
			step--
		} else {
			step++
		}
		if step < 0 {
			return false, nil
		}
	}
}

func encodeJob(job transferJob) string {
	b, _ := json.Marshal(job)
	return base64.RawURLEncoding.EncodeToString(b)
}

func launchTransfers(t Transfer, hosts []Host, dest string, o Origin, c Config, sshConfig, exe string) error {
	if len(hosts) == 0 || (t.Direction == "download" && len(hosts) != 1) {
		return fmt.Errorf("select at least one host for upload or exactly one for download")
	}
	if strings.HasPrefix(dest, "tmux-") && o.Pane == "" {
		return fmt.Errorf("%s requires tmux", dest)
	}
	if dest == "current" && len(hosts) > 1 {
		return fmt.Errorf("multiple hosts need panes, windows or terminals, not current")
	}
	var jobs []transferJob
	for _, h := range hosts {
		argv, err := transferCommand(t, h, sshConfig, false)
		if err != nil {
			return err // Validate every job before launching any of them.
		}
		jobs = append(jobs, transferJob{Label: t.Mode + " " + t.Direction + " · " + h.Alias, Argv: argv})
	}
	var failures []error
	session := ""
	for _, job := range jobs {
		argv := []string{exe, "__run-transfer", encodeJob(job)}
		var err error
		if dest == "tmux-session" {
			if session == "" {
				name := "transfer-" + fmt.Sprintf("%x", time.Now().UnixNano())
				var id string
				id, err = output("tmux", "new-session", "-d", "-P", "-F", "#{session_id}", "-s", name, shellCommand(argv))
				if err == nil {
					session = id
				}
			} else {
				err = run([]string{"tmux", "new-window", "-d", "-t", session + ":", shellCommand(argv)})
			}
		} else if dest == "current" {
			err = runTransferJob(job)
		} else {
			err = launchCommand(dest, argv, o, c, false)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", job.Label, err))
		}
	}
	if session != "" {
		if err := run([]string{"tmux", "switch-client", "-c", o.Client, "-t", session}); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func runTransferJob(job transferJob) error {
	if len(job.Argv) == 0 || (job.Argv[0] != "scp" && job.Argv[0] != "rsync") {
		return fmt.Errorf("invalid transfer executable")
	}
	fmt.Printf("%s\n%s\n\n", clean(job.Label), clean(shellCommand(job.Argv)))
	// Let the client receive Ctrl+C without killing the output-holding parent.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	err := run(job.Argv)
	signal.Stop(interrupts)
	if err != nil {
		fmt.Printf("\nFAILED: %s\n", clean(err.Error()))
	} else {
		fmt.Println("\nCompleted successfully.")
	}
	fmt.Print("Press Enter to close… ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return err
}

func previewTransfers(t Transfer, hosts []Host, sshConfig string) error {
	var failures []error
	for _, h := range hosts {
		argv, err := transferCommand(t, h, sshConfig, true)
		if err != nil {
			return err
		}
		fmt.Printf("\nDRY RUN · %s\n%s\n\n", clean(h.Alias), clean(shellCommand(argv)))
		if err := run(argv); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", h.Alias, err))
		}
	}
	fmt.Print("\nPreview finished. No transfer is authorized yet. Press Enter to return to review… ")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return errors.Join(failures...)
}

func transferInternal(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("invalid internal transfer arguments")
	}
	switch args[0] {
	case "__mode", "__mode-prompt":
		mode, err := readMode(args[1])
		if err != nil {
			return err
		}
		if args[0] == "__mode-prompt" {
			fmt.Print(strings.ToUpper(mode) + " > ")
			return nil
		}
		chosen, back, err := chooseMode(mode)
		if err != nil || back {
			return err
		}
		return writeMode(args[1], chosen)
	case "__complete":
		matches, err := completeLocal(args[1])
		if err != nil {
			return err
		}
		for _, match := range matches {
			fmt.Println(match)
		}
		return nil
	case "__text":
		b, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(b), "\n") {
			fmt.Println(clean(line))
		}
		return nil
	case "__run-transfer":
		b, err := base64.RawURLEncoding.DecodeString(args[1])
		if err != nil {
			return err
		}
		var job transferJob
		if err := json.Unmarshal(b, &job); err != nil {
			return err
		}
		return runTransferJob(job)
	}
	return fmt.Errorf("unknown internal command")
}
