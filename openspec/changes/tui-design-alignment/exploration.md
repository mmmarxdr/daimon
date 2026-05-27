# Exploration — TUI Design Alignment

> **Source of truth design**: Claude Design handoff bundle at `docs/tui-design/daimon/`
> (primary file `project/Daimon TUI.html`; sources `project/tui*.jsx`; intent in `chats/`).
> **Date**: 2026-05-26
> **Method**: file-by-file gap analysis, design (JSX) vs implementation (Go `internal/tui/`).

## Summary verdict

Change #9 (tui-poweruser) built the **structural skeleton** of the 7 screens but did **not** use
this design as a faithful visual reference, and left **2 screens functionally incomplete**.
Result: the running TUI does not resemble the design — systematic color/glyph/boxing divergence
plus two missing interactions (diff approval, permission approval).

## The 7 screens (from `Daimon TUI.html`)

| #   | Screen                                                | Design component | Impl status                      |
| --- | ----------------------------------------------------- | ---------------- | -------------------------------- |
| 01  | boot / welcome                                        | `TUI_Welcome`    | exists, major visual gaps        |
| 02  | chat activo (tools + subagent + todolist + telemetry) | `TUI_Chat`       | exists, major gaps               |
| 03  | diff approval (hunk-by-hunk + rationale)              | `TUI_Diff`       | **STUB / MISSING**               |
| 04  | slash palette                                         | `TUI_Slash`      | exists, major gaps               |
| 05  | tools & MCPs                                          | `TUI_Tools`      | exists, MCP list missing         |
| 06  | sessions + model picker                               | `TUI_Sessions`   | exists, major gaps               |
| 07  | permission denied / policy                            | `TUI_Error`      | exists, core interaction MISSING |

Aesthetic (declared): _opencode-feeling, Daimon aesthetic — ⫶ glyph, italic stage directions,
phosphor teal, dark, monospace, frameless. Mockups are HTML/CSS; target is a real Bubble Tea /
lipgloss terminal app — recreate the aesthetic within terminal constraints._

## A. Design tokens comparison

| Token                              | Design (tui.jsx)                                           | Impl (styles.go)                               | Match      |
| ---------------------------------- | ---------------------------------------------------------- | ---------------------------------------------- | ---------- |
| bg                                 | `#0e0f13`                                                  | `#0e0f13` (colorBG)                            | ✅         |
| bgElev/bgDeep/bgPanel              | `#15171d`/`#0a0b0f`/`#11131a`                              | missing                                        | ❌         |
| line/lineSoft/lineStrong           | `#22242c`/`#1a1c22`/`#2e3038`                              | missing (RoundedBorder, no color)              | ❌         |
| ink (primary text)                 | `#eae5d8` (warm parchment)                                 | topBar `#cdd6f4` (Catppuccin blue-white)       | ❌ WRONG   |
| inkSoft/inkMuted/inkFaint/inkGhost | `#c2bca9`/`#7a7465`/`#4a4438`/`#2c2a25`                    | missing (Faint)                                | ❌         |
| accent (phosphor teal)             | `#5dbfa7`                                                  | `#5dbfa7` (colorAccent)                        | ✅         |
| amber                              | `#e3b67a` (muted/warm)                                     | `#ffb347` (vivid orange)                       | ❌ WRONG   |
| red                                | `#e38775`                                                  | ANSI `9` (no hex control)                      | ❌         |
| green                              | `#7aba8a`                                                  | absent                                         | ❌ MISSING |
| pink                               | `#d67b9e`                                                  | `#f48fb1` (too bright)                         | ❌ WRONG   |
| accentDim/accentBg/redBg/amberBg   | rgba tints                                                 | missing                                        | ❌         |
| `⫶` speaker glyph                  | brand identity (TopBar, MsgDaimon, palette title, welcome) | only welcome + palette; **MsgDaimon uses `δ`** | ❌ PARTIAL |
| `›` prompt                         | input cursor prefix                                        | `"› "` sentinel                                | ✅         |
| `▌` user prefix                    | MsgUser header                                             | impl uses `"you  "`                            | ❌ WRONG   |
| border style                       | `1px solid #22242c` square                                 | `RoundedBorder()` no color                     | ❌         |
| panel header                       | `── TITLE` uppercase, letter-spacing                       | `◈ title` lowercase, no rule                   | ❌         |
| rail width                         | 320px                                                      | 32 cols                                        | ≈ ok       |

## B. Screen-by-screen gaps

### 01 Welcome — MAJOR

- No ASCII δ wordmark (design `tui-screens-a.jsx:9–17`; impl `layout.go:127` only `"⫶ daimon"`).
- No italic tagline "speak, and daimon listens." (`tui-screens-a.jsx:37`).
- Footer hints differ: design `/, ⇥ switch agent, ⌃P palette, ?` vs impl `enter/ctrl+c/tab/^t` (`components_shell.go:122`).
- Input placeholder: design "what shall we build today?" vs impl "message daimon…".
- Panels: design = bordered boxes side-by-side; impl = stacked plain-text columns.

### 02 Chat (hero) — MAJOR

- MsgUser `"you  "` not `"▌ name · time"`; MsgDaimon `"δ    "` not `"⫶ daimon speaks · time"` (`components_thread.go:101–145`).
- Reasoning: `"△ reasoning (N chars) — press r"` not `"▸ pondered for 6s"` italic.
- ToolLine missing: `input` arg, `cost` field, `"▸ view"` expand (computed `wasTruncated` then discarded).
- Telemetry panel: no "── by tool" breakdown, no subagent section (`rail_panels.go:84–102`).
- Context meter: total-only, no system/memory/conversation/tool/workspace categories.
- Input bar: no hint chips (`⇥ /commands`, `@ mention`, `# memory`, `⌃R retry`), no mode pill.
- No session breadcrumb (cwd · iter · turns · tokens · autosave).
- Footer: 5 design hints vs impl flat string.

### 03 Diff approval — MISSING (stub)

- `model.go:47` declares `screenDiff`; `model.go:310` → `return m, nil // stub: PR5`.
- `renderCenter` falls to placeholder; no `renderDiff`.
- `panelHunksNav/panelRationale/panelImpact` listed in `panels.go:42` but have NO concrete structs.
- No diff body, no action bar (`a/A/r/e/s/q`), no rationale/impact.

### 04 Slash palette — MAJOR

- Implemented (`overlay_palette.go`) but flat list — no 3 groups (session/agent/workspace), no `── group` headers.
- No keybinding column, no "N matches · M total" counter, no Tab autocomplete, no stage direction.

### 05 Tools & MCPs — MAJOR

- `renderTools` shows only name/risk/cat (3 fields); design has toggle, calls, p50 (6 cols).
- **No MCP servers list** (impl note: "MCP servers are NOT wired in the embedded TUI"); design has 5-server panel with status dots.
- No risk color-coding, no `⚒` header/summary, no toggle/filter/add-MCP footer actions.

### 06 Sessions + model picker — MAJOR

- 4-column list (id/ago/status/title) vs design 7 columns (+turns/cost/tokens/branch).
- No search bar; preview much simpler than design quoted-dialogue.
- Model picker shows 2 lines (provider+model); design = 6 selectable models + active-model detail (thinking/temp/top-p).
- Footer missing new/delete/model-change/filter hints.

### 07 Permission denied — MAJOR (exists, core missing)

- `renderError` renders flat text; **no interactive approval** (`a/A/d/D/e/s`). Impl note `screen_error.go:13`: "render-only. No blocking approve/reject seam exists yet."
- No red-bordered explanation block, no syntax-highlighted tool call, no daimon narrative.
- activePolicy panel shows `"mode: X"` not the 5-row policy table (fs.read/write/shell/net/spawn color-coded).
- recentDenials: no timestamps, no tool-type coloring.
- Footer has no response actions.

## C. Top divergences ranked

1. **Screen 03 (Diff) entirely absent** — stub since PR1 (`model.go:310`).
2. **Error screen (07) has no interactive approval** — the point of the screen; needs backend approve/reject seam.
3. **MsgUser/MsgDaimon identity wrong** — `δ`/`"you"` instead of `⫶`/`▌`; ⫶ brand glyph absent from thread.
4. **ToolLine missing input arg, cost, expand**.
5. **Telemetry missing by-tool breakdown + subagent section**.
6. **Rail panels have no visual boxing** (plain text vs bordered panels).
7. **Color tokens systematically wrong** (Catppuccin leaked; amber/pink/red/green off; only 4 of 13 tokens defined).
8. **TopBar no per-slot color differentiation** (2 styles vs 6 colored segments + amber mode pill).
9. **Input bar missing hint chips + mode badge**.
10. **Sessions missing search + columns + interactive model picker**.

## D. Quick wins vs structural work

### Quick wins (cosmetic, localized)

1. `δ` → `⫶` in MsgDaimon (`components_thread.go:131`).
2. amber `#ffb347`→`#e3b67a`; pink `#f48fb1`→`#d67b9e`; topBar ink `#cdd6f4`→`#eae5d8` (`styles.go`).
3. Add missing color tokens (inkSoft/inkMuted/inkFaint/green/red hex) + wire into tuiStyles.
4. MsgUser `"you  "`→`"▌ "` + username.
5. Reasoning `"▸ pondered for Xs"` italic.
6. Welcome ASCII δ logo + tagline.
7. Welcome/screen footers aligned to design hint sets.
8. Panel headers `◈ title`→`── TITLE`.
9. Borders `RoundedBorder()`→ square/box-drawing with `line` color.
10. Restore ToolLine `"▸ view"` expand hint.

### Structural work (screens / panels / backend deps)

1. **Build Screen 03 (Diff)** from scratch: data model, renderDiff, hunks-nav/rationale/impact panels, action bar.
2. **Error screen approval seam** — needs backend blocking/async approve-reject mechanism + PermissionPrompt overlay + rewired updateError. **Backend dependency.**
3. **activePolicy panel as policy table** (structured policy data, color-coded).
4. **Telemetry by-tool + subagent** (per-tool token map; accumulate() tracks tool names). May need backend.
5. **Context meter categories** — needs backend per-category context counts. **Backend dependency.**
6. **Input bar hint chips + mode badge** (raises inputHeight from 3).
7. **Sessions search + columns + interactive model picker** (needs turns/cost/tokens/branch from store).
8. **Rail visual boxing** — bordered panels; touches every panel + layout math.
9. **MsgDaimon "speaks" + timestamps** — event model must carry timestamps to thread items.
10. **TopBar per-slot colors** — rewrite topBar.Render into 6 styled segments + amber mode pill.

## Backend dependencies to validate during design phase

- Permission approve/reject seam (screen 07) — does the agent expose a blocking approval hook?
- Per-tool / subagent telemetry — does EventToolEnd carry enough to aggregate by tool?
- Per-category context accounting (system/memory/conversation/tool/workspace).
- Event timestamps reaching TUI thread items.
- Session metadata (turns/cost/tokens/branch) available from store.
- MCP server status wiring into the embedded TUI.
