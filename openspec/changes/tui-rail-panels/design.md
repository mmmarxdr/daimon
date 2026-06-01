# Design: tui-rail-panels

> Technical design for Phase-2 chat-rail data wiring.
> Consumes the LIVE `tui-backend-seams` contract. TUI-only.
> Source of truth: `proposal.md` (6 decisions, 3-PR plan a→b→c).
> This document FINALIZES every point the proposal left "to spec/design".

---

## 1. Context & Constraints

### What this change does

Wire live backend data into three existing chat-rail panel skeletons and add one
new panel, consuming the four `tui-backend-seams` seams. It is a **data-wiring
change, not a visual restyle**.

### The non-negotiable invariant: `View = pure(Model)`

Every panel's `Render(width, height int) string` reads ONLY cached struct fields.
The full data path is, without exception:

```
notify.Bus.Emit(Event)
  → bus subscriber → evCh (cap 256)
    → pumpEvents tea.Cmd → bubbletea runtime
      → Model.Update → case busEventMsg → handleBusEvent(ev)
        → [accumulate]  copyRailWith → panel.accumulate(ev)        (event carries data)
        → [cmd-refresh] schedule tea.Cmd → RefreshMsg → Update → copyRailWith → panel.setXxx
          → Model.View → rail.Render → panel.Render  (reads cached fields ONLY)
```

No `Render` may read the store, the agent, or the clock. All IO lives inside a
`tea.Cmd` closure (goroutine). All panel mutation goes through `copyRailWith`
(`rail.go:70`) so prior `Model` snapshots are never mutated — the windowed chat
viewport (WU-c, 500-item cap) depends on snapshot immutability.

### Verified ground truth (file:line anchors)

| Fact                                                                              | Anchor                                                                 |
| --------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `contextMeterPanel` struct + `accumulate` + `Render`                              | `rail_panels.go:321-377`                                               |
| `telemetryPanel` struct + `accumulate` + `Render`                                 | `rail_panels.go:76-139`                                                |
| `todolistPanel` cmd-refresh precedent                                             | `rail_panels.go:147-200`, `rail_panels_cmd.go:16-35`                   |
| `fetchTodolist` / `todolistRefreshMsg` precedent                                  | `rail_panels_cmd.go:18,27`                                             |
| `panelID` consts + `panelsFor`                                                    | `panels.go:16-29,34-54`                                                |
| `panelsFor(screenChat)` = `{Todolist, ContextMeter, Telemetry}`                   | `panels.go:39`                                                         |
| `copyRailWith` COW                                                                | `rail.go:70-77`                                                        |
| `rail.Render` stacks `out += s + "\n"`, IGNORES `height`                          | `rail.go:42-57`                                                        |
| `newRail` panel wiring                                                            | `rail.go:26-38`                                                        |
| `handleBusEvent` switch                                                           | `screen_chat.go:148-314`                                               |
| `EventTokensUsage` case (calls `cm.accumulate`/`tp.accumulate`)                   | `screen_chat.go:271-302`                                               |
| `EventToolStart`/`End` cases (call `tp.accumulate`)                               | `screen_chat.go:161-186,188-217`                                       |
| `EventTodolistChanged → fetchTodolist`                                            | `screen_chat.go:304-308`                                               |
| `todolistRefreshMsg` Update case                                                  | `model.go:288-296`                                                     |
| `Model.store store.Store` field                                                   | `model.go:150`                                                         |
| run.go boot `copyRailWith` block (where statics are injected)                     | `run.go:78-98`                                                         |
| `Event.SysToks/MsgToks/ToolToks int` (omitempty)                                  | `notify/bus.go:34-36`                                                  |
| `Event.TokenCount/CostUSD/ToolName/DurationMs/IsError`                            | `notify/bus.go:24-29`                                                  |
| `EventSubagentCompleted` / `EventSubagentFailed` consts (both in KnownEventTypes) | `notify/events.go:38,39,83,84`                                         |
| `EventMemoryChanged` const (in KnownEventTypes)                                   | `notify/events.go:51,90`                                               |
| `store.Store.SearchMemory(ctx, scopeID, query, limit)`                            | `store/store.go:99`                                                    |
| `store.MemoryEntry{ScopeID, Title, Content, Cluster, Source, ...}`                | `store/store.go:61-91`                                                 |
| `atoiSafe(string) int` helper                                                     | `components_breadcrumb.go:89`                                          |
| `panelHeaderWithBadge(label, badge)` / `panelHeaderWithBadgeWidth`                | used at `rail_panels.go:120,184`                                       |
| golden infra: `golden.RequireEqual(t, []byte)` + `-update` flag                   | `golden_test.go:31`, `tui_test.go:3`                                   |
| contract matrix test (screenChat row)                                             | `model_test.go:19-22`                                                  |
| rail wiring test (chat panels registered)                                         | `rail_panels_test.go:323-334` (`TestRail_ChatScreen_PanelsRegistered`) |

> Correction to the launch brief: `TestPanelsFor_ContractMatrix` lives in
> `model_test.go:10-61` (the screenChat row at `:19-22`), NOT in
> `rail_panels_test.go`. The chat-panel registration test is
> `TestRail_ChatScreen_PanelsRegistered` at `rail_panels_test.go:323`
> (the brief's "line 317/329" points at the same test body). Tasks must edit
> BOTH files: the matrix row in `model_test.go` and the registration loop at
> `rail_panels_test.go:329`.

### Charm / style constraints

Charm v1 (`bubbletea`/`lipgloss` v1.1.0). ANSI width via `github.com/charmbracelet/x/ansi`
(`StringWidth` / `Truncate` — never `len()`). All color from centralized `tuiStyles`
(`accent`, `amber`, `dimLabel`, `errStyle`, `label`, `panelHeader`,
`panelHeaderWithBadge`). No inline hex. `wrapPanelBox` (`rail_panels.go:46`) owns
the border + per-line ANSI truncation; inner content width is `width-4`.

---

## 2. Architecture Decisions (per slice)

### ADR-1 (PR-a): context-meter — REPLACE category snapshot + real limit + count rows

**Decision.** Rewrite `contextMeterPanel` to hold the real window `limit`
(threaded once at boot) and the per-category snapshot (`sysToks/msgToks/toolToks`),
REPLACED on every `EventTokensUsage`. Render an aggregate bar plus three **labeled
category rows with token counts** (NOT mini sub-bars). Fall back to the aggregate
`TokenCount` accumulator (sub-bars hidden) when category fields are zero.

**Rationale.** Each `EventTokensUsage` is a snapshot of current window fill
(seams spec: REPLACE semantics, `bus.go:34-36`). Accumulating it would inflate the
bar. The real limit comes from `ContextWindowSize()` — a static, read once
(`agent_accessors.go:56`; `0` ⇒ nil contextMgr ⇒ heuristic). On legacy/`none`
strategy the three fields are `0`, so the panel MUST degrade to the existing
aggregate behavior (seams spec scenario "degrades gracefully on zero category
fields").

**Concrete struct change** (`rail_panels.go:321-338`):

```go
type contextMeterPanel struct {
	styles    tuiStyles
	limit     int  // real window from ContextWindowSize(); 0 ⇒ heuristic fallback
	tokenUsed int  // aggregate current fill (sum of categories, or TokenCount fallback)
	sysToks   int  // REPLACE per EventTokensUsage snapshot
	msgToks   int  // REPLACE
	toolToks  int  // REPLACE
	hasData   bool
}

func newContextMeterPanel(s tuiStyles) *contextMeterPanel { return &contextMeterPanel{styles: s} }

// setLimit threads the real window size once at boot (via copyRailWith in run.go).
func (p *contextMeterPanel) setLimit(n int) { p.limit = n }

func (p *contextMeterPanel) accumulate(ev notify.Event) {
	if ev.Type != notify.EventTokensUsage {
		return
	}
	if ev.SysToks+ev.MsgToks+ev.ToolToks > 0 {
		// REPLACE — snapshot of current window fill.
		p.sysToks = ev.SysToks
		p.msgToks = ev.MsgToks
		p.toolToks = ev.ToolToks
		p.tokenUsed = ev.SysToks + ev.MsgToks + ev.ToolToks
	} else {
		// Legacy / none strategy: no breakdown. Keep aggregate-delta behavior,
		// leave category fields 0 so Render hides sub-bars.
		p.tokenUsed += ev.TokenCount
	}
	p.hasData = true
}
```

> **REPLACE trap (Risk 2).** Category fields use `=`, not `+=`. The aggregate
> fallback branch keeps `+=` because `TokenCount` is a per-turn delta (seams spec
> contrasts `TokenCount`/`CostUSD` deltas vs. category snapshots). A unit test
> sends two category events and asserts the SECOND wins.

**Render** (`rail_panels.go:341-377`). Resolve the limit locally and label it:

```go
limit := p.limit
label := "of " + humanK(limit)            // e.g. "of 200k"
if limit == 0 {
	limit = 200_000
	label = "of 200k est."                // heuristic sentinel — suffix "est."
}
```

- Always render: `panelHeader("context")`, the aggregate bar `[████░░░░]` computed
  from `p.tokenUsed / limit` (clamp ≤ 1.0, reuse the existing fill algorithm
  `rail_panels.go:360-365`), and the pct line `"%.1f%% <label>"`.
- When `p.sysToks > 0` (smart strategy ran): append three **labeled count rows**
  under the bar, each `dimLabel`-styled and ANSI-truncated to `inner`:

  ```
  context                 ← panelHeader
  [██████░░░░░░░░░░]       ← aggregate bar
  64.0% of 200k           ← pct line
  sys   1.5k              ← category rows (dimLabel), only when sysToks>0
  msg   4.2k
  tool  0.8k
  ```

  Counts use the same `humanK` short form. Numeric counts (not mini-bars) chosen
  because the rail is only `railWidth-4 = 28` columns — three stacked mini-bars
  would each be < 10 cells and visually meaningless, and three labeled rows are
  trivially golden-testable and unambiguous. **This finalizes the proposal's open
  "3 mini-bars vs. segmented bar" question → labeled count rows.**

- When `p.sysToks == 0`: render ONLY header + aggregate bar + pct line (today's
  shape), no category rows, no `0%` artifact.

Add a tiny pure helper `humanK(n int) string` in `rail_panels.go` (e.g. `1500 → "1.5k"`,
`200000 → "200k"`, `< 1000 → "%d"`). Pure, no clock/IO.

**Boot wiring** (`run.go`, inside the existing `copyRailWith` block at `run.go:78-98`):

```go
ctxLimit := ag.ContextWindowSize() // static; read ONCE
...
r = copyRailWith(r, func(panels map[panelID]Panel) {
	...existing modelPicker/environment/resumeList/activePolicy entries...
	if cm, ok := panels[panelContextMeter].(*contextMeterPanel); ok {
		cp := *cm
		cp.setLimit(ctxLimit)
		panels[panelContextMeter] = &cp
	}
})
```

Mirrors how `panelModelPicker` / `panelActivePolicy` are already replaced in that
same block (`run.go:79,97`). COW value-copy `cp := *cm`.

**No new `handleBusEvent` case.** The existing `EventTokensUsage` block already
calls `cm.accumulate(ev)` through `copyRailWith` (`screen_chat.go:297-301`). PR-a
touches only the struct, `run.go`, and tests.

---

### ADR-2 (PR-b): telemetry — per-tool accumulator + per-subagent rows

**Decision.** Add two maps to `telemetryPanel`: `toolStats map[string]toolStat`
(ACCUMULATE from `EventToolStart`/`End`) and `subagentStats map[string]subagentStat`
(live ACCUMULATE bucket from `EventTokensUsage[subagent_id]`, authoritative REPLACE
from a NEW `EventSubagentCompleted` case). Render aggregate lines first, then
per-tool rows (cap 5 + "+N more"), then per-subagent rows (cap 3). `EventSubagentFailed`
marks the row failed (lightweight `failed` flag) rather than no-op.

**Rationale.** Tool events carry their own data ⇒ accumulate (no fetch). Subagent
live tokens accumulate per turn; `EventSubagentCompleted.Meta["tokens"]` is the
authoritative running total (seams spec guarantees `"0"` minimum) ⇒ REPLACE on
completion. Showing a `failed` marker (vs. silent no-op) is nearly free and gives
the operator signal that a subagent died; it still "completes" the row so the live
bucket stops looking in-flight. **This finalizes the proposal's open
`EventSubagentFailed` question → failed marker (`done=true, failed=true`).**

**Concrete struct change** (`rail_panels.go:76-106`):

```go
type toolStat struct {
	calls      int
	errors     int
	durationMs int64
}

type subagentStat struct {
	tokens int
	done   bool
	failed bool   // set by EventSubagentFailed; renders a ✗ marker
}

type telemetryPanel struct {
	styles     tuiStyles
	totalIn    int
	totalCost  float64
	toolCalls  int
	toolErrors int
	hasData    bool
	toolStats     map[string]toolStat     // per-tool, ACCUMULATE; nil-lazy-init
	subagentStats map[string]subagentStat // per-subagent, live bucket + authoritative total
	saOrder       []string                // first-seen order of subagent IDs (stable render)
}
```

Maps are lazily allocated inside `accumulate` (`if p.toolStats == nil { ... }`) so
the zero-value panel and `newTelemetryPanel` stay valid.

```go
func (p *telemetryPanel) accumulate(ev notify.Event) {
	switch ev.Type {
	case notify.EventTokensUsage:
		p.totalIn += ev.TokenCount
		p.totalCost += ev.CostUSD
		p.hasData = true
		// Live per-subagent bucket (ACCUMULATE) when this event is a child agent's.
		if id := ev.Meta["subagent_id"]; id != "" {
			if p.subagentStats == nil { p.subagentStats = map[string]subagentStat{} }
			st, seen := p.subagentStats[id]
			if !seen { p.saOrder = append(p.saOrder, id) }
			// Don't clobber an authoritative completed total with late live events.
			if !st.done {
				st.tokens += atoiSafe(ev.Meta["input_tokens"]) + atoiSafe(ev.Meta["output_tokens"])
			}
			p.subagentStats[id] = st
		}
	case notify.EventToolStart:
		p.toolCalls++
		if p.toolStats == nil { p.toolStats = map[string]toolStat{} }
		st := p.toolStats[ev.ToolName]; st.calls++; p.toolStats[ev.ToolName] = st
	case notify.EventToolEnd:
		if p.toolStats == nil { p.toolStats = map[string]toolStat{} }
		st := p.toolStats[ev.ToolName]
		st.durationMs += ev.DurationMs
		if ev.IsError { st.errors++; p.toolErrors++ }
		p.toolStats[ev.ToolName] = st
	case notify.EventSubagentCompleted:
		if p.subagentStats == nil { p.subagentStats = map[string]subagentStat{} }
		id := ev.Meta["subagent_id"]
		st, seen := p.subagentStats[id]
		if !seen { p.saOrder = append(p.saOrder, id) }
		st.tokens = atoiSafe(ev.Meta["tokens"]) // REPLACE — authoritative
		st.done = true
		p.subagentStats[id] = st
	case notify.EventSubagentFailed:
		if p.subagentStats == nil { p.subagentStats = map[string]subagentStat{} }
		id := ev.Meta["subagent_id"]
		st, seen := p.subagentStats[id]
		if !seen { p.saOrder = append(p.saOrder, id) }
		st.done = true; st.failed = true
		p.subagentStats[id] = st
	}
}
```

> **COW with maps (Risk 5).** `cp := *tp` shallow-copies the struct, but the map
> header is shared. The `accumulate` calls in `handleBusEvent` already do
> `cp := *tp; cp.accumulate(ev)` — so a copy mutating `cp.toolStats` mutates the
> SAME backing map as the prior snapshot. **This is the one real COW hazard in the
> change.** Mitigation (mandated for tasks): `accumulate` (or a `clone()` step in
> the copyRailWith closure) MUST deep-copy the maps before mutating. Cleanest:
> have `accumulate` rebuild the map it touches:
>
> ```go
> // inside accumulate, before mutating toolStats:
> p.toolStats = cloneToolStats(p.toolStats) // returns a fresh map copy
> ```
>
> with `cloneToolStats` / `cloneSubagentStats` (+ a copied `saOrder` slice) helpers.
> Because `accumulate` runs on the COPY (`cp`) inside the closure, cloning there
> keeps the prior snapshot's maps untouched. The todolist precedent had no maps
> (`setList` replaces a value `tool.TodoList`), so this is NEW discipline for PR-b
> and MUST be golden/again-asserted: a test that retains the pre-event panel
> pointer and asserts its map is unchanged after a second event on the copy.

**Render** (`rail_panels.go:109-139`). Keep the four aggregate lines, then:

- **Per-tool rows**, capped 5, ordered **by call count desc, ties broken by tool
  name asc** (deterministic for goldens). Each row: `"<tool>  <calls>×  <errN?>"`,
  e.g. `Read   12×`, with an error suffix in `errStyle` when `errors>0`. When
  `len(toolStats) > 5`, append a final `dimLabel` line `"+N more"`.
  **This finalizes the proposal's open tool-row "sort order" question → count desc,
  name asc.**
- **Per-subagent rows**, capped 3, ordered by **first-seen (`saOrder`)** so a
  completing subagent does not reorder the list mid-session (stable, golden-safe).
  Each row: truncated 8-rune ID + `humanK(tokens)` + a status marker — `✓` (accent)
  when `done && !failed`, `✗` (errStyle) when `failed`, `●` (amber) while live.
  When `len > 3`, append `"+N more"`.

  > Subagent ordering uses first-seen (insertion) rather than tokens-desc on
  > purpose: tokens change every turn, so tokens-desc would make rows jump around
  > and break golden stability. **This finalizes the proposal's open subagent
  > "sort order" question → first-seen.**

**New `handleBusEvent` cases** (`screen_chat.go`, after the `EventTokensUsage`
block ~`:302`):

```go
case notify.EventSubagentCompleted, notify.EventSubagentFailed:
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		if tp, ok := panels[panelTelemetry].(*telemetryPanel); ok {
			cp := *tp
			cp.accumulate(ev)        // clones the map internally (see Risk 5)
			panels[panelTelemetry] = &cp
		}
	})
```

The live per-subagent bucket needs NO new case: it rides the existing
`EventTokensUsage` `copyRailWith` block (`screen_chat.go:291-302`) because that
block already calls `tp.accumulate(ev)`; the new `subagent_id` branch lives inside
`accumulate`.

---

### ADR-3 (PR-c): memory-peek — NEW 4th panel, cmd-refresh mirror of todolist

**Decision.** Build a new `memoryPeekPanel` end-to-end mirroring the todolist
cmd-refresh precedent EXACTLY, triggered by `EventMemoryChanged`. Placed LAST in
`panelsFor(screenChat)`. Each row shows the entry **Title** (fallback truncated
**Content**). scopeID resolution = option (a): start empty, fetch using
`ev.Meta["scope_id"]` on the first `EventMemoryChanged`.

**Rationale.** `EventMemoryChanged` is a bare signal (seams spec: `Meta` carries
`scope_id`/`entry_id`/`title`/`cluster`); the data must be re-fetched via
`store.SearchMemory` ⇒ cmd-refresh pattern, exactly like
`EventTodolistChanged → fetchTodolist → todolistRefreshMsg`. Title is the human
label the Curator assigns; Content can be a long paragraph, so Title-first with a
Content fallback gives the densest, most legible single-line row. **This finalizes
the proposal's open "Title vs Content" question → Title, Content fallback.**
scopeID option (a) is the only choice that needs no backend surface (option b
`Agent.ScopeID()` is forbidden post-archive; option c `activeConvID` queries the
wrong key — `MemoryEntry.ScopeID ≠ MemoryEntry.Source`, `store.go:63,69`).

**1. `panels.go`** — new const + matrix entry:

```go
panelMemoryPeek panelID = "memory-peek" // chat
...
case screenChat:
	return []panelID{panelTodolist, panelContextMeter, panelTelemetry, panelMemoryPeek}
```

**2. `rail_panels.go`** — new struct (mirrors `todolistPanel`):

```go
type memoryPeekPanel struct {
	styles  tuiStyles
	entries []store.MemoryEntry
}
func newMemoryPeekPanel(s tuiStyles) *memoryPeekPanel { return &memoryPeekPanel{styles: s} }
func (p *memoryPeekPanel) setEntries(entries []store.MemoryEntry) { p.entries = entries }

func (p *memoryPeekPanel) Render(width, _ int) string {
	if len(p.entries) == 0 { return "" }   // zero-height until first memory write
	inner := width - 4; if inner < 4 { inner = 4 }
	rows := []string{ ansi.Truncate(p.styles.panelHeaderWithBadge("memory", "live"), inner, "…") }
	const maxRows = 5
	entries := p.entries
	if len(entries) > maxRows { entries = entries[:maxRows] }
	for _, e := range entries {
		text := e.Title
		if text == "" { text = e.Content }     // Content fallback
		rows = append(rows, ansi.Truncate(p.styles.dimLabel.Render("• "+text), inner, "…"))
	}
	return wrapPanelBox(strings.Join(rows, "\n"), width, p.styles)
}
```

`store` is already imported in `rail_panels.go` (line 30). Title-first, Content
fallback; `wrapPanelBox` + `ansi.Truncate` handle width — no manual byte slicing.

**3. `rail_panels_cmd.go`** — new msg + cmd (mirror `fetchTodolist` EXACTLY,
including the nil/empty no-op):

```go
type memoryRefreshMsg struct { entries []store.MemoryEntry }

func fetchMemory(st store.Store, scopeID string) tea.Cmd {
	return func() tea.Msg {
		if st == nil || scopeID == "" { return memoryRefreshMsg{} }
		entries, _ := st.SearchMemory(context.Background(), scopeID, "", 5) // err ignored; stale data shown
		return memoryRefreshMsg{entries: entries}
	}
}
```

Requires adding imports `context` and `daimon/internal/store` to
`rail_panels_cmd.go` (currently imports only `tea`, `agent`, `tool`). `limit=5`
matches the render cap. `query=""` returns the scope's most-recent entries
(matches `resumeListPanel`'s "recent N" idiom).

**4. `screen_chat.go` handleBusEvent** — new case (mirror `EventTodolistChanged`
at `:304-308`):

```go
case notify.EventMemoryChanged:
	cmds = append(cmds, fetchMemory(m.store, ev.Meta["scope_id"]))
```

**5. `model.go` Update** — new case (mirror `todolistRefreshMsg` at `:288-296`):

```go
case memoryRefreshMsg:
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		if mp, ok := panels[panelMemoryPeek].(*memoryPeekPanel); ok {
			cp := *mp
			cp.setEntries(msg.entries)
			panels[panelMemoryPeek] = &cp
		}
	})
	return m, nil
```

> COW note: `setEntries` REPLACES the slice header with the freshly-fetched slice
> (a brand-new slice from `SearchMemory`), so no aliasing with a prior snapshot —
> same shape as `todolistPanel.setList`. Safe.

**6. `rail.go` newRail** — add the entry:

```go
panelMemoryPeek: newMemoryPeekPanel(s),
```

**7. Contract tests (deliberate break-then-fix).**

- `model_test.go:19-22` — add `panelMemoryPeek` to the frozen screenChat row.
- `rail_panels_test.go:329` — add `panelMemoryPeek` to the registration loop.

---

### ADR-4: rail-height overflow — DOCUMENT the deferral (no guard in this change)

**Decision.** Do NOT add a height clamp in this change. Document the risk and
recommend the clamp as the FIRST item of the deferred visual-restyle Phase-2
change.

**Rationale.** `rail.Render` (`rail.go:42-57`) stacks `out += s + "\n"` and ignores
the `height` arg; panels ignore `height`; `JoinHorizontal(lipgloss.Top, …)`
(`layout.go:138`) grows the row to the tallest column. Four fully-populated panels
CAN overflow `centerHeight` on a short terminal. BUT in this change the overflow is
structurally bounded:

1. Every chat panel returns `""` until it has data (todolist empty-until-change,
   context-meter empty-until-first-token, telemetry empty-until-first-token,
   memory-peek empty-until-first-`EventMemoryChanged`). So the 4th panel adds ZERO
   height until the first memory write of the session (option a).
2. A real fix touches `rail.Render`'s stacking loop + per-panel height awareness —
   that is a layout/visual change, explicitly OUT of scope (proposal Scope/Out,
   Decision 5). Adding a "minimal" clamp here would mean teaching `rail.Render` to
   measure `lipgloss.Height` per panel and truncate/drop overflow, which is neither
   minimal nor cheap (it changes the rail contract every screen depends on).

**Therefore:** ship the deferral. The follow-up clamp recommendation (height-aware
`rail.Render` that drops or truncates the lowest panels when the stacked height
exceeds `height`) is recorded here and in engram (id 603) for the visual change.
Tasks MUST NOT add a clamp. **This finalizes the rail-height decision → document +
defer, no guard now.**

---

## 3. Data-Flow Diagrams (event → msg → Update → Model field → Render)

### context-meter (PR-a) — accumulate path + boot static

```
boot:  run.go ag.ContextWindowSize() ──copyRailWith──▶ contextMeterPanel.limit   (once)

live:  loop.go EventTokensUsage{SysToks,MsgToks,ToolToks,TokenCount}
        → bus → evCh → pumpEvents → busEventMsg
          → handleBusEvent case EventTokensUsage (screen_chat.go:291-302)
            → copyRailWith → cp:=*cm → cp.accumulate(ev)
                 sys+msg+tool>0 ? REPLACE sys/msg/tool + tokenUsed=Σ : tokenUsed+=TokenCount
              → View → rail.Render → contextMeterPanel.Render
                   reads p.limit/p.sysToks/p.msgToks/p.toolToks/p.tokenUsed   (PURE)
```

### telemetry (PR-b) — accumulate (tools + live subagent) + completed REPLACE

```
EventToolStart/End ─▶ handleBusEvent (screen_chat.go:180,211) ─copyRailWith─▶ accumulate
                          → toolStats[ToolName] {calls++ / durationMs+= / errors++}

EventTokensUsage[subagent_id] ─▶ existing EventTokensUsage block (screen_chat.go:291)
                          → accumulate → subagentStats[id].tokens += in+out  (live, if !done)

EventSubagentCompleted ─▶ NEW case (screen_chat.go) ─copyRailWith─▶ accumulate
                          → subagentStats[id].tokens = Meta["tokens"] (REPLACE); done=true
EventSubagentFailed    ─▶ same NEW case ─▶ accumulate → done=true; failed=true

      → View → telemetryPanel.Render: aggregate + toolStats(cap5,count-desc) + subagentStats(cap3,first-seen)  (PURE)
```

### memory-peek (PR-c) — cmd-refresh (mirrors todolist)

```
curator/consolidator/save_memory EventMemoryChanged{Meta[scope_id,entry_id,title,cluster]}
  → bus → evCh → pumpEvents → busEventMsg
    → handleBusEvent NEW case EventMemoryChanged (screen_chat.go)
      → fetchMemory(m.store, ev.Meta["scope_id"]) tea.Cmd
        → st.SearchMemory(ctx, scopeID, "", 5)        [goroutine — the only IO]
          → memoryRefreshMsg{entries}
            → Model.Update case memoryRefreshMsg (model.go)
              → copyRailWith → cp:=*mp → cp.setEntries(entries)
                → View → memoryPeekPanel.Render reads p.entries (Title|Content)  (PURE)
```

---

## 4. COW / Purity Analysis

| Panel         | Field(s) added                                                      | Mutation site                                                           | COW correctness                                                                                                                                                                      |
| ------------- | ------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| context-meter | `limit` (static), `sysToks/msgToks/toolToks/tokenUsed` (value ints) | boot `copyRailWith` + `EventTokensUsage` `copyRailWith`                 | `cp := *cm` value-copies all ints; no shared references. **Safe by value semantics.**                                                                                                |
| telemetry     | `toolStats`, `subagentStats` (maps), `saOrder` (slice)              | `EventTool*` / `EventTokensUsage` / new `EventSubagent*` `copyRailWith` | `cp := *tp` shares map headers ⇒ **HAZARD**. Mitigation: `accumulate` clones the map+slice it touches before mutating (runs on `cp`). Asserted by a "prior snapshot unchanged" test. |
| memory-peek   | `entries` (slice)                                                   | `memoryRefreshMsg` `copyRailWith`                                       | `setEntries` REPLACES the header with a fresh `SearchMemory` slice; no aliasing. **Safe (mirrors `setList`).**                                                                       |

Purity: no `Render` reads store/agent/clock. `fetchMemory` does all IO inside the
`tea.Cmd` goroutine. `humanK` is pure. The only NEW purity discipline is the
telemetry map-clone (above); every other panel matches an existing precedent.

---

## 5. Test Strategy (per PR)

Infra: table tests drive synthetic `notify.Event`s through `handleBusEvent` (or
panel `accumulate` directly) and assert the `Model`/panel field; render assertions
use `strings.Contains` on stripped output for content checks AND
`golden.RequireEqual(t, []byte(got))` (`golden_test.go:31`) for full-shape pins.
Golden files regenerate ONLY via `go test ... -update` (`tui_test.go:3`); CI runs
without `-update` to assert stability. Strict TDD (`make test`): failing test FIRST.

**PR-a (context-meter):**

- `accumulate` REPLACE: two category events, assert the second snapshot wins
  (not summed) — `sysToks/msgToks/toolToks/tokenUsed`.
- `accumulate` fallback: event with all-zero categories + non-zero `TokenCount`,
  assert `tokenUsed += TokenCount` and category fields stay `0`.
- `setLimit` + `run.go` boot: assert panel renders the real limit label (and the
  `est.` suffix when limit `0`).
- Render golden: (1) smart strategy with category rows; (2) fallback aggregate-only
  (no category rows, no `0%`).
- Degrade scenario (seams): zero categories ⇒ no panic, no sub-bars.

**PR-b (telemetry):**

- per-tool ACCUMULATE: two `EventToolStart` + one `EventToolEnd(IsError)` for one
  tool, assert `calls/errors/durationMs`.
- subagent live ACCUMULATE: two `EventTokensUsage[subagent_id]` turns, assert
  bucket = Σ(in+out).
- subagent REPLACE: live bucket then `EventSubagentCompleted{tokens}`, assert the
  authoritative value WINS (overwrites the running sum) and `done=true`.
- `EventSubagentFailed`: assert `failed=true, done=true` and the `✗` marker renders.
- **COW snapshot test (mandatory):** retain the pre-event `*telemetryPanel`, drive
  a second event on the copied rail, assert the retained panel's `toolStats`/
  `subagentStats` are UNCHANGED (proves map clone).
- new `handleBusEvent` case: synthetic `EventSubagentCompleted` through
  `handleBusEvent`, assert the telemetry panel field updated (non-vacuous, mirrors
  `TestHandleBusEvent_TodolistChanged_*`).
- Render golden: tool rows (cap 5 + "+N more", count-desc order) and subagent rows
  (cap 3, first-seen, markers).

**PR-c (memory-peek):**

- `EventMemoryChanged → fetchMemory` cmd: drive the event through `handleBusEvent`
  with a closed events channel + fake store returning canned entries; assert a
  `memoryRefreshMsg` appears in the batch (non-vacuous, mirrors
  `TestHandleBusEvent_TodolistChanged_ReturnsTodoRefreshCmd`,
  `rail_panels_test.go:386`).
- `fetchMemory` nil/empty: `st==nil` or `scopeID==""` ⇒ zero `memoryRefreshMsg`,
  no panic.
- `memoryRefreshMsg` Update: drive through `Update`, assert
  `panelMemoryPeek.entries` set via COW; assert prior snapshot unaffected.
- Title/Content fallback: entry with empty Title ⇒ row shows Content.
- contract tests: update `TestPanelsFor_ContractMatrix` screenChat row
  (`model_test.go:19`) and the registration loop (`rail_panels_test.go:329`).
- Render golden: populated memory-peek (Title rows) + empty (`""`).

---

## 6. Alternatives Rejected

- **Category mini-bars (3 stacked bars).** Rejected: at `inner≈28` cols each bar is
  < 10 cells, visually meaningless and hard to golden. Chose labeled count rows.
- **Subagent rows ordered tokens-desc.** Rejected: tokens mutate every turn ⇒ rows
  reorder mid-session, breaking golden stability and confusing the eye. Chose
  first-seen.
- **Tool rows in first-seen order.** Considered, but count-desc surfaces the hot
  tools (the operator-useful view) and is still deterministic with the name
  tiebreak. Chose count-desc, name-asc.
- **`EventSubagentFailed` as no-op.** Rejected: a silent failure leaves a `●` live
  marker forever. Chose a `✗` failed marker (`done=true, failed=true`) — near-zero
  cost, real signal.
- **scopeID from `activeConvID` (option c) / `Agent.ScopeID()` (option b).** Both
  rejected: (c) queries the wrong key (`ScopeID ≠ Source`); (b) adds backend
  surface forbidden post-archive. Chose option (a) `Meta["scope_id"]`.
- **Adding a rail height clamp now.** Rejected: it rewrites the cross-screen
  `rail.Render` contract (a visual change, out of scope) and overflow is already
  bounded by empty-until-data + memory-peek-empty-until-first-write. Deferred.
- **High-watermark context fill.** Rejected per proposal Decision 2: the bar must
  reflect CURRENT window fill (drops after compaction). REPLACE, no watermark.

---

## 7. Open Risks for the Tasks Phase

1. **Telemetry map COW (highest).** The map-clone discipline is NEW (todolist had
   none). If `accumulate` mutates the shared map without cloning, prior viewport
   snapshots corrupt silently. Tasks MUST implement `cloneToolStats` /
   `cloneSubagentStats` (+ `saOrder` copy) and the "prior snapshot unchanged" test.
2. **`rail_panels_cmd.go` imports.** PR-c adds `context` + `daimon/internal/store`
   to a file that currently imports only `tea`/`agent`/`tool`. Trivial but easy to
   miss; `agent`/`tool` stay (still used by `fetchTodolist`).
3. **Contract test is in `model_test.go`, not `rail_panels_test.go`.** Two files
   must change for PR-c (`model_test.go:19` matrix row + `rail_panels_test.go:329`
   registration loop). The brief's "rail_panels_test.go:317" is the registration
   test, not the matrix.
4. **`humanK` rounding determinism.** Golden files pin exact strings; `humanK` must
   be deterministic (fixed `%.1fk` / `%dk` rules, no locale). Define once, test
   boundary values (999, 1000, 1500, 200000).
5. **Subagent live-vs-authoritative ordering.** A late `EventTokensUsage` arriving
   AFTER `EventSubagentCompleted` must NOT re-inflate the authoritative total — the
   `if !st.done` guard handles it; tasks must keep that guard and test it.
6. **`EventSubagentFailed` Meta shape.** `tokens` is only guaranteed on
   `EventSubagentCompleted` (seams spec), not on `Failed`; the Failed branch must
   NOT call `atoiSafe(ev.Meta["tokens"])` (it keeps the last live bucket value).
7. **PR-c golden churn includes the existing chat golden.** Adding the 4th panel to
   `panelsFor(screenChat)` may shift `TestModel_View_ChatScreen_Golden` if any chat
   panel has data in that fixture; memory-peek is empty there so likely no shift —
   tasks must run the golden and regenerate ONLY if the diff is the intended
   (empty) memory-peek addition.

```

```
