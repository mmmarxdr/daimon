# Explore — conversations-liminal-resume

**Status**: ok
**Date**: 2026-04-24
**Change**: conversations-liminal-resume
**Predecessor exploration**: engram memories #1164 (frontend map), #1166 (backend lifecycle), #1167 v2 (plan). This doc VALIDATES and EXTENDS them against current HEAD.

## Executive summary

- V2 plan holds — zero drift in the relevant files since 2026-04-22. Only backend change on those paths is today's ping-goroutine fix (`452e841`), which doesn't touch the handshake.
- Schema is at version **13**; our soft delete column lands as migration **v14**.
- **convID-as-identity is cleaner than previously scoped**: `userScope(channelID, senderID)` at `internal/agent/loop.go:34-39` already treats `senderID` as optional. Resume can inject a precomputed `ConversationID` into `channel.IncomingMessage` and bypass `userScope` entirely. Backward-compatible.
- **Mid-turn abort (user closes browser while LLM streams)** → `SaveConversation` is called ONLY at turn end (`loop.go:682`). If the agent's context is not cancelled when the WS disconnects, the turn completes in the background and IS persisted — Resume would show the full answer. If ctx IS cancelled, everything since turn start is lost. **Needs targeted verification in spec phase** — changes the UX story for the "continuar" button.
- **Pre-existing bug exposed by Resume**: frontend `useWebSocket` auto-reconnects with exp backoff (`hooks/useWebSocket.ts:49-53`). Today every reconnection produces a NEW `channelID = "web:" + uuid[:8]` on the backend, orphaning the prior conversation. Giving the client stable identity via `conversation_id` **also fixes this silent bug**. Worth calling out in the proposal.
- **Liminal primitive reuse is narrower than the plan implied**. `MemoryCard` (`src/components/liminal/memory/MemoryCard.tsx`) types its `mem` prop to a memory shape, and `ClusterHeader` types `cluster` to a memory-domain enum (`identity` / `preferences` / `projects` / …). They cannot be reused as-is for conversations. Two options: refactor them to accept generic props (risky, scope creep) or build parallel `ConversationCard` + `TimeClusterHeader` that match the visual language but own their own types. **Recommend: parallel components**.

## Codebase anchors (concrete)

### Backend

| Path | Line(s) | What it is | Touched by |
|---|---|---|---|
| `internal/agent/loop.go` | 34-39 | `userScope(channelID, senderID)` — senderID is already optional | Phase 3 (override with ConversationID) |
| `internal/agent/loop.go` | 99-111 | `processMessage` — convID computation + LoadConversation | Phase 3, Phase 5 (title hook around here) |
| `internal/agent/loop.go` | 119, 388, 436, 624 | Append points into `conv.Messages` (in-memory) | Phase 5 (title trigger after N turns) |
| `internal/agent/loop.go` | 681-682 | Only `SaveConversation` call — runs at turn end | Phase 3 (verify ctx cancellation behavior) |
| `internal/channel/channel.go` | — | `IncomingMessage` struct — needs optional `ConversationID string` field | Phase 3 |
| `internal/channel/web.go` | 179-222 | `HandleWebSocket` — upgrade accepts no query params today | Phase 3 (read `?conversation_id=` from URL) |
| `internal/channel/web.go` | 199 | `connID := "web:" + uuid.New().String()[:8]` — always fresh | Phase 3 (reuse provided convID's channelID if present) |
| `internal/channel/web.go` | 253-258 | `continue_turn` handling — pattern for Resume's "continuar" button | Phase 3 |
| `internal/store/store.go` | 18-25 | `Conversation` struct | Phase 4 (add `DeletedAt *time.Time`?) |
| `internal/store/migration.go` | — | Schema version 13; next migration = v14 | Phase 4 |
| `internal/store/sqlite.go` | — | `SaveConversation`, `LoadConversation`, `ListConversationsPaginated`, `DeleteConversation` | Phase 4 (soft delete rewrite) |
| `internal/web/handler_conversations.go` | 44-80 | `handleListConversations` paginated | Phase 1-4 (filter deleted, return title metadata) |
| `internal/web/handler_conversations.go` | 107-120 | `handleDeleteConversation` (hard delete) | Phase 4 (→ soft delete) |
| `internal/web/server.go` | routes | Need new: `PATCH /api/conversations/{id}` (title), `POST /api/conversations/{id}/restore` | Phase 2, Phase 4 |
| `internal/web/server.go` | routes | Need new: `GET /api/conversations/{id}/messages?before={id}&limit=N` for pagination | Phase 2, Phase 3 |
| (new) `internal/store/pruner.go` | — | Background goroutine: DELETE WHERE deleted_at < now-retention | Phase 4 |
| (new) `internal/agent/titler.go` | — | Post-turn async LLM title job | Phase 5 |

### Frontend

| Path | LOC | What it is | Touched by |
|---|---|---|---|
| `src/pages/ConversationsPage.tsx` | 199 | Current admin-style list | Phase 1 (full Liminal rewrite) |
| `src/pages/ConversationDetailPage.tsx` | 236 | Already readonly, not yet Liminal | Phase 2 (Liminal rewrite + Resume button + msg pagination) |
| `src/pages/ChatPage.tsx` | 934 | Live chat; 934 LOC is the soft ceiling | Phase 3 (read `?resume=`, load history paginated — EXTRACT to `useResumeSession` hook, don't inline) |
| `src/pages/MemoryPage.tsx` | 231 | Liminal reference | Read-only model |
| `src/hooks/useWebSocket.ts` | 80 | WS hook; `path` is hardcoded, no query support | Phase 3 (accept query params or search-params arg) |
| `src/components/liminal/memory/` | — | Memory-scoped primitives (MemoryCard, ClusterHeader, etc) — NOT directly reusable | Phase 1-2 (build parallel components) |
| `src/components/liminal/` | — | Generic primitives (LiminalThread, LiminalUserMsg, LiminalAssistantMsg, LiminalPill, LiminalGlyph) — reusable | Phase 2 |
| (new) `src/components/liminal/conversations/ConversationCard.tsx` | — | Parallel to MemoryCard, matching visual language | Phase 1 |
| (new) `src/components/liminal/conversations/TimeClusterHeader.tsx` | — | Parallel to ClusterHeader, relative-time labels | Phase 1 |
| (new) `src/components/liminal/conversations/ConversationsPreamble.tsx` | — | Liminal preamble for the page header | Phase 1 |
| (new) `src/hooks/useResumeSession.ts` | — | Extracted logic for ChatPage resume flow | Phase 3 |

## Open technical question (partially answered)

> When a WS connection dies mid-turn, does the in-progress assistant message persist?

**Partial answer from code read**: `SaveConversation` runs once at `loop.go:682`, at the end of `processMessage`. Everything between turn start and save is buffered in `conv.Messages` (in-memory). If the agent's `ctx` is cancelled on WS disconnect, the goroutine likely exits early via one of the append points or provider stream errors, and the save is skipped — turn's work is lost.

If the agent's `ctx` is derived from channel/server startup (not per-connection), the agent keeps running even after the client disconnects, eventually hitting line 682 and saving. In that case Resume WOULD show the complete answer (best-case).

**What to verify in `/sdd-spec`** (it's a 15-min code trace, but the answer shapes the UX story):
1. In `internal/channel/web.go`, what `ctx` is passed down to the inbox dispatch? Is it `r.Context()` from the WS handler (which dies on disconnect), or a longer-lived `ctx` from `Start()`?
2. What does the provider stream do on `WriteMessage` error — continue buffering to `conv.Messages` internally, or bail?

Two possible UX branches for Resume:
- **A)** Turns survive disconnects → Resume always shows a clean conv, no "continuar" button needed.
- **B)** Turns die on disconnect → Resume may show "last msg = user", need "continuar" button that sends `continue_turn`.

Design for **B (pessimistic)** regardless, because it's the superset — the button is hidden if the last message is already from the assistant.

## Risks by phase

### Phase 1 — ConversationsPage Liminal
- **Risk**: Liminal primitive reuse is narrower than plan assumed (MemoryCard/ClusterHeader typed to memory domain). Mitigated by building `ConversationCard` + `TimeClusterHeader` parallel to them. Minor scope bump; no refactor of MemoryPage.
- **Risk**: Time-bucket edge cases (conv spans "yesterday" into "today" as I read this → needs stable pivot tied to fetch time, not render time). Low complexity but easy to get wrong.
- **Risk**: Empty state (0 conversations) — plan doesn't specify. Suggest a Liminal preamble-style "nothing here yet, start a chat and it'll show up".

### Phase 2 — ConversationDetailPage Liminal + Resume button
- **Risk**: Message pagination endpoint doesn't exist yet (`GET /api/conversations/{id}/messages` with cursor). New backend work.
- **Risk**: Export JSON/MD today exports the full conv (from the already-loaded store). Pagination + partial load changes this — either keep "export fetches all" as a separate code path, or paginate export too. Recommend former.
- **Risk**: Rename title is a new endpoint (PATCH). Need validation (max length, strip markdown, etc).

### Phase 3 — Resume end-to-end (convID as identity)
- **Risk (highest)**: `processMessage` invariants. `userScope` is used to scope memory search and RAG scope in many places — verify that providing convID directly still computes the same memory scope as if we'd computed it from channel+sender. In practice: convID = `"conv_" + scope`, so scope = convID without `"conv_"` prefix, which means scope reconstruction is trivial. Formalize in design.
- **Risk**: Frontend `useWebSocket` doesn't support query params today. API tweak required; auto-reconnect must preserve the `?conversation_id=` param so reconnections don't orphan mid-conv. This is itself the fix for the pre-existing silent bug.
- **Risk**: Identity spoofing. Today there's no auth requiring a user prove they own a convID. Anyone hitting `/ws/chat?conversation_id=conv_web:abc:user1` can resume someone else's conv. **Acceptable for single-user local daimon, but if the auth layer is ever enabled (there's an `auth.go` in web/), gate by session**. Flag for design, don't block on it.

### Phase 4 — Soft delete + pruner
- **Risk**: Migration v14 on existing DBs. Schema migrations already go through a versioned pattern (evidence: `migration_v10_test.go` asserts `schema_version=13`). Add `deleted_at TIMESTAMP NULL` and create index on `(deleted_at)`. All existing queries that read conversations must filter `WHERE deleted_at IS NULL`. Audit:  `LoadConversation`, `ListConversationsPaginated`, any FTS if added later.
- **Risk**: Pruner retention policy. 30 days default, configurable. If too short, users lose convs they wanted back. If too long, DB bloats. Recommend 30 days with config override, document prominently.
- **Risk**: Pruner goroutine lifecycle. Needs graceful shutdown alongside server Stop(). Same pattern as the channel managers.

### Phase 5 — LLM title async
- **Risk**: Provider cost visibility. Each title generation = ~1 provider call. Users may not notice until they see the bill. Config flag defaulted to true, docs note the cost.
- **Risk**: Model selection. Plan says "haiku equivalente". Each provider has a different "cheap" tier. Easiest: reuse the same model as the main conversation for v1. Iterate to explicit "haiku-like" in a follow-up.
- **Risk**: Trigger fires once at turn 3, then skipped. What if the first 3 turns are "hola"/"ok"/"gracias"? Title is useless. Recommend: trigger post-turn-3 AND on first real user-msg >20 runes, whichever comes later.

## Library / API verifications

No blockers on external APIs for this scope. Relevant surfaces:
- `gorilla/websocket` `Upgrader.Upgrade(w, r, nil)` — URL query params from `r` are available to the handler BEFORE upgrade via `r.URL.Query().Get(...)`. No library change, just read earlier in `HandleWebSocket`.
- `@tanstack/react-query` v5 `useInfiniteQuery` for message pagination — idiomatic and supports cursor-based. No tricks needed.
- `modernc.org/sqlite` — full SQL support for `ALTER TABLE` + partial indexes on `deleted_at IS NULL`.

Context7 not consulted inline (prior sessions validated these libs for this codebase). If spec phase needs precise API for anything, defer the query there.

## Phase ordering recommendation

```
┌──────────────────────┐
│ Phase 4 (soft delete)│──► must land first — downstream endpoints
└──────────────────────┘     depend on it
            │
            ▼
┌──────────────────────┐     ┌──────────────────────┐
│ Phase 1 (Convs page) │     │ Phase 3 (Resume WS)  │──► can develop in parallel
│                      │     │                      │     with Phase 1 & 2 — WS path
│ depends only on the  │     │ depends on ConvCard  │     is independent of list UI
│ new list endpoint    │     │ handoff → Chat       │
└──────────────────────┘     └──────────────────────┘
            │                           │
            └───────────┬───────────────┘
                        ▼
            ┌──────────────────────┐
            │ Phase 2 (Detail page)│──► depends on Resume existing (button target)
            │                      │     + message pagination endpoint
            └──────────────────────┘
                        │
                        ▼
            ┌──────────────────────┐
            │ Phase 5 (LLM title)  │──► last, independent feature, opt-in via config
            └──────────────────────┘
```

**Suggested execution order**: 4 → (1 ∥ 3) → 2 → 5. Strict TDD applies throughout.

## Artifacts

- File: `openspec/changes/conversations-liminal-resume/explore.md` (this doc)
- Engram topic key: `sdd/conversations-liminal-resume/explore`
- Predecessors (context): memory #1164, #1166, #1167 v2

## Next recommended

**Proceed to `/sdd-propose`**. Scope is validated, anchors mapped, risks surfaced. The mid-turn-abort question remains open but shouldn't block proposal — it shapes UX detail, not structure. Design for the pessimistic branch (B above).

## Skill resolution

`injected` — received compact rules for `context-mode`, `go-testing`, `golang-patterns`, `context7-mcp` in the launch prompt. Cached and applied.
