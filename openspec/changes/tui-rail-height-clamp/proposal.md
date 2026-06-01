# Proposal: tui-rail-height-clamp

> Follow-on to `tui-rail-panels` (archived 2026-06-01).
> Closes the height-overflow risk that change explicitly deferred (engram #603).
> **TUI-only change.** No `internal/agent` / `internal/notify` source edits.

---

## Intent

`rail.Render` has no per-panel height clamp. It stacks every populated panel
box with `out += s + "\n"` and NEVER consults the `height` budget that
`layout.go` already threads in; all four chat panels declare the height arg as
`_ int` and ignore it. Because `layout.go:139`
`lipgloss.JoinHorizontal(lipgloss.Top, center, railRendered)` PADS (never
clips) the shorter column, an over-tall rail expands `mainRow` and silently
pushes the InputBar and footer below the visible terminal bottom — layout
corruption, not a panic.

The `tui-rail-panels` proposal logged this as a known, unmitigated risk: it
relied on panels returning `""` until they had data so the 4th panel "adds zero
height until then." That mitigation evaporates the moment the rail is populated.

### Worst-case quantification (from exploration)

Fixed chrome = 8 rows (`centerHeight = h - 8`).

| Panel             | Min rows | Worst-case                |
| ----------------- | -------- | ------------------------- |
| todolistPanel     | 4        | Unbounded (15 items → 19) |
| contextMeterPanel | 5        | 9 (smart)                 |
| telemetryPanel    | 6        | 18                        |
| memoryPeekPanel   | 4        | 8                         |

At `h=24` → 16 rows available; worst-case all four populated ≈ **54 rows**.
Even a modestly populated rail (~19 rows) already overflows. At `h=40` → 32
available, the worst case STILL overflows. `todolistPanel` is the single
unbounded contributor — it renders every item with no cap
(`rail_panels.go:410`).

**Success looks like:** the rail's rendered height never exceeds its budget at
any terminal height; the InputBar and footer remain visible; truncation is
explicit (a dim `+N more` row), never silent; and the behavior is pinned by
deterministic golden tests at boundary heights.

---

## Scope

### In

- **`rail.Render` becomes height-aware** (`rail.go:43-58`): distribute the
  `height` budget across the _populated_ panels and pass each panel its slice
  of the budget instead of forwarding the full `height` unused.
- **Each chat panel becomes height-aware**: `telemetryPanel`,
  `todolistPanel`, `contextMeterPanel`, `memoryPeekPanel` truncate their inner
  rows to `(budget − borders)` and append a dim `+N more` row when content is
  cut. The existing telemetry "+N more" pattern (`rail_panels.go:299`) is the
  template.
- **`todolistPanel` hard cap** (~10 items + "+N more"): it is currently
  unbounded; a cap is added in THIS change so its worst-case is bounded
  regardless of terminal height.
- **`screenDiff` is covered**: the clamp lives in `rail.Render`, which serves
  BOTH `screenChat` and `screenDiff` (`panels.go:39-42` — both lists end in
  `panelTelemetry`), so the diff rail is clamped by the same code path with no
  extra wiring.
- **Tests**: height-boundary unit cases (panel truncation at small budgets) +
  golden render variants at small heights.

### Out

- **Rail scroll / viewport (exploration option d).** Rejected: a scroll offset
  is display state, which breaks `View = pure(Model)` and forces focus/keys
  onto a stateless rail — disproportionate to the problem.
- **Silent panel-drop (exploration option a).** Rejected: panels vanishing with
  no indication is poor UX; all panels stay visible under the chosen policy.
- **Priority-drop hybrid (exploration option e).** A possible FOLLOW-ON for
  very-short terminals (see Risks); not in this change.
- **Any visual restyle** (borders, chips, colors, new panels) — orthogonal.
- **Any file outside `internal/tui/`.** No `internal/agent` / `internal/notify`
  edits. If a backend accessor seems necessary during apply, STOP and flag it.

---

## Capabilities

- **Extends `tui-rail-panels`** — adds the rail height-budget contract:
  `rail.Render` distributes a height budget across populated panels, and every
  chat panel is height-aware (truncate inner rows to budget, append `+N more`
  when cut). The pure-Render / COW invariants are unchanged and reaffirmed.
- No existing spec's semantics are redefined; the panel contract is _extended_
  with a height-budget clause, not rewritten.

---

## Approach (high level)

**Policy = exploration Option (b): per-panel height truncation + "+N more".**

1. **Budget distribution in `rail.Render`.** Count the panels that will
   actually render (populated, non-empty), then derive a per-panel height
   budget from the total `height` and hand each panel its slice. Panels that
   return `""` consume no budget. The exact distribution formula is a DESIGN
   decision (see deferred list) — the proposal locks only that distribution
   happens here and that the summed rendered height never exceeds `height`.

2. **Panels truncate to their budget.** Each chat panel stops declaring height
   as `_ int`, computes its inner row budget as `budget − borders`, truncates
   its content rows to that budget, and appends a dim `+N more` row when rows
   were cut — reusing the established telemetry "+N more" idiom and
   `wrapPanelBox`. `memoryPeekPanel` and `telemetryPanel` already self-limit;
   this generalizes the pattern under an explicit budget.

3. **`todolistPanel` hard cap.** Independent of the height math, cap todolist at
   ~10 items + "+N more" so the single unbounded panel can no longer dominate
   the rail even before distribution kicks in.

This is the minimum-friction policy: the `height` param is ALREADY threaded
`layout → rail → panels`; activating it follows an existing in-repo pattern.
Pure-render and COW are untouched; golden tests stay deterministic at fixed
heights.

### Invariants carried forward (MUST hold)

- `Render` is a PURE function of cached `Model` fields — no clock, no IO, no
  store reads in any `Render` path.
- All state mutations go through `copyRailWith` (copy-on-write); prior `Model`
  snapshots are never mutated.
- ANSI-aware width math via `x/ansi`; styles from `tuiStyles` (no inline hex).
- Charm v1 only.

---

## Deferred to DESIGN (open — NOT decided here)

- **Budget distribution formula**: equal (`height / N`) vs priority-weighted.
- **Per-panel minimum useful heights** (suggested in exploration: contextMeter
  5, telemetry 6, todolist 4, memoryPeek 4) — below which a panel renders
  degenerately.
- **Two-pass vs one-pass**: render → measure → re-render, vs probing a
  `MinHeight()` interface method on `Panel` in a single pass.

---

## Affected Areas

| File                               | Change                                                              |
| ---------------------------------- | ------------------------------------------------------------------- |
| `internal/tui/rail.go`             | `rail.Render` distributes the height budget across populated panels |
| `internal/tui/rail_panels.go`      | four panel `Render` methods become height-aware; todolist hard cap  |
| `internal/tui/rail_panels_test.go` | height-boundary unit cases + golden variants at small heights       |
| `internal/tui/panels.go`           | possible min-height / budget constants (design-dependent)           |
| `internal/tui/*.golden`            | new / updated golden fixtures at boundary heights                   |

Read-only reference (no edits): `layout.go`, `model.go`, `screen_chat.go`.
No source files outside `internal/tui` are modified.

---

## Risks

1. **`todolistPanel` still needs its own cap.** Budget distribution alone does
   not bound an unbounded panel before distribution; the explicit ~10-item hard
   cap (in scope) is the mitigation. If the cap is skipped, todolist can still
   dominate.
2. **Two-pass doubles render work.** If design picks render → measure →
   re-render, each rail draw runs panel `Render` twice. Pure functions make
   this safe but it doubles CPU per frame. A one-pass `MinHeight()` probe avoids
   it — design tradeoff.
3. **Very-short-terminal degradation is a known limit of pure-(b).** At `h=16`
   (~2 rows/panel after chrome), four panels each get a near-useless budget.
   Option (b) keeps all visible but tiny; the priority-drop **hybrid (option e)
   is a possible follow-on** for graceful degradation below per-panel minimums —
   explicitly OUT of this change.
4. **Golden-fixture churn.** New boundary-height goldens must be regenerated
   deterministically (force `termenv.TrueColor`); reviewers verify the `+N more`
   rows appear exactly where content is cut.
5. **Pure-render / COW regressions.** Adding a height arg must not tempt any
   panel into reading live state to "decide" what to drop — truncation operates
   only on already-cached rows. Reaffirmed against the existing panel contract.

---

## Success Criteria

- The rail's rendered height NEVER exceeds its height budget at any terminal
  height (boundary heights `h=8`, `h=12`, plus an aggressive-clamp case).
- InputBar and footer remain visible at all tested heights (no `mainRow`
  overflow).
- Truncation is always explicit: a dim `+N more` row appears whenever a panel's
  content is cut; no panel silently disappears.
- `todolistPanel` is bounded (~10 items + "+N more") independent of terminal
  height.
- `screenDiff` is clamped by the same `rail.Render` code path as `screenChat`.
- All panels still obey `View = pure(Model)`: no live reads in `Render`, all
  mutations via `copyRailWith`.
- Deterministic golden tests pin the output at boundary heights; `make test`
  green.
- No file outside `internal/tui` is modified.

---

## SDD session config

- **artifact_store:** openspec
- **strict TDD:** enabled (`make test`) — height-boundary tests + golden
  variants at small heights
- **Charm:** v1 only (bubbletea/lipgloss v1, `x/ansi` width math)
