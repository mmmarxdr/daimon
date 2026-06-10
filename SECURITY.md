# Security

## Security model

Daimon is a **single-user, self-hosted** AI agent. You run it on infrastructure you control — your laptop, a VPS, a home server — and you are the only person who uses it. The security design follows from that: protect your data at rest, prevent the agent's network tools from being turned against your internal network, and authenticate the one browser session that reaches the web UI. Daimon is **not** designed as a multi-tenant hosted service; the gaps documented below are acceptable trade-offs for single-user operation, not oversights.

---

## Threat model

### In scope

| Threat | Where |
|--------|-------|
| SSRF — agent tools fetching internal network endpoints | `http_fetch` / `web_fetch` tools |
| Webhook injection — unauthenticated or forged payloads reaching the agent | WhatsApp webhook handler |
| Local data at rest — conversation history, memory, media blobs readable if the DB file is accessed | SQLite store (`daimon.db`) |
| Stolen / long-lived web UI token | `internal/web/auth*.go` |
| Prompt injection via a malicious MCP server | MCP subprocess lifecycle |

### Explicitly out of scope

- **Hostile local user** — if an attacker already has OS-level access to the machine running daimon, the game is over regardless of in-process controls.
- **Multi-tenant isolation** — daimon has one user, one token, one database. No tenant-separation design exists.
- **TLS termination** — handled by your reverse proxy (nginx, Caddy, Tailscale). Daimon's HTTP listener is not TLS by default.

---

## Implemented protections

### SSRF guard — `http_fetch` and `web_fetch` (PR #65)

An allowlist-based guard (`internal/tool/fetch_guard.go`, landing in PR #65) blocks requests to private and reserved address space before the connection is opened:

- Scheme allowlist: only `http` and `https` are accepted.
- IP blocklist: loopback (127.0.0.0/8), link-local (169.254.0.0/16, fe80::/10), private RFC-1918 ranges, the AWS/GCP IMDS address (169.254.169.254), and CGNAT (100.64.0.0/10).
- Redirect re-validation: each redirect destination is re-checked against the same rules before following.

**Residual limitation — DNS rebinding (TOCTOU):** the guard resolves the hostname, validates the resulting IP, then the Go dialer resolves again on `Dial`. A server that flips its DNS record between the two resolutions (TTL=0) has a narrow window to redirect the connection to a private address. A complete fix requires a custom `DialContext` that validates the IP at the moment the socket is opened, not before it.

### WhatsApp webhook HMAC (PR #66)

Inbound POST requests to the WhatsApp webhook are verified against the `X-Hub-Signature-256` header using HMAC-SHA256 over the raw request body and the configured `app_secret`. Requests with a missing or invalid signature are rejected before the payload is parsed.

**Residual limitations:**

1. **Replay attacks** — Meta's signature covers only the body; it carries no nonce or timestamp. A captured valid request body can be replayed indefinitely. Full mitigation requires app-level deduplication keyed on `message.ID`.
2. **Non-constant-time verify-token comparison** — the one-time GET handshake that registers the webhook compares `hub.verify_token` with a plain `==` (`internal/channel/whatsapp.go`). This is negligible in practice (single-use during setup, attacker learns nothing useful from timing), but a strict fix would use `subtle.ConstantTimeCompare`.

---

## Known limitations

### 1. MCP servers run with full daimon-process capabilities

**Risk:** A malicious or compromised MCP server subprocess inherits the full environment of the daimon process — file system access, network access, environment variables including any secrets passed via `env:` in the config. There is no seccomp filter, no Linux capability drop, no namespace isolation (`unshare`), and no `Credential`/`setuid` applied to the subprocess (`internal/mcp/manager.go`).

**Why it's acceptable for single-user self-hosted:** You configure which MCP servers run. The threat is only relevant if you install an untrusted server — the same risk as installing any binary on your machine.

**Workaround / mitigation:** Only install MCP servers from sources you trust. Review the `command` and `env` fields in your config before adding a new server. A sandbox layer (seccomp profile, rootless container, dedicated low-privilege user) can be applied at the OS level around the daimon process itself. Tracked as DAIM-4.

---

### 2. Chat history, memory, and media are stored as plaintext in SQLite

**Risk:** The SQLite database (`daimon.db`) stores all conversation messages, agent memory entries, and media blobs as unencrypted plaintext (`internal/store/migration.go`; tables `conversations`, `memory`, `media_blobs`). Anyone who can read the database file — via a backup, a misconfigured file share, or physical access — can read your full conversation history.

**Why it's acceptable for single-user self-hosted:** The database lives on your own machine under your user account. File-system permissions are the primary control. Only the `secrets` table (API keys, tokens stored via `SetSecret`) is encrypted with AES-256-GCM (`internal/store/crypto.go`, `internal/store/sqlitestore.go`).

**Workaround / mitigation:** Use full-disk encryption (LUKS, FileVault, BitLocker) on the host. Restrict file permissions on the data directory. At-rest encryption for chat/memory/media is tracked as DAIM-10.

---

### 3. No multi-process lock on the database file

**Risk:** SQLite operates in WAL mode with a 5-second `busy_timeout` (`internal/store/sqlitestore.go`). There is no PID file, no `flock` on `daimon.db`, and no guard preventing two daimon instances from opening the same database file simultaneously. Running two instances against the same file can produce write conflicts, corrupted in-flight agent state, or duplicate cron triggers.

**Why it's acceptable for single-user self-hosted:** Normal usage is one instance per machine. The WAL mode and busy_timeout handle the transient concurrent-write case (e.g., the async indexer racing the agent loop) that occurs within a single process.

**Workaround / mitigation:** Run one daimon instance per database file. If you need a second instance (e.g., a test environment), point it at a separate `store.path`. An explicit single-instance guard is tracked as DAIM-9.

---

### 4. Auth token is long-lived with no per-session revocation

**Risk:** The web UI authentication token has a 30-day TTL (`authCookieMaxAge = 30 * 24 * 60 * 60` in `internal/web/auth_cookie.go`). There is a single global token; logging out rotates it (invalidating all existing cookies), but there is no per-session or per-device token, no IP binding, and no second-factor. A stolen token is valid until it expires or logout is called.

**Why it's acceptable for single-user self-hosted:** Daimon is designed to be accessed from your own devices on a trusted network or via a VPN/Tailscale tunnel. The token is stored in an `HttpOnly` cookie and compared with `subtle.ConstantTimeCompare` to prevent timing attacks.

**Workaround / mitigation:** Run daimon behind a network boundary (Tailscale, WireGuard, SSH tunnel) rather than exposing it directly to the internet. Use TLS via a reverse proxy so the auth cookie is protected in transit. A more granular session model is tracked as DAIM-11.

---

## Reporting a vulnerability

Please use **GitHub's private security advisory** system as the primary channel: go to the **Security** tab of this repository and click **"Report a vulnerability"**. This keeps the report confidential until a fix is available.

<!-- maintainer: add a security contact email here if you want one, e.g. security@yourdomain -->
You can also reach the maintainer via the email on their GitHub profile.

Please include:
- A description of the vulnerability and its potential impact
- Steps to reproduce or a proof-of-concept
- The version or commit you tested against

We aim to acknowledge reports within 72 hours and to publish a fix or mitigation with a public advisory once it is ready.
