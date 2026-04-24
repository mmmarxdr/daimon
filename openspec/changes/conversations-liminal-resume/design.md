# Design: conversations-liminal-resume

## 1. Architectural overview

```
                      ┌──────────────────────────┐
 Browser              │ React + TanStack Query   │
 ─────────────────    │                          │
  /conversations   ◄──┤ ConversationsPage        │
  /conversations/:id  │ ConversationDetailPage   │
  /chat?conv_id=    ◄─┤ ChatPage + useResumeSession
                      └───────────┬──────────────┘
                                  │
                   HTTP REST       │      WebSocket
                   GET /api/…      │      /ws/chat?conversation_id=…
                   PATCH, POST     │
                                  ▼
          ┌─────────────────────────────────────────┐
          │  internal/web/server.go                  │
          │  - handler_conversations.go (REST)       │
          │  - channel/web.go (WS upgrade)           │
          └──────────┬──────────────────┬───────────┘
                     │                  │
       IncomingMessage{                 │
         ConversationID: "conv_web:…"   │
         …                              │
       }                                │
                     │                  ▼
                     │    ┌─────────────────────────────┐
                     └───►│ internal/agent/loop.go      │
                          │  processMessage(ctx, msg)    │
                          │    ├─ resolve convID         │
                          │    ├─ Load / Save            │
                          │    └─ Enqueue title job ────┼──► TitleGenerator (async pool)
                          └─────────────┬────────────────┘
                                        ▼
                          ┌──────────────────────────────┐
                          │ internal/store (SQLite)      │
                          │  + v14 migration: deleted_at  │
                          │  + GetConversationMessages    │
                          │  + RestoreConversation        │
                          └──────────────────────────────┘
                                        ▲
                          ConversationPruner ◄─ ticker
```

Three cross-cutting changes:
- **Backend**: new `ConversationID` flowing from WS query param → `IncomingMessage` → `processMessage`
- **Store**: soft delete + paginated read + pruner
- **Frontend**: Liminal UI + Resume flow + history pagination

## 2. Backend design

### 2.1 `IncomingMessage.ConversationID` (additive, backward-compat)

```go
// internal/channel/channel.go — add one field.
type IncomingMessage struct {
    ID             string
    ChannelID      string
    SenderID       string
    Timestamp      time.Time
    Content        content.Blocks
    IsContinuation bool
    Unlimited      bool
    ConversationID string // NEW — when non-empty, processMessage bypasses userScope
}
```

All existing channel implementations keep working because the new field zero-valued is "fallback to userScope".

### 2.2 WS upgrade with `?conversation_id=`

```go
// internal/channel/web.go — HandleWebSocket
func (w *WebChannel) HandleWebSocket(rw http.ResponseWriter, r *http.Request) {
    if w.connCount() >= wsMaxConnections { /* existing cap */ }

    // Read identity BEFORE Upgrade. We validate size to guard against
    // pathological query strings. 200 chars > any realistic convID.
    resumedConvID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
    if len(resumedConvID) > 200 {
        slog.Warn("ws: conversation_id too long, ignoring", "len", len(resumedConvID))
        resumedConvID = ""
    }

    conn, err := w.upgrader.Upgrade(rw, r, nil)
    // … existing setup: ReadLimit, deadlines, pongHandler, sync.Map store …
    connID := "web:" + uuid.New().String()[:8]
    wc := &wsConn{conn: conn}
    w.conns.Store(connID, wc)

    // When we dispatch IncomingMessages to the inbox, we tag every message
    // from this connection with resumedConvID. The connID stays as the
    // transport handle (used for w.conns lookup / streaming replies).
    // …
}
```

The dispatch point (where `IncomingMessage` is constructed) is already inside `HandleWebSocket` (reading from `conn.ReadMessage`). We just set `ConversationID: resumedConvID` alongside the existing `ChannelID: connID`.

**Backward compat**: old clients send no query param → `resumedConvID=""` → `IncomingMessage.ConversationID=""` → `processMessage` falls back to `userScope`. Zero behavior change.

### 2.3 `processMessage` convID resolution

```go
// internal/agent/loop.go — at the top of processMessage, replacing lines 99-111
convID := msg.ConversationID
if convID == "" {
    convID = "conv_" + userScope(msg.ChannelID, msg.SenderID)
}
scope := strings.TrimPrefix(convID, "conv_") // for SearchMemory / RAG scope

conv, err := a.store.LoadConversation(ctx, convID)
if err != nil && !errors.Is(err, store.ErrNotFound) {
    slog.Warn("failed to load conversation, starting fresh", "id", convID, "error", err)
}
if conv == nil {
    conv = &store.Conversation{
        ID:        convID,
        ChannelID: msg.ChannelID,
        CreatedAt: time.Now(),
    }
}
```

One invariant to preserve: `scope` is what `SearchMemory` and RAG scope use. Today it's `userScope(channelID, senderID)`. After the change, `scope = strings.TrimPrefix(convID, "conv_")`. Because `convID = "conv_" + userScope(…)` is how we *generate* convIDs, the two are algebraically equivalent. The only edge case is someone passing a `conversation_id` that doesn't start with `"conv_"` — we treat the whole thing as scope. Acceptable; validation at the REST layer can reject malformed IDs if needed (out of scope for v1).

### 2.4 Title hook wiring

After `SaveConversation` at line 682:

```go
conv.UpdatedAt = time.Now()
_ = a.store.SaveConversation(ctx, *conv)

if a.titler != nil && a.cfg.AI.TitleGeneration.Enabled {
    if shouldGenerateTitle(conv) {
        a.titler.Enqueue(ctx, conv.ID)
    }
}

// … existing bus.Emit / telemetry …
```

```go
// eligibility helper, pure, exported as private package fn for tests
func shouldGenerateTitle(conv *store.Conversation) bool {
    if conv == nil { return false }
    if len(conv.Messages) < 6 { return false }
    if title := conv.Metadata["title"]; strings.TrimSpace(title) != "" { return false }
    firstUser := firstUserMessageText(conv.Messages)
    return utf8.RuneCountInString(firstUser) >= 20
}
```

### 2.5 Schema migration v14

Append to the existing migration chain (`internal/store/migration.go`):

```go
// migrateV14 adds soft-delete column to conversations.
// Idempotent via schema_version gate.
func migrateV14(db *sql.DB) error {
    // Use the standard column-exists guard — SQLite's ALTER TABLE is
    // version-agnostic but repeated adds fail with "duplicate column name".
    if colExists(db, "conversations", "deleted_at") {
        return nil
    }
    if _, err := db.Exec(`ALTER TABLE conversations ADD COLUMN deleted_at TIMESTAMP NULL`); err != nil {
        return fmt.Errorf("v14: add deleted_at: %w", err)
    }
    if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_conversations_deleted_at
        ON conversations(deleted_at) WHERE deleted_at IS NOT NULL`); err != nil {
        return fmt.Errorf("v14: index: %w", err)
    }
    return nil
}
```

Register in the migration ordered list alongside v2…v13. `schema_version` updates to 14 after success.

### 2.6 Soft delete + restore

Replace `DeleteConversation` body:

```go
// internal/store/sqlite.go (wherever DeleteConversation lives)
func (s *sqliteStore) DeleteConversation(ctx context.Context, id string) error {
    res, err := s.db.ExecContext(ctx,
        `UPDATE conversations SET deleted_at = ? WHERE id = ? AND deleted_at IS NULL`,
        time.Now().UTC(), id)
    if err != nil { return err }
    rows, _ := res.RowsAffected()
    if rows == 0 {
        // Could be: conv doesn't exist, or already soft-deleted. The existing
        // test expects ErrNotFound only when conv truly doesn't exist. Check.
        var count int
        _ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversations WHERE id = ?`, id).Scan(&count)
        if count == 0 { return ErrNotFound }
        // already soft-deleted — no-op per spec
    }
    return nil
}

func (s *sqliteStore) RestoreConversation(ctx context.Context, id string) error {
    res, err := s.db.ExecContext(ctx,
        `UPDATE conversations SET deleted_at = NULL WHERE id = ? AND deleted_at IS NOT NULL`,
        id)
    if err != nil { return err }
    rows, _ := res.RowsAffected()
    if rows == 0 { return ErrNotFound } // either missing or already live
    return nil
}
```

**Read-path filter (CRITICAL AUDIT)**: every SELECT on `conversations` must add `AND deleted_at IS NULL`. Audit list:
- `LoadConversation(id)` — add filter
- `ListConversationsPaginated(channel, limit, offset)` — add filter (COUNT(*) + SELECT rows)
- New `GetConversationMessages(id, …)` — add filter (exits with ErrNotFound if deleted)
- Any future FTS5 search — add filter

A spec scenario pins this; tests cover it. A grep check (`grep -n "FROM conversations" internal/store/`) at PR review catches drift.

### 2.7 Paginated messages — load-and-slice for v1

```go
func (s *sqliteStore) GetConversationMessages(
    ctx context.Context, id string, beforeIndex int, limit int,
) (msgs []provider.ChatMessage, hasMore bool, oldestIndex int, err error) {
    if limit <= 0 { limit = 50 }
    if limit > 200 { limit = 200 }

    conv, err := s.LoadConversation(ctx, id) // already filters deleted_at
    if err != nil { return nil, false, 0, err }

    total := len(conv.Messages)
    if total == 0 {
        return []provider.ChatMessage{}, false, 0, nil
    }

    endExclusive := total
    if beforeIndex >= 0 && beforeIndex < total {
        endExclusive = beforeIndex
    }
    start := endExclusive - limit
    if start < 0 { start = 0 }

    window := conv.Messages[start:endExclusive]
    // Return a COPY so callers mutating the slice don't corrupt the cached conv
    // (if we ever add caching). Cheap at this scale.
    out := make([]provider.ChatMessage, len(window))
    copy(out, window)

    return out, start > 0, start, nil
}
```

**Tradeoff documented**: this materializes the full JSON blob into memory to slice in Go. For typical convs (<1000 messages) this is ~hundreds of KB, fine. For pathological convs (100k+ messages, MB of JSON) it will slow down and use extra memory. **Not a v1 concern.** Follow-up can normalize messages into a separate table or use SQLite JSON1 `json_extract('$[N]')` for random access.

### 2.8 `TitleGenerator`

```go
// internal/agent/titler.go
type TitleGenerator struct {
    jobs    chan string   // convID
    store   ConvLoaderSaver // subset of store.Store we depend on
    provider provider.Provider
    model   string
    cfg     TitleGenConfig
    clock   Clock

    stopOnce sync.Once
    doneCtx  context.Context
    cancel   context.CancelFunc
    wg       sync.WaitGroup
}

type TitleGenConfig struct {
    Enabled       bool
    Model         string        // empty → use provider default
    WorkerCount   int           // default 2, min 1, max 8
    QueueSize     int           // default 32, min 4, max 256
    CallTimeout   time.Duration // default 30s
}

func NewTitleGenerator(store ConvLoaderSaver, prov provider.Provider, cfg TitleGenConfig, clock Clock) *TitleGenerator {
    cfg = applyTitleDefaults(cfg)
    ctx, cancel := context.WithCancel(context.Background())
    tg := &TitleGenerator{
        jobs: make(chan string, cfg.QueueSize),
        store: store, provider: prov, cfg: cfg, clock: clock,
        doneCtx: ctx, cancel: cancel,
    }
    for i := 0; i < cfg.WorkerCount; i++ {
        tg.wg.Add(1)
        go tg.worker()
    }
    return tg
}

func (tg *TitleGenerator) Enqueue(_ context.Context, convID string) {
    // Non-blocking. Drop on full queue.
    select {
    case tg.jobs <- convID:
    case <-tg.doneCtx.Done():
    default:
        slog.Warn("title_generator: queue full, job dropped", "conv_id", convID)
    }
}

func (tg *TitleGenerator) Stop(ctx context.Context) error {
    tg.stopOnce.Do(func() {
        tg.cancel()   // signals workers to exit after current job
        close(tg.jobs) // (workers drain remaining then exit)
    })
    done := make(chan struct{})
    go func() { tg.wg.Wait(); close(done) }()
    select {
    case <-done:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (tg *TitleGenerator) worker() {
    defer tg.wg.Done()
    for convID := range tg.jobs {
        tg.run(convID)
    }
}

func (tg *TitleGenerator) run(convID string) {
    ctx, cancel := context.WithTimeout(tg.doneCtx, tg.cfg.CallTimeout)
    defer cancel()

    conv, err := tg.store.LoadConversation(ctx, convID)
    if err != nil {
        if !errors.Is(err, store.ErrNotFound) {
            slog.Warn("title_generator: load failed", "conv_id", convID, "error", err)
        }
        return
    }
    if !shouldGenerateTitle(conv) {
        return // race: someone renamed or added <20 runes, skip
    }

    prompt := buildTitlePrompt(conv.Messages)
    resp, err := tg.provider.Chat(ctx, provider.ChatRequest{
        Model:    tg.cfg.Model, // empty → provider uses its default
        Messages: []provider.ChatMessage{{Role: "user", Content: content.TextBlock(prompt)}},
    })
    if err != nil {
        slog.Warn("title_generator: chat failed", "conv_id", convID, "error", err)
        return
    }
    title := normalizeTitle(resp.TextContent()) // implementation matches spec: trim, strip MD, ≤100 runes
    if title == "" { return }

    // Reload + check to avoid overwriting a manual rename that happened during the call
    conv, err = tg.store.LoadConversation(ctx, convID)
    if err != nil || !shouldGenerateTitle(conv) { return }

    if conv.Metadata == nil { conv.Metadata = map[string]string{} }
    conv.Metadata["title"] = title
    if err := tg.store.SaveConversation(ctx, *conv); err != nil {
        slog.Warn("title_generator: save failed", "conv_id", convID, "error", err)
    }
}
```

**Design decision — wiring from main**: `cmd/daimon/main.go` and `cmd/daimon/web_cmd.go` both construct the agent. Add a shared helper `buildTitler(store, prov, cfg)` similar to `buildSummaryFn` / `buildHypothesisFn` in `cmd/daimon/rag_wiring.go`. Passes `nil` when `!enabled`.

### 2.9 `ConversationPruner`

```go
// internal/store/pruner.go
type Clock interface { Now() time.Time }

type systemClock struct{}
func (systemClock) Now() time.Time { return time.Now().UTC() }

type ConversationPruner struct {
    store    ConvPruneStore
    clock    Clock
    cfg      PruneConfig

    cancel context.CancelFunc
    done   chan struct{}
}

type PruneConfig struct {
    Enabled        bool
    RetentionDays  int           // clamped 1..3650, default 30
    Interval       time.Duration // from interval_hours, clamped 1h..168h, default 6h
}

func NewConversationPruner(store ConvPruneStore, clock Clock, cfg PruneConfig) *ConversationPruner {
    return &ConversationPruner{store: store, clock: clock, cfg: applyPruneDefaults(cfg)}
}

func (p *ConversationPruner) Start(ctx context.Context) {
    if !p.cfg.Enabled {
        slog.Info("conversation_pruner_disabled")
        return
    }
    ctx, p.cancel = context.WithCancel(ctx)
    p.done = make(chan struct{})
    go p.loop(ctx)
}

func (p *ConversationPruner) loop(ctx context.Context) {
    defer close(p.done)
    ticker := time.NewTicker(p.cfg.Interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            p.Tick(ctx)
        }
    }
}

// Tick exposed for tests (fake clock + manual drive).
func (p *ConversationPruner) Tick(ctx context.Context) {
    start := p.clock.Now()
    cutoff := start.Add(-time.Duration(p.cfg.RetentionDays) * 24 * time.Hour)
    n, err := p.store.DeleteConversationsOlderThan(ctx, cutoff)
    dur := time.Since(start).Milliseconds()
    if err != nil {
        slog.Error("pruner_run_error", "error", err, "duration_ms", dur)
        return
    }
    slog.Info("pruner_run", "deleted_count", n, "duration_ms", dur)
}

func (p *ConversationPruner) Stop() {
    if p.cancel != nil {
        p.cancel()
        <-p.done
    }
}
```

`ConvPruneStore` is a narrow interface:

```go
type ConvPruneStore interface {
    DeleteConversationsOlderThan(ctx context.Context, cutoff time.Time) (int, error)
}
```

Implemented once on `sqliteStore`:

```go
func (s *sqliteStore) DeleteConversationsOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
    res, err := s.db.ExecContext(ctx,
        `DELETE FROM conversations WHERE deleted_at IS NOT NULL AND deleted_at < ?`,
        cutoff.UTC())
    if err != nil { return 0, err }
    n, _ := res.RowsAffected()
    return int(n), nil
}
```

### 2.10 Config shape

Place both new blocks under a new `Conversations` top-level config:

```go
// internal/config/config.go
type Config struct {
    // … existing fields …
    Conversations ConversationsConfig `yaml:"conversations" json:"conversations"`
    AI            AIConfig            `yaml:"ai"            json:"ai"`
}

type ConversationsConfig struct {
    Prune PruneConfig `yaml:"prune" json:"prune"`
}

type PruneConfig struct {
    Enabled        bool `yaml:"enabled"         json:"enabled"`
    RetentionDays  int  `yaml:"retention_days"  json:"retention_days"`
    IntervalHours  int  `yaml:"interval_hours"  json:"interval_hours"`
}

type AIConfig struct {
    TitleGeneration TitleGenYAMLConfig `yaml:"title_generation" json:"title_generation"`
}

type TitleGenYAMLConfig struct {
    Enabled        bool   `yaml:"enabled"           json:"enabled"`
    Model          string `yaml:"model,omitempty"   json:"model,omitempty"`
    WorkerCount    int    `yaml:"worker_count"      json:"worker_count"`
    QueueSize      int    `yaml:"queue_size"        json:"queue_size"`
    CallTimeoutMS  int    `yaml:"call_timeout_ms"   json:"call_timeout_ms"`
}
```

YAML example (added to `DAIMON.md`):
```yaml
conversations:
  prune:
    enabled: true
    retention_days: 30
    interval_hours: 6

ai:
  title_generation:
    enabled: true
    model: ""              # empty → provider default
    worker_count: 2
    queue_size: 32
    call_timeout_ms: 30000
```

`ApplyDefaults` in config.go fills any missing field. A `patchConversations` / `patchAI` allow-list in `handler_config.go` mirrors the RAG / memory pattern so the UI config editor doesn't silently drop new fields (avoiding the class of bug flagged in the rag-hyde proposal §9).

### 2.11 REST handlers

New functions in `internal/web/handler_conversations.go`:

- `handleGetConversationMessages(w http.ResponseWriter, r *http.Request)` — reads `r.PathValue("id")`, `before`, `limit` query params; calls `store.GetConversationMessages`.
- `handlePatchConversation(w http.ResponseWriter, r *http.Request)` — JSON decode `{title string}`, validate, call `store.UpdateConversationTitle(ctx, id, title)` (new method, pure SQL: `UPDATE conversations SET metadata = json_set(COALESCE(metadata,'{}'), '$.title', ?) WHERE id = ? AND deleted_at IS NULL`).
- `handleRestoreConversation(w http.ResponseWriter, r *http.Request)` — calls `store.RestoreConversation`.

Register in `server.go`:

```go
s.mux.HandleFunc("GET /api/conversations/{id}/messages",   s.handleGetConversationMessages)
s.mux.HandleFunc("PATCH /api/conversations/{id}",           s.handlePatchConversation)
s.mux.HandleFunc("POST /api/conversations/{id}/restore",    s.handleRestoreConversation)
```

The existing `handleDeleteConversation` stays routed the same way; its implementation switches from hard to soft via the store change.

### 2.12 Ownership check deferred (but hook present)

In each new handler, read `r.Context().Value(authContextKey).(*Session)` if available. If nil → allow (single-user local). If non-nil → verify `session.UserID` owns the conv. For v1, we skip the check entirely (single-user). We leave a one-line TODO marker in each handler referencing the follow-up item. Pattern documented in design, NOT implemented in v1.

## 3. Frontend design

### 3.1 `useWebSocket` query-param preservation

```ts
// src/hooks/useWebSocket.ts — minimal extension
interface UseWebSocketOptions {
  path: string
  onMessage: (data: unknown) => void
  enabled?: boolean
  searchParams?: Record<string, string>   // NEW
}

// Inside connect():
const search = options.searchParams
  ? '?' + new URLSearchParams(options.searchParams).toString()
  : ''
const ws = createWebSocket(path + search)
```

The `searchParams` object is captured by the `useCallback(connect, [...])` closure; on reconnect the same values are sent. If the consumer wants to *change* the params mid-session, they can `disconnect()` and re-render with new params.

### 3.2 `useInfiniteConversationMessages`

```ts
// src/hooks/useInfiniteConversationMessages.ts
import { useInfiniteQuery } from '@tanstack/react-query'

interface MessagePage {
  messages: ChatMessage[]
  oldest_index: number
  has_more: boolean
}

export function useInfiniteConversationMessages(convID: string | null) {
  return useInfiniteQuery<MessagePage>({
    queryKey: ['conversation-messages', convID],
    enabled: !!convID,
    initialPageParam: null as number | null,
    queryFn: async ({ pageParam }) => {
      const params = new URLSearchParams({ limit: '50' })
      if (pageParam !== null) params.set('before', String(pageParam))
      const res = await fetch(`/api/conversations/${convID}/messages?${params}`)
      if (!res.ok) throw new Error(`messages fetch ${res.status}`)
      return res.json()
    },
    getNextPageParam: (lastPage) => lastPage.has_more ? lastPage.oldest_index : undefined,
  })
}
```

Pages arrive newest-first. The consumer flattens `data.pages.flatMap(p => p.messages).reverse()` OR uses page-order directly and prepends in the DOM. Up to the caller.

### 3.3 `useResumeSession`

```ts
// src/hooks/useResumeSession.ts
import { useMemo } from 'react'
import { useInfiniteConversationMessages } from './useInfiniteConversationMessages'

export function useResumeSession(convID: string | null, send: (data: unknown) => boolean) {
  const query = useInfiniteConversationMessages(convID)

  const canContinue = useMemo(() => {
    if (!query.data) return false
    // Pages are newest-first; first page's last message is the most recent.
    const firstPage = query.data.pages[0]
    if (!firstPage || firstPage.messages.length === 0) return false
    const last = firstPage.messages[firstPage.messages.length - 1]
    return last.role === 'user'
  }, [query.data])

  const continueTurn = () => send({ type: 'continue_turn' })

  return {
    messages: query.data,
    isLoading: query.isLoading,
    hasOlder: query.hasNextPage ?? false,
    loadOlder: query.fetchNextPage,
    canContinue,
    continueTurn,
  }
}
```

### 3.4 Component composition — `LiminalCard` base extracted

To honor the "share primitives" intention of the plan without forcing a full refactor of `MemoryCard`, extract one small shell component:

```tsx
// src/components/liminal/LiminalCard.tsx — NEW
type Density = 'sparse' | 'normal' | 'dense'
interface LiminalCardProps {
  density: Density
  accentVar: string           // CSS var for the left accent
  onClick?: () => void
  onHoverChange?: (h: boolean) => void
  children: ReactNode
  className?: string
}
export function LiminalCard({ density, accentVar, onClick, children, ... }) {
  // Owns: border-left, hover transform, density padding math, click target
  // Does NOT own: content layout, action buttons, domain-specific rendering
}
```

`MemoryCard` is **NOT** refactored in v1 — the risk of breaking MemoryPage outweighs the consistency gain. `ConversationCard` uses `LiminalCard` directly. Follow-up (outside this SDD) refactors MemoryCard to also use it.

`ConversationCard` owns: title display with italics-for-auto-derived, preview truncation to 2 lines (CSS), channel pill, relative-time label, delete action with stopPropagation.

### 3.5 `TimeClusterHeader`

Trivial component:

```tsx
// src/components/liminal/conversations/TimeClusterHeader.tsx
type TimeBucket = 'today' | 'thisWeek' | 'thisMonth' | 'lastMonths' | 'older'
const LABELS: Record<TimeBucket, string> = {
  today: 'Hoy',
  thisWeek: 'Esta semana',
  thisMonth: 'Este mes',
  lastMonths: 'Últimos meses',
  older: 'Más antiguas',
}
interface Props { bucket: TimeBucket; count: number }
export function TimeClusterHeader({ bucket, count }: Props) {
  return (
    <div className="cluster-header /* Liminal styling */">
      <span className="cluster-label">{LABELS[bucket]}</span>
      <span className="cluster-count">{count}</span>
    </div>
  )
}
```

Visually equivalent to `ClusterHeader.tsx` (memory side). Not shared because the memory one types `cluster: Cluster` (domain enum). Generalizing the memory one is refactor scope.

### 3.6 `bucketForTimestamp` pure helper

```ts
// src/utils/timeBuckets.ts
export type TimeBucket = 'today' | 'thisWeek' | 'thisMonth' | 'lastMonths' | 'older'
const HOUR = 1000 * 60 * 60
const DAY = 24 * HOUR

export function bucketForTimestamp(updatedAt: Date, now: Date): TimeBucket {
  const diff = now.getTime() - updatedAt.getTime()
  if (diff <= DAY)        return 'today'      // future → today too (skew tolerance)
  if (diff <= 7 * DAY)    return 'thisWeek'
  if (diff <= 30 * DAY)   return 'thisMonth'
  if (diff <= 90 * DAY)   return 'lastMonths'
  return 'older'
}
```

Pure, trivially testable via a table.

### 3.7 Routing

Existing `/conversations` and `/conversations/:id` routes remain. Ensure `/chat` accepts `?conversation_id=` as a search param — no route config change, just a `useSearchParams` read in `ChatPage`.

### 3.8 Toast

Search the codebase for existing toast infra before inventing:

```sh
grep -rn "toast\|<Toast" /home/marxdr/workspace/daimon-frontend/src
```

- If an existing library is in use → extend with undo semantics.
- If not → add a minimal `Toast` component + `useToast` hook with: `showToast({ message, actionLabel, durationMs, onAction, onExpire })`. Store in component-level state or a React context (tiny surface).

The apply phase picks based on what it finds. Spec requires the behavior regardless of implementation detail.

## 4. Testing approach (Strict TDD alignment)

Strict TDD order per Phase 4 tasks:

**Backend**:
1. Write test for migration v14 upgrade + idempotency → run → fail → implement → pass
2. Tests for `DeleteConversation` soft, `RestoreConversation`, filter-on-read → implement
3. Tests for `GetConversationMessages` cursor behavior (4 scenarios from spec) → implement
4. Tests for `ConversationPruner` with FakeClock (4 scenarios) → implement
5. Tests for `TitleGenerator`: enqueue-nonblocking, worker-executes, timeout-silent, empty-response-dropped, conv-deleted-during-run → implement
6. Tests for `processMessage` convID resolution (3 scenarios) → implement field + resolution
7. Tests for WS upgrade reading conversation_id (3 scenarios) → implement
8. Tests for REST endpoints (PATCH title validation, POST restore, GET messages pagination) → implement

**Frontend**:
1. Unit test `bucketForTimestamp` (boundary + skew) → implement
2. Component test `TimeClusterHeader` → implement
3. Component test `ConversationCard` (title fallback, click behavior) → implement
4. Hook test `useInfiniteConversationMessages` with msw-mocked API → implement
5. Hook test `useResumeSession` (canContinue cases) → implement
6. Hook test `useWebSocket` searchParams preservation on reconnect → implement
7. Page test `ConversationsPage` renders clusters correctly + undo toast flow → implement
8. Page test `ConversationDetailPage` load-more prepend + rename → implement
9. Page test `ChatPage` reads conversation_id from URL, shows continuar button when appropriate → implement

**Integration-ish**:
10. End-to-end Go test: upgrade WS with `?conversation_id=`, send msg, verify inbox gets `IncomingMessage.ConversationID` populated → implement

## 5. Rollout order (respects the phase graph from exploration)

1. **Phase 4 (soft delete + pruner)** backend-only landable in isolation. No UX visible yet, but ready.
2. **Phase 3 (Resume WS + `useResumeSession`)** can develop in parallel after the `IncomingMessage.ConversationID` field ships in Phase 4's PR. Ships when ready, even without UI redesign.
3. **Phase 1 (ConversationsPage Liminal)** frontend-heavy, consumes the updated list endpoint (which filters soft-deleted).
4. **Phase 2 (ConversationDetailPage Liminal + pagination + rename)** frontend-heavy, consumes `GetConversationMessages` + PATCH title endpoints.
5. **Phase 5 (LLM title async)** last — it depends on the hook being wired but the hook is a noop when titler=nil, so we can ship it even after frontend changes land.

Each phase can be its own commit (or small group of commits). Strict TDD means tests land with implementation, not after.

## 6. Open implementation questions (for apply phase)

- **Q6.1**: Does the `cmd/daimon/web_cmd.go` path construct the pruner + titler? If not, does that path hit the conversations table at all? Quick verification at apply time.
- **Q6.2**: Toast infra — use existing or build? Confirm at apply time via grep.
- **Q6.3**: Bucket label language — Spanish per spec (user's preference in this project). Sanity check against other UI copy in the codebase to stay coherent.
- **Q6.4**: When `LiminalCard` is extracted, should `MemoryCard` be refactored opportunistically? Decision: **NO** in this SDD — add a TODO comment on `MemoryCard.tsx` pointing to the extracted shell. Refactor is follow-up.
- **Q6.5**: For the paginated messages endpoint, should error responses include a machine-readable error code (e.g., `{"error": "not_found"}`) beyond HTTP status? Convention check: inspect one existing handler for style, match it.

## 7. Success definition

Passes `go test ./... -race`, `golangci-lint run`, `go vet ./...`, `vitest run`, `tsc --noEmit`. All acceptance criteria from the proposal §8 demonstrable manually. Specs §33 requirements / §75 scenarios all covered by tests.
