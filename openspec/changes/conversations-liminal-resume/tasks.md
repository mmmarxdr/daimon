# Tasks — conversations-liminal-resume

TDD-ordered checklist. Each task references the scenario(s) it validates from `specs/<capability>/spec.md`. Ordering respects the rollout graph in `design.md §5`.

**TDD rule for this change**: for every task touching code, write test first → run it → observe red → implement → run test → observe green. Mark the task complete only after green.

Legend:
- `[T]` test-writing task
- `[I]` implementation task
- `[V]` verification / manual check
- Deps listed as `← T-NN`.

---

## Group A — Foundations (must land first)

### A1. Config shape
- [ ] `[T] A1.1` — Unit test: `ApplyDefaults` populates `Conversations.Prune.{Enabled=true, RetentionDays=30, IntervalHours=6}` and `AI.TitleGeneration.{Enabled=true, WorkerCount=2, QueueSize=32, CallTimeoutMS=30000}` when blocks are absent.
- [ ] `[T] A1.2` — Unit test: `ApplyDefaults` clamps out-of-range `retention_days` (0 → 1, 5000 → 3650) and `interval_hours` (0 → 1, 999 → 168), emits `slog.Warn`.
- [ ] `[T] A1.3` — Unit test: `patchConversations` / `patchAI` preserve the field when set via config PUT (regression guard matching rag-hyde T16 bug class).
- [ ] `[I] A1.4` — Add `ConversationsConfig`, `PruneConfig`, `AIConfig`, `TitleGenYAMLConfig` structs to `internal/config/config.go`. Wire into top-level `Config`.
- [ ] `[I] A1.5` — Extend `ApplyDefaults` for the new blocks (+ clamps).
- [ ] `[I] A1.6` — Extend `patchConversations` / `patchAI` allow-list in `internal/web/handler_config.go`.
- [ ] `[V] A1.7` — Run A1.1–A1.3; all green.

### A2. `IncomingMessage.ConversationID` field
- [ ] `[T] A2.1` — Compile-time guard: every channel package builds with the new field as zero-value (no code change needed — just run `go build ./...`).
- [ ] `[I] A2.2` — Add `ConversationID string` to `channel.IncomingMessage` in `internal/channel/channel.go`. Update struct comment.
- [ ] `[V] A2.3` — `go build ./...` green; `go vet ./...` green.

### A3. Schema migration v14
- [ ] `[T] A3.1` — Test: starting from a v13 DB (seed via existing test helpers), run the store constructor, assert `schema_version = 14`, assert `deleted_at` column exists on `conversations`, assert partial index exists (query `sqlite_master`).
- [ ] `[T] A3.2` — Test: running the constructor a second time on the v14 DB is a no-op (idempotent), still `schema_version = 14`, no duplicate index errors.
- [ ] `[I] A3.3` — Implement `migrateV14` in `internal/store/migration.go` with the column-exists guard (sketch in `design.md §2.5`).
- [ ] `[I] A3.4` — Register v14 in the ordered migration chain.
- [ ] `[V] A3.5` — Both A3 tests green.

---

## Group B — Store changes (soft delete + pagination + title)

Dep: A3.

### B1. Soft delete + restore
- [ ] `[T] B1.1` — Test `DeleteConversation` sets `deleted_at` (validates `conversation-store/spec.md` scenario "Deleting a live conv marks it as soft-deleted").
- [ ] `[T] B1.2` — Test `DeleteConversation` on already-soft-deleted conv is no-op (scenario "Deleting an already-deleted conv is a no-op").
- [ ] `[T] B1.3` — Test `DeleteConversation` on nonexistent id returns `ErrNotFound`.
- [ ] `[T] B1.4` — Test `RestoreConversation` clears `deleted_at` on a soft-deleted conv (scenario "Restoring a soft-deleted conv").
- [ ] `[T] B1.5` — Test `RestoreConversation` on live conv returns `ErrNotFound` (scenario "Restoring a non-soft-deleted conv is an error").
- [ ] `[T] B1.6` — Test `RestoreConversation` on nonexistent id returns `ErrNotFound`.
- [ ] `[I] B1.7` — Rewrite `DeleteConversation` to soft-delete (design §2.6).
- [ ] `[I] B1.8` — Implement `RestoreConversation` on `store.Store` interface + sqlite impl.
- [ ] `[V] B1.9` — B1.1–B1.6 green.

### B2. Read-path soft-delete filter
- [ ] `[T] B2.1` — Test `LoadConversation` returns `ErrNotFound` for soft-deleted conv (scenario "LoadConversation skips soft-deleted convs").
- [ ] `[T] B2.2` — Test `ListConversationsPaginated` excludes soft-deleted from results + `total` (scenario "ListConversationsPaginated skips soft-deleted convs").
- [ ] `[I] B2.3` — Add `WHERE deleted_at IS NULL` to `LoadConversation` query.
- [ ] `[I] B2.4` — Add `WHERE deleted_at IS NULL` to `ListConversationsPaginated` both SELECT and COUNT.
- [ ] `[V] B2.5` — B2.1, B2.2 green.
- [ ] `[V] B2.6` — Grep audit: `grep -n "FROM conversations" internal/store/`; every match has the filter or a comment explaining why not (pruner's DELETE is the only intentional exception).

### B3. `DeleteConversationsOlderThan` (pruner helper)
- [ ] `[T] B3.1` — Test: with 3 convs at `deleted_at = T-31d` and 1 at `deleted_at = T-5d` (retention 30d → cutoff T-30d), `DeleteConversationsOlderThan(ctx, cutoff)` returns `3` and rows are GONE (SELECT COUNT returns only the remaining one).
- [ ] `[T] B3.2` — Test: live convs are never affected.
- [ ] `[I] B3.3` — Implement `DeleteConversationsOlderThan` on sqlite (design §2.9).
- [ ] `[V] B3.4` — B3.1, B3.2 green.

### B4. `GetConversationMessages` paginated
- [ ] `[T] B4.1` — Scenario "Initial load returns last 50 of 200": 200-msg conv, `GetConversationMessages(ctx, id, -1, 50)` returns indexes 150..199, `oldestIndex=150`, `hasMore=true`.
- [ ] `[T] B4.2` — Scenario "Paging upward": `beforeIndex=150, limit=50` → indexes 100..149, `oldestIndex=100`, `hasMore=true`.
- [ ] `[T] B4.3` — Scenario "Reaching the start": `beforeIndex=50, limit=100` → indexes 0..49, `oldestIndex=0`, `hasMore=false`.
- [ ] `[T] B4.4` — Scenario "Soft-deleted conv is not readable": soft-delete a conv, call `GetConversationMessages`, expect `ErrNotFound`.
- [ ] `[T] B4.5` — Test: `limit` clamping — limit=0 → 50, limit=500 → 200, limit=-3 → 50.
- [ ] `[T] B4.6` — Test: empty conv → empty slice, `hasMore=false`, `oldestIndex=0`, no error.
- [ ] `[T] B4.7` — Test: mutating returned slice does NOT affect subsequent reads (defensive copy).
- [ ] `[I] B4.8` — Implement per design §2.7 (load-and-slice).
- [ ] `[V] B4.9` — B4.1–B4.7 green.

### B5. `UpdateConversationTitle`
- [ ] `[T] B5.1` — Test: valid title persists in `metadata["title"]`, survives Load.
- [ ] `[T] B5.2` — Test: empty title → `ErrInvalidArgument` (new sentinel or `fmt.Errorf`).
- [ ] `[T] B5.3` — Test: title > 100 runes → `ErrInvalidArgument`.
- [ ] `[T] B5.4` — Test: title on soft-deleted conv → `ErrNotFound`.
- [ ] `[T] B5.5` — Test: title on nonexistent conv → `ErrNotFound`.
- [ ] `[T] B5.6` — Test: newline in title is replaced with space (scenario "Newline is stripped").
- [ ] `[I] B5.7` — Implement `UpdateConversationTitle(ctx, id, title)` using `json_set` pattern (design §2.11).
- [ ] `[V] B5.8` — B5.1–B5.6 green.

---

## Group C — Agent-loop changes

Dep: A2, B1–B5.

### C1. `processMessage` convID resolution
- [ ] `[T] C1.1` — Scenario "Explicit ConversationID is respected": craft `IncomingMessage{ConversationID: "conv_web:x:u1", ChannelID: "…", SenderID: "…"}`, assert `LoadConversation` sees `"conv_web:x:u1"` (use a mock store).
- [ ] `[T] C1.2` — Scenario "Missing ConversationID falls back to userScope": empty `ConversationID`, assert convID = `"conv_" + userScope(channelID, senderID)`.
- [ ] `[T] C1.3` — Scenario "ConversationID on a new conv creates it fresh": convID not in store, assert new Conversation struct created with that ID.
- [ ] `[I] C1.4` — Apply the resolution sketch from design §2.3 to `loop.go` around lines 99-111.
- [ ] `[V] C1.5` — C1.1–C1.3 green.

### C2. `shouldGenerateTitle` helper
- [ ] `[T] C2.1` — Pure-function test table: eligible conv (6 msgs, empty title, first user ≥20 runes) → true; <6 msgs → false; existing non-empty title → false; first user <20 runes → false; nil conv → false.
- [ ] `[I] C2.2` — Implement `shouldGenerateTitle` + `firstUserMessageText` helpers in `loop.go` (private).
- [ ] `[V] C2.3` — C2.1 green.

### C3. Title hook + `Agent.WithTitler`
- [ ] `[T] C3.1` — Scenario "Eligible conv triggers title generation": mock titler, run processMessage to completion with a qualifying conv, assert `titler.Enqueue(ctx, convID)` was called.
- [ ] `[T] C3.2` — Scenario "Short first message does not trigger": same setup but first user-msg "hola" → Enqueue not called.
- [ ] `[T] C3.3` — Scenario "Existing title is not overwritten": conv with `metadata["title"] = "..."` → Enqueue not called.
- [ ] `[T] C3.4` — Scenario "Title generation disabled via config": `cfg.AI.TitleGeneration.Enabled = false` → Enqueue not called even for eligible conv.
- [ ] `[T] C3.5` — Scenario "Feature hook is a no-op when TitleGenerator is not wired": `a.titler = nil` → no panic, no call.
- [ ] `[I] C3.6` — Add `WithTitler(*TitleGenerator) *Agent` setter + `titler *TitleGenerator` field on Agent struct.
- [ ] `[I] C3.7` — Wire the post-save hook in `processMessage` per design §2.4.
- [ ] `[V] C3.8` — C3.1–C3.5 green.

### C4. Concurrency warning (defensive add)
- [ ] `[T] C4.1` — Test: two `processMessage` calls on same convID concurrently both complete; assert `slog.Warn` is emitted for the loser when its starting `UpdatedAt` was older than the winner's final save time. (Implementation-deferred: if detecting collision is non-trivial, skip the Warn and just document; flag it with a comment.)
- [ ] `[I] C4.2` — (Optional) Record `conv.UpdatedAt` at load, compare at save, emit warn on mismatch beyond a small delta.
- [ ] `[V] C4.3` — C4.1 green OR task explicitly deferred with a doc comment in the code.

---

## Group D — TitleGenerator

Dep: A1, B5.

### D1. Clock interface
- [ ] `[T] D1.1` — Trivial unit test: `systemClock{}.Now()` returns a time within 1ms of `time.Now()`. `(&fakeClock{t:T}).Now()` returns T. `Advance(d)` moves it.
- [ ] `[I] D1.2` — Define `Clock` interface, `systemClock`, `fakeClock` in `internal/store/clock.go` (shared across pruner + titler, or duplicate if import cycle).
- [ ] `[V] D1.3` — D1.1 green.

### D2. Config + defaults
- [ ] `[T] D2.1` — `applyTitleDefaults`: empty config → `{Enabled:true, WorkerCount:2, QueueSize:32, CallTimeout:30s}`. Clamps: WorkerCount 0→1, 20→8; QueueSize 2→4, 999→256.
- [ ] `[I] D2.2` — Implement `applyTitleDefaults`.
- [ ] `[V] D2.3` — D2.1 green.

### D3. `TitleGenerator` construction + shutdown
- [ ] `[T] D3.1` — Scenario "Bounded queue does not block caller": fill queue to capacity, `Enqueue` returns immediately, `slog.Warn` observable via a slog handler sink.
- [ ] `[T] D3.2` — Scenario "Graceful shutdown": enqueue 4 jobs with a slow mock provider, `Stop(ctx=5s)` returns nil, in-flight completes, pending dropped.
- [ ] `[T] D3.3` — Scenario "Shutdown deadline exceeded": stuck provider call, `Stop(ctx=100ms)` returns `ctx.Err()`.
- [ ] `[I] D3.4` — Implement `NewTitleGenerator`, `Enqueue`, `Stop`, worker goroutine skeleton.
- [ ] `[V] D3.5` — D3.1–D3.3 green.

### D4. Worker execution
- [ ] `[T] D4.1` — Scenario "Successful generation": mock provider returns `"A great chat"`, worker processes one job, assert `metadata["title"] = "A great chat"` after `SaveConversation`.
- [ ] `[T] D4.2` — Scenario "Provider timeout is silent": provider blocks >30s, worker aborts, no save, slog.Warn observable.
- [ ] `[T] D4.3` — Scenario "Empty/whitespace response": provider returns `"   "`, no save.
- [ ] `[T] D4.4` — Scenario "Conversation was deleted before worker ran": soft-delete between enqueue and processing → LoadConversation returns ErrNotFound → silent drop.
- [ ] `[T] D4.5` — Scenario "Conversation already has a title" (race): manual rename between enqueue and processing → re-check fails → no overwrite.
- [ ] `[T] D4.6` — Scenario "Media blocks omitted from prompt": conv with image attachment, assert prompt string has no binary/base64 content; only text.
- [ ] `[I] D4.7` — Implement `run()`, `buildTitlePrompt()`, `normalizeTitle()`.
- [ ] `[V] D4.8` — D4.1–D4.6 green.

### D5. Wiring
- [ ] `[T] D5.1` — Scenario "Config flag disables wiring": with `enabled=false`, `buildTitler` returns nil, `Agent.WithTitler(nil)` path works (C3.5 covers the Agent side).
- [ ] `[I] D5.2` — Add `buildTitler(store, prov, cfg, clock)` helper in `cmd/daimon/titler_wiring.go` (or append to `rag_wiring.go`).
- [ ] `[I] D5.3` — Call from `main.go` and `web_cmd.go` (verify both paths — open question Q6.1).
- [ ] `[V] D5.4` — `go build ./...` green; D5.1 green.

---

## Group E — ConversationPruner

Dep: A1, B3.

### E1. Core
- [ ] `[T] E1.1` — Scenario "Prune removes expired soft-deleted convs": seed 3 convs at `deleted_at = clock.Now() - 31d`, retention=30d, `Tick(ctx)` → 3 rows deleted, slog.Info captures counts.
- [ ] `[T] E1.2` — Scenario "Prune leaves recent soft-deleted alone": conv at 5d old, retention=30d → not deleted.
- [ ] `[T] E1.3` — Scenario "Prune leaves live convs alone": live convs untouched.
- [ ] `[T] E1.4` — Scenario "Test with injected clock": use `fakeClock`, `Advance(8d)`, `Tick` — validates deterministic behavior end-to-end.
- [ ] `[T] E1.5` — Scenario "DB error does not kill goroutine": inject a store that returns an error on DeleteConversationsOlderThan → slog.Error emitted, next Tick tries again.
- [ ] `[I] E1.6` — Implement `ConversationPruner` struct + `NewConversationPruner`, `Start`, `Tick`, `loop`, `Stop` (design §2.9).
- [ ] `[V] E1.7` — E1.1–E1.5 green.

### E2. Wiring + startup/shutdown
- [ ] `[T] E2.1` — Integration-ish: start a server with pruner enabled, call `server.Stop()`, assert pruner goroutine exits within 100ms.
- [ ] `[T] E2.2` — Scenario "Pruner disabled via config": `enabled=false` → no goroutine, slog.Info at startup.
- [ ] `[T] E2.3` — Scenario "Out-of-range retention is clamped": 5000 days in config → effective 3650, slog.Warn noted (A1.2 already covers the clamp; this is the integration path).
- [ ] `[I] E2.4` — Construct + start the pruner in `internal/web/server.go`'s `Start()` alongside existing server lifetime goroutines. Call `pruner.Stop()` in `server.Stop()`.
- [ ] `[V] E2.5` — E2.1–E2.3 green.

---

## Group F — Web endpoints

Dep: A2, B1–B5.

### F1. WS upgrade + IncomingMessage tagging
- [ ] `[T] F1.1` — Scenario "Upgrade with conversation_id binds the session": dial `/ws/chat?conversation_id=conv_web:x:u1`, send a message, assert inbox receives `IncomingMessage.ConversationID = "conv_web:x:u1"`.
- [ ] `[T] F1.2` — Scenario "Upgrade without conversation_id is unchanged": dial `/ws/chat`, assert `IncomingMessage.ConversationID == ""`.
- [ ] `[T] F1.3` — Scenario "Upgrade with conversation_id that does not match existing conv": dial with bogus id, send msg, no error at WS layer (agent creates fresh conv per C1.3).
- [ ] `[T] F1.4` — Defensive test: conversation_id over 200 chars is dropped, logged, treated as empty.
- [ ] `[I] F1.5` — Modify `HandleWebSocket` to read `?conversation_id=` before Upgrade and tag every dispatched `IncomingMessage` with it.
- [ ] `[V] F1.6` — F1.1–F1.4 green.

### F2. `GET /api/conversations/{id}/messages`
- [ ] `[T] F2.1` — Scenario "Initial load fetches most recent window": 200-msg conv, `GET …/messages?limit=50` → 50 msgs, oldest_index=150, has_more=true.
- [ ] `[T] F2.2` — Scenario "Paging upward": follow-up with `?before=150&limit=50` → indexes 100..149.
- [ ] `[T] F2.3` — Scenario "404 on soft-deleted conv".
- [ ] `[T] F2.4` — Test `limit` clamping via HTTP params; invalid `before` strings return 400.
- [ ] `[I] F2.5` — Implement `handleGetConversationMessages` + register route.
- [ ] `[V] F2.6` — F2.1–F2.4 green.

### F3. `PATCH /api/conversations/{id}`
- [ ] `[T] F3.1` — Scenario "Valid rename persists": PATCH `{"title": "Mi nuevo hilo"}` → 200, metadata.title = "Mi nuevo hilo".
- [ ] `[T] F3.2` — Scenario "Empty title is rejected": `{"title": "   "}` → 400.
- [ ] `[T] F3.3` — Scenario "Oversize title is rejected": 101-rune title → 400.
- [ ] `[T] F3.4` — Scenario "Newline is stripped": title `"line1\nline2"` → stored as `"line1 line2"`.
- [ ] `[T] F3.5` — Test: malformed JSON body → 400.
- [ ] `[T] F3.6` — Test: PATCH on soft-deleted conv → 404.
- [ ] `[I] F3.7` — Implement `handlePatchConversation` + register route.
- [ ] `[V] F3.8` — F3.1–F3.6 green.

### F4. `POST /api/conversations/{id}/restore`
- [ ] `[T] F4.1` — Scenario "Restoring a soft-deleted conv" → 200, next GET returns the conv.
- [ ] `[T] F4.2` — Scenario "Restoring a live conv is 404".
- [ ] `[T] F4.3` — Test: restoring nonexistent id → 404.
- [ ] `[I] F4.4` — Implement `handleRestoreConversation` + register route.
- [ ] `[V] F4.5` — F4.1–F4.3 green.

### F5. Title in list summary
- [ ] `[T] F5.1` — Scenario "List returns metadata title when present": conv with `metadata.title="Sobre RAG"` → summary.title = "Sobre RAG".
- [ ] `[T] F5.2` — Scenario "List falls back to truncated first msg": no metadata.title, first user-msg "una pregunta muy larga..." → title = that truncated.
- [ ] `[T] F5.3` — Test: conv with no user message yet → title = "".
- [ ] `[I] F5.4` — Extend the summary builder in `handleListConversations` to derive title per spec.
- [ ] `[V] F5.5` — F5.1–F5.3 green.

### F6. DELETE becomes soft (no handler code change, just verify)
- [ ] `[T] F6.1` — Scenario "Delete performs soft delete": existing test extended — after DELETE, row exists in DB (query directly) but subsequent GET returns 404.
- [ ] `[V] F6.2` — F6.1 green; no additional code needed because B1 made the store method soft.

---

## Group G — Frontend hooks

Dep: Backend Groups A–F (for API contract).

### G1. `useWebSocket` searchParams
- [ ] `[T] G1.1` — Scenario "Reconnect after network blip": render with searchParams={conversation_id: "conv_x"}, simulate close + reconnect (mock timers), assert new socket URL has the query string.
- [ ] `[T] G1.2` — Scenario "No query string unchanged": render without searchParams, reconnect, URL is just `path`.
- [ ] `[I] G1.3` — Extend `UseWebSocketOptions` with optional `searchParams`, include in `connect()` URL build.
- [ ] `[V] G1.4` — G1.1, G1.2 green (vitest + mocked WebSocket).

### G2. `useInfiniteConversationMessages`
- [ ] `[T] G2.1` — Scenario "Initial load": mock fetch returns first page (50 msgs, has_more=true, oldest_index=150). Hook state shows one page loaded.
- [ ] `[T] G2.2` — Scenario "fetchNextPage advances cursor": second call to mock receives `?before=150`.
- [ ] `[T] G2.3` — Test: `enabled: false` (null convID) → no fetch.
- [ ] `[I] G2.4` — Implement hook (design §3.2).
- [ ] `[V] G2.5` — G2.1–G2.3 green.

### G3. `useResumeSession`
- [ ] `[T] G3.1` — Scenario "canContinue true when last msg is user": mock `useInfiniteConversationMessages` returns a page ending in a user-role message. `canContinue === true`.
- [ ] `[T] G3.2` — Scenario "canContinue false when last msg is assistant": similar setup, last = assistant. `canContinue === false`.
- [ ] `[T] G3.3` — Test: no convID (null) → `canContinue === false`, nothing fetched.
- [ ] `[T] G3.4` — Test: `continueTurn()` calls `send({type: "continue_turn"})`.
- [ ] `[I] G3.5` — Implement hook (design §3.3).
- [ ] `[V] G3.6` — G3.1–G3.4 green.

### G4. `bucketForTimestamp`
- [ ] `[T] G4.1` — Table test: boundaries at 24h, 7d, 30d, 90d; exactly-on and just-past assertions.
- [ ] `[T] G4.2` — Scenario "Future timestamp returns today" (clock skew tolerance).
- [ ] `[I] G4.3` — Implement pure fn in `src/utils/timeBuckets.ts`.
- [ ] `[V] G4.4` — G4.1, G4.2 green.

---

## Group H — Frontend components

Dep: G (for hooks), existing Liminal primitives.

### H1. `LiminalCard` base shell
- [ ] `[T] H1.1` — Component test: renders children; applies accent style via CSS var; calls `onClick` when body clicked.
- [ ] `[T] H1.2` — Test: density variants produce different padding (query computed style or snapshot).
- [ ] `[I] H1.3` — Extract shell in `src/components/liminal/LiminalCard.tsx` (design §3.4).
- [ ] `[V] H1.4` — H1.1, H1.2 green.
- [ ] NOTE: MemoryCard is NOT refactored in this SDD. Add TODO comment on `MemoryCard.tsx` pointing to LiminalCard for future adoption.

### H2. `ConversationCard`
- [ ] `[T] H2.1` — Test: conv with `metadata.title="..."` → that title rendered; without → truncated first user msg rendered in italic/auto style.
- [ ] `[T] H2.2` — Test: clicking card body calls `onClick(id)`; clicking delete icon calls `onDelete(id)` and does NOT trigger onClick (stopPropagation).
- [ ] `[T] H2.3` — Test: empty preview rendered for 0-message convs.
- [ ] `[I] H2.4` — Implement `ConversationCard` using `LiminalCard` base.
- [ ] `[V] H2.5` — H2.1–H2.3 green.

### H3. `TimeClusterHeader`
- [ ] `[T] H3.1` — Test: renders Spanish label for each bucket; renders count number.
- [ ] `[I] H3.2` — Implement per design §3.5.
- [ ] `[V] H3.3` — H3.1 green.

### H4. `ConversationsPreamble`
- [ ] `[T] H4.1` — Test: renders default copy when convs exist; renders empty-state copy when count=0.
- [ ] `[I] H4.2` — Implement with Liminal voice.
- [ ] `[V] H4.3` — H4.1 green.

### H5. `ConversationsToolbar`
- [ ] `[T] H5.1` — Test: search input emits `onSearchChange` on typing (debounced or immediate — match MemoryToolbar convention).
- [ ] `[I] H5.2` — Implement based on `MemoryToolbar` with conv-specific filters (channel filter dropdown v1).
- [ ] `[V] H5.3` — H5.1 green.

### H6. Toast undo
- [ ] `[V] H6.1` — Grep existing toast infra (open question Q6.2). Decide extend vs build.
- [ ] `[T] H6.2` — Test: undo toast expires after 10s via `onExpire`; clicking action button fires `onAction` and dismisses immediately.
- [ ] `[I] H6.3` — Implement or extend.
- [ ] `[V] H6.4` — H6.2 green.

---

## Group I — Frontend pages

Dep: G, H.

### I1. `ConversationsPage` rewrite
- [ ] `[T] I1.1` — Scenario "Empty state": render with 0 convs → preamble visible, no cluster headers.
- [ ] `[T] I1.2` — Scenario "Convs render into their time buckets": 5 convs spread across buckets → correct distribution and bucket ordering.
- [ ] `[T] I1.3` — Scenario "Search filters client-side": type "RAG" → only matching cards visible, no network call.
- [ ] `[T] I1.4` — Scenario "Delete with undo": click delete → DELETE fires, card gone, toast visible; click Deshacer → POST restore, card back.
- [ ] `[T] I1.5` — Scenario "Clicking a card navigates to detail".
- [ ] `[I] I1.6` — Rewrite `ConversationsPage.tsx` using Preamble + Toolbar + TimeClusterHeader + ConversationCard + useConversations.
- [ ] `[V] I1.7` — I1.1–I1.5 green.

### I2. `ConversationDetailPage` rewrite
- [ ] `[T] I2.1` — Scenario "Initial load shows latest 50 messages": 200-msg conv → only 50 in DOM, "Cargar anteriores" button visible.
- [ ] `[T] I2.2` — Scenario "Load-more prepends older messages": click → next page fetched and prepended, scroll position preserved.
- [ ] `[T] I2.3` — Scenario "Rename title": click title → editable → submit → PATCH fires → summary re-fetched.
- [ ] `[T] I2.4` — Scenario "Resume button": click → router navigates to `/chat?conversation_id={id}`.
- [ ] `[T] I2.5` — Test: JSON/MD export still works (call existing export with full conv fetch).
- [ ] `[I] I2.6` — Rewrite `ConversationDetailPage.tsx` using LiminalThread + useInfiniteConversationMessages + rename input + Resume CTA + soft-delete toast.
- [ ] `[V] I2.7` — I2.1–I2.5 green.

### I3. `ChatPage` Resume flow
- [ ] `[T] I3.1` — Scenario "Resume with history": URL has `?conversation_id=conv_x`, WS connects with that query, thread prefilled with last 50 msgs.
- [ ] `[T] I3.2` — Scenario "Resume without conversation_id": no query → pre-change behavior (fresh conv).
- [ ] `[T] I3.3` — Scenario "canContinue true shows button": setup where last msg = user → button visible.
- [ ] `[T] I3.4` — Scenario "canContinue false hides button": last msg = assistant → button hidden.
- [ ] `[T] I3.5` — Click "Continuar" → send(`{type: "continue_turn"}`) observed on WS.
- [ ] `[I] I3.6` — Extract `useResumeSession` integration in ChatPage. Keep LOC bounded ≤ ~1000.
- [ ] `[V] I3.7` — I3.1–I3.5 green.
- [ ] `[V] I3.8` — `wc -l src/pages/ChatPage.tsx` ≤ 1000.

---

## Group J — Docs + final integration

### J1. Docs
- [ ] `[I] J1.1` — Update `DAIMON.md` §config with the `conversations.prune` + `ai.title_generation` blocks + descriptions.
- [ ] `[I] J1.2` — Update `DAIMON.md` API section with the new endpoints (GET /messages, PATCH, POST /restore).
- [ ] `[I] J1.3` — `TESTS.md` DoD updated: note new spec scenarios covered.
- [ ] `[V] J1.4` — Skim-proofread both files.

### J2. End-to-end integration test
- [ ] `[T] J2.1` — Go integration test: start a test server + in-memory SQLite, spawn WS client with `?conversation_id=conv_e2e:x:u1`, send message, wait for response, close socket, reconnect with same conversation_id, assert history is preserved and the agent treats the new message as a continuation of the same conv.
- [ ] `[V] J2.2` — J2.1 green.

### J3. Manual acceptance walkthrough
- [ ] `[V] J3.1` — Run `go test ./... -race` → all green.
- [ ] `[V] J3.2` — Run `golangci-lint run` → clean.
- [ ] `[V] J3.3` — Run `go vet ./...` → clean.
- [ ] `[V] J3.4` — Run `vitest run` in daimon-frontend → all green.
- [ ] `[V] J3.5` — Run `tsc --noEmit` in daimon-frontend → clean.
- [ ] `[V] J3.6` — Spin up daimon dev server + frontend dev server. Manually verify each acceptance criterion from proposal §8 (7 items). Document any deviations.
- [ ] `[V] J3.7` — If J3.6 clean → ready for `/sdd-verify` phase.

---

## Counts / estimates

- **Groups A–J: 78 task items** (mix of T/I/V). The T+I pairs align with the 75 scenarios from specs; J items are verification/docs.
- **Strict TDD ordering**: write `[T]` → red → `[I]` → green → `[V]` per task.
- **Apply phase**: expected to span multiple batches. A good batch is one Group (A, B, C, …) per sitting, with commits per Group.
- **Commits**: one per Group minimum; Group F and Group I may split further by sub-section (F1 WS separate from F2 messages endpoint, etc).

## Dependency graph (high-level)

```
A ───► B ─┬─► C
          └─► D ─┐
                │
          B ───► E ─┐
                   │
A,B,C,D,E ────► F ──► G ───► H ───► I
                                     │
                                     └─► J (docs + E2E)
```

Apply phase can parallelize Group G (frontend hooks) with Groups C–E once A and B land, but serialized execution is simpler and the SDD runs interactive-mode anyway.
