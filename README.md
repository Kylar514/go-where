# ssh-pick

A Linux-first SSH host picker: one Go executable, fzf for the UI, OpenSSH for connections. No plugin framework or host database. Native Windows integration is deferred.

## Build and run

Requires Go 1.23+ to build. Runtime requirements: fzf 0.38+ and OpenSSH; tmux 3.3+ for popups, `xdg-terminal-exec` for system-default terminal selection (or an explicit terminal command), and Neovim for editing. Go is not needed to run the compiled executable.

```sh
go test ./...
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/ssh-pick" .
ssh-pick
```

Inside an attached tmux pane, this opens a popup and captures the originating session/window/pane/client. Outside tmux, it runs in your terminal. Enter opens new panes in the original window, or new windows in your preferred terminal outside tmux. It never picks an arbitrary tmux session.

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

Type to search aliases, configured hostnames/IPs, jump hosts, and descriptions. Plain `j`/`k` remain search characters, not modal Vim navigation.

The preview occupies 10 rows at the top; longer blocks scroll. The host list shows up to 25 results at once, scrolling through the rest; smaller terminals show fewer rows. It grows upward from the bottom, with the first result nearest the search prompt and the shortcut legend underneath.

Columns are labeled **HOST**, **HOSTNAME / IP**, **JUMPHOST**, and **COMMENT**, with aligned labels pinned just above search. JUMPHOST shows the block's `ProxyJump` value, including multi-hop chains, or `—` when absent/disabled. A block using `ProxyCommand` shows `ProxyCommand` rather than guessing a jump alias from a shell command. Like HOSTNAME / IP, this is block-level metadata, not evaluation of wildcard/Match defaults; OpenSSH determines the actual connection route.

| Key | Action |
|---|---|
| Ctrl+J / Ctrl+K or arrows | Move down / up |
| Tab / Shift+Tab | Toggle selection and move down / up |
| Enter | Connect using your configured default |
| Ctrl+Alt+P | New pane per host in the originating window |
| Ctrl+Alt+W | New window per host in the originating session |
| Ctrl+Alt+N | New session, one window per host; switch the originating client |
| Ctrl+Alt+T | New terminal window per host |
| Ctrl+S | Settings in a floating editor terminal |
| Ctrl+E | Highlighted host's SSH config at its source line in a floating editor terminal |
| Ctrl+Alt+S | Settings in a new tmux pane |
| Ctrl+Alt+E | Highlighted host's SSH config in a new tmux pane |
| Ctrl+U / Ctrl+D | Move half a page up / down in the host list |
| Shift+↑ / Shift+↓ | Scroll the preview up / down |
| Ctrl+R | Reload SSH config and settings; preserve the search query |
| Esc / Ctrl+C | Cancel |

Connection actions use marked hosts, or the highlighted host if nothing is marked. Editing always uses the highlighted host. Editing closes the picker so it doesn't obscure the editor; reopen when finished. Reopening or reloading clears marks. Pane-edit shortcuts require tmux rather than falling back to a different destination.

Ctrl+Shift letter bindings were tested and rejected by fzf itself (`unsupported key: ctrl-shift-p`), so modified actions use Ctrl+Alt instead. N means new session, leaving S for settings. No Alt-only or function-key bindings are required. fzf's raw terminal mode receives Ctrl+S without enabling shell flow control; the physical terminal/compositor must still pass these keys through.

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
  "terminal": ["auto"],
  "fallback_terminal": ["kitty"],
  "floating_terminal": ["auto"]
}
```

Destinations: `tmux-pane`, `tmux-window`, `tmux-session`, `terminal`, `current`. The outside-tmux fallback is `terminal` or `current`. `edit_destination` controls the `ssh-pick config` CLI command; picker shortcuts explicitly choose floating or pane editing. Editing uses the Vim-compatible `+LINE -- FILE` convention.

`terminal: ["auto"]` asks `xdg-terminal-exec` to resolve your preferred terminal using its XDG terminal lists and desktop entries. The resolver returns NUL-separated arguments, not shell code. If the resolver is unavailable or cannot find a terminal, `fallback_terminal` is used; set it to `[]` to disable fallback. `$TERM` is never used to guess your preferred application. A terminal that resolves successfully but fails to launch is reported, not retried in another terminal.

To override terminal selection, use an argument array such as `["kitty"]`, `["alacritty", "-e"]`, or `["foot", "--"]`. Include the command separator your terminal needs. `floating_terminal: ["auto"]` follows that selection and requests app ID `org.go-ssh.editor`; with explicit Kitty/Alacritty/foot commands, the matching app-ID flag is inserted. Other explicit terminals need a `floating_terminal` override with their own flags, e.g. `["custom-terminal", "--app-id=org.go-ssh.editor", "--"]`. Existing explicit `terminal`/`floating_terminal` arrays are preserved; change them to `["auto"]` to opt into system defaults.

Terminal processes detach from the popup, but their output is retained in `$XDG_CACHE_HOME/ssh-pick/terminal.log` (normally `~/.cache/ssh-pick/terminal.log`, mode 0600). Failures during the first 300ms keep the error popup open and point to this log. Later failures remain in the log; the picker does not supervise terminal lifetimes. If a new terminal still fails to appear, inspect the log on the host for the actual display/session error.

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

The terminal's app ID/class identifies the editor; the window manager makes it float. System-default selection requests this through `xdg-terminal-exec --app-id`; the selected desktop entry must support that option. The companion rule is installed in this workspace's `thedotfiles/.config/hypr/rules.lua`. For a different Hyprland Lua setup, add:

```lua
hl.window_rule({
    name = "ssh-pick-editor",
    match = { class = "^org.go-ssh.editor$" },
    float = true,
    center = true,
    size = { "(monitor_w*3/10)", "(monitor_h*3/4)" },
    no_max_size = true,
})
```

Without an equivalent compositor rule, or if the selected terminal ignores the requested app ID, Ctrl+S/E still open Neovim in a separate terminal, but it may tile instead of float. Physical shortcuts and desktop behavior must be verified on the host, not just in a container.

## Private examples

Put personal/work SSH config samples under `local/`. The entire directory is ignored by Git. Never add private keys. Nothing from `local/` is required to build or test; tests use synthetic documentation-range addresses.
