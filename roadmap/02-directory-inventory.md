# Directory-backed inventory

Status: proposed. Configurable multi-directory discovery and supplemental target metadata are not implemented yet.

## Goal

Collect connection targets from a configured set of local directories while keeping existing SSH config useful and authoritative for SSH behavior. Support personal, work, and project inventories without requiring a database or server.

This is the second roadmap item. Design a small on-disk format that could later be distributed by the optional server described in [Shared inventory](03-shared-inventory.md), without building that server now.

## Scope

- Explicitly configured directories and source files, not a scan of the entire home directory.
- SSH configuration sources and supplemental metadata for connection types such as RDP.
- Source filename and location tracking for previewing and editing.
- Searchable descriptions, groups, and protocol availability where explicitly declared.
- Predictable behavior when sources overlap, disappear, or contain errors.

## Phased implementation

### 1. Source discovery

Add a configuration list of inventory locations. Define supported filenames, scan depth, symlink behavior, and deterministic source ordering before adding recursive discovery.

Keep the default `~/.ssh/config` workflow working without extra configuration. Respect OpenSSH Include semantics rather than assuming paths are relative to the discovered file.

Every SSH target must retain the context needed to connect exactly as intended. Merely discovering a `Host` block in a directory does not make it part of the user's active SSH configuration. Decide whether each source is an explicitly selected root config or must be included from a trusted root, and carry that choice into SSH/SCP/rsync execution.

### 2. Supplemental target metadata

Define a minimal, versioned metadata format only when the first non-SSH target needs it. Prefer existing standard-library support and avoid a speculative universal connection schema.

Candidate fields include a stable target ID, display name, protocol, destination address/port, description, group, and an SSH jump-alias reference. RDP may also need a username/domain or a reference to an external credential provider. Store no passwords, private keys, or authentication tokens in the inventory.

Avoid copying SSH settings into a second source of truth. Supplemental metadata references SSH aliases and their source context instead.

### 3. Consistent browsing and editing

Merge discovered targets into the picker with visible source identity and protocol-specific views. Preserve the established searchable list and explicit destination choices.

Define duplicate handling before merging: two workspaces can legitimately use the same alias for different hosts. Do not silently collapse them or connect through whichever source happened to load first.

Edits open the canonical local source at the relevant location. Read-only or imported sources must be visibly distinguished; editing must not overwrite generated caches or downloaded content by accident.

## Trust and safety

SSH configuration is executable authority, not harmless metadata: `ProxyCommand`, `Match exec`, and related settings can run local commands. Source discovery must not execute them.

Treat downloaded, shared, and newly discovered configuration as untrusted until the user explicitly approves its use. Directory scanning must not silently add files to active SSH configuration or expand an approved file's scope through unexpected includes or symlinks.

Define validation, source boundaries, and approval behavior before accepting external inventory packages. Importing metadata never authorizes arbitrary shell commands.

## Boundaries

- No inventory server, synchronization engine, or database in this phase.
- No automatic rewriting or cleanup of personal/work SSH configs.
- No credential vault.
- No inference that every host supports SSH, RDP, SCP, and rsync.

## Open questions

- What are the actual directory layouts and comment conventions in the user's inventory?
- Should source groups be declared in configuration, inferred from directory names, or both?
- How should stable IDs and alias/source collisions appear in the picker?
- Which sources are writable, and what constitutes explicit approval of an imported SSH config?

## Verification

Use synthetic inventories to test deterministic ordering, duplicate aliases, missing sources, malformed metadata, include cycles, symlink boundaries, and source-aware command generation. Prove that discovery does not execute config commands or modify source files.
