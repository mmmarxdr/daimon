# Design: Subagents (Spawnable Specialized Agent Loops)

**Change**: `subagents`
**Date**: 2026-05-10
**Author**: sdd-design (Opus 4.7)
**Status**: Draft
**artifact_store**: hybrid

> **Schema-version reconciliation (binding)**: the proposal/exploration referred to migrations as "v11" / "v12". The current head of `internal/store/migration.go` is already at **v15** (v11 added `memory.cluster`, v12 added RAG trust columns, v13/v14/v15 are in production). The two new migrations introduced by this change MUST therefore be implemented as **v16** (conversations: `parent_conv_id`, `status`) and **v17** (cost_records: `conv_id`, `parent_conv_id`, `attribution_kind`). All references in this design use v16 / v17. The proposal text remains historically accurate as written; tasks and apply must use v16/v17.

---

## 1. Architecture Overview

### 1.1 Component Diagram

```
                            ┌──────────────────────────────────────┐
                            │            Parent Agent              │
                            │ (config, sem, inbox, ctx, provider)  │
                            │                                      │
              user msg ──▶  │  processMessage()  ─── for/select ─┐ │
                            │     │                              │ │
                            │     ▼                              │ │
                            │  buildToolDefs() ◀── a.tools (RWmu)│ │
                            │     │                              │ │
                            │     ▼                              │ │
                            │  LLM Chat ──▶ tool_calls           │ │
                            │     │                              │ │
                            │     ├──▶ regular Tool.Execute()    │ │
                            │     │                              │ │
                            │     └──▶ SubagentSpawnTool.Execute │ │
                            │             │                      │ │
                            └─────────────┼──────────────────────┘ │
                                          │                        │
                                          ▼                        │
                            ┌──────────────────────────────────────┐
                            │          SubagentManager             │
                            │  - parentAgent   *Agent              │
                            │  - subs          map[id]*subRecord   │
                            │  - bus           notify.Bus          │
                            │  - store         store.Store         │
                            │  - clock         func() time.Time    │
                            │  Spawn() / Cancel() / Active()       │
                            └──────────┬───────────────────────────┘
                                       │ creates per spawn
            ┌──────────────────────────┴──────────────────────────┐
            ▼                          ▼                          ▼
   ┌──────────────────┐    ┌────────────────────┐    ┌─────────────────────┐
   │  child Agent     │    │  SubagentChannel   │    │  budgetMonitor()    │
   │  (own ctx, sem,  │◀──▶│  (headless,        │    │  (goroutine; reads  │
   │   inbox, store,  │    │   captures Send)   │    │   bus events for    │
   │   provider)      │    └────────────────────┘    │   turn.completed)   │
   │  go a.Run(subCtx)│                              └──────────┬──────────┘
   └────────┬─────────┘                                         │
            │ writes                                            │
            ▼                                                   │
   ┌──────────────────┐    ┌─────────────────────────┐         │
   │ store.Store      │    │   notify.Bus            │◀────────┘
   │  - conversations │───▶│  EventSubagentSpawned   │
   │    (parent_conv) │    │  EventSubagentCompleted │
   │  - cost_records  │    │  EventSubagentFailed    │
   │    (conv_id,     │    │  EventTurnCompleted     │
   │     parent_id)   │    └────────┬────────────────┘
   └──────────────────┘             │ subscribed by
                                    ▼
                       ┌──────────────────────────────┐
                       │  handler_subagents (web/ws)  │
                       │  GET /api/subagents/active   │
                       │  WS subagent_panel feed      │
                       └──────────────────────────────┘
```

### 1.2 Sequence Diagram — Spawn → Run → Budget Hit → Cancel → Result

```
Parent.processMessage   SubagentSpawnTool   SubagentManager   ChildAgent     notify.Bus    Store
        │                       │                  │                │             │           │
        │── tool_call ─────────▶│                  │                │             │           │
        │                       │── Spawn(prompt) ▶│                │             │           │
        │                       │                  │── new conv ───────────────────────────▶ │
        │                       │                  │   (parent_conv_id=parent.ID,            │
        │                       │                  │    status='running')                    │
        │                       │                  │── New(child cfg, subChannel) ──▶ Agent  │
        │                       │                  │── go childAgent.Run(subCtx) ──▶│        │
        │                       │                  │── start budgetMonitor() goroutine        │
        │                       │                  │── Emit(EventSubagentSpawned) ─▶│        │
        │                       │                  │                │                │        │
        │                       │                  │   inject prompt into child.inbox via    │
        │                       │                  │   SubagentChannel.deliver(prompt)       │
        │                       │                  │                │                │        │
        │                       │  (sync mode)     │                │                │        │
        │                       │── handle.wait() ▶│                │                │        │
        │                       │     blocks       │                │                │        │
        │                       │                  │                │ turn N done    │        │
        │                       │                  │                │── RecordCost ──────────▶│
        │                       │                  │                │── Emit(turn.completed)─▶│
        │                       │                  │◀── event ──────────────────────│        │
        │                       │                  │  budgetMonitor: cost > cap →             │
        │                       │                  │  subCancel()                              │
        │                       │                  │── Emit(EventSubagentFailed,              │
        │                       │                  │        reason="budget_exceeded") ──────▶ │
        │                       │                  │── SetConversationStatus(child,'failed')─▶│
        │                       │                  │                │ ctx canceled            │
        │                       │                  │                │ Run returns ctx.Err     │
        │                       │                  │── handle.result = {failed, cost,...}    │
        │                       │◀── result ───────│                                          │
        │◀── tool_result(json) ─│                                                             │
        │  parent loop continues with synthetic tool_result message in conv.Messages         │
```

For async mode the only difference is: `Spawn()` returns immediately with `{handle_id}`; the parent's `tool_result` payload is just `{"handle_id": "...", "status": "running"}`. The parent polls `Active()` (via prose / future tool) or subscribes via the WS feed.

---

## 2. Component Specifications

### 2.1 `SubagentManager` — `internal/agent/subagent_manager.go`

Single owner of spawn lifecycle, budget polling, status tracking, and ctx cascade.

```go
package agent

type subRecord struct {
    id           string                  // UUID
    batchID      string                  // UUID per spawn group (V1: same as id; V2: real batch)
    skillName    string                  // "researcher"
    convID       string                  // child conversation ID
    parentConvID string                  // parent's conv ID
    parentSubID  string                  // empty for root spawns; set when caller is itself a sub (rejected in V1)

    agent        *Agent                  // child Agent instance
    subChannel   *channel.SubagentChannel
    ctx          context.Context         // child of parent ctx, cancellable
    cancel       context.CancelFunc

    budget       BudgetConfig
    cost         float64                 // cumulative USD (read+write under mu)
    turns        int                     // completed turns (read+write under mu)
    softWarned   bool                    // 80% warning fired once

    status       string                  // "running" | "completed" | "failed" | "cancelled"
    failReason   string                  // populated on failure
    result       *SubagentResult         // populated on completion
    spawnedAt    time.Time
    completedAt  *time.Time

    done         chan struct{}           // closed when finished
    mu           sync.Mutex              // guards cost, turns, status, softWarned
}

type BudgetConfig struct {
    MaxCostUSD float64
    MaxTurns   int
    Timeout    time.Duration
}

type SpawnMode string
const (
    SpawnModeSync  SpawnMode = "sync"
    SpawnModeAsync SpawnMode = "async"
)

type SubagentResult struct {
    Status    string            `json:"status"`              // "completed" | "failed" | "cancelled"
    Summary   string            `json:"summary"`             // final assistant text
    Artifacts map[string]string `json:"artifacts,omitempty"` // optional KV (V1 unused; reserved EP-4)
    Cost      float64           `json:"cost_usd"`
    Turns     int               `json:"turns"`
    Errors    []string          `json:"errors,omitempty"`
    Metadata  map[string]string `json:"metadata,omitempty"`  // includes subagent_id, batch_id
}

type SubagentStatus struct {
    ID, BatchID, SkillName, ConvID, ParentConvID string
    Status                                       string
    Cost                                         float64
    Turns                                        int
    SpawnedAt                                    time.Time
    Budget                                       BudgetConfig
}

type SubagentManager struct {
    parent      *Agent
    bus         notify.Bus
    store       store.Store
    cs          store.CostStore  // for CostSummaryForTree polling fallback
    clock       func() time.Time

    mu          sync.RWMutex
    subs        map[string]*subRecord  // id → record
    callerIsSub map[string]bool        // convID → true if it belongs to a sub (depth-1 guard)

    // newChildAgent is the seam tests use to inject a fake Agent.
    newChildAgent func(def ExecutableSkillDef, prompt string, subCtx context.Context, subCh *channel.SubagentChannel, parentTools map[string]tool.Tool, st store.Store) (*Agent, error)
}

// Spawn creates a child Agent, starts it, and returns a handle.
// In sync mode the caller blocks via handle.Wait() in SubagentSpawnTool.Execute.
// In async mode the caller returns immediately.
func (m *SubagentManager) Spawn(ctx context.Context, def ExecutableSkillDef, prompt string, mode SpawnMode, callerConvID string) (*SubagentHandle, error)

// Cancel cancels a single subagent. Idempotent. Does NOT touch the parent.
func (m *SubagentManager) Cancel(subID string) error

// Active returns a snapshot of currently running subagents (status="running").
func (m *SubagentManager) Active() []SubagentStatus

// All returns a snapshot of every record we still hold (running + finished
// retained for short-lived status queries; pruned via reaper goroutine).
func (m *SubagentManager) All() []SubagentStatus

// Get returns the status for a single subagent (for /api/subagents/{id}).
func (m *SubagentManager) Get(subID string) (SubagentStatus, bool)
```

`SubagentHandle` (returned by `Spawn`):

```go
type SubagentHandle struct {
    ID      string
    BatchID string
    rec     *subRecord
}

func (h *SubagentHandle) Wait(ctx context.Context) (*SubagentResult, error) {
    select {
    case <-h.rec.done:
        return h.rec.result, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

func (h *SubagentHandle) Cancel() { h.rec.cancel() }

func (h *SubagentHandle) Status() SubagentStatus { /* snapshot under rec.mu */ }

// Subscribe is V1-stub: returns a channel that receives one SubagentStatus
// every time the bus emits a turn-completed/budget-warning/lifecycle event
// for this sub. V1 implementation is a thin filter over notify.Bus.
func (h *SubagentHandle) Subscribe(ctx context.Context) (<-chan SubagentStatus, error)
```

#### Internal flow inside `Spawn()`

```
1. Reject if callerConvID belongs to a subagent (depth-1 guard):
       m.mu.RLock(); isSub := m.callerIsSub[callerConvID]; m.mu.RUnlock()
       if isSub { return nil, ErrSubagentDepthExceeded }
2. id := uuid.New().String(); batchID := id   // V1: 1:1
3. subCtx, cancel := context.WithTimeout(parentCtx, def.Budget.Timeout)
4. childConvID := "sub_" + id
5. Insert conversation row:
       store.SaveConversation(Conversation{
           ID: childConvID, ChannelID: "subagent",
           ParentConvID: callerConvID, Status: "running",
           Metadata: { subagent_id: id, batch_id: batchID, skill: def.Name },
           CreatedAt: now, UpdatedAt: now,
       })
6. subChannel := channel.NewSubagentChannel(id)
7. childTools := filterParentTools(m.parent.tools, def.ToolsAllowlist)
8. childProvider := provider.NewFromConfig(def.ProviderConfig())
9. childAgent := agent.New(
       def.AgentConfig(), m.parent.limits, m.parent.filterCfg,
       subChannel, childProvider, m.store, m.parent.auditorFn(),
       childTools, []skill.SkillContent{def.SystemPromptAddendum()}, nil,
       1 /*maxConcurrent=1 — one inbox msg per spawn*/, false,
   ).WithBus(m.bus)
10. rec := &subRecord{ ... } ; m.mu.Lock(); m.subs[id]=rec; m.callerIsSub[childConvID]=true; m.mu.Unlock()
11. go childAgent.Run(subCtx)            // child loop
12. go m.budgetMonitor(rec)              // budget poll (subscribes to bus.EventTurnCompleted scoped to childConvID)
13. m.bus.Emit(EventSubagentSpawned, ...)
14. subChannel.Deliver(prompt)           // injects IncomingMessage so the child's for/select fires
15. return &SubagentHandle{...}
```

#### Internal flow inside `budgetMonitor(rec)`

```
1. ch := m.bus.Subscribe-equivalent — V1 uses a per-rec channel populated by
   a single bus subscription installed in NewSubagentManager that fans out by ChannelID.
2. for {
     select {
     case <-rec.ctx.Done():
         m.finalize(rec, "cancelled" or "failed", reasonFromCtx)
         return
     case ev := <-ch:
         if ev.Type != EventTurnCompleted || ev.ChannelID != rec.subChannel.ID() { continue }
         // pull tokens/cost from event Meta (input_tokens, output_tokens)
         // recompute via cost.ComputeCost(model, in, out) — single source of truth
         turnCost := computeCostFromEventMeta(ev, rec.agent.provider.Model())
         rec.mu.Lock()
         rec.cost += turnCost
         rec.turns++
         softHit := !rec.softWarned && rec.cost >= 0.8*rec.budget.MaxCostUSD
         hardCost := rec.cost >= rec.budget.MaxCostUSD
         hardTurns := rec.turns >= rec.budget.MaxTurns
         if softHit { rec.softWarned = true }
         rec.mu.Unlock()
         if softHit { m.injectSoftWarning(rec) }   // see §3
         if hardCost || hardTurns {
             m.finalize(rec, "failed", "budget_exceeded")
             rec.cancel()
             return
         }
         // also check if child loop ended naturally: if rec.subChannel observed a
         // final assistant message, finalize "completed"
         if final := rec.subChannel.FinalAssistant(); final != "" {
             m.finalize(rec, "completed", "")
             return
         }
     }
   }
```

#### Error contract

| Error | When | Surface |
|-------|------|---------|
| `ErrSubagentDepthExceeded` | Spawn called from within a sub | Returned from `Spawn`; tool_result `IsError:true` text: "subagents may not spawn other subagents in this version" |
| `ErrBudgetExceeded` | Cost/turns/timeout breach | `EventSubagentFailed{reason:"budget_exceeded"}`, tool_result `IsError:true` |
| `ctx.Canceled` | Parent ctx cascade | `EventSubagentFailed{reason:"cancelled"}`, tool_result text `"subagent cancelled by parent"` |
| Provider error from child Run | child Agent's `formatProviderError` already shaped | `EventSubagentFailed{reason:"provider_error"}` with `Meta["error"]=msg`, tool_result text wraps the provider message |

#### Concurrency

- `m.mu` (RWMutex): guards `subs` and `callerIsSub`.
- `rec.mu` (Mutex): guards mutable counters/status (cost, turns, status, softWarned).
- Budget monitor and child Agent run in separate goroutines and never write the same field without `rec.mu`.
- The single bus subscription installed by `NewSubagentManager` is the only place that races; it dispatches by `ev.ChannelID == rec.subChannel.ID()` so events for unrelated convs are dropped without contention.

---

### 2.2 `SubagentSpawnTool` — `internal/agent/subagent_tool.go`

One instance per executable skill, registered into `a.tools` at `agent.New()` time.

```go
type SubagentSpawnTool struct {
    def     skill.ExecutableSkillDef
    manager *SubagentManager
}

func (t *SubagentSpawnTool) Name() string        { return t.def.Name }
func (t *SubagentSpawnTool) Description() string { return t.def.Description } // ≤200 chars from frontmatter

func (t *SubagentSpawnTool) Schema() json.RawMessage {
    return json.RawMessage(`{
      "type":"object",
      "properties":{
        "prompt":{"type":"string","description":"Task description for the subagent"},
        "mode":{"type":"string","enum":["sync","async"],"default":"sync",
                "description":"sync blocks parent until subagent finishes; async returns a handle"}
      },
      "required":["prompt"]
    }`)
}

func (t *SubagentSpawnTool) Execute(ctx context.Context, params json.RawMessage) (tool.ToolResult, error) {
    var p struct {
        Prompt string `json:"prompt"`
        Mode   string `json:"mode,omitempty"`
    }
    if err := json.Unmarshal(params, &p); err != nil {
        return tool.ToolResult{IsError:true, Content:"invalid params: "+err.Error()}, nil
    }
    if strings.TrimSpace(p.Prompt) == "" {
        return tool.ToolResult{IsError:true, Content:"prompt is required"}, nil
    }
    mode := SpawnMode(p.Mode)
    if mode == "" { mode = SpawnModeSync }

    callerConvID, _ := tool.ConvIDFromContext(ctx)  // existing helper used by other tools
    handle, err := t.manager.Spawn(ctx, t.def, p.Prompt, mode, callerConvID)
    if err != nil {
        return tool.ToolResult{IsError:true, Content:err.Error()}, nil
    }

    if mode == SpawnModeAsync {
        payload, _ := json.Marshal(map[string]any{
            "handle_id": handle.ID,
            "batch_id":  handle.BatchID,
            "status":    "running",
        })
        return tool.ToolResult{
            Content: string(payload),
            Meta: map[string]string{"subagent_id": handle.ID, "batch_id": handle.BatchID, "mode": "async"},
        }, nil
    }

    // Sync: block on Wait. We bound Wait by the subagent's own context; the parent's
    // tool ctx already carries limits.tool_timeout — Wait returns ctx.Err on parent timeout.
    res, waitErr := handle.Wait(ctx)
    if waitErr != nil {
        return tool.ToolResult{IsError:true, Content:"subagent wait failed: "+waitErr.Error()}, nil
    }
    payload, _ := json.Marshal(res) // SubagentResult JSON satisfies REQ-8
    isError := res.Status != "completed"
    return tool.ToolResult{
        Content: string(payload),
        IsError: isError,
        Meta: map[string]string{
            "subagent_id": handle.ID, "batch_id": handle.BatchID, "mode": "sync",
            "status": res.Status, "cost_usd": fmt.Sprintf("%.4f", res.Cost),
        },
    }, nil
}
```

Why this satisfies REQ-8 (synthetic `tool_result` in parent conv): the parent loop in `loop.go` already wraps every `Tool.Execute` return in a `<tool_result status=...>` CDATA block and appends it to `conv.Messages` with `Role:"tool", ToolCallID:tc.ID`. We do not need a special path. The JSON payload (a `SubagentResult`) becomes the tool_result body verbatim. The principal's next turn sees the digest naturally.

---

### 2.3 `SubagentChannel` — `internal/channel/subagent.go`

Implements `channel.Channel`. Headless: there is no transport. Outbound messages from the child Agent (via `Send`) are captured into an in-memory collector that the manager reads to form `SubagentResult.Summary`. Inbound delivery is via `Deliver(prompt)` which the manager calls once after spawn to seed the child's inbox.

```go
package channel

type SubagentChannel struct {
    id        string  // matches subRecord.subChannel.ID(), used as ChannelID for IncomingMessage
    inbox     chan<- IncomingMessage
    mu        sync.Mutex
    output    []OutgoingMessage  // every Send appended; manager reads on completion
    finalText string             // last non-empty Send text (treated as the "answer")
    closed    bool
}

func NewSubagentChannel(id string) *SubagentChannel { return &SubagentChannel{id: "sub:" + id} }

func (c *SubagentChannel) Name() string { return "subagent" }
func (c *SubagentChannel) ID()   string { return c.id }

func (c *SubagentChannel) Start(ctx context.Context, inbox chan<- IncomingMessage) error {
    c.inbox = inbox
    return nil
}

// Deliver pushes the spawn prompt as the single user message that drives the
// child loop. Called by SubagentManager exactly once per spawn.
func (c *SubagentChannel) Deliver(prompt string) error {
    if c.inbox == nil { return errors.New("subagent channel: not started") }
    select {
    case c.inbox <- IncomingMessage{
        ID: uuid.New().String()[:8], ChannelID: c.id, SenderID: "principal",
        Content: content.Blocks{{Type: content.BlockText, Text: prompt}},
        Timestamp: time.Now(),
    }:
        return nil
    case <-time.After(time.Second):
        return errors.New("subagent inbox blocked")
    }
}

func (c *SubagentChannel) Send(ctx context.Context, msg OutgoingMessage) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    if c.closed { return nil } // post-cancel writes drop silently
    c.output = append(c.output, msg)
    if strings.TrimSpace(msg.Text) != "" { c.finalText = msg.Text }
    return nil
}

func (c *SubagentChannel) Stop() error {
    c.mu.Lock(); defer c.mu.Unlock()
    c.closed = true
    return nil
}

func (c *SubagentChannel) FinalAssistant() string {
    c.mu.Lock(); defer c.mu.Unlock()
    return c.finalText
}

func (c *SubagentChannel) Outputs() []OutgoingMessage {
    c.mu.Lock(); defer c.mu.Unlock()
    out := make([]OutgoingMessage, len(c.output))
    copy(out, c.output)
    return out
}

// Compile-time assertion.
var _ Channel = (*SubagentChannel)(nil)
```

Notes:
- We deliberately do NOT implement `StreamSender` or `TelemetryEmitter`. The child loop's streaming branch falls back automatically to the non-streaming path (already verified by the existing `if streamSender, ok := ...; ok` checks in `loop.go`).
- ~70 lines including imports. Within the proposal's "~50 lines" envelope (proposal counted nominal LOC; actual is slightly higher because of `Outputs()` introspection and the close guard).

---

### 2.4 Store Extensions — `internal/store/`

#### 2.4.1 Migration v16 (conversations) — exact SQL

```sql
ALTER TABLE conversations ADD COLUMN parent_conv_id TEXT;
ALTER TABLE conversations ADD COLUMN status TEXT NOT NULL DEFAULT 'active';

CREATE INDEX IF NOT EXISTS idx_conv_parent
    ON conversations(parent_conv_id)
    WHERE parent_conv_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_conv_status_updated
    ON conversations(status, updated_at);

UPDATE schema_version SET version = 16;
```

Wrap in `tx, _ := s.db.Begin(); defer tx.Rollback()` like every other migrateVN; commit at end. Include the standard `if version < 16` gate in `initSchemaVersioned`.

Backfill: existing rows get `status='active'` automatically via the column default; `parent_conv_id` stays NULL (correct — they have no parent).

#### 2.4.2 Migration v17 (cost_records) — exact SQL

```sql
ALTER TABLE cost_records ADD COLUMN conv_id TEXT;
ALTER TABLE cost_records ADD COLUMN parent_conv_id TEXT;
ALTER TABLE cost_records ADD COLUMN attribution_kind TEXT NOT NULL DEFAULT 'self';

UPDATE cost_records SET conv_id = session_id WHERE conv_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_cost_conv ON cost_records(conv_id);
CREATE INDEX IF NOT EXISTS idx_cost_parent_conv
    ON cost_records(parent_conv_id)
    WHERE parent_conv_id IS NOT NULL;

UPDATE schema_version SET version = 17;
```

#### 2.4.3 Compactor query change

In `internal/store/sqlitestore.go` `ListCompactableConversations` (currently around line 919), the SELECT becomes:

```sql
SELECT id FROM conversations
WHERE deleted_at IS NULL
  AND compacted_at IS NULL
  AND status != 'running'           -- NEW guard
  AND updated_at < ?
ORDER BY updated_at ASC
LIMIT ?
```

`status != 'running'` matches both `'active'` (default for new and pre-existing rows) and `'completed'` / `'failed'` / `'cancelled'`.

#### 2.4.4 Struct field additions

```go
// store.Conversation (additions)
type Conversation struct {
    // ...existing fields...
    ParentConvID string `json:"parent_conv_id,omitempty"`
    Status       string `json:"status,omitempty"` // "active" | "running" | "completed" | "failed" | "cancelled"
}

// store.CostRecord (additions)
type CostRecord struct {
    // ...existing fields...
    ConvID          string `json:"conv_id,omitempty"`
    ParentConvID    string `json:"parent_conv_id,omitempty"`
    AttributionKind string `json:"attribution_kind,omitempty"` // "self" (V1)
}
```

`SaveConversation` SQL must include the new columns in the UPSERT. `LoadConversation` SELECT must read them. `RecordCost` INSERT must write `conv_id`, `parent_conv_id`, `attribution_kind` (V1 always passes `attribution_kind="self"` from the cost-recording call site in `loop.go`).

#### 2.4.5 New Store methods

Added to the `Store` interface (FileStore stubs no-op; only SQLiteStore implements meaningfully):

```go
type Store interface {
    // ... existing ...

    // ListChildConversations returns every conversation whose parent_conv_id == parentConvID,
    // ordered by created_at ASC. Empty slice (not error) when none.
    ListChildConversations(ctx context.Context, parentConvID string) ([]Conversation, error)

    // SetConversationStatus updates conversations.status. Returns ErrNotFound if id missing.
    // Used by SubagentManager: 'running' at spawn, 'completed'/'failed'/'cancelled' at finalize.
    SetConversationStatus(ctx context.Context, convID, status string) error
}
```

Added to the `CostStore` interface:

```go
type CostStore interface {
    // ... existing ...

    // CostSummaryForTree returns aggregated cost for rootConvID and all of its
    // descendant conversations (transitive via parent_conv_id). Used by the
    // metrics endpoint and (optionally) by SubagentManager as a periodic
    // sanity-check fallback when bus events are dropped.
    CostSummaryForTree(ctx context.Context, rootConvID string) (CostSummary, error)
}
```

SQLite implementation uses a recursive CTE:

```sql
WITH RECURSIVE tree(id) AS (
    SELECT id FROM conversations WHERE id = ?
    UNION ALL
    SELECT c.id FROM conversations c
    JOIN tree t ON c.parent_conv_id = t.id
)
SELECT
    COALESCE(SUM(input_tokens),0)  AS in_t,
    COALESCE(SUM(output_tokens),0) AS out_t,
    COALESCE(SUM(total_cost_usd),0) AS total_usd,
    COUNT(*) AS recs
FROM cost_records WHERE conv_id IN (SELECT id FROM tree);
```

Per-model breakdown is a second query GROUPed by `model`. Performance: depth=1 in V1 → CTE expands to 1+N rows, where N is number of subs ever spawned for that root. For V2 (depth>1) the CTE still bounds at total descendants — well within SQLite's recursion limit (default 1000).

Transaction boundary: `CostSummaryForTree` is read-only and runs without an explicit transaction. `SaveConversation`, `SetConversationStatus`, and `RecordCost` already use single-statement implicit transactions.

#### 2.4.6 FileStore behavior

FileStore is the legacy fallback (used in tests and embedded mode). It does not implement `CostStore` nor compaction. For the new `Store` methods:
- `ListChildConversations` → returns `nil, nil` (FileStore has no parent linkage).
- `SetConversationStatus` → loads conv, mutates `Status`, saves. Idempotent.

This keeps the interface honest without fragmenting test setup.

---

### 2.5 Skill Schema — `internal/skill/`

#### 2.5.1 New `SkillContent` fields (additive)

```go
type SkillContent struct {
    Name        string
    Description string
    Prose       string
    Autoload    bool

    // V1 additions (all optional; absent → zero value).
    Version          int               // schema version; default 1 (EP-1)
    Executable       bool              // true → produce ExecutableSkillDef
    Model            string            // provider model identifier
    ProviderName     string            // "anthropic" | "openrouter" | ...
    ProviderConfig   map[string]any    // raw frontmatter sub-block; passed through to provider.NewFromConfig
    SystemAddendum   string            // appended to child's system prompt (separate from Prose)
    ToolsAllowlist   []string          // exact tool names; empty = inherit none
    Budget           BudgetFrontmatter // see below
}

// BudgetFrontmatter is the frontmatter shape; ExecutableSkillDef converts
// to BudgetConfig (time.Duration) post-validation.
type BudgetFrontmatter struct {
    Defaults     bool    `yaml:"-"`               // true when frontmatter says `budget: defaults`
    MaxCostUSD   float64 `yaml:"max_cost_usd"`
    MaxTurns     int     `yaml:"max_turns"`
    TimeoutMin   int     `yaml:"timeout_min"`
}
```

YAML mapping in `parser.go` `frontmatter`:

```go
type frontmatter struct {
    Name           string         `yaml:"name"`
    Description    string         `yaml:"description"`
    Version        int            `yaml:"version"`        // default 1 if absent
    Author         string         `yaml:"author"`
    Autoload       bool           `yaml:"autoload"`
    Executable     bool           `yaml:"executable"`
    Model          string         `yaml:"model"`
    Provider       string         `yaml:"provider"`
    ProviderConfig map[string]any `yaml:"provider_config"`
    SystemAddendum string         `yaml:"system_prompt_addendum"`
    ToolsAllowlist []string       `yaml:"tools_allowlist"`
    Budget         yaml.Node      `yaml:"budget"`         // hand-decoded to support `defaults` shortcut
}
```

Parsing the `budget` block in `parseSkillContent`:

```go
func decodeBudget(node yaml.Node) (BudgetFrontmatter, error) {
    if node.IsZero() {
        // Caller decides whether absence is a load error (executable=true → yes; else no).
        return BudgetFrontmatter{}, nil
    }
    if node.Kind == yaml.ScalarNode && node.Value == "defaults" {
        return BudgetFrontmatter{Defaults: true}, nil
    }
    var b BudgetFrontmatter
    if err := node.Decode(&b); err != nil {
        return b, fmt.Errorf("budget: %w", err)
    }
    return b, nil
}
```

#### 2.5.2 `ExecutableSkillDef` — separate type (NOT a SkillContent extension)

Why separate: SkillContent is used everywhere that injects skills into the system prompt. `ExecutableSkillDef` is consumed only by `agent.New()` to register spawn tools. Keeping them apart prevents accidental re-injection of the addendum / budget into the principal's prompt.

```go
type ExecutableSkillDef struct {
    Name           string
    Description    string
    Version        int
    Model          string
    ProviderName   string
    ProviderConfig map[string]any
    SystemAddendum string
    ToolsAllowlist []string
    Budget         BudgetConfig    // resolved (Duration, defaults applied)
}

// AgentConfig builds a config.AgentConfig overlay for the child Agent.
// Inherits the parent's config but overrides Model and provider-specific fields.
func (d ExecutableSkillDef) AgentConfig(parent config.AgentConfig) config.AgentConfig {
    cfg := parent
    if d.Model != "" { cfg.Model = d.Model }
    cfg.MaxIterations = d.Budget.MaxTurns      // hard cap inside child's loop too (defense in depth)
    return cfg
}

func (d ExecutableSkillDef) ProviderCfg(parent config.ProviderConfig) config.ProviderConfig {
    pc := parent
    if d.ProviderName != "" { pc.Type = d.ProviderName }
    if d.Model != "" { pc.Model = d.Model }
    for k, v := range d.ProviderConfig {
        // shallow merge of free-form options
        pc.Options[k] = v
    }
    return pc
}
```

#### 2.5.3 Loader changes

`LoadSkills` signature gains a third return slice:

```go
func LoadSkills(
    paths []string,
    shellCfg config.ShellToolConfig,
    limits config.LimitsConfig,
) ([]SkillContent, map[string]tool.Tool, []ExecutableSkillDef, []error)
```

For each skill file:

1. Parse frontmatter (now includes new fields).
2. If `executable: false` (or absent), behave exactly as today.
3. If `executable: true`:
   a. Default `Version` to 1 if absent.
   b. Validate budget: if frontmatter has no `budget` key at all → load error `skills: %q: executable skills must declare a budget block (or 'budget: defaults')`.
   c. If `budget: defaults` → expand to floor `{MaxCostUSD: 0.50, MaxTurns: 20, TimeoutMin: 10}`.
   d. Validate `tools_allowlist`: this is a **two-phase validation**. The loader CANNOT cross-check against the parent's tool registry (tools are not yet materialized when LoadSkills runs from cmd/daimon's wiring). The loader stores the names verbatim; **`agent.New()` performs the cross-check after the registry is built** and emits warnings (non-fatal: an unknown allow-list entry is simply dropped from the child's tool map, with a `slog.Warn`). This matches the existing pattern for missing MCP tools and avoids load-order coupling.
   e. The skill's `Prose` is treated as `SystemAddendum` if frontmatter `system_prompt_addendum` is absent; if both are present, `system_prompt_addendum` wins (frontmatter is more explicit).
   f. Append an `ExecutableSkillDef` to the new return slice.
   g. Also still append the `SkillContent` so the principal's system prompt can mention "you may delegate to /researcher".

Backward compat: existing 3-return callers must update to 4-return. Single call site is in `cmd/daimon` (and tests).

#### 2.5.4 `agent.New()` wiring

New parameter (or `WithExecutableSkills(...)` option to keep `New` backward-compat-ish):

```go
func New(
    ...existing params...,
    skills []skill.SkillContent,
    skillIndex skill.SkillIndex,
    execSkills []skill.ExecutableSkillDef,  // NEW
    maxConcurrent int,
    stream bool,
    storeCfg ...config.StoreConfig,
) *Agent
```

After the `a := &Agent{...}` block, before returning:

```go
if len(execSkills) > 0 {
    cs, _ := st.(store.CostStore)
    a.subMgr = NewSubagentManager(a, a.bus, st, cs, time.Now)
    for _, def := range execSkills {
        // Two-phase tool validation: drop unknown entries with a warning.
        def.ToolsAllowlist = filterKnownTools(def.ToolsAllowlist, a.tools)
        spawn := &SubagentSpawnTool{def: def, manager: a.subMgr}
        a.toolsMu.Lock()
        if _, exists := a.tools[def.Name]; exists {
            slog.Warn("subagent tool name collides with existing tool; subagent wins", "name", def.Name)
        }
        a.tools[def.Name] = spawn
        a.toolsMu.Unlock()
    }
}
```

`Agent` struct gains: `subMgr *SubagentManager` field.

---

### 2.6 Notify Events — `internal/notify/events.go`

Add 3 constants and update `KnownEventTypes`:

```go
const (
    EventSubagentSpawned   = "agent.subagent.spawned"
    EventSubagentCompleted = "agent.subagent.completed"
    EventSubagentFailed    = "agent.subagent.failed"
)

var KnownEventTypes = map[string]bool{
    // ... existing ...
    EventSubagentSpawned:   true,
    EventSubagentCompleted: true,
    EventSubagentFailed:    true,
}
```

Payload uses the existing `notify.Event` struct (no new struct). Mapping convention (V1):

| Field | Spawned | Completed | Failed |
|-------|---------|-----------|--------|
| `Type` | `agent.subagent.spawned` | `...completed` | `...failed` |
| `Origin` | `OriginAgent` | `OriginAgent` | `OriginAgent` |
| `ChannelID` | `subRecord.subChannel.ID()` (e.g. `"sub:<uuid>"`) | same | same |
| `Text` | spawn prompt (truncated 200 chars) | result.Summary (truncated 500) | empty |
| `Error` | empty | empty | `failReason` |
| `Meta` | `{subagent_id, batch_id, skill, parent_conv_id, model, max_cost_usd, max_turns, timeout_sec}` | `+ cost_usd, turns, status` | `+ cost_usd, turns, reason` |

Reusing the existing `Event` shape avoids touching every Bus consumer (RulesEngine, web subscribers, cron path).

---

### 2.7 Web/REST handler — `internal/web/handler_subagents.go`

Two endpoints:

```go
// GET /api/subagents/active
//   Returns: {"subagents":[ {id, batch_id, skill, conv_id, parent_conv_id, status,
//                            cost_usd, turns, spawned_at, budget:{...}} ]}
//   Source: agent.SubagentManager.Active()
func (s *Server) handleSubagentsActive(w http.ResponseWriter, r *http.Request)

// GET /api/subagents/{id}
//   Returns single SubagentStatus + final result if completed
func (s *Server) handleSubagentByID(w http.ResponseWriter, r *http.Request)
```

WS feed: extend the existing metrics WS (`handler_ws_metrics.go`) to forward `EventSubagent*` frames to subscribed clients. The handler subscribes to the bus on connect, filters by `Type` prefix `agent.subagent.`, and writes JSON frames `{"type":"subagent.spawned", ...payload}` to the connection.

The Server already holds `*Agent`; expose via `agent.SubagentManager()` accessor returning `a.subMgr` (may be nil when no executable skills are loaded — handler returns `{"subagents":[]}` cleanly in that case).

---

## 3. Cross-Cutting Concerns

### 3.1 Cancellation Hierarchy

```
parentCtx (from agent.Run)
   └─▶ processMessage ctx (passed to SubagentSpawnTool.Execute)
          └─▶ subCtx, subCancel := context.WithTimeout(parentCtx, def.Budget.Timeout)
                 └─▶ child agent.Run(subCtx)
                 └─▶ child's per-turn loopCtx (WithTimeout(subCtx, totalTimeout))
                 └─▶ child's per-tool toolCtx (WithTimeout(loopCtx, toolTimeout))
```

Properties:
- Parent cancel → `parentCtx.Done()` → `subCtx.Done()` → child `Run` returns → all child tool/loop contexts cascade. **No explicit cleanup** in `SubagentManager` is needed for cascade itself; the manager's own goroutine (`budgetMonitor`) observes `rec.ctx.Done()` and runs `finalize`.
- `SubagentManager.Cancel(id)` → calls `rec.cancel()` (the per-spawn `subCancel`) → child cascades, parent untouched.
- Timeout → same `subCancel` fires when `time.WithTimeout` deadline hits.

**Race: parent ctx cancelled during Spawn before child Run starts.** Spawn checks `parentCtx.Err()` after step 5 (conversation insert) and before step 11 (`go childAgent.Run`). If cancelled, set status `"cancelled"`, emit `EventSubagentFailed{reason:"cancelled_during_spawn"}`, return error.

### 3.2 Budget Enforcement Timing

The check fires inside `budgetMonitor` (a goroutine per spawn), driven by `EventTurnCompleted` events on the bus. Concretely:

```go
// inside SubagentManager
func (m *SubagentManager) installBusSubscription() {
    m.bus.Subscribe(func(ev notify.Event) {
        if ev.Type != notify.EventTurnCompleted { return }
        m.mu.RLock()
        rec, ok := m.byChannelID(ev.ChannelID)
        m.mu.RUnlock()
        if !ok { return }
        select {
        case rec.events <- ev:    // bounded chan, dropped if monitor is behind
        default:
            slog.Warn("subagent budget monitor lagging", "id", rec.id)
        }
    })
}
```

The handler MUST return quickly (per `notify.Bus` doc on handler_timeout: 5s) — we forward to `rec.events` (buffered cap 8) and let `budgetMonitor` do the work asynchronously.

### 3.3 Cost Rollup

`CostSummaryForTree` SQL is in §2.4.5. Performance: with depth=1 the CTE expands to 1 root + N children. Even with 50 active subs the query scans <100 rows from `cost_records` per spawn (10–50 rows typical). Index `idx_cost_conv` covers the IN clause.

For V2 (depth > 1) the CTE recursion bound is SQLite's `max_recursion_depth` (default 1000); we never approach this. If V2 grows wide trees, we add a `tree_depth` column and a partial index.

### 3.4 Compactor Guard

§2.4.3 already shows the WHERE-clause edit. Lifecycle:

1. `SubagentManager.Spawn` writes `Conversation{Status:"running"}`.
2. `budgetMonitor.finalize` calls `store.SetConversationStatus(convID, "completed" | "failed" | "cancelled")`.
3. From this point the compactor can pick the conv up after its normal idle window.

Race: if the agent crashes mid-spawn (between insert and finalize), the conv stays `'running'` forever and the compactor never touches it. Mitigation in V1 (acceptable): a **boot-time sweep** in `agent.New()` that sets every conv whose `status='running'` AND `updated_at < now-24h` to `status='cancelled'`. Documented as a known limitation that future versions may surface to the user.

### 3.5 MCP Share-and-Filter

```go
func filterParentTools(parent map[string]tool.Tool, allowlist []string) map[string]tool.Tool {
    out := make(map[string]tool.Tool, len(allowlist))
    for _, name := range allowlist {
        if t, ok := parent[name]; ok {
            out[name] = t
        }
        // unknown names: filterKnownTools already dropped them at agent.New() with a warn
    }
    return out
}
```

The child's `tools` map is a **shallow copy of selected entries** from the parent's map. Because `tool.Tool` is an interface, both maps reference the same `Tool` instances — the underlying MCP client connections are shared (no new subprocess). The child cannot mutate the parent's map. The parent's `toolsMu` is NOT held by the child (childAgent has its own `toolsMu`).

Caveat: if the parent hot-removes an MCP server while a child is running, the child still holds the (now closed) `tool.Tool`. Subsequent calls return errors that propagate naturally up via `Execute`. Acceptable for MVP.

### 3.6 Error Propagation to Parent

The `SubagentResult` JSON serialized into the parent's tool_result already carries `status`, `errors`, and `cost`. We deliberately do NOT include Go stack traces. For provider errors we copy the formatted message from `formatProviderError` (already user-facing). The principal sees something like:

```json
{
  "status": "failed",
  "summary": "",
  "cost_usd": 0.42,
  "turns": 8,
  "errors": ["budget exceeded: max_cost_usd=0.50 reached after 8 turns"],
  "metadata": {"subagent_id": "...", "batch_id": "...", "model": "claude-haiku-4-5"}
}
```

The principal's next LLM turn reads this verbatim through the existing CDATA-wrapped `<tool_result>` wrapper.

---

## 4. Trade-Offs Considered

| Decision | Alternative considered | Why this | Why not the other |
|----------|------------------------|----------|-------------------|
| **Independent `Agent` per spawn** | Reuse parent loop with a "virtual thread" tag on inbox messages | Spec REQ-2 demands independent inbox/sem/ctx; independent provider per profile is impossible without a new Agent. Existing `agent.New` already accepts every dependency we need. | Sharing parent's `sem` blocks the principal whenever a sub holds a slot. Sharing inbox tangles routing and forces every channel to ignore subagent traffic. |
| **Bus-driven budget monitor** | Estimate cost from token counts during streaming and stop mid-turn | Turn-granularity is enough for MVP and avoids invasive provider hooks. `EventTurnCompleted` already exists and carries the data we need. | Mid-turn enforcement requires every provider's streaming path to expose a usage callback — none do today. Adding it is outside the scope of this change. |
| **Headless `SubagentChannel`** | Use a `nil` Channel and add nil-checks in `agent.Run` | Implementing the existing interface is ~70 lines and requires zero changes elsewhere. | Nil-checks would scatter throughout `loop.go` (`if a.channel != nil { a.channel.Send(...) }`) — invasive, prone to regressions, breaks the compile-time `_ Channel = (*WebChannel)(nil)` discipline. |
| **New `subagents` capability spec** | Extend `agent-loop` capability | Subagent lifecycle, budget, attribution, and visibility are a coherent unit; spreading them across `agent-loop` and `output-store` would fracture review. | Requirements would split awkwardly (some in `agent-loop`, some in `output-store`); REQ traceability suffers. |
| **`ExecutableSkillDef` separate from `SkillContent`** | Boolean flag `IsExecutable` on `SkillContent` plus optional fields | Keeps the principal's prompt-injection path untouched. Type system enforces "this is a spawn definition, not prose". | A flag-laden struct invites accidental inclusion of `system_prompt_addendum` in the principal's prompt and bloats every consumer of `SkillContent`. |
| **Migration v16/v17 (NOT v11/v12)** | Stick with the proposal's labels | The repo is already at schema v15; collisions are a non-starter. | The proposal numbers were drafted from an older mental model; correctness wins over historical labelling. |
| **Two-phase `tools_allowlist` validation (load + agent.New)** | Validate fully at load time | At load time the parent's MCP tools are not yet materialized (MCP wiring runs after skill load in `cmd/daimon`). Cross-check has to happen later. | Load-time-only validation would either reject MCP tools (false negatives) or ship without MCP coverage. |
| **Always write `attribution_kind="self"` in V1** | Defer the column until V2 needs it | Schema present + EP-6 satisfied; V2 ships without another migration. | Adding a column later has the same cost as adding it now and forces a v18 migration we already know we'll need. |
| **Recursive CTE for `CostSummaryForTree`** | Materialized rollup table | Depth=1 in V1 means N+1 rows; CTE is O(N) and SQLite optimizes it. | Materialized table doubles the write path on every `RecordCost` and creates consistency risks for negligible read-side benefit at this scale. |
| **Boot-time sweep for orphaned `status='running'` convs** | Heartbeat column + watcher | One-line sweep; aligns with existing boot-time orphan cleanup pattern (`document_chunks` cleanup in `initSchemaVersioned`). | Heartbeats add a write per turn and a separate watcher goroutine — overkill for the rare crash-mid-spawn case. |

---

## 5. Testing Strategy

### 5.1 Per-component unit tests (table-driven)

| File | What it covers |
|------|----------------|
| `internal/skill/parser_executable_test.go` | Frontmatter parsing: executable absent / true / false; budget present / `defaults` shortcut / missing → error; tools_allowlist parsing; version default 1; system_prompt_addendum + Prose interaction. |
| `internal/skill/loader_executable_test.go` | LoadSkills returns ExecutableSkillDef slice; absence of budget on executable=true → load error; non-executable skills produce no def; collision warning when name overlaps. |
| `internal/store/migration_v16_v17_test.go` | Round-trip apply on a fresh DB; apply on a v15 DB (existing rows stay); idempotent re-run. Verify `status` defaults to `'active'`; verify `conv_id = session_id` backfill on existing rows. |
| `internal/store/sqlitestore_subagent_test.go` | `ListChildConversations` returns ordered children; `SetConversationStatus` flips state and is idempotent / errors on missing; `CostSummaryForTree` sums root + children correctly with multiple models. |
| `internal/agent/compactor_status_guard_test.go` | A conv with `status='running'` is NOT returned by `ListCompactableConversations` even when its `updated_at` is past `idleBefore`. Once flipped to `'completed'`, it IS returned. |
| `internal/channel/subagent_test.go` | NewSubagentChannel + Start sets inbox; Deliver pushes one IncomingMessage; Send appends and tracks `finalText`; Stop is idempotent; Outputs returns a defensive copy. |
| `internal/agent/subagent_manager_test.go` | Spawn rejects when caller is itself a sub; Spawn writes parent_conv_id; Cancel is idempotent; budget monitor cancels on `cost > cap`; soft-warning fires once at 80%; Active() returns only `'running'` records. Uses `newChildAgent` test seam. |
| `internal/agent/subagent_tool_test.go` | Schema validates; sync mode returns SubagentResult JSON; async mode returns handle envelope; missing prompt → tool error; wait error propagates without a panic. |

### 5.2 Integration tests

| File | What |
|------|------|
| `internal/agent/subagent_integration_test.go` | End-to-end: load a `testdata/skills/researcher.skill.md`, register via `agent.New`, drive a turn that emits a tool call to `researcher`, assert (a) child Agent ran, (b) parent received a `<tool_result>` containing `SubagentResult{Status:"completed",...}`, (c) `EventSubagentSpawned` and `EventSubagentCompleted` were emitted in order, (d) child conv exists with `parent_conv_id = parent.ID` and `status = 'completed'`. |
| `internal/agent/subagent_cancel_cascade_test.go` | Spawn 3 subs; cancel parent ctx; assert all three `subRecord.done` channels close within 1s and `EventSubagentFailed{reason:"cancelled"}` fired for each. |
| `internal/agent/subagent_budget_test.go` | Synthetic skill with `max_cost_usd:0.0001`; assert exactly one `EventSubagentFailed{reason:"budget_exceeded"}` and the conv is marked `'failed'`. |
| `internal/web/handler_subagents_test.go` | `GET /api/subagents/active` returns the live spawn list; WS feed receives `subagent.spawned` and `subagent.completed` frames in order. |

### 5.3 Test artifacts (`testdata/skills/`)

- `researcher.skill.md` — full executable skill (model: tiny test stub provider, budget: defaults, tools_allowlist: [`shell_exec`]).
- `cheap.skill.md` — budget `max_cost_usd: 0.0001` for budget tests.
- `noallowlist.skill.md` — empty `tools_allowlist` (child has zero tools); used to verify parent's MCP tools are NOT leaked.
- `nonexecutable.skill.md` — pure prose skill (sanity that loader still treats it as before).

The test stub provider (already used elsewhere in `internal/provider/...test.go`) returns deterministic `Usage{InputTokens:N, OutputTokens:M}` so cost computations are predictable.

### 5.4 Strict TDD ordering (project standard)

Every component test above is RED-first:
1. Add the test.
2. Run `go test ./...` — confirm RED.
3. Implement until GREEN.
4. `go vet ./... && golangci-lint run && go test -race ./...`.

---

## 6. Open Design Risks (flagged for orchestrator triage)

1. **Schema-version slot mismatch (HIGH for tasks/apply, none for design)**: the proposal & spec text both reference v11/v12. This design pins v16/v17. Tasks must use v16/v17; spec language can stay capability-level (it never names version numbers in scenarios).
2. **`maxConcurrent=1` for child Agent (LOW)**: every child runs with `sem` cap 1. This means a sub processes exactly one inbox message at a time — fine because we Deliver exactly once. If V2 introduces multi-turn user-driven sub interaction, we need to revisit.
3. **Bus subscriber fan-out latency (MEDIUM)**: the manager installs ONE bus subscriber that fans out to per-rec channels. Under heavy turn-completed traffic from many parallel subs, the single subscriber callback may queue. Mitigation: per-rec buffered channel cap 8 + drop-with-warn. Acceptable; flagged for V2 perf review.
4. **`Conversation.ChannelID = "subagent"` collides with future channels (LOW)**: we set the child conv's `channel_id` to the literal string `"subagent"` (or `subChannel.ID()`). The existing `idx_conv_channel` covers this. No queries today filter on a "subagent" prefix; if a future channel uses the same name, prefix the ID with `sub:` (already done in `SubagentChannel.ID`). Documented.
5. **Title generator fires for sub convs (LOW)**: the post-save title hook runs on every `SaveConversation`, including sub convs. It will enqueue a title job even for ephemeral sub conversations. Two options: (a) exempt convs with non-empty `parent_conv_id`, or (b) let it run (titles are harmless). **Design choice**: exempt them in `shouldGenerateTitle` with an extra check `conv.ParentConvID == ""`. One-line edit; documented in tasks.
6. **`provider.NewFromConfig` may not read all per-sub fields (MEDIUM)**: not every provider type honours every field on `ProviderConfig.Options`. Tasks must verify Anthropic / OpenRouter / Gemini all accept `Model` overrides; if any silently ignore it, the budget check will be against the parent's actual cost. Flag for tasks: add a sanity test `TestProviderModelOverride_AllTypes`.
7. **Audit emission for child Agent (LOW)**: child Agent reuses parent's `auditorFn`. Audit events for child turns will appear under the child's `scope` (which is derived from `userScope` of `subChannel.ID()` + sender `"principal"`). Documented; the dashboard will need a small filter to group by `parent_conv_id` later — not in this change.

---
