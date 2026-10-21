# go-where

A lightweight, keyboard- and mouse-driven connection workspace. One Go executable with a Bubble Tea TUI: SSH, RDP, managed tunnels, and transfers using installed native tools. Credentials stay local. This branch is the first unified-TUI prototype; no shared server or authentication service is included yet.

## Build and run

Requires Go 1.26.2+ to build (matching the reused Diglet module). Runtime requirements depend on the action: OpenSSH for SSH/SCP, FreeRDP for RDP, kubectl for Kubernetes tunnels, rsync 3.1+ for rsync, and `ss` for Linux tunnel monitoring. fzf is no longer required. tmux is optional; Linux terminal auto-selection uses `xdg-terminal-exec` or your configured terminal. Editing uses your configured editor (default Neovim). Go is not needed to run the compiled executable.

```sh
go test ./...
mkdir -p "$HOME/.local/bin"
CGO_ENABLED=0 go build -trimpath -o "$HOME/.local/bin/go-where" .
go-where
```

The TUI runs in the current terminal by default, capturing the originating tmux session/window/pane/client when available. Connections still open in that original tmux target. Outside tmux, the configured fallback is used; **Target / Ctrl+G** lets you explicitly select an existing tmux pane or another destination. It never guesses a "current" tmux session. Use `--popup` to opt into a tmux popup; it closes automatically after a successful launch (SSH, RDP or transfer) so the connection takes over. Failed launches, cancels and edits keep it open. In a standalone terminal the picker stays open after launching.

```sh
go-where --destination tmux-window
go-where --destination current          # SSH in the picker terminal; one host only
go-where --popup                       # optional tmux popup; default is standalone
go-where --ssh-config ./local/config    # also passes this file to ssh -F
go-where config                        # create/open settings without overwriting them
go-where --help
go-where scp                           # Start in modern SCP mode
go-where scp --legacy                  # Explicit legacy SCP; never an automatic retry
go-where rsync
go-where tunnels                       # Existing Diglet connections, start/stop/status
go-where tunnels --tunnels-config ./local/connections.yaml
```

With the default SSH config, connections use `ssh -- alias`, preserving normal user and system SSH configuration. With `--ssh-config`, connections use `ssh -F path -- alias`; OpenSSH's usual `-F` semantics apply (including skipping the system config).

## Keys

Type to search aliases, configured hostnames/IPs, jump hosts, and descriptions. Plain `j`/`k` remain search characters, not modal Vim navigation.

Click a mode tab or use **Alt+1/2/3/4** for SSH/RDP/Tunnels/Transfers. Wide terminals show a scrolling host list beside a details preview; narrow terminals hide the preview. Lists and dialogs resize with the terminal rather than requesting a fixed height. The minimum picker size is 30 columns × 12 rows; smaller terminals show a resize hint and cannot launch hidden actions.

Rows show host aliases and addresses; the preview shows jump routes, comments, source location, and the SSH block. This remains block-level metadata, not evaluation of wildcard/Match defaults; OpenSSH determines the actual route. Click a row to select it, its checkbox to mark it, and **Connect** to launch. Mouse wheel scrolls the list or preview beneath the pointer.

| Key | Action |
|---|---|
| ? | Open mode-specific shortcut help; Enter or Esc returns without losing picker state. Reserved for help, not search input. |
| Alt+A | In RDP mode, choose an account and launch selected hosts (Esc returns without launching). |
| Ctrl+J / Ctrl+K or arrows | Move down / up |
| Tab / Shift+Tab | Toggle selection and move to the next / previous result |
| Enter | Connect using your configured default |
| Ctrl+O | Choose SSH, SCP, Legacy SCP, rsync, RDP, or Tunnels |
| Alt+1 / 2 / 3 / 4 | SSH / RDP / Tunnels / Transfers |
| ← / →, Ctrl+H / Ctrl+L | Previous / next mode, wrapping at either end |
| Ctrl+G | Choose a tmux pane or terminal destination |
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

N means new session, leaving S for settings. The terminal/compositor must pass the configured shortcuts through. Use the mouse or Ctrl+O when an Alt+number binding is reserved by your terminal.

Explicit tmux actions require tmux; only the default Enter action uses the configured fallback. Panes are split horizontally from the original pane. Large selections can run out of pane space: failures report how many hosts opened and never silently reroute the rest. A failed new-session operation may leave a partially populated session; no existing session is deleted or replaced.

Popups have a rounded, terminal-background frame titled `go-where`, capped at 150 columns. The reverse list shows at most 20 entries beside a rounded Details box in a 70/30 split. Search sits below the list, with one shortcut footer below the buttons. Ctrl+H switches tabs; terminals that encode Backspace as Ctrl+H must send DEL instead to delete search text.

## Local SQLite inventory (first milestone)

SQLite is optional. No database is created, imported, or exported at startup; existing SSH-config connections and Diglet tunnels keep their existing behavior.

1. Press **Ctrl+B** or click **Sources**, then **Create local SQLite source**. Enter a display name and database file path, and confirm creation. Existing files are never overwritten. Use **Add existing SQLite source** to attach a database created by go-where (not an arbitrary SQLite file).
2. Press **Ctrl+N** to create an SSH connection. When viewing All sources or SSH config, choose the destination SQLite source first. Fill in alias, IP/DNS address, port, optional user/key, tags and notes. Choose an existing same-source connection as the jump host, then **Save connection**. Nothing is saved until Save succeeds.
3. Press **Ctrl+E** on a managed record to edit, duplicate, or delete it. Deleting a referenced jump is blocked. A duplicate gets a new stable ID. Esc discards unsaved edits.
4. Use Sources to choose **All sources**, **SSH config**, or a named local source. Same aliases in different sources remain separate records; marks and focus use source/connection IDs. Managed rows carry source labels, and Details shows resolved SSH settings or missing-variable errors.
5. Press Enter to connect using the existing tmux/terminal destinations. Managed SSH connections use a private temporary config snapshot and **do not read personal or system SSH configuration**. Each hop has its own user/key; snapshot cleanup happens when SSH exits. Standard host-key verification, agent support, OpenSSH default identities and interactive authentication remain in effect. Blank user uses the OS username. Explicit key paths must be absolute or start with `~/`.

Managed records currently appear only in **SSH mode**. Managed RDP, transfer and tunnel integration, remote API sources, SSH-config import/export, bulk profiles and tag rules are deferred. Existing file-based modes remain available. Databases use schema version 1; unknown schema versions are rejected rather than modified. Do not manually edit database tables. Keep databases on a local filesystem; sharing a SQLite file over a network share is not the future API-server design.

Source registrations are stored in the local settings file. Creating/attaching a source updates that file atomically (it may reformat JSON and include default settings). A registration failure leaves the database in place so it can be attached after fixing settings. A source whose file is missing or whose identity/schema changed is an error, not an empty source; correct its path or remove its registration in settings. No personal SSH config is changed.

### Variables and per-connection local overrides

User, address and identity-file fields support **`${NAME}`** placeholders. Values come only from the local go-where settings—not the shell environment—and substitution is literal, single-pass, and never executes commands. Missing variables block launch; saving a template before variables are configured is allowed. Per-connection variable overrides win over default variables. Explicit overridden fields replace stored fields before substitution, independently for each hop.

Add these settings alongside your existing settings; use the source and connection IDs shown in settings/Details:

```json
{
  "variables": {
    "ADMIN_USER": "my-admin-account",
    "SSHKEY": "~/.ssh/company_key"
  },
  "overrides": {
    "SOURCE_ID": {
      "CONNECTION_ID": {
        "variables": {"SSHKEY": "~/.ssh/alternate_key"},
        "user": "another-account"
      }
    }
  }
}
```

For that example, store `${ADMIN_USER}` as the connection's user and `${SSHKEY}` as its identity file. Supported explicit override fields: `user`, `identity_file`, `address`, `port`, `jump_id`. An empty `identity_file` clears the stored key; an empty `jump_id` makes the connection direct. Overrides live in local settings, not in the database, so they are not transferred with a copied source file. Variable/profile editing in the TUI is not included yet; Ctrl+S opens local settings.

Database files contain inventory metadata, not stored passwords or key contents. Treat them as private and back them up. New databases and SSH snapshots request owner-only POSIX permissions; Windows uses the containing directory's ACLs, which must be protected appropriately. The pure-Go SQLite driver requires no installed SQLite executable or CGO.

## Tunnels (Diglet)

The Tunnels mode embeds Diglet's existing tunnel engine as a pinned Go dependency; it does not launch a second Diglet executable or modify that project. It reads `~/.config/diglet/connections.yaml` on Linux (the OS user-config directory on Windows). Override this with `--tunnels-config PATH` or the top-level JSON setting `"tunnels_config": "PATH"`. Existing Diglet YAML remains valid and uses the same ownership registry, so the two tools can recognize their managed tunnels.

Press **Enter** or click **Toggle** to start/stop a tunnel. Status distinguishes inactive, connecting, active, and occupied ports. Only owned processes with matching process identities are stopped; unrelated port users and RDP session tunnels are not touched. Managed tunnels intentionally outlive the TUI. **Ctrl+E** creates/edits the tunnel config; **Ctrl+R** reloads it. SSH and kubectl are supported; Docker remains unimplemented. SSH-agent/key access and kubeconfig remain local.

```yaml
connections:
  - name: database
    tunnel_type: ssh
    ssh_host: db-jump
    local_port: 15432
    remote_port: 5432
```

## Platform status

- **Linux:** unit tests and real-terminal smoke checks cover navigation, mouse clicks, account selection, native launches, and tunnel lifecycle using synthetic endpoints. Existing native connection tests remain in place.
- **Windows:** cross-compiles; native terminal auto-selection uses Windows Terminal (`wt.exe`). Diglet supplies a Windows process/listener backend. Runtime verification is still required. FreeRDP password-command piping is explicitly unsupported here for now; use the client prompt rather than putting passwords on argv.
- **macOS:** cross-compiles; terminal auto-selection uses Terminal.app through AppleScript. Managed tunnel monitoring/start/stop is explicitly unavailable until the Darwin backend is added. Native connection behavior still needs macOS testing.

Cross-compilation is not a claim of complete platform support. No installer, desktop shortcut, shared-inventory server, or SSO is included in this first UI merge. The tunnel engine comes from [Diglet](https://github.com/kylar514/diglet), also MIT-licensed.

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

Press **Ctrl+O** and choose **RDP** (or run `go-where rdp`) to list Windows hosts from your SSH config. A host is an RDP target when an indented comment inside its `Host` block (or an inline comment on a directive) is exactly one of the configured tags (`windows` or `rdp` by default). Unindented standalone comments and section headings are ignored.

```text
Host some-pc
    # Windows
    HostName 192.0.2.10
    ProxyJump bastion
```

Selecting a host opens a loopback SOCKS5 tunnel (`ssh -N -D`) through the host's **last** `ProxyJump` hop, with any earlier hops passed as `-J`, and runs a FreeRDP client through it. A host with no `ProxyJump` connects directly. This keeps the RDP certificate name matching the target instead of the tunnel. If the client lacks SOCKS5 support, the launcher falls back to a loopback port forward and prints a certificate-name warning. Only `ProxyJump` routes are supported; `ProxyCommand` is rejected.

**Enter launches in the background** (no terminal window; output goes to `$XDG_CACHE_HOME/go-where/rdp.log`). **Ctrl+Alt+T launches in a visible terminal** for debugging, using the same terminal key as the other modes. Placement (`rdp.destination`) is set in settings, not chosen from the picker.

Certificate handling defaults to FreeRDP's `/cert:tofu` (trust on first use): the first certificate is accepted and remembered automatically; changed certificates are rejected. First-use trust assumes a trusted network/server. An explicit certificate option in `rdp.client` or `rdp.client_args` overrides this default. With local port-forward fallback, the temporary loopback endpoint may prevent stable trust tracking; prefer SOCKS-capable FreeRDP for a stable target identity.

The username comes from `rdp.user`, and the domain from `rdp.domain_rules` matched against the ProxyJump hops in order, falling back to `rdp.default_domain`. FreeRDP prompts for the password unless `rdp.password_command` supplies it: the command's stdout is piped to the client (appended as `/p:...` via `/args-from`), never placed on a command line or written to a file.

### Alternate RDP accounts

Add shared profiles to `rdp.accounts` in your private `~/.config/go-where/config.json` (merge this into your existing `rdp` settings):

```json
"accounts": [
  {
    "name": "AD administrator",
    "user": "admin-user",
    "domain": "ad.example",
    "password_command": ["rbw", "get", "RDP/admin"]
  },
  {
    "name": "Other AD account",
    "user": "other-user",
    "domain": "other.example",
    "password_command": ["rbw", "get", "RDP/other"]
  }
]
```

In RDP mode, **Alt+A** opens the account chooser; choosing a profile launches marked hosts (or the highlighted host) with that account. **Esc** returns to the unchanged picker. **Default** uses your existing settings, as does ordinary **Enter**. Profiles are not remembered between launches, and credentials are never automatically retried.

Omit `domain` to keep your existing domain rules; an explicit domain overrides those rules. Omit `password_command` to let the client prompt—it never borrows the default account's password command. Unlock `rbw` before a background launch. Keep profiles private and passwords in the vault, not in this JSON.

FreeRDP is detected from `rdp.client` or, if empty, from `xfreerdp3`, `xfreerdp`, `wlfreerdp3`, `sdl-freerdp`, and their `3` variants. SOCKS5 proxying requires FreeRDP 3.x; older clients fall back to a loopback port forward. `go-where rdm-report EXPORT.json` reads a Devolutions RDM JSON export and proposes `# Windows` tags by matching RDM `Url` values to SSH `HostName`s; it is read-only and never edits your config.

### RDP window metadata

Windows show the SSH alias and resolved login, for example `[QA] app-qa — ad.example\operator`. Alternate accounts update the login in the title. Unmatched hosts get an alias/login title without an environment prefix.

The X11 FreeRDP **connection bar** is pinned and visible by default in both windowed and fullscreen modes. It shows that title without a compositor plugin. Unpin it for auto-hide; move to the top edge to reveal it. Set `rdp.title_bar` to `false` to disable the bar for new sessions. This is a client-drawn overlay, not a Hyprland title bar; other FreeRDP frontends may not implement it. Explicit `/title:` (or `/t:`) and `/floatbar:` client options override defaults. Custom titles need the environment prefix to retain border matching.

Add rules inside your private `rdp` settings, for example:

```json
"title_bar": true,
"environments": [
  { "name": "PROD", "jump": "prod-jump*" },
  { "name": "DEV", "jump": "lab-jump*", "aliases": ["*dev*"] },
  { "name": "QA", "jump": "lab-jump*", "aliases": ["*qa*"] },
  { "name": "STG", "jump": "lab-jump*", "aliases": ["*stg*", "*tst*"] },
  { "name": "AZURE", "aliases": ["az-*"] }
]
```

Matching is case-insensitive and ordered: first match wins. `jump` matches any ProxyJump hop's hostname (without SSH user/port); `aliases` matches any listed pattern against the SSH alias. If both are present, both must match. At least one is required. Names are unique letters/digits/spaces/underscores/hyphens. New environments need no Go changes. Labels are visual hints, not server environment verification or access controls.

Border colors and sizes belong entirely to your window manager, not this binary or its JSON config. For example, add an ordinary rule to your Hyprland `rules.lua`:

```lua
hl.window_rule({
  name = "rdp-qa",
  match = { class = "^xfreerdp.*$", title = "^\\[QA\\] .*" },
  border_color = "rgb(ebb450) rgb(ebb450)",
  border_size = 3,
  decorate = true,
})
```

Reload Hyprland after changing its rules. No generator, loader, or running `go-where` process is needed to apply styling. If upgrading from the experimental generated rules, remove `color` fields from `rdp.environments`; unknown settings are rejected.

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
    "accounts": [],
    "environments": [],
    "title_bar": true,
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
- Uses inline comments and indented comments within a `Host` block. Unindented standalone headings and common commented-out SSH directives are excluded from search descriptions but remain visible in the source preview. It does not rewrite your files.
- Does not run `ssh -G`, `Match exec`, ProxyCommand, or network probes while browsing. Only connecting runs OpenSSH and the commands authorized by your SSH config.
- Includes inside `Match` are rejected explicitly rather than guessed. Include environment/token expansion, system-wide host discovery, and `known_hosts` discovery are not implemented. Missing Include glob matches are ignored, as in OpenSSH; cycles, unreadable matched files, and malformed quoting are errors.

Preview data is kept in a mode-0600 temporary file and removed on normal exit. An uncatchable kill can leave it behind in the system temporary directory. Source text is control-character sanitized before display; sources are never modified by the picker itself.

## tmux launch binding

No launch key is imposed. Once you choose one, a binding can launch the same executable, for example:

```tmux
# Example only: prefix, then Ctrl+F. Opt into a popup while retaining tmux origin.
bind-key C-f run-shell 'go-where --popup --client "#{client_name}"'
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

## License

[MIT](LICENSE)
