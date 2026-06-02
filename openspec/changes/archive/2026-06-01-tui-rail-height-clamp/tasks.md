# Tasks: tui-rail-height-clamp

> Artifact store: openspec. Delivery strategy: ask-on-risk. Strict TDD: `make test`.
> All files under `internal/tui/`. No edits outside that directory.

---

## Review Workload Forecast

| Field                                     | Value                         |
| ----------------------------------------- | ----------------------------- |
| Estimated changed lines (PR-a)            | ~180 (impl ~90, tests ~90)    |
| Estimated changed lines (PR-b)            | ~200 (impl ~80, tests ~120)   |
| Total estimated                           | ~380 across two PRs           |
| 400-line budget risk (per-slice)          | Low / Low                     |
| 400-line budget risk (combined single PR) | Medium–High (~380)            |
| Chained PRs recommended                   | Yes                           |
| Suggested split                           | PR-a → PR-b (stacked-to-main) |
| Delivery strategy                         | ask-on-risk                   |
| Chain strategy                            | stacked-to-main               |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal                                                                | Likely PR   | Notes                                                             |
| ---- | ------------------------------------------------------------------- | ----------- | ----------------------------------------------------------------- |
| PR-a | `assignBudgets` + two-pass `rail.Render` + consts + `renderMoreRow` | PR 1 → main | base: main; all four panels still ignore budget (height ≡ passed) |
| PR-b | Per-panel height truncation + `+N more` + todolist cap + goldens    | PR 2 → main | base: main (depends structurally on PR-a for `assignBudgets`)     |

---

## PR-a: Two-pass budget engine + shared infra

Files touched: `panels.go`, `rail.go`, `rail_panels.go`.

Covers: TR-HC-1 (budget algorithm), TR-0-A, TR-0-B, TR-0-D.

### RED — failing tests first

- [x] **a.1** In `rail_panels_test.go`, add `TestAssignBudgets_WorkedExample_h12`: call `assignBudgets` with natural heights `[9,8,7,5]` and `avail=9`; assert `budgets == [3,2,2,2]` (spec scenario TR-HC-1 "Separator reservation — worked example at h=12"). Run `make test` → RED.

- [x] **a.2** In `rail_panels_test.go`, add `TestAssignBudgets_SurplusReflow`: call `assignBudgets` with natural heights `[3,2,10]` and `avail=10`; assert `budgets[0]==3, budgets[1]==2, budgets[2]==5` (spec scenario TR-HC-1 "Surplus reflow — under-full panel donates rows"). Run `make test` → RED.

- [x] **a.3** In `rail_panels_test.go`, add `TestAssignBudgets_SumNeverExceedsAvail`: table-driven over several `(naturals, avail)` pairs; assert `sum(budgets) <= avail` for all (spec TR-HC-1 ADR-3 invariant). Run `make test` → RED.

- [x] **a.4** In `rail_panels_test.go`, add `TestRailRender_HeightGuarantee_ChatScreen`: construct model with all four `screenChat` panels populated; for each `h` in `{8, 12, 24}` assert `lipgloss.Height(m.rail.Render(screenChat, 32, h)) <= h`; force `lipgloss.SetColorProfile(termenv.TrueColor)` + `t.Cleanup` (spec scenario TR-HC-1 "Core height guarantee"). Run `make test` → RED.

- [x] **a.5** In `rail_panels_test.go`, add `TestRailRender_HeightGuarantee_DiffScreen`: same as a.4 for `screenDiff` at `h` in `{8, 12}` (spec scenario TR-HC-1 "screenDiff clamped by same code path"). Run `make test` → RED.

- [x] **a.6** In `rail_panels_test.go`, add `TestRailRender_EmptyPanel_ExcludedFromBudget`: panels A (populated), B (empty), C (populated); assert `n=2` separator reservation (`avail = h - 1`) and B contributes no rows to output (spec scenario TR-HC-1 "Empty panel excluded from budget distribution"). Run `make test` → RED.

- [x] **a.7** In `rail_panels_test.go`, add `TestRenderMoreRow_ExactFormat`: call `renderMoreRow(6, 28, styles)` and assert: raw format contains `"  +6 more"`, style is `dimLabel`, result is `ansi.Truncate`-safe (spec scenario TR-HC-2 "+N more uses exact format and style"). Run `make test` → RED.

### GREEN — minimal implementation

- [x] **a.8** In `panels.go`, add package-level constants `panelMinHeight = 4` and `todolistMaxItems = 10` (spec TR-HC-3 + design ADR-2/ADR-4). Place alongside existing panel-ID consts.

- [x] **a.9** In `rail_panels.go`, add pure helper `renderMoreRow(n, inner int, s tuiStyles) string` — exact body: `ansi.Truncate(s.dimLabel.Render(fmt.Sprintf("  +%d more", n)), inner, "…")` (design ADR-2, spec TR-HC-2). Export nothing; place before `wrapPanelBox`.

- [x] **a.10** In `rail.go`, add package-level function `assignBudgets(naturals []int, avail int) []int` implementing the forward-pass algorithm from design ADR-3 (equal split via ceiling division, surplus reflow). Signature simplified from struct to []int (test scaffolding fix). No maps; pure integer arithmetic.

- [x] **a.11** In `rail.go`, rewrite `rail.Render` (`rail.go:43-58`) to implement the two-pass algorithm: pass-1 measure natural heights into a `populated` slice (skip empty panels), compute `avail = height - (n-1)`, call `assignBudgets`, pass-2 re-render each at its budget (spec TR-HC-1). Rail-level safety clamp added as PR-a bridge until PR-b implements per-panel truncation. `layout.go` is UNCHANGED.

### REFACTOR

- [x] **a.12** Run `make test` → all PR-a tests GREEN. `TestAssignBudgets_SumNeverExceedsAvail` covers `avail=0` and `n=1` edge cases. No file outside `internal/tui/` was modified. `golangci-lint` clean.

---

## PR-b: Per-panel truncation + todolist cap + golden determinism

Files touched: `rail_panels.go`, `rail_panels_test.go`, `internal/tui/testdata/` (golden files).

Covers: TR-HC-2, TR-HC-3, TR-HC-4, TR-0-C (extended), TR-0-D.

### RED — failing tests first

- [x] **b.1** In `rail_panels_test.go`, add `TestTodolist_HeightTruncation_BudgetGe4`: `todolistPanel` with 8 items, `Render(32, 6)` (`contentRowBudget=3`, show 2 data rows); assert header present, exactly 2 data rows shown, output contains `"  +6 more"` (spec scenario TR-HC-2 "Budget >= 4 — partial data rows + +N more"). Run `make test` → RED.

- [x] **b.2** In `rail_panels_test.go`, add `TestTodolist_HeightTruncation_Budget3`: `todolistPanel` with 6 items, `Render(32, 3)`; assert header present, 0 data rows, output contains `"  +N more"` (spec scenario TR-HC-2 "Budget == 3 — header + +N more only"). Run `make test` → RED.

- [x] **b.3** In `rail_panels_test.go`, add `TestTodolist_HeightTruncation_BudgetLe2`: populated `todolistPanel`, `Render(32, 2)`; assert return is `""`, no panic (spec scenario TR-HC-2 "Budget <= 2 — returns \"\""). Run `make test` → RED.

- [x] **b.4** In `rail_panels_test.go`, add `TestTodolist_Cap_15Items_Budget14`: 15 items, `Render(32, 14)`; assert exactly 10 item rows shown, output contains `"  +5 more"`, no second `+N more` line (spec scenario TR-HC-3 "Cap alone"). Run `make test` → RED.

- [x] **b.5** In `rail_panels_test.go`, add `TestTodolist_Cap_12Items_Budget6`: 12 items, `Render(32, 6)` (`contentRowBudget=3`, cap→10, clamp shows 2); assert exactly 2 item rows, output contains `"  +10 more"`, exactly ONE `+N more` line (spec scenario TR-HC-3 "Both cap and clamp fire — single reconciled +N more"). Run `make test` → RED.

- [x] **b.6** In `rail_panels_test.go`, add `TestTodolist_Cap_Budget3_With12Items`: 12 items, `Render(32, 3)`; assert header present, 0 item rows, output contains `"  +12 more"` (spec scenario TR-HC-3 "Budget == 3 with cap"). Run `make test` → RED.

- [x] **b.7** In `rail_panels_test.go`, add `TestContextMeter_HeightTruncation_Budget3_SmartStrategy`: smart-strategy panel (6 natural rows), `Render(32, 3)`; assert header present, 0 data rows, output contains `"  +N more"` (spec scenario TR-HC-2 "Budget == 3 — header + +N more only"). Run `make test` → RED.

- [x] **b.8** In `rail_panels_test.go`, add `TestContextMeter_HeightTruncation_BudgetLe2`: populated context-meter, `Render(32, 2)`; assert return is `""` (spec scenario TR-HC-2 "Budget <= 2"). Run `make test` → RED.

- [x] **b.9** In `rail_panels_test.go`, add `TestTelemetry_HeightTruncation_HeaderMandatory`: `telemetryPanel` with 10 assembled rows, `Render(32, 4)` (`contentRowBudget=1`); assert `rows[0]` (header) present, 0 data rows, output contains `"  +N more"` (spec scenario TR-HC-2 "Header mandatory — never dropped"). Run `make test` → RED.

- [x] **b.10** In `rail_panels_test.go`, add `TestTelemetry_HeightTruncation_BudgetLe2`: populated telemetry panel, `Render(32, 2)`; assert return is `""` (spec scenario TR-HC-2 "Budget <= 2"). Run `make test` → RED.

- [x] **b.11** In `rail_panels_test.go`, add `TestMemoryPeek_HeightTruncation_BudgetGe4`: memory-peek with 5 entries, `Render(32, 6)`; assert header present, truncated tail, output contains `"  +N more"` (spec scenario TR-HC-2 "Budget >= 4 — partial data rows + +N more"). Run `make test` → RED.

- [x] **b.12** In `rail_panels_test.go`, add `TestMemoryPeek_HeightTruncation_BudgetLe2`: populated memory-peek, `Render(32, 2)`; assert return is `""` (spec scenario TR-HC-2 "Budget <= 2"). Run `make test` → RED.

- [x] **b.13** In `rail_panels_test.go`, add `TestRailRender_h12_WorkedExample_Golden_Legacy`: all four `screenChat` panels populated (natural heights matching §5 of design), `termenv.TrueColor` forced + `t.Cleanup`; call `rail.Render(screenChat, 32, 12)` with legacy context-meter; assert `lipgloss.Height(result) <= 12`; assert only todolist renders (header + `"  +6 more"`); use `golden.RequireEqual` against `TestRailRender_h12_WorkedExample_Legacy.golden` (spec scenario TR-HC-4 "h=12 golden matches worked example"). Run `make test` → RED.

- [x] **b.14** In `rail_panels_test.go`, add `TestRailRender_h12_WorkedExample_Golden_Smart`: same as b.13 with smart context-meter; assert golden differs from legacy, `lipgloss.Height <= 12` (spec scenario TR-HC-4 "Smart-strategy golden differs from legacy at h=12"). Run `make test` → RED.

- [x] **b.15** In `rail_panels_test.go`, add `TestRailRender_h8_Golden_Legacy` and `TestRailRender_h8_Golden_Smart`: `h=8`, both strategies; assert `lipgloss.Height(result) <= 8`, `golden.RequireEqual` against `TestRailRender_h8_*_Legacy/Smart.golden` (spec scenario TR-HC-4 "h=8 — most aggressive clamp"). Run `make test` → RED.

- [x] **b.16** In `rail_panels_test.go`, add `TestRailRender_h24_Golden_Legacy` and `TestRailRender_h24_Golden_Smart`: `h=24`, both strategies; assert `lipgloss.Height(result) <= 24`, `golden.RequireEqual` against `TestRailRender_h24_*_Legacy/Smart.golden` (spec scenario TR-HC-4 "h=24 — comfortable height"). Run `make test` → RED.

- [x] **b.17** In `rail_panels_test.go`, flag any existing goldens that will change due to height-awareness (most likely `TestContextMeter_Render_LegacyStrategy_Golden` and `TestContextMeter_Render_SmartStrategy_Golden` if `Render(w, 0)` now processes `height=0`). Add explicit `// GOLDEN-CHURN: regenerate after b.18–b.22` comment so reviewers know which files change intentionally.

### GREEN — minimal implementation

- [x] **b.18** In `rail_panels.go`, update `todolistPanel.Render` (`rail_panels.go:387`): rename `_ int` to `height int`; apply `todolistMaxItems` cap (`items[:min(len(items), todolistMaxItems)]`); implement `contentRowBudget = height - 2 - 1` gating (`<= 2 → ""`, `== 3 → header + renderMoreRow`, `>= 4 → head + min(contentRowBudget-1, len) data rows + renderMoreRow if cut`); compute single reconciled `N = totalItems - shownItems` for `+N more` (spec TR-HC-2 + TR-HC-3 + design ADR-4).

- [x] **b.19** In `rail_panels.go`, update `contextMeterPanel.Render` (`rail_panels.go:612`): rename `_ int` to `height int`; implement budget contract — `<= 2 → ""`, `== 3 → header + renderMoreRow(allDataRows, ...)`, `>= 4 → bar+pct always + category rows tail-first to budget + renderMoreRow if cut` (spec TR-HC-2, design ADR-2 contextMeterPanel rules).

- [x] **b.20** In `rail_panels.go`, update `telemetryPanel.Render` (`rail_panels.go:233`): rename `_ int` to `height int`; after assembling the `rows` slice (existing logic unchanged), truncate tail to `contentRowBudget - 1` and append `renderMoreRow` if cut; gate `<= 2 → ""`, `== 3 → header + renderMoreRow` (spec TR-HC-2, design ADR-2 telemetryPanel rules; existing cap-5 + cap-3 notices remain independent).

- [x] **b.21** In `rail_panels.go`, update `memoryPeekPanel.Render` (`rail_panels.go:966`): rename `_ int` to `height int`; truncate entry rows tail-first to `contentRowBudget - 1` + `renderMoreRow` if cut; gate `<= 2 → ""`, `== 3 → header + renderMoreRow` (spec TR-HC-2, design ADR-2 memoryPeekPanel rules; reconciles with existing `maxRows=5` cap).

- [x] **b.22** Regenerate changed golden files: run `go test ./internal/tui/... -update` to regenerate `TestContextMeter_Render_LegacyStrategy_Golden.golden`, `TestContextMeter_Render_SmartStrategy_Golden.golden`, and any other goldens that shift due to height-awareness. Record the exact list of regenerated files in the PR description (no silent golden churn — each file must be explicitly acknowledged).

- [x] **b.23** Generate new boundary goldens by running `go test ./internal/tui/... -update` for the six new golden tests (b.13–b.16): `TestRailRender_h12_WorkedExample_Legacy/Smart.golden`, `TestRailRender_h8_Legacy/Smart.golden`, `TestRailRender_h24_Legacy/Smart.golden`. Force `termenv.TrueColor` is already in each test.

### REFACTOR

- [x] **b.24** Run `make test` without `-update`; assert ALL tests GREEN including the six new boundary goldens and the height-guarantee table tests (a.4, a.5). Assert `lipgloss.Height(rail.Render(screenChat, 32, h)) <= h` holds for `h ∈ {8, 12, 24}`. Verify the h=12 result matches the §5 worked example (only todolist renders, header + `"  +6 more"`). Confirm no file outside `internal/tui/` was modified (`git diff --name-only`). Run `golangci-lint` on changed files.

---

## Cross-cutting notes (apply-time reminders)

- **`panelMinHeight` / `todolistMaxItems` placement**: both constants in `panels.go` alongside `panelID` consts — NOT inside `rail_panels.go` (design ADR-2/ADR-4 mandate `panels.go`).
- **`renderMoreRow` placement**: package-private pure helper in `rail_panels.go`, before `wrapPanelBox`. Receives `n int, inner int, s tuiStyles` — no pointer receiver.
- **`assignBudgets` placement**: package-private function in `rail.go`, immediately before `rail.Render`. Slice parameter (`populated []struct{...}`) — NOT a method.
- **`avail = height - (n-1)` arithmetic**: off-by-one is the primary overflow vector; pinned by a.1 and a.4. Clamp `avail` to `0` when negative (absurd terminal heights).
- **Golden determinism**: every new golden test MUST call `lipgloss.SetColorProfile(termenv.TrueColor)` and `t.Cleanup(func() { lipgloss.SetColorProfile(termenv.HasDarkBackground) })` — carry the tui-rail-panels pattern verbatim.
- **Single reconciled `+N more`**: for todolist, compute `shownItems` AFTER both cap and clamp have run; emit one `renderMoreRow` iff `shownItems < totalItems`. Never emit two stacked `+N more` rows.
- **Existing telemetry `+N more` at `rail_panels.go:320-322`**: OPTIONAL refactor to call `renderMoreRow` (mechanical, golden-neutral). Do not block PR-b on this.
- **`screenDiff` panels** (`HunksNav, Rationale, Impact, Telemetry`): telemetry is updated in b.20; the three diff-only panels continue ignoring `height` (they are short and return `""` when empty). The smoke test (a.5) catches any regression.
- **`layout.go` is UNCHANGED**: height already flows in via `centerHeight`; no wiring change needed.
