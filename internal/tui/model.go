// Package tui implements daimon's embedded full-screen terminal UI.
// A SINGLE ROOT MODEL (Model) owns tea.Model; sub-components are imperative
// structs with Render/SetData/Focus methods — never nested Elm sub-models.
package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/agent"
	"daimon/internal/config"
	"daimon/internal/notify"
	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// modeAgent — interface for mode read/write used by Tab cycling (A3)
//
// The production path wires *agent.Agent (which satisfies this via CurrentMode
// and a thin SetModeImmediate wrapper). Tests inject a mockModeAgent stub.
// ---------------------------------------------------------------------------

// modeAgent abstracts mode reads/writes so the TUI can cycle mode in tests
// without a real store, context, or conversation.
type modeAgent interface {
	// CurrentMode returns the active mode name ("build", "plan", or "review").
	CurrentMode() string
	// SetModeImmediate updates the in-memory mode cache without persistence.
	// The TUI uses this for optimistic UI; the real agent also persists asynchronously
	// via a tea.Cmd (switchModeCmd).
	SetModeImmediate(name string)
	// ReconcileMode clears the optimistic override once the async switch to
	// `confirmed` has landed, so CurrentMode() resumes delegating to the
	// authoritative cache. It MUST be a no-op when a newer override has
	// superseded `confirmed` (race-safe across rapid Tab presses).
	ReconcileMode(confirmed string)
}

// switchModeMsg is delivered by switchModeCmd after a mode change attempt.
// On success err is nil. On failure (ErrTurnInProgress, store error) err is set
// and the model should show the error in the thread or ignore it.
type switchModeMsg struct {
	mode string
	err  error
}

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

	// mode — cached agent mode string (WU-a: written in Update, read in View).
	// Eliminates the live modeAgent.CurrentMode() call from renderLayout.
	// Initialized from ag.CurrentMode() at construction; updated in cycleMode,
	// the /mode commandResultMsg handler, and (for purity) never in View.
	mode string

	// viewport — owns the chat-center scroll window (WU-c / PR-2).
	// Stored by value; dimensions set via WindowSizeMsg, content pushed by
	// refreshThreadViewport after every thread mutation. Initialized to
	// viewport.New(0,0) in both constructors so non-chat tests never nil-deref.
	viewport viewport.Model

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

	// breadcrumb (Inc.2): chat session header row under the topbar.
	// Accumulated from EventTokensUsage in handleBusEvent (copy-on-write).
	breadcrumb breadcrumb

	// overlays (PR3): dialog stack drawn last
	overlays overlayManager

	// session-scoped routing identity (AD-7)
	channelID    string // "tui"
	senderID     string // "local_user"
	activeConvID string // tracked for TodoListForConv + sessions (PR2)

	// sessions screen (PR3b)
	sessions    []store.Conversation // loaded from store on navigation
	sessionsAgo []string             // WU-b: pre-computed "ago" strings parallel to sessions
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

	// mode cycling (Phase A / A3): thin interface over *agent.Agent for testability.
	// Nil when no agent is wired (tests / welcome screen without agent). When nil,
	// Tab is a no-op mode cycle (does nothing, does NOT navigate to sessions).
	modeAgent modeAgent
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

// refreshThreadViewport recomputes the viewport content from the current thread.
// It is called in Update after any thread mutation or dimension change. It MUST
// NOT be called from View() — this is the Update/View boundary (design §C.2).
//
// Auto-scroll policy (design §C.4): capture AtBottom BEFORE SetContent; only
// call GotoBottom if the user was already at the bottom (stick-to-bottom).
// If the user has scrolled up, the YOffset is left untouched (freeze-on-scroll).
func (m Model) refreshThreadViewport() Model {
	width := m.viewport.Width
	content := m.thread.Render(width)
	if bc := m.breadcrumb.Render(width); bc != "" {
		content = bc + "\n" + content
	}
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(content)
	if atBottom {
		m.viewport.GotoBottom()
	}
	return m
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
		// WU-c: propagate dimensions to viewport and re-render content at new width.
		vw, vh := chatViewportSize(m)
		m.viewport.Width = vw
		m.viewport.Height = vh
		m = m.refreshThreadViewport()
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
			// WU-b: pre-compute "ago" strings in Update so renderSessions never
			// calls relativeTime (i.e. time.Since) from the View path.
			m.sessionsAgo = make([]string, len(m.sessions))
			for i, c := range m.sessions {
				m.sessionsAgo[i] = relativeTime(c.UpdatedAt)
			}
			// Update the resume-list panel (welcome + sessions rail) via copy-on-write.
			// setSessions now also pre-computes panel-level ago strings (WU-b).
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
			m.thread.append(&MsgDaimon{text: "no agent connected", time: nowHHMM(), styles: m.styles})
			return m, nil
		}
		return m, runCommandCmd(m.ag, msg.name, "", msg.allowDestructive)

	case commandResultMsg:
		// WU-a: the /mode command changes the agent mode externally; refresh the
		// cached m.mode from the authoritative source (the real agent) so the
		// render path sees the new value without a live call. trueMode() reads the
		// agent's ground truth, never the adapter's optimistic Tab override.
		if msg.name == "mode" {
			m.mode = m.trueMode()
		}
		text := msg.reply
		if msg.err != nil {
			text = "command failed: " + msg.err.Error()
		}
		m.thread.append(&MsgDaimon{text: text, time: nowHHMM(), styles: m.styles})
		m = m.refreshThreadViewport()
		return m, nil

	case switchModeMsg:
		// A3: async mode-persistence result for a Tab cycle. Reconcile the
		// adapter's optimistic override now that the switch has landed: clear it
		// iff it still matches this confirmed mode (a newer Tab keeps its own
		// override — race-safe, no flicker). Then re-sync m.mode from ground
		// truth, which reverts the optimistic value on error.
		if m.modeAgent != nil {
			m.modeAgent.ReconcileMode(msg.mode)
		}
		m.mode = m.trueMode()
		if msg.err != nil {
			m.thread.append(&MsgDaimon{
				text:   "mode switch failed: " + msg.err.Error(),
				time:   nowHHMM(),
				styles: m.styles,
			})
			m = m.refreshThreadViewport()
		}
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
				m.thread.append(&MsgUser{text: text, time: nowHHMM(), styles: m.styles})
				m.screen = screenChat
				m.focus = focusEditor
				m.footer = footerHints{screen: screenChat}
				// WU-c §C.6: reset viewport scroll on welcome→chat transition.
				m.viewport.SetContent("")
				m.viewport.GotoTop()
				m = m.refreshThreadViewport()
				return m, m.ch.submit(text, m.activeConvID)
			}

		case "tab":
			// A3: Tab cycles the agent mode (build → plan → review → build).
			// Sessions are now accessible via /sessions in the command palette.
			return m.cycleMode()

		case "ctrl+t":
			// ctrl+t navigates to the tools screen.
			// Bare 't' is NOT used to avoid breaking typed messages starting with 't'.
			m.prevScreen = screenWelcome
			m.screen = screenTools
			m.footer = footerHints{screen: screenTools}
			return m, loadToolsCmd(m.ag)

		case "ctrl+p":
			// A1: ctrl+p always opens the command palette.
			var cmds []agent.CommandInfo
			if m.ag != nil {
				cmds = m.ag.Commands()
			}
			m.overlays.Push(newCommandPalette(cmds, m.styles))
			return m, nil

		case "/":
			// A1: "/" with empty input opens the command palette.
			// Non-empty: fall through to the input bar (typed "/").
			if m.input.Value() == "" {
				var cmds []agent.CommandInfo
				if m.ag != nil {
					cmds = m.ag.Commands()
				}
				m.overlays.Push(newCommandPalette(cmds, m.styles))
				return m, nil
			}

		case "?":
			// A2: "?" with empty input opens the help overlay.
			if m.input.Value() == "" {
				m.overlays.Push(newHelpOverlay(m.styles))
				return m, nil
			}
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
		styles:     s,
		ch:         newTUIChannel(),
		channelID:  "tui",
		senderID:   "local_user",
		screen:     screenWelcome,
		focus:      focusEditor,
		input:      newInputBar(),
		rail:       newRail(s),
		breadcrumb: breadcrumb{styles: s},
		// WU-c (PR-2) prerequisite: initialize viewport so non-chat tests that
		// call newTestModel() never nil-deref when viewport methods are used.
		// Zero dimensions are safe — AtBottom/View on an unsized viewport are no-ops.
		viewport: viewport.New(0, 0),
		// WU-c (task 2.11): thread.styles for truncation marker rendering.
		thread: thread{styles: s},
	}
}

// ---------------------------------------------------------------------------
// Mode cycling helpers (A3)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// agentModeAdapter — production modeAgent wrapping *agent.Agent
// ---------------------------------------------------------------------------

// agentModeAdapter wraps *agent.Agent to satisfy the modeAgent interface.
// It maintains a local optimistic override so the TUI renders the new mode
// instantly (before the async store persistence via switchModeCmd completes).
type agentModeAdapter struct {
	ag            *agent.Agent
	localOverride string // non-empty while a switch is in-flight
}

func newAgentModeAdapter(ag *agent.Agent) *agentModeAdapter {
	if ag == nil {
		return nil
	}
	return &agentModeAdapter{ag: ag}
}

// trueMode returns the authoritative current mode for refreshing the cached
// m.mode in Update. It prefers the real agent (ground truth, bypassing the
// adapter's optimistic Tab override); in tests where no agent is wired it falls
// back to the modeAgent stub, then to the last cached value. Never called from
// a render path.
func (m Model) trueMode() string {
	if m.ag != nil {
		return m.ag.CurrentMode()
	}
	if m.modeAgent != nil {
		return m.modeAgent.CurrentMode()
	}
	return m.mode
}

// CurrentMode returns the optimistic override if set, else delegates to
// the real agent cache.
func (a *agentModeAdapter) CurrentMode() string {
	if a.localOverride != "" {
		return a.localOverride
	}
	return a.ag.CurrentMode()
}

// SetModeImmediate stores the new mode as a local optimistic override.
// The caller is responsible for issuing a switchModeCmd that calls
// agent.SetMode (persists to store + updates the real cache).
func (a *agentModeAdapter) SetModeImmediate(name string) {
	a.localOverride = name
}

// ReconcileMode clears the optimistic override iff it still equals the
// confirmed mode — i.e. the async switch this override anticipated has landed
// and the agent cache is now authoritative. If a newer Tab set a different
// override, confirmed != localOverride and the override is preserved, so rapid
// Tab cycling never flickers back to a stale agent value.
func (a *agentModeAdapter) ReconcileMode(confirmed string) {
	if a.localOverride == confirmed {
		a.localOverride = ""
	}
}

// ---------------------------------------------------------------------------
// modeOrderedNames is the canonical rotation order for Tab mode cycling.
// Matches agent.ModeNames() order: build → plan → review → build.
var modeOrderedNames = []string{"build", "plan", "review"}

// nextModeName returns the mode name that follows current in the rotation.
// Unknown current modes default to the first mode ("build").
func nextModeName(current string) string {
	for i, name := range modeOrderedNames {
		if name == current {
			return modeOrderedNames[(i+1)%len(modeOrderedNames)]
		}
	}
	return modeOrderedNames[0]
}

// cycleMode advances modeAgent to the next mode, updates the optimistic cache,
// and returns the updated model plus a switchModeCmd for async persistence.
// If modeAgent is nil, cycleMode is a no-op.
func (m Model) cycleMode() (Model, tea.Cmd) {
	if m.modeAgent == nil {
		return m, nil
	}
	// Compute the next mode from the cached m.mode, NOT from the adapter's
	// optimistic override: the override can be stale relative to ground truth
	// after a non-Tab mode change (e.g. the /mode command), whereas m.mode is
	// always kept in sync (cycleMode here, trueMode() on /mode and switchModeMsg).
	// This keeps Tab cycling correct after a /mode command.
	next := nextModeName(m.mode)
	m.modeAgent.SetModeImmediate(next)
	// WU-a: write the new mode into the cached field so renderLayout reads it
	// without calling CurrentMode() on the live agent.
	m.mode = next
	// Issue async persistence via the real agent (if wired).
	if m.ag != nil {
		return m, switchModeCmd(m.ag, next, m.channelID, m.senderID)
	}
	return m, nil
}

// switchModeCmd returns a tea.Cmd that calls ag.SetMode in a goroutine and
// delivers the result as a switchModeMsg. No mutation in the closure.
func switchModeCmd(ag *agent.Agent, name, channelID, senderID string) tea.Cmd {
	return func() tea.Msg {
		// Use a background context; no cancellation on TUI exit (V1 limitation,
		// same policy as runCommandCmd).
		err := ag.SetMode(context.Background(), channelID, senderID, name)
		return switchModeMsg{mode: name, err: err}
	}
}
