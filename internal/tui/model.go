// Package tui implements daimon's embedded full-screen terminal UI.
// A SINGLE ROOT MODEL (Model) owns tea.Model; sub-components are imperative
// structs with Render/SetData/Focus methods — never nested Elm sub-models.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/agent"
	"daimon/internal/config"
	"daimon/internal/notify"
	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// focusRegion — intra-screen focus routing (AD-3, reintroduced in PR2)
//
// Declared alongside screenState so both enums live in model.go.
// Values are frozen per the AD-3 forward-compat contract.
// ---------------------------------------------------------------------------

// focusRegion identifies which sub-region of the screen has keyboard focus.
type focusRegion int

const (
	focusNone   focusRegion = iota // no explicit focus (default)
	focusEditor                    // input bar (text entry)
	focusMain                      // center column (thread navigation)
	focusRail                      // right rail (panel navigation)
)

// ---------------------------------------------------------------------------
// screenState — forward-compat FROZEN enum (AD-3)
//
// PR1 declares ALL 7 values so later PRs only fill updateXxx/renderXxx bodies.
// NEVER renumber or insert values mid-iota — the iota order is locked by
// TestScreenState_EnumContract. Appending new values is forbidden in this
// change; all 7 exist from day 1.
// ---------------------------------------------------------------------------

// screenState identifies which of the seven TUI screens is active.
type screenState int

const (
	screenWelcome  screenState = iota // 01 — boot / welcome; logo + input + environment rail
	screenChat                        // 02 — chat (hero); thread + input + todolist/context/telemetry rail
	screenDiff                        // 03 — diff viz-only (no blocking approval in V1)
	screenSlash                       // 04 — slash palette (overlay over dimmed chat; canonical path is Push, not switch)
	screenTools                       // 05 — tools & MCPs; tool-detail rail
	screenSessions                    // 06 — sessions + model picker; resume-list + model-picker rail
	screenError                       // 07 — permission denied; telemetry + active-policy + recent-denials rail
)

// String returns a human-readable name for use in test output and logs.
func (s screenState) String() string {
	switch s {
	case screenWelcome:
		return "welcome"
	case screenChat:
		return "chat"
	case screenDiff:
		return "diff"
	case screenSlash:
		return "slash"
	case screenTools:
		return "tools"
	case screenSessions:
		return "sessions"
	case screenError:
		return "error"
	default:
		return "unknown"
	}
}

// ---------------------------------------------------------------------------
// Model — single root tea.Model (AD-2)
//
// Only Model implements tea.Model. Sub-components (topBar, footerHints,
// inputBar, thread, rail, overlayManager) are imperative structs.
// ---------------------------------------------------------------------------

// Model is the root Bubble Tea model for the daimon TUI.
// It is the ONLY type that implements tea.Model; all sub-components are
// imperative structs that expose Render / SetData / Focus / Blur methods.
type Model struct {
	// chrome / layout
	width  int
	height int
	screen screenState
	focus  focusRegion // intra-screen focus region (PR2)
	styles tuiStyles

	// backend handles (injected by RunTUI; never constructed here)
	ag    *agent.Agent // embedded dispatch + accessors (ToolRegistry, TodoListForConv)
	bus   notify.Bus   // subscribe in Init; bus may be nil (welcome/static screens)
	store store.Store  // session list, conv reads (threaded from ag)
	ch    *TUIChannel  // input-bar → inbox; agent Send → tea.Msg
	cfg   *config.Config

	// event bridge (PR2): bus handler does non-blocking send here; tea.Cmd drains it.
	// The channel carries both busEventMsg (from notify.Bus) and agentReplyMsg
	// (from TUIChannel.out) multiplexed by the goroutine started in Init.
	events <-chan tea.Msg

	// persistent shell (PR1)
	topBar topBar
	footer footerHints
	input  inputBar // bubbles/textinput-backed; shown per matrix (welcome, chat, error)

	// thread + rail (PR2+)
	thread thread // ordered list of threadItems for the chat center column
	rail   rail

	// overlays (PR3): dialog stack drawn last
	overlays overlayManager

	// session-scoped routing identity (AD-7)
	channelID    string // "tui"
	senderID     string // "local_user"
	activeConvID string // tracked for TodoListForConv + sessions (PR2)
}

// Init implements tea.Model. When a notify.Bus is wired (i.e. we are running
// inside RunTUI, not in a unit test), it subscribes to bus events and starts
// the pumpEvents Cmd so bus events flow into the TUI loop (AD-5).
//
// Value-receiver: the Init copy's mutations are intentionally NOT reflected
// back to the live model — this is safe because Init is called once by
// bubbletea immediately before the first Update, and the live model receives
// the pump Cmd via the returned tea.Cmd (not via model field assignment).
// The events channel is set up in RunTUI before the program starts so the
// live model already holds it; Init on the copy does not re-create it.
func (m Model) Init() tea.Cmd {
	if m.events != nil {
		// Bus bridge already wired (RunTUI path); just start the pump.
		return pumpEvents(m.events)
	}
	return nil
}

// Update implements tea.Model. It dispatches messages globally first, then
// to overlays (AD-9), then to the active screen handler. Unimplemented
// screen stubs return (m, nil) until their respective PR fills them in.
//
// CONTRACT (AD-3): all 7 screenState cases are present here from PR1.
// Later PRs replace the stub bodies with real handlers (updateChat, etc.).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 1. Global messages.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}

	// 2. Overlays intercept ALL messages before screen routing (AD-9 / PR3).
	if m.overlays.Active() {
		top := m.overlays.Top()
		next, cmd, consumed := top.HandleMsg(msg)
		m.overlays.Replace(next)
		if consumed {
			return m, cmd
		}
	}

	// 3. Route by screen. All 7 cases are declared here (AD-3 contract).
	// Unimplemented screens return (m, nil); PR2-5 replace stub bodies.
	switch m.screen {
	case screenWelcome:
		return m.updateWelcomeStub(msg)
	case screenChat:
		return m.updateChat(msg) // PR2
	case screenDiff:
		return m, nil // stub: PR5
	case screenSlash:
		return m, nil // stub: PR3 (canonical path: overlay push while screen==chat)
	case screenTools:
		return m, nil // stub: PR4
	case screenSessions:
		return m, nil // stub: PR3
	case screenError:
		return m, nil // stub: PR5
	}
	return m, nil
}

// updateWelcomeStub handles messages on the welcome screen (stub for PR4 enrichment).
func (m Model) updateWelcomeStub(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// View implements tea.Model. It renders the active screen with persistent
// shell (TopBar + optional InputBar + FooterHints) and the right rail.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	return renderLayout(m)
}

// ---------------------------------------------------------------------------
// newTestModel creates a minimal Model for unit tests.
// It avoids any real agent/bus/store to keep tests hermetic.
// A fresh TUIChannel is allocated so tests that exercise m.ch don't panic on nil.
// ---------------------------------------------------------------------------
func newTestModel() Model {
	return Model{
		styles:    newTuiStyles(),
		ch:        newTUIChannel(),
		channelID: "tui",
		senderID:  "local_user",
		screen:    screenWelcome,
		focus:     focusEditor,
	}
}
