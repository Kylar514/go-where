# Optional shared inventory

Status: exploratory. No server or synchronization implementation is planned for the first release.

## Goal

Optionally share connection metadata between machines or people while retaining the fast local picker and offline operation. This is a possible step toward a lightweight Remote Desktop Manager alternative, not a commitment to replicate its entire feature set.

This is the third roadmap item. Build on the local source format and trust model from [Directory-backed inventory](02-directory-inventory.md). Start only when a concrete sharing need exists.

## Scope

- Publish and fetch versioned connection inventory.
- Cache an approved local snapshot for offline use.
- Distinguish personal overrides from shared records.
- Authenticate clients and restrict access to appropriate inventories.
- Keep inventory distribution separate from protocol execution and credentials.

## Phased implementation

### 1. Validate the need without a server

First try the directory-backed format with a read-only export, a private Git repository, or another existing distribution mechanism. Establish who shares what, how often it changes, and whether multiple people need to edit the same records.

Do not build a custom service if distributing files solves the actual problem.

### 2. Read-only distribution

If a server becomes justified, begin with authenticated, read-only snapshots over verified TLS. Include a format version and revision identity; validate downloads before publishing them to the local cache.

Download into temporary storage and replace cached snapshots atomically only after complete validation. A failed or partial refresh must leave the last usable snapshot intact.

The picker should show source and freshness information and continue operating from the last approved snapshot when offline. Define stale-data policy explicitly, particularly when access has been revoked or a destination has changed.

### 3. Controlled shared editing

Add writes only if real collaboration requires them. Define ownership, authorization, revision checks, conflict handling, and an audit trail before supporting updates.

Personal overrides must remain separate and must not accidentally publish private addresses, descriptions, usernames, or local file paths back to a shared source. Shared edits must not silently overwrite local changes.

## Security boundaries

- The server distributes inventory; it does not receive SSH private keys, passwords, or session credentials.
- Authentication tokens belong in a suitable local credential store, not inventory files or URLs.
- Shared metadata can itself be sensitive. Minimize disclosure and enforce access checks on every relevant operation.
- The server must not authorize local command execution through arbitrary launcher fields, editor commands, or shell fragments.
- Received SSH configuration remains untrusted even when its download was authenticated. Explicit approval is required before it can supply executable SSH behavior, and updates to approved config need a defined reapproval policy.
- Do not disable TLS, SSH host-key, or RDP certificate verification to make synchronization or connections easier.

Offline availability and immediate revocation conflict: cached records can remain visible after access changes. Establish the required policy rather than promising both without qualification.

## Boundaries

- No remote-control service, session relay, or embedded RDP/SSH implementation.
- No password vault or credential-sharing service.
- No always-online dependency for normal local operation.
- No full Remote Desktop Manager parity as an acceptance criterion.
- No speculative role hierarchy, multi-tenant platform, or fleet-management framework.

## Open questions

- Is the first sharing need one person's devices, a small team, or something larger?
- Can an existing identity provider and distribution service cover authentication and hosting?
- Which records require offline access, and what staleness/revocation rules are acceptable?
- Who owns shared targets, approves executable config changes, and resolves conflicting edits?

## Verification

Before release, test unauthorized access, malformed and oversized responses, interrupted downloads, revision conflicts, offline startup, stale snapshots, and recovery after a failed update. Verify that refreshes cannot change local command configuration, overwrite private source files, or activate unapproved SSH behavior.
