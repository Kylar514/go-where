# Remote inventory server (first version)

Shared hosts, profiles, attachments, and tunnels remain in ordinary server-local
inventory SQLite files. Clients use HTTPS JSON, never a mounted SQLite file.
Private identities, agent definitions, keys, passwords, and personal selections
remain on each device. No registration website or password accounts exist.

## Build and initialize

```sh
CGO_ENABLED=0 go build -trimpath -o go-where-server .
./go-where-server --data ./server-data inventory create shared
./go-where-server --data ./server-data user create alice
./go-where-server --data ./server-data user grant alice shared read-write
./go-where-server --data ./server-data token create alice work-laptop 90
```

The last command displays a random token once. Deliver it through a secure
channel; never paste it into command arguments, logs, or source control.
Tokens are SHA-256 hashed server-side, expire, and have no administrator powers.
Permissions belong to users, not tokens. Use `read-only` instead of `read-write`
for browsing-only users; `none` removes a grant. Add more inventories with
`inventory create NAME` or `inventory add NAME /absolute/path/inventory.db`.
Stable source IDs survive registration. Do not edit server inventory files with
other processes while the API is serving them; run only one server per data set.

## HTTPS

```sh
./go-where-server --data ./server-data --listen 0.0.0.0:8443 \
  --tls-cert /secure/server.crt --tls-key /secure/server.key serve
```

Certificates must match the hostname and be trusted by client operating systems.
There is no insecure certificate bypass. For a reverse proxy, bind the server
to `127.0.0.1:8080` without TLS and terminate HTTPS at the proxy. The server
rejects public plaintext binds. Do not publish a plaintext backend externally.
Protect certificate private keys and all data directories with owner-only access.
An exclusive `serve.lock` prevents two cooperating servers using the same data
directory. After an unclean stop, verify the old process is gone before removing
that lock. This does not authorize concurrent external SQLite editing.

The same executable also accepts `go-where server ...` if named `go-where`.
Global flags precede commands. Run as an unprivileged service account with
write access only to its data directory; restart via your service manager.
Back up metadata and all registered inventories with a consistent SQLite backup
or while the server is stopped. Never put SQLite files on network mounts.

## Client

1. Settings → **Add API server**, or Sources → **Add API server**.
2. Enter `https://inventory.example.com:8443` and the masked token.
3. Select an authorized inventory. Repeat to register another inventory.
4. Select local identities/agents as usual, then browse/edit/connect.

The token is stored in Linux Secret Service / Windows Credential Manager. Only
URL, source ID, permission hint, and credential reference enter local settings.
The OS secret store must be available; no plaintext fallback is provided.
Remote profiles can include usernames/key path templates but never private key
contents or go-where password records. Use local identities for device-specific
paths. Check imported inventories before sharing to avoid disclosing metadata.

Inventory snapshots are cached only for the current client session. Reloading
the main inventory fetches new data; no offline write queue exists. Saves use an
inventory revision and reject stale writes. Cancel/reload/review to resolve a
conflict; drafts are not automatically merged. Writes spanning inventories and
local settings are separate commits and may partially succeed, as in local mode.

Settings → **API device tokens** lists only your own token names/expiry, creates
new device tokens, revokes them, and rotates the active token. Rotation saves and
verifies a replacement before revoking the previous credential. Additional
tokens are shown once for secure transfer. They inherit all current user grants.
Revoking the active token disconnects that device; another token/admin can recover.

Administrator operations:

```sh
./go-where-server --data ./server-data token list alice
./go-where-server --data ./server-data token revoke TOKEN_ID
./go-where-server --data ./server-data user disable alice
```

Disabling an account rejects all its tokens. There is no self-service permission
change, password recovery, SSO, anonymous access, or public signup.

## Docker

```sh
docker build -f Dockerfile.server -t go-where-server .
```

Create a local data directory writable by container UID 65532. Initialize it with
the same commands above using `docker run --rm -v /srv/go-where:/data
go-where-server --data /data ...`. Start TLS service:

```sh
docker run --name go-where-server --restart unless-stopped \
  --read-only --cap-drop ALL --security-opt no-new-privileges \
  -p 8443:8443 -v /srv/go-where:/data -v /srv/go-where-tls:/tls:ro \
  go-where-server
```

Keep all inventory paths inside mounted local storage. No secrets are baked into
the image. Protect `/data` and `/tls` from other users.

## Verification on two real clients

Create two users/device tokens, grant read-write to one and read-only to the
other. Confirm discovery isolation, blocked read-only edits, normal SSH/RDP/
transfer/tunnel launching, and stale-save rejection when both writers open the
same inventory before saving. Test revoke, expiry, rotation, server restart,
unavailability, and Linux/Windows OS credential stores before deploying broadly.

This initial API has no offline synchronization, automatic TLS provisioning,
browser administration, or request audit database. The bounded global API lock
favors simple authorization/revision correctness over high throughput.
