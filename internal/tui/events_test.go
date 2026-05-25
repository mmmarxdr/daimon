package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/channel"
	"daimon/internal/notify"
)

// TestPumpEvents_DeliversMsg verifies that pumpEvents returns a function that
// blocks until a message arrives and returns it as a tea.Msg.
func TestPumpEvents_DeliversMsg(t *testing.T) {
	ch := make(chan tea.Msg, 1)
	expected := busEventMsg{event: notify.Event{Type: notify.EventToolStart, ToolName: "bash"}}
	ch <- expected

	cmd := pumpEvents(ch)
	if cmd == nil {
		t.Fatal("pumpEvents returned nil cmd")
	}

	result := cmd()
	got, ok := result.(busEventMsg)
	if !ok {
		t.Fatalf("pumpEvents cmd returned %T, want busEventMsg", result)
	}
	if got.event.Type != notify.EventToolStart {
		t.Errorf("busEventMsg.event.Type = %q, want %q", got.event.Type, notify.EventToolStart)
	}
}

// TestBusEventMsg_IsTeaMsg verifies busEventMsg satisfies tea.Msg (compile-time).
var _ tea.Msg = busEventMsg{}

// TestSpinnerTickMsg_IsTeaMsg verifies spinnerTickMsg satisfies tea.Msg.
var _ tea.Msg = spinnerTickMsg{}

// ---------------------------------------------------------------------------
// W4 — non-vacuous update tests (REAL assertions on RETURNED model)
// ---------------------------------------------------------------------------

// TestUpdateChat_ToolStart_AddsToolLine verifies that receiving a busEventMsg
// with EventToolStart inserts a ToolLine into the RETURNED model's thread.
func TestUpdateChat_ToolStart_AddsToolLine(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	ev := notify.Event{
		Type:       notify.EventToolStart,
		ToolCallID: "call-abc",
		ToolName:   "bash",
	}
	m2, cmd := m.Update(busEventMsg{event: ev})
	got := m2.(Model)

	// Assert on the RETURNED model — not via pointer aliasing.
	tl := got.thread.findToolLine("call-abc")
	if tl == nil {
		t.Fatal("after EventToolStart, returned model thread should have a ToolLine with the given callID")
	}
	if tl.state != toolRunning {
		t.Errorf("ToolLine.state = %v, want toolRunning", tl.state)
	}
	if tl.name != "bash" {
		t.Errorf("ToolLine.name = %q, want %q", tl.name, "bash")
	}
	// Cmd must be non-nil: spinner tick + pump re-arm.
	if cmd == nil {
		t.Error("Update must return non-nil cmd (spinner + pump)")
	}
}

// TestUpdateChat_ToolEnd_TransitionsState verifies EventToolEnd moves a ToolLine
// from toolRunning to toolDone ON THE RETURNED model (not via pointer aliasing).
func TestUpdateChat_ToolEnd_TransitionsState(t *testing.T) {
	s := newTuiStyles()
	m := newTestModel()
	m.screen = screenChat

	// Pre-populate thread with a running tool line.
	tl := &ToolLine{callID: "call-xyz", name: "read_file", state: toolRunning, styles: s}
	m.thread.append(tl)

	ev := notify.Event{
		Type:       notify.EventToolEnd,
		ToolCallID: "call-xyz",
		ToolName:   "read_file",
		DurationMs: 123,
		IsError:    false,
	}
	m2, cmd := m.Update(busEventMsg{event: ev})
	got := m2.(Model)

	// Assert on RETURNED model.
	found := got.thread.findToolLine("call-xyz")
	if found == nil {
		t.Fatal("ToolLine should still be present in returned model after EventToolEnd")
	}
	if found.state != toolDone {
		t.Errorf("ToolLine.state = %v, want toolDone after EventToolEnd", found.state)
	}
	if found.stats.duration != 123*time.Millisecond {
		t.Errorf("ToolLine.stats.duration = %v, want 123ms", found.stats.duration)
	}
	// Pump must be re-armed.
	if cmd == nil {
		t.Error("Update must return non-nil cmd (pump re-arm)")
	}
}

// TestUpdateChat_ToolEnd_Error_SetsErrorState verifies EventToolEnd with IsError=true
// transitions the ToolLine to toolError on the RETURNED model.
func TestUpdateChat_ToolEnd_Error_SetsErrorState(t *testing.T) {
	s := newTuiStyles()
	m := newTestModel()
	m.screen = screenChat

	tl := &ToolLine{callID: "call-err", name: "write_file", state: toolRunning, styles: s}
	m.thread.append(tl)

	ev := notify.Event{
		Type:       notify.EventToolEnd,
		ToolCallID: "call-err",
		IsError:    true,
	}
	m2, _ := m.Update(busEventMsg{event: ev})
	got := m2.(Model)

	// Assert on RETURNED model.
	found := got.thread.findToolLine("call-err")
	if found == nil {
		t.Fatal("ToolLine should remain in returned model after error")
	}
	if found.state != toolError {
		t.Errorf("ToolLine.state = %v, want toolError after IsError=true EventToolEnd", found.state)
	}
}

// TestUpdateChat_TurnCompleted_IsSignalOnly verifies that EventTurnCompleted
// does NOT append a MsgDaimon to the thread (C4 fix: agentReplyMsg is the
// single source of truth for thread appends; EventTurnCompleted is telemetry only).
func TestUpdateChat_TurnCompleted_IsSignalOnly(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	ev := notify.Event{
		Type: notify.EventTurnCompleted,
		Text: "Here is the answer.",
	}
	m2, cmd := m.Update(busEventMsg{event: ev})
	got := m2.(Model)

	// Thread must remain EMPTY — EventTurnCompleted does NOT append text.
	if len(got.thread.items) != 0 {
		t.Errorf("EventTurnCompleted must NOT append to thread (C4 fix); got %d items", len(got.thread.items))
	}
	// But the pump must still be re-armed.
	if cmd == nil {
		t.Error("EventTurnCompleted must still return non-nil cmd (pump re-arm)")
	}
}

// TestUpdateChat_AgentReplyMsg_AppendsMsgDaimon verifies that an agentReplyMsg
// from TUIChannel.Send is appended to the returned model's thread.
func TestUpdateChat_AgentReplyMsg_AppendsMsgDaimon(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	m2, cmd := m.Update(agentReplyMsg{text: "pong from agent"})
	got := m2.(Model)

	// Assert on RETURNED model — not via aliasing.
	if len(got.thread.items) == 0 {
		t.Error("thread.items in RETURNED model should have at least one item after agentReplyMsg")
	}
	// Verify it's a MsgDaimon with correct text.
	foundDaimon := false
	for _, item := range got.thread.items {
		if md, ok := item.(*MsgDaimon); ok {
			if md.text == "pong from agent" {
				foundDaimon = true
				break
			}
		}
	}
	if !foundDaimon {
		t.Error("returned model should have a MsgDaimon with text 'pong from agent'")
	}
	// Pump must be re-armed.
	if cmd == nil {
		t.Error("agentReplyMsg handler must return non-nil cmd (pump re-arm)")
	}
}

// TestUpdateChat_Enter_SubmitsText verifies that pressing Enter on the chat
// screen with text returns a non-nil Cmd AND appends a MsgUser to the returned model.
func TestUpdateChat_Enter_SubmitsText(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	// Simulate text in input bar.
	m.input.ti.SetValue("hello agent")

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)

	// Must return a cmd (the submit cmd).
	if cmd == nil {
		t.Error("pressing Enter with text in input bar must return a non-nil Cmd (submit)")
	}

	// The returned model must have a MsgUser appended (optimistic append).
	foundUser := false
	for _, item := range got.thread.items {
		if mu, ok := item.(*MsgUser); ok {
			if mu.text == "hello agent" {
				foundUser = true
				break
			}
		}
	}
	if !foundUser {
		t.Error("returned model must have MsgUser with entered text appended to thread")
	}

	// Executing the cmd must eventually yield promptSentMsg (synchronous path: inbox==nil).
	msg := cmd()
	if _, ok := msg.(promptSentMsg); !ok {
		t.Errorf("submit cmd returned %T, want promptSentMsg", msg)
	}
}

// TestUpdateChat_Enter_EmptyInput_NoSubmit verifies that pressing Enter with
// empty input does NOT append a MsgUser.
func TestUpdateChat_Enter_EmptyInput_NoSubmit(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	m.input.ti.SetValue("") // empty
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)

	// RETURNED model must NOT have any MsgUser.
	for _, item := range got.thread.items {
		if _, ok := item.(*MsgUser); ok {
			t.Error("empty Enter must NOT append MsgUser to thread")
		}
	}
}

// ---------------------------------------------------------------------------
// C3 — busEventMsg on non-chat screen must re-arm pump (RED → GREEN)
// ---------------------------------------------------------------------------

// TestUpdate_BusEventMsg_NonChatScreen_RearmsPump verifies that a busEventMsg
// arriving while screen != screenChat still re-arms the pump (non-nil cmd)
// so events never stop flowing after a screen transition.
func TestUpdate_BusEventMsg_NonChatScreen_RearmsPump(t *testing.T) {
	m := newTestModel()
	m.screen = screenWelcome // NOT the chat screen

	// Wire a real events channel so the model can re-arm.
	evCh := make(chan tea.Msg, 1)
	m.events = evCh

	ev := notify.Event{Type: notify.EventToolStart, ToolCallID: "c3-call", ToolName: "bash"}
	_, cmd := m.Update(busEventMsg{event: ev})

	// The pump MUST be re-armed regardless of screen.
	if cmd == nil {
		t.Fatal("busEventMsg on non-chat screen must return non-nil cmd (pump re-arm)")
	}

	// Feed another event and verify the pump cmd can still receive it.
	nextEv := busEventMsg{event: notify.Event{Type: notify.EventTurnCompleted, Text: "hello"}}
	evCh <- nextEv
	result := cmd()
	if result == nil {
		t.Error("re-armed pump cmd must return a non-nil msg from the channel")
	}
}

// ---------------------------------------------------------------------------
// C4 — duplicate MsgDaimon deduplication (RED → GREEN)
// ---------------------------------------------------------------------------

// TestUpdateChat_NoDoubleMsgDaimon verifies that driving both an agentReplyMsg
// and an EventTurnCompleted with text for one turn results in EXACTLY ONE
// MsgDaimon in the thread (not two).
func TestUpdateChat_NoDoubleMsgDaimon(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	// Step 1: agent reply arrives via TUIChannel.Send path.
	m2, _ := m.Update(agentReplyMsg{text: "the answer"})
	got1 := m2.(Model)

	// Step 2: EventTurnCompleted also arrives with the same text (bus path).
	ev := notify.Event{Type: notify.EventTurnCompleted, Text: "the answer"}
	m3, _ := got1.Update(busEventMsg{event: ev})
	got2 := m3.(Model)

	// Count MsgDaimon items with the matching text.
	count := 0
	for _, item := range got2.thread.items {
		if md, ok := item.(*MsgDaimon); ok {
			if md.text == "the answer" {
				count++
			}
		}
	}
	if count != 1 {
		t.Errorf("expected EXACTLY 1 MsgDaimon with text 'the answer', got %d", count)
	}
}

// ---------------------------------------------------------------------------
// FIX 1 (CRITICAL) — Send after Stop must never panic; wireEvents goroutine exits
// ---------------------------------------------------------------------------

// TestTUIChannel_Send_AfterStop_NoPanic verifies that calling Send() after
// Stop() (with a cancelled ctx so Send can't block) does NOT panic. Previously,
// Stop() closed c.out and a racing Send() would panic when the runtime selected
// the send case on a closed channel.
func TestTUIChannel_Send_AfterStop_NoPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	ch := newTUIChannel()
	// Simulate Start so ctx is captured.
	ch.ctx = ctx

	// Stop first (signals done, does NOT close c.out).
	if err := ch.Stop(); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}

	// Now cancel the ctx to make Send take the ctx.Done() branch rather than
	// blocking, mirroring real shutdown where cancel() fires before Stop().
	cancel()

	// Send must not panic — even though Stop() was called concurrently.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Send() panicked after Stop(): %v", r)
		}
	}()

	_ = ch.Send(ctx, channel.OutgoingMessage{Text: "should not panic"})
}

// TestTUIChannel_Stop_Idempotent verifies that calling Stop() twice does not panic.
func TestTUIChannel_Stop_Idempotent(t *testing.T) {
	ch := newTUIChannel()

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Stop() panicked on second call: %v", r)
		}
	}()

	_ = ch.Stop()
	_ = ch.Stop() // must not panic
}

// TestWireEvents_GoroutineExits_AfterStop verifies that the agent-reply
// forwarding goroutine started by wireEvents exits cleanly after Stop() is
// called (i.e., done channel is closed), so there are no goroutine leaks.
func TestWireEvents_GoroutineExits_AfterStop(t *testing.T) {
	bus := notify.NewEventBus(0, 0, 0)
	defer bus.Close()

	ch := newTUIChannel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	evCh := wireEvents(ctx, bus, ch)

	// Send a message before stopping to confirm the goroutine is running.
	go func() {
		_ = ch.Send(context.Background(), channel.OutgoingMessage{Text: "hello"})
	}()

	// Drain the message.
	select {
	case <-evCh:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first message from wireEvents")
	}

	// Now stop — the goroutine must exit (done channel closed).
	_ = ch.Stop()

	// The done channel must be closed (unblocks immediately after Stop()).
	// This confirms the forwarding goroutine will exit.
	select {
	case <-ch.done:
		// correct — done was closed by Stop()
	case <-time.After(time.Second):
		t.Error("ch.done was not closed after Stop() — goroutine may have leaked")
	}
}
