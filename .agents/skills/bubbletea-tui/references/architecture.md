# Architecture Reference — daimon TUI

## In-repo Exemplars (read these first)

| File                         | What it demonstrates                                                                                                                                     |
| ---------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/tui/dashboard.go`  | `dashStyles` constructor pattern, `dataLoadedMsg` one-shot load, `RunDashboard` entry point, `lipgloss.JoinVertical` composition                         |
| `internal/tui/mcp_manage.go` | State enum (`manageState`), per-state `handleKey` dispatch, form focus cycling, `spinner.TickMsg` guarded by state, `tea.ExecProcess` subprocess handoff |

## Screen State Machine

```
welcome → chat ⇄ diff
              ↓      ↑
           slash ────┘
           tools
           sessions
           error (any screen → error → back)
```

Define a `screenState int` const block on the root model. Add a `focusRegion int` (none / editor / main / rail) for intra-screen focus routing. Example:

```go
type screenState int
const (
    screenWelcome  screenState = iota
    screenChat
    screenDiff
    screenSlash
    screenTools
    screenSessions
    screenError
)

type focusRegion int
const (
    focusNone   focusRegion = iota
    focusEditor
    focusMain
    focusRail
)
```

## Single-Root Model Shape

```go
type Model struct {
    screen  screenState
    focus   focusRegion
    width   int
    height  int
    styles  tuiStyles      // centralized — pass to sub-components
    overlays overlayManager

    // sub-components (imperative structs, not tea.Model)
    thread    threadPane
    rail      railPanel
    inputBar  inputBar
    topBar    topBar
    footer    footerHints
}
```

**Rule:** Only `Model` implements `tea.Model`. Sub-components expose methods like `Render(width int) string`, `SetData(...)`, `Focus()`, `Blur()`. They NEVER implement `Update(tea.Msg)`.

## Update Routing

```go
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    // 1. Global messages first
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width, m.height = msg.Width, msg.Height
        return m, nil
    case tea.KeyMsg:
        if msg.String() == "ctrl+c" { return m, tea.Quit }
    }

    // 2. Overlays intercept BEFORE screen routing
    if m.overlays.Active() {
        return m.overlays.Handle(m, msg)
    }

    // 3. Route by screen
    switch m.screen {
    case screenChat:  return m.updateChat(msg)
    case screenDiff:  return m.updateDiff(msg)
    // ...
    }
    return m, nil
}
```

**Rule:** "Never use commands to send messages when you can directly mutate children or state." Prefer `m.thread.SetMessages(msgs)` over emitting a `setMessagesMsg`.

## Overlay Manager

```go
type dialog interface {
    ID() string
    HandleMsg(msg tea.Msg) (dialog, tea.Cmd, bool) // bool = consumed
    Render(width, height int, styles tuiStyles) string
}

type overlayManager struct {
    stack []dialog
}

func (o *overlayManager) Push(d dialog)       { o.stack = append(o.stack, d) }
func (o *overlayManager) Pop()                { o.stack = o.stack[:len(o.stack)-1] }
func (o *overlayManager) Active() bool        { return len(o.stack) > 0 }
func (o *overlayManager) Top() dialog         { return o.stack[len(o.stack)-1] }
```

Draw overlays in `View()` last, overlaid via `lipgloss.Place` or a manual overlay compositor.

## Layout Math (Critical)

```go
// Always subtract border + padding before passing width to children.
// lipgloss RoundedBorder = 1 cell each side; Padding(0,1) = 1 each side.
// A border+pad(0,1) box steals 4 columns total.
func innerWidth(outerWidth, borderW, padH int) int {
    return outerWidth - 2*borderW - 2*padH
}

// ANSI-safe truncation — NEVER truncate at byte index on a styled string.
import "github.com/charmbracelet/x/ansi"

safe := ansi.Truncate(styledStr, maxWidth, "…")
w    := ansi.StringWidth(styledStr)
```

## Cmd Discipline

```go
// CORRECT: IO in a Cmd closure, result in a Msg.
func loadMessages(store store.Store) tea.Cmd {
    return func() tea.Msg {
        msgs, err := store.ListMessages()
        return messagesLoadedMsg{msgs: msgs, err: err}
    }
}

// WRONG: mutating model state inside a Cmd.
func bad(m *Model) tea.Cmd {
    return func() tea.Msg {
        m.thread.items = fetch() // DATA RACE — never do this
        return nil
    }
}
```

Multiple cmds: `tea.Batch(loadMessages(store), spinner.Tick)`.

## Tool Renderer Factory

```go
type ToolRenderOpts struct {
    State    toolState // done | running | error | queued
    Width    int
    Expanded bool
}

// Factory — add a case per tool name.
func RenderTool(name string, styles tuiStyles, opts ToolRenderOpts) string {
    switch name {
    case "bash":      return renderBashTool(styles, opts)
    case "read_file": return renderReadFileTool(styles, opts)
    default:          return renderGenericTool(name, styles, opts)
    }
}
```

Interface ladder (opt-in capabilities):

```go
type Renderable  interface { Render(width int) string }
type Focusable   interface { Renderable; Focus(); Blur(); Focused() bool }
type Expandable  interface { Renderable; Expand(); Collapse(); Expanded() bool }
type Animatable  interface { Renderable; Tick() tea.Cmd }
```

## Testing Setup

Use `github.com/charmbracelet/x/exp/teatest` (or `charmbracelet/x/exp/teatest` — check go.mod).

```go
// Golden file test pattern (harmonized with repo's go-testing skill).
func TestChatView_Golden(t *testing.T) {
    m := newChatModel(testStyles())
    m.width, m.height = 80, 24

    got := m.View()
    golden := filepath.Join("testdata", t.Name()+".golden")

    if *update {
        _ = os.WriteFile(golden, []byte(got), 0644)
    }
    want, _ := os.ReadFile(golden)
    if diff := cmp.Diff(string(want), got); diff != "" {
        t.Fatalf("view mismatch (-want +got):\n%s", diff)
    }
}

var update = flag.Bool("update", false, "regenerate golden files")
```

State-machine test pattern:

```go
func TestModel_SlashPaletteOpen(t *testing.T) {
    m := newModel(testConfig())
    m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
    got := m2.(Model).screen
    if got != screenSlash {
        t.Fatalf("expected screenSlash, got %v", got)
    }
}
```

Run golden update: `go test ./internal/tui/... -update`.
Run all TUI tests: `go test ./internal/tui/... -race`.

## Design Source Bundle (Ephemeral)

HTML/JSX prototypes live at `/tmp/daimon-design/daimon/project/` when present:
`Daimon TUI.html`, `Daimon TUI Components.html`, `tui.jsx`, `tui-components.jsx`, `tui-screens-a.jsx`, `tui-screens-b.jsx`.

These files are ephemeral — re-download from the design URL recorded in engram if the directory is gone.
