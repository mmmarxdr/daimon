# Exploration: tui-rail-height-clamp

> Investigated 2026-06-01 against `main` (tip 9d45714, post tui-rail-panels).
> All claims verified against live source under `internal/tui/`.

## Problem

The TUI right rail has no per-panel height clamp. `rail.Render` and all four
chat panels ignore the `height` budget, so a fully-populated rail can exceed
the terminal height and silently push the InputBar and footer off-screen.

## 1. Current behavior (code anchors)

- **`rail.Render` ignores `height`** — `internal/tui/rail.go:43-58`: iterates
  `panelsFor(screen)`, calls `p.Render(width, height)`, concatenates with
  `out += s + "\n"`. No row counter, no budget check, no truncation. `height`
  is forwarded to panels but unused.
- **All four chat panels declare `_ int` for height**:
  `telemetryPanel.Render` (rail_panels.go:233), `todolistPanel.Render` (:387),
  `contextMeterPanel.Render` (:612), `memoryPeekPanel.Render` (:966).
- **`layout.go` computes but discards the budget** — `layout.go:99`
  `centerHeight = m.height - 8` (topBar 2 + footer 2 + inputBar 4); line 133
  passes it to `m.rail.Render(...)`; downstream it is thrown away.
- **Overflow = silent layout corruption, not panic** — `layout.go:139`
  `lipgloss.JoinHorizontal(lipgloss.Top, center, railRendered)` PADS the shorter
  block; it does NOT clip. When the rail exceeds `centerHeight`, `mainRow` grows
  and pushes InputBar + footer below the visible terminal bottom.
- **Partial self-limiting already exists**: `memoryPeekPanel` caps 5 entries
  (:980); `telemetryPanel` caps tool rows 5 + "+N more" (:299) and subagent rows
  3 (:334); `contextMeterPanel` is fixed 3 (legacy) / 6 (smart) rows.
  **`todolistPanel` has NO cap** — renders every item (:410).

### Worst-case quantification

Fixed chrome = 8 rows; `centerHeight = h - 8`.

| Panel             | Min rows | Worst-case                |
| ----------------- | -------- | ------------------------- |
| todolistPanel     | 4        | Unbounded (15 items → 19) |
| contextMeterPanel | 5        | 9 (smart)                 |
| telemetryPanel    | 6        | 18                        |
| memoryPeekPanel   | 4        | 8                         |

At h=24 → 16 rows available; worst-case all 4 populated = 54 rows; even minimal
population ≈ 19 rows (already overflows). At h=40 → 32 available, still overflows.

## 2. Invariants any policy MUST respect

From the archived `tui-rail-panels` / `tui-render-purity` specs: Render is a PURE
function of cached Model fields (no clock/IO/store); state mutations go through
`copyRailWith` (COW); width via `x/ansi`; styles from `tuiStyles`.

## 3. Policy options

| Policy                                          | Mechanism                                               | UX                                       | Impl LOC | Blast radius                          | Testability | Invariant fit          |
| ----------------------------------------------- | ------------------------------------------------------- | ---------------------------------------- | -------- | ------------------------------------- | ----------- | ---------------------- |
| (a) Drop/hide panels                            | priority drop in rail.Render                            | Poor — panels vanish silently            | ~30      | rail.go                               | Excellent   | Full                   |
| (b) Per-panel truncation                        | activate `height`; truncate rows + "+N more"            | Good — all visible, truncation indicated | ~100     | rail.go + 4 panels                    | Good        | Full                   |
| (c) Proportional allocation                     | weight table → per-panel budget → truncate              | Fair — small budgets, rounding           | ~90      | rail.go + 4 panels                    | Fair        | Full                   |
| (d) Rail scroll/viewport                        | viewport.Model + focus/keys                             | Best access, needs keyboard              | ~200     | rail.go + model.go + layout.go + keys | Poor        | **BREAKS pure-render** |
| (e) Hybrid: min + priority-drop + per-panel cap | per-panel min/cap; drop lowest-priority if budget < min | Best static UX, graceful degradation     | ~120     | rail.go + 4 panels + constants        | Good        | Full                   |

## 4. Recommendation

**Option (b) — per-panel truncation** (minimum-friction): the `height` param is
already threaded from layout → rail → panels; activating it follows the existing
telemetry "+N more" pattern. All panels stay visible; pure-render + COW untouched;
golden tests deterministic at fixed heights.

Sketch: `rail.Render` counts populated panels, computes
`perPanelBudget = height / numPopulated`, passes it as each panel's `height`;
each panel truncates its inner `rows` to `budget - 2` (borders) and appends a
`+N more` dim row when cut. `wrapPanelBox` unchanged.

**Runner-up: Option (e) — hybrid.** (b) alone degrades at very short terminals
(h=16 → budget 2/panel). The hybrid adds a priority-drop outer layer when
`perPanelBudget < panelMinHeight`. Natural second PR after (b).

**Not recommended: (d) scroll** — breaks `View=pure(Model)` (offset is display
state), adds focus/keys to a stateless rail, disproportionate to the problem.

## 5. Open questions for proposal/design

1. Budget distribution: equal (`height/N`) vs priority-weighted?
2. Minimum useful height per panel (suggest contextMeter 5, telemetry 6,
   todolist 4, memoryPeek 4) — below which drop rather than render 1 row.
3. todolistPanel soft cap (currently unbounded) — add a hard cap independent of
   height math? Reduces worst-case regardless of terminal height.
4. Priority order for drop (if (e)): is `panelsFor` order the priority, or a
   separate table?
5. Two-pass (render → measure → re-render) vs one-pass (probe `MinHeight()`).
6. `screenDiff` (panels.go:42) has the same 4-panel structure — in scope or follow-on?
7. Test strategy: height-boundary fixtures (h=8, h=12) + aggressive-clamp golden (h=6).

## Affected files

- `internal/tui/rail.go` — `rail.Render` (budget distribution; optional priority-drop)
- `internal/tui/rail_panels.go` — four panel `Render` methods (height-aware truncation)
- `internal/tui/rail_panels_test.go` — height-clamped cases + golden variants
- `internal/tui/panels.go` — possible priority/min-height constants
- `model.go`, `screen_chat.go`, `layout.go` — read-only reference; no changes

> Persisted to engram `sdd/tui-rail-height-clamp/explore` (#618).
