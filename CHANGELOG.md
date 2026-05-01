# Changelog

All notable changes to Daimon are documented here.

Releases follow [semver](https://semver.org). Pre-1.0 minors may break configuration; patch releases never do.

---

## [v0.11.1] — New-chat escape hatch

**Release date**: 2026-05-01

Patch release closing a UX gap surfaced minutes after v0.11.0 shipped:
v0.11.0's mount-once `ChatPage` left users stuck in whichever
conversation they happened to be on. Clicking the sidebar's "Chat" link
from inside an active thread just kept appending — there was no way to
start fresh. Backend is unchanged in this release; everything ships
through the embedded frontend bundle (daimon-frontend v0.11.1).

### New

- **`+ new chat`** button at the top of the sidebar, styled to match
  the existing `summon…` action (italic serif label, line border,
  `bg-elev` fill, `+` glyph in accent color). On click: ChatPage
  remounts via a key bump → state resets, WebSocket reconnects without
  a `conversation_id`, backend assigns a fresh server-side conversation,
  and the URL is replaced with `/chat` (dropping any
  `?conversation_id` / `?prompt`).

### Notes on the architecture

This is an **intentional** remount triggered by a key change, distinct
from the **accidental** remount that v0.11.0 fixed (PR #5: conditional
Outlet shrinking the children list on `/chat ⇄ dock` route flips). Both
coexist cleanly — accidental remounts are still prevented by the
unconditional Outlet; intentional ones are now possible via the
`chatSessionKey` counter in AppLayout.

---

## [v0.11.0] — Chat dock v2: chat-from-dock, drag, resize

**Release date**: 2026-05-01

The floating chat dock stops being a passive preview and becomes a real
work surface. Users can keep typing while moving between Memory, Logs,
Settings, etc. without losing the in-flight turn or going back to the
fullscreen view. Backend is unchanged in this release — all features
ship via the embedded frontend bundle (daimon-frontend v0.11.0).

### New

- **Chat from inside the dock.** The dock now has its own textarea and
  send button. Type, hit Enter (Shift+Enter for newline), and the message
  flows on the same WebSocket and conversation as the fullscreen view —
  there is exactly one ChatPage instance, the dock and fullscreen are two
  views of the same component. Send is disabled while a turn is in
  flight or the WS is disconnected.
- **Drag the dock.** Grab the header and move the dock anywhere; on
  release it snaps to the nearest viewport corner (tl/tr/bl/br). Position
  persists across reloads and tabs.
- **Resize from the corner grip.** A diagonal grip lives at the corner
  opposite the anchor; drag it to grow or shrink the dock within
  320×240–720×720 bounds. Dimensions persist.
- **Sidebar context-usage percent.** The footer ctx row now shows
  `ctx XX%   [bar]   X.Xk` — the percent makes the bar legible without
  having to know the model's context window by heart.
- **Dock as a real chat surface.** Messages render with full multi-line
  wrap (`whitespace-pre-wrap`), no truncation. The list scrolls with new
  tokens when you're near the bottom and stays put if you scrolled up to
  read history. Selecting text inside the dock no longer triggers
  expand-to-fullscreen on mouseup.
- **Thinking indicator.** When `isWaiting` is true and the agent has not
  yet emitted a streaming assistant message (LLM first-token latency,
  inter-iteration tool-use windows), a `daimon …` row pulses so the dock
  doesn't look frozen during the pre-stream phase.

### Fixed

- **Chat-dock route remount.** v0.10.1's chat-dock relied on ChatPage
  staying mounted across `/chat` ⇄ dock transitions, but a conditional
  Outlet in AppLayout shrank the children list every time the route
  flipped, and React's positional reconciler unmounted ChatPage. Symptom:
  sending a message after returning from the dock landed in a fresh
  server-side conversation with empty history. AppLayout now renders
  Outlet unconditionally (the /chat route is `element={null}`, so it
  remains a no-op visually); ChatPage truly stays mounted.
- **Chat-dock z-index covered by Toast.** Toast was at `z-50` and the
  dock at `zIndex:40` in the same corner — the first toast hid the dock.
  Toast bumped to `z-[60]`, dock raised to `zIndex:50`.
- **Streaming-token re-render storm.** The dock used to re-render the
  message list on every WebSocket token because `messages.slice(-3)` ran
  inside ChatDockView, defeating `React.memo`. The slice is now lifted
  to ChatPage's `useMemo([messages])` and ChatDockView is wrapped in
  `React.memo` — the dock skips re-renders when its visible tail hasn't
  changed.
- **Nested-button a11y.** The card used to be a `role="button"` div with
  a real `<button>` (close X) nested inside — invalid HTML, screen
  readers announced "button, button". Replaced with a single transparent
  `<button>` covering the card as the primary expand target plus a
  sibling close button positioned absolutely above via z-index.

### Performance discipline (drag/resize)

The drag/resize handlers do **zero** React state updates per
`pointermove`. The dock element's `style.transform` (drag) or
`style.{width,height}` (resize) is mutated directly via a ref;
`will-change` is set during the gesture and cleared on release; pointer
listeners live on `document` so a drag past the dock's bounds doesn't
lose the gesture; `user-select: none` is applied to `<body>` for the
gesture's lifetime so text selection doesn't fight the drag. React
re-renders **once** on `pointerup` when geometry is committed to
localStorage and the `useSyncExternalStore` subscriber fires.

---

## [v0.10.1] — Frontend catch-up + audit/config race fixes

**Release date**: 2026-05-01

A patch release with two motivations:

1. **The v0.10.0 release shipped the backend endpoints documented below
   (system pulse, audit hot-swap, sidebar telemetry, audit-from-Settings)
   but the embedded frontend bundle was still v0.8.0 and could not
   consume them.** Users running `daimon update` saw a stale UI. v0.10.1
   ships frontend v0.9.0 so the v0.10.0 UI is finally visible.
2. Two CRITICAL data races surfaced by an internal review on v0.10.0
   are closed.

### Fixed

- **Audit hot-swap data race.** The agent and `/ws/logs` held the
  previous auditor after `PUT /api/config` swapped the backend, so a
  `Close()` on the old one could fire concurrent with reads. The agent
  now resolves the auditor through an accessor callback (`auditorFn`)
  read under `auditorMu.RLock` on every `Emit`. WS logs re-resolves via
  `s.CurrentAuditor()` on each 2s poll tick.
- **Config snapshot torn-read.** `*s.deps.Config = merged` is a
  non-atomic struct assignment; readers without a lock could observe
  partially updated state. `configMu` is now `sync.RWMutex` and every
  reader takes a snapshot via `s.config()`. `handlePutConfig` releases
  the write lock before `rebuildAuditor` and `handleGetConfig` to avoid
  the non-reentrant RWMutex deadlock.
- **Embedded frontend bundle** updated to daimon-frontend v0.9.0. The
  v0.10.0 features that shipped backend-only — sidebar telemetry,
  system pulse panel, audit toggle in Settings, LogsPage/ToolsPage
  Liminal redesigns, empty states, sidebar version footer — are now
  reachable from the dashboard.

### New

- **Chat dock mini-player** (`daimon-frontend` PR #2 by
  `@mauroasoriano22`). When you navigate away from `/chat`, the chat
  compresses into a small dock anchored to the bottom-right corner.
  Click to expand back to fullscreen; X to dismiss. Dismissal persists
  in `localStorage` until you revisit `/chat`. The WebSocket and turn
  state stay alive across navigation, so a long-running turn is no
  longer killed when you peek at Memory or Settings.

---

## [v0.10.0] — Audit hot-swap, system pulse, full Liminal coverage

**Release date**: 2026-04-25

A polish release focused on UX completeness ahead of the 1.0 milestone:
the dashboard now covers every page in the Liminal aesthetic, surfaces
process and host telemetry, and audit logging works out of the box —
no hand-edited YAML required.

### New

- **System pulse on Metrics** — process RSS, host CPU/memory/disk, and a
  per-subsystem storage breakdown (store / audit / skills) with bar fills
  that turn amber and red at configurable thresholds. Refreshes every 5s.
- **Audit logging from Settings** — toggle audit on/off, pick the backend
  (sqlite for streaming, file for append-only), and choose a path. Changes
  hot-swap the running auditor without a restart; `/logs` picks up the new
  stream on the next connection.
- **LogsPage** redesigned in Liminal with level filters, pulsing connection
  dot, and a banner that explains *why* streaming is unavailable when audit
  is off or pointed at a non-streaming backend.
- **ToolsPage** redesigned in Liminal — last page on the migration list.
- **Empty states** — Conversations, Memory, and Knowledge greet first-run
  users with editorial copy and a clear CTA instead of blank screens.
- **Version visible** in the sidebar footer.

### Changed

- `audit.enabled` now defaults to `true` and `audit.type` defaults to
  `sqlite`. The setup wizard provisions sqlite. Existing configs with an
  explicit `enabled: false` are honored — the *bool migration preserves
  opt-out.
- `PUT /api/config` accepts the `audit` subtree (previously dropped).
  When any audit field is patched, the server hot-swaps the auditor.
- Conversations page strings translated to English (search placeholder,
  loading, error, no-match, pagination, delete confirm).

### Fixed

- `/ws/logs` no longer renders the "audit backend does not support
  RecentEvents" status frame as a regular log line — it carries
  `event_type=stream_unavailable` and surfaces as a banner with an
  actionable CTA.

### Internals

- `currentAuditor()` / `rebuildAuditor()` — read under RWMutex.RLock for
  wait-free WS handshakes; rebuild closes the old backend and atomically
  installs a new one.
- `audit.LogStreamer` interface remains the contract; only
  `SQLiteAuditor` implements it. FileAuditor and NoopAuditor degrade
  gracefully through the WS handler with distinct messages.
- `gopsutil/v3` added as a dependency for process and host metrics.

---

## [v0.9.0] — Conversations resume + PDF inlining

**Release date**: 2026-04-24

### New

- **Conversation resume** — closing the browser no longer ends the
  conversation. Re-opening the dashboard hydrates the last thread on
  `/chat` directly from the server, replacing the previous intermediate
  preview page.
- **Conversation lifecycle** — soft-delete with 30-day retention, a
  background pruner that hard-deletes after the window, an async title
  generator that names threads from their first user turn, and stable
  `conversation_id` propagated end-to-end (config, store, agent loop).
- **Conversations REST endpoints** — paginated list, message window, and
  delete; all auth-gated.

### Fixed

- **PDF and document attachments** — content is now extracted server-side
  and inlined into the user message instead of being silently dropped on
  providers that don't support native documents (the OpenAI shim used by
  OpenRouter, in particular).
- **RAG over-recall on attachment turns** — short user prompts that lean
  on a fresh attachment ("summarize this") used to trigger BM25 against
  unrelated docs; the loop now skips RAG when the message is
  attachment-dominant and the text is short.
- **WebSocket goroutine leak** — `WebChannel.HandleWebSocket`'s ping
  goroutine now exits cleanly when the handler returns.
- **Shell whitelist bypass (RCE)** — `cmd; rm -rf` no longer slips past
  the allow-list check; the executor splits whitelisted vs. raw paths.

---

## [v0.8.0] — RAG precision + self-updating CLI

**Release date**: 2026-04-22

### New

- **`daimon update`** and **`daimon version`** subcommands — the binary
  fetches matching release assets from GitHub, verifies, and atomically
  replaces itself at `os.Executable()`. Flags: `--check`, `--version`.
- **HyDE retrieval** — generates a hypothetical answer with a configurable
  model, embeds it alongside the query, and fuses scores via RRF.
  Configurable timeout, query weight, and candidate cap.
- **Neighbor expansion + score thresholds** — pulls adjacent chunks
  around top hits and discards low-similarity results so the final
  context window stays dense with relevant material.
- **RAG-wide metrics** — counter and timing histograms surfaced via
  `/api/metrics/rag`.
- **Pure-vector search mode** — `SearchOptions.SkipFTS` for clients that
  want only embedding similarity.

### Fixed

- Chunker tail-junk and triple-overlap edge cases.
- Two dead-config bugs surfaced during HyDE testing.
- `web` subcommand wires RAG so `/api/knowledge` works.

---

## [v0.7.0] — Knowledge base + curated memory

**Release date**: 2026-04-21

### New

- **Knowledge base** — endpoints, schema, extractors (PDF, DOCX,
  Markdown, plain text), and a batch ingestion worker. Documents are
  chunked, embedded, and made available to the agent through RAG search.
- **Embedding subsystem** — pluggable provider with a dedicated config
  block; supports batching and a configurable model. OpenAI and Gemini
  backends shipped.
- **Memory clusters** — observations now carry a cluster classification
  (certain / inferred / assumed) surfaced on MemoryPage.
- **Memorable-fact curator** — selects high-signal observations from the
  conversation history and persists them as memory entries.
- **Cross-scope memory access** for the admin user (project + personal).
- **Actionable tool timeout copy** — when a tool exceeds its budget the
  agent surfaces *why* and offers a retry hint instead of a stack trace.
- **Process-group kill on timeout** — child processes spawned by tools
  no longer leak when the wrapper times out.
- **Turn deadline** — total turn time is enforced in addition to
  per-tool timeouts.

---

## [v0.6.0] — Liminal redesign + budget loop + loop detection

**Release date**: 2026-04-20

### New

- **Liminal design system** — typography (serif display, mono data),
  CSS-variable palette, breathing glyph wordmark, and a new layout that
  replaced the previous flat dashboard. ChatPage migrated as the
  reference implementation.
- **Budget-based loop control** — the agent now tracks token spend per
  turn against a configurable budget instead of capping iterations
  blindly, allowing long-but-cheap turns and stopping expensive ones.
- **Loop detection** — recognizes when the same tool call repeats
  unproductively and breaks out with an explanation.
- **`search_output` tool by default** — the agent can grep its own
  recent tool outputs without re-running them.
- **Truncation byte counts** surfaced in tool output so the agent knows
  *how much* was cut, not just *that* it was cut. Shell limit raised
  10K → 64K, HTTP 512K → 2MB.
- **Gemini and Ollama reasoning streaming** — the providers now stream
  thinking tokens alongside content, matching Anthropic's behavior.

---

## [v0.5.0] — Dynamic model discovery + reasoning streaming

**Release date**: 2026-04-19

### New

- **Dynamic model discovery** — providers expose `ListModels()` so the
  dashboard can populate the model picker from the live catalog instead
  of hard-coded lists, and runtime pricing lookups stay correct as
  vendors release new models.
- **Reasoning token streaming** for providers that emit them
  (Anthropic native, OpenRouter compatible).

---

## [v0.4.0] — BREAKING: Product renamed microagent → daimon

**Release date**: 2026-04-19

This release completes the product rename from `microagent` to `daimon`.
It is a breaking change for all users. No backward compatibility is provided.

### Breaking changes

| What changed | Old | New |
|-------------|-----|-----|
| Binary name | `microagent` | `daimon` |
| Config directory | `~/.microagent/` | `~/.daimon/` |
| Database filename | `microagent.db` | `daimon.db` |
| Web token env var | `MICROAGENT_WEB_TOKEN` | `DAIMON_WEB_TOKEN` |
| Jina API key env var | `MICROAGENT_JINA_API_KEY` | `DAIMON_JINA_API_KEY` |
| Secret key env var | `MICROAGENT_SECRET_KEY` | `DAIMON_SECRET_KEY` |
| Go module path | `module microagent` | `module daimon` |
| GitHub repository | `github.com/mmmarxdr/micro-claw` | `github.com/mmmarxdr/daimon` |

### Migration steps (manual — no automatic migration)

1. **Move config directory:**
   ```bash
   mv ~/.microagent ~/.daimon
   ```

2. **Rename the database file:**
   ```bash
   mv ~/.daimon/data/microagent.db ~/.daimon/data/daimon.db
   ```

3. **Update environment variables** in your shell profile or secrets manager:
   - `MICROAGENT_WEB_TOKEN` → `DAIMON_WEB_TOKEN`
   - `MICROAGENT_JINA_API_KEY` → `DAIMON_JINA_API_KEY`
   - `MICROAGENT_SECRET_KEY` → `DAIMON_SECRET_KEY`

4. **Update any systemd service files** or scripts that reference the old
   binary name or env vars.

5. **Go consumers** (if you use `go install`): the module path is now
   `github.com/mmmarxdr/daimon/cmd/daimon`. Update your `go.mod` accordingly.

### What does NOT change

- Configuration file format — YAML structure is unchanged
- API endpoints — all REST and WebSocket routes are unchanged
- Cookie name (`auth`) — unchanged
- Data format — existing conversations and memory entries are compatible
  after the db rename above

---

*Older pre-0.4.0 entries are not documented here (pre-public-release history).*
