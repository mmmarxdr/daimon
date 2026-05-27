# Design — TUI Design Alignment (Phase 1: Visual Token Alignment)

> **Scope guard**: This document designs **Phase 1 ONLY** — backend-free visual alignment of the
> embedded Bubble Tea TUI. No backend seam, no Screen 03/07 interaction, no Phase 2/3 structural
> panels. See proposal §Decisions (resolved 2026-05-26): "This change implements Phase 1 ONLY."
> **Inputs**: `proposal.md`, `exploration.md` (verified file:line gap analysis), design bundle
> `docs/tui-design/daimon/project/tui*.jsx`.
> **Standards**: Charm v1; single root model; centralized `tuiStyles`; ANSI width via `x/ansi`;
> never hardcode colors inline; subtract border+padding before sizing children.

## The answer (TL;DR)

Phase 1 is a **style-layer and glyph-layer realignment** plus two localized render fixes. The
architecture does NOT change: the single root `Model`, the imperative `Render(w)` sub-components,
and the `tuiStyles` threading all stay exactly as they are. We:

1. Extend the central color palette from 4 to **13 tokens** (exact hex from `tui.jsx:5–27`), add
   matching `tuiStyles` style slots, and fix the 4 wrong/leaked Catppuccin colors.
2. Add a **box-drawing border + `── TITLE` header** style pair, centralized in `tuiStyles`, and a
   `panelFrame` helper consumed by every panel.
3. Move the brand glyphs (`⫶`, `▌`) into named **package constants** and thread them through the
   thread components.
4. Embed the **welcome ASCII logo + italic tagline** in `renderWelcomeCenter`, measured with `x/ansi`.
5. Replace the flat footer strings with a **per-screen hint-set data model** + a render helper that
   matches the design's `key label` chips.
6. Restore the **ToolLine `▸ view` expand hint** that is currently computed-then-discarded.

Everything is deterministic rendered output, so the test strategy is: **golden files for the two
composed screens** (welcome, chat) that already have goldens, plus **direct `Render(w)` string
assertions** for the new helpers and tokens. No backend, no `Model.Update()` behavior changes.

## Architecture approach

**Pattern: unchanged.** This is the established `internal/tui` architecture (single root `Model`,
sub-components as plain structs with `Render(width)`/`SetX`, styles threaded by value). Phase 1 is a
**decoration pass over the existing seams** — we are correcting the _values_ flowing through
`tuiStyles` and the _glyphs/strings_ the renderers emit, not restructuring control flow.

**Layering (where each concern lives):**

| Layer                  | File                                                      | Phase 1 responsibility                                  |
| ---------------------- | --------------------------------------------------------- | ------------------------------------------------------- |
| Color tokens (data)    | `styles.go` constants                                     | 13 hex constants, single source of truth                |
| Style slots (lipgloss) | `styles.go` `tuiStyles`                                   | one style per semantic role; the ONLY place hex is read |
| Glyph constants (data) | `styles.go` (or `const.go`)                               | `glyphDaimon`, `glyphUser`, `glyphPrompt`               |
| Border/header helper   | `styles.go` + small helper fn                             | `panelFrame(title, body)` + box border style            |
| Thread components      | `components_thread.go`                                    | consume glyphs + new styles                             |
| Shell components       | `components_shell.go`                                     | footer hint-set model, input placeholder                |
| Layout                 | `layout.go`                                               | welcome ASCII logo + tagline                            |
| Rail/screen headers    | `rail_panels.go`, `screen_tools.go`, `screen_sessions.go` | swap `◈ x` → `── X` via helper                          |

**Boundary that must NOT be crossed in Phase 1:** no `notify.Event` field reads beyond what already
exists, no new `Model.Update()` message types, no store/agent calls. If a design element needs a
data source the impl does not have (timestamps, username, per-tool token maps), Phase 1 renders the
**static/glyph portion only** and the dynamic portion is explicitly deferred (see §Deferred).

## 1. Token model — the 13-token palette

### Decision

Add **9 new color constants** to the existing 4 in `styles.go`, fix the 3 wrong values, and map each
to a `tuiStyles` slot. Keep the existing constant-naming convention (`colorX`). All hex stays in the
constant block; `newTuiStyles()` remains the only function that reads them.

### Exact values (verbatim from `tui.jsx:5–27`)

| Token (design) | Hex       | Constant name     | Status vs impl                  |
| -------------- | --------- | ----------------- | ------------------------------- |
| bg             | `#0e0f13` | `colorBG`         | keep (exists)                   |
| bgElev         | `#15171d` | `colorBGElev`     | new                             |
| bgDeep         | `#0a0b0f` | `colorBGDeep`     | new                             |
| bgPanel        | `#11131a` | `colorBGPanel`    | new                             |
| ink            | `#eae5d8` | `colorInk`        | new (replaces topBar `#cdd6f4`) |
| inkSoft        | `#c2bca9` | `colorInkSoft`    | new                             |
| inkMuted       | `#7a7465` | `colorInkMuted`   | new                             |
| inkFaint       | `#4a4438` | `colorInkFaint`   | new                             |
| inkGhost       | `#2c2a25` | `colorInkGhost`   | new (used by `dim` overlay)     |
| line           | `#22242c` | `colorLine`       | new (border color)              |
| lineSoft       | `#1a1c22` | `colorLineSoft`   | new                             |
| lineStrong     | `#2e3038` | `colorLineStrong` | new (input border)              |
| accent         | `#5dbfa7` | `colorAccent`     | keep (exists)                   |
| amber          | `#e3b67a` | `colorAmber`      | **fix** `#ffb347` → `#e3b67a`   |
| red            | `#e38775` | `colorRed`        | **fix** ANSI `9` → hex          |
| green          | `#7aba8a` | `colorGreen`      | new                             |
| pink           | `#d67b9e` | `colorPink`       | **fix** `#f48fb1` → `#d67b9e`   |

### rgba tints → nearest-color approximation

The design's `accentDim/accentBg/amberBg/redBg` are **rgba tints over a dark bg** used as CSS
`background` fills (e.g. `rgba(93,191,167,0.07)`). Terminals cannot alpha-composite. Per proposal
decision "rgba tints / glow → nearest-color approximation":

- **Do NOT add token constants for the `*Bg`/`*Dim` rgba values in Phase 1.** They are background
  fills for boxed regions (mode pill, code-block bg) that belong to Phase 2 structural panels.
- Where Phase 1 needs a "soft accent" (e.g. the input border vs the brighter `accent`), use the
  already-present `colorLineStrong` for borders and `colorAccent` for the prompt — no tint needed.
- Document this in a `styles.go` comment block so a future phase knows the rgba values were
  intentionally deferred, not forgotten. Pre-computed nearest hex (for the future phase, NOT added
  now): `accentBg` over `#0e0f13` ≈ `#12201d`; `amberBg` ≈ `#1c1813`; `redBg` ≈ `#1c1311`.

### Style-slot mapping (`tuiStyles`)

Add/repoint these slots (semantic role → token). Existing slot names are reused where possible to
minimize call-site churn:

| Slot                 | Foreground               | Notes                                         |
| -------------------- | ------------------------ | --------------------------------------------- |
| `topBar`             | `colorInk`               | was `#cdd6f4`; background stays `colorBG`     |
| `label`              | `colorInk`               | primary text, bold                            |
| `dimLabel`           | `colorInkMuted`          | was `Faint(true)` only → explicit muted color |
| `hint`               | `colorInkFaint` + italic | stage directions                              |
| `accent`             | `colorAccent`            | unchanged                                     |
| `amber`              | `colorAmber`             | now `#e3b67a`                                 |
| `pink`               | `colorPink`              | now `#d67b9e`                                 |
| `green` (new slot)   | `colorGreen`             | success/positive (e.g. done states)           |
| `errStyle`           | `colorRed`               | was terminal `9`; now hex `#e38775`           |
| `inkSoft` (new slot) | `colorInkSoft`           | user glyph `▌`, secondary text                |
| `dim`                | `colorInkGhost`          | overlay backdrop                              |

**Gotcha (verified):** `dimLabel` is currently `Faint(true)` with no color, and it is consumed in
~15 call sites across `rail_panels.go`/`screen_tools.go`/`screen_sessions.go`. Repointing it to an
explicit `colorInkMuted` foreground changes rendered output everywhere → **golden + width-assertion
churn**. This is expected and is the single largest blast-radius edit in Phase 1.

### ADR-1: keep `dashStyles` untouched

`dashboard.go`/`mcp_manage.go` use their own `dashStyles` and do **not** read the `colorAccent`/
`colorBG` tokens (verified in proposal §Out of Scope and grep: `dashboard.go:60` defines its own
border). **Rejected alternative:** unifying both style systems now — that expands scope into the
legacy dashboard, which is out of scope, and risks regressions in a surface we are not realigning.

## 2. Border + header styling

### Decision

The design panel (`tui-components.jsx:201–227`) is a **square box-drawing border colored with
`line`**, with a header row `── TITLE` (uppercase, muted) separated from the body by a border-bottom.
Implement this as:

1. A centralized **`panelBorder` style** in `tuiStyles`: `Border(lipgloss.NormalBorder())` (square
   corners `┌┐└┘`, vs the current `RoundedBorder()` `╭╮╰╯`) with
   `BorderForeground(colorLine)` and `Padding(0,1)`.
2. A **`panelHeader(title string) string` helper** that returns `── ` + `strings.ToUpper(title)`
   rendered with `dimLabel` (muted). It lives next to `tuiStyles` (a method `func (s tuiStyles)
panelHeader(title string) string`) so every panel calls `s.panelHeader("telemetry")` instead of
   inlining `accent.Render("◈ telemetry")`.

### Where it lives, how sub-components consume it

- **Header helper**: replaces all 11 `◈ <lowercase>` sites (`rail_panels.go` ×9, `screen_tools.go`
  ×1, `screen_sessions.go` ×1). Each becomes `s.panelHeader("...")`. The helper is the single place
  the `──` rule + uppercase + color live.
- **Border**: Phase 1 introduces the `panelBorder` style slot and **repoints the existing rounded
  borders** (`border`, `inputBarStyle`, `paletteBox`) to square + `colorLine`/`colorLineStrong`.
  - `inputBarStyle`: square border, `BorderForeground(colorLineStrong)` (design input uses
    `lineStrong`, `tui.jsx:233`), prompt `›` stays `accent`.
  - `paletteBox`: square border, `colorLine`.
- **Wrapping panel bodies in the box is Phase 2** (proposal: rail visual boxing = Phase 2). Phase 1
  changes the **header glyph/case + border style + border color**; it does NOT restructure the rail
  layout math to wrap each panel in a full box (that touches width budgeting across every panel and
  is explicitly Phase 2 in the proposal's Affected Areas table).

### ADR-2: `NormalBorder()` not a custom `lipgloss.Border{}`

Charm v1 ships `lipgloss.NormalBorder()` = square `─│┌┐└┘`. **Rejected alternative:** a hand-rolled
`lipgloss.Border{}` literal — unnecessary, error-prone, and the design's `1px solid` square maps
exactly to `NormalBorder()`.

### Off-by-N note

`NormalBorder()` + `Padding(0,1)` steals the same 4 columns as `RoundedBorder()` (1 border + 1 pad
each side). The existing `layout.go:14` off-by-N math (`inputHeight = 3`, `Width(width-2)`,
`ti.Width = width-4`) is unchanged because the border _thickness_ is identical. **Verified**: no
width-budget edit needed for the border swap alone.

## 3. Glyph constants

### Decision

Promote the brand glyphs to named package-level constants in `styles.go` (co-located with the color
constants, since both are "design tokens"):

```text
glyphDaimon = "⫶"   // MsgDaimon header, topBar brand (tui.jsx:154,294)
glyphUser   = "▌"   // MsgUser header             (tui.jsx:275)
glyphPrompt = "›"   // input prompt (already the inputBarSentinel "› ")
```

### How threaded

- `components_thread.go` `MsgDaimon.Render`: `prefix := glyphDaimon + " "` styled with `s.accent`
  (design: daimon glyph + label are `accent`, `tui.jsx:294–295`). **Removes the `δ` literal at
  `components_thread.go:131`.**
- `MsgUser.Render`: `prefix := glyphUser + " "` styled with `s.inkSoft` (design: `▌` is `inkSoft`,
  label is `ink`, `tui.jsx:275–276`). **Removes the `"you  "` literal at `components_thread.go:102`.**
- `inputBarSentinel` already = `"› "`; keep it but reference `glyphPrompt` so all glyphs are in one
  place. `topBar` already renders `⫶` as the brand slot (set via `SetData("⫶", ...)` in tests) — no
  change needed there for Phase 1.

### ADR-3: name/timestamp are NOT added in Phase 1

The design header is `▌ name · time` / `⫶ daimon speaks · time`. **`MsgUser` has no `name` field and
no timestamp** (verified: all 7 construction sites pass only `text`+`styles`). Event timestamps are a
flagged **backend dependency** (proposal §Backend dependencies). Phase 1 renders **glyph + static
label only** (`▌` + the message, `⫶ daimon` header). The `· time` and dynamic username land with the
backend-seams change. **Rejected alternative:** faking a `time.Now()` at render time — non-deterministic,
breaks golden tests, and misrepresents data.

## 4. Welcome ASCII logo + italic tagline

### Decision

Embed the 8-line ASCII art (`tui-screens-a.jsx:9–17`) + tagline "speak, and daimon listens."
(`:37`) into `renderWelcomeCenter` (`layout.go:124`), replacing the current single-line
`accent.Render("⫶ daimon")` + `"your embedded AI agent"`.

- Store the art as a **package-level `[]string` constant** (`welcomeLogo`) co-located in `layout.go`.
- Render each line with `s.accent`; the design applies `textShadow` glow — **not reproducible in a
  terminal**, approximated by accent color only (proposal decision: recreate aesthetic, not DOM).
- Tagline rendered with `s.hint` (italic, faint/inkSoft) — the design uses Fraunces italic
  `inkSoft`; terminals have no serif, so italic + soft color is the faithful approximation.
- Footer hints below the tagline (`⇥ /commands`, `⌃R resume last`, `⌃C exit`) are handled by the
  **footer hint-set** (§5), not hardcoded in the center.

### ANSI width considerations (`x/ansi`)

The art lines contain box-drawing + block glyphs (`▄█▐│┌┐└┘`) — **all width-1 in a monospace
terminal**, but they MUST be measured with `ansi.StringWidth`, never `len()` (the existing
`centerText` already uses `ansi.StringWidth` — reuse it per line). Two correctness rules:

1. **Center each art line independently** through `centerText(line, width)` so the block stays
   visually centered; do NOT center the joined multi-line string (that measures the longest line and
   left-pads the rest inconsistently).
2. **Guard for narrow terminals**: the art is ~67 cols wide. If `width < artWidth`, fall back to the
   current single-line `⫶ daimon` logo so an 80-col-minus-rail or small terminal does not wrap the
   art into garbage. Compute `artWidth` once via `ansi.StringWidth(welcomeLogo[0])`.

### Off-by-N / height

The welcome golden runs at 80×24 with no rail. The art (8 lines) + tagline + hints must fit
`centerHeight` (height − topBar − footer − input = 24 − 1 − 1 − 3 = 19). 8+blank+1+blank+1 ≈ 11
lines — fits. The vertical-centering `padTop` math stays but is recomputed against the taller block
(`padTop = (height - blockHeight) / 2`, clamped ≥ 0).

## 5. Footer hint sets

### Decision

Replace the flat per-screen strings in `footerHints.hintsForScreen()` with a **structured hint
model** + a render helper that matches the design's `key label   key label` chips
(`tui.jsx:180–198`, where each hint is `{k, l}` rendered as accent-key + muted-label).

```text
type footerHint struct { key, label string }   // e.g. {"/", "commands"}
```

- `hintsForScreen()` returns `[]footerHint` per `screenState`.
- `footerHints.Render` joins them: each hint = `s.accent.Render(key) + " " + s.dimLabel.Render(label)`,
  separated by two spaces, then `ansi.Truncate` to width. The trailing italic "daimon listens."
  (`tui.jsx:194`) is appended right-aligned with `s.hint` when width allows (drop it first under
  truncation pressure).

### Per-screen hint data (from design, verified file:line)

| Screen   | Hints (key → label)                                    | Source                             |
| -------- | ------------------------------------------------------ | ---------------------------------- |
| welcome  | `⇥`/commands, `⌃R`/resume last, `⌃C`/exit              | `tui-screens-a.jsx:43–45`          |
| chat     | `/`/commands, `⇥`/switch agent, `⌃P`/palette, `?`/help | exploration §01 design footer      |
| diff     | `↑↓`/scroll hunks, `q`/back, `⌃C`/quit                 | keep (Phase 3 screen, footer only) |
| slash    | `↑↓`/navigate, `↵`/run, `esc`/close                    | keep                               |
| tools    | `↑↓`/navigate, `esc`/back                              | keep                               |
| sessions | `↑↓`/navigate, `↵`/resume, `esc`/back                  | keep                               |
| error    | `esc`/back to chat, `⌃C`/quit                          | keep                               |

**Phase 1 faithfully aligns welcome + chat** (the two screens with verified design footers). Diff/
slash/tools/sessions/error footers are migrated to the **same `[]footerHint` structure** for
consistency but keep their current semantics (their full design alignment is Phase 2/3).

### ADR-4: hint model is per-screen data, not a keymap registry

**Rejected alternative:** deriving hints from a central keymap. The TUI has no keymap registry today
(hints are hand-authored strings). Building one is scope creep. Phase 1 keeps hints as static
per-screen data, just _structured_ so the renderer can color keys vs labels like the design.

## 6. ToolLine `▸ view` expand hint

### Decision

Restore the discarded affordance. `components_thread.go:277–287` computes `wasTruncated` then
**throws it away** (`_ = wasTruncated`). When `wasTruncated && !tl.expanded`, append a second line
`s.hint.Render("  ▸ view")` (indented under the tool name) — matching the design's `▸ view` expand
hint (exploration §02: "computed `wasTruncated` then discarded").

- The `▸` glyph is a new local constant (`glyphExpand = "▸"`) or reuse a literal in the one site;
  prefer the constant for consistency with §3.
- When `tl.expanded`, the full name is shown (existing branch) and no `▸ view` hint is rendered.
- **No state-machine change**: `ToolLine.expanded` already exists and is already toggled elsewhere;
  Phase 1 only makes the _collapsed-and-truncated_ state render the hint. This is a pure `Render`
  change — no `Update` edit.

### Width note

The hint is on its own line, so it does not compete with the stats budget on the main line. Measure
the hint with `ansi.StringWidth` only if it could exceed `width` (it won't at `"  ▸ view"` = 8 cols),
but truncate defensively with `ansi.Truncate` to honor the ANSI-width rule.

## 7. Test strategy (strict TDD, `make test`)

### Golden files vs direct assertions

| Change                                  | Test kind                                          | Rationale                                                                                            |
| --------------------------------------- | -------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| Welcome ASCII logo + tagline            | **golden** (`TestModel_View_WelcomeScreen_Golden`) | composed multi-line layout; whole-screen render is the contract                                      |
| Chat thread (`⫶`/`▌`/ToolLine `▸ view`) | **golden** (`TestModel_View_ChatScreen_Golden`)    | composed thread; existing golden already covers MsgUser/MsgDaimon/ToolLine                           |
| 13 tokens defined + correct hex         | **direct** (`styles_test.go`)                      | assert constant values + `tuiStyles` slot non-empty; no rendering                                    |
| `glyphDaimon`/`glyphUser` constants     | **direct**                                         | assert `MsgDaimon.Render` contains `⫶` and NOT `δ`; `MsgUser.Render` contains `▌` and NOT `"you"`    |
| `panelHeader` helper                    | **direct**                                         | assert output `== "── TELEMETRY"` (uppercase + rule), ANSI-stripped                                  |
| Footer hint-set render                  | **direct** (`components_shell_test.go`)            | assert welcome footer contains `/commands` and the accent-keyed structure; ANSI-strip then substring |
| Border style = square                   | **direct**                                         | assert `tuiStyles.panelBorder` renders a `┌`/`─` corner, not `╭`                                     |
| ToolLine `▸ view`                       | **direct** (`components_thread_test.go`)           | construct a truncated unexpanded `ToolLine`, assert `Render` contains `▸ view`; expanded → absent    |

**Rule applied (go-testing standard):** TUI _rendered composed output_ → golden file; _unit-level
component/string behavior_ → direct `Render(w)` assertion with `ansi.Strip` for substring checks.
Use table-driven `t.Run` for the token table and the per-screen hint sets.

### Determinism

- Goldens already run at fixed 80×24 with fixed `SetData` and a fixed thread (no `time.Now`, no
  randomness). Phase 1 introduces **no** non-deterministic data (we explicitly do NOT add live
  timestamps — §3). So goldens stay deterministic.
- The braille spinner in the chat golden uses `ToolLine{state: toolDone}` (no spinner frame), so the
  golden has no animated state. Keep new ToolLine golden cases in `toolDone`/`toolError` states.
- ANSI escapes ARE part of the golden bytes (lipgloss color codes). Because we change color _values_,
  the golden bytes change — this is the expected, reviewed churn (see §Risks).

### `-update` workflow

The repo uses `github.com/charmbracelet/x/exp/golden` (`golden.RequireEqual`), which writes
`testdata/<TestName>.golden` when `-update` is passed (verified `golden_test.go:24`,
`screen_chat_test.go`). Workflow per go-testing standard:

1. Write/adjust the test (RED) — assert new expected content via direct tests first.
2. Implement.
3. Regenerate goldens once: `go test ./internal/tui -run 'Golden' -update` (or via the repo's
   `make test` + `-update` path).
4. **Manually eyeball the diff** of both `.golden` files to confirm the visual change is intended
   (new `⫶`/`▌`, ASCII art, square borders, new colors) and nothing regressed.
5. Re-run **without** `-update` to confirm stability: `make test`.

### Golden files needing (re)generation

- `internal/tui/testdata/TestModel_View_WelcomeScreen_Golden.golden` — logo + tagline + footer + ink color.
- `internal/tui/testdata/TestModel_View_ChatScreen_Golden.golden` — `⫶`/`▌` glyphs + ToolLine + color tokens.

No new golden files are required for Phase 1 (the two composed screens cover the visual surface); new
behavior is covered by direct tests to keep the golden set small and reviewable.

## 8. PR slicing hint for the tasks phase

Phase 1 touches ~6 files and is **forecast to exceed 400 lines** (palette + slots + glyphs + border/
header helper + footers + welcome art + ToolLine, plus regenerated goldens which are large). With
`delivery_strategy = ask-on-risk`, recommend **3 chained PRs**, each independently shippable and
each ≤400 lines, in dependency order:

| PR                         | Theme                                                                                              | Files                                                                                            | Why it stands alone                                                | Est. risk                                                         |
| -------------------------- | -------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------ | ----------------------------------------------------------------- |
| **1a — Palette + glyphs**  | 13 tokens, fixed hex, new `tuiStyles` slots, glyph constants, `⫶`/`▌` in thread, ToolLine `▸ view` | `styles.go`, `components_thread.go`, `styles_test.go`, `components_thread_test.go`, chat golden  | Foundational; chat golden regenerates here; no layout math touched | **High golden churn** (chat golden + dimLabel color blast radius) |
| **1b — Borders + headers** | square `NormalBorder` + `colorLine`, `panelHeader` helper, swap all `◈`→`── TITLE`                 | `styles.go` helper, `rail_panels.go`, `screen_tools.go`, `screen_sessions.go`                    | Pure header/border swap; depends on 1a's tokens                    | Medium (11 header sites; sessions/tools/chat render assertions)   |
| **1c — Welcome + footers** | ASCII logo + tagline, footer hint-set model + render                                               | `layout.go`, `components_shell.go`, `golden_test.go`, `components_shell_test.go`, welcome golden | Welcome golden regenerates here; depends on 1a tokens              | Medium (welcome golden churn)                                     |

**Chaining rationale:** 1b and 1c both depend on 1a's tokens/slots but are independent of each other,
so they can be stacked on 1a in parallel review. Splitting the two golden regenerations into separate
PRs (1a = chat, 1c = welcome) keeps each golden diff small and reviewable rather than one giant
binary-ish diff. The orchestrator should surface this to the user at the tasks-phase `ask-on-risk`
gate.

## Deferred (explicitly NOT Phase 1)

These appear in the design but require data the impl lacks or are Phase 2/3 per the proposal:

- `· time` timestamps on MsgUser/MsgDaimon, dynamic username — **backend dependency** (event timestamps).
- rgba tint background fills (mode pill bg, code-block bg) — Phase 2 structural panels.
- Wrapping each rail panel body in a full box — Phase 2 rail boxing (width-math heavy).
- TopBar per-slot colors, input hint-chips + mode badge, reasoning `pondered for Xs` italic with real
  duration — Phase 2.
- `glow`/`textShadow` — not terminal-reproducible; approximated by accent color (decision recorded).

## Risks

| Risk                                                                                                                       | Likelihood | Impact | Mitigation                                                                                                                                             |
| -------------------------------------------------------------------------------------------------------------------------- | ---------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Golden-file churn** — both goldens regenerate; `dimLabel`→`colorInkMuted` changes color bytes across many rendered lines | **High**   | Medium | Split chat (1a) and welcome (1c) golden regen into separate PRs; manual diff review of each `.golden`; goldens stay deterministic (no live data added) |
| **400-line budget exceeded** in a single PR                                                                                | **High**   | Medium | 3 chained PRs (§8); flag at `ask-on-risk` tasks gate                                                                                                   |
| `dimLabel` repoint silently shifts many render-width tests                                                                 | Medium     | Low    | Repoint is foreground-only (no width change); width assertions measure visible cols, unaffected; color-substring tests updated in 1a                   |
| ASCII art wraps on narrow terminals                                                                                        | Medium     | Low    | Width guard: fall back to single-line `⫶ daimon` when `width < artWidth` (§4)                                                                          |
| Square-border swap changes input/palette render bytes (overlay tests)                                                      | Medium     | Low    | `NormalBorder` is same thickness as `RoundedBorder` (off-by-N unchanged); only corner glyphs/color differ; update overlay assertions in 1b             |
| Scope creep into name/timestamp/box-wrapping                                                                               | Medium     | Medium | Hard boundary in §Deferred; Phase 1 renders glyph+static only                                                                                          |
