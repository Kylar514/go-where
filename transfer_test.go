package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTransferCommands(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "local file: '$.txt")
	writeTestFile(t, source, "payload")
	remote := `C:\Users\Kyle\Downloads\a file.txt`
	tx := Transfer{Mode: "scp", Direction: "upload", Local: source, Remote: remote}
	got, err := transferCommand(tx, Host{Alias: "work"}, "/a config's path", false)
	want := []string{"scp", "-r", "-s", "-F", "/a config's path", "--", source, "work:" + remote}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("modern SCP must not shell-quote or normalize remote paths: %q, %v", got, err)
	}
	tx.Direction = "download"
	got, err = transferCommand(tx, Host{Alias: "2001:db8::1"}, "", false)
	want = []string{"scp", "-r", "-s", "--", "[2001:db8::1]:" + remote, source}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("download: %q, %v", got, err)
	}
	tx.Mode, tx.Remote = "legacy-scp", "/a directory/it's $(not-a-command); [file]"
	got, err = transferCommand(tx, Host{Alias: "work"}, "", false)
	if err != nil || got[2] != "-O" || got[len(got)-2] != "work:"+shellQuote(tx.Remote) {
		t.Fatalf("legacy must quote for the remote POSIX shell: %q, %v", got, err)
	}
	tx.Mode, tx.Direction, tx.Local, tx.Remote = "rsync", "upload", dir+"/", "/remote dir/"
	got, err = transferCommand(tx, Host{Alias: "work"}, "/a config's path", true)
	if err != nil || !hasArg(got, "--protect-args") || !hasArg(got, "--dry-run") || !hasArg(got, "--itemize-changes") || hasArg(got, "--delete") {
		t.Fatalf("rsync safety flags: %q, %v", got, err)
	}
	if got[len(got)-2] != dir+"/" || got[len(got)-1] != "work:/remote dir/" {
		t.Fatalf("trailing slash semantics lost: %q", got)
	}
	if got[5] != shellCommand([]string{"ssh", "-F", "/a config's path"}) {
		t.Fatalf("custom config not propagated to rsync SSH transport: %q", got)
	}
	for _, bad := range []Transfer{
		{Mode: "ssh", Direction: "upload", Local: source, Remote: "/tmp"},
		{Mode: "scp", Direction: "wrong", Local: source, Remote: "/tmp"},
		{Mode: "scp", Direction: "upload", Local: source, Remote: ""},
		{Mode: "scp", Direction: "upload", Local: source, Remote: "bad\npath"},
		{Mode: "scp", Direction: "upload", Local: source + "missing", Remote: "/tmp"},
	} {
		if _, err := transferCommand(bad, Host{Alias: "work"}, "", false); err == nil {
			t.Errorf("accepted invalid transfer: %+v", bad)
		}
	}
}

func hasArg(argv []string, arg string) bool {
	for _, s := range argv {
		if s == arg {
			return true
		}
	}
	return false
}

func TestLegacyShellPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, path := range []string{"/a file/it's [literal] $(false); &", "~/a file", "~", "-file", `C:\a file\test`} {
		out, err := exec.Command("/bin/sh", "-c", "printf '%s' "+legacyRemotePath(path)).Output()
		want := path
		if strings.HasPrefix(path, "~/") {
			want = home + path[1:]
		} else if path == "~" {
			want = home
		} else if strings.HasPrefix(path, "-") {
			want = "./" + path
		}
		if err != nil || string(out) != want {
			t.Errorf("remote POSIX argument %q: %q, %v", path, out, err)
		}
	}
}

func TestLocalCompletionAndPathSemantics(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestFile(t, filepath.Join(home, "a file"), "")
	writeTestFile(t, filepath.Join(home, ".hidden"), "")
	writeTestFile(t, filepath.Join(home, "a dir", "file"), "")
	got, err := completeLocal("~/a")
	if err != nil || !reflect.DeepEqual(got, []string{"~/a dir/", "~/a file"}) {
		t.Fatalf("completion: %q, %v", got, err)
	}
	if got, err := completeLocal("~"); err != nil || !reflect.DeepEqual(got, []string{"~/"}) {
		t.Fatalf("bare home completion: %q, %v", got, err)
	}
	if got, err := completeLocal("~/."); err != nil || !reflect.DeepEqual(got, []string{"~/.hidden"}) {
		t.Fatalf("hidden completion: %q, %v", got, err)
	}
	if got, err := completeLocal("~/new directory/new file"); err != nil || len(got) != 0 {
		t.Fatalf("new destinations must remain typeable: %q, %v", got, err)
	}
	for _, input := range []string{"~/a dir/", "~/a dir/."} {
		path, err := localTransferPath(input)
		if err != nil || path != home+input[1:] {
			t.Fatalf("content-copy semantics changed: %q, %v", path, err)
		}
	}
}

func TestTransferBatchContinuesAfterLaunchFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("CALLS", filepath.Join(dir, "calls"))
	fakeCommand(t, dir, "tmux", `printf '%s\n' "$@" >> "$CALLS"
if [ ! -f "$CALLS.first" ]; then touch "$CALLS.first"; exit 1; fi`)
	source := filepath.Join(dir, "source")
	writeTestFile(t, source, "payload")
	tx := Transfer{Mode: "scp", Direction: "upload", Local: source, Remote: "/tmp"}
	o := Origin{"%1", "@1", "$1", "test"}
	err := launchTransfers(tx, []Host{{Alias: "first"}, {Alias: "second"}}, "tmux-pane", o, defaultConfig(), "", "/fake/ssh-pick")
	b, _ := os.ReadFile(os.Getenv("CALLS"))
	if err == nil || !strings.Contains(err.Error(), "first") || strings.Count(string(b), "split-window") != 2 {
		t.Fatalf("independent jobs not attempted: %v\n%s", err, b)
	}
	if err := launchTransfers(Transfer{Direction: "download"}, []Host{{Alias: "a"}, {Alias: "b"}}, "terminal", o, defaultConfig(), "", ""); err == nil {
		t.Fatal("allowed colliding multi-host downloads")
	}
}

func TestModeStateAndFilteredRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mode")
	if err := writeMode(path, "rsync"); err != nil {
		t.Fatal(err)
	}
	if mode, err := readMode(path); err != nil || mode != "rsync" {
		t.Fatalf("mode state: %q, %v", mode, err)
	}
	if err := writeMode(path, "shell-command"); err == nil {
		t.Fatal("accepted invalid mode")
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	fakeCommand(t, dir, "fzf", "printf '1\\tdb\\n0\\tweb\\n'")
	hosts := []Host{{Alias: "web"}, {Alias: "db"}}
	state := PickerState{Mode: "rsync", Focus: "db", Marked: []string{"web"}}
	actions, err := restorePicker(hostRows(hosts), hosts, "query", &state)
	if err != nil || actions != "pos(2)+select+pos(1)" {
		t.Fatalf("filtered rank restore: %q, %v", actions, err)
	}
	// Source ordering can change on reload without selecting a different alias.
	hosts = []Host{{Alias: "db"}, {Alias: "web"}}
	actions, err = restorePicker(hostRows(hosts), hosts, "query", &state)
	if err != nil || actions != "pos(1)+select+pos(2)" {
		t.Fatalf("alias identity restore: %q, %v", actions, err)
	}
}

func TestPathPromptCompletesAndPreservesBackInput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("STEPS", dir)
	fakeCommand(t, dir, "fzf", `if [ ! -f "$STEPS/first" ]; then
  touch "$STEPS/first"
  printf 'tab\nquery\n%s\n' "$STEPS/a file"
else
  printf 'enter\n%s\n' "$STEPS/a file"
fi`)
	path, back, err := pathPrompt("Local source", "", true, "/fake/ssh-pick")
	if err != nil || back || path != filepath.Join(dir, "a file") {
		t.Fatalf("Tab/Enter: %q, %v, %v", path, back, err)
	}
	fakeCommand(t, dir, "fzf", `printf '%s\n' back 'C:\Users\Kyle\Downloads\new file'`)
	path, back, err = pathPrompt("Remote", "", false, "/fake/ssh-pick")
	if err != nil || !back || path != `C:\Users\Kyle\Downloads\new file` {
		t.Fatalf("Back/Windows syntax: %q, %v, %v", path, back, err)
	}
}

func TestRsyncPreviewVisitsEveryHost(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("CALLS", filepath.Join(dir, "calls"))
	fakeCommand(t, dir, "rsync", `printf 'CALL\n' >> "$CALLS"; printf '%s\n' "$@" >> "$CALLS"
case "$*" in *first:*) exit 1 ;; esac`)
	source := filepath.Join(dir, "source")
	writeTestFile(t, source, "payload")
	input, err := os.CreateTemp(dir, "input")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	_, _ = input.WriteString("\n")
	_, _ = input.Seek(0, 0)
	output, err := os.CreateTemp(dir, "output")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	stdin, stdout, stderr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = input, output, output
	defer func() { os.Stdin, os.Stdout, os.Stderr = stdin, stdout, stderr }()
	err = previewTransfers(Transfer{Mode: "rsync", Direction: "upload", Local: source, Remote: "/target"}, []Host{{Alias: "first"}, {Alias: "second"}}, "")
	b, _ := os.ReadFile(os.Getenv("CALLS"))
	if err == nil || !strings.Contains(err.Error(), "first") || strings.Count(string(b), "--dry-run") != 2 || strings.Count(string(b), "--itemize-changes") != 2 {
		t.Fatalf("preview did not visit every host safely: %v\n%s", err, b)
	}
}

func TestTransferRunnerHoldsFailureOutput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	fakeCommand(t, dir, "scp", "echo 'transfer failed' >&2; exit 9")
	input, err := os.CreateTemp(dir, "input")
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	_, _ = input.WriteString("\n")
	_, _ = input.Seek(0, 0)
	output, err := os.CreateTemp(dir, "output")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	stdin, stdout, stderr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin, os.Stdout, os.Stderr = input, output, output
	defer func() { os.Stdin, os.Stdout, os.Stderr = stdin, stdout, stderr }()
	job := transferJob{Label: "scp upload · test", Argv: []string{"scp", "--", "source", "host:/target"}}
	err = transferInternal([]string{"__run-transfer", encodeJob(job)})
	_, _ = output.Seek(0, 0)
	b, _ := io.ReadAll(output)
	if err == nil || !strings.Contains(string(b), "FAILED") || !strings.Contains(string(b), "Press Enter to close") {
		t.Fatalf("failure output not held: %v, %s", err, b)
	}
	decoded, _ := base64.RawURLEncoding.DecodeString(encodeJob(job))
	var roundtrip transferJob
	if err := json.Unmarshal(decoded, &roundtrip); err != nil || !reflect.DeepEqual(job, roundtrip) {
		t.Fatal("job arguments changed in transit")
	}
}

func TestTransferWizardBackAndMultiUpload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("STEPS", dir)
	fakeCommand(t, dir, "fzf", `n=0; if [ -f "$STEPS/count" ]; then read -r n < "$STEPS/count"; fi
n=$((n+1)); printf '%s\n' "$n" > "$STEPS/count"
printf '%s\n' "$@" >> "$STEPS/args"
cat "$STEPS/$n"`)
	fakeCommand(t, dir, "scp", "exit 99") // Must only be checked, never run in the picker.
	fakeCommand(t, dir, "tmux", `printf '%s\n' "$@" >> "$STEPS/launches"`)
	source := filepath.Join(dir, "source file")
	writeTestFile(t, source, "data")
	steps := []string{"upload\tUpload\n", "enter\n" + source + "\n", "back\n/old destination\n", "enter\n" + source + "\n", "enter\n/first destination\n", "back\tBack\n", "enter\n/final destination\n", "run\tRun\n"}
	for i, out := range steps {
		writeTestFile(t, filepath.Join(dir, strconv.Itoa(i+1)), out)
	}
	draft := Transfer{Mode: "scp", Direction: "upload"}
	launched, err := transferWizard(&draft, []Host{{Alias: "a"}, {Alias: "b"}}, "tmux-pane", Origin{"%1", "@1", "$1", "client"}, defaultConfig(), "", "/fake/ssh-pick")
	if err != nil || !launched || draft.Remote != "/final destination" || draft.Local != source {
		t.Fatalf("wizard state: %+v, %v, %v", draft, launched, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "launches"))
	if strings.Count(string(b), "split-window") != 2 {
		t.Fatalf("expected one independent job per host: %s", b)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "args"))
	if !strings.Contains(string(b), "--query=/old destination") || !strings.Contains(string(b), "--query="+source) {
		t.Fatalf("Back discarded entered values: %s", b)
	}
}

// Opt in only with a disposable loopback SSH server sharing this filesystem.
func TestTransferEndpointIntegration(t *testing.T) {
	config := os.Getenv("SSH_PICK_TEST_CONFIG")
	if config == "" {
		t.Skip("set SSH_PICK_TEST_CONFIG to a disposable loopback SSH config with alias test-target")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source file's $(literal).txt")
	writeTestFile(t, source, "transfer payload\n")
	host := Host{Alias: "test-target"}
	execute := func(tx Transfer, dry bool) {
		t.Helper()
		argv, err := transferCommand(tx, host, config, dry)
		if err != nil {
			t.Fatal(err)
		}
		if ssh := os.Getenv("SSH_PICK_TEST_SSH"); argv[0] == "scp" && ssh != "" {
			// Extracted test clients may have a compiled-in /usr/bin/ssh path.
			argv = append([]string{"scp", "-S", ssh}, argv[1:]...)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", shellCommand(argv), err, out)
		}
	}
	check := func(path, want string) {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil || string(b) != want {
			t.Fatalf("%s: %q, %v", path, b, err)
		}
	}
	for _, mode := range []string{"scp", "legacy-scp"} {
		remote := filepath.Join(dir, mode+" remote's $literal.txt")
		execute(Transfer{Mode: mode, Direction: "upload", Local: source, Remote: remote}, false)
		check(remote, "transfer payload\n")
		download := filepath.Join(dir, mode+" downloaded.txt")
		execute(Transfer{Mode: mode, Direction: "download", Local: download, Remote: remote}, false)
		check(download, "transfer payload\n")
	}
	sourceDir := filepath.Join(dir, "source directory")
	writeTestFile(t, filepath.Join(sourceDir, "a file's $[literal]"), "directory payload\n")
	writeTestFile(t, filepath.Join(sourceDir, ".hidden"), "hidden\n")
	for _, mode := range []string{"scp", "legacy-scp", "rsync"} {
		remote := filepath.Join(dir, mode+" remote directory")
		if err := os.Mkdir(remote, 0700); err != nil {
			t.Fatal(err)
		}
		tx := Transfer{Mode: mode, Direction: "upload", Local: sourceDir + "/", Remote: remote + "/"}
		if mode == "rsync" {
			execute(tx, true)
			entries, _ := os.ReadDir(remote)
			if len(entries) != 0 {
				t.Fatal("rsync dry run wrote files")
			}
		}
		execute(tx, false)
		copiedDir := remote
		if mode != "rsync" {
			copiedDir = filepath.Join(remote, filepath.Base(sourceDir))
		}
		check(filepath.Join(copiedDir, "a file's $[literal]"), "directory payload\n")
		check(filepath.Join(copiedDir, ".hidden"), "hidden\n")
		if mode == "rsync" {
			download := filepath.Join(dir, "rsync download")
			execute(Transfer{Mode: mode, Direction: "download", Local: download + "/", Remote: copiedDir + "/"}, false)
			check(filepath.Join(download, "a file's $[literal]"), "directory payload\n")
		}
	}
}
