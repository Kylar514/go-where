package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTerminalResolution(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	log := filepath.Join(dir, "resolver-args")
	t.Setenv("CALLS", log)
	fakeCommand(t, dir, "xdg-terminal-exec", `printf '%s\n' "$@" > "$CALLS"
shift
printf 'native-terminal\000'
if [ "$1" = --app-id=org.gowhere.editor ]; then
  printf '%s\000' --class org.gowhere.editor
  shift
fi
shift
printf '%s\000' "$@"`)
	c := defaultConfig()
	argv := []string{"nvim", "+5", "--", "file name'; $(false)"}
	for _, floating := range []bool{false, true} {
		got, err := terminalCommand(c, argv, floating)
		want := []string{"native-terminal"}
		if floating {
			want = append(want, "--class", editorAppID)
		}
		want = append(want, argv...)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("native resolution: %q, %v; want %q", got, err, want)
		}
	}
	fakeCommand(t, dir, "xdg-terminal-exec", "echo 'no terminal configured' >&2; exit 1")
	got, err := terminalCommand(c, argv, true)
	want := append([]string{"kitty", "--class", editorAppID}, argv...)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback: %q, %v", got, err)
	}
	c.FallbackTerminal = nil
	if _, err := terminalCommand(c, argv, false); err == nil || !strings.Contains(err.Error(), "no terminal configured") {
		t.Fatalf("missing resolver diagnostic: %v", err)
	}
	c.Terminal = []string{"alacritty", "-e"}
	got, err = terminalCommand(c, argv, true)
	want = append([]string{"alacritty", "--class", editorAppID, "-e"}, argv...)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("explicit terminal: %q, %v", got, err)
	}
	c.Terminal = []string{"unknown-terminal"}
	if _, err := terminalCommand(c, argv, true); err == nil || !strings.Contains(err.Error(), "floating_terminal") {
		t.Fatalf("unknown terminal must not silently claim to float: %v", err)
	}
	c.FloatingTerminal = []string{"custom-terminal", "--my-app-id=" + editorAppID, "--"}
	got, err = terminalCommand(c, argv, true)
	want = append(append([]string{}, c.FloatingTerminal...), argv...)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("floating override: %q, %v", got, err)
	}
	c = defaultConfig()
	fakeCommand(t, dir, "xdg-terminal-exec", "printf 'shell text is not argv'")
	if _, err := terminalCommand(c, argv, false); err == nil {
		t.Fatal("accepted ambiguous resolver output")
	}
}

func TestTerminalStartupDiagnostics(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_CACHE_HOME", dir)
	fakeCommand(t, dir, "test-terminal", "echo 'display unavailable' >&2; exit 7")
	c := defaultConfig()
	c.Terminal, c.FloatingTerminal = []string{"test-terminal"}, []string{"test-terminal"}
	for _, floating := range []bool{false, true} {
		err := launchCommand("terminal", []string{"ssh", "--", "web"}, Origin{}, c, floating)
		if err == nil || !strings.Contains(err.Error(), "terminal.log") {
			t.Fatalf("startup failure hidden: %v", err)
		}
	}
	path := filepath.Join(dir, "go-where", "terminal.log")
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "display unavailable") {
		t.Fatalf("missing retained diagnostics: %q, %v", b, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("terminal diagnostics must be private")
	}
	fakeCommand(t, dir, "test-terminal", "sleep 0.5; echo 'late diagnostic' >&2")
	if err := spawnTerminal(c, []string{"ssh", "--", "web"}, false); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), "late diagnostic") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child diagnostics lost after startup observation")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPickerActionDestinations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	log := filepath.Join(dir, "calls")
	t.Setenv("CALLS", log)
	fakeCommand(t, dir, "tmux", `printf '%s\n' "$@" >> "$CALLS"
if [ "$1" = new-session ]; then printf '$42\n'; fi`)
	fakeCommand(t, dir, "test-terminal", `printf 'terminal\n' >> "$CALLS"; printf '%s\n' "$@" >> "$CALLS"`)
	c := defaultConfig()
	c.Editor = []string{"test-editor"}
	c.Terminal = []string{"test-terminal"}
	c.FloatingTerminal = []string{"test-terminal", "--class", editorAppID}
	settings := filepath.Join(dir, "settings.json")
	data, _ := json.Marshal(c)
	writeTestFile(t, settings, string(data))
	sshConfig := filepath.Join(dir, "ssh-config")
	writeTestFile(t, sshConfig, "Host web\n HostName 192.0.2.1\n")
	o := Origin{"%7", "@8", "$9", "/dev/pts/2"}
	for _, tc := range []struct{ key, want string }{
		{"ctrl-s", "terminal\n--class\n" + editorAppID + "\ntest-editor\n--\n" + settings},
		{"ctrl-alt-s", "split-window\n-h\n-t\n$9:@8.%7\n" + shellCommand([]string{"test-editor", "--", settings})},
		{"ctrl-e", "terminal\n--class\n" + editorAppID + "\ntest-editor\n+1\n--\n" + sshConfig},
		{"ctrl-alt-e", "split-window\n-h\n-t\n$9:@8.%7\n" + shellCommand([]string{"test-editor", "+1", "--", sshConfig})},
		{"ctrl-alt-p", "split-window\n-h\n-t\n$9:@8.%7"},
		{"ctrl-alt-w", "new-window\n-t\n$9:"},
		{"ctrl-alt-n", "switch-client\n-c\n/dev/pts/2\n-t\n$42"},
		{"ctrl-alt-t", "terminal\nssh\n-F\n" + sshConfig + "\n--\nweb"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			scriptTUI(t, "\n"+tc.key+"\n0\tweb\n")
			t.Setenv("ACTION", tc.key)
			writeTestFile(t, log, "")
			if err := app([]string{"--no-popup", "--origin", encodeOrigin(o), "--config", settings, "--ssh-config", sshConfig}); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(log)
			if !strings.Contains(string(b), tc.want) {
				t.Fatalf("want %q in calls:\n%s", tc.want, b)
			}
		})
	}
}
