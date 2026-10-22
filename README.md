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
| Ctrl+S | Settings menu; raw editor is available inside |
| Ctrl+N | New database connection; change Type inside the editor |
| Ctrl+E | Edit the highlighted database record in the TUI, or open its legacy source file |
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

## Local SQLite inventory

SQLite is optional. No database is created, imported, or exported at startup; existing SSH-config connections and Diglet tunnels keep their existing behavior.

1. Press **Ctrl+B** or click **Sources**, then **Create local SQLite source**. Enter a display name and database file path, and confirm creation. Existing files are never overwritten. Use **Add existing SQLite source** to attach a database created by go-where (not an arbitrary SQLite file).
2. Press **Ctrl+N** to open **Connection**. **Type** is a field in this editor: SSH, RDP, or Tunnel. Changing it updates the available fields; there is no preliminary type wizard. Choose **Source** in the same editor (it defaults to the selected SQLite source or the first configured source). Common fields are alias, tags and notes. Select **Save connection** when done; nothing is stored until validation succeeds. Changing types requires confirmation before resetting incompatible fields.
3. Press **Ctrl+E** on a database record, including RDP and tunnels, to open that same editor with its values. Edit, duplicate, or delete there. Deleting or converting an SSH record referenced by other connections is blocked. A duplicate gets a new stable ID and may choose another source; changing sources clears SSH references after confirmation. Existing records stay in their source. Esc discards unsaved edits. Stop an active database tunnel before saving edits, converting, or deleting it. Legacy SSH/RDP entries retain their SSH-file editor; legacy tunnels retain their YAML editor. No files are silently imported.
4. Use Sources to choose **All sources**, **SSH config**, or a named local source. Same aliases in different sources remain separate records; marks and focus use source/connection IDs. Host columns show **Host / Hostname or IP / Source / Tags / Notes**. Narrow lists hide Notes first, then Tags; Details retains the full values, even when resolved SSH settings have an error. SSH-config comments appear as Notes, with `SSH config` as their Source. Tunnels retain their own Name/Status columns.
5. Press Enter to connect using the existing tmux/terminal destinations. Managed SSH connections use a private temporary config snapshot and **do not read personal or system SSH configuration**. Each hop has its own user/key; snapshot cleanup happens when SSH exits. Standard host-key verification, agent support, OpenSSH default identities and interactive authentication remain in effect. Blank user uses the OS username. Explicit key paths must be absolute or start with `~/`.

Database SSH, RDP and tunnel records appear in their respective modes. SCP/rsync remain actions on SSH hosts, not connection types; their database integration is still deferred. Remote API sources, SSH-config import/export, bulk profiles and tag rules are deferred. Existing file-based modes remain available. Databases use **schema version 6**, with shared hosts and named SSH/RDP profiles; see **Shared hosts and connection profiles** below for migration and editing. Unknown versions are rejected. Back up databases/settings before upgrading if rollback to an older binary matters. Do not manually edit tables. Keep databases on a local filesystem; network-shared SQLite files are not the future API-server design.

### Type-specific fields

- **SSH:** address, port, user, identity file and same-source SSH jump. Uses the existing managed snapshot launcher.
- **RDP:** address, port (default 3389), **Account**, and optional same-source SSH jump. Account offers **Custom** or a saved local profile from `rdp.accounts`. Custom reveals editable Username and Domain (supporting local variables), uses local username/domain defaults when unspecified, and uses the client's password prompt—never the global password command. A saved profile shows its username read-only; its domain is also read-only when provided, otherwise the connection's domain remains editable. Switching accounts preserves custom values, but hidden values are not resolved or used when overridden by the selected profile. A missing saved profile is an error, not a silent fallback. Alt+A explicitly overrides the record for one launch. Global FreeRDP arguments, destinations, certificates and environment title rules remain local preferences. Only the profile name, never passwords or password commands, is stored with the connection; custom values are retained for switching back. No schema upgrade is required for this account selector.
- **Tunnel:** SSH or kubectl backend, loopback bind IP, local port and remote port. SSH adds a same-source SSH connection reference and remote host/IP (reachable from that SSH host). kubectl adds resource, namespace and context, using local kubeconfig. Switching backend confirms/reset incompatible settings while preserving bind and ports. Local binds may be `127.0.0.10` or another loopback IP; public-interface binds are rejected. Tunnel authentication must work noninteractively through local keys/agent or kubeconfig.

Database tunnels reuse Diglet's detached-process, PID-identity, ownership and listener-status engine on Linux/Windows. They outlive the TUI. SSH configuration snapshots are held in the private cache under `go-where/managed-tunnels/` and removed on normal stop; abnormal termination can leave metadata-only snapshots, overwritten on the next start. Status indicates listener/process ownership, not remote website health. Unlike legacy Diglet tunnels, database tunnels use passive listener polling rather than the probe that assumes `127.0.0.1`. **The registry still reserves ports globally: the same local port cannot be used simultaneously on two different bind addresses.** macOS retains the existing explicit unsupported-tunnel-backend message; editing records and RDP/SSH cross-builds are available, but native runtime validation remains necessary.

Source registrations are stored in the local settings file. Creating/attaching a source appends only its registration, preserving JSONC comments and unrelated settings, using the same validated atomic writer as Settings. A registration failure leaves the database in place so it can be attached after fixing settings. A source whose file is missing or whose identity/schema changed is an error, not an empty source; correct its path or remove its registration in settings. No personal SSH config is changed.

### Searching inventory

Tags are comma-separated in the editor, e.g. `homelab, superfreak`. Plain search fuzzily matches host, address, jump metadata, source name, tags, and notes. `#superfreak` matches that exact tag (case-insensitive), not text in notes. Combine filters with text, e.g. `#homelab #superfreak web`; every specified tag must match, then remaining text is fuzzy-searched. `#tag` syntax uses whitespace-delimited tag names. Hidden/truncated columns remain searchable. Database tunnel tags/notes/source names are searchable too; legacy tunnels have no structured tags.

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

For that example, store `${ADMIN_USER}` as the connection's user and `${SSHKEY}` as its identity file. Supported explicit override fields: `user`, `identity_file`, `address`, `port`, `jump_id`, `ssh_agent`, `identities_only`, `extra_kex_algorithms`, `extra_host_key_algorithms`, `extra_pubkey_algorithms`. An empty `identity_file` clears the stored key; an empty `jump_id` makes the connection direct. Overrides live in local settings, not in the database, so they are not transferred with a copied source file. SSH-agent definitions and compatibility overrides have TUI forms; other variables/overrides still use **Settings → Edit raw settings**.

Database files contain inventory metadata, not stored passwords or key contents. Treat them as private and back them up. New databases and SSH snapshots request owner-only POSIX permissions; Windows uses the containing directory's ACLs, which must be protected appropriately. The pure-Go SQLite driver requires no installed SQLite executable or CGO.

## Tunnels (Diglet)

The Tunnels mode embeds Diglet's existing tunnel engine as a pinned Go dependency; it does not launch a second Diglet executable or modify that project. It reads `~/.config/diglet/connections.yaml` on Linux (the OS user-config directory on Windows). Override this with `--tunnels-config PATH` or the top-level JSON setting `"tunnels_config": "PATH"`. Existing Diglet YAML remains valid and uses the same ownership registry, so the two tools can recognize their managed tunnels.

Press **Enter** or click **Toggle** to start/stop a tunnel. Status distinguishes inactive, connecting, active, and occupied ports. Only owned processes with matching process identities are stopped; unrelated port users and RDP session tunnels are not touched. Managed tunnels intentionally outlive the TUI. **Ctrl+E** edits the selected database tunnel in the TUI, or the legacy YAML config for a file-backed tunnel; **Ctrl+R** reloads. SSH and kubectl are supported; Docker remains unimplemented. SSH-agent/key access and kubeconfig remain local.

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

Host editors and SSH/RDP profile menus expose **Save host** in the footer.
Ctrl+S (the configured menu-save binding) persists the host/profile draft without
leaving the screen, including current connection-profile edits. Headers show
Saved/Unsaved changes; Back discards only changes made since the last save.
Local overrides and tunnel definitions retain their separate-save workflows.
Color choices show a swatch before their name and hex value, even when selected.

### Hotkeys and host colors

**Settings → Hotkeys** edits comma-separated bindings for picker and menu actions.
Conflicts are checked within each scope; ordinary printable letters are rejected
so search remains usable. Empty bindings disable an action, and Restore all
defaults removes overrides. Changes take effect after Settings Save. Help and
menu hints use the configured bindings. Text-field cursor/editing keys and
checklist Space remain native rather than configurable action bindings.

Host editor → **Color** accepts optional `#RRGGBB`, offers a basic palette,
preview and clearing. A single swatch precedes the host name and appears in
Details; SSH/RDP profiles share the host color, and SSH tunnels inherit their
transport host's color. No full-row coloring or automatic tag/environment rules.
Lip Gloss adapts RGB to the terminal palette; exact colors require true-color
support (including Windows Terminal). Very dark colors may be difficult to see.
Colors are optional metadata; the database schema remains v6.

Current launch location and source filter appear in the bottom controls:
**Launch: …** and **Sources: …**, not a separate line beneath the tabs.
Launch changes apply for the session: RDP has its own location (including
background), separate from SSH/transfers. Tunnels have no launch-location control.

In the main picker, drag the vertical List/Details divider or a separator in the
column headings to resize. Widths remain in the current picker session, not the
database or settings file. Minimum widths protect readability. **Ctrl+Z** resets
the layout; a **Reset layout** action appears after resizing in host modes.
Resizing is separate from row selection and double-click launch.

Menu headers show a breadcrumb (`Settings › SSH agents › profile`). Wide menus
use the picker's list/Details split; narrow menus show wrapped guidance above
the list. Guidance uses short paragraphs and bullets rather than full-width
run-on text. Settings Save/Cancel live in the bottom connected action bar;
Ctrl+S saves the draft and Esc backs out without saving.

The picker has six clickable tabs: **SSH, RDP, Tunnels, SCP, Legacy SCP, Rsync**.
Left/Right or Ctrl+H/L cycle through them; Alt+1 through Alt+6 select directly.
Connected controls use tight shared borders without inner padding or background
fills. Tabs occupy two rows, with bold accent text and a missing underline for
the active tab. Actions occupy one row; submenu Back/Done/input controls use the
same compact style with hotkey hints alongside. Controls wrap in narrow terminals.
Host-row selection and checklist/radio markers retain their selection styling.
Single-click a row to select it and view Details; double-click its text to perform
Enter's action for that row only (other marked rows are not launched). Transfer
double-click opens the review workflow, never bypassing confirmation. Tunnel
double-click uses normal start/stop tracking. Clicking the checkbox still marks
a host rather than launching it. Tunnel action buttons display Start or Stop.

Settings menus and checklists use the picker-style bottom search bar (`⌕ Search:`)
and bottom-up lists. **Ctrl+H / Left arrow / Esc** go back; **Ctrl+L / Enter**
activate the highlighted row (toggle in a checklist). **Up / Ctrl+K** move visually
up and **Down / Ctrl+J** move down, wrapping between the first and last rows in
the picker, menus, checklists (including pinned actions), and completion lists.
Mouse-wheel and page scrolling remain bounded. Parent menus retain their filter, highlighted
item and scroll position while visiting submenus; reopening the overall menu
starts a fresh visit. Text-entry fields retain normal cursor movement. Checklist
Done and manual-entry actions remain pinned below the algorithm list.

Screens distinguish **Settings / actions** (editable/submenu rows end in `›`),
**Choose one** (`○` choices and `●` for the current value), and **Select several**
(`[ ]` / `[✓]` checklists with a separate Apply/Done action). Single-choice
selection returns immediately; checklist toggles remain a draft until Done.
Text/path editors place their field label and input below a separator at the
bottom, with completion candidates above and acceptance/cancel controls below.
Ordinary selections apply directly to the draft without a second confirmation.
Changing a tunnel backend only asks again when populated backend settings would
be discarded. Deletion, separate saves and transfer execution retain confirmation.

### Shared hosts and connection profiles (inventory v6)

Database inventory is host-centric: **Ctrl+N** adds a host with one shared name,
address, tags and notes. **Ctrl+E** opens its host editor. Add any number of named
SSH and RDP profiles inside that host; each protocol has exactly one default
when configured. Profile names use letters, digits, dots, underscores or hyphens.
Hosts without profiles remain visible in SSH mode for editing but cannot launch.

- SSH profiles keep independent ports, usernames, identities, agents,
  compatibility settings and jump references.
- RDP profiles keep independent ports, custom login/domain or named local account,
  and an optional SSH jump. Passwords and password commands stay out of inventory.
- Enter uses the default profile. **Connect as** (button or **Alt+P**) selects
  a profile for that launch without changing defaults. The existing RDP account
  override remains available independently.
- Jumps and SSH tunnels reference a specific SSH profile ID, not whichever
  profile is currently default. Changing defaults never repoints saved routes.
- The host's **Tunnels** menu lists its saved SSH forwards. Tunnel edits save
  separately with explicit confirmation. Choose a saved host as the remote target
  to share its address, or use a custom remote address; targets are reached from
  the SSH host. Kubernetes tunnels remain standalone in the Tunnels tab.
- **Local host address override** in the host editor saves under
  `host_overrides.SOURCE_ID.HOST_ID.address`, applying across profiles and saved-host
  tunnel targets. Existing per-profile address overrides take precedence. Other
  SSH local overrides remain keyed by profile/connection ID.

Opening a v1–v5 database transactionally migrates SSH/RDP records to individual
hosts with a **Default** profile, preserving connection IDs, settings, local
override keys, tunnel definitions and jump references. Same-address records are
**never merged automatically**. Back up databases and settings before using this
build: schema v6 is rejected by older builds. There is no combine-host UI yet;
add new SSH/RDP profiles to an existing host to share its address going forward.
The old Type editor remains for standalone tunnels; normal SSH/RDP editing uses
the host editor. Managed SCP/rsync execution, export and configurable hotkeys are
still deferred.

**Ctrl+S** or **Settings** opens a draft-based settings menu: **General** (startup mode), **SSH agents**, **Connection settings & overrides** (SSH agents), and **Edit raw settings**. Save validates and atomically replaces the file; Cancel writes nothing. Raw editing explicitly discards an unsaved draft. Saved-account management, other preferences forms and configurable keybindings are deferred.

The existing `config.json` accepts **JSONC** (`//` and `/* … */` comments and trailing commas); no rename is required. `--config path.jsonc` works too. Existing strict JSON remains valid. Reads and targeted edits use HuJSON; comments and unrelated settings are preserved, duplicate object keys are rejected, and stale drafts fail instead of overwriting external edits. Writers serialize with a `.lock` file; after an abnormal termination, remove an abandoned lock only when no settings write is running. Standard JSON-only tools will not understand files once comments are added.

`go-where config` remains a direct raw-editor/recovery command for `$XDG_CONFIG_HOME/go-where/config.json` (normally `~/.config/go-where/config.json`), even when settings are malformed. Ctrl+Alt+S keeps the direct raw tmux-pane editor. Missing fields use these defaults:

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

### Startup mode

General → Startup mode sets `startup_mode` to `ssh`, `rdp`, `tunnels`, `scp`, `rsync`, or `legacy-scp`. Unspecified means SSH. An explicit command such as `go-where rdp` always wins. Saving the preference changes the next launch, not the view already open.

### SSH compatibility (legacy servers)

An SSH connection's editor has **SSH compatibility…** with four per-connection settings, stored in the database:

- **Identities only:** Default / Yes / No. Yes emits `IdentitiesOnly yes`, restricting offered keys to the configured identity (the same as the SSH-config entry's `IdentitiesOnly yes`).
- **Additional key exchanges:** chosen from a searchable checklist, not typed. Emitted as `KexAlgorithms +name,…`, which **adds** to OpenSSH's defaults rather than replacing them.

  The list comes from your own OpenSSH: `ssh -Q kex` (everything it can negotiate) minus the defaults reported by `ssh -G -F none` (already on). What remains is exactly what a `+` can add. SHA-1 exchanges are marked **⚠ weak** and listed after stronger ones. Space, Enter or a click toggles an entry, typing filters, and **Esc discards** every change made in the list. **Done** applies; **Type a name manually…** adds a bare name the list does not offer (prefixes `+`/`-`/`^`, wildcards, spaces and directives are rejected). Checking nothing means no additions.

  Names already stored on a shared record stay in the list even when your OpenSSH lacks them, marked *not supported by this machine's OpenSSH*, so opening the picker never silently drops them. **Launching with such a name fails with a message naming it** (and Details shows the same), rather than OpenSSH's "Bad SSH2 KexAlgorithms". Fix it by unchecking the name or with a local override. If `ssh` cannot be queried (not installed, or it errors), a built-in list of three common legacy exchanges is shown, the check is skipped, and OpenSSH remains the authority at connect time. The capability query runs once per go-where process.

- **Additional host-key algorithms:** the same checklist, emitting `HostKeyAlgorithms +name,…` for server identity verification. Candidates come from `ssh -Q HostKeyAlgorithms` minus the defaults from `ssh -G -F none`.
- **Additional public-key signature algorithms:** the same checklist, emitting `PubkeyAcceptedAlgorithms +name,…` for user authentication. Candidates come from `ssh -Q PubkeyAcceptedAlgorithms` minus the defaults.

For a legacy RSA server, select the bare name **ssh-rsa** in both new checklists if diagnostics show both exceptions are required. The launcher adds `+` automatically. Do not put `ssh-rsa` in the key-exchange checklist. RSA/SHA-2 signatures are not weak; `ssh-rsa` (including certificates) uses legacy SHA-1 and is marked weak, as is DSA. If OpenSSH cannot be queried, the signature checklists offer only `ssh-rsa` as an unverified fallback plus manual entry. Stored unsupported names remain visible and are rejected at launch when local support is known. Capability results are cached per directive for the process.

Legacy algorithms are weak, so enable only what a specific server requires. All exceptions remain connection-specific; global defaults and host-key verification are unchanged. The agent choice is separate: selecting the legacy agent changes which agent signs, not which algorithms are negotiated.

To override locally, use **Settings → Connection settings & overrides → (host) → SSH compatibility**, or **Local compatibility override…** in the connection editor (this saves local settings separately). Each field can **inherit** the database value or be set locally. For key exchanges, choosing "Choose local additions…" opens the same checklist, starting from the list that currently applies so removing one entry is one click. Checking nothing is an explicit empty local list that disables the database's additions for you; "Inherit" follows the database. Local settings example:

```jsonc
"overrides": {
  "SOURCE_ID": {
    "CONNECTION_ID": {
      "identities_only": true,
      "extra_kex_algorithms": ["diffie-hellman-group1-sha1"],
      "extra_host_key_algorithms": ["ssh-rsa"],
      "extra_pubkey_algorithms": ["ssh-rsa"]   // [] disables DB additions; omit to inherit
    }
  }
}
```

Settings apply per hop: the destination and each jump host use their own record's values, including jumps used by RDP and SSH tunnels. Details shows the effective value and whether it came from a local override, the connection, or the system default.

### SSH agents and local agent overrides

1. Settings → SSH agents → Add. Give the profile a name, such as `legacy`, and either an environment variable (`SSH_AUTH_SOCK_LEGACY`) or a literal absolute socket/Windows pipe path. Apply the profile to the draft, then Save Settings.
2. An SSH connection's **Agent** is an optional database default: unspecified/system default, Disabled, or a named local profile. Only the profile name goes in inventory; endpoints remain local settings.
3. To override it locally, choose Settings → Connection settings & overrides and select an SSH record. **Inherit** removes the override; **System default** explicitly overrides a database profile with normal OpenSSH behavior; **Disabled** writes `IdentityAgent none`; a named profile uses its resolved endpoint. Save Settings to commit. RDP/tunnel agent choices belong to their referenced SSH records.
4. Existing SSH connection editors also offer **Local agent override…**, with an explicit, separate local-settings Save. This does not modify the database, and cancelling the connection editor later does not undo that separately confirmed save. Other unsaved connection edits remain in the editor.

Precedence is **local override → database connection default → system default**. Effective selection/origin is shown in the editor and Details. Each destination/jump gets its own `IdentityAgent` in the private SSH snapshot, including jumps used by RDP/tunnels. No agent forwarding or legacy SSH algorithms are enabled. Disabling an agent does not disable key-file or password authentication.

System default emits no custom directive: Linux uses normal OpenSSH/`SSH_AUTH_SOCK`; Windows keeps its OpenSSH client's agent discovery. Named Unix endpoints must exist and be socket files; socket liveness/signing remains the client's responsibility. Named Windows endpoints must be absolute pipe/socket paths supported by the installed OpenSSH client; Windows runtime validation is still required. Missing definitions or unset environment variables fail clearly, never by silently selecting a different agent. Existing SSH-config-backed launches remain governed by their own SSH configuration, not these database overrides.

Environment variables must be inherited by the go-where process. A custom variable may need adding to tmux's `update-environment`/server environment when launching through a binding. Agent changes affect new sessions; stop/restart an existing tunnel to regenerate its snapshot.

Example local settings (merge with existing settings):

```jsonc
{
  // Default first view; an explicit CLI mode wins.
  "startup_mode": "rdp",
  "ssh_agents": {
    "legacy": { "socket_env": "SSH_AUTH_SOCK_LEGACY" },
  },
  "overrides": {
    "SOURCE_ID": {
      "CONNECTION_ID": { "ssh_agent": "legacy" },
    },
  },
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

Without an equivalent compositor rule, or if the selected terminal ignores the requested app ID, legacy file editing and Settings' raw-editor action still open Neovim in a separate terminal, but it may tile instead of float. Database connection forms and Ctrl+S Settings stay inside the TUI. Physical shortcuts and desktop behavior must be verified on the host, not just in a container.

## Private examples

Put personal/work SSH config samples under `local/`. The entire directory is ignored by Git. Never add private keys. Nothing from `local/` is required to build or test; tests use synthetic documentation-range addresses.

## License

[MIT](LICENSE)
