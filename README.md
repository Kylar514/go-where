# go-where

A Linux-first SSH host picker: one Go executable, fzf for the UI, OpenSSH for connections. No plugin framework or host database. Native Windows integration is deferred.

## Build and run

Requires Go 1.23+ to build. Runtime requirements: fzf and OpenSSH; tmux 3.3+ for popups, `xdg-terminal-exec` for system-default terminal selection (or an explicit terminal command), and Neovim for editing. Transfer modes need OpenSSH SCP with `-s`/`-O` support or rsync 3.1+ as applicable. Go is not needed to run the compiled executable. The mode UI is verified with fzf 0.74.5; older builds must support `transform-prompt`, `reload`, `become`, and result-event bindings.

```sh
go test ./...
mkdir -p "$HOME/.local/bin"
go build -o "$HOME/.local/bin/go-where" .
go-where
```

Inside an attached tmux pane, this opens a popup and captures the originating session/window/pane/client. Outside tmux, it runs in your terminal. Enter opens new panes in the original window, or new windows in your preferred terminal outside tmux. It never picks an arbitrary tmux session.

```sh
go-where --destination tmux-window
go-where --destination current          # SSH in the picker terminal; one host only
go-where --no-popup                     # inline picker, still targets the original tmux pane
go-where --ssh-config ./local/config    # also passes this file to ssh -F
go-where config                        # create/open settings without overwriting them
go-where --help
go-where scp                           # Start in modern SCP mode
go-where scp --legacy                  # Explicit legacy SCP; never an automatic retry
go-where rsync
```

With the default SSH config, connections use `ssh -- alias`, preserving normal user and system SSH configuration. With `--ssh-config`, connections use `ssh -F path -- alias`; OpenSSH's usual `-F` semantics apply (including skipping the system config).

## Keys

Type to search aliases, configured hostnames/IPs, jump hosts, and descriptions. Plain `j`/`k` remain search characters, not modal Vim navigation.

The preview occupies 10 rows at the top; longer blocks scroll. The host list shows up to 25 results at once, scrolling through the rest; smaller terminals show fewer rows. It grows upward from the bottom, with the first result nearest the search prompt and the shortcut legend underneath.

Columns are labeled **HOST**, **HOSTNAME / IP**, **JUMPHOST**, and **COMMENT**, with aligned labels pinned just above search. JUMPHOST shows the block's `ProxyJump` value, including multi-hop chains, or `—` when absent/disabled. A block using `ProxyCommand` shows `ProxyCommand` rather than guessing a jump alias from a shell command. Like HOSTNAME / IP, this is block-level metadata, not evaluation of wildcard/Match defaults; OpenSSH determines the actual connection route.

| Key | Action |
|---|---|
| Ctrl+J / Ctrl+K or arrows | Move down / up |
| Tab / Shift+Tab | Toggle selection and move to the next / previous result |
| Enter | Connect using your configured default |
| Ctrl+O | Choose SSH, SCP, Legacy SCP, rsync, or RDP without losing the current search, highlight, or marks |
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

Connection actions use marked hosts, or the highlighted host if nothing is marked. Editing always uses the highlighted host. Editing closes the picker so it doesn't obscure the editor; reopen when finished. Mode switching keeps the existing picker alive, preserving its exact navigation state. After reopening/reloading, only matching selections can be restored. Pane-edit shortcuts require tmux rather than falling back to a different destination.

Ctrl+Shift letter bindings were tested and rejected by fzf itself (`unsupported key: ctrl-shift-p`), so modified actions use Ctrl+Alt instead. N means new session, leaving S for settings. No Alt-only or function-key bindings are required. fzf's raw terminal mode receives Ctrl+S without enabling shell flow control; the physical terminal/compositor must still pass these keys through.

The picker ignores `FZF_DEFAULT_OPTS`, `FZF_DEFAULT_OPTS_FILE`, and `FZF_DEFAULT_COMMAND` so personal fzf bindings/output options cannot corrupt its selection protocol.

Explicit tmux actions require tmux; only the default Enter action uses the configured fallback. Panes are split horizontally from the original pane. Large selections can run out of pane space: failures report how many hosts opened and never silently reroute the rest. A failed new-session operation may leave a partially populated session; no existing session is deleted or replaced.

## Transfers

Press **Ctrl+O** in the host picker to select SCP, Legacy SCP, or rsync. SSH remains the default on a normal launch. The prompt identifies the current mode and the host preview shows protocol requirements.

```text
Select host(s) → Upload / Download → Local path → Remote path → Review → Run
```

Uploads support multiple selected hosts with one local source and one shared remote destination. Downloads require exactly one host to prevent destination collisions. Transfer placement shortcuts (Ctrl+Alt+P/W/N/T) choose the execution destination before setup; they never bypass review. Multiple transfers need panes, windows, or terminals, not the single `current` destination.

### Paths and navigation

- Local path fields show filesystem candidates while typing. Tab fills the highlighted candidate; completing a directory opens its contents. Ctrl+J/K or arrows move through candidates, and Enter accepts the typed path. Hidden files are included.
- Local fields accept spaces, quotes, `~/`, relative paths, and new destination names. Source paths must exist. Trailing `/`, `/.`, and symlink-sensitive path components are preserved rather than cleaned away.
- Remote paths are typed without remote browsing. Their spelling is preserved, including Windows drive letters/backslashes; no local Linux normalization is applied.
- Esc goes Back and retains typed path values. Esc from the direction menu returns to host selection; Esc from the host picker exits.
- Use a shared remote destination that is valid for every selected host. Mixed Windows/POSIX targets may need separate batches.
- One source path per transfer. Control characters in paths are rejected; glob patterns and filename interpretation otherwise follow the selected native tool.

### Protocol behavior

| Mode | Client behavior and limits |
|---|---|
| SCP | Explicit `scp -s -r`: SFTP-based, recursive copy. Requires an SFTP-capable SSH server. Windows SFTP paths depend on that server's conventions. |
| Legacy SCP | Explicit `scp -O -r`. Requires remote SCP and a POSIX-compatible shell. Remote paths get separate shell quoting; `~/` is supported. Windows shells are unsupported in v1. |
| rsync | `-rt --protect-args --info=progress2`, using SSH. Recursive copy with timestamps, not archive/ownership/permissions preservation. rsync 3.1+ must exist at both ends; native Windows OpenSSH alone is insufficient. |

Review shows FROM/TO and the exact command for every host, execution placement, overwrite warnings, and protocol limitations. Existing files can be overwritten. No automatic legacy fallback and no rsync deletion flag are provided. SCP and rsync have different directory-copy semantics; rsync's `dir/` means contents whereas `dir` means the directory itself.

For rsync, **Preview changes** runs `--dry-run --itemize-changes` against every selected host and returns to review. It never authorizes the subsequent writes; **Run transfers** is a separate confirmation. Previewing is an explicit network operation and may prompt for SSH authentication. Host-key verification is left enabled.

Each confirmed transfer runs independently and keeps success/failure output open until Enter dismisses it. A failed job or failed launcher does not cancel other hosts. Custom `--ssh-config` selection and native jump-host behavior carry through to each client. As with SSH itself, that configuration may authorize local commands. Some OpenSSH builds mishandle spaces/quotes in a custom config filename when constructing their implicit ProxyJump command; use a config filename without those characters if native SSH fails the same way.

Windows legacy-shell support, remote autocomplete, multiple source paths, per-host destination overrides, and multi-host downloads remain deferred. Modern SCP against Windows has not been verified in this container.

## RDP

Press **Ctrl+O** and choose **RDP** (or run `go-where rdp`) to list Windows hosts from your SSH config. A host is an RDP target when a comment inside or directly above its `Host` block is exactly one of the configured tags (`windows` or `rdp` by default), so ordinary SSH hosts stay hidden.

```text
# Windows
Host some-pc
    HostName 192.0.2.10
    ProxyJump bastion
```

Selecting a host opens a loopback SOCKS5 tunnel (`ssh -N -D`) through the host's **last** `ProxyJump` hop, with any earlier hops passed as `-J`, and runs a FreeRDP client through it. A host with no `ProxyJump` connects directly. This keeps the RDP certificate name matching the target instead of the tunnel. If the client lacks SOCKS5 support, the launcher falls back to a loopback port forward and prints a certificate-name warning. Only `ProxyJump` routes are supported; `ProxyCommand` is rejected.

**Enter launches in the background** (no terminal window; output goes to `$XDG_CACHE_HOME/go-where/rdp.log`). **Ctrl+Alt+T launches in a visible terminal** for debugging, using the same terminal key as the other modes. Placement (`rdp.destination`) is set in settings, not chosen from the picker.

The username comes from `rdp.user`, and the domain from `rdp.domain_rules` matched against the ProxyJump hops in order, falling back to `rdp.default_domain`. FreeRDP prompts for the password unless `rdp.password_command` supplies it: the command's stdout is piped to the client (appended as `/p:...` via `/args-from`), never placed on a command line or written to a file.

FreeRDP is detected from `rdp.client` or, if empty, from `xfreerdp3`, `xfreerdp`, `wlfreerdp3`, `sdl-freerdp`, and their `3` variants. SOCKS5 proxying requires FreeRDP 3.x; older clients fall back to a loopback port forward. `go-where rdm-report EXPORT.json` reads a Devolutions RDM JSON export and proposes `# Windows` tags by matching RDM `Url` values to SSH `HostName`s; it is read-only and never edits your config.

### Transfer checks

`go test ./...` covers command construction, quoting, completion, Back navigation, independent batch launches, dry runs, and output retention using synthetic fixtures. To run the optional native transfer test, supply `GO_WHERE_TEST_CONFIG` pointing to a **disposable loopback server sharing the local filesystem** with alias `test-target`:

```sh
GO_WHERE_TEST_CONFIG=/path/to/disposable/config go test -run TestTransferEndpointIntegration -v .
```

The test creates its own temporary source/destination directories and checks SCP, legacy SCP, rsync dry-run safety, directory contents, and uploads/downloads. It can exercise ProxyJump when configured in the fixture. `GO_WHERE_TEST_SSH` optionally selects an extracted test SSH client for SCP. Do not point this test at production hosts.

## Settings

`go-where config` creates and opens `$XDG_CONFIG_HOME/go-where/config.json` (normally `~/.config/go-where/config.json`). JSON keeps the executable dependency-free. Missing fields use these defaults:

```json
{
  "destination": "tmux-pane",
  "fallback": "terminal",
  "editor": ["nvim"],
  "edit_destination": "tmux-pane",
  "terminal": ["auto"],
  "fallback_terminal": ["kitty"],
  "floating_terminal": ["auto"],
  "rdp": {
    "user": "",
    "default_domain": "",
    "domain_rules": [],
    "tags": ["windows", "rdp"],
    "port": 3389,
    "tunnel": "auto",
    "client": [],
    "client_args": ["/dynamic-resolution"],
    "password_command": [],
    "destination": "background"
  }
}
```

Destinations: `tmux-pane`, `tmux-window`, `tmux-session`, `terminal`, `current`. The outside-tmux fallback is `terminal` or `current`. `edit_destination` controls the `go-where config` CLI command; picker shortcuts explicitly choose floating or pane editing. Editing uses the Vim-compatible `+LINE -- FILE` convention.

RDP uses its own `rdp.destination`, which adds `background` (detached, no terminal window; the default) to the destinations above. `rdp.tunnel` is `auto`, `socks`, or `forward`. `rdp.tags` names the SSH-config comment tags that mark Windows/RDP hosts. `rdp.password_command` is an argument array, e.g. `["rbw", "get", "RDP/work"]`.

`terminal: ["auto"]` asks `xdg-terminal-exec` to resolve your preferred terminal using its XDG terminal lists and desktop entries. The resolver returns NUL-separated arguments, not shell code. If the resolver is unavailable or cannot find a terminal, `fallback_terminal` is used; set it to `[]` to disable fallback. `$TERM` is never used to guess your preferred application. A terminal that resolves successfully but fails to launch is reported, not retried in another terminal.

To override terminal selection, use an argument array such as `["kitty"]`, `["alacritty", "-e"]`, or `["foot", "--"]`. Include the command separator your terminal needs. `floating_terminal: ["auto"]` follows that selection and requests app ID `org.gowhere.editor`; with explicit Kitty/Alacritty/foot commands, the matching app-ID flag is inserted. Other explicit terminals need a `floating_terminal` override with their own flags, e.g. `["custom-terminal", "--app-id=org.gowhere.editor", "--"]`. Existing explicit `terminal`/`floating_terminal` arrays are preserved; change them to `["auto"]` to opt into system defaults.

Terminal processes detach from the popup, but their output is retained in `$XDG_CACHE_HOME/go-where/terminal.log` (normally `~/.cache/go-where/terminal.log`, mode 0600). Failures during the first 300ms keep the error popup open and point to this log. Later failures remain in the log; the picker does not supervise terminal lifetimes. If a new terminal still fails to appear, inspect the log on the host for the actual display/session error.

Invalid or unknown settings fail clearly. `go-where config` can still open malformed settings using default editor/placement so you can repair them.

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
# Example only: prefix, then Ctrl+F. go-where opens its own popup.
bind-key C-f run-shell 'go-where --client "#{client_name}"'
```

Passing the client explicitly matters when multiple clients share the same pane. Without it, the command requires exactly one attached client viewing the invoking pane and refuses ambiguity. Session-qualified targets also avoid sending new panes into the wrong session when windows are linked.

## Floating editor on Hyprland / Omarchy

The terminal's app ID/class identifies the editor; the window manager makes it float. System-default selection requests this through `xdg-terminal-exec --app-id`; the selected desktop entry must support that option. The companion rule is installed in this workspace's `thedotfiles/.config/hypr/rules.lua`. For a different Hyprland Lua setup, add:

```lua
hl.window_rule({
    name = "go-where-editor",
    match = { class = "^org.gowhere.editor$" },
    float = true,
    center = true,
    size = { "(monitor_w*3/10)", "(monitor_h*3/4)" },
    no_max_size = true,
})
```

Without an equivalent compositor rule, or if the selected terminal ignores the requested app ID, Ctrl+S/E still open Neovim in a separate terminal, but it may tile instead of float. Physical shortcuts and desktop behavior must be verified on the host, not just in a container.

## Private examples

Put personal/work SSH config samples under `local/`. The entire directory is ignored by Git. Never add private keys. Nothing from `local/` is required to build or test; tests use synthetic documentation-range addresses.
