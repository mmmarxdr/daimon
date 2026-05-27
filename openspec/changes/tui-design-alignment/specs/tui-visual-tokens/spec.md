# TUI Visual Tokens Specification

## Purpose

Defines the exact visual tokens, glyphs, border style, panel-header form,
footer hint sets, welcome-screen content, and ToolLine expand affordance
that the embedded Bubble Tea TUI (`internal/tui/`) MUST present after
Phase 1 of the tui-design-alignment change. All requirements are
backend-free and independently shippable. Phase 2 (structural panels)
and Phase 3 (missing screens) are out of scope.

---

## ADDED Requirements

### Requirement: Full 13-token color palette

`internal/tui/styles.go` MUST define package-level constants for all
thirteen design tokens sourced from `docs/tui-design/daimon/project/tui.jsx`.
No color literal outside this constant block is permitted in `internal/tui/`.

| Constant        | Hex value | Role                                |
| --------------- | --------- | ----------------------------------- |
| `colorBG`       | `#0e0f13` | Terminal background (unchanged)     |
| `colorBGElev`   | `#15171d` | Elevated surface                    |
| `colorBGDeep`   | `#0a0b0f` | Deep background                     |
| `colorBGPanel`  | `#11131a` | Panel background                    |
| `colorInk`      | `#eae5d8` | Primary text (warm parchment)       |
| `colorInkSoft`  | `#c2bca9` | Secondary text                      |
| `colorInkMuted` | `#7a7465` | Muted text                          |
| `colorInkFaint` | `#4a4438` | Faint text                          |
| `colorInkGhost` | `#2c2a25` | Ghost / placeholder text            |
| `colorLine`     | `#22242c` | Default border / divider            |
| `colorLineSoft` | `#1a1c22` | Soft border                         |
| `colorLineStr`  | `#2e3038` | Strong border                       |
| `colorAccent`   | `#5dbfa7` | Phosphor teal (unchanged)           |
| `colorAmber`    | `#e3b67a` | Mode badge, running-tool state      |
| `colorRed`      | `#e38775` | Error / danger                      |
| `colorGreen`    | `#7aba8a` | Success / ok                        |
| `colorPink`     | `#d67b9e` | Subagent threads, branch indicators |

RGBA tint tokens (`accentDim`, `accentBg`, `amberBg`, `redBg`) MUST be
approximated to their nearest terminal-safe lipgloss color and documented
inline. Each MUST have a corresponding `tuiStyles` field.

The Catppuccin value `#cdd6f4` MUST NOT appear anywhere in
`internal/tui/` after this change.

#### Scenario: All 13 color constants present and correct

- GIVEN the compiled `styles.go` package
- WHEN a test reads all exported/unexported color constants from the package
- THEN every constant in the table above exists with its exact hex value
- AND `#cdd6f4` does not appear in any constant or style initializer

> Assertion: direct string comparison against constant values; a golden
> snapshot of `newTuiStyles()` field values is NOT needed here — constant
> equality is sufficient.

#### Scenario: No inline hex literals in render functions

- GIVEN all `.go` files under `internal/tui/`
- WHEN the files are scanned for hex color patterns outside `styles.go`
- THEN no match is found (enforced by existing "RULE: No hex color
  literals may appear in any Render function" comment)

#### Scenario: Wrong amber replaced

- GIVEN a model rendered with `newTuiStyles()`
- WHEN a test inspects `s.amber` foreground color
- THEN the color is `#e3b67a`
- AND NOT `#ffb347`

#### Scenario: Wrong pink replaced

- GIVEN a model rendered with `newTuiStyles()`
- WHEN a test inspects `s.pink` foreground color
- THEN the color is `#d67b9e`
- AND NOT `#f48fb1`

#### Scenario: topBar ink corrected

- GIVEN a model rendered with `newTuiStyles()`
- WHEN a test inspects `s.topBar` foreground color
- THEN the color is `#eae5d8`
- AND NOT `#cdd6f4`

---

### Requirement: Speaker glyphs — MsgDaimon and MsgUser

`internal/tui/components_thread.go` MUST render message headers using
the design-canonical glyphs.

- MsgDaimon header MUST use `⫶` (U+2AF6) as the speaker prefix, NOT `δ`.
- MsgUser header MUST use `▌` (U+258C) as the user-line prefix, NOT the
  string `"you  "`.

#### Scenario: MsgDaimon glyph

- GIVEN a thread model containing one assistant message
- WHEN `RenderThread` (or equivalent) produces its output string
- THEN the output contains `⫶`
- AND does NOT contain the header prefix `δ`

> Assertion: golden file for the thread component render with one stub
> assistant message. Update via the repo `-update` flag.

#### Scenario: MsgUser glyph

- GIVEN a thread model containing one user message
- WHEN `RenderThread` produces its output string
- THEN the output contains `▌`
- AND does NOT contain the header string `"you  "`

> Assertion: golden file, same as above scenario.

---

### Requirement: Square borders colored with `line` token

All borders inside `internal/tui/` (panels, input bar, palette overlay)
MUST use `lipgloss.NormalBorder()` (box-drawing characters) with
`BorderForeground` set to `colorLine` (`#22242c`). `RoundedBorder()` is
prohibited as the primary panel border style.

The `inputBarStyle` border MAY retain accent color per the design's
focused-input treatment, but MUST switch from rounded to normal border
characters.

#### Scenario: Panel border is normal (square) not rounded

- GIVEN `newTuiStyles()`
- WHEN a test renders a generic bordered box with `s.border`
- THEN the output string contains `┌` (U+250C, top-left corner of normal border)
- AND does NOT contain `╭` (U+256D, rounded corner)

> Assertion: direct string scan of the rendered lipgloss box output.

#### Scenario: Border foreground is `line` token

- GIVEN `newTuiStyles()`
- WHEN a test inspects `s.border` border foreground color
- THEN the color is `#22242c`

---

### Requirement: Panel headers in `── TITLE` form

Every panel header rendered in `internal/tui/` MUST follow the form
`── TITLE` where TITLE is uppercase. The `◈ title` lowercase prefix form
MUST NOT be used.

#### Scenario: Panel header format

- GIVEN any rendered panel (thread, telemetry, sessions, tools, etc.)
- WHEN the output string is inspected
- THEN it contains `── ` followed by an uppercase label
- AND does NOT contain `◈`

> Assertion: golden file per screen render; update via `-update` flag.

---

### Requirement: Footer hint sets match the design

Each screen's footer MUST display the hint set defined in the design
reference:

| Screen        | Hint set (design canonical)                        |
| ------------- | -------------------------------------------------- |
| Welcome (01)  | `⇥ /commands · ⌃R resume last · ⌃C exit`           |
| Chat (02)     | `↑↓ scroll · ⇥ switch panel · ⌃P palette · ? help` |
| Slash (04)    | `↑↓ select · ↵ run · esc close · ⇥ autocomplete`   |
| Tools (05)    | `↑↓ select · ↵ toggle · f filter · a add-MCP`      |
| Sessions (06) | `↑↓ select · ↵ open · n new · d delete · m model`  |

(Screens 03 and 07 are Phase 3 — out of scope.)

Footers MUST be rendered using the `hint` style (faint + italic) from
`tuiStyles`. Each hint token MUST be separated by `·` (space + middle
dot + space).

#### Scenario: Welcome footer hint set

- GIVEN the welcome screen model
- WHEN `renderWelcome` (or equivalent) produces its output string
- THEN the footer region contains `⇥ /commands`
- AND contains `⌃R resume last`
- AND contains `⌃C exit`

> Assertion: golden file for welcome screen render at a fixed width (80
> cols). Update via `-update` flag.

#### Scenario: Chat footer hint set

- GIVEN the chat screen model
- WHEN `renderChat` produces its output
- THEN the footer region contains `⇥ switch panel`
- AND contains `⌃P palette`

---

### Requirement: Welcome screen ASCII δ logo and tagline

The welcome screen MUST render:

1. The ASCII δ logo block (8 lines, accent color `#5dbfa7`) as defined in
   `docs/tui-design/daimon/project/tui-screens-a.jsx` lines 9–17.
2. The italic tagline `"speak, and daimon listens."` in `inkSoft`
   (`#c2bca9`) color immediately below the logo.

The existing single-line `"⫶ daimon"` text used as the welcome heading
MUST be replaced by this logo block + tagline pair.

#### Scenario: ASCII logo present on welcome screen

- GIVEN the welcome screen rendered at width ≥ 80 cols
- WHEN the output string is inspected
- THEN it contains the literal substring `▄▄▄▄▄` (first distinctive line
  of the logo)
- AND contains `speak, and daimon listens.`

> Assertion: golden file for welcome render. The logo lines are
> deterministic; terminal-width sensitivity requires the test to fix the
> model width at 80 cols via `WindowSizeMsg`.

#### Scenario: Logo absent in non-welcome screens

- GIVEN any non-welcome screen model
- WHEN its render output is inspected
- THEN it does NOT contain `▄▄▄▄▄`

---

### Requirement: ToolLine `▸ view` expand affordance

When a ToolLine's output was truncated (i.e. the full output exceeds the
display budget), the rendered ToolLine MUST append a `▸ view` affordance
styled with the `accent` color. When the output was NOT truncated, `▸ view`
MUST NOT appear.

The `wasTruncated` boolean already computed in the ToolLine render path
MUST drive this conditional — it MUST NOT be discarded.

#### Scenario: Truncated tool output shows expand hint

- GIVEN a ToolLine with output longer than the display budget
- WHEN the ToolLine is rendered
- THEN the output string contains `▸ view`

> Assertion: direct string match on `RenderToolLine` (or equivalent)
> called with a stub tool event whose output length exceeds the truncation
> threshold.

#### Scenario: Non-truncated tool output hides expand hint

- GIVEN a ToolLine with output shorter than or equal to the display budget
- WHEN the ToolLine is rendered
- THEN the output string does NOT contain `▸ view`

---

## Non-requirements (Phase 1 scope boundary)

- Rail panel visual boxing (bordered telemetry / policy / context panels) — Phase 2.
- Telemetry by-tool breakdown and subagent section — Phase 2.
- Input bar hint chips and mode badge — Phase 2.
- Sessions search, additional columns, model picker — Phase 2.
- TopBar per-slot color differentiation — Phase 2.
- Reasoning `"▸ pondered for Xs"` italic — Phase 2.
- Session breadcrumb (cwd · iter · turns · tokens · autosave) — Phase 2.
- Screen 03 (diff approval) — Phase 3.
- Screen 07 interactive approve/reject seam — Phase 3.
- Input placeholder text change ("what shall we build today?") — Phase 2.
- MCP server list in Screen 05 — depends on `tui-backend-seams`.
- Any backend seam changes (`agent/event/store`).
