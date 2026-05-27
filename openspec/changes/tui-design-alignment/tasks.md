# Tasks: TUI Design Alignment — Phase 1 (tui-visual-tokens)

> **Scope**: Phase 1 ONLY — backend-free visual token alignment.
> No timestamps/usernames, no panel restructuring (Phase 2), no new screens (Phase 3).
> **TDD**: strict RED→GREEN per task. Write failing test first, then implementation.
> **Delivery**: 3 chained PRs (stacked-to-main). Each ≤400 lines.

---

## Review Workload Forecast

| Field                   | Value                                                                              |
| ----------------------- | ---------------------------------------------------------------------------------- |
| Estimated changed lines | 850–1 100 (goldens are large binary-ish text blobs)                                |
| 400-line budget risk    | High                                                                               |
| Chained PRs recommended | Yes                                                                                |
| Suggested split         | PR 1a → PR 1b → PR 1c (1b and 1c are independent of each other; both depend on 1a) |
| Delivery strategy       | ask-on-risk                                                                        |
| Chain strategy          | stacked-to-main                                                                    |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal                                                                        | Likely PR | Notes                                                                                                            |
| ---- | --------------------------------------------------------------------------- | --------- | ---------------------------------------------------------------------------------------------------------------- |
| 1    | Palette (4→13 tokens), glyph constants, thread glyphs, ToolLine `▸ view`    | PR 1a     | Foundational; chat golden regenerates here; base = main                                                          |
| 2    | Square borders + `colorLine`, `panelHeader` helper, swap all `◈`→`── TITLE` | PR 1b     | Pure header/border; depends on PR 1a tokens; base = PR 1a                                                        |
| 3    | Welcome ASCII logo + tagline, footer hint-set model + render                | PR 1c     | Welcome golden regenerates here; depends on PR 1a tokens only; can be reviewed in parallel with 1b; base = PR 1a |

---

## PR 1a — Palette + Glyphs + ToolLine expand

**Base branch**: `main`
**Files touched**: `internal/tui/styles.go`, `internal/tui/components_thread.go`,
`internal/tui/styles_test.go`, `internal/tui/components_thread_test.go`,
`internal/tui/testdata/TestModel_View_ChatScreen_Golden.golden`

### PR 1a — RED: write failing tests first

- [x] 1a.1 In `internal/tui/styles_test.go`, add a table-driven test asserting all 13 (+ amber, red, green, pink) color constants exist with exact hex values — including new ones (`colorBGElev`, `colorBGDeep`, `colorBGPanel`, `colorInk`, `colorInkSoft`, `colorInkMuted`, `colorInkFaint`, `colorInkGhost`, `colorLine`, `colorLineSoft`, `colorLineStr`) and fixed ones (`colorAmber=#e3b67a`, `colorPink=#d67b9e`, `colorRed=#e38775`). Assert `#cdd6f4` is absent from all constant values. **[Req: Full 13-token color palette — scenarios: All 13 constants present, Wrong amber replaced, Wrong pink replaced, topBar ink corrected]**
- [x] 1a.2 In `internal/tui/styles_test.go`, add a test asserting `newTuiStyles()` `.amber` foreground = `#e3b67a`, `.pink` foreground = `#d67b9e`, `.topBar` foreground = `#eae5d8`. **[Req: Full 13-token color palette — scenarios: Wrong amber replaced, Wrong pink replaced, topBar ink corrected]**
- [x] 1a.3 In `internal/tui/components_thread_test.go`, add a table test for `MsgDaimon.Render`: assert rendered output contains `⫶`, does NOT contain `δ`. Golden test for chat screen: assert it fails because current output has `δ`. **[Req: Speaker glyphs — MsgDaimon glyph scenario]**
- [x] 1a.4 In `internal/tui/components_thread_test.go`, add a test for `MsgUser.Render`: assert output contains `▌`, does NOT contain `"you  "`. **[Req: Speaker glyphs — MsgUser glyph scenario]**
- [x] 1a.5 In `internal/tui/components_thread_test.go`, add two tests for `ToolLine` expand hint: (a) stub `ToolLine` with output exceeding display budget + `expanded=false` → rendered output contains `▸ view`; (b) output within budget or `expanded=true` → output does NOT contain `▸ view`. **[Req: ToolLine expand — truncated/non-truncated scenarios]**
- [x] 1a.6 Run `make test` — confirm all new test cases fail (RED). Do not proceed to GREEN before this.

### PR 1a — GREEN: implement

- [x] 1a.7 In `internal/tui/styles.go`: add 9 new color constants (`colorBGElev`, `colorBGDeep`, `colorBGPanel`, `colorInk`, `colorInkSoft`, `colorInkMuted`, `colorInkFaint`, `colorInkGhost`, `colorLine`, `colorLineSoft`, `colorLineStr`, `colorGreen`). Fix `colorAmber` (`#ffb347`→`#e3b67a`), `colorPink` (`#f48fb1`→`#d67b9e`), `colorRed` (ANSI 9→`#e38775`). **[Req: Full 13-token palette]**
- [x] 1a.8 In `internal/tui/styles.go` `newTuiStyles()`: repoint `topBar` foreground to `colorInk`; add new slots `green`, `inkSoft`; repoint `amber`, `pink`, `errStyle` to fixed hex constants; repoint `dimLabel` to `colorInkMuted` (explicit foreground, not `Faint(true)`). Add comment block documenting deferred rgba tints (`accentBg`≈`#12201d`, `amberBg`≈`#1c1813`, `redBg`≈`#1c1311`) for Phase 2. **[Req: Full 13-token palette]**
- [x] 1a.9 In `internal/tui/styles.go` (or `const.go`): add package-level glyph constants: `glyphDaimon = "⫶"`, `glyphUser = "▌"`, `glyphPrompt = "›"`, `glyphExpand = "▸"`. **[Req: Speaker glyphs, ToolLine expand]**
- [x] 1a.10 In `internal/tui/components_thread.go`: replace `δ` literal with `glyphDaimon`; replace `"you  "` literal with `glyphUser`; apply `s.inkSoft` style to user glyph, `s.accent` to daimon glyph. **[Req: Speaker glyphs]**
- [x] 1a.11 In `internal/tui/components_thread.go`: stop discarding `wasTruncated`; when `wasTruncated && !tl.expanded`, append `s.hint.Render("  " + glyphExpand + " view")` on its own line, using `ansi.Truncate` defensively. **[Req: ToolLine expand]**
- [x] 1a.12 Scan all `.go` files under `internal/tui/` (excluding `styles.go`) for hex color patterns; confirm none exist outside `styles.go`. **[Req: No inline hex literals scenario]**

### PR 1a — Golden regeneration

- [x] 1a.13 Regenerate chat golden: `go test ./internal/tui -run 'TestModel_View_ChatScreen_Golden' -update`. Manually eyeball `internal/tui/testdata/TestModel_View_ChatScreen_Golden.golden` diff: confirm `⫶`/`▌` glyphs, new color bytes, `▸ view` hint, NO `δ`/`"you  "` regressions.
- [x] 1a.14 Re-run `make test` WITHOUT `-update` — all tests must pass (GREEN). Confirm `#cdd6f4` does not appear anywhere in `internal/tui/` (grep check).

---

## PR 1b — Square borders + `── TITLE` panel headers

**Base branch**: PR 1a branch
**Files touched**: `internal/tui/styles.go` (helper + border slot), `internal/tui/rail_panels.go`,
`internal/tui/screen_tools.go`, `internal/tui/screen_sessions.go`,
`internal/tui/styles_test.go`, `internal/tui/rail_panels_test.go` (or equivalent panel render tests)

### PR 1b — RED: write failing tests first

- [x] 1b.1 In `internal/tui/styles_test.go`, add a test: render a bordered box using `tuiStyles.panelBorder`; assert output contains `┌` (U+250C), does NOT contain `╭` (U+256D). **[Req: Square borders — Panel border is normal scenario]**
- [x] 1b.2 In `internal/tui/styles_test.go`, add a test: assert `tuiStyles.panelBorder` border foreground = `#22242c`. **[Req: Square borders — Border foreground is line token scenario]**
- [x] 1b.3 In `internal/tui/styles_test.go` (or a new `panel_header_test.go`), add a test: `s.panelHeader("telemetry")` returns a string that, when ANSI-stripped, equals `"── TELEMETRY"`. Assert it does NOT contain `◈`. **[Req: Panel headers in ── TITLE form]**
- [x] 1b.4 Run `make test` — confirm new cases fail (RED).

### PR 1b — GREEN: implement

- [x] 1b.5 In `internal/tui/styles.go`: add `panelBorder` style slot using `lipgloss.NormalBorder()` (square corners), `BorderForeground(colorLine)`, `Padding(0,1)`. Repoint existing `border` slot (currently `RoundedBorder`) to `NormalBorder` + `colorLine`. Repoint `inputBarStyle` border to `NormalBorder` + `colorLineStrong`. Repoint `paletteBox` border to `NormalBorder` + **`colorAccent`** (command palette border = accent per design, tui-components.jsx:441: "1px solid ${TUI.accent}"; do NOT use colorLine — this regressed twice). **[Req: Square borders]**
- [x] 1b.6 In `internal/tui/styles.go`: add `func (s tuiStyles) panelHeader(title string) string` that returns `"── " + strings.ToUpper(title)` rendered with `s.dimLabel`. **[Req: Panel headers in ── TITLE form]**
- [x] 1b.7 In `internal/tui/rail_panels.go`: replace all 9 `◈ <lowercase>` header sites with `s.panelHeader("...")` calls. **[Req: Panel headers]**
- [x] 1b.8 In `internal/tui/screen_tools.go`: replace 1 `◈ <lowercase>` header site with `s.panelHeader("...")`. **[Req: Panel headers]**
- [x] 1b.9 In `internal/tui/screen_sessions.go`: replace 1 `◈ <lowercase>` header site with `s.panelHeader("...")`. **[Req: Panel headers]**
- [x] 1b.10 Run `make test` — all tests pass (GREEN). Check that no rounded border `╭` or `◈` glyph appears in any panel render test output.

---

## PR 1c — Welcome ASCII logo + tagline + footer hint sets

**Base branch**: PR 1a branch (independent of 1b; can be reviewed in parallel)
**Files touched**: `internal/tui/layout.go`, `internal/tui/components_shell.go`,
`internal/tui/layout_test.go` or `internal/tui/model_welcome_test.go`,
`internal/tui/components_shell_test.go`,
`internal/tui/testdata/TestModel_View_WelcomeScreen_Golden.golden`

### PR 1c — FIRST: verify footer hints against design source

- [ ] 1c.0 **VERIFY footer hints for screens 04/05/06 against the design files BEFORE implementing.** Read `docs/tui-design/daimon/project/tui-screens-b.jsx` (the `TUIFooter hints={[...]}` blocks for screens 4, 5, 6) and reconcile against what the spec states. Record any discrepancies as inline comments in the implementation. Known findings to confirm:
  - Screen 04 outer `TUIFooter` (`tui-screens-b.jsx:153–157`): `esc`/close palette · `/`/search prefix · `?`/help — differs from spec's `↑↓ select · ↵ run · esc close · ⇥ autocomplete` (the spec inferred from the inner palette overlay footer). Verify which is correct for the outer screen footer.
  - Screen 05 `TUIFooter` (`tui-screens-b.jsx:319–325`): `space`/toggle enabled · `↵`/open detail · `a`/add MCP server · `d`/remove · `/`/filter — differs from spec's `↑↓ select · ↵ toggle · f filter · a add-MCP`. Verify.
  - Screen 06 `TUIFooter` (`tui-screens-b.jsx:492–497`): `↵`/resume thread · `n`/new from this · `d`/delete · `m`/change model · `/`/filter — mostly matches spec except label differences. Verify.
  - Align implementation to the **design source**, not the spec; note any delta in a code comment.

### PR 1c — RED: write failing tests first

- [ ] 1c.1 In `internal/tui/model_welcome_test.go` (or `layout_test.go`): add a test rendering the welcome screen at width=80, assert output contains `▄▄▄▄▄` (first distinctive ASCII logo line) and `speak, and daimon listens.`. Assert non-welcome screen render does NOT contain `▄▄▄▄▄`. **[Req: Welcome ASCII logo — both scenarios]**
- [ ] 1c.2 In `internal/tui/components_shell_test.go`: add a test for `footerHints.Render` on the welcome screen; assert ANSI-stripped output contains `⇥`, `/commands`, `⌃R`, `resume last`, `⌃C`, `exit`. **[Req: Footer hint sets — Welcome footer scenario]**
- [ ] 1c.3 In `internal/tui/components_shell_test.go`: add a test for the chat screen footer; assert ANSI-stripped output contains `⇥ switch panel` (or `switch agent`) and `⌃P`. **[Req: Footer hint sets — Chat footer scenario]**
- [ ] 1c.4 Add table-driven tests for all remaining screen hint sets (slash, tools, sessions) using verified values from step 1c.0. Use `t.Run` per screen.
- [ ] 1c.5 Run `make test` — confirm all new cases fail (RED).

### PR 1c — GREEN: implement

- [ ] 1c.6 In `internal/tui/layout.go`: define `var welcomeLogo = []string{ ... }` (8 lines from `tui-screens-a.jsx:9–17`). In `renderWelcomeCenter`: replace single-line `accent.Render("⫶ daimon")` + tagline with: (a) `artWidth` guard (`ansi.StringWidth(welcomeLogo[0])`; fall back to `⫶ daimon` if `width < artWidth`), (b) each logo line centered via `centerText(line, width)` with `s.accent`, (c) tagline `"speak, and daimon listens."` with `s.hint` (italic), (d) recompute `padTop` against new taller block height. **[Req: Welcome ASCII logo]**
- [ ] 1c.7 In `internal/tui/components_shell.go`: define `type footerHint struct { key, label string }`. Replace flat per-screen footer strings in `hintsForScreen()` (or equivalent) with `[]footerHint` returns per `screenState`. Use verified hint data from step 1c.0. **[Req: Footer hint sets]**
- [ ] 1c.8 In `internal/tui/components_shell.go`: update `footerHints.Render`: each hint = `s.accent.Render(h.key) + " " + s.dimLabel.Render(h.label)`, hints joined by `"  "` (two spaces), `ansi.Truncate` to width. Append right-aligned italic `"daimon listens."` with `s.hint` when width allows (drop first under truncation pressure). **[Req: Footer hint sets — hint style + separator]**
- [ ] 1c.9 Run `make test` — confirm new direct tests pass (GREEN) before touching goldens.

### PR 1c — Golden regeneration

- [ ] 1c.10 Regenerate welcome golden: `go test ./internal/tui -run 'TestModel_View_WelcomeScreen_Golden' -update`. Manually eyeball `internal/tui/testdata/TestModel_View_WelcomeScreen_Golden.golden` diff: confirm ASCII art block, tagline, footer hints, new color bytes. Confirm no prior `"⫶ daimon"` single-line heading remains.
- [ ] 1c.11 Re-run `make test` WITHOUT `-update` — all tests must pass (GREEN). No `◈` or old footer strings in welcome output.

---

## Cross-PR notes (do not implement — reference only)

- `dashStyles` in `dashboard.go` / `mcp_manage.go` is intentionally untouched (ADR-1).
- No `Model.Update()` changes, no new message types, no backend calls.
- `· time` and username on `MsgUser`/`MsgDaimon` are deferred to `tui-backend-seams`.
- rgba tint backgrounds (`accentBg`, `amberBg`, `redBg`) are deferred to Phase 2; nearest-hex approximations documented in `styles.go` comment.
