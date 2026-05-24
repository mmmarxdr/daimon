package agent

import (
	"context"
	"fmt"
	"sync"
)

// cancelKey identifies a (channel, sender) pair — the canonical scope for
// per-user-channel operations such as turn cancellation.
// Using a struct key avoids string concatenation races and is directly
// comparable as a Go map key.
type cancelKey struct {
	ChannelID string
	SenderID  string
}

// cancelRegistry maps (channelID, senderID) pairs to their active turn's
// context.CancelFunc. It is used by the /cancel built-in command (WU4) and
// wired into processMessage (WU4) — this file contains only the standalone
// data structure with no agent coupling.
//
// Thread-safety: all public methods are safe for concurrent use.
// The cancel function is invoked OUTSIDE the mutex to avoid holding mu
// across user-supplied cleanup code (callback-under-lock anti-pattern).
type cancelRegistry struct {
	mu      sync.Mutex
	cancels map[cancelKey]context.CancelFunc
}

// newCancelRegistry returns an initialized cancelRegistry.
func newCancelRegistry() *cancelRegistry {
	return &cancelRegistry{
		cancels: make(map[cancelKey]context.CancelFunc),
	}
}

// Register stores fn under key. Returns an error if a cancel func is already
// registered for key (indicates a concurrent-turn collision, which should not
// happen in normal operation — the caller logs a warning and drops the turn).
// Returns nil on success.
func (cr *cancelRegistry) Register(k cancelKey, fn context.CancelFunc) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if _, exists := cr.cancels[k]; exists {
		return fmt.Errorf("turn already in progress for channel=%s sender=%s", k.ChannelID, k.SenderID)
	}
	cr.cancels[k] = fn
	return nil
}

// Cancel looks up the cancel func for key, removes the entry, and invokes
// the func OUTSIDE the mutex. Returns true if a func was found and invoked,
// false if no entry existed (idempotent — calling Cancel on an already-
// cancelled or never-registered key is safe).
func (cr *cancelRegistry) Cancel(k cancelKey) bool {
	cr.mu.Lock()
	fn, ok := cr.cancels[k]
	if ok {
		delete(cr.cancels, k)
	}
	cr.mu.Unlock() // release BEFORE invoking fn — avoid callback-under-lock
	if !ok {
		return false
	}
	fn()
	return true
}

// Unregister removes the entry for key without invoking the cancel func.
// Used by processMessage's deferred cleanup to remove the entry after the
// turn completes naturally (the turn's defer also calls cancel(), which is
// idempotent via context.WithCancel semantics).
func (cr *cancelRegistry) Unregister(k cancelKey) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	delete(cr.cancels, k)
}

// Size returns the number of currently registered entries. Used in tests
// and for observability.
func (cr *cancelRegistry) Size() int {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	return len(cr.cancels)
}
