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
		// Telemetry rail update (PR2b rail panels will consume this).
		// Store on activeConvID for future rail panels; no thread mutation here.
		_ = ev.TokenCount
		_ = ev.CostUSD

	case notify.EventTodolistChanged:
		// Todolist rail refresh (PR2b rail panels will react to this).
		// No thread mutation; the rail panel will call TodoListForConv.
		_ = ev.ChannelID
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
			submitCmd := m.ch.submit(text)
			return m, submitCmd
		}

	case "r":
		// Toggle the most-recent Reasoning item's expanded state.
		// The collapsed view shows "press r to expand" as the affordance.
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

	case "tab":
		// Switch focus between editor and main (thread navigation).
		if m.focus == focusEditor {
			m.focus = focusMain
			m.input.Blur()
		} else {
			m.focus = focusEditor
			m.input.Focus()
		}
		return m, nil

	case "esc":
		// Return focus to editor from main.
		m.focus = focusEditor
		m.input.Focus()
		return m, nil
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
