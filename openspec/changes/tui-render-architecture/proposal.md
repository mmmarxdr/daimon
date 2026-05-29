# Proposal: tui-render-architecture

> SDD propose phase. Built on `explore.md` (engram `sdd/tui-render-architecture/explore`, ID 563).
> Recommended approach **B**. Spinner copy-on-write is pulled IN SCOPE per user decision.

## 1. Why / Intent

The embedded TUI lacks a hard, enforced invariant: today `View()` reaches into
live objects (`m.modeAgent.CurrentMode()` takes an RLock into the running agent
every frame) and calls `time.Since()` inside `Render`, so the rendered output is
a function of wall-clock and shared mutable state, not of the Model alone. On top
of that, the chat thread costs **O(n)** per frame (full string join over every
item) and **O(n)** per 100ms spinner tick (full slice copy), with `thread.items`
never trimmed — so a long session pays linearly more CPU and memory the longer it
runs. A leak audit was performed across the bus subscription, `pumpEvents`, the
mux reply goroutine, and the spinner `tea.Tick`; **no leaks were found** — this
change fixes purity and per-frame/per-tick cost, not lifetimes.

Success looks like: `View` is a pure, deterministic function of `Model`; render
and spinner cost are bounded regardless of session length; and the purity rule is
guarded by a test, not by convention alone.

## 2. Scope — IN

Four work units (WU):

- **(a) Mode caching.** Promote the agent mode to a `Model` field (e.g. `m.mode`),
  refreshed in `Update` on the events that already signal a mode change. Remove
  `m.modeAgent.CurrentMode()` from `renderLayout` (layout.go:70). Render reads the
  cached field only.
- **(b) relativeTime pre-compute (2 sites).** Compute the "ago" strings in `Update`
  when the source data arrives, store them alongside the data, and have Render read
  the stored strings. Sites:
  - `resumeListPanel.Render` (rail_panels.go:615) — pre-compute in `setSessions`
    via an added `ago []string` field (the panel has no Update path of its own).
  - `renderSessions` (screen_sessions.go:134) — pre-compute on `sessionsLoadedMsg`.
- **(c) Viewport + thread memory cap (`renderChat`).** Integrate `bubbles/viewport`
  (already in go.mod v1.0.0 — no new dependency) so the thread render is
  O(viewport-height) instead of O(n), and cap `thread.items` so memory is bounded.
- **(d) Spinner COW fix.** Replace the O(n) full-slice copy on every `spinnerTickMsg`
  (screen_chat.go:48-60) with an O(1) update that still preserves snapshot isolation.
  Chosen strategy and justification in §4.

## 3. Scope — OUT

- **No new screens or features.** No new key bindings, panels, or flows.
- **No behavioral or visual change** beyond what purity and windowing strictly
  require. The cached mode/ago values must render identically to the live ones at
  steady state; viewport changes how much of the thread is materialized, not how a
  visible row looks.
- **Thread-structure work (Inc.2)** is a separate, already-completed effort and is
  not touched here.
- **No dirty-flag/incremental render cache** (explore approach C) — higher effort,
  cache-invalidation risk, not needed once viewport bounds the cost.
- **No periodic "ago" refresh ticker** in this change — staleness policy is "as of
  last event" (see Risks); a refresh Cmd is a deferrable follow-up if desired.

## 4. Approach

**Approach B** (explore §Recommendation): mode caching + relativeTime pre-compute

- `bubbles/viewport` with an items cap, extended with the spinner COW fix.

### Architectural decisions

1. **State that View needs lives in the Model.** Anything Render currently computes
   from a live object or the clock becomes a Model field written in Update: `m.mode`,
   per-session `ago` strings. This is the concrete mechanism for the invariant in §5.
2. **Viewport owns the scroll window; Model owns viewport state.** `m.viewport`
   (a `viewport.Model`) holds `YOffset`/dimensions. `renderChat` calls
   `m.viewport.View()`. Content is pushed via `SetContent(m.thread.Render(width))`
   in Update whenever the thread changes — not in `View`. Width/height propagate
   from the global `tea.WindowSizeMsg` handler into `m.viewport`.
3. **Items cap is a ring/trim in Update.** When `append` would exceed the cap,
   trim the oldest items in the Update path (never in Render). Proposed value 500
   (decision flagged in Risks); trimming oldest preserves the stick-to-bottom view.
4. **Auto-scroll = stick-to-bottom unless the user scrolled up.** Use the viewport's
   `AtBottom()` to decide: if the user is at the bottom when new content arrives,
   `GotoBottom()`; if they scrolled up, leave `YOffset` untouched so reading is not
   interrupted. Reset `YOffset`/content on screen transitions to avoid stale-scroll
   bleed.

### Chosen COW strategy (WU-d) — pointer swap of a single copied ToolLine

**Decision: keep copy-on-write at the granularity of the single mutated `*ToolLine`,
but stop copying the items slice. Reuse the existing backing array; swap only the one
element.**

The current code already copies the `*ToolLine` (`tlCopy := *oldTL`) — that part is
correct and must stay. The waste is `make + copy` of the whole `[]threadItem`. The
options considered:

- **Option 1 — in-place mutate `oldTL.AdvanceSpinner()` (no copy at all).** O(1), but
  **rejected**: `m.thread.items` shares its backing array across value-copied Model
  snapshots, so mutating the existing `*ToolLine` through its pointer retroactively
  mutates every prior model's view. This is exactly the aliasing the W1 FIX comment
  guards against. Unsound under Bubble Tea v1's value-Model pattern.
- **Option 2 — index of running tools + per-tool sub-slice copy.** Reduces the copy
  to the running subset, but still O(running) and adds an index to keep in sync;
  complexity without closing the O(1) gap.
- **Option 3 (CHOSEN) — copy only the one ToolLine, then assign it back into the
  shared slice element: `m.thread.items[idx] = &tlCopy`.** O(1) per tick. Snapshot
  isolation is preserved because the _prior_ model's `items[idx]` still points at the
  original `oldTL` (we never mutate `oldTL`), and the new model's `items[idx]` points
  at the fresh `tlCopy`. The element _slot_ is shared, so writing a new pointer into
  it is the subtle part: it overwrites the slot the prior snapshot also reads.

  To make Option 3 fully sound we need the slot itself to be owned by the current
  model, not shared. The proposed mechanism: **copy-on-write the slice exactly once
  when a turn begins materializing tool lines** (or lazily on first mutation of a
  given backing array), tracked by a small `owned bool`/generation marker on
  `thread`, then mutate slots in place for the life of that owned slice. This
  amortizes the single slice copy across all ticks of a turn instead of paying it
  every 100ms — O(n) once per turn, O(1) per tick — while keeping snapshot isolation.

**Justification.** This is the minimal change that turns a per-tick O(n) into an
amortized O(1) without weakening the correctness property the original author
encoded. It avoids a parallel running-tools index (Option 2's sync hazard) and
avoids the unsound in-place mutation (Option 1). The "copy slice once per turn, own
it thereafter" rule is the standard persistent-vs-owned tradeoff and is the smallest
delta to the existing code. The exact ownership marker (generation counter vs bool)
is a design-phase detail; the proposal commits to the strategy, not the field name.

## 5. Invariant statement

> **View = pure(Model).** `Model.View()` and every `Render(width)` it transitively
> calls MUST be a deterministic function of the receiver's fields alone. No access
> to live/shared objects (no `agent.CurrentMode()`, no RLocks into running services),
> no clock reads (`time.Now`/`time.Since`), no IO, no mutation. All such inputs are
> snapshotted into Model fields by `Update`.

**Guard.** A determinism test: build a fixed `Model` (with running tool lines, sessions
with ago strings, a set mode), call `View()` N times consecutively (no intervening
`Update`), and assert all N outputs are byte-identical. Because the only previously
non-deterministic inputs were the clock and live objects, removing them makes repeated
`View()` calls provably stable; the test fails loudly if anyone reintroduces a clock
read or live lookup into a render path. Pairs with `-race` to catch any concurrent
live-object access.

## 6. Delivery / Review Workload Forecast

- **Estimated changed lines:** ~480–620 across the 4 units (a: ~40, b: ~70 over 2
  sites, c: ~180–260 incl. viewport wiring + window-size propagation + golden
  regeneration, d: ~60–90 incl. ownership marker + tests). Golden `-update`
  regeneration for the chat screen inflates the diff.
- **400-line budget risk: High.** The change exceeds the 400-line/PR budget.
- **Chained PRs recommended: Yes.** The work units are separable and ordered:
  1. **PR-1 — purity (WU-a + WU-b):** mode caching + relativeTime pre-compute. Pure
     refactor, no viewport, low golden churn. Independently green on `make test`.
  2. **PR-2 — viewport + items cap (WU-c):** stacked on PR-1. Carries the golden
     regeneration and window-size propagation. Largest unit; judgment-day review here.
  3. **PR-3 — spinner COW (WU-d):** stacked on PR-2 (or parallel to it — depends only
     on `thread`, not viewport). Small, focused, with the determinism + ownership tests.
     The determinism guard (§5) lands in PR-1 and is extended per PR.
- **Decision needed before apply: Yes.** Per `delivery_strategy = ask-on-risk` and
  the High budget risk, the orchestrator must STOP and confirm chained/stacked PRs
  vs a maintainer-approved `size:exception` before launching apply.

## 7. Risks / Open Questions

- **Viewport `YOffset` reset on screen transitions.** welcome→chat→sessions must
  reset offset/content or stale scroll bleeds across screens. Needs an explicit reset
  in the transition handlers.
- **`tea.WindowSizeMsg` → viewport propagation.** The global handler must forward
  width/height to `m.viewport`; missing this leaves the window mis-sized.
- **Auto-scroll policy.** Stick-to-bottom vs freeze-when-scrolled-up — proposed
  `AtBottom()`-gated `GotoBottom()`. Confirm desired UX.
- **Items cap value.** Proposed 500. Too low truncates history mid-session; too high
  weakens the memory bound. Decision needed.
- **relativeTime staleness.** "Ago" strings are computed at event time and not
  refreshed until the next event — a session list can show "2m ago" indefinitely.
  Accepted policy is "as of last event"; a periodic refresh Cmd is an explicit
  out-of-scope follow-up.
- **Spinner COW ownership marker.** The "own the slice once per turn" mechanism must
  not leak ownership across model snapshots in a way that reintroduces aliasing;
  the determinism + `-race` tests must cover concurrent ticks during a turn.
- **Golden regeneration.** Chat-screen golden tests require `-update`; the diff must
  be reviewed for unintended visual drift, since `-update` blindly accepts output.

## Status

Ready for spec + design (can run in parallel). Approach B + Option-3 COW committed.
Artifacts: `openspec/changes/tui-render-architecture/proposal.md`, engram
`sdd/tui-render-architecture/proposal`.
