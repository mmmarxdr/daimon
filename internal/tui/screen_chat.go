package tui

// screen_chat.go — chat screen handler (screen 02, REQ-5/REQ-6).
//
// updateChat handles all tea.Msg types relevant to the chat screen:
//   - busEventMsg: bus events (tool lifecycle, turn completion, telemetry, subagent)
//   - agentReplyMsg: completed turn text from TUIChannel.Send
//   - spinnerTickMsg: braille spinner advancement for running ToolLines
//   - tea.KeyMsg: input routing (Enter → submit, focus switching)
//
// RULE: No IO in Update. All side effects return tea.Cmd.
// RULE: pumpEvents(m.events) is re-issued after every busEventMsg / agentReplyMsg
//       so the drain loop never stops.

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/agent"
	"daimon/internal/notify"
)

// updateChat is the screenChat Update handler. It is called from Model.Update
// when m.screen == screenChat.
func (m Model) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ------------------------------------------------------------------
	// Bus events (notify.Bus → busEventMsg via pumpEvents)
	// ------------------------------------------------------------------
	case busEventMsg:
		return m.handleBusEvent(msg.event)

	// ------------------------------------------------------------------
	// Agent reply (TUIChannel.Send → agentReplyMsg via pumpEvents)
	// ------------------------------------------------------------------
	case agentReplyMsg:
		md := &MsgDaimon{text: msg.text, time: nowHHMM(), styles: m.styles}
		m.thread.append(md)
		m = m.refreshThreadViewport()
		// Re-issue pump so the drain continues.
		return m, pumpEvents(m.events)

	// ------------------------------------------------------------------
	// Batch spinner tick — advances ALL running ToolLines in one pass
	// ------------------------------------------------------------------
	case spinnerTickMsg:
		// Design §D.5: single model-level ticker (no callID).
		// runningToolIdxs() finds all toolRunning lines in one pass.
		// If none are running, self-stop: return nil cmd (no re-arm).
		idxs := m.thread.runningToolIdxs()
		if len(idxs) == 0 {
			m.spinnerActive = false
			return m, nil
		}
		// own() allocates a fresh slice (O(n) copy, once per tick regardless of k).
		// All k in-place slot writes below are provably non-aliasing (design §D.6).
		nt := m.thread.own()
		for _, i := range idxs {
			old := nt.items[i].(*ToolLine) //nolint:forcetypeassert // runningToolIdxs guarantees *ToolLine
			clone := *old                  // value copy of the ToolLine struct (deep-clone pointer)
			clone.spinnerFrame = (clone.spinnerFrame + 1) % len(brailleSpinner)
			nt.items[i] = &clone // replace pointer in the owned slice
		}
		m.thread = nt
		m = m.refreshThreadViewport()
		return m, spinnerTickCmd() // re-arm single ticker

	// ------------------------------------------------------------------
	// promptSentMsg: input bar submitted — clear and show waiting state
	// ------------------------------------------------------------------
	case promptSentMsg:
		m.input.Reset()
		return m, nil

	// ------------------------------------------------------------------
	// Key events — route by focus region
	// ------------------------------------------------------------------
	case tea.KeyMsg:
		return m.handleChatKey(msg)
	}

	// Unhandled msgs (e.g. blink from textinput) — forward to input bar.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// applyDenial captures a policy/mode denial from ev into the model's error
// state and returns the updated model (copy-on-write throughout).
//
// It is shared by TWO callers:
//   - handleBusEvent (screen==screenChat): first denial transition. The caller
//     additionally sets prevScreen, screen=screenError, and footer.
//   - the global busEventMsg handler (screen==screenError): re-entrant denial.
//     The caller stays on screenError; no screen transition is needed.
//
// What applyDenial does:
//  1. Sets errorToolName + errorReason to the new denial.
//  2. Appends to recentDenials (copy-on-write slice, capped at 10).
//  3. Updates recentDenialsPanel in the rail (copy-on-write).
//  4. Updates activePolicyPanel mode to ag.CurrentMode() (guards nil ag).
func (m Model) applyDenial(ev notify.Event) Model {
	m.errorToolName = ev.ToolName
	m.errorReason = ev.Error

	// Copy-on-write: build a fresh slice so we never alias the prior model's
	// backing array. Cap at 10 most-recent denials.
	const maxDenials = 10
	prev := m.recentDenials
	next := make([]denialEntry, len(prev)+1)
	copy(next, prev)
	next[len(prev)] = denialEntry{tool: ev.ToolName, reason: ev.Error}
	if len(next) > maxDenials {
		next = next[len(next)-maxDenials:]
	}
	m.recentDenials = next

	// Update the recent-denials rail panel (copy-on-write).
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		if p, ok := panels[panelRecentDenials].(*recentDenialsPanel); ok {
			cp := *p
			cp.setDenials(m.recentDenials)
			panels[panelRecentDenials] = &cp
		}
	})

	// Fix 1: Update the active-policy panel's mode at denial time so it always
	// reflects the mode that triggered this denial, not the startup snapshot.
	// Guard m.ag == nil for tests and non-RunTUI paths.
	if m.ag != nil {
		currentMode := m.ag.CurrentMode()
		m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
			if p, ok := panels[panelActivePolicy].(*activePolicyPanel); ok {
				cp := *p
				cp.setMode(currentMode)
				panels[panelActivePolicy] = &cp
			}
		})
	}

	return m
}

// handleBusEvent processes a single notify.Event and returns the updated model
// plus any commands (spinner tick, pump re-issue).
func (m Model) handleBusEvent(ev notify.Event) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Track the active conversation ID from turn events.
	// EventTurnStarted carries Meta["conversation_id"] when the agent starts a turn.
	if ev.Type == notify.EventTurnStarted {
		if id := ev.Meta["conversation_id"]; id != "" {
			m.activeConvID = id
		}
	}

	switch ev.Type {

	case notify.EventToolStart:
		// Insert a new ToolLine in toolRunning state.
		tl := &ToolLine{
			callID: ev.ToolCallID,
			name:   ev.ToolName,
			input:  toolInputSummary(ev.Meta["input"]),
			state:  toolRunning,
			styles: m.styles,
		}
		m.thread.append(tl)
		m = m.refreshThreadViewport()
		// Arm the single model-level batch ticker — deduplicated by spinnerActive.
		// Only arm when no ticker is already running; a second EventToolStart while
		// an existing ticker is live must NOT stack a second ticker (design §D.7).
		if !m.spinnerActive {
			m.spinnerActive = true
			cmds = append(cmds, spinnerTickCmd())
		}
		// PR2b: Update telemetry panel tool-call count (copy-on-write).
		m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
			if tp, ok := panels[panelTelemetry].(*telemetryPanel); ok {
				cp := *tp
				cp.accumulate(ev)
				panels[panelTelemetry] = &cp
			}
		})

	case notify.EventToolEnd:
		// Transition existing ToolLine to done or error.
		// COW via own(): own() allocates a fresh backing array; the in-place slot
		// write below is safe and cannot alias a prior model snapshot (design §D.7).
		idx := m.thread.findToolLineIdx(ev.ToolCallID)
		if idx >= 0 {
			nt := m.thread.own()
			oldTL := nt.items[idx].(*ToolLine) //nolint:forcetypeassert // findToolLineIdx guarantees *ToolLine
			tlCopy := *oldTL                   // value copy of the ToolLine struct
			if ev.IsError {
				tlCopy.state = toolError
			} else {
				tlCopy.state = toolDone
			}
			tlCopy.stats.duration = time.Duration(ev.DurationMs) * time.Millisecond
			if ev.TokenCount > 0 {
				tlCopy.stats.tokens = ev.TokenCount
			}
			nt.items[idx] = &tlCopy
			m.thread = nt
			m = m.refreshThreadViewport()
		}
		// PR2b: Update telemetry panel error count (copy-on-write).
		m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
			if tp, ok := panels[panelTelemetry].(*telemetryPanel); ok {
				cp := *tp
				cp.accumulate(ev)
				panels[panelTelemetry] = &cp
			}
		})
		// PR5: Policy/mode DENIAL detection — only Meta["denied"]=="true" events
		// hijack the screen. Runtime errors (tool crash, not-found) stay in the
		// chat thread (existing ToolLine toolError behavior above is preserved).
		if ev.Meta["denied"] == "true" {
			m = m.applyDenial(ev)
			// First transition: come from screenChat → set prevScreen + screen + footer.
			m.prevScreen = screenChat
			m.screen = screenError
			m.footer = footerHints{screen: screenError}
		}

	case notify.EventTurnCompleted:
		// C4 FIX: agentReplyMsg (TUIChannel.Send path) is the SINGLE source of
		// truth for thread appends. Consuming ev.Text here for telemetry only —
		// do NOT append a MsgDaimon. Doing so would produce a duplicate whenever
		// both agentReplyMsg and EventTurnCompleted arrive for the same turn.
		_ = ev.Text // turn-complete signal consumed; text already in thread via agentReplyMsg

	case notify.EventSubagentSpawned:
		// Insert a new Subagent mini-thread.
		id := ev.Meta["subagent_id"]
		if id == "" {
			id = ev.ToolCallID
		}
		sa := &Subagent{id: id, styles: m.styles}
		m.thread.append(sa)
		m = m.refreshThreadViewport()

	case notify.EventReasoningStart:
		// Insert a Reasoning block in collapsed state.
		r := &Reasoning{styles: m.styles}
		m.thread.append(r)
		m = m.refreshThreadViewport()

	case notify.EventReasoningEnd:
		// Update the most recent Reasoning block with the completed text.
		// COW via own(): own() allocates a fresh backing array; the in-place slot
		// write below is safe and cannot alias a prior model snapshot (design §D.7).
		if ev.Text != "" {
			nt := m.thread.own()
			for i := len(nt.items) - 1; i >= 0; i-- {
				if r, ok := nt.items[i].(*Reasoning); ok {
					rCopy := *r
					rCopy.text = ev.Text
					rCopy.duration = time.Duration(ev.DurationMs) * time.Millisecond
					nt.items[i] = &rCopy
					break
				}
			}
			m.thread = nt
			m = m.refreshThreadViewport()
		}

	case notify.EventTokensUsage:
		// Inc.2: accumulate the session breadcrumb (one EventTokensUsage per turn).
		// tokens in/out and elapsed come from ev.Meta; the event timestamp is the
		// autosave proxy (the conversation is persisted at turn end — loop.go).
		bc := m.breadcrumb
		bc.turns++
		bc.tokensIn += atoiSafe(ev.Meta["input_tokens"])
		bc.tokensOut += atoiSafe(ev.Meta["output_tokens"])
		// Pre-compute the relative "ago" string here (Update may read the clock);
		// breadcrumb.Render stays pure.
		bc.ago = relativeTime(ev.Timestamp)
		if bc.label == "" {
			bc.label = breadcrumbLabel(ev.Meta["conv_id"], m.activeConvID)
		}
		m.breadcrumb = bc
		// WU-c: breadcrumb is baked into viewport content; refresh after change.
		m = m.refreshThreadViewport()

		// PR2b: Update telemetry and context-meter rail panels.
		// Copy-on-write: copy each panel value, mutate the copy, replace in map.
		m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
			if tp, ok := panels[panelTelemetry].(*telemetryPanel); ok {
				cp := *tp
				cp.accumulate(ev)
				panels[panelTelemetry] = &cp
			}
			if cm, ok := panels[panelContextMeter].(*contextMeterPanel); ok {
				cp := *cm
				cp.accumulate(ev)
				panels[panelContextMeter] = &cp
			}
		})

	case notify.EventSubagentCompleted, notify.EventSubagentFailed:
		// PR-b: Update telemetry panel with subagent lifecycle events (copy-on-write).
		// EventSubagentCompleted: REPLACE tokens with authoritative total, set done=true.
		// EventSubagentFailed: set done=true, failed=true — do NOT read Meta["tokens"].
		m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
			if tp, ok := panels[panelTelemetry].(*telemetryPanel); ok {
				cp := *tp
				cp.accumulate(ev)
				panels[panelTelemetry] = &cp
			}
		})

	case notify.EventTodolistChanged:
		// PR2b: Schedule a TodoListForConv re-read via a tea.Cmd (Cmd discipline —
		// no IO in Update). The result arrives as a todolistRefreshMsg which is
		// handled in Model.Update to update the todolist panel.
		cmds = append(cmds, fetchTodolist(m.ag, m.activeConvID))

	case notify.EventMemoryChanged:
		// PR-c: Schedule a SearchMemory re-read via a tea.Cmd (Cmd discipline —
		// no IO in Update). The scopeID comes from the event meta (option a design).
		// The result arrives as a memoryRefreshMsg handled in Model.Update.
		cmds = append(cmds, fetchMemory(m.store, ev.Meta["scope_id"]))
	}

	// Re-issue pump so the drain loop continues.
	cmds = append(cmds, pumpEvents(m.events))
	return m, tea.Batch(cmds...)
}

// handleChatKey routes keyboard input on the chat screen.
// Focus routing: focusEditor → input bar; focusMain → thread navigation.
//
// Scroll key routing (design §C.7):
//   - pgup/pgdown/ctrl+u/ctrl+d: always forward to viewport (never text-entry keys).
//   - up/down: forward to viewport ONLY when focusMain (thread navigation);
//     when focusEditor, they belong to the input bar.
func (m Model) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {

	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		// WU-c §C.7: always route page scroll keys to the viewport regardless of focus.
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case "up", "down":
		// WU-c §C.7: route arrow keys to viewport only when in thread navigation mode.
		if m.focus != focusEditor {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		// focusEditor: fall through to input bar (handled at the bottom).

	case "enter":
		if m.focus == focusEditor || m.focus == focusNone {
			text := m.input.Value()
			if text == "" {
				// Empty input — do not submit; forward to input for blink.
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
			// Append user message to thread immediately (optimistic).
			mu := &MsgUser{text: text, time: nowHHMM(), styles: m.styles}
			m.thread.append(mu)
			m = m.refreshThreadViewport()
			// Submit via channel (IO-free: runs in Cmd goroutine).
			// Pass activeConvID so a resumed session binds to the right conversation (AD-7).
			submitCmd := m.ch.submit(text, m.activeConvID)
			return m, submitCmd
		}

	case "r":
		// FIX 3: 'r' toggles the most-recent Reasoning ONLY when focus is on
		// the thread (focusMain). When focusEditor is active, 'r' must fall
		// through to the input bar so the user can type 'r' normally.
		if m.focus != focusEditor && m.focus != focusNone {
			// COW via own(): own() allocates a fresh backing array; in-place
			// slot write below cannot alias a prior model snapshot (design §D.7).
			nt := m.thread.own()
			for i := len(nt.items) - 1; i >= 0; i-- {
				if r, ok := nt.items[i].(*Reasoning); ok {
					rCopy := *r
					if rCopy.Expanded() {
						rCopy.Collapse()
					} else {
						rCopy.Expand()
					}
					nt.items[i] = &rCopy
					break
				}
			}
			m.thread = nt
			m = m.refreshThreadViewport()
			return m, nil
		}
		// Focus is on editor — fall through to input bar below.

	case "tab":
		// A3: Tab cycles the agent mode (build → plan → review → build).
		// Sessions are now accessible via /sessions in the command palette.
		return m.cycleMode()

	case "ctrl+t":
		// ctrl+t navigates to the tools screen.
		// Bare 't' is NOT used to avoid breaking typed messages starting with 't'.
		m.prevScreen = screenChat
		m.screen = screenTools
		m.footer = footerHints{screen: screenTools}
		return m, loadToolsCmd(m.ag)

	case "esc":
		// FIX 3: Esc toggles focusEditor ↔ focusMain so the reasoning toggle
		// ('r') remains reachable after the user switches to thread navigation.
		// Previously Esc always returned to focusEditor unconditionally.
		if m.focus == focusEditor || m.focus == focusNone {
			m.focus = focusMain
			m.input.Blur()
		} else {
			m.focus = focusEditor
			m.input.Focus()
		}
		return m, nil

	case "ctrl+p":
		// A1: ctrl+p always opens the command palette (mirrors "/" behavior).
		var cmds []agent.CommandInfo
		if m.ag != nil {
			cmds = m.ag.Commands()
		}
		m.overlays.Push(newCommandPalette(cmds, m.styles))
		return m, nil

	case "/":
		// PR3a: "/" at the start of an empty input opens the command palette.
		// If the input is non-empty the slash falls through to the input bar
		// so users can type "/" inside a message normally.
		if m.input.Value() == "" {
			// Guard against nil agent (tests / welcome screen).
			var cmds []agent.CommandInfo
			if m.ag != nil {
				cmds = m.ag.Commands()
			}
			m.overlays.Push(newCommandPalette(cmds, m.styles))
			return m, nil
		}
		// Non-empty input: fall through to input bar.

	case "?":
		// A2: "?" with empty input opens the help overlay.
		if m.input.Value() == "" {
			m.overlays.Push(newHelpOverlay(m.styles))
			return m, nil
		}
		// Non-empty: fall through to input bar.
	}

	// Forward remaining keys to the focused region.
	if m.focus == focusEditor || m.focus == focusNone {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

// renderChat renders the center column for the chat screen.
//
// WU-c (design §C.3): delegates to m.viewport.View() so only the visible
// window is materialized (O(viewport-height), not O(N items)). The breadcrumb
// is baked into the viewport content by refreshThreadViewport (called in
// Update after every thread mutation), so it scrolls with the thread (ADR-3).
//
// Guard: when thread is empty, return the placeholder instead of an empty
// viewport view so the welcome-like "start chatting" hint is still shown.
func renderChat(m Model, width, height int) string {
	if len(m.thread.items) == 0 {
		return renderCenterPlaceholder(screenChat, width, height)
	}
	return m.viewport.View()
}
