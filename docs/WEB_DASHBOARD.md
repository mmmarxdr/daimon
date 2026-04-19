# Web Dashboard

The web dashboard gives you a browser-based UI with real-time chat, metrics,
conversation history, and config management.

## Start the dashboard

```bash
# Standalone (web is the only interface)
microagent web

# Alongside CLI/Telegram (both work simultaneously)
microagent --web
```

On startup, the agent generates an auth token (if none is configured) and
prints it to the console:

```
INFO web dashboard available url=http://127.0.0.1:8080 auth_token=a1b2c3d4...
```

Open the URL in your browser, enter the token, and you are in.

## Auth model

The dashboard uses **HttpOnly cookies** for browser authentication. The token
is never stored in `localStorage` or exposed to JavaScript — it lives
exclusively in the browser's cookie jar and on the server's config file.

### Session lifetime

Sessions expire **30 days** after the token was last issued (setup or logout).
This is enforced server-side: even if the browser cookie is still present, the
server rejects it after the TTL elapses. Logging out rotates the token and
resets the 30-day clock.

### Auth token options

The dashboard is **always authenticated**. Three ways to set the token:

| Method                | Example                                                      |
| --------------------- | ------------------------------------------------------------ |
| Config file           | `web.auth_token: "my-secret-token"`                          |
| Environment variable  | `MICROAGENT_WEB_TOKEN=my-secret-token microagent web`        |
| Auto-generated        | Leave empty — token is printed on startup                    |

For production or VPS deployments, set a fixed token in config or env so
it persists across restarts. See [docs/DEPLOY.md](DEPLOY.md).

### Login / logout endpoints

The browser dashboard uses two dedicated auth endpoints:

| Endpoint              | Method | Description                                    |
| --------------------- | ------ | ---------------------------------------------- |
| `/api/auth/login`     | POST   | Submit `{"token":"<value>"}` — sets HttpOnly cookie on success (204) |
| `/api/auth/logout`    | POST   | Rotates the token, clears the cookie (204)     |

These endpoints are used by the web dashboard only. CLI and script clients
continue to use `Authorization: Bearer <token>` or `?token=<token>` on
WebSocket connections.

## Config

```yaml
web:
  enabled: true
  port: 8080
  host: "127.0.0.1"    # 0.0.0.0 to expose to network
  auth_token: ""        # auto-generated if empty; also: MICROAGENT_WEB_TOKEN env var

  # auth_token_issued_at is managed automatically — do not edit by hand.
  # Records when the current auth token was last issued. TTL is 30 days.

  # Reverse proxy (nginx, Caddy, etc.) — set true when TLS is terminated by the proxy.
  # Causes the auth cookie to include the Secure flag when X-Forwarded-Proto: https.
  # trust_proxy: false

  # Cross-origin deployment — list every frontend origin (scheme + host + port).
  # Leave empty (default) for same-origin mode (frontend and backend on the same host).
  # allowed_origins: []
  #   - "https://app.example.com"
```

### Same-origin vs cross-origin mode

| Setting | Cookie SameSite | CORS | WS CheckOrigin |
| ------- | --------------- | ---- | -------------- |
| `allowed_origins: []` (default) | `Strict` | no credentials header | same-origin only |
| `allowed_origins: [...]` | `None; Secure` | listed origins + `Allow-Credentials: true` | listed origins only |

Wildcard `*` is not supported in `allowed_origins` — it is incompatible with
`Allow-Credentials: true` and is silently ignored.

## API endpoints

Browser requests use the HttpOnly cookie automatically (no manual header
required). Non-browser clients (CLI, scripts) use
`Authorization: Bearer <token>`. WebSocket connections from non-browser
clients may also pass `?token=<token>` as a query parameter.

| Endpoint                        | Method     | Description                        |
| ------------------------------- | ---------- | ---------------------------------- |
| `/api/status`                   | GET        | Agent status, uptime, version      |
| `/api/auth/login`               | POST       | Authenticate and set cookie        |
| `/api/auth/logout`              | POST       | Rotate token and clear cookie      |
| `/api/config`                   | GET/PUT    | Active config (secrets masked)     |
| `/api/conversations`            | GET        | List conversations                 |
| `/api/conversations/{id}`       | GET/DELETE | Get or delete a conversation       |
| `/api/memory`                   | GET/POST   | List or create memory entries      |
| `/api/memory/{id}`              | DELETE     | Delete a memory entry              |
| `/api/metrics`                  | GET        | Token usage and cost metrics       |
| `/api/mcp/servers`              | GET        | MCP server status                  |
| `/ws/chat`                      | WS         | Real-time chat with the agent      |
| `/ws/metrics`                   | WS         | Live metrics stream                |
| `/ws/logs`                      | WS         | Live audit log stream              |
