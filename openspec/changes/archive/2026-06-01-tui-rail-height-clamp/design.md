# Design: tui-rail-height-clamp

> Technical design for the rail height-budget clamp.
> Follow-on to `tui-rail-panels` (archived 2026-06-01); closes the
> height-overflow risk that change documented and deferred (engram #603).
> Source of truth: `proposal.md` (policy LOCKED = Option (b) per-panel
> truncation + "+N more"). This document FINALIZES every point the proposal
> left "Deferred to DESIGN" so the spec phase has zero ambiguity.
> **TUI-only.** No source edits outside `internal/tui/`.

---

## 1. Context & Constraints

### What this change does

`rail.Render` currently stacks every populated panel with `out += s + "\n"` and
**ignores** the `height` budget that `layout.go` already threads in (`rail.go:49-56`).
All four chat panels declare the height arg as `_ int` and discard it
(`telemetryPanel.Render` `rail_panels.go:233`, `todolistPanel.Render` `:387`,
`contextMeterPanel.Render` `:612`, `memoryPeekPanel.Render` `:966`). Because
`lipgloss.JoinHorizontal(lipgloss.Top, center, railRendered)` (`layout.go:139`)
**pads** the shorter column and never clips, an over-tall rail grows `mainRow` and
pushes the InputBar + footer below the visible terminal bottom — silent layout
corruption, not a panic.

This change makes `rail.Render` **height-aware**: it distributes the `height`
budget across the populated panels and hands each panel its slice; each panel
truncates its inner rows to that slice and appends a dim `+N more` row when it
cuts content. `todolistPanel` — the single unbounded panel (`rail_panels.go:410`
renders EVERY item) — also gets a fixed hard cap. The clamp lives entirely in
`rail.Render`, which serves BOTH `screenChat` and `screenDiff` (`panels.go:40,42`,
both lists end in `panelTelemetry`), so the diff rail is clamped by the same path
with no extra wiring.

### The non-negotiable invariants (carried from `tui-rail-panels`)

- **`View = pure(Model)`.** Every `Render(width, height int) string` reads ONLY
  cached struct fields. No clock, no IO, no store/agent reads in any `Render`
  path. The height arg activated here is a pure integer input — truncation
  operates only on already-cached rows; no panel may read live state to "decide"
  what to drop.
- **COW via `copyRailWith`** (`rail.go:71-78`). This change adds NO new mutating
  field and NO new `copyRailWith` site — the budget is a per-frame render-time
  computation, not stored state. (The todolist cap is a render-time slice, not a
  stored cap.) Prior `Model` snapshots remain untouched by construction.
- **ANSI-aware width** via `github.com/charmbracelet/x/ansi` (`StringWidth` /
  `Truncate`, never `len()`); styles from centralized `tuiStyles`
  (`dimLabel`, `panelHeader`, `panelHeaderWithBadge`, `accent`, `amber`,
  `errStyle`); no inline hex. Charm v1 only.

### The vertical-overhead arithmetic (the heart of the truncation contract)

`wrapPanelBox` (`rail_panels.go:46-68`) renders its content inside
`s.panelBorder.Width(width-2)`. The border adds **2 rows vertically** (top rule +
bottom rule) and **0 extra content rows**. Therefore for any panel:

```
renderedHeight(panel) = 2 (border)  +  len(rows)
```

where `rows` is the slice the panel joins with `"\n"` and passes to
`wrapPanelBox`. `rows[0]` is ALWAYS the header line (`panelHeader` /
`panelHeaderWithBadge`). So the budget accounting per panel is:

```
budget                     ← rows assigned to this panel by rail.Render
contentRowBudget = budget - 2 (border) - 1 (header)   ← rows available for DATA
```

`rail.Render` also appends a `"\n"` after each non-empty panel block (`:53`).
That trailing newline is the inter-panel separator; it is counted against the
total (see §2 ADR-1 algorithm) so the stacked total never exceeds `height`.

### Verified ground truth (file:line anchors — confirmed against live `main`)

| Fact                                                                                       | Anchor                                         |
| ------------------------------------------------------------------------------------------ | ---------------------------------------------- |
| `rail.Render(screen, width, height int)`; loop `out += s + "\n"`; forwards `height` UNUSED | `rail.go:43-58` (loop `:49-56`)                |
| `panelsFor(screenChat)` = `{Todolist, ContextMeter, Telemetry, MemoryPeek}`                | `panels.go:40`                                 |
| `panelsFor(screenDiff)` = `{HunksNav, Rationale, Impact, Telemetry}`                       | `panels.go:42`                                 |
| `copyRailWith` COW shallow-copy                                                            | `rail.go:71-78`                                |
| `telemetryPanel.Render(width, _ int)`                                                      | `rail_panels.go:233`                           |
| `todolistPanel.Render(width, _ int)` — renders EVERY item, no cap                          | `rail_panels.go:387`, loop `:410-422`          |
| `contextMeterPanel.Render(width, _ int)` — 3 rows legacy / 6 smart                         | `rail_panels.go:612`, category rows `:659-668` |
| `memoryPeekPanel.Render(width, _ int)` — caps 5 (`maxRows`)                                | `rail_panels.go:966`, cap `:980-984`           |
| `wrapPanelBox` border = `Width(width-2)`; +2 rows vertical; `inner = width-4`              | `rail_panels.go:46-68`                         |
| Existing `+N more` idiom: `fmt.Sprintf("  +%d more", overflow)` + `dimLabel`               | `rail_panels.go:320-322`                       |
| telemetry tool cap-5 + overflow block (the `+N more` template)                             | `rail_panels.go:299-323`                       |
| `centerHeight = m.height - 8` (topBar 2 + footer 2 + input 4)                              | `layout.go:99`                                 |
| rail invoked with `centerHeight` as the height arg                                         | `layout.go:133`                                |
| `JoinHorizontal(lipgloss.Top, …)` PADS, never clips                                        | `layout.go:139`                                |
| `humanK` pure helper (token short form)                                                    | `rail_panels.go` (used `:625,660-662`)         |

> Anchor correction vs. the launch brief: the brief cites the `+N more`
> template at `rail_panels.go:299-305`. That range is the **cap-5 slice** of the
> tool-rows block; the actual `+N more` literal and `dimLabel` styling are at
> `:320-322`. The whole reusable block is `:299-323`. Tasks/spec must reference
> the literal at `:320-322`.

---

## 2. Architecture Decisions (ADRs)

### ADR-1 — Budget distribution formula: EQUAL split with deterministic front-loaded remainder + surplus redistribution

**Decision.** `rail.Render` distributes the total `height` budget **equally**
across the panels that will actually render, with two refinements that are
LOCKED here for determinism:

1. **Integer-division remainder is front-loaded** in `panelsFor` order. With
   `N` populated panels and a total content budget `H`, base share is `H / N`
   and remainder `R = H % N`. The **first `R` panels** (in `panelsFor` order)
   get `base + 1`; the rest get `base`. This is fully deterministic and stable
   for goldens — no map iteration, no float rounding.

2. **Surplus from under-full panels redistributes** to later panels in a single
   forward pass. If a panel's NATURAL height (its untruncated rendered height,
   measured in pass 1 — see ADR-3) is less than its assigned share, the unused
   rows are added back into the pool and re-spread (same front-loaded rule) over
   the panels not yet assigned. A panel that needs fewer rows than its share is
   given exactly its natural height; the surplus is **never wasted** while any
   later panel still wants more.

**Options considered.**

| Option                                              | Mechanism                                                | Determinism                         | UX at small h                                    | Verdict                                                                                                                                                                             |
| --------------------------------------------------- | -------------------------------------------------------- | ----------------------------------- | ------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Equal + front-loaded remainder + surplus reflow** | `H/N`, remainder to first panels, reflow surplus forward | Excellent (order-stable, no floats) | Fair — all visible, top panels get the spare row | **CHOSEN**                                                                                                                                                                          |
| Equal, remainder wasted                             | `H/N`, drop `H%N` rows                                   | Excellent                           | Worse — wastes up to N-1 rows                    | Rejected (wastes scarce rows)                                                                                                                                                       |
| Priority-weighted                                   | weight table → per-panel budget                          | Good                                | Better for "important" panels                    | Rejected — introduces a weight table + tuning surface for a problem equal-split already solves; weights are subjective and would need their own ADR + tests. YAGNI for this change. |

**Rationale.** Equal split is the minimum mechanism that satisfies the success
criterion ("stacked height never exceeds budget") with zero tuning surface.
Front-loading the remainder makes the spare rows land on the panels users see
first (`panelsFor` order is top-to-bottom), which is the intuitive place for the
extra row, and — critically — is **deterministic** so goldens are stable.
Surplus redistribution is nearly free given we already measure natural heights in
pass 1 (ADR-3) and prevents the common case (e.g. context-meter naturally needs
only 6 rows but was handed 9) from starving a hungry neighbor (telemetry with 12
tools). Priority-weighting is explicitly deferred: it is the runner-up hybrid
(exploration option e) and belongs to a follow-on, not here.

**`panelsFor` order = budget order.** The order returned by `panelsFor(screen)`
(`panels.go:35-55`) is authoritative for BOTH render order (top-to-bottom
stacking, unchanged) AND budget order (remainder front-loading + surplus reflow
direction). For `screenChat`: todolist → context-meter → telemetry → memory-peek.
For `screenDiff`: hunks-nav → rationale → impact → telemetry.

**SPEC-PHASE CONFIRM:** the spec must pin the remainder rule with a worked
example so the golden at `h=12` is unambiguous (see §5 worked example).

---

### ADR-2 — Per-panel minimum useful height + the truncation contract

**Decision — minimum useful height.** A panel needs at least **header + 1 data
row + 2 border = 4 rendered rows** to show anything meaningful. We pin
`panelMinHeight = 4` as a **single shared constant** (NOT per-panel) in
`panels.go`. Below 4 a panel cannot show a header AND one row inside a box.

Pure-(b) policy has **NO panel-drop**. So when the assigned budget is below the
minimum, the panel still renders — degenerately — per the **uniform per-panel
box-budget contract** (judgment-day fix; the original design had an arithmetic
error at budget==3):

```
maxContent = budget - 2   (rows available inside the border)
```

| Assigned budget | `maxContent` | What renders                                                                        |
| --------------- | ------------ | ----------------------------------------------------------------------------------- |
| `>= 4`          | `>= 2`       | header + up to `maxContent-1` data rows + (`+N more` if cut)                        |
| `== 3`          | `== 1`       | **header ONLY** — one content row → rendered box = 3 rows = budget ✓                |
| `<= 2`          | `<= 0`       | the panel renders `""` (cannot even fit a bordered header; contributes zero height) |

> **Why budget==3 is header-only (not header+"+N more"):** The border adds 2
> rows (top + bottom). At budget=3, `maxContent = 3 - 2 = 1`. The single
> content slot holds the header. Adding `+N more` would require 2 content rows
> → rendered box = 4 rows > budget 3 → overflow. The original design incorrectly
> stated "header + overflow notice + 2 border" which sums to 4 for a budget of 3.
> The corrected contract: budget==3 → header ONLY, no `+N more`.
>
> The `<= 2` case returns `""` rather than a broken half-box. This is the ONE
> place pure-(b) sheds a panel, and it is forced (a 2-row box is physically
> impossible: 2 border rows leave 0 content rows). It is NOT priority-drop — it
> is "this budget cannot hold a box." Because budgets only get this small at
> absurd terminal heights (h≈8 with 4 panels), and surplus reflow (ADR-1) tends
> to push usable rows toward earlier panels, this is an edge guard, not the
> normal path. The known graceful-degradation gap below per-panel minimums
> remains the deferred hybrid (option e) — see Risks.

**Decision — truncation contract (per panel).** Each panel computes:

```
inner      = width - 4                       (existing convention)
budget     = the rows assigned by rail.Render (ADR-1)
maxContent = budget - 2                      (rows inside the border)
```

- If `len(dataRows) <= maxContent-1`: render header + all data rows (no `+N
more`). This is the surplus case — the panel reports its natural height back
  (ADR-3) and reflow gives the spare to a neighbor.
- If `len(dataRows) > contentRowBudget`: render header + the **first
  `(contentRowBudget - 1)` data rows** + a final `+N more` row. Reserving one
  row for the notice is why we subtract 1: the `+N more` row itself consumes a
  content row. `N = len(dataRows) - (contentRowBudget - 1)`.

**Which rows are droppable / where `+N more` goes.** The header row (`rows[0]`)
is NEVER dropped — it is the panel's identity. Data rows are dropped from the
**bottom** (tail), preserving the most-relevant top rows, EXCEPT where a panel
already has a deterministic sort that defines "most relevant":

- **telemetryPanel** (`:233-269`): order is fixed — aggregate lines
  (tokens/cost/tools/[errors]) FIRST, then per-tool rows (count-desc, already
  capped 5), then per-subagent rows (first-seen, capped 3). The height clamp
  truncates this **already-assembled `rows` slice from the tail**, so the
  aggregate block survives first, then tools, then subagents. The panel's own
  cap-5 / cap-3 `+N more` notices are INDEPENDENT of the height `+N more` (a row
  may carry both notices in extreme cases; see ADR-4 layering note).
- **todolistPanel** (`:387-423`): after the hard cap (ADR-4), the remaining item
  rows truncate from the tail (drop the last items) with a height `+N more`.
- **contextMeterPanel** (`:612-671`): the bar + pct line are the panel's whole
  point; the 3 category rows (`sys/msg/tool`, `:660-666`) are the droppable
  tail. If the budget can't fit all category rows, drop them and show `+N more`.
  If the budget can't fit even bar+pct, this is the `==3` degenerate case
  (header + `+N more`). The bar is never split.
- **memoryPeekPanel** (`:966-995`): entry rows truncate from the tail; height
  `+N more` replaces / supplements the existing `maxRows=5` cap (see ADR-4).

**`+N more` text/style — REUSE the existing idiom verbatim.** Cite
`rail_panels.go:320-322`:

```go
overflowLine := fmt.Sprintf("  +%d more", overflow)
rows = append(rows, ansi.Truncate(p.styles.dimLabel.Render(overflowLine), inner, "…"))
```

The height-clamp `+N more` uses the SAME format string (`"  +%d more"`), the
SAME `dimLabel` style, and the SAME `ansi.Truncate(..., inner, "…")` wrap. This
makes the four panels visually consistent and lets the spec assert one exact
golden string. Extract a tiny pure helper to avoid four copies:

```go
// renderMoreRow returns the standard dim "+N more" overflow row, ANSI-truncated.
func renderMoreRow(n, inner int, s tuiStyles) string {
    return ansi.Truncate(s.dimLabel.Render(fmt.Sprintf("  +%d more", n)), inner, "…")
}
```

telemetry's existing inline overflow (`:320-322`) MAY be refactored to call it
(mechanical, golden-neutral) but that is optional and tasks-phase discretion.

**SPEC-PHASE CONFIRM:** the spec must state that the header row is mandatory and
that data rows truncate from the tail, and must pin the exact `+N more` string.

---

### ADR-3 — Two-pass render (measure natural height, assign budgets, re-render)

**Decision.** `rail.Render` uses a **two-pass** algorithm: render each panel once
at the full `height` to measure its NATURAL (untruncated) height, compute budgets
from those measurements (ADR-1 surplus reflow needs them), then re-render each
panel at its assigned budget. We do NOT add a `MinHeight()` / `PreferredHeight()`
interface method to `Panel`.

**Options considered.**

| Option                                      | Added interface surface                                 | Render cost @ 4 panels | Testability                                                     | Verdict                          |
| ------------------------------------------- | ------------------------------------------------------- | ---------------------- | --------------------------------------------------------------- | -------------------------------- |
| **Two-pass (render → measure → re-render)** | none — `Panel` stays `Render(w,h) string`               | 2× panel renders/frame | Excellent — measure = `lipgloss.Height(s)`, no new seam to mock | **CHOSEN**                       |
| One-pass `MinHeight()` probe                | new `MinHeight(width) int` on `Panel` (5+ implementers) | 1× render + 1 probe    | More seams to test; probe must stay in sync with Render         | Rejected                         |
| One-pass, no measure (blind equal split)    | none                                                    | 1× render              | Can't do surplus reflow (ADR-1)                                 | Rejected — wastes rows, worse UX |

**Rationale.** The `Panel` interface is `Render(width, height int) string`
(`rail.go:12-14`) and FOUR-plus types implement it. Adding `MinHeight`/
`PreferredHeight` widens that contract for every implementer (including
non-clamped panels like `modelPickerPanel`, `environmentPanel`) and creates a
**drift hazard**: a panel whose `MinHeight` disagrees with what `Render` actually
emits would mis-budget silently. Two-pass measures the REAL output
(`lipgloss.Height(rendered)` = `strings.Count(s,"\n")+1`), so there is no second
source of truth to keep in sync. The cost is 2× `Render` calls for ≤4 panels per
frame — these are pure string-builders over already-cached fields (no IO), so the
doubling is microseconds and the purity guarantee makes re-rendering provably
side-effect-free. Testability is the decider: two-pass needs no new mock surface;
the existing golden infra exercises it end-to-end.

**The algorithm (precise — this is what the spec freezes):**

```
func (r *rail) Render(screen, width, height) string:
    ids := panelsFor(screen)
    if len(ids) == 0 { return "" }

    # ---- PASS 1: measure natural heights of panels that WILL render ----
    # A panel that returns "" at full height contributes nothing (empty-until-data).
    type meas struct{ id panelID; natural int }
    populated := []meas{}
    for id in ids:
        p, ok := r.panels[id]; if !ok { continue }
        s := p.Render(width, height)           # full height = "give me your natural size"
        if s == "":                            # empty panel consumes no budget
            continue
        populated = append(populated, meas{id, lipglossHeight(s)})
    n := len(populated)
    if n == 0 { return "" }

    # ---- budget: total height minus the (n-1) inter-panel separator newlines ----
    # rail.Render appends "\n" after each block; n blocks stacked use n-1 separators
    # of vertical space inside the column. Reserve them so the column fits `height`.
    avail := height - (n - 1)
    if avail < 0 { avail = 0 }

    # ---- assign budgets: equal + front-loaded remainder + surplus reflow ----
    budgets := assignBudgets(populated, avail)   # see below

    # ---- PASS 2: re-render each at its budget, stack ----
    out := ""
    for i, m := range populated:
        p := r.panels[m.id]
        s := p.Render(width, budgets[i])         # budget < natural ⇒ panel truncates + "+N more"
        if s != "":
            out += s + "\n"
    return out

func assignBudgets(populated, avail) []int:
    n := len(populated)
    budgets := make([]int, n)
    remaining := avail
    # forward pass: give each panel min(its fair share of `remaining`, its natural).
    for i := 0; i < n; i++:
        panelsLeft := n - i
        base := remaining / panelsLeft
        rem  := remaining % panelsLeft
        share := base
        if rem > 0 { share = base + 1 }     # front-load the remainder onto earlier panels
        give := share
        if populated[i].natural < share {    # surplus: panel needs less than its share
            give = populated[i].natural       # surplus stays in `remaining` for later panels
        }
        budgets[i] = give
        remaining -= give
    return budgets
```

This forward-pass `assignBudgets` folds the front-loaded remainder AND the
surplus reflow into one loop: each panel takes the ceil-share of the
still-remaining pool, capped at its natural height; whatever it leaves stays in
`remaining` and is re-divided over the panels after it. It is O(n), deterministic,
and never assigns more than `avail` total (sum of `give` ≤ `avail` by
construction). A panel whose `give` lands `<= 2` renders `""` (ADR-2 degenerate
rule), so its rows fall out and the stacked total only shrinks — never overflows.

> **Purity note.** Pass 1 calls `Render` with the FULL `height`. A correct panel
> ignores oversize budgets (it never pads to fill), so `Render(width, height)`
> with a generous height yields the panel's natural untruncated output. This is
> already true today (panels never pad vertically). The clamp logic only ACTIVATES
> when budget < natural. So pass-1 measurement is just "render big, count lines."

**SPEC-PHASE CONFIRM:** the spec must pin (a) the `avail = height - (n-1)`
separator reservation and (b) the forward-pass surplus rule, with the §5 worked
example as the canonical golden anchor.

---

### ADR-4 — todolist hard cap is a FIXED constant applied BEFORE height truncation (cap-then-clamp)

**Decision.** `todolistPanel` gets a **fixed `todolistMaxItems = 10`** constant,
independent of the height budget, applied **inside `todolistPanel.Render` BEFORE**
the height-truncation step. Layering is **cap-then-clamp**:

1. **Cap (height-independent):** slice items to the first 10; if more exist, the
   cap produces its OWN `+N more` (`N = total - 10`).
2. **Clamp (height-dependent):** the (already ≤10) item rows then truncate to the
   `contentRowBudget` from ADR-2, producing a SECOND `+N more` if the height
   budget is even tighter than 10.

**Options considered.**

| Option                                          | Worst-case bound                                                      | Coupling                                                                                                                            | Verdict    |
| ----------------------------------------------- | --------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | ---------- |
| **Fixed cap 10, before clamp (cap-then-clamp)** | bounded at 10 items REGARDLESS of terminal height                     | cap is independent of layout math                                                                                                   | **CHOSEN** |
| Cap derived from height budget (clamp-only)     | bounded only by budget; unbounded `dataRows` still measured in pass 1 | couples item count to terminal height; pass-1 natural height of todolist could be huge (15 items → 19 rows) and skew surplus reflow | Rejected   |
| No cap, clamp-only                              | bounded by budget at render, but pass-1 NATURAL height is unbounded   | a 50-item list reports natural=53 in pass 1, distorting `assignBudgets` fairness                                                    | Rejected   |

**Rationale.** The proposal LOCKS the todolist cap as in-scope precisely because
budget distribution alone does not tame an unbounded panel: in two-pass (ADR-3),
pass 1 measures the panel's NATURAL height, and an uncapped 50-item todolist would
report a natural height of ~53, which — even though it gets truncated in pass 2 —
distorts the surplus-reflow fairness math and makes the panel look artificially
"hungry." Capping at a fixed 10 BEFORE measurement bounds the natural height at
`2 + 1 + 10 + 1(cap-more) = 14` rows max, keeping pass-1 measurements honest and
the worst-case rail bounded independent of terminal height. The cap is a product
decision (a 10-item visible todo is already long for a rail), not a layout one, so
it must NOT be derived from height. The two `+N more` notices can coexist (cap
overflow vs. height overflow); in practice the tighter of the two wins visually
because clamp runs second on the already-capped rows — if height truncation cuts
below 10, only the height `+N more` shows the true remaining count
(`total - shownData`). The spec must define `+N more` count as **total undisplayed
data items** so the two layers reconcile to one honest number when both fire.

> **Reconciliation rule (SPEC-PHASE CONFIRM):** when both cap and clamp would
> drop rows, render a SINGLE `+N more` where `N = totalItems - shownItems` (the
> true count of hidden items), not two stacked notices. The cleanest
> implementation computes `shownItems` after BOTH cap and clamp, then emits one
> `+N more` if `shownItems < totalItems`. This avoids a confusing "+5 more" above
> another "+3 more".

---

## 3. The `rail.Render` algorithm & the new height-arg flow

### How `height` now flows into each panel

Today: `layout.go:133` → `m.rail.Render(m.screen, rWidth, centerHeight)` →
`rail.go:51` `p.Render(width, height)` → **ignored** (`_ int`).

After: `layout.go` is UNCHANGED (it already passes `centerHeight`). `rail.Render`
intercepts `height`, runs the two-pass ADR-3 algorithm, and calls each panel's
`Render(width, budget)` with the panel's ASSIGNED budget (not the full height).
Each panel's signature changes from `Render(width, _ int)` to
`Render(width, height int)` and the body uses `height` as its `renderBudget`
(ADR-2). The `Panel` interface (`rail.go:12-14`) is UNCHANGED — it already
declares `Render(width, height int) string`; only the implementations stop
discarding the second arg.

### Panels affected

| Panel                      | File:line            | Change                                                        |
| -------------------------- | -------------------- | ------------------------------------------------------------- |
| `rail.Render`              | `rail.go:43-58`      | two-pass measure + `assignBudgets` + re-render                |
| `todolistPanel.Render`     | `rail_panels.go:387` | `todolistMaxItems=10` cap, then height truncation + `+N more` |
| `contextMeterPanel.Render` | `rail_panels.go:612` | drop category rows tail-first to budget + `+N more`           |
| `telemetryPanel.Render`    | `rail_panels.go:233` | truncate assembled `rows` tail to budget + `+N more`          |
| `memoryPeekPanel.Render`   | `rail_panels.go:966` | entry rows tail-truncate to budget + `+N more`                |
| `panels.go`                | new                  | `panelMinHeight=4`, `todolistMaxItems=10` consts              |
| `rail_panels.go`           | new                  | `renderMoreRow` pure helper                                   |

`modelPickerPanel`, `environmentPanel`, and other non-chat/diff panels need NO
change — they keep ignoring `height` (they are short and single-screen). The
clamp only engages for panels reached via `screenChat`/`screenDiff` budgets, and
a panel that ignores its budget simply reports its (small) natural height in pass
1 and renders identically in pass 2. No regression.

---

## 4. Purity / COW Analysis

| Concern             | Analysis                                                                                                                                                                                                                                          |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| New stored state?   | **None.** Budget is a per-frame computation in `rail.Render`. The todolist cap is a render-time slice (`items[:10]`), not a stored field. No new `copyRailWith` site.                                                                             |
| `Render` purity     | Pass 1 + pass 2 both call `Render` over cached fields only. Truncation reads `len(rows)` and slices — pure. No clock/IO/store.                                                                                                                    |
| COW snapshot safety | Slicing `p.list.Items[:n]` inside `Render` creates a sub-slice header on a read; it does NOT mutate the backing array. No prior snapshot is touched. (Contrast PR-b's telemetry map clone — that was a MUTATION hazard; truncation is read-only.) |
| Determinism         | `assignBudgets` is order-stable (no maps); telemetry/tool/subagent sorts already deterministic (count-desc/name-asc, first-seen). Goldens stable.                                                                                                 |

---

## 5. Worked example (canonical golden anchor)

`screenChat`, `width=32`, **`height=12`**, all four panels populated. Natural
heights measured in pass 1:

| Panel         | Natural rows | Notes                                                 |
| ------------- | ------------ | ----------------------------------------------------- |
| todolist      | 9            | 6 items (after cap 10): 2 border + 1 header + 6 items |
| context-meter | 8            | smart: 2 + header + bar + pct + 3 category = 8        |
| telemetry     | 7            | 2 + header + tokens/cost/tools + 2 tool rows          |
| memory-peek   | 5            | 2 + header + 2 entries                                |

`n=4`, `avail = 12 - (4-1) = 9`. Forward pass over `avail=9`:

- i=0 todolist: panelsLeft=4, base=2, rem=1 → share=3; natural 9 ≥ 3 → give 3. remaining=6.
- i=1 context-meter: panelsLeft=3, base=2, rem=0 → share=2; natural 8 ≥ 2 → give 2. remaining=4.
- i=2 telemetry: panelsLeft=2, base=2, rem=0 → share=2; natural 7 ≥ 2 → give 2. remaining=2.
- i=3 memory-peek: panelsLeft=1, base=2, rem=0 → share=2; natural 5 ≥ 2 → give 2. remaining=0.

Budgets `[3,2,2,2]`, sum 6, plus 3 separators = 9 ≤ 12. ✓

At budget 3 todolist renders **header ONLY** (`maxContent = 3-2 = 1`; one
content slot holds the header; adding `+N more` would yield 4 rows > budget 3 —
overflow). At budget 2 the other three hit the `<= 2` degenerate rule and
render `""`. Result: only todolist shows (header only), total 3 rendered rows +
0 separators (others empty) = 3 ≤ 12. ✓

> **Correction from original design:** The original §5 prose stated
> "header + overflow notice + 2 border" = 4 rows for a budget of 3. That
> arithmetic was wrong — it sums to 4 > 3. The judgment-day fix corrects this:
> budget==3 → `maxContent=1` → header only, no `+N more`. The golden at `h=12`
> pins the corrected output (header-only box, 3 rows).

This exposes the harsh-degradation reality at `h=12` with 4 panels and is
EXACTLY why the priority-drop hybrid (option e) is the deferred follow-on. A
roomier golden at `h=24` (avail=21) shows all four near-natural; `h=8` shows
the most aggressive clamp.

**Golden boundary set (SPEC-PHASE CONFIRM):** `h=8` (aggressive), `h=12`
(boundary, worked above), `h=24` (comfortable). Force `termenv.TrueColor` for
determinism. Reviewers verify each `+N more` lands exactly where content is cut.

---

## 6. Alternatives Rejected

- **Priority-weighted budget (ADR-1 runner-up).** Adds a weight table + tuning
  surface for a problem equal-split + surplus-reflow already solves. Deferred to
  the option-e hybrid follow-on.
- **`MinHeight()`/`PreferredHeight()` interface probe (ADR-3 runner-up).** Widens
  the `Panel` contract for 5+ implementers and introduces a drift hazard between
  the probe and actual `Render` output. Two-pass measures the real output.
- **todolist cap derived from height (ADR-4 runner-up).** Couples item count to
  terminal height and leaves pass-1 natural height unbounded, distorting surplus
  reflow. Fixed cap-then-clamp keeps measurements honest.
- **Rail scroll/viewport (exploration option d).** Breaks `View=pure(Model)`
  (offset is display state) and forces focus/keys onto a stateless rail.
  Rejected by the proposal.
- **Silent panel-drop (exploration option a).** Panels vanishing with no notice
  is poor UX. The only `""`-drop here is the forced `<=2` rows case, which is
  physically unavoidable, not a policy choice.

---

## 7. Risks (carried + design-specific)

**Carried from the proposal:**

1. **todolist must be capped before distribution** — mitigated by ADR-4
   cap-then-clamp (fixed 10, before pass-1 measurement).
2. **Two-pass doubles render work** — accepted: panels are pure string-builders
   over cached fields; 2× for ≤4 panels is microseconds (ADR-3 rationale).
3. **Very-short-terminal degradation is a known limit of pure-(b)** — at `h=12`
   with 4 panels, three panels render `""` (worked example §5). Graceful
   degradation below per-panel minimums is the deferred hybrid (option e).
4. **Golden-fixture churn** — new boundary goldens (`h=8/12/24`) regenerated
   deterministically (`termenv.TrueColor`); reviewers verify `+N more` placement.
5. **Pure-render / COW regressions** — truncation is read-only slicing; no new
   stored state, no new `copyRailWith` site (§4).

**Design-specific (new):**

6. **Remainder-allocation determinism.** `assignBudgets` MUST iterate the
   `populated` slice in `panelsFor` order, NEVER a map. Any map iteration in the
   budget path would make goldens flaky. Tasks: assert budget order with a
   table test independent of the golden.
7. **`avail = height - (n-1)` separator accounting.** Off-by-one here either
   overflows (forgot separators) or wastes rows (over-reserved). Pinned in ADR-3
   and §5; tasks must unit-test that `lipgloss.Height(rail.Render(...)) <= height`
   for the full height-boundary table (the core success criterion).
8. **todolist cap interacts with the two `+N more` layers.** Cap-overflow and
   clamp-overflow can both fire; the reconciliation rule (ADR-4) emits ONE
   `+N more` = `total - shown`. Tasks must test the both-fire case (e.g. 12
   items at a 3-row budget → single `+9 more`... computed as total−shown).
9. **Smart-strategy context-meter has variable height (6 vs 3 rows).** Its
   natural height in pass 1 differs by strategy, so the SAME terminal height
   yields different budgets for neighbors under smart vs legacy. Both golden
   strategies (`LegacyStrategy`, `SmartStrategy` — existing goldens) must be
   re-pinned at the boundary heights so this interaction is captured, not
   discovered later. This is the closest analog to the prior `tui-rail-panels`
   W1 stale-spec bug — the spec must reference §5 and the smart/legacy split
   explicitly.
10. **`screenDiff` shares the clamp but has different panels** (`{HunksNav,
Rationale, Impact, Telemetry}`, `panels.go:42`). Those three diff-only panels
    must tolerate a budget arg; if any currently pads vertically it would
    misreport natural height. Tasks: smoke-test a diff-rail golden at a boundary
    height (likely no change since most are empty-until-data, but must verify).

---

## 8. SPEC-PHASE CONFIRM checklist (so the spec aligns with this design)

1. Equal split + front-loaded remainder + forward surplus reflow; `panelsFor`
   order is authoritative (ADR-1).
2. `panelMinHeight=4`; degenerate rules: `==3` → header+`+N more`; `<=2` → `""`
   (ADR-2).
3. Truncation contract: header mandatory, data rows drop from tail, `+N more`
   exact string `"  +%d more"` in `dimLabel` via `renderMoreRow` (ADR-2).
4. Two-pass: `avail = height - (n-1)`; forward-pass `assignBudgets`; pass-1 = full
   height measure (ADR-3).
5. todolist: fixed `todolistMaxItems=10`, cap-then-clamp, single reconciled
   `+N more` = `total - shown` (ADR-4).
6. Goldens at `h=8/12/24`, smart AND legacy context-meter strategies, force
   `termenv.TrueColor`; core assertion `lipgloss.Height(railRender) <= height`.

---

## SDD session config

- **artifact_store:** openspec
- **strict TDD:** enabled (`make test`) — height-boundary tests + golden variants
  at `h=8/12/24`, both context-meter strategies
- **Charm:** v1 only (bubbletea/lipgloss v1, `x/ansi` width math)
