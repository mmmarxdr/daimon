package channel

import (
	"context"
	"testing"
	"time"
)

// TestSubagentChannel_StartSetsInbox verifies that after Start the channel
// can receive messages via Deliver.
func TestSubagentChannel_StartSetsInbox(t *testing.T) {
	ch := NewSubagentChannel("test-id-1")
	inbox := make(chan IncomingMessage, 4)

	if err := ch.Start(context.Background(), inbox); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := ch.Deliver("hello"); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	select {
	case msg := <-inbox:
		if msg.Content.TextOnly() != "hello" {
			t.Errorf("expected content 'hello', got %q", msg.Content.TextOnly())
		}
		if msg.ChannelID != ch.ID() {
			t.Errorf("expected ChannelID=%q, got %q", ch.ID(), msg.ChannelID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

// TestSubagentChannel_DeliverPushesCorrectChannelID verifies Deliver sets ChannelID to ch.ID().
func TestSubagentChannel_DeliverPushesCorrectChannelID(t *testing.T) {
	ch := NewSubagentChannel("abc-123")
	inbox := make(chan IncomingMessage, 4)
	if err := ch.Start(context.Background(), inbox); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := ch.Deliver("task prompt"); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	msg := <-inbox
	if msg.ChannelID != ch.ID() {
		t.Errorf("ChannelID: got %q, want %q", msg.ChannelID, ch.ID())
	}
}

// TestSubagentChannel_DeliverBeforeStartReturnsError verifies Deliver before
// Start returns an error.
func TestSubagentChannel_DeliverBeforeStartReturnsError(t *testing.T) {
	ch := NewSubagentChannel("no-start")
	if err := ch.Deliver("should fail"); err == nil {
		t.Fatal("expected error on Deliver before Start, got nil")
	}
}

// TestSubagentChannel_SendAppendsAndTracksFinalText verifies Send appends
// messages to internal output and records the last non-empty text.
func TestSubagentChannel_SendAppendsAndTracksFinalText(t *testing.T) {
	ch := NewSubagentChannel("send-test")
	inbox := make(chan IncomingMessage, 4)
	if err := ch.Start(context.Background(), inbox); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx := context.Background()
	if err := ch.Send(ctx, OutgoingMessage{Text: "first answer"}); err != nil {
		t.Fatalf("Send 1: %v", err)
	}
	if err := ch.Send(ctx, OutgoingMessage{Text: "final answer"}); err != nil {
		t.Fatalf("Send 2: %v", err)
	}

	if got := ch.FinalAssistant(); got != "final answer" {
		t.Errorf("FinalAssistant: got %q, want %q", got, "final answer")
	}
	if outs := ch.Outputs(); len(outs) != 2 {
		t.Errorf("Outputs: got %d messages, want 2", len(outs))
	}
}

// TestSubagentChannel_StopIsIdempotent verifies that calling Stop twice does not panic or error.
func TestSubagentChannel_StopIsIdempotent(t *testing.T) {
	ch := NewSubagentChannel("stop-test")
	inbox := make(chan IncomingMessage, 4)
	if err := ch.Start(context.Background(), inbox); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := ch.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := ch.Stop(); err != nil {
		t.Fatalf("second Stop (idempotent): %v", err)
	}
}

// TestSubagentChannel_OutputsReturnsDefensiveCopy verifies that mutating the
// returned slice does not affect internal state.
func TestSubagentChannel_OutputsReturnsDefensiveCopy(t *testing.T) {
	ch := NewSubagentChannel("copy-test")
	inbox := make(chan IncomingMessage, 4)
	if err := ch.Start(context.Background(), inbox); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx := context.Background()
	if err := ch.Send(ctx, OutgoingMessage{Text: "msg1"}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	out1 := ch.Outputs()
	if len(out1) != 1 {
		t.Fatalf("expected 1 output, got %d", len(out1))
	}

	// Mutate the returned slice.
	out1[0] = OutgoingMessage{Text: "mutated"}

	// Internal state must be unchanged.
	out2 := ch.Outputs()
	if out2[0].Text != "msg1" {
		t.Errorf("Outputs defensive copy failed: internal state mutated to %q", out2[0].Text)
	}
}

// TestSubagentChannel_CompileTimeAssertion verifies the compile-time assertion
// that *SubagentChannel implements Channel.
func TestSubagentChannel_CompileTimeAssertion(t *testing.T) {
	// If this test compiles, the assertion passes.
	// The actual compile-time var is in subagent.go.
	var _ Channel = (*SubagentChannel)(nil)
}

// TestSubagentChannel_Name verifies Name() returns "subagent".
func TestSubagentChannel_Name(t *testing.T) {
	ch := NewSubagentChannel("name-test")
	if ch.Name() != "subagent" {
		t.Errorf("Name: got %q, want %q", ch.Name(), "subagent")
	}
}

// TestSubagentChannel_IDHasPrefix verifies ID() returns a value with "sub:" prefix.
func TestSubagentChannel_IDHasPrefix(t *testing.T) {
	ch := NewSubagentChannel("my-uuid")
	id := ch.ID()
	if id != "sub:my-uuid" {
		t.Errorf("ID: got %q, want %q", id, "sub:my-uuid")
	}
}
