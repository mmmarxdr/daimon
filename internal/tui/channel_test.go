package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/channel"
)

// Compile-time assertion: TUIChannel must satisfy channel.Channel.
var _ channel.Channel = (*TUIChannel)(nil)

// TestTUIChannel_ImplementsChannelInterface is the runtime companion —
// ensures the interface check above surfaces in the test binary.
func TestTUIChannel_ImplementsChannelInterface(t *testing.T) {
	ch := newTUIChannel()
	// Must not panic and must return "tui".
	if ch.Name() != "tui" {
		t.Errorf("Name() = %q, want %q", ch.Name(), "tui")
	}
}

// TestTUIChannel_Submit_EnqueuesIncomingMessage verifies that calling submit()
// and running the returned tea.Cmd sends an IncomingMessage onto the inbox
// with the expected fields.
func TestTUIChannel_Submit_EnqueuesIncomingMessage(t *testing.T) {
	inbox := make(chan channel.IncomingMessage, 1)
	ch := newTUIChannel()
	_ = ch.Start(context.Background(), inbox)

	cmd := ch.submit("hello world")
	msg := cmd() // execute the cmd synchronously

	// The cmd should return a promptSentMsg.
	if _, ok := msg.(promptSentMsg); !ok {
		t.Fatalf("cmd() returned %T, want promptSentMsg", msg)
	}

	// inbox should have one IncomingMessage with correct fields.
	select {
	case im := <-inbox:
		if im.ChannelID != "tui" {
			t.Errorf("ChannelID = %q, want %q", im.ChannelID, "tui")
		}
		if im.SenderID != "local_user" {
			t.Errorf("SenderID = %q, want %q", im.SenderID, "local_user")
		}
		if im.Content.TextOnly() != "hello world" {
			t.Errorf("Content.TextOnly() = %q, want %q", im.Content.TextOnly(), "hello world")
		}
		if im.ID == "" {
			t.Errorf("ID must be non-empty (uuid)")
		}
		if im.Timestamp.IsZero() {
			t.Errorf("Timestamp must be non-zero")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: no IncomingMessage received on inbox")
	}
}

// TestTUIChannel_Send_EnqueuesAgentReplyMsg verifies that calling Send()
// with an OutgoingMessage enqueues an agentReplyMsg onto the out channel.
func TestTUIChannel_Send_EnqueuesAgentReplyMsg(t *testing.T) {
	ch := newTUIChannel()
	ctx := context.Background()

	err := ch.Send(ctx, channel.OutgoingMessage{
		ChannelID:   "tui",
		RecipientID: "local_user",
		Text:        "hello from agent",
	})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	select {
	case raw := <-ch.out:
		reply, ok := raw.(agentReplyMsg)
		if !ok {
			t.Fatalf("out received %T, want agentReplyMsg", raw)
		}
		if reply.text != "hello from agent" {
			t.Errorf("agentReplyMsg.text = %q, want %q", reply.text, "hello from agent")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: no agentReplyMsg received on out channel")
	}
}

// TestTUIChannel_Stop_ReturnsNil verifies Stop() is a clean no-op.
func TestTUIChannel_Stop_ReturnsNil(t *testing.T) {
	ch := newTUIChannel()
	if err := ch.Stop(); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}

// TestTUIChannel_Submit_NilInbox_DoesNotBlock verifies that calling submit()
// on a TUIChannel whose Start has NOT been called (inbox == nil) returns
// promptly without blocking. Before the fix, c.inbox <- im would block
// forever, leaking the goroutine and causing this test to time out.
//
// RED: without the nil-inbox guard the goroutine blocks indefinitely.
func TestTUIChannel_Submit_NilInbox_DoesNotBlock(t *testing.T) {
	ch := newTUIChannel()
	// inbox is nil — Start has NOT been called.

	done := make(chan tea.Msg, 1)
	go func() {
		cmd := ch.submit("will this block?")
		done <- cmd()
	}()

	select {
	case msg := <-done:
		// Must return promptSentMsg (or at minimum must not block).
		if _, ok := msg.(promptSentMsg); !ok {
			t.Errorf("submit() on nil inbox returned %T, want promptSentMsg", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("submit() on nil inbox blocked for 2s — goroutine leak (nil-inbox guard missing)")
	}
}

// TestTUIChannel_WiringIntegrity verifies that the TUIChannel passed to the
// model construction path is the SAME instance that the mux (agent) wires.
//
// Specifically:
//   - A submit() call on the model's channel (m.ch) reaches the inbox that
//     Start() initialised (as the mux would).
//   - A Send() from the agent side lands on m.ch.out (the channel the model reads).
//
// RED (orphan-channel bug): without the fix, runTUIWithStdin creates its own
// internal channel via newTUIChannel(), so m.ch is never the same object that
// the mux called Start on.  The inbox stays nil → submit silently drops; and
// the agent writes to tuiCh.out which nobody reads.
func TestTUIChannel_WiringIntegrity(t *testing.T) {
	// Simulate what the fixed tui_cmd.go and runTUIWithStdin do:
	// 1. Caller constructs the channel once.
	tuiCh := NewTUIChannel()

	// 2. Mux calls Start (as agent.Run → mux.Start → tuiCh.Start would do).
	inbox := make(chan channel.IncomingMessage, 4)
	ctx := context.Background()
	if err := tuiCh.Start(ctx, inbox); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 3. Model is constructed with that exact channel (as the fixed runTUIWithStdin does).
	m := Model{
		styles:    newTuiStyles(),
		ch:        tuiCh,
		channelID: "tui",
		senderID:  "local_user",
		screen:    screenWelcome,
	}

	// --- Assert A: submit() via m.ch reaches the mux-wired inbox ---
	cmd := m.ch.submit("ping")
	msg := cmd() // run synchronously
	if _, ok := msg.(promptSentMsg); !ok {
		t.Fatalf("submit() returned %T, want promptSentMsg", msg)
	}
	select {
	case im := <-inbox:
		if im.Content.TextOnly() != "ping" {
			t.Errorf("inbox received content %q, want %q", im.Content.TextOnly(), "ping")
		}
	case <-time.After(time.Second):
		t.Fatal("submit() did not deliver message to agent inbox — orphan channel bug present")
	}

	// --- Assert B: agent Send() lands on m.ch.out (the channel the model reads) ---
	if err := tuiCh.Send(ctx, channel.OutgoingMessage{Text: "pong"}); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	select {
	case raw := <-m.ch.out:
		reply, ok := raw.(agentReplyMsg)
		if !ok {
			t.Fatalf("m.ch.out received %T, want agentReplyMsg", raw)
		}
		if reply.text != "pong" {
			t.Errorf("agentReplyMsg.text = %q, want %q", reply.text, "pong")
		}
	case <-time.After(time.Second):
		t.Fatal("agent Send() did not reach m.ch.out — model is reading a different channel")
	}
}

// TestTUIChannel_Submit_ShutdownGuard verifies that submit() does not block
// when the TUIChannel's context is cancelled (shutdown path), even when the
// inbox is full.
//
// Without the select/ctx.Done guard, the blocking send c.inbox <- im would
// hang indefinitely if the inbox is full during shutdown.
func TestTUIChannel_Submit_ShutdownGuard(t *testing.T) {
	// Build the channel with a cancellable context.
	ctx, cancel := context.WithCancel(context.Background())

	tuiCh := NewTUIChannel()
	tuiCh.ctx = ctx

	// Wire an unbuffered inbox so the blocking path would immediately stall if
	// nothing is reading from it and no select guard is in place.
	inbox := make(chan channel.IncomingMessage) // unbuffered — will block if not selected away
	if err := tuiCh.Start(ctx, inbox); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Cancel the context to simulate shutdown BEFORE the send.
	cancel()

	done := make(chan tea.Msg, 1)
	go func() {
		cmd := tuiCh.submit("after cancel")
		done <- cmd()
	}()

	select {
	case msg := <-done:
		// The send should be dropped cleanly; promptSentMsg is still returned.
		if _, ok := msg.(promptSentMsg); !ok {
			t.Errorf("submit() after cancel returned %T, want promptSentMsg", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("submit() blocked for 2s after context cancel — shutdown guard missing")
	}
}
