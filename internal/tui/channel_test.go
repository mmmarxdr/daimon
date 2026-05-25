package tui

import (
	"context"
	"testing"
	"time"

	"daimon/internal/channel"
)

// Compile-time assertion: TUIChannel must satisfy channel.Channel.
var _ channel.Channel = (*TUIChannel)(nil)

// TestTUIChannel_ImplementsChannelInterface is the runtime companion —
// ensures the interface check above surfaces in the test binary.
func TestTUIChannel_ImplementsChannelInterface(t *testing.T) {
	ch := &TUIChannel{out: make(chan interface{}, 64)}
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
	ch := &TUIChannel{out: make(chan interface{}, 64)}
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
	ch := &TUIChannel{out: make(chan interface{}, 64)}
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
	ch := &TUIChannel{out: make(chan interface{}, 64)}
	if err := ch.Stop(); err != nil {
		t.Errorf("Stop() = %v, want nil", err)
	}
}
