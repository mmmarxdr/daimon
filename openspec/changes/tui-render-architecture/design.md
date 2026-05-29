# Technical Design: tui-render-architecture

> SDD design phase. Built on `proposal.md` (engram `sdd/tui-render-architecture/proposal`)
> and `explore.md` (engram `sdd/tui-render-architecture/explore`, ID 563).
> Approach **B** + Option-3 COW are committed by the proposal; this document is the HOW.
>
> Invariant under construction: **View = pure(Model)** — `Model.View()` and every
> `Render(width)` it transitively calls is a deterministic function of the receiver's
> fields alone. No clock reads, no live/shared-object access, no IO, no mutation.

---

## 0. Architecture position (no new pattern)

The TUI keeps its existing shape: a SINGLE root `Model` implementing `tea.Model`;
sub-components are imperative structs with `Render`/`SetData` methods (AD-2). This
change introduces NO new architectural pattern. It moves three classes of input out
of the render path and into `Update`, and adds ONE owned sub-component (`viewport.Model`)
that already follows the imperative-struct convention.

Layering stays:

```
Model.View()                         ── pure, reads Model fields only
  └─ renderLayout(m)                 ── pure
       ├─ topBar.Render               reads tb.mode (snapshotted)
       ├─ renderCenter → renderChat   reads m.viewport (content set in Update)
       ├─ rail.Render                 reads panel fields (ago snapshotted)
       └─ input.RenderWithMode        reads m.mode (snapshotted)

Model.Update(msg)                    ── ALL impurity lives here
  ├─ writes m.mode on mode-change msgs
  ├─ writes ago strings on data-arrival msgs
  ├─ writes m.viewport content/size on thread-change + WindowSizeMsg
  └─ owns/trims thread.items on append
```

The boundary is the `Update`/`View` seam. Everything that was a function of the clock
or a live object becomes a `Model` field written by `Update`.

---

## WU-a — Mode caching

### Decision

Add `mode string` to `Model`. It is the single source of truth that the render path
reads. `modeAgent` remains, but is consulted ONLY in `Update` (the `cycleMode` path).

### Data flow

```
construction (run.go)      m.mode = ag.CurrentMode()           ── snapshot at startup
Tab pressed → cycleMode()  modeAgent.SetModeImmediate(next)
                           m.mode = next                        ── optimistic, in Update
                           returns switchModeCmd (async persist)
switchModeMsg{mode,err}    on success: m.mode already == mode (set optimistically)
                           on err: surface to thread; m.mode left as-is (optimistic)
renderLayout / input / tb  read m.mode                          ── NO live call
```

### Concrete edits

1. `model.go`: add field `mode string` to `Model`.
2. `run.go`: in the `Model{...}` literal, set `mode: ag.CurrentMode()`. The existing
   `panelActivePolicy` already snapshots `ag.CurrentMode()` at startup — same instant,
   so they agree. `newTestModel()` leaves `mode` as zero value `""`; topBar/input
   already treat `""` as "BUILD" default, so the golden output is unchanged (see §Golden).
3. `cycleMode()` (model.go): after `m.modeAgent.SetModeImmediate(next)`, add `m.mode = next`.
   This is the optimistic write — `cycleMode` already runs in `Update`, so this is legal.
4. `layout.go`: DELETE lines 66–71 (the `currentMode := m.topBar.mode; if m.modeAgent != nil { currentMode = m.modeAgent.CurrentMode() }` block). Replace with `currentMode := m.mode`.
   Lines 74–76 (`tb := m.topBar; tb.mode = currentMode; tb.Render(...)`) and line 97
   (`ib.RenderWithMode(..., currentMode)`) now read the cached `currentMode = m.mode`.

### Where modeAgent is STILL needed

`modeAgent` is consulted ONLY in Update (`SetModeImmediate` in `cycleMode`,
`ReconcileMode` in the `switchModeMsg` handler, and as the test-only fallback in
`trueMode()`). The `agentModeAdapter.localOverride` field is kept for the
optimistic-Tab window. Do NOT remove the adapter.

### AMENDMENT (apply-time, judgment-day R1 fix)

Implementation refined the mode-refresh decisions after dual review found that
reading the adapter for the `/mode` refresh returned a stale optimistic override
(localOverride was never cleared), and that the original "read `m.ag.CurrentMode()`"
plan left a second bug: a Tab pressed AFTER `/mode` computed the next mode from
the stale override. Final, implemented design:

1. **`trueMode()` helper** — `m.ag.CurrentMode()` when an agent is wired (ground
   truth, race-proof, bypasses the override); falls back to `m.modeAgent` then
   `m.mode` so it stays unit-testable (tui tests wire no real agent). Used by the
   `/mode` `commandResultMsg` handler and the `switchModeMsg` handler to refresh
   `m.mode`. NEVER called from a render path.
2. **`cycleMode` computes `next` from `m.mode`**, NOT `modeAgent.CurrentMode()`.
   `m.mode` is always kept in sync (cycleMode + trueMode on /mode and switchModeMsg),
   so Tab cycling is correct even after a `/mode` command.
3. **`ReconcileMode(confirmed string)`** added to the `modeAgent` interface +
   adapter + stubs: the `switchModeMsg` handler clears `localOverride` iff it still
   equals the confirmed mode (race-safe — a newer Tab keeps its override, no flicker).

Regression guards: `TestCycleMode_UsesCachedModeNotStaleOverride`,
`TestSwitchModeMsg_ReconcilesOverride`, `TestSwitchModeMsg_ReconcileRaceSafe`, and
`failingModeAgent` added to `TestView_Deterministic`.

### switchModeMsg handler

The existing handler (model.go:328) only surfaces errors. Mode is already written
optimistically in `cycleMode`, so on success there is nothing to do. On error, leave
`m.mode` at the optimistic value (matches current UX — the adapter's `localOverride`
already behaved this way). No change required beyond what cycleMode now writes.

> Note: `/mode` slash command also changes mode via the agent. It currently relies on
> the live `ag.CurrentMode()` read at render time to reflect the change. After this WU,
> a `/mode` swap must also refresh `m.mode`. The slash path runs `runCommandCmd` →
> `commandResultMsg`. Add: when the dispatched command name is `mode`, re-read and set
> `m.mode = m.ag.CurrentMode()` in the `commandResultMsg` handler (guarded `m.ag != nil`).
> This is the one non-obvious extra site; without it, `/mode` would not update the pill.

---

## WU-b — relativeTime pre-computation (2 sites)

### Decision

Compute the "ago" strings in `Update` when source data arrives; store them alongside
the data; Render reads the stored strings. This mirrors the EXISTING `breadcrumb.ago`
pattern (components_breadcrumb.go:35, set in handleBusEvent at screen_chat.go:268) —
we are extending a precedent, not inventing one.

**Staleness policy (committed):** "as of last data event." Ago strings are computed at
the moment `sessionsLoadedMsg` is processed and are NOT refreshed by any ticker. A list
loaded once can read "2m ago" indefinitely until the next load. No periodic refresh Cmd
in this change (proposal §3, §7). This is acceptable because the sessions list is
reloaded on navigation and on data changes.

### Site 1 — `resumeListPanel` (rail_panels.go)

The panel has no Update path; data flows through `setSessions` (called from
`copyRailWith` in the global `sessionsLoadedMsg` handler, model.go:262). So pre-compute
in `setSessions`.

```go
type resumeListPanel struct {
    styles   tuiStyles
    sessions []store.Conversation
    ago      []string   // NEW: ago[i] is the pre-computed "ago" for sessions[i]
}

func (p *resumeListPanel) setSessions(convs []store.Conversation) {
    p.sessions = convs
    p.ago = make([]string, len(convs))
    for i, c := range convs {
        p.ago[i] = relativeTime(c.UpdatedAt)   // clock read happens HERE, in Update
    }
}
```

`Render` (rail_panels.go:615): replace `ago := relativeTime(conv.UpdatedAt)` with an
index into `p.ago`. The render loop caps at 5 (`convs = convs[:maxSessions]`); the `ago`
slice is parallel to `p.sessions`, so index `i` from `range convs` (a prefix of
`p.sessions`) is valid. Guard `i < len(p.ago)` defensively (slices always parallel, but
cheap insurance against a future divergence).

### Site 2 — `renderSessions` (screen_sessions.go)

The center sessions list reads `m.sessions` directly. Pre-compute into a parallel
`Model` field set in the global `sessionsLoadedMsg` handler.

```go
// model.go Model struct, sessions section:
sessionsAgo []string   // NEW: parallel to m.sessions; pre-computed "ago" strings
```

In the `sessionsLoadedMsg` success branch (model.go:252–268), after `m.sessions = msg.convs`:

```go
m.sessionsAgo = make([]string, len(m.sessions))
for i, c := range m.sessions {
    m.sessionsAgo[i] = relativeTime(c.UpdatedAt)
}
```

`renderSessions` (screen_sessions.go:134): replace `ago := relativeTime(conv.UpdatedAt)`
with `ago := ""; if i < len(m.sessionsAgo) { ago = m.sessionsAgo[i] }`.

`relativeTime` itself stays where it is (screen_sessions.go:184) — it is now called ONLY
from Update paths, which is the same status `nowHHMM` already enjoys.

---

## WU-c — Viewport integration + thread memory cap

### C.1 Viewport ownership

Add `viewport viewport.Model` to `Model` (`bubbles/viewport` v1.0.0, already in go.mod).
Construct it in BOTH constructors:

- `run.go`: `viewport: viewport.New(0, 0)` (dimensions arrive via WindowSizeMsg).
- `newTestModel()`: `viewport: viewport.New(0, 0)`.

`viewport.Model` is a value type holding `YOffset`, width, height, and the rendered
content lines. Storing it by value in `Model` is consistent with Bubble Tea v1's
value-Model snapshotting (each Update returns a new `Model` value; the viewport value
is copied with it — its internal `[]string` content is shared, see §C.5 aliasing note).

### C.2 Content push (in Update, never in View)

The thread content string is pushed into the viewport whenever the thread changes —
that is, after EVERY mutation of `m.thread.items` in `updateChat`/`handleBusEvent`.
Centralize this so it cannot be forgotten:

```go
// refreshThreadViewport recomputes the viewport content from the current thread.
// Called in Update after any thread mutation. NEVER called from View.
func (m Model) refreshThreadViewport() Model {
    width := m.viewport.Width
    content := m.thread.Render(width)
    if bc := m.breadcrumb.Render(width); bc != "" {
        content = bc + "\n" + content
    }
    atBottom := m.viewport.AtBottom()
    m.viewport.SetContent(content)
    if atBottom {
        m.viewport.GotoBottom()   // stick-to-bottom (see C.4)
    }
    return m
}
```

Call sites (all in Update): after `m.thread.append(...)` in handleBusEvent (EventToolStart,
EventSubagentSpawned, EventReasoningStart), after the COW slot writes (EventToolEnd,
EventReasoningEnd, spinnerTickMsg, the `r`-toggle), after `agentReplyMsg` append, after
the optimistic `MsgUser` append in handleChatKey/updateWelcome, and after breadcrumb
updates (EventTokensUsage changes the breadcrumb header that prefixes the content).

> Cost note: `m.thread.Render(width)` is still O(n) — it runs once per Update that
> mutates the thread, NOT once per frame. Bubble Tea calls View on every keystroke /
> blink / tick; previously each of those paid O(n). After this WU, View pays
> O(viewport-height) (`viewport.View()` slices the pre-rendered content window), and the
> O(n) render runs only on actual thread mutations. This is the windowing win.

### C.3 renderChat after viewport

```go
func renderChat(m Model, width, height int) string {
    if m.thread.items == nil || len(m.thread.items) == 0 {
        return renderCenterPlaceholder(screenChat, width, height)
    }
    return m.viewport.View()
}
```

The breadcrumb is now baked into the viewport content (C.2) so it scrolls with the
thread — acceptable and simpler than a fixed header. (Alternative: render breadcrumb
outside the viewport as a fixed row and shrink viewport height by 1. Decision: bake it
in for V1; a fixed breadcrumb is a deferrable refinement. Flagged in Risks.)

### C.4 Auto-scroll policy (committed)

**Stick-to-bottom unless the user scrolled up.** Mechanism: in `refreshThreadViewport`,
capture `atBottom := m.viewport.AtBottom()` BEFORE `SetContent`, then `GotoBottom()` only
if it was true. New content while the user sits at the bottom keeps them at the bottom;
if they scrolled up to read history, `YOffset` is left untouched and reading is not
interrupted. This is the proposal's committed UX (§4.4).

### C.5 Dimension propagation (WindowSizeMsg)

The global handler (model.go:215) currently only stores width/height. Extend it to
forward to the viewport AND re-render content at the new width:

```go
case tea.WindowSizeMsg:
    m.width = msg.Width
    m.height = msg.Height
    // Compute the viewport's content area: center column width/height for chat.
    vw, vh := chatViewportSize(m)   // mirrors layout.go centerWidth/centerHeight math
    m.viewport.Width = vw
    m.viewport.Height = vh
    m = m.refreshThreadViewport()   // re-wrap thread content at the new width
    return m, nil
```

`chatViewportSize(m)` is a small helper that replicates the chrome-reservation math
from `renderLayout` (topBarHeight=2, footerHeight=2, inputHeight=4 when hasInput,
minus railWidth when hasRail) so the viewport's window matches the slot `renderChat`
is given. Extract the math into a shared helper used by BOTH `renderLayout` and the
WindowSizeMsg handler to keep them in sync (single source of truth for layout math).

### C.6 YOffset / content reset on screen transitions

Stale scroll must not bleed across screens (proposal §7). The screen transitions that
enter chat are: welcome→chat (updateWelcome Enter, model.go:384), sessions→chat
(updateSessions Enter, screen_sessions.go:69), error→chat (if any). At each transition
INTO chat, and on session resume (which already does `m.thread = thread{}`), reset:

```go
m.viewport.SetContent("")
m.viewport.GotoTop()        // YOffset = 0
m = m.refreshThreadViewport()  // repopulate with the (possibly reset) thread
```

For session resume (screen_sessions.go:74, `m.thread = thread{}`), the reset is mandatory
because the thread is cleared — otherwise the viewport shows stale content from the prior
session. Add the three lines after the `m.thread = thread{}` / first-append block.

### C.7 Key routing for scroll

Scroll keys (PgUp/PgDn, and arrows when focus is NOT on the editor) must drive the
viewport WITHOUT stealing keys from the input editor.

Routing rule in `handleChatKey`:

- `pgup` / `pgdown` / `ctrl+u` / `ctrl+d`: always forward to `m.viewport.Update(msg)`
  (these are never valid text-entry keys). Capture the returned viewport value:
  `m.viewport, cmd = m.viewport.Update(msg)`.
- `up` / `down`: forward to viewport ONLY when `m.focus != focusEditor`
  (i.e. focusMain — thread navigation). When `focusEditor`, they belong to the input.
- All other keys: unchanged (existing focus routing at handleChatKey:410).

This preserves the existing `esc` focus toggle (focusEditor ↔ focusMain) as the way the
user "enters" scroll/navigation mode. No new key binding is introduced (proposal §3 OUT).

### C.8 Thread memory cap

**Cap value: 500 items** (proposal §4.3 committed value). Rationale: a tool-heavy turn
emits ~5–20 items; 500 holds dozens of turns of scrollback while bounding memory. Too
low truncates mid-session; too high weakens the bound. 500 is the committed tradeoff.

Trim happens in `thread.append`, in the Update path (NEVER in Render). Drop-oldest with
a visible truncation marker so the user knows history was elided:

```go
const maxThreadItems = 500

type thread struct {
    items     []threadItem
    truncated bool   // NEW: true once any drop-oldest has occurred
}

func (t *thread) append(item threadItem) {
    t.items = append(t.items, item)
    if len(t.items) > maxThreadItems {
        // drop oldest; keep the most recent maxThreadItems.
        drop := len(t.items) - maxThreadItems
        t.items = t.items[drop:]
        t.truncated = true
    }
}
```

**Truncation marker:** rather than store a marker item inside `items` (which would itself
count against the cap and complicate COW indices), render a synthetic marker line at the
TOP of `thread.Render` when `t.truncated`:

```go
func (t *thread) Render(width int) string {
    // ...existing build of parts...
    out := strings.Join(parts, "\n")
    if t.truncated {
        marker := "  ⋮ earlier messages trimmed"   // styled via a passed/struct style
        out = marker + "\n" + out
    }
    return out
}
```

> Style note: `thread` has no `styles` field today. Either add one (set in both
> constructors) or render the marker with a plain dim style threaded through. Adding a
> `styles tuiStyles` field to `thread` is the cleaner option and keeps Render pure (it
> reads a struct field, not a global). Decision: add `thread.styles`, set it in run.go
> and newTestModel (newTestModel currently builds `thread` via zero value — set it
> explicitly). This keeps the marker honest and consistent with every other component.

### C.8.1 Cap interaction with COW and viewport

- **COW indices:** the slice-trim in `append` changes indices, but `append` is only
  called when ADDING an item; the spinner/EventToolEnd COW look up by `callID` via
  `findToolLineIdx` each time, so they never hold a stale index across a trim. A trim
  could drop a still-running ToolLine that has a pending `tea.Tick`; the next
  `spinnerTickMsg` for that callID will simply not find it (`findToolLineIdx` returns
  -1) and no-op. Safe — no panic, the orphaned tick falls through (same as the existing
  orphaned-tick handling per explore §Leak audit).
- **Viewport:** trimming oldest preserves the stick-to-bottom view (the user is reading
  the newest content); `refreshThreadViewport` re-renders after the trim, so the
  viewport content stays consistent.
- **Owned-slice marker (WU-d):** see §WU-d.4 — a trim that replaces `t.items` with a
  re-sliced view of the SAME backing array does NOT establish sole ownership; the COW
  ownership generation must account for it. Detailed below.

---

## WU-d — Spinner COW fix (Option 3: own-slice-once-per-turn)

This is the highest-risk decision. The rest of this section is the rigorous design and
the aliasing-safety proof.

### D.1 The hazard being preserved

Under Bubble Tea v1, `Update` has a VALUE receiver and returns a `Model` value. The
framework keeps the value it last received (call it the "live model") and may, in
principle, still reference prior returned values transiently. Critically, `m.thread.items`
is a SLICE — copying a `Model` value copies the slice HEADER (ptr/len/cap) but SHARES the
backing array. So if model snapshot A and snapshot B both have `items` headers pointing
at the same backing array, then `B.items[idx] = x` (or mutating `*B.items[idx]`)
retroactively changes what `A.items[idx]` observes. The W1 FIX comment guards exactly
this: it does `make + copy` of the whole slice on every spinner tick so the new model
owns a private backing array before writing.

The cost: that full-slice copy is O(n) and runs every 100ms per running tool.

### D.2 The Option-3 mechanism (own-once-per-turn)

Introduce an ownership generation on `thread`:

```go
type thread struct {
    items     []threadItem
    truncated bool
    styles    tuiStyles
    ownedGen  uint64   // generation tag of the backing array this thread VALUE owns
}
```

And a process-global monotonic counter used ONLY to mint fresh generations:

```go
// threadGenSeq is a package-level monotonic source for ownership generations.
// It is mutated ONLY inside Update (single-threaded w.r.t. the Bubble Tea loop)
// via nextThreadGen(); never read in Render. Not a render input — does not break purity.
var threadGenSeq uint64

func nextThreadGen() uint64 { threadGenSeq++; return threadGenSeq }
```

> Purity note: `threadGenSeq` is global mutable state, which the project standards warn
> against. It is justified and bounded: it is touched ONLY by `nextThreadGen()`, called
> ONLY from Update (the Bubble Tea loop is single-threaded — Update is never concurrent
> with itself), and it is NEVER read by any Render. It is not a render input, so it does
> not weaken View=pure(Model). The `-race` determinism test (§Tests) covers it. An
> alternative that avoids the global is a per-Model `genSeq uint64` field bumped in place;
> but that field is itself copied by value across snapshots, so two snapshots could mint
> the same "next" gen — defeating the uniqueness the scheme needs. The global is the
> correct, minimal choice precisely BECAUSE Update is single-threaded.

### D.3 The owned-write helper

```go
// ownItems guarantees the receiver's items backing array is solely owned by THIS
// thread value, copying once if it is not. After ownItems, in-place slot writes
// (items[idx] = x) are safe — no prior snapshot shares the array.
//
// Ownership is tracked by ownedGen: a thread value "owns" its array iff its ownedGen
// equals the generation stamped when that exact array was last freshly allocated.
// Because copying a Model value copies ownedGen too, a copied snapshot carries the
// SAME ownedGen as the original — so BOTH would believe they own it. To prevent that,
// the FIRST mutation after any copy MUST re-establish exclusive ownership by minting a
// NEW generation and a NEW array. We achieve this by treating ownership as
// "valid only until the next time we hand the model back to the framework".
func (t *thread) own() {
    fresh := make([]threadItem, len(t.items))
    copy(fresh, t.items)
    t.items = fresh
    t.ownedGen = nextThreadGen()
}
```

### D.4 The actual ownership protocol (the subtle, correct version)

The naive "own once, then mutate forever" is UNSOUND because a copied snapshot inherits
`ownedGen` and would skip the copy. The sound protocol is **own-once-per-turn, where a
"turn" is the span during which we KNOW no other snapshot can have been retained.**

Bubble Tea's contract: it calls `Update(msg)` with the live model, takes the returned
model as the new live model, and renders it. Between two `Update` calls it does not mutate
the model. The danger is ONLY that a `tea.Cmd` closure or a retained prior return value
aliases the array. The existing code already proves the safe pattern: copy before write.

We make the copy AMORTIZED, not eliminated, with this rule:

**An array is "turn-owned" iff it was allocated by `own()` during the CURRENT model's
mutation chain and has not yet been observed by the framework.** We cannot prove "not yet
observed" from inside one Update call alone — BUT within a SINGLE `Update` invocation,
the model value is local; no other goroutine or snapshot can hold the array we allocate
mid-call. Therefore:

- **Within one Update call**, after the FIRST `own()` copy, subsequent slot writes in the
  SAME call are O(1) and safe (the array was born this call; nothing else references it).
- **Across Update calls**, we must `own()` again on the first mutation, because the array
  we returned last call is now the live model's array AND may be aliased by any `tea.Cmd`
  closure that captured the model, or by a transient prior reference.

So the amortization the proposal describes ("O(n) once per turn, O(1) per tick") is
realized as **O(n) once per Update-that-mutates, O(1) for every additional mutation in
that same Update.** A spinner tick is ONE mutation per Update (one `spinnerTickMsg` →
one slot write), so a spinner tick still pays one `own()` copy per tick.

**This means raw Option-3-per-tick does NOT reduce spinner cost below O(n)/tick unless
multiple tools tick in the same Update.** That is the honest truth and it must be stated.
The real O(1) win requires batching: see D.5.

### D.5 Realizing O(1) per tick — single-array reuse keyed by generation

To actually get O(1) per tick while staying sound, we exploit that the spinner write does
not change the slice LENGTH or any OTHER element — it swaps ONE `*ToolLine` pointer. The
sound O(1) tick is:

```go
case spinnerTickMsg:
    idx := m.thread.findToolLineIdx(msg.callID)
    if idx >= 0 {
        oldTL := m.thread.items[idx].(*ToolLine)
        if oldTL.state == toolRunning {
            tlCopy := *oldTL
            tlCopy.AdvanceSpinner()
            m.thread.ownForWrite()        // O(1) amortized — see below
            m.thread.items[idx] = &tlCopy
            m = m.refreshThreadViewport()
            return m, tlCopy.Tick()
        }
    }
    return m, nil
```

where `ownForWrite()` copies the array ONLY when this model value does not already own a
PRIVATELY-ALLOCATED array from a prior mutation in an UNBROKEN chain. To make "unbroken
chain" decidable, we tie ownership to a per-Model `ownedGen` that is INVALIDATED whenever
the model could have been shared. The only moment the model is guaranteed shared is when
it is returned to the framework. We cannot hook that moment.

**Resolution — accept the honest bound and pick the safe, simple mechanism:**

Given the above, there is NO sound way to make a SINGLE spinner tick O(1) under Bubble
Tea v1 value semantics WITHOUT a hook on "model handed to framework." The proposal's
amortization assumes multiple mutations per owned span; a 100ms tick is one mutation per
Update.

**Committed decision for WU-d:** keep copy-on-write CORRECTNESS, and reduce cost by
**reducing how often the O(n) copy runs**, not by making each tick O(1) unsoundly:

1. **Coalesce: at most one running spinner needs animation visible at a time is FALSE**
   (multiple tools can run concurrently). Instead, **drive ALL running spinners from a
   SINGLE ticker** and advance them in ONE Update with ONE `own()` copy. Replace the
   per-ToolLine `tea.Tick` (which fires one `spinnerTickMsg{callID}` per tool) with a
   single model-level `spinnerTickMsg{}` (no callID). On each tick:

   ```go
   case spinnerTickMsg:
       running := m.thread.runningToolIdxs()   // indices of toolRunning lines
       if len(running) == 0 {
           return m, nil                         // no tick re-armed; spinner loop idle
       }
       m.thread.own()                            // ONE O(n) copy for the whole batch
       for _, idx := range running {
           tl := *m.thread.items[idx].(*ToolLine)
           tl.AdvanceSpinner()
           m.thread.items[idx] = &tl             // O(1) per running tool, array already owned
       }
       m = m.refreshThreadViewport()
       return m, spinnerTickCmd()                // single 100ms re-arm
   ```

   This gives **O(n) once per 100ms total** (one copy regardless of running-tool count),
   not O(n) per running tool per tick. With k running tools the old code did k×O(n) copies
   per 100ms; the new code does 1×O(n). That is the real, SOUND win, and it strictly
   dominates the old behavior.

2. The `own()` copy per tick is unavoidable for soundness (D.4). But it now runs ONCE per
   tick interval regardless of k, and the per-element work after it is O(k) in-place
   writes on the freshly-owned array — provably non-aliasing because `own()` just minted
   that array in THIS Update call (D.6 proof).

> Honesty: this does NOT achieve "O(1) per tick" in the literal sense the proposal's
> headline claimed; it achieves "O(n) once per tick interval, independent of running-tool
> count, with sound snapshot isolation." That is the correct and achievable bound under
> Bubble Tea v1 value semantics. The proposal's O(1) framing assumed an ownership hook
> that the framework does not provide. This design states the real bound and delivers the
> strict improvement. (Flagged as an open decision in Risks — if the team wants literal
> O(1), it requires a pointer-Model or a custom program loop, both out of scope.)

### D.6 Aliasing-safety proof (for the committed §D.5 mechanism)

Claim: after `m.thread.own()` inside a single `Update` call, writing `m.thread.items[idx] = x`
does not mutate any other `Model` snapshot's view.

Proof:

1. `own()` executes `fresh := make([]threadItem, len)`; `copy(fresh, t.items)`;
   `t.items = fresh`. The `fresh` array is allocated DURING this `Update` call.
2. No reference to `fresh` exists anywhere except `t.items` (i.e. `m.thread.items`) at the
   moment of allocation: `make` returns a brand-new array; `copy` writes element values
   into it but does not leak its address; the only assignment of its header is to
   `t.items`. No `tea.Cmd` closure created earlier could have captured `fresh` because
   `fresh` did not exist when those closures were created.
3. Any PRIOR snapshot A (a `Model` value returned by an earlier `Update`, possibly
   retained by the framework or captured by a Cmd) has `A.thread.items` pointing at a
   DIFFERENT backing array (the one current before `own()` ran). Step 1 reassigned
   `t.items` to `fresh`, so the live model no longer shares A's array.
4. Therefore `m.thread.items[idx] = x` writes into `fresh`, which only the live model
   references. A's view (`A.thread.items[idx]`) is unchanged. ∎

Corollary (multiple writes in the same call): every `m.thread.items[idx] = &tl` in the
D.5 loop writes into `fresh`. Since `fresh` is sole-owned (step 2), all k writes are safe;
no copy is needed between them. The single `own()` amortizes across all k writes IN THIS
CALL — which is exactly the batch, not across calls. Soundness does not depend on
cross-call ownership, so the `ownedGen` field is NOT actually required for correctness.

### D.7 Simplification — drop `ownedGen`

Per the D.6 corollary, correctness needs only "copy-once-at-start-of-this-Update, then
write in place for the rest of THIS Update." It does NOT need a cross-call generation
tag. Therefore:

- **DROP** the `ownedGen` field and the global `threadGenSeq`/`nextThreadGen()` from the
  committed design. They were a red herring from the proposal's "own across the turn"
  framing. Keeping them would add global mutable state for no correctness benefit (and
  would violate the no-global-mutable-state standard).
- **KEEP** `own()` as a private helper that does the one copy. Call it exactly once at the
  start of any Update branch that performs ≥1 in-place slot write
  (spinnerTickMsg batch, EventToolEnd, EventReasoningEnd, the `r`-toggle).
- This also lets the EXISTING per-mutation handlers (EventToolEnd, EventReasoningEnd,
  r-toggle) drop their bespoke `make+copy` and call `m.thread.own()` instead — same cost,
  one code path, less duplication.

Net WU-d change:

1. Add `thread.own()` (the single copy helper) and `thread.runningToolIdxs()`.
2. Replace per-ToolLine `Tick()` fan-out with a single model-level spinner ticker
   (`spinnerTickCmd()` returning `spinnerTickMsg{}`), armed when the first ToolLine enters
   `toolRunning` and re-armed only while ≥1 tool runs.
3. Rewrite the `spinnerTickMsg` handler per D.5 (one `own()`, batch advance).
4. Refactor EventToolEnd / EventReasoningEnd / r-toggle to call `own()` + in-place write
   (mechanical; behavior-identical, proven by D.6).

> Spinner ticker arming: `ToolLine.Tick()` (per-line) is replaced. The single ticker is
> started in handleBusEvent on the FIRST `EventToolStart` that creates a running line
> (guard: only arm if not already running — track with a `m.spinnerActive bool` to avoid
> stacking multiple tickers). It self-stops when `runningToolIdxs()` is empty (handler
> returns `m, nil` with no re-arm). This matches the existing self-cancel behavior
> (explore §Leak audit, components_thread.go:292) and avoids ticker pile-up.

---

## Determinism guard test (§5 invariant)

`internal/tui/purity_test.go` (new file). Table-driven where useful.

```go
func TestView_Deterministic(t *testing.T) {
    m := newTestModel()
    m.width, m.height = 80, 24
    m.screen = screenChat
    // running tool line, sessions with ago, set mode, breadcrumb with ago
    m.mode = "plan"
    m.thread.styles = m.styles
    m.thread.append(&MsgUser{text: "hi", styles: m.styles})
    tl := &ToolLine{callID: "c1", name: "bash", state: toolRunning, styles: m.styles}
    m.thread.append(tl)
    m.sessions = []store.Conversation{{ID: "abc12345", UpdatedAt: time.Now().Add(-2 * time.Minute)}}
    m.sessionsAgo = []string{"2m ago"}
    m = m.refreshThreadViewport()

    first := m.View()
    for i := 0; i < 50; i++ {
        if got := m.View(); got != first {
            t.Fatalf("View() not deterministic at call %d", i)
        }
    }
}
```

Lives in package `tui`. A second variant on `screenSessions` (exercises `m.sessionsAgo`)
and one on the rail (`resumeListPanel.ago`). Pair with `make test-race` — a reintroduced
live `ag.CurrentMode()` or `time.Since` in a render path would either vary the bytes
(test fails) or race on the live object under `-race`.

---

## Test + golden plan (strict TDD: RED → GREEN per WU)

| WU  | New/changed unit tests (RED first)                                                                                                                                                                                                                                                                                                                                        | Golden                                                                                                                                                                                                         |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| a   | `TestMode_CachedField` (cycleMode sets m.mode; switchModeMsg err path); `TestLayout_ReadsCachedMode` (no modeAgent call — inject a modeAgent stub that fails the test if `CurrentMode` is called during View)                                                                                                                                                             | none new; **regen** `TestModel_View_ChatScreen_Golden` only IF output changes (it must NOT — `mode:""`→"BUILD" default unchanged; verify byte-identical, do NOT blind-`-update`)                               |
| b   | `TestResumeListPanel_PrecomputedAgo` (setSessions fills ago; Render reads it); `TestSessions_PrecomputedAgo` (sessionsLoadedMsg fills m.sessionsAgo)                                                                                                                                                                                                                      | none — ago strings were already in golden via `relativeTime`; now sourced from field. **Verify** chat/sessions goldens unchanged (sessions screen has no golden today; add one only if desired)                |
| c   | `TestViewport_WindowSizeMsg_Propagates`; `TestViewport_StickToBottom` (AtBottom→GotoBottom); `TestViewport_FreezeWhenScrolledUp`; `TestViewport_ResetOnTransition`; `TestThreadCap_DropsOldest` + `TestThreadCap_TruncationMarker`; `TestScrollKeys_DoNotStealEditor`                                                                                                     | **regen** `TestModel_View_ChatScreen_Golden` — viewport changes the chat center rendering. Drive a WindowSizeMsg in the golden test BEFORE View (see below). Review the diff for visual drift, then `-update`. |
| d   | `TestSpinner_BatchAdvance_SingleCopy` (k=3 running tools, one tick advances all, prior snapshot unchanged); `TestSpinner_SnapshotIsolation` (hold a pre-tick model value, tick, assert pre-tick frame unchanged — the D.6 property); `TestSpinner_TickerSelfStops` (no running tools → no re-arm); refactor-safety tests for EventToolEnd/ReasoningEnd unchanged behavior | none — spinner frame is not in the static golden (golden uses `toolDone`/`toolRunning` with frame 0); confirm unchanged                                                                                        |

### Golden test adaptation (critical)

The current `TestModel_View_ChatScreen_Golden` sets `m.width/m.height` directly and calls
`View()` WITHOUT a WindowSizeMsg. After WU-c, `m.viewport` has zero width/height at that
point, so `viewport.View()` would render empty. **The golden test must drive a
WindowSizeMsg through Update first** so the viewport is sized and content is pushed:

```go
upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
m = upd.(Model)
// (thread was appended before the WindowSizeMsg; refreshThreadViewport in the handler
//  populates the viewport. If items are appended AFTER, call m = m.refreshThreadViewport())
got := m.View()
```

This is the ONLY structural change to the existing golden test. Same for any future
sessions golden. `newTestModel()` must construct a valid (zero-sized) `viewport.New(0,0)`
so non-chat tests that never size it don't panic.

### Strict TDD cadence

Per WU: write the failing test (RED) for each behavior, run `make test` to confirm RED,
implement minimally to GREEN, then `make test-race`. Golden regen (`-update`) is done
ONLY after the human-reviewable diff is inspected — never blind. `go vet` + `golangci-lint`
clean before each commit.

---

## Commit / PR mapping (3 chained PRs — confirmed)

```
main
 └─ PR-1  purity: WU-a (mode cache) + WU-b (ago precompute) + determinism guard skeleton
     └─ PR-2  viewport + items cap: WU-c (stacked on PR-1)
         └─ PR-3  spinner COW batch: WU-d (stacked on PR-2)
```

- **PR-1 — purity (WU-a + WU-b).** Pure refactor. Adds `m.mode`, `m.sessionsAgo`,
  `resumeListPanel.ago`; removes the live `modeAgent.CurrentMode()` and both
  `relativeTime` calls from render paths; adds the `/mode`→`m.mode` refresh; lands
  `TestView_Deterministic` (welcome + sessions variants). No viewport. Low golden churn
  (must be byte-identical; verify, don't blind-update). Independently green on `make test`
  - `make test-race`. Est. ~110 lines.
- **PR-2 — viewport + items cap (WU-c).** Stacked on PR-1. Adds `m.viewport`,
  `refreshThreadViewport`, `chatViewportSize` (+ extracted shared layout math), WindowSizeMsg
  propagation, YOffset/content reset on transitions, scroll-key routing, the 500-item cap +
  truncation marker (+ `thread.styles`). Carries the chat-golden regen and the WindowSizeMsg
  golden adaptation. Largest unit; this is the judgment-day review. Extends
  `TestView_Deterministic` with a running-tool/viewport variant. Est. ~230 lines.
- **PR-3 — spinner COW batch (WU-d).** Stacked on PR-2 (depends on `thread`; the batch
  ticker also touches the viewport refresh, so stack rather than parallel). Adds
  `thread.own()`, `runningToolIdxs()`, single model-level spinner ticker, batch-advance
  handler; refactors EventToolEnd/ReasoningEnd/r-toggle onto `own()`. Drops the proposal's
  `ownedGen`/global-counter idea (D.7). Lands the snapshot-isolation test (D.6 property).
  Small, focused. Est. ~120 lines.

Each PR is independently green on `make test` and `make test-race`. Total est. ~460 lines —
over the 400-line single-PR budget, which is why the split is mandatory (proposal §6,
delivery_strategy = ask-on-risk → orchestrator confirms chained PRs before apply).

---

## ADR-style decisions (rationale + rejected alternatives)

**ADR-1: Mode as a Model field, modeAgent retained for Update-only.**
Chosen: cache `mode string`, read in render, write in `cycleMode`/`switchModeMsg`/`/mode`.
Rejected: (a) keep live `modeAgent.CurrentMode()` in render — the impurity we are removing;
(b) delete `modeAgent` entirely — breaks `cycleMode`'s next-mode computation and the
optimistic-override across rapid Tab presses. Retain the adapter, isolate its reads to Update.

**ADR-2: "as of last event" ago staleness, no ticker.**
Chosen: pre-compute ago in Update on data arrival; never refresh. Rejected: a periodic
refresh Cmd — adds a clock-driven Cmd and complexity for marginal UX; explicitly deferred
(proposal §3). Accepts that a stale list can show an old "ago" until reload.

**ADR-3: Breadcrumb baked into viewport content (scrolls with thread).**
Chosen: prefix breadcrumb into the viewport content string. Rejected: fixed breadcrumb row
above the viewport (shrink viewport height by 1) — cleaner UX but more layout coupling;
deferred to a follow-up. Flagged in Risks.

**ADR-4: Single model-level spinner ticker + batch own-once-per-tick (not per-tool, not
literal-O(1)-per-tick).**
Chosen: one ticker, one `own()` copy per 100ms regardless of running-tool count, in-place
batch advance proven non-aliasing (D.6). Rejected: (1) in-place mutate with no copy —
unsound aliasing (proposal Option 1); (2) per-tool sub-slice copy — O(running) and a sync
index (proposal Option 2); (3) cross-call `ownedGen` ownership — needs a framework hook that
Bubble Tea v1 does not provide; the global counter it requires violates no-global-mutable-
state for zero correctness gain (D.7). The chosen design is the strict, sound improvement
over the status quo (1×O(n) vs k×O(n) per tick interval) and is the best achievable under
v1 value semantics.

**ADR-5: 500-item cap, drop-oldest, synthetic top-of-render truncation marker.**
Chosen: trim in `append` (Update path), marker rendered at top when `truncated`. Rejected:
storing a marker item in `items` (counts against the cap, complicates COW indices); a
ring buffer (more code, same external behavior). 500 is the committed value (proposal §4.3).

---

## Risks / open questions carried forward

1. **WU-d real bound vs proposal headline.** The proposal said "O(1) per tick"; the sound
   achievable bound under Bubble Tea v1 is "O(n) once per tick interval, independent of
   running-tool count." This design delivers the latter (a strict improvement) and documents
   why literal O(1) is not soundly reachable without a pointer-Model or custom loop. **If the
   team requires literal O(1), that is a larger architectural change (out of scope).**
2. **Golden regen blind-accept risk.** `-update` blindly accepts output; the PR-2 chat-golden
   diff MUST be human-reviewed for visual drift before regen. PR-1 goldens must be
   byte-identical (verify, do not `-update`).
3. **Breadcrumb-in-viewport scroll behavior** (ADR-3) — the session header scrolls away with
   history. If the team wants it pinned, that is a small follow-up (fixed row + height-1).
4. **Viewport sizing math duplication.** `chatViewportSize` must stay in lockstep with
   `renderLayout` chrome reservation; mitigated by extracting shared layout-math helpers.
   If they drift, the viewport window mis-sizes. Covered by a sizing test.
5. **Spinner ticker arming dedupe.** Must guard against stacking multiple model-level
   tickers (`m.spinnerActive bool`); a leaked second ticker would double the tick rate.
   Covered by `TestSpinner_TickerSelfStops` + an arming-dedupe test.
6. **Trim dropping a running ToolLine.** A pending tick for a trimmed line no-ops via
   `findToolLineIdx == -1` — safe, but means a tool that scrolls out of the 500-window stops
   animating (it is no longer visible anyway). Acceptable.

## Status

Design complete. Approach B + the REVISED Option-3 (batch own-once-per-tick, ownedGen
dropped per D.7) committed. Ready for `sdd-tasks` (after spec). Artifacts:
`openspec/changes/tui-render-architecture/design.md`, engram `sdd/tui-render-architecture/design`.
</content>
</invoke>
