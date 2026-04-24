package store

import "time"

// Clock abstracts time.Now for tests that need deterministic time control.
// Used by ConversationPruner (and extensible to any other timer-driven
// subsystem in this package). Real code passes SystemClock{}; tests pass a
// FakeClock that can be advanced manually.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production implementation backed by time.Now().UTC().
type SystemClock struct{}

// Now returns the current UTC time.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// FakeClock is a test Clock that returns a fixed time. Use Advance to move
// it forward. Not safe for concurrent use — tests that exercise the pruner
// own a single fake clock and drive it serially.
type FakeClock struct {
	T time.Time
}

// Now returns the fake clock's current time.
func (f *FakeClock) Now() time.Time { return f.T }

// Advance moves the fake clock forward by d.
func (f *FakeClock) Advance(d time.Duration) { f.T = f.T.Add(d) }
