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
		md := &MsgDaimon{text: msg.text, styles: m.styles}
		m.thread.append(md)
		// Re-issue pump so the drain continues.
		return m, pumpEvents(m.events)

	// ------------------------------------------------------------------
	// Spinner tick for a running ToolLine
	// ------------------------------------------------------------------
	case spinnerTickMsg:
		// W1 FIX: copy-on-write — never mutate the shared backing array
		// through a pointer into the prior model's slice.
		idx := m.thread.findToolLineIdx(msg.callID)
		if idx >= 0 {
			oldTL := m.thread.items[idx].(*ToolLine) //nolint:forcetypeassert // findToolLineIdx guarantees *ToolLine
			if oldTL.state == toolRunning {
				tlCopy := *oldTL
				tlCopy.AdvanceSpinner()
				newItems := make([]threadItem, len(m.thread.items))
				copy(newItems, m.thread.items)
				newItems[idx] = &tlCopy
				m.thread.items = newItems
				return m, tlCopy.Tick()
			}
		}
		return m, nil

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
			state:  toolRunning,
			styles: m.styles,
		}
		m.thread.append(tl)
		// Start spinner animation.
		cmds = append(cmds, tl.Tick())
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
		// W1 FIX: copy-on-write — build a new items slice with the updated ToolLine
		// value so we never mutate through a pointer into the prior model's slice.
		idx := m.thread.findToolLineIdx(ev.ToolCallID)
		if idx >= 0 {
			oldTL := m.thread.items[idx].(*ToolLine) //nolint:forcetypeassert // findToolLineIdx guarantees *ToolLine
			tlCopy := *oldTL                         // value copy
			if ev.IsError {
				tlCopy.state = toolError
			} else {
				tlCopy.state = toolDone
			}
			tlCopy.stats.duration = time.Duration(ev.DurationMs) * time.Millisecond
			if ev.TokenCount > 0 {
				tlCopy.stats.tokens = ev.TokenCount
			}
			newItems := make([]threadItem, len(m.thread.items))
			copy(newItems, m.thread.items)
			newItems[idx] = &tlCopy
			m.thread.items = newItems
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
			// Switch to the error screen.
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

	case notify.EventReasoningStart:
		// Insert a Reasoning block in collapsed state.
		r := &Reasoning{styles: m.styles}
		m.thread.append(r)

	case notify.EventReasoningEnd:
		// Update the most recent Reasoning block with the completed text.
		// W1 FIX: copy-on-write — build a new items slice with the updated
		// Reasoning value; never mutate through a pointer into the prior model's slice.
		if ev.Text != "" {
			for i := len(m.thread.items) - 1; i >= 0; i-- {
				if r, ok := m.thread.items[i].(*Reasoning); ok {
					rCopy := *r
					rCopy.text = ev.Text
					newItems := make([]threadItem, len(m.thread.items))
					copy(newItems, m.thread.items)
					newItems[i] = &rCopy
					m.thread.items = newItems
					break
				}
			}
		}

	case notify.EventTokensUsage:
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

	case notify.EventTodolistChanged:
		// PR2b: Schedule a TodoListForConv re-read via a tea.Cmd (Cmd discipline —
		// no IO in Update). The result arrives as a todolistRefreshMsg which is
		// handled in Model.Update to update the todolist panel.
		cmds = append(cmds, fetchTodolist(m.ag, m.activeConvID))
	}

	// Re-issue pump so the drain loop continues.
	cmds = append(cmds, pumpEvents(m.events))
	return m, tea.Batch(cmds...)
}

// handleChatKey routes keyboard input on the chat screen.
// Focus routing: focusEditor → input bar; focusMain → thread navigation.
func (m Model) handleChatKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {

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
			mu := &MsgUser{text: text, styles: m.styles}
			m.thread.append(mu)
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
			// Copy-on-write: build a new items slice with the updated Reasoning value.
			newItems := make([]threadItem, len(m.thread.items))
			copy(newItems, m.thread.items)
			for i := len(newItems) - 1; i >= 0; i-- {
				if r, ok := newItems[i].(*Reasoning); ok {
					// Copy the Reasoning value so we don't mutate the prior model.
					rCopy := *r
					if rCopy.Expanded() {
						rCopy.Collapse()
					} else {
						rCopy.Expand()
					}
					newItems[i] = &rCopy
					break
				}
			}
			m.thread.items = newItems
			return m, nil
		}
		// Focus is on editor — fall through to input bar below.

	case "tab":
		// tab navigates to the sessions screen (per footer hint: "tab: sessions").
		// Focus-toggle is preserved via esc (esc already toggles focusEditor↔focusMain).
		m.prevScreen = screenChat
		m.screen = screenSessions
		m.footer = footerHints{screen: screenSessions}
		return m, loadSessionsCmd(m.store)

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
func renderChat(m Model, width, height int) string {
	content := m.thread.Render(width)
	if content == "" {
		return renderCenterPlaceholder(screenChat, width, height)
	}
	return content
}
