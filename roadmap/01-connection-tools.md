# Connection tools

Status: initial SCP, legacy SCP, rsync, and RDP implementation complete.

Implemented: Ctrl+O in-picker mode selection; Back-capable local/remote path setup with local Tab completion; multi-host uploads and single-host downloads; explicit review; rsync dry runs; source-aware native commands; independent jobs with output held until dismissal. Native SCP/SFTP, legacy SCP, and rsync uploads/downloads were checked against a disposable loopback SSH endpoint, including ProxyJump. Windows endpoint behavior is not verified. See the [transfer guide](../README.md#transfers) for current behavior and limits.

## Goal

Extend the existing SSH host picker into a fast connection and transfer launcher. Reuse installed protocol clients instead of implementing SSH, file transfer, or remote desktop protocols ourselves.

This is the first roadmap item. Directory inventory and a shared server are not prerequisites.

## Scope

- SCP upload and download through existing SSH aliases.
- Explicit legacy SCP mode using `scp -O` for targets without SFTP support.
- rsync upload and download for efficient repeated transfers.
- Later, RDP connections through SSH tunnels or native RD Gateway support.
- Continue supporting intentional tmux pane/window/session and external-terminal placement where appropriate. Graphical RDP clients open their own windows.

## Phased implementation

### 1. SCP and legacy SCP

Proposed commands:

```sh
go-where scp
go-where scp --legacy
```

Start with one host per transfer. Pick the host, choose upload/download, supply local and remote paths, then confirm the generated operation before running it. Allow required values to be supplied as arguments later without forcing interactive prompts.

Use OpenSSH aliases to retain configured users, ports, keys, and jump hosts. Propagate an explicitly selected SSH config to the transfer client using its supported options.

Modern SCP uses SFTP by default. Legacy mode is an explicit choice, not an automatic retry: authentication, permissions, and path errors must not silently trigger a different protocol. Legacy SCP interprets remote paths through a shell, so its quoting and validation need separate tests.

Show progress and preserve failures in the execution pane or terminal. Do not report a transfer as successful merely because its terminal launched.

### 2. rsync

Proposed command:

```sh
go-where rsync
```

Reuse host selection, direction, path entry, confirmation, and execution placement. Use rsync over SSH, preserving SSH config selection and jump-host behavior.

- Offer `--dry-run --itemize-changes` before actual execution.
- Show transfer progress and meaningful exit status.
- Do not enable `--delete` by default; any future deletion option needs explicit confirmation.
- Preserve and explain trailing-slash semantics: `folder` and `folder/` have different meanings.
- Surface missing rsync on either endpoint clearly. Do not silently fall back to SCP.

### 3. RDP

Implemented. Pick RDP from Ctrl+O, or run:

```sh
go-where rdp
```

Only SSH hosts carrying an explicit RDP tag comment are listed. A host's last `ProxyJump` becomes the bastion; the launcher opens a loopback SOCKS5 tunnel (`ssh -N -D`) through it and points FreeRDP at the real target host, so the RDP certificate name still matches. Earlier hops are passed as `-J`. The tunnel is bound to loopback, readiness-checked, and cleaned up when the client exits or launch fails; the port is allocated, not fixed. The CLI filename does not determine this mode; the CLI remains `go-where`.

Preserve server certificate and identity verification across the tunnel. Do not bypass certificate checks to work around a localhost connection name. Passwords belong in client prompts or supported credential stores, not target metadata or command-line arguments.

## Boundaries

- No embedded file manager or remote desktop rendering engine.
- No automatic protocol downgrade, deletion, or credential storage.
- No multi-host transfers initially. Upload fan-out and download naming/collision rules can be designed when needed.
- No network probes merely from browsing the inventory.
- No requirement for an inventory server.

## Open questions

- Which local path picker gives a useful minimum experience without adding a full file browser?
- What are the initial rsync flags, and which metadata-preservation options are safe across the intended endpoints?
- Which RDP client should be supported first, and are the real gateways SSH bastions, RD Gateways, or both?
- How should cancellation affect partial transfers, subprocesses, and temporary tunnels?

## Verification

Test argument construction and path handling for spaces, quotes, leading dashes, IPv6, custom SSH configs, and jump hosts. Exercise success, cancellation, and failure against disposable endpoints before claiming protocol support.
