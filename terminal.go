package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const editorAppID = "org.go-ssh.editor"

func terminalCommand(c Config, argv []string, floating bool) ([]string, error) {
	prefix := append([]string{}, c.Terminal...)
	if floating && c.FloatingTerminal[0] != "auto" {
		// Explicit overrides include their own app-ID flags and command separator.
		return append(append([]string{}, c.FloatingTerminal...), argv...), nil
	}
	if prefix[0] == "auto" {
		args := []string{"--print-cmd=\\0"}
		if floating {
			args = append(args, "--app-id="+editorAppID)
		}
		args = append(args, "--")
		args = append(args, argv...)
		cmd := exec.Command("xdg-terminal-exec", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err == nil {
			// Ask the resolver for NUL-separated argv: never evaluate shell text.
			if !bytes.ContainsRune(out, 0) {
				return nil, fmt.Errorf("xdg-terminal-exec returned invalid command data; update it or configure terminal explicitly")
			}
			resolved := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
			if resolved[0] == "" {
				return nil, fmt.Errorf("xdg-terminal-exec returned an empty command")
			}
			return resolved, nil
		}
		if len(c.FallbackTerminal) == 0 {
			return nil, fmt.Errorf("system terminal unavailable: %w (%s); configure terminal or fallback_terminal", err, clean(strings.TrimSpace(stderr.String())))
		}
		prefix = append([]string{}, c.FallbackTerminal...)
	}
	if floating {
		// Native resolver handles other terminals via their desktop entries.
		var flags []string
		switch filepath.Base(prefix[0]) {
		case "kitty", "alacritty":
			flags = []string{"--class", editorAppID}
		case "foot", "footclient":
			flags = []string{"--app-id=" + editorAppID}
		default:
			return nil, fmt.Errorf("cannot set floating editor app ID for %q; configure floating_terminal with its app-ID flags and command separator", prefix[0])
		}
		prefix = append(append([]string{prefix[0]}, flags...), prefix[1:]...)
	}
	return append(prefix, argv...), nil
}

// cacheLog opens an append-only log under the user's cache dir for a detached job.
func cacheLog(name string) (*os.File, string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, "", err
	}
	dir := filepath.Join(cache, "ssh-pick")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, name)
	log, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return nil, "", err
	}
	return log, path, nil
}

// spawnBackground runs a job fully detached: no tty, output to a log file, so the
// picker returns immediately and the RDP window appears on its own.
func spawnBackground(argv []string) error {
	log, _, err := cacheLog("rdp.log")
	if err != nil {
		return err
	}
	defer log.Close()
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, log, log
	// ponytail: detached job; its output lives in rdp.log, no supervisor process.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch background RDP session: %w", err)
	}
	return cmd.Process.Release()
}

func spawnTerminal(c Config, argv []string, floating bool) error {
	args, err := terminalCommand(c, argv, floating)
	if err != nil {
		return err
	}
	log, path, err := cacheLog("terminal.log")
	if err != nil {
		return err
	}
	defer log.Close()
	if _, err := fmt.Fprintf(log, "\n[%s] launch %s\n", time.Now().Format(time.RFC3339), clean(args[0])); err != nil {
		return err
	}
	cmd := exec.Command(args[0], args[1:]...)
	// No popup pipes: child diagnostics remain available after the picker exits.
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch terminal: %w; configure terminal/fallback_terminal", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("terminal exited during startup: %w; diagnostics: %s", err, path)
		}
		return nil
	case <-time.After(300 * time.Millisecond):
		// ponytail: catch early failures; later errors stay in terminal.log.
		// Full lifecycle supervision would require a persistent parent process.
		return nil
	}
}
