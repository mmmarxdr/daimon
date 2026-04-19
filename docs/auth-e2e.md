# Auth E2E Manual Test Checklist

Manual acceptance checklist for the `auth-httponly-complete` SDD.
Run this against a local deployment before releasing or after any auth-related change.

> **Environment**: local `microagent web` behind nginx (for proxy tests) OR plain HTTP for same-origin tests.

---

## 1. Fresh Setup Wizard → Cookie Set (AS-1)

- [ ] Start the agent with no existing config (or `auth_token: ""` in config).
- [ ] Navigate to `http://127.0.0.1:8080` in the browser.
- [ ] Complete the setup wizard (enter provider key, pick model).
- [ ] On completion, open DevTools → Application → Cookies.
- [ ] Verify cookie named `auth` is present with flags: `HttpOnly`, `SameSite=Strict`, `Path=/`, `Max-Age=2592000`.
- [ ] Verify the cookie value is NOT visible in the JavaScript console (`document.cookie` must not contain `auth=`).
- [ ] Verify `GET /api/status` with only the cookie (no Authorization header) returns 200.

---

## 2. Returning User Login → 204 + Cookie (AS-2)

- [ ] Clear the `auth` cookie (DevTools → Application → Cookies → Delete).
- [ ] Reload the page — the AuthGate form should appear.
- [ ] Enter the correct token from the config file and submit.
- [ ] Verify the response is `POST /api/auth/login 204` in DevTools → Network.
- [ ] Verify the `auth` cookie reappears with the correct flags.
- [ ] Verify the dashboard renders (AuthGate disappears, app is shown).
- [ ] Verify `localStorage` has no entry for any auth token (`Object.keys(localStorage)` in console).

---

## 3. Wrong Token → 401, No Cookie Set (AS-19)

- [ ] Clear the `auth` cookie.
- [ ] Submit the AuthGate form with an intentionally wrong token.
- [ ] Verify the response is `POST /api/auth/login 401`.
- [ ] Verify no `Set-Cookie` header is present in the 401 response.
- [ ] Verify the error message is shown in the AuthGate form.

---

## 4. Logout → Cookie Cleared + Token Rotated (AS-3)

- [ ] While authenticated, click the Logout button in the sidebar.
- [ ] Verify `POST /api/auth/logout 204` in DevTools → Network.
- [ ] Verify the response includes `Set-Cookie: auth=; Max-Age=0`.
- [ ] Verify the `auth` cookie is gone from DevTools → Application → Cookies.
- [ ] Verify the AuthGate form reappears.
- [ ] Inspect the config file on disk — `auth_token` and `auth_token_issued_at` should both be updated (new values).

---

## 5. Stale Cookie After Rotation → 401 (AS-4, AS-16)

- [ ] In tab A, copy the current `auth` cookie value from DevTools.
- [ ] Log out in tab A (cookie is now stale).
- [ ] In a new tab (or via `curl`), manually set a request cookie to the OLD value and call `GET /api/status`.
- [ ] Verify the response is 401.
- [ ] Verify the 401 response includes `Set-Cookie: auth=; Max-Age=0`.

---

## 6. Multi-Tab Logout via BroadcastChannel (AS-13)

- [ ] Open the dashboard in two tabs (tab A and tab B), both authenticated.
- [ ] In tab A, click Logout.
- [ ] Verify tab B renders the AuthGate form WITHOUT making a network request to `/api/status` (check Network tab — no request should fire).
- [ ] Verify this happens within one animation frame (near-instant).

---

## 7. Nginx TrustProxy=true → Secure Flag (AS-7)

- [ ] Configure nginx to proxy to `http://127.0.0.1:8080` and terminate TLS.
- [ ] Set `web.trust_proxy: true` in config.
- [ ] Log in via the browser (connected to nginx over HTTPS).
- [ ] Verify the `auth` cookie includes the `Secure` flag in DevTools.

---

## 8. CLI Bearer → Still Authenticates (AS-14)

- [ ] Retrieve the current `auth_token` from the config file.
- [ ] Run: `curl -H "Authorization: Bearer <token>" http://127.0.0.1:8080/api/status`
- [ ] Verify the response is 200 (Bearer header still works for non-browser clients).

---

## 9. WS Cookie-Only Upgrade → Succeeds (AS-10)

- [ ] While authenticated (cookie set), open a new browser tab and navigate to the dashboard.
- [ ] In DevTools → Network, filter for `ws/` connections.
- [ ] Verify the WS handshake URL does NOT contain `?token=`.
- [ ] Verify the WS connection status is 101 (Switching Protocols).
- [ ] Verify real-time chat/metrics work normally.

---

## 10. WS Reconnect After Network Drop → Cookie Only (FR-41)

- [ ] While the dashboard is open and WebSocket is connected, disable Wi-Fi for 5 seconds then re-enable.
- [ ] Verify the WS reconnects successfully.
- [ ] Verify the reconnect URL in DevTools does NOT contain `?token=`.

---

## 11. Cross-Origin Mode — SameSite=None; Secure (AS-5)

- [ ] Set `allowed_origins: ["https://app.example.com"]` in config.
- [ ] Set `trust_proxy: true` and configure nginx with TLS.
- [ ] Log in from the frontend origin.
- [ ] Verify the `auth` cookie has `SameSite=None; Secure`.
- [ ] Verify a CORS preflight from `https://app.example.com` returns `Access-Control-Allow-Origin: https://app.example.com` and `Access-Control-Allow-Credentials: true`.
- [ ] Verify a CORS preflight from `https://evil.example.com` does NOT return `Access-Control-Allow-Credentials: true`.

---

## 12. Session TTL Expiry → 401 + Clear Cookie (AS-22)

> This test requires temporarily setting `auth_token_issued_at` to a date 31+ days ago in the config file.

- [ ] Stop the server.
- [ ] Edit the config file: set `auth_token_issued_at` to a date 31 days ago (e.g. `2024-01-01T00:00:00Z`).
- [ ] Start the server.
- [ ] Make any authenticated request (e.g. `GET /api/status` with the cookie).
- [ ] Verify the response is 401.
- [ ] Verify the 401 response includes `Set-Cookie: auth=; Max-Age=0`.
- [ ] Verify the `auth_token` on disk has NOT changed (TTL expiry does not rotate the token).

---

## Sign-off

All items above passed on: _______________________ (date)

Tested by: _______________________

Environment: _______________________
