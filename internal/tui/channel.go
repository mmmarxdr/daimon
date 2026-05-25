package tui

// channel.go — TUIChannel implements channel.Channel (AD-4).
//
// TUIChannel bridges:
//   - User input → agent inbox (via submit tea.Cmd → IncomingMessage)
//   - Agent output → tea.Msg queue (via Send → agentReplyMsg on c.out)
//
// RULE: submit() is called ONLY from a tea.Cmd closure, NEVER inline in Update.
// No IO occurs in Update; all blocking operations happen in Cmd closures.

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"

	"daimon/internal/channel"
	"daimon/internal/content"
)

// agentReplyMsg carries a completed agent reply text into the TUI event loop.
// Emitted by the pumpEvents Cmd (PR2) after TUIChannel.Send enqueues it.
type agentReplyMsg struct {
	text string
}

// promptSentMsg is returned by the submit() tea.Cmd to signal that the
// IncomingMessage was successfully enqueued into the agent's inbox.
// The model uses this to clear the input bar and show a waiting indicator.
type promptSentMsg struct{}

// TUIChannel implements channel.Channel for the embedded TUI.
// It bridges the input bar (user → agent inbox) and the agent's Send callback
// (agent reply → tea.Msg via the out channel).
//
// Concurrency contract:
//   - out is buffered (cap 64) so Send never blocks the agent goroutine.
//   - inbox is captured in Start; submit writes to it from a tea.Cmd closure
//     (which runs on its own goroutine, off the Update path).
type TUIChannel struct {
	inbox chan<- channel.IncomingMessage // captured in Start; nil until Start is called
	out   chan interface{}               // agent Send pushes agentReplyMsg here; tea.Cmd drains
}

// newTUIChannel constructs a TUIChannel ready for use (package-internal).
func newTUIChannel() *TUIChannel {
	return &TUIChannel{
		out: make(chan interface{}, 64),
	}
}

// NewTUIChannel constructs a TUIChannel for use by cmd/daimon/tui_cmd.go.
// It returns *TUIChannel which satisfies channel.Channel.
func NewTUIChannel() *TUIChannel {
	return newTUIChannel()
}

// Name implements channel.Channel. Returns "tui".
func (c *TUIChannel) Name() string { return "tui" }

// Start implements channel.Channel. Captures the agent's inbox channel so
// submit() can enqueue IncomingMessages. Non-blocking (no goroutines started).
func (c *TUIChannel) Start(_ context.Context, inbox chan<- channel.IncomingMessage) error {
	c.inbox = inbox
	return nil
}

// Send implements channel.Channel. Enqueues an agentReplyMsg onto the out
// channel so the TUI's pumpEvents Cmd can deliver it as a tea.Msg.
// Non-blocking under normal conditions (buffered cap 64); blocks only when
// the pump is stalled (should not happen at turn-level cadence).
func (c *TUIChannel) Send(ctx context.Context, msg channel.OutgoingMessage) error {
	select {
	case c.out <- agentReplyMsg{text: msg.Text}:
	case <-ctx.Done():
	}
	return nil
}

// Stop implements channel.Channel. No goroutines to stop — returns nil.
func (c *TUIChannel) Stop() error { return nil }

// submit returns a tea.Cmd that enqueues an IncomingMessage on the agent inbox
// and returns promptSentMsg to the TUI event loop.
//
// This MUST be called only as a tea.Cmd (i.e., returned from Update), never
// invoked inline. The blocking send to c.inbox runs on the Cmd's goroutine,
// keeping Update IO-free.
//
// Guard: if inbox is nil (Start has not been called), returns promptSentMsg
// immediately to avoid an indefinite goroutine block.
func (c *TUIChannel) submit(text string) tea.Cmd {
	return func() tea.Msg {
		if c.inbox == nil {
			// Agent not started yet; drop the message rather than block forever.
			return promptSentMsg{}
		}
		im := channel.IncomingMessage{
			ID:        uuid.New().String(),
			ChannelID: "tui",
			SenderID:  "local_user",
			Content:   content.TextBlock(text),
			Timestamp: time.Now(),
		}
		c.inbox <- im // blocking send; runs in Cmd goroutine (off Update path)
		return promptSentMsg{}
	}
}
