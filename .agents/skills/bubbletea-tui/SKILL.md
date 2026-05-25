---
name: bubbletea-tui
description: "Trigger: Bubbletea, Bubble Tea, TUI, Lipgloss, terminal UI, Charm, daimon power-user UI. Guides building daimon's embedded TUI with Charm v1 (bubbletea v1.3.10 / lipgloss v1.1.0 / bubbles v1.0.0)."
license: Apache-2.0
metadata:
  author: gentleman-programming
  version: "1.0"
---

# Bubbletea TUI — daimon power-user terminal UI

## Activation Contract

Activate when the user touches or asks about: `internal/tui/`, Bubbletea models, Lipgloss styling, TUI screens, overlays, slash palette, diff approval, terminal layout, or any Charm library code. Co-activate with `golang-pro` and `go-testing` for implementation and test phases respectively.

## Hard Rules

1. **Charm v1 ONLY.** Pinned: `bubbletea v1.3.10`, `lipgloss v1.1.0`, `bubbles v1.0.0`, `x/ansi v0.11.6`. NEVER import `charm.land/*` paths. NEVER reference v2 APIs (`uv.Screen`, `ScreenBuffer`, `Draw(scr, area)`).
2. **SINGLE ROOT MODEL.** One struct owns `tea.Model`. Sub-components are plain structs with imperative methods (`Render(w int) string`, `SetMessages(...)`, `Focus()`). NEVER nest Elm sub-models with their own `Update(tea.Msg)`.
3. **Centralized styles.** All Lipgloss styles live in one semantic struct (like `dashStyles`). NEVER hardcode colors inline. Thread the struct to sub-components.
4. **ANSI width math.** Use `github.com/charmbracelet/x/ansi` (`StringWidth`, `Truncate`, `Strip`) for every truncation/alignment. NEVER slice ANSI strings at byte level.
5. **Cmd discipline.** IO/expensive work → return a `tea.Cmd`. No state mutation inside a command closure. Multiple cmds → `tea.Batch(...)`.
6. **Layout sizing.** Compute rectangles from `WindowSizeMsg`. Always subtract border+padding widths before passing to children (off-by-N is the #1 truncation bug).
7. **Overlays last.** Draw all dialog overlays after the main layout; use an overlay manager with push/pop keyed by dialog ID.
8. **Embedded dispatch.** The TUI calls daimon's internal command registry directly (same process). NEVER make REST calls from Update.
9. **Run entry point pattern.** Expose a `RunXxx(cfg) error` function, call `tea.NewProgram(m, tea.WithAltScreen())`, require TTY (see `RunMCPManage`).

## Decision Gates

| Situation                                | Decision                                                                      |
| ---------------------------------------- | ----------------------------------------------------------------------------- |
| Need a child to respond to msgs          | Imperative method on root Update, NOT sub-Update                              |
| Multiple async results needed            | `tea.Batch(cmd1, cmd2)`                                                       |
| Navigation between full screens          | Switch `state` enum on root model                                             |
| Floating palette/prompt/dialog           | Push to overlay manager, render last                                          |
| Tool result rendering varies by tool     | Factory: `RenderTool(styles, width, opts) string`                             |
| Full-screen navigation controller needed | Consider donderom/bubblon Model Stack (mention only; daimon uses single-root) |
| Width of a rendered string               | `ansi.StringWidth(s)`, never `len(s)`                                         |
| Injecting state into a sub-component     | Direct field mutation in Update or imperative setter                          |

## Execution Steps

1. Read `internal/tui/dashboard.go` (exemplar: `dashStyles`, `dataLoadedMsg`, one-shot `Init`, `RunDashboard`).
2. Read `internal/tui/mcp_manage.go` (exemplar: state enum, `manageStyles`, form focus cycling, `handleKey` dispatch, `RunMCPManage`).
3. Define screen state enum and focus enum on root model.
4. Build centralized style struct via constructor; thread to all sub-components.
5. Implement `Init()` with one-shot data-load cmd (`dataLoadedMsg` pattern).
6. Implement `Update()`: WindowSizeMsg → compute layout; overlay msgs intercepted first; route to screen handler by state.
7. Implement `View()`: compose with `lipgloss.JoinVertical`/`JoinHorizontal`; draw overlays last.
8. For new components: imperative struct + `Render(width int) string`; never standalone `tea.Model`.
9. Write teatest golden file + state-transition unit test. See `references/` for detail.

## Output Contract

- All new Go files in `internal/tui/` package `tui`.
- Exported entry point: `RunXxx(cfg *config.Config, ...) error`.
- No inline hex/color literals in render functions.
- All width math uses `ansi.StringWidth` / `ansi.Truncate`.
- Tests: golden file per rendered view + at least one Update state-machine test.

## References

- `references/architecture.md` — screen enum, overlay manager, layout math, testing setup
- `references/components.md` — component taxonomy, panel×screen matrix, theme tokens
- `assets/model-skeleton.go.tmpl` — root model + imperative sub-component + style struct + teatest golden
