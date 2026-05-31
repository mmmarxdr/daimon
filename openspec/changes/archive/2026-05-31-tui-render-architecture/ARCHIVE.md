# Archive Report — TUI Render Architecture

**Change**: `tui-render-architecture`
**Archived**: 2026-05-31
**Final phase outcome**: PASS WITH WARNINGS (verify-report)
**PRs**: #64–#66 (PR-1 mode caching + ago pre-compute, PR-2 viewport + thread cap, PR-3 spinner COW batch ticker), all merged into `main`

## Summary

`tui-render-architecture` enforces a hard invariant: **View = pure(Model)**. The TUI's rendering layer is rearchitected so that `Model.View()` and every `Render(width)` it calls is a deterministic, side-effect-free function of the receiver's fields alone. No live-object access (`modeAgent.CurrentMode()` calls from render), no clock reads (`time.Since()` in Render), no IO, no mutation. All such state is snapshotted into Model fields by `Update`.

Concurrently, the chat thread's per-frame cost is bounded from O(n) to O(viewport-height) via `bubbles/viewport` integration, and memory is bounded by capping `thread.items` at 500 items with a visible truncation marker when history is dropped. The spinner's per-tick O(n) full-slice copy is replaced by a single batched O(n) copy per 100ms tick interval, coalescing updates across all running tools.

All 29 tasks across 3 stacked PRs shipped and verified; `-race` clean on all TUI packages.

## Capabilities

### Added

- `tui-render-purity` — `openspec/specs/tui-render-purity/spec.md`
  - REQ-1 (`View Purity`): deterministic, pure render, no live-object access, no clock reads
  - REQ-2 (`Mode Cached in Model`): agent mode snapshotted to `Model.mode`, read-only in View
  - REQ-3 (`Relative Timestamps Pre-computed`): session "ago" strings computed in Update, stored in Model
  - REQ-4 (`Bounded Render Cost`): chat thread render bounded by viewport height, not total items
  - REQ-5 (`Bounded Thread Memory`): items capped at 500, oldest trimmed with visible marker
  - REQ-6 (`Bounded Spinner Update Cost`): spinner ticks batched, one O(n) copy per 100ms, O(1) in-place advances
  - REQ-7 (`No Behavioral/Visual Regression`): chat screen structure preserved, scroll state reset on transitions

## Implementation Footprint

29 tasks across 3 stacked PRs (PR-1 ~110 lines, PR-2 ~230 lines, PR-3 ~120 lines); all packages under `internal/tui/` pass `go test -race`.

Files created/modified (highlights):

**PR-1 (Purity: Mode + Ago)**

- `internal/tui/model.go` — added `mode string` field, wired in `cycleMode()` and `/mode` handler
- `internal/tui/layout.go` — removed `modeAgent.CurrentMode()` live call, read `m.mode` instead
- `internal/tui/rail_panels.go` — added `ago []string` field to `resumeListPanel`, pre-compute in `setSessions`
- `internal/tui/screen_sessions.go` — added `m.sessionsAgo` to Model, pre-compute in `sessionsLoadedMsg` handler, read in render
- `internal/tui/purity_test.go` — new file; `TestView_Deterministic` (chat, sessions, rail, viewport variants)
- `internal/tui/run.go` — set `viewport.New(0,0)` in test model constructor (unblocks PR-2 golden)

**PR-2 (Viewport + Thread Cap)**

- `internal/tui/model.go` — added `viewport viewport.Model` field, `refreshThreadViewport()` helper
- `internal/tui/layout.go` — extracted `chatViewportSize(m)` helper for viewport/layout math sync
- `internal/tui/screen_chat.go` — `renderChat` now calls `m.viewport.View()`; scroll key routing (PgUp/Dn always forward, arrows only when not focused on editor)
- `internal/tui/components_thread.go` — added `truncated bool`, `styles tuiStyles` fields; `append` now trims oldest when exceeding 500 items; `Render` prepends truncation marker
- `internal/tui/golden_test.go` — `TestModel_View_ChatScreen_Golden` adapted to drive WindowSizeMsg before View
- Viewport content pushed via `refreshThreadViewport()` after every thread mutation (Update path, never View)

**PR-3 (Spinner COW Batch)**

- `internal/tui/components_thread.go` — added `own()` helper (make+copy), `runningToolIdxs()` helper; removed per-ToolLine `Tick()`
- `internal/tui/model.go` — added `spinnerActive bool` field; single model-level `spinnerTickCmd()` replaces per-tool ticker
- `internal/tui/screen_chat.go` — batch spinner handler: one `own()` copy for all k running tools, loop in-place advances, single 100ms re-arm
- Refactored EventToolEnd, EventReasoningEnd, r-toggle onto `own()` (behavior-identical, proven by D.6 aliasing proof in design.md)

## Design Decisions (rationale in design.md)

1. **WU-a: Mode as Model field, modeAgent retained for Update-only.** Cache `mode string`, read in render, write in `cycleMode`/`switchModeMsg`/`/mode` handler. Keeps the adapter for next-mode computation without exposing it to View. (ADR-1)

2. **WU-b: "as of last event" ago staleness, no periodic ticker.** Pre-compute "ago" strings in Update on data arrival; do NOT refresh by a periodic Cmd. Acceptable staleness policy; refresh is a deferred follow-up. (ADR-2)

3. **WU-c: Viewport integration + 500-item cap.** Mode caching + ago pre-compute + `bubbles/viewport` window the thread render from O(n) to O(viewport-height). Cap value 500 balances scrollback depth (dozens of turns) vs memory (dozens of KB per 500-item thread). Drop-oldest with visible truncation marker so the user knows history was elided. (ADR-5)

4. **WU-c: Breadcrumb baked into viewport content (scrolls with thread).** Simpler than a fixed row; pinned breadcrumb is a deferrable refinement. (ADR-3)

5. **WU-d: Single model-level batch spinner ticker, amortized O(n) once per tick interval.** Coalesce all running spinners into one ticker, one `own()` copy per 100ms (vs k×O(n) per tick for k running tools). Per-tool literal O(1) is unsoundly impossible under Bubble Tea v1 value semantics (no framework hook on "model handed back"); amortized O(n) once per interval is the sound improvement. (ADR-4, design.md §D.5)

6. **WU-d: Own-once per Update call, no cross-call `ownedGen`.** Drop the `ownedGen`/global-counter idea from the proposal. Correctness needs only "copy-once-at-start-of-this-Update" (D.6 corollary); cross-call generation is not needed and would add global mutable state. (D.7 simplification)

## Determinism Guard (§5 Invariant)

Four test variants cover the headline invariant **View = pure(Model)**:

| Test                              | Screen                     | Guards                                                                                                |
| --------------------------------- | -------------------------- | ----------------------------------------------------------------------------------------------------- |
| `TestView_Deterministic`          | screenChat                 | failingModeAgent stub (rejects live CurrentMode() calls from View), running ToolLine, sessionsAgo set |
| `TestView_Deterministic_Sessions` | screenSessions             | sessionsAgo pre-set; detects live relativeTime calls                                                  |
| `TestView_Deterministic_Rail`     | screenWelcome              | resumeListPanel.ago pre-set via setSessions                                                           |
| `TestView_Deterministic_Viewport` | screenChat + WindowSizeMsg | failingModeAgent stub, viewport sized and populated                                                   |

All four pass GREEN with `-race` clean. The tests call `View()` 50 times per variant on a fixed Model with no intervening `Update`; any byte difference is a regression, and `-race` catches concurrent live-object access.

## Verification Snapshot

| Check                           | Result                                                 |
| ------------------------------- | ------------------------------------------------------ |
| `go test ./internal/tui/`       | PASS                                                   |
| `go test -race ./internal/tui/` | PASS — 0 races                                         |
| `make test` (all packages)      | PASS (pre-existing failure in cmd/daimon out of scope) |
| Tasks checklist                 | 29 / 29 (100%) — all marked `[x]`                      |
| Spec compliance                 | 7 requirements, 18 scenarios — all PASS                |
| Design decision conformance     | WU-a (amended)/b/c/d all implemented                   |

Full verify-report archived at `openspec/changes/archive/2026-05-31-tui-render-architecture/verify-report.md`.

## Known Follow-ups (non-blocking)

1. **Spinner ticker dies on nav-away mid-run (pre-existing).** If the user navigates away from chat while a tool is running, the ticker keeps firing but the ToolLine is no longer in `m.thread.items` (either the thread was reset or the item was trimmed). The orphaned tick no-ops safely (`findToolLineIdx` returns -1). This is pre-existing behavior and is acceptable (the tool is not visible anyway). Tracked as a future enhancement if desired.

2. **Inline spinner-frame arithmetic vs AdvanceSpinner helper (cosmetic).** The spinner frame advancement in the batch handler (PR-3) is done via inline `tl.AdvanceSpinner()` calls after copying the ToolLine. An alternative would be a dedicated helper or a method on ToolLine. No correctness gap; purely a code-organization preference for a follow-up.

## Deviations (all authorized and documented)

1. **WU-a AMENDMENT (judgment-day R1 fix).** The design's original mode-refresh plan left bugs: reading the adapter's `localOverride` returned a stale optimistic value, and Tab presses after a `/mode` command computed next-mode from stale state. Final, implemented design adds: `trueMode()` helper (ground truth via `m.ag.CurrentMode()` when agent is wired, fallback to `m.mode` for unit tests), `ReconcileMode(confirmed)` to race-safely clear the override, and cycleMode computes from `m.mode` (not the stale adapter). Tests added: `TestCycleMode_UsesCachedModeNotStaleOverride`, `TestSwitchModeMsg_ReconcilesOverride`, `TestSwitchModeMsg_ReconcileRaceSafe`. All PASS.

2. **Viewport sizing math duplication (design risk #4).** `chatViewportSize` must stay in lockstep with `renderLayout` chrome reservation. Mitigated by extracting shared layout-math helpers and a sizing test. If they drift, the viewport window mis-sizes — covered by `TestViewport_WindowSizeMsg_Propagates`.

3. **Golden regen blind-accept (design risk #2).** PR-2 chat-golden diff MUST be human-reviewed before `-update`. Happened per the verify-report: visual drift was assessed and the new golden was accepted.

## Deviations from Original Proposal

- **Spinner COW literal O(1) not achieved.** The proposal claimed "O(1) per tick" for spinner animation. The sound achievable bound under Bubble Tea v1 value semantics is "O(n) once per tick interval, independent of running-tool count." The framework has no hook for "model handed back," so cross-call ownership tracking cannot be sound. The design (D.5) delivers the strict practical improvement: k×O(n) → 1×O(n) per 100ms for k running tools, with snapshot isolation proven (D.6). The proposal's O(1) framing was aspirational; the design states the honest bound.

## Status

SDD cycle complete. All artifacts in `openspec/changes/archive/2026-05-31-tui-render-architecture/`; main spec synced at `openspec/specs/tui-render-purity/spec.md`. Ready for the next change.
