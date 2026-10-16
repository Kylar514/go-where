# ssh-pick

A Linux-first SSH host picker: one Go executable, fzf for the UI, OpenSSH for connections. No plugin framework or host database. Native Windows integration is deferred.

## Build and run

Requires Go 1.23+ to build. Runtime requirements: fzf 0.38+ and OpenSSH; tmux 3.3+ for popups, Kitty for the default external terminal, and Neovim for editing. Go is not needed to run the compiled executable.

```sh
go test ./...
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/ssh-pick" .
ssh-pick
```

Inside an attached tmux pane, this opens a popup and captures the originating session/window/pane/client. Outside tmux, it runs in your terminal. Enter opens new panes in the original window, or new Kitty windows outside tmux. It never picks an arbitrary tmux session.

```sh
ssh-pick --destination tmux-window
ssh-pick --destination current          # SSH in the picker terminal; one host only
ssh-pick --no-popup                     # inline picker, still targets the original tmux pane
ssh-pick --ssh-config ./local/config    # also passes this file to ssh -F
ssh-pick config                        # create/open settings without overwriting them
ssh-pick --help
```

With the default SSH config, connections use `ssh -- alias`, preserving normal user and system SSH configuration. With `--ssh-config`, connections use `ssh -F path -- alias`; OpenSSH's usual `-F` semantics apply (including skipping the system config).

## Keys

Type to search aliases, configured hostnames/IPs, and descriptions. Plain `j`/`k` remain search characters, not modal Vim navigation.

| Key | Action |
|---|---|
| Ctrl+J / Ctrl+K or arrows | Move down / up |
| Tab / Shift+Tab | Toggle selection and move down / up |
| Enter | Connect using your configured default |
| Alt+P | New pane per host in the originating window |
| Alt+W | New window per host in the originating session |
| Alt+S | New session, one window per host; switch the originating client |
| Alt+T | New terminal window per host |
| Ctrl+E | Edit the highlighted host's SSH config at its source line |
| Alt+E | Edit the highlighted host in a floating terminal |
| Alt+C | Open picker settings |
| Ctrl+U / Ctrl+D | Page the preview up / down |
| Ctrl+R | Reload SSH config and settings; preserve the search query |
| Esc / Ctrl+C | Cancel |

Connection actions use marked hosts, or the highlighted host if nothing is marked. Editing always uses the highlighted host. Editing in another pane/window/terminal closes the picker so it doesn't obscure the editor; reopen when finished. With `edit_destination: "current"`, the picker resumes after the editor exits. Reopening or reloading clears marks.

The picker ignores `FZF_DEFAULT_OPTS`, `FZF_DEFAULT_OPTS_FILE`, and `FZF_DEFAULT_COMMAND` so personal fzf bindings/output options cannot corrupt its selection protocol.

Explicit tmux actions require tmux; only the default Enter action uses the configured fallback. Panes are split horizontally from the original pane. Large selections can run out of pane space: failures report how many hosts opened and never silently reroute the rest. A failed new-session operation may leave a partially populated session; no existing session is deleted or replaced.

## Settings

`ssh-pick config` creates and opens `$XDG_CONFIG_HOME/ssh-pick/config.json` (normally `~/.config/ssh-pick/config.json`). JSON keeps the executable dependency-free. Missing fields use these defaults:

```json
{
  "destination": "tmux-pane",
  "fallback": "terminal",
  "editor": ["nvim"],
  "edit_destination": "tmux-pane",
  "terminal": ["kitty"],
  "floating_terminal": ["kitty", "--class", "org.go-ssh.editor"]
}
```

Destinations: `tmux-pane`, `tmux-window`, `tmux-session`, `terminal`, `current`. Editing supports all except `tmux-session`. The outside-tmux fallback is `terminal` or `current`. Commands are argument arrays, not shell fragments; for terminals needing a command separator, include it in the array. Editing uses the Vim-compatible `+LINE -- FILE` convention.

Invalid or unknown settings fail clearly. `ssh-pick config` can still open malformed settings using default editor/placement so you can repair them.

## SSH config discovery

- Reads `~/.ssh/config` and nested `Include` globs in lexical order, including quoted paths and `~/`. Relative includes resolve under `~/.ssh`, as OpenSSH specifies for user config, even with `--ssh-config`.
- Lists explicit aliases, including multiple aliases on a `Host` line. Wildcards, negated patterns, and option-like aliases are excluded. Duplicate aliases use their first source location for preview/editing.
- Searches a block's directly configured `HostName`, falling back to its alias. This is **not** a DNS lookup or fully evaluated effective hostname: wildcard defaults, Match conditions, and token expansion remain OpenSSH's responsibility.
- Uses descriptive comments immediately before `Host`, inline comments, and standalone comments within a block. Common commented-out SSH directives and separator lines are excluded from search descriptions but remain visible in the preview. Comment classification is deliberately heuristic; it does not rewrite your files.
- Does not run `ssh -G`, `Match exec`, ProxyCommand, or network probes while browsing. Only connecting runs OpenSSH and the commands authorized by your SSH config.
- Includes inside `Match` are rejected explicitly rather than guessed. Include environment/token expansion, system-wide host discovery, and `known_hosts` discovery are not implemented. Missing Include glob matches are ignored, as in OpenSSH; cycles, unreadable matched files, and malformed quoting are errors.

Preview data is kept in a mode-0600 temporary file and removed on normal exit. An uncatchable kill can leave it behind in the system temporary directory. Source text is control-character sanitized before display; sources are never modified by the picker itself.

## tmux launch binding

No launch key is imposed. Once you choose one, a binding can launch the same executable, for example:

```tmux
# Example only: prefix, then Ctrl+F. ssh-pick opens its own popup.
bind-key C-f run-shell 'ssh-pick --client "#{client_name}"'
```

Passing the client explicitly matters when multiple clients share the same pane. Without it, the command requires exactly one attached client viewing the invoking pane and refuses ambiguity. Session-qualified targets also avoid sending new panes into the wrong session when windows are linked.

## Floating editor on Hyprland / Omarchy

Kitty's class identifies the editor; the window manager makes it float. The companion rule is installed in this workspace's `thedotfiles/.config/hypr/rules.lua`. For a different Hyprland Lua setup, add:

```lua
hl.window_rule({
    name = "ssh-pick-editor",
    match = { class = "^org.go-ssh.editor$" },
    float = true,
    center = true,
    size = { "(monitor_w*3/5)", "(monitor_h*4/5)" },
    no_max_size = true,
})
```

Without an equivalent compositor rule, Alt+E still opens Neovim in a separate terminal, but it may tile instead of float. Kitty/desktop behavior must be verified on the host, not just in a container.

## Private examples

Put personal/work SSH config samples under `local/`. The entire directory is ignored by Git. Never add private keys. Nothing from `local/` is required to build or test; tests use synthetic documentation-range addresses.
