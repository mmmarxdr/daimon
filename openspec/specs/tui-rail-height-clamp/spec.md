# TUI Rail Height-Clamp Specification

## Purpose

Closes the height-overflow risk deferred by `tui-rail-panels` (archived
2026-06-01). `rail.Render` becomes height-aware: it distributes a height
budget across the populated panels and passes each its slice; every chat
panel truncates its inner rows to that slice and appends a dim `+N more`
row when content is cut. `todolistPanel` receives a fixed hard cap.

This is a **TUI-only rendering change**. No `internal/agent` /
`internal/notify` source edits. The pure-Render / COW invariants from
`tui-rail-panels` (TR-0-A through TR-0-D) are **unchanged and reaffirmed**
— they continue to apply to every Render path touched here.

---

## Architectural invariants (cross-cutting — all requirements)

The following invariants carry forward verbatim from `tui-rail-panels`.
They apply to every requirement in this spec without exception.

**TR-0-A (pure render):** `Render(width, height int) string` reads ONLY
cached struct fields. No clock, no IO, no store reads. Truncation operates
only on already-cached rows; no panel MAY read live state to decide what to
drop.

**TR-0-B (COW):** Every field mutation goes through `copyRailWith`. The
budget computed by `rail.Render` is a per-frame computation, NOT stored
state; the todolist cap is a render-time slice, not a stored field. No new
`copyRailWith` site is introduced by this change.

**TR-0-C (empty-until-data):** A panel with no data MUST return `""`.
A panel assigned a budget `<= 2` MUST also return `""` (physically
impossible box — see TR-HC-2).

**TR-0-D (ANSI width; centralized styles):** All width math via
`x/ansi`; all style values from `tuiStyles`. No inline hex. Charm v1 only.

---

## ADDED Requirements

### Requirement TR-HC-1: rail.Render distributes a height budget (two-pass)

`rail.Render(screen, width, height int) string` MUST implement a two-pass
algorithm:

**Pass 1 — measure.** For each panel ID in `panelsFor(screen)` order, call
`p.Render(width, height)` (full height = "give me your natural size"). If
the result is `""`, the panel is unpopulated and MUST be excluded from
budget distribution. Collect the populated panels with their `lipgloss.Height`
measurement.

**Separator reservation.** With `n` populated panels:
`avail = height - (n - 1)`. This reserves one newline per inter-panel
separator. If `avail < 0`, clamp to `0`.

**Budget assignment — `assignBudgets(populated, avail)`.** Forward pass
(index 0 … n-1), deterministic, no map iteration:

```
remaining = avail
for i in 0..n-1:
    panelsLeft = n - i
    base = remaining / panelsLeft
    rem  = remaining % panelsLeft
    share = base + 1  if rem > 0, else base   // front-load remainder
    give  = min(share, populated[i].natural)  // surplus stays in pool
    budgets[i] = give
    remaining -= give
```

**Pass 2 — re-render.** For each populated panel at index `i`, call
`p.Render(width, budgets[i])`. If the result is non-empty, append it
followed by `"\n"`. The final `"\n"` is harmless.

The summed rendered height of the output MUST satisfy
`lipgloss.Height(rail.Render(screen, w, h)) <= h` for all `h >= 0`.

`panelsFor` order is authoritative for BOTH render order AND budget
order (remainder front-loading, surplus reflow direction). For
`screenChat`: todolist → context-meter → telemetry → memory-peek. For
`screenDiff`: hunks-nav → rationale → impact → telemetry.

#### Scenario: Core height guarantee — no overflow at any height

- GIVEN all four `screenChat` panels populated
- WHEN `rail.Render(screenChat, 32, h)` is called for each `h` in
  `{8, 12, 24}` with `termenv.TrueColor` forced
- THEN `lipgloss.Height(result) <= h` for every `h`
- AND no panic occurs

#### Scenario: Separator reservation — worked example at h=12

- GIVEN all four `screenChat` panels populated with natural heights
  `[9, 8, 7, 5]` (todolist, context-meter, telemetry, memory-peek)
- WHEN `rail.Render(screenChat, 32, 12)` is called
- THEN `n=4`, `avail = 12 - 3 = 9`
- AND `assignBudgets` forward pass yields `budgets = [3, 2, 2, 2]`
  (sum=9; i=0: panelsLeft=4, share=ceil(9/4)=3; i=1: share=2; i=2:
  share=2; i=3: share=2)
- AND the total stacked height of rendered panels plus separators is `<= 12`

#### Scenario: Surplus reflow — under-full panel donates rows

- GIVEN three populated panels with natural heights `[3, 2, 10]` and
  `avail = 10`
- WHEN `assignBudgets([3,2,10], 10)` runs
- THEN panel 0 gets `give=3` (natural < ceil-share), remaining=7
- AND panel 1 gets `give=2` (natural < ceil-share), remaining=5
- AND panel 2 gets `give=5` (all remaining rows)
- AND sum of budgets `<= 10`

#### Scenario: Empty panel excluded from budget distribution

- GIVEN panels A (populated), B (returns ""), C (populated)
- WHEN `rail.Render` runs pass 1
- THEN `n=2` (B excluded), `avail = height - 1`
- AND B contributes no rendered rows to the output

#### Scenario: screenDiff clamped by same code path

- GIVEN `screenDiff` panels with at least one populated panel
- WHEN `rail.Render(screenDiff, 32, h)` is called for `h` in `{8, 12}`
- THEN `lipgloss.Height(result) <= h`
- AND no source edit to any file outside `internal/tui` is required

---

### Requirement TR-HC-2: Per-panel truncation contract (judgment-day fix)

Each panel MUST use its assigned `height` as `budget` and apply the
**uniform per-panel box-budget contract**:

```
maxContent = budget - 2   (rows available inside the border)
```

| Assigned budget | Behavior                                                                                   |
| --------------- | ------------------------------------------------------------------------------------------ |
| `>= 4`          | header + up to `maxContent - 1` data rows + `+N more` if cut; or all data rows if they fit |
| `== 3`          | **header ONLY** (`maxContent = 1`, adding any row yields 4 rows > budget)                  |
| `<= 2`          | returns `""` (TR-0-C; physically impossible box; `maxContent <= 0`)                        |

**Arithmetic proof (budget==3):** `maxContent = 3 - 2 = 1`. The border
occupies 2 rows (top + bottom). The 1 remaining row holds the header.
Adding `+N more` would require 2 content rows → rendered box = 4 rows >
budget 3. Therefore `+N more` MUST NOT appear at budget==3; the panel
renders the header row only.

**Arithmetic proof (budget==4):** `maxContent = 4 - 2 = 2`. `maxContent -
1 = 1` data slot. If data exists: show 1 data row + `+N more`. Box = 4 =
budget ✓.

**Header is MANDATORY.** `rows[0]` (the `panelHeader` / `panelHeaderWithBadge`
line) MUST NEVER be dropped regardless of budget.

**Data rows truncate tail-first.** Rows are dropped from the bottom,
preserving the most-relevant top rows. Panel-specific order:

- **telemetryPanel:** aggregate lines first, then per-tool rows
  (count-desc), then per-subagent rows (first-seen). Clamp truncates the
  already-assembled `rows` slice from the tail.
- **todolistPanel:** post-cap item rows; drop last items.
- **contextMeterPanel:** bar + pct line are the panel's body; category rows
  (`sys/msg/tool`) are the droppable tail. If budget cannot fit bar+pct,
  this is the `==3` degenerate case. The bar is never split.
- **memoryPeekPanel:** entry rows truncate from the tail.

**`+N more` exact string and style — verbatim reuse of
`rail_panels.go:320-322` idiom** via a new pure helper `renderMoreRow`:

```go
func renderMoreRow(n, inner int, s tuiStyles) string {
    return ansi.Truncate(s.dimLabel.Render(fmt.Sprintf("  +%d more", n)), inner, "…")
}
```

The format string MUST be `"  +%d more"` (two leading spaces). Style MUST
be `dimLabel`. Truncation MUST use `ansi.Truncate(..., inner, "…")`.
`inner = width - 4` (existing convention).

#### Scenario: Budget >= 4 — partial data rows + +N more

- GIVEN `todolistPanel` with 8 items (after cap), assigned `budget=6`
  (`maxContent = 6-2 = 4`, `maxDataRows = 3`, show 2 + "+N more")
- WHEN `Render(32, 6)` is called
- THEN the output contains the header row
- AND exactly 2 data rows are shown (`maxDataRows - 1 = 2`)
- AND the output contains `"  +6 more"` (`N = 8 - 2 = 6`)

#### Scenario: Budget == 3 — header ONLY (judgment-day fix)

- GIVEN `contextMeterPanel` with smart-strategy data (5 natural data rows),
  assigned `budget=3`
- WHEN `Render(32, 3)` is called
- THEN the output contains the panel header
- AND zero data rows are shown
- AND `"  +N more"` does NOT appear (`maxContent=1`; adding it would yield 4
  rows > budget 3)
- AND `lipgloss.Height(output) == 3`

#### Scenario: Budget <= 2 — returns ""

- GIVEN any populated panel assigned `budget=2`
- WHEN `Render(32, 2)` is called
- THEN the return value is `""`
- AND no panic occurs

#### Scenario: Header mandatory — never dropped

- GIVEN `telemetryPanel` with 10 assembled rows, assigned `budget=4`
  (`contentRowBudget = 1`)
- WHEN `Render(32, 4)` is called
- THEN `rows[0]` (the header line) is present in the output
- AND exactly 0 data rows are shown plus `"  +N more"`

#### Scenario: +N more uses exact format and style

- GIVEN any panel with cut content, `inner = width - 4`
- WHEN the `+N more` row is assembled
- THEN the raw string before styling is `fmt.Sprintf("  +%d more", N)`
- AND the style applied is `dimLabel`
- AND the result is `ansi.Truncate(..., inner, "…")`

---

### Requirement TR-HC-3: todolist cap-then-clamp with reconciled +N more

`todolistPanel.Render` MUST apply a **fixed hard cap** of `todolistMaxItems
= 10` items **before** height truncation. Layering is cap-then-clamp:

1. **Cap (height-independent):** Slice to the first `min(len(items), 10)`
   items. This bounds the natural height at `2 + 1 + 10 + 1 = 14` rows
   regardless of terminal height, keeping pass-1 measurement honest for
   `assignBudgets`.
2. **Clamp (height-dependent):** The already-capped item rows then truncate
   to `contentRowBudget` per TR-HC-2.

**Reconciled single `+N more`:** When both cap and clamp would drop items,
MUST emit ONE `+N more` row where `N = totalItems - shownItems`. `shownItems`
is computed after both cap and clamp. Two stacked notices MUST NOT appear.

The `todolistMaxItems = 10` constant MUST be defined in `panels.go`
alongside `panelMinHeight = 4`.

#### Scenario: Cap alone — more than 10 items, budget fits all 10

- GIVEN `todolistPanel` with 15 items, assigned `budget=14` (fits all 10)
- WHEN `Render(32, 14)` is called
- THEN exactly 10 item rows are shown (cap applied)
- AND the output contains `"  +5 more"` (`N = 15 - 10`)
- AND no second `+N more` row appears

#### Scenario: Clamp alone — 6 items, budget too small

- GIVEN `todolistPanel` with 6 items, assigned `budget=6`
  (`contentRowBudget=3`, show 2 items)
- WHEN `Render(32, 6)` is called
- THEN exactly 2 item rows are shown
- AND the output contains `"  +4 more"` (`N = 6 - 2`)

#### Scenario: Both cap and clamp fire — single reconciled +N more

- GIVEN `todolistPanel` with 12 items, assigned `budget=6`
  (`contentRowBudget=3`, cap→10, clamp shows 2)
- WHEN `Render(32, 6)` is called
- THEN exactly 2 item rows are shown
- AND the output contains `"  +10 more"` (`N = 12 - 2`)
- AND exactly ONE `+N more` line appears (no stacked notices)

#### Scenario: Budget == 3 with cap — header ONLY (judgment-day fix)

- GIVEN `todolistPanel` with 12 items, assigned `budget=3`
- WHEN `Render(32, 3)` is called
- THEN the output contains the panel header
- AND zero item rows are shown
- AND `"  +N more"` does NOT appear (`maxContent=1`; any extra row overflows)
- AND `lipgloss.Height(output) == 3`

---

### Requirement TR-HC-4: Golden determinism at boundary heights

Height-boundary golden tests MUST be written for `h ∈ {8, 12, 24}` with
`termenv.TrueColor` forced. Both context-meter strategies (legacy and
smart) MUST have separate goldens at each boundary height so both are
pinned. The strategies' natural heights differ (smart=8 rows, legacy=5
rows), but the rendered output differs by strategy ONLY where the
context-meter receives enough budget to render: at `h=24` the two goldens
differ; at the tight heights `h=8` and `h=12` the context-meter is floored
to `""` (budget ≤ 2), so both strategies produce IDENTICAL output.

The `h=12` golden MUST match the §5 worked example exactly: budgets
`[3, 2, 2, 2]`, only todolist renders (**header ONLY** — budget==3 means
`maxContent=1`, so no data rows and no `+N more`), the other three return
`""`. (The pre-fix spec incorrectly stated "header + `+6 more`"; the corrected
arithmetic shows that "+N more" at budget==3 produces 4 rows > 3 — overflow.)

The `h=24` golden MUST show all four panels near-natural with minimal
truncation.

The `h=8` golden MUST show the most aggressive clamp (`avail = 8 - 3 = 5`,
budgets even tighter than `h=12`).

Each golden MUST be regenerated deterministically; `make test` MUST be
green on the pinned goldens.

The core assertion `lipgloss.Height(rail.Render(screen, w, h)) <= h` MUST
be asserted as a table-driven unit test independent of the golden files,
covering `h ∈ {8, 12, 24}` for both `screenChat` and `screenDiff`.

#### Scenario: h=12 golden matches worked example (judgment-day fix)

- GIVEN all four `screenChat` panels populated (natural heights as in §5)
  and `termenv.TrueColor` forced
- WHEN `rail.Render(screenChat, 32, 12)` is called
- THEN the output matches the `h=12` golden file exactly
- AND only todolist renders with **header only** (budget==3 → `maxContent=1`
  → no data rows, no `+N more`); three panels return `""`
- AND `lipgloss.Height(output) <= 12`

#### Scenario: Smart and legacy are identical at h=12 (both floored)

- GIVEN `contextMeterPanel` in smart-strategy state (natural=8) vs. legacy
  state (natural=5), all other panels identical, `h=12`
- WHEN `rail.Render(screenChat, 32, 12)` is called for each strategy
- THEN the two outputs are IDENTICAL — at `h=12` the context-meter receives
  budget=2 (≤ floor) in both strategies and renders `""`, so only todolist
  shows; the strategy's natural-height difference cannot affect the result
- AND BOTH satisfy `lipgloss.Height(result) <= 12`

#### Scenario: Smart-strategy golden differs from legacy at h=24

- GIVEN `contextMeterPanel` in smart-strategy state (natural=8) vs. legacy
  state (natural=5), all other panels identical, `h=24`
- WHEN `rail.Render(screenChat, 32, 24)` is called for each strategy
- THEN the two outputs differ — at `h=24` the context-meter has enough
  budget to render, and its smart (per-category rows, `2.7% of 128k`) vs.
  legacy (`21.0% of 200k est.`) content diverges
- AND BOTH satisfy `lipgloss.Height(result) <= 24`

#### Scenario: h=8 — most aggressive clamp

- GIVEN all four `screenChat` panels populated, `termenv.TrueColor` forced
- WHEN `rail.Render(screenChat, 32, 8)` is called
- THEN `lipgloss.Height(result) <= 8`
- AND the output matches the `h=8` golden file exactly

#### Scenario: h=24 — comfortable height, all panels near-natural

- GIVEN all four `screenChat` panels populated, `termenv.TrueColor` forced
- WHEN `rail.Render(screenChat, 32, 24)` is called
- THEN `lipgloss.Height(result) <= 24`
- AND the output matches the `h=24` golden file exactly

---

## Requirement mapping summary

| ID      | Description                                  | Scenarios |
| ------- | -------------------------------------------- | --------- |
| TR-0-A  | Pure render (carried)                        | 1         |
| TR-0-B  | COW (carried)                                | 1         |
| TR-0-C  | Empty renders `""` (extended: budget≤2)      | 1         |
| TR-0-D  | ANSI width; centralized styles (carried)     | —         |
| TR-HC-1 | `rail.Render` two-pass budget distribution   | 5         |
| TR-HC-2 | Per-panel truncation contract (all 4 panels) | 5         |
| TR-HC-3 | todolist cap-then-clamp, reconciled +N more  | 4         |
| TR-HC-4 | Golden determinism at h=8/12/24              | 4         |

Total new scenarios: **18** (plus 3 carried from TR-0-A/B/C).

---

## Non-requirements (scope boundary)

- Rail scroll / viewport — rejected (breaks `View = pure(Model)`).
- Silent panel-drop policy — rejected; `<= 2` budget drop is forced/physical, not a policy.
- Priority-weighted budget — deferred to follow-on hybrid (option e).
- `MinHeight()` / `PreferredHeight()` interface method on `Panel` — rejected (ADR-3).
- Any visual restyle (borders, colors, new panels) — orthogonal.
- Any file outside `internal/tui/` — prohibited.
- Any `internal/agent` / `internal/notify` source edit.
