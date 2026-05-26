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

	// sessions screen (PR3b)
	sessions    []store.Conversation // loaded from store on navigation
	sessionIdx  int                  // selected row index in the sessions list
	prevScreen  screenState          // screen to return to on esc
	sessionsErr error                // set when loadSessionsCmd fails; cleared on success

	// tools screen (PR4a)
	tools   []toolEntry // loaded from agent.ToolRegistry on navigation
	toolIdx int         // selected row index in the tools list

	// error screen (PR5): permission-denied state
	errorToolName string        // tool name that triggered the denial
	errorReason   string        // human-readable denial reason from EventToolEnd.Error
	recentDenials []denialEntry // copy-on-write; capped to 10; never nil after first denial
}

// denialEntry is a single policy/mode denial captured from EventToolEnd.
type denialEntry struct {
	tool   string // tool name that was denied
	reason string // human-readable reason
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
		// Bus bridge already wired (RunTUI path); start pump AND pre-load sessions
		// so the welcome resume-list panel is populated immediately on launch.
		return tea.Batch(pumpEvents(m.events), loadSessionsCmd(m.store))
	}
	// Test / no-bus path: still kick off session load (nil store is guarded inside).
	return loadSessionsCmd(m.store)
}

// Update implements tea.Model. It dispatches messages globally first, then
// to overlays (AD-9), then to the active screen handler. Unimplemented
// screen stubs return (m, nil) until their respective PR fills them in.
//
// CONTRACT (AD-3): all 7 screenState cases are present here from PR1.
// Later PRs replace the stub bodies with real handlers (updateChat, etc.).
//
// C3 FIX: busEventMsg and agentReplyMsg are handled GLOBALLY here, before
// the screen switch. This guarantees the pump is ALWAYS re-armed even when
// the active screen is not screenChat — events never stop flowing mid-session.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// 1. Global messages.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// ctrl+c always quits regardless of overlay or focus state.
			return m, tea.Quit
		case "q":
			// Bare 'q' quits ONLY in chat-nav context: screen==screenChat AND no
			// active overlay AND focus is NOT on the editor (where 'q' is a valid
			// typed character). On sessions/tools/error screens, 'q' is a no-op —
			// esc navigates back; ctrl+c always quits.
			if m.screen == screenChat && !m.overlays.Active() && m.focus != focusEditor && m.focus != focusNone {
				return m, tea.Quit
			}
		}

	// PR2b: todolistRefreshMsg arrives from fetchTodolist Cmd after
	// EventTodolistChanged. Update the todolist panel via copy-on-write.
	case todolistRefreshMsg:
		m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
			if tp, ok := panels[panelTodolist].(*todolistPanel); ok {
				cp := *tp
				cp.setList(msg.list)
				panels[panelTodolist] = &cp
			}
		})
		return m, nil

	// PR4b: sessionsLoadedMsg is handled GLOBALLY so both the welcome screen
	// (resume-list panel) and the sessions screen (renderSessions reads m.sessions)
	// receive the update regardless of which screen is active.
	case sessionsLoadedMsg:
		if msg.err != nil {
			m.sessionsErr = msg.err
		} else {
			m.sessionsErr = nil
			m.sessions = msg.convs
			// Clamp sessionIdx to [0, len-1].
			if len(m.sessions) == 0 {
				m.sessionIdx = 0
			} else if m.sessionIdx >= len(m.sessions) {
				m.sessionIdx = len(m.sessions) - 1
			}
			// Update the resume-list panel (welcome + sessions rail) via copy-on-write.
			m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
				if p, ok := panels[panelResumeList].(*resumeListPanel); ok {
					cp := *p
					cp.setSessions(m.sessions)
					panels[panelResumeList] = &cp
				}
			})
		}
		return m, nil

	// C3: Handle bus/reply messages globally so the pump is re-armed regardless
	// of which screen is active. The chat screen handler also processes these
	// for thread mutations when screen == screenChat.
	case busEventMsg:
		if m.screen == screenChat {
			return m.updateChat(msg) // full handler including thread mutations
		}
		// Fix 3: re-entrant denial — a 2nd denial arriving while already on
		// screenError must not be swallowed. Update error state and stay on
		// screenError so the user sees the latest denial.
		ev := msg.event
		if m.screen == screenError && ev.Type == notify.EventToolEnd && ev.Meta["denied"] == "true" {
			m = m.applyDenial(ev)
			// Re-arm the pump so events keep flowing.
			if m.events != nil {
				return m, pumpEvents(m.events)
			}
			return m, nil
		}
		// All other non-chat screens: apply no thread mutations but always re-arm the pump.
		if m.events != nil {
			return m, pumpEvents(m.events)
		}
		return m, nil

	case agentReplyMsg:
		if m.screen == screenChat {
			return m.updateChat(msg) // full handler including thread append
		}
		// Non-chat screen: re-arm the pump so replies aren't lost.
		if m.events != nil {
			return m, pumpEvents(m.events)
		}
		return m, nil

	// PR3a: overlay lifecycle messages — handled globally, before overlay-interception.
	case popOverlayMsg:
		m.overlays.Pop()
		return m, nil

	case dispatchCommandMsg:
		m.overlays.Pop()
		if m.ag == nil {
			m.thread.append(&MsgDaimon{text: "no agent connected", styles: m.styles})
			return m, nil
		}
		return m, runCommandCmd(m.ag, msg.name, "", msg.allowDestructive)

	case commandResultMsg:
		text := msg.reply
		if msg.err != nil {
			text = "command failed: " + msg.err.Error()
		}
		m.thread.append(&MsgDaimon{text: text, styles: m.styles})
		return m, nil
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
		return m.updateWelcome(msg)
	case screenChat:
		return m.updateChat(msg) // PR2
	case screenDiff:
		return m, nil // stub: PR5
	case screenSlash:
		return m, nil // stub: PR3 (canonical path: overlay push while screen==chat)
	case screenTools:
		return m.updateTools(msg) // PR4a
	case screenSessions:
		return m.updateSessions(msg) // PR3b
	case screenError:
		return m.updateError(msg) // PR5
	}
	return m, nil
}

// updateWelcome handles messages on the welcome screen. Pressing Enter with
// non-empty input transitions to the chat screen and submits the first message
// (mirrors handleChatKey's Enter path). All other keys forward to the input bar.
//
// The thread is empty on welcome, so thread.append allocates a fresh slice — no
// copy-on-write aliasing with a prior model for this first message. The input is
// cleared via the existing promptSentMsg path (submit → updateChat → input.Reset)
// since screen is screenChat by the time promptSentMsg arrives.
func (m Model) updateWelcome(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			if text := m.input.Value(); text != "" {
				m.thread.append(&MsgUser{text: text, styles: m.styles})
				m.screen = screenChat
				m.focus = focusEditor
				m.footer = footerHints{screen: screenChat}
				return m, m.ch.submit(text, m.activeConvID)
			}
		case "tab":
			// tab navigates to the sessions screen (matches footer hint).
			m.prevScreen = screenWelcome
			m.screen = screenSessions
			m.footer = footerHints{screen: screenSessions}
			return m, loadSessionsCmd(m.store)

		case "ctrl+t":
			// ctrl+t navigates to the tools screen.
			// Bare 't' is NOT used to avoid breaking typed messages starting with 't'.
			m.prevScreen = screenWelcome
			m.screen = screenTools
			m.footer = footerHints{screen: screenTools}
			return m, loadToolsCmd(m.ag)
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// View implements tea.Model. It renders the active screen with persistent
// shell (TopBar + optional InputBar + FooterHints) and the right rail.
// When overlays are active the topmost dialog is composited OVER the dimmed
// base (AD-9: "composited over the dimmed main layout") so chat content remains
// visible around the palette instead of being discarded.
func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	base := renderLayout(m)
	if m.overlays.Active() {
		box := m.overlays.Render(m.width, m.height, m.styles)
		if box != "" {
			// Dim the base (best-effort; content remains visible outside the box).
			dimmedBase := m.styles.dim.Render(base)
			return placeOverlay(dimmedBase, box, m.width, m.height)
		}
	}
	return base
}

// ---------------------------------------------------------------------------
// newTestModel creates a minimal Model for unit tests.
// It avoids any real agent/bus/store to keep tests hermetic.
// A fresh TUIChannel is allocated so tests that exercise m.ch don't panic on nil.
// ---------------------------------------------------------------------------
func newTestModel() Model {
	s := newTuiStyles()
	return Model{
		styles:    s,
		ch:        newTUIChannel(),
		channelID: "tui",
		senderID:  "local_user",
		screen:    screenWelcome,
		focus:     focusEditor,
		input:     newInputBar(),
		rail:      newRail(s),
	}
}
