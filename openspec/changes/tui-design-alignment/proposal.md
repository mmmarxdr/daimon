# Proposal: TUI Design Alignment

> **Source of truth**: design bundle at `docs/tui-design/daimon/` (`project/Daimon TUI.html` + `tui*.jsx`; intent in `chats/`).
> **Primary input**: `openspec/changes/tui-design-alignment/exploration.md` (verified file-by-file gap analysis).

## Intent

The embedded TUI (`internal/tui/`, Bubble Tea + lipgloss) was built as a structural skeleton of the 7 design screens but **never used the design as a faithful visual reference**, and left **2 screens functionally incomplete**:

| Problem                                                             | Evidence                                                                                              |
| ------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| Wrong color tokens (Catppuccin leaked; only 4 of 13 tokens defined) | `styles.go` topBar `#cdd6f4`, amber `#ffb347`, pink `#f48fb1` vs design `#eae5d8`/`#e3b67a`/`#d67b9e` |
| Brand glyph wrong                                                   | MsgDaimon uses `δ` not `⫶`; MsgUser uses `"you  "` not `▌`                                            |
| No panel boxing                                                     | rounded uncolored borders, `◈ title` lowercase vs design square boxes + `── TITLE`                    |
| Screen 03 (diff approval) is a stub                                 | `model.go:310` → `return m, nil // stub: PR5`                                                         |
| Screen 07 (permission) is render-only                               | `screen_error.go:10` "No blocking approve/reject seam exists yet"                                     |

**Goal**: realign the embedded TUI to the design bundle as the authoritative visual + functional reference. The design medium is HTML/CSS; the target is a terminal app — **recreate the aesthetic within terminal constraints, do not copy DOM structure.**

## Scope

### In Scope

- **Phase 1 — Visual tokens** (no backend): full 13-token palette + wiring; `δ`→`⫶`, `"you  "`→`▌`; square/box borders with `line` color; `── TITLE` headers; aligned footers/hint sets; welcome ASCII logo + tagline; ToolLine `▸ view` expand.
- **Phase 2 — Structural panels**: rail visual boxing; telemetry by-tool + subagent; policy table; input hint-chips + mode badge; sessions search/columns/model-picker; topbar per-slot colors; context-meter categories; reasoning `pondered for Xs` italic.
- **Phase 3 — Missing screens**: Screen 03 diff viewer (data model, `renderDiff`, hunks-nav/rationale/impact panels, `a/A/r/e/s/q` action bar); Screen 07 interactive approval seam.

### Out of Scope

- Parked nil-bus panic + `base_url` release fixes (branch `fix-tui-nil-bus-panic`).
- Legacy read-only dashboard (`dashboard.go`, `mcp_manage.go`) — verified it uses its own `dashStyles`, does **not** share the `colorAccent`/`colorBG` tokens, so token changes do not touch it.
- Any web-SPA / non-terminal rendering.

## Capabilities

### New Capabilities

- `tui-visual-tokens`: design-faithful palette, glyphs, borders, headers, footers (Phase 1).
- `tui-rail-panels`: bordered telemetry / policy / context / sessions panels (Phase 2).
- `tui-diff-approval`: Screen 03 hunk-by-hunk diff viewer + action bar (Phase 3).
- `tui-permission-approval`: Screen 07 interactive approve/reject seam (Phase 3).

### Modified Capabilities

- None at spec level today; spec phase confirms whether existing `internal/tui` behavior has a prior spec to delta.

## Approach

Phased because the work is large and `delivery_strategy = ask-on-risk` with chained PRs (≤400 lines each). Phase boundaries = independently-shippable PR slices.

| Phase | Theme                           | Backend?                     | ROI                    |
| ----- | ------------------------------- | ---------------------------- | ---------------------- |
| 1     | Visual token alignment          | No                           | High visual / low cost |
| 2     | Structural panels               | Partial (validate in design) | Medium                 |
| 3     | Missing screens + functionality | Yes (seams)                  | High functional        |

Charm v1 only; single root model; centralized `tuiStyles` threaded to sub-components; ANSI width via `x/ansi`; layout from `WindowSizeMsg`.

## Affected Areas

| Area                                                         | Impact              | Description                                 |
| ------------------------------------------------------------ | ------------------- | ------------------------------------------- |
| `internal/tui/styles.go`                                     | Modified            | +9 tokens, real border/header styles (P1)   |
| `internal/tui/components_thread.go`                          | Modified            | `⫶`/`▌`, reasoning, ToolLine expand (P1)    |
| `internal/tui/layout.go`, `components_shell.go`              | Modified            | welcome logo, footers, panel boxing (P1/P2) |
| `internal/tui/rail_panels.go`, `panels.go`                   | Modified            | telemetry/policy/context panels (P2)        |
| `internal/tui/model.go` (`screenDiff`), new `screen_diff.go` | New                 | Screen 03 (P3)                              |
| `internal/tui/screen_error.go`                               | Modified            | approval seam (P3)                          |
| agent/event/store backend                                    | Needs investigation | seams for P2/P3 (design phase)              |

## Risks

| Risk                                                     | Likelihood | Mitigation                                                                                      |
| -------------------------------------------------------- | ---------- | ----------------------------------------------------------------------------------------------- |
| Phase 2/3 need backend beyond `internal/tui`             | High       | Flag each seam for design-phase investigation (below); keep P1 backend-free and shippable first |
| Panel boxing breaks layout width math                    | Med        | Subtract border+padding before sizing children; `x/ansi` width                                  |
| PR slices exceed 400 lines                               | Med        | Keep phases split into work-unit PRs; `ask-on-risk` gate at tasks                               |
| Terminal cannot reproduce CSS effects (glow, rgba tints) | High       | Recreate aesthetic, not DOM; approximate tints with nearest terminal-safe colors                |

### Backend dependencies — investigate in design phase

- Permission approve/reject seam (does the agent expose a blocking approval hook?) — Screen 07.
- Per-tool / subagent telemetry (does `EventToolEnd` carry enough to aggregate?).
- Per-category context accounting (system/memory/conversation/tool/workspace).
- Event timestamps reaching TUI thread items.
- Session metadata (turns/cost/tokens/branch) from store.
- MCP server status wiring into the embedded TUI.

## Decisions (resolved 2026-05-26)

**This change implements Phase 1 ONLY** — visual token alignment, backend-free, independently shippable. Phases 2 and 3 become separate follow-up changes.

- [x] **Phase 1 standalone.** This change = Phase 1 (13-token palette + wiring; `δ`→`⫶`, `"you  "`→`▌`; square borders with `line` color; `── TITLE` headers; aligned footer hint sets; welcome ASCII logo + tagline; ToolLine `▸ view` expand). P2/P3 deferred to their own SDD changes.
- [x] **Backend work split out.** Any missing backend seam (Screen 07 approval, per-tool/subagent telemetry, context categories, event timestamps, session metadata, MCP status wiring) goes in a separate backend SDD change; the TUI consumes it when ready. Phase 1 needs none of this.
- [x] **rgba tints / glow → nearest-color approximation** (truecolor where available). Recreate the aesthetic, not the DOM (design README).
- [x] **MCP server list (Screen 05) is in scope for the OVERALL realignment but NOT Phase 1** — lands in a later phase/change, depends on the separate backend change (MCP status wiring into the embedded TUI).

### Follow-up changes (roadmap, not this change)

- `tui-rail-panels` (Phase 2): rail boxing, telemetry by-tool, policy table, input chips + mode badge, sessions search/cols/model-picker, topbar per-slot colors, context categories, reasoning italic.
- `tui-screens-diff-permission` (Phase 3): Screen 03 diff viewer, Screen 07 interactive approval.
- `tui-backend-seams` (backend): approval seam, per-tool telemetry, context accounting, event timestamps, session metadata, MCP status wiring.
- Screen 05 MCP server list — depends on `tui-backend-seams` MCP wiring.

## Rollback Plan

Each phase is its own PR(s); revert the phase's merge commit(s). Phase 1 is pure visual/style — reverting restores prior tokens with no behavioral risk. No data migrations.

## Success Criteria

- [ ] Phase 1: all 13 tokens defined + wired; `⫶`/`▌`/`── TITLE`/square borders match design; no Catppuccin colors remain.
- [ ] Phase 2: rail panels boxed; telemetry/policy/context/sessions match design structure.
- [ ] Phase 3: Screen 03 renders a real diff with working action bar; Screen 07 supports interactive approve/reject.
- [ ] All backend dependencies resolved or explicitly deferred with rationale.
