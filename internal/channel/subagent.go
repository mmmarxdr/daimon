package channel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"daimon/internal/content"
)

// SubagentChannel is a headless Channel implementation used by the
// SubagentManager to drive a child Agent. There is no network transport:
// outbound messages from the child Agent (via Send) are captured in memory so
// the manager can build a SubagentResult.Summary; inbound delivery is seeded
// once by SubagentManager.Spawn via Deliver.
type SubagentChannel struct {
	id    string // "sub:<spawn-uuid>"
	mu    sync.Mutex
	inbox chan<- IncomingMessage // set by Start; nil before Start is called

	output    []OutgoingMessage // all Send calls appended here
	finalText string            // last non-empty Send text (treated as the answer)
	closed    bool
}

// NewSubagentChannel creates a SubagentChannel with the given spawn-UUID.
// The channel ID is set to "sub:<id>" to namespace it from other channel types.
func NewSubagentChannel(id string) *SubagentChannel {
	return &SubagentChannel{id: "sub:" + id}
}

// Name returns the channel type identifier, always "subagent".
func (c *SubagentChannel) Name() string { return "subagent" }

// ID returns the unique channel identifier (e.g., "sub:<uuid>").
func (c *SubagentChannel) ID() string { return c.id }

// Start stores the inbox channel so Deliver can enqueue messages.
// Must be called before Deliver; safe to call from agent.Run.
func (c *SubagentChannel) Start(_ context.Context, inbox chan<- IncomingMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inbox = inbox
	return nil
}

// Deliver enqueues the spawn prompt as a single user IncomingMessage so the
// child agent's event loop fires. Called by SubagentManager exactly once per
// spawn. Returns an error if Start has not been called yet, or if the inbox
// blocks for more than one second (stuck child loop).
func (c *SubagentChannel) Deliver(prompt string) error {
	c.mu.Lock()
	inbox := c.inbox
	id := c.id
	c.mu.Unlock()

	if inbox == nil {
		return errors.New("subagent channel: not started")
	}

	msg := IncomingMessage{
		ID:        shortID(),
		ChannelID: id,
		SenderID:  "principal",
		Content:   content.Blocks{{Type: content.BlockText, Text: prompt}},
		Timestamp: time.Now(),
	}

	select {
	case inbox <- msg:
		return nil
	case <-time.After(time.Second):
		return errors.New("subagent inbox blocked")
	}
}

// Send captures the outgoing message from the child Agent. Post-Stop calls are
// silently dropped (the context is already cancelled; errors would be noise).
// Implements channel.Channel.
func (c *SubagentChannel) Send(_ context.Context, msg OutgoingMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil // drop silently after Stop
	}
	c.output = append(c.output, msg)
	if strings.TrimSpace(msg.Text) != "" {
		c.finalText = msg.Text
	}
	return nil
}

// Stop marks the channel as closed. Subsequent Send calls are no-ops.
// Idempotent — safe to call multiple times.
func (c *SubagentChannel) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

// FinalAssistant returns the text of the last non-empty Send call. This is
// the child Agent's final answer, used as SubagentResult.Summary.
func (c *SubagentChannel) FinalAssistant() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.finalText
}

// Outputs returns a defensive copy of all outgoing messages collected so far.
// Mutation of the returned slice does not affect internal state.
func (c *SubagentChannel) Outputs() []OutgoingMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]OutgoingMessage, len(c.output))
	copy(out, c.output)
	return out
}

// shortID returns a short pseudo-random string for message IDs.
// We use nanosecond timestamp converted to hex-ish string (no crypto needed here).
func shortID() string {
	ns := time.Now().UnixNano()
	// Simple 8-char hex-like string from the lower 32 bits of nanoseconds.
	const hex = "0123456789abcdef"
	b := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		b[i] = hex[ns&0xf]
		ns >>= 4
	}
	return string(b)
}

// Compile-time assertion: *SubagentChannel must implement Channel.
var _ Channel = (*SubagentChannel)(nil)
