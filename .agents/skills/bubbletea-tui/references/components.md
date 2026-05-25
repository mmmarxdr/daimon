# Component Taxonomy — daimon TUI

## Persistent Shell (present on every screen)

| Component     | Slots / content                                                     |
| ------------- | ------------------------------------------------------------------- |
| `TopBar`      | `⫶ daimon · cwd · branch · model · mode · cost · status`            |
| `InputBar`    | prompt + chip hints + mode badge; only where conversation is active |
| `FooterHints` | contextual keymap + italic stage direction                          |

These are imperative structs. `TopBar.Render(width int) string` uses `ansi.StringWidth` for slot budgeting.

## Thread Components (center column, varies by screen)

| Component   | Notes                                                                                                                                             |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MsgUser`   | User turn bubble                                                                                                                                  |
| `MsgDaimon` | Assistant turn bubble                                                                                                                             |
| `Reasoning` | Collapsed by default; expand toggles height                                                                                                       |
| `ToolLine`  | 4 states: `done / running / error / queued`; 4 stats slots: `lines / matches / tokens / duration`; truncate long names with "expand to view more" |
| `Subagent`  | Nested mini-thread with its own telemetry row; uses pink accent                                                                                   |

`ToolLine` state type:

```go
type toolState int
const (
    toolDone    toolState = iota
    toolRunning
    toolError
    toolQueued
)
```

Braille spinner glyphs for `toolRunning`: `⣾ ⣽ ⣻ ⢿ ⡿ ⣟ ⣯ ⣷`. Return a `tea.Cmd` via the `Animatable` interface.

## Right Rail — Modular Panel Slots

The rail is NOT a fixed panel. It is a list of contextual panel slots rendered vertically.

Available panels:

| Panel            | When shown        |
| ---------------- | ----------------- |
| `environment`    | welcome           |
| `resume-list`    | welcome, sessions |
| `todolist`       | chat              |
| `context-meter`  | chat              |
| `telemetry`      | chat, diff, error |
| `hunks-nav`      | diff              |
| `rationale`      | diff              |
| `impact`         | diff              |
| `tool-detail`    | tools             |
| `model-picker`   | sessions          |
| `active-policy`  | error             |
| `recent-denials` | error             |

### Panel × Screen Matrix

Authoritative — transcribed from the design's `tui-components.jsx` §05 (`ComponentsMatrix`). Screen order: `welcome · chat · diff · slash · tools · sessions · error`. If you change a panel's placement, change it HERE first.

| Group   | Component          | welcome | chat | diff | slash | tools | sessions | error |
| ------- | ------------------ | :-----: | :--: | :--: | :---: | :---: | :------: | :---: |
| shell   | `TopBar`           |    ✓    |  ✓   |  ✓   |   ✓   |   ✓   |    ✓     |   ✓   |
| shell   | `InputBar`         |    ✓    |  ✓   |      |       |       |          |   ✓   |
| shell   | `FooterHints`      |    ✓    |  ✓   |  ✓   |   ✓   |   ✓   |    ✓     |   ✓   |
| thread  | `MsgUser`          |         |  ✓   |      |       |       |          |       |
| thread  | `MsgDaimon`        |         |  ✓   |      |       |       |          |   ✓   |
| thread  | `Reasoning`        |         |  ✓   |      |       |       |          |       |
| thread  | `ToolLine`         |         |  ✓   |      |       |       |          |   ✓   |
| thread  | `Subagent`         |         |  ✓   |      |       |       |          |       |
| rail    | `environment`      |    ✓    |      |      |       |       |          |       |
| rail    | `resume-list`      |    ✓    |      |      |       |       |    ✓     |       |
| rail    | `todolist`         |         |  ✓   |      |       |       |          |       |
| rail    | `context-meter`    |         |  ✓   |      |       |       |          |       |
| rail    | `telemetry`        |         |  ✓   |  ✓   |       |       |          |   ✓   |
| rail    | `hunks-nav`        |         |      |  ✓   |       |       |          |       |
| rail    | `rationale`        |         |      |  ✓   |       |       |          |       |
| rail    | `impact`           |         |      |  ✓   |       |       |          |       |
| rail    | `tool-detail`      |         |      |      |       |   ✓   |          |       |
| rail    | `model-picker`     |         |      |      |       |       |    ✓     |       |
| rail    | `active-policy`    |         |      |      |       |       |          |   ✓   |
| rail    | `recent-denials`   |         |      |      |       |       |          |   ✓   |
| overlay | `CommandPalette`   |         |      |      |   ✓   |       |          |       |
| overlay | `ApprovalPrompt`   |         |      |  ✓   |       |       |          |       |
| overlay | `PermissionPrompt` |         |      |      |       |       |          |   ✓   |

**Default rule (designer intent):** shell is always present; thread renders only where there is a conversation (chat, plus the offending turn on error); rail panels are contextual — `telemetry` is the most common (3 of 7). The designer's "almost always when chatting" set is `telemetry` + `context-meter` + `todolist`, but the as-designed matrix above is the source of truth — follow it, not the prose.

Rail rendering: iterate the active panel list, call each panel's `Render(width, height int) string`, join vertically. Panels that have no data render as empty string (zero height).

## Overlays (drawn last, above all other content)

| Overlay           | Trigger                                                     |
| ----------------- | ----------------------------------------------------------- |
| Slash palette     | `/` key in chat; full-text search of commands               |
| Permission prompt | Agent requests a sensitive tool; blocks until user responds |
| Diff approval bar | Hunk-by-hunk review; approve / reject / skip                |

Each overlay implements the `dialog` interface (see `references/architecture.md`). The overlay manager intercepts all msgs before screen routing when `Active() == true`.

## 7 Screens

| #   | Screen                  | Key content + rail (per matrix above)                                                                                  |
| --- | ----------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| 01  | boot / welcome          | Logo + input; rail: `environment` + `resume-list`                                                                      |
| 02  | chat (hero)             | Thread (tools + subagents) + input bar; rail: `todolist` + `context-meter` + `telemetry`                               |
| 03  | diff approval           | Hunk viewer; rail: `hunks-nav` + `rationale` + `impact` + `telemetry`; `ApprovalPrompt` overlay                        |
| 04  | slash palette           | `CommandPalette` overlay over dimmed chat; command search + preview                                                    |
| 05  | tools & MCPs            | Tool/MCP list; rail: `tool-detail`                                                                                     |
| 06  | sessions + model picker | Session list; rail: `resume-list` + `model-picker`                                                                     |
| 07  | permission denied       | Offending `MsgDaimon` + `ToolLine`; rail: `telemetry` + `active-policy` + `recent-denials`; `PermissionPrompt` overlay |

## Theme Tokens

Define all values in the centralized `tuiStyles` struct. NEVER use literals in render functions.

| Token    | Value                   | Usage                                              |
| -------- | ----------------------- | -------------------------------------------------- |
| `bg`     | `#0e0f13`               | Terminal background (frameless, dark-only)         |
| `accent` | `#5dbfa7`               | Phosphor teal — primary accent, `⫶` glyph, borders |
| `amber`  | `#ffb347`               | Mode badge, running tool state                     |
| `pink`   | `#f48fb1`               | Subagent threads, branch indicators                |
| `faint`  | lipgloss `Faint(true)`  | Secondary text, hints, dim labels                  |
| `italic` | lipgloss `Italic(true)` | Stage directions in FooterHints                    |
| `errRed` | `lipgloss.Color("9")`   | Error states (matches existing `errStyle`)         |

Glyphs:

- `⫶` — daimon logo / top bar brand mark
- `δ` — delta indicator (diff screen)
- Braille frames `⣾⣽⣻⢿⡿⣟⣯⣷` — running tool spinner

Typography: monospace throughout. Italic for stage directions only. Dark-only palette — no light-mode branch needed.

## Style Struct Pattern (mirrors `dashStyles`)

```go
type tuiStyles struct {
    // chrome
    topBar    lipgloss.Style
    footer    lipgloss.Style
    border    lipgloss.Style

    // text hierarchy
    label     lipgloss.Style
    dimLabel  lipgloss.Style
    hint      lipgloss.Style // faint + italic

    // accents
    accent    lipgloss.Style // phosphor teal
    amber     lipgloss.Style // mode / running
    pink      lipgloss.Style // subagents
    errStyle  lipgloss.Style

    // states
    activeTab   lipgloss.Style
    inactiveTab lipgloss.Style
    selected    lipgloss.Style
}

func newTUIStyles() tuiStyles {
    accent := lipgloss.Color("#5dbfa7")
    amber  := lipgloss.Color("#ffb347")
    pink   := lipgloss.Color("#f48fb1")
    return tuiStyles{
        topBar:      lipgloss.NewStyle().Background(lipgloss.Color("#0e0f13")),
        border:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
        label:       lipgloss.NewStyle().Bold(true),
        dimLabel:    lipgloss.NewStyle().Faint(true),
        hint:        lipgloss.NewStyle().Faint(true).Italic(true),
        accent:      lipgloss.NewStyle().Foreground(accent),
        amber:       lipgloss.NewStyle().Foreground(amber),
        pink:        lipgloss.NewStyle().Foreground(pink),
        errStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
        activeTab:   lipgloss.NewStyle().Bold(true).Underline(true).Foreground(accent),
        inactiveTab: lipgloss.NewStyle().Faint(true),
        selected:    lipgloss.NewStyle().Bold(true).Foreground(accent),
    }
}
```
