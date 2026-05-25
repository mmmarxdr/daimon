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
	"sync"
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
//     out is NEVER closed — it is left open and GC'd after the program exits.
//     Closing out while in-flight Send() goroutines are running causes a panic
//     (send-on-closed-channel); using a separate done channel avoids this.
//   - inbox is captured in Start; submit writes to it from a tea.Cmd closure
//     (which runs on its own goroutine, off the Update path).
//   - ctx is captured in Start; submit selects on ctx.Done() so it does not
//     block during shutdown even when inbox is full.
//   - FIX 1: Stop() closes the separate done channel exactly once (sync.Once
//     guard for idempotency). wireEvents goroutines select on done to exit.
//     Send() keeps its ctx.Done() branch — on shutdown ctx is cancelled so
//     Send takes the ctx.Done() branch rather than blocking on out.
type TUIChannel struct {
	inbox    chan<- channel.IncomingMessage // captured in Start; nil until Start is called
	out      chan interface{}               // agent Send pushes agentReplyMsg here; NEVER closed
	done     chan struct{}                  // closed by Stop() exactly once; signals goroutine exit
	ctx      context.Context                // captured in Start; guards submit send against shutdown
	stopOnce sync.Once                      // guards against double-close of done
}

// newTUIChannel constructs a TUIChannel ready for use (package-internal).
func newTUIChannel() *TUIChannel {
	return &TUIChannel{
		out:  make(chan interface{}, 64),
		done: make(chan struct{}),
	}
}

// NewTUIChannel constructs a TUIChannel for use by cmd/daimon/tui_cmd.go.
// It returns *TUIChannel which satisfies channel.Channel.
func NewTUIChannel() *TUIChannel {
	return newTUIChannel()
}

// Name implements channel.Channel. Returns "tui".
func (c *TUIChannel) Name() string { return "tui" }

// Start implements channel.Channel. Captures the agent's inbox channel and
// shutdown context so submit() can enqueue IncomingMessages and bail out
// during shutdown. Non-blocking (no goroutines started).
func (c *TUIChannel) Start(ctx context.Context, inbox chan<- channel.IncomingMessage) error {
	c.inbox = inbox
	c.ctx = ctx
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

// Stop implements channel.Channel. Closes the done channel exactly once,
// signalling wireEvents goroutines to exit. c.out is deliberately left open
// so that any in-flight Send() calls do not panic (send-on-closed-channel).
// c.out will be GC'd when the program exits. A sync.Once guard ensures
// idempotency.
func (c *TUIChannel) Stop() error {
	c.stopOnce.Do(func() {
		close(c.done)
	})
	return nil
}

// submit returns a tea.Cmd that enqueues an IncomingMessage on the agent inbox
// and returns promptSentMsg to the TUI event loop.
//
// This MUST be called only as a tea.Cmd (i.e., returned from Update), never
// invoked inline. The blocking send to c.inbox runs on the Cmd's goroutine,
// keeping Update IO-free.
//
// Guards:
//   - nil-inbox: if Start has not been called, returns promptSentMsg immediately.
//   - shutdown: if c.ctx is cancelled (e.g. the agent is shutting down), the
//     send is dropped via a select so the goroutine does not leak.
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
		// Select on ctx.Done() so a shutdown does not leak this goroutine.
		if c.ctx != nil {
			select {
			case c.inbox <- im:
			case <-c.ctx.Done():
			}
		} else {
			c.inbox <- im // fallback: no ctx captured (pre-Start path guarded above)
		}
		return promptSentMsg{}
	}
}
