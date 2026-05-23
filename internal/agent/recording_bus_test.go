package agent

import (
	"sync"

	"daimon/internal/notify"
)

// recordingBus implements notify.Bus for tests. It records every event emitted
// and (optionally) calls a per-test inspector hook. Thread-safe.
//
// Note: the existing busRecorder in subagent_manager_test.go wraps a real
// *EventBus and forwards Subscribe calls. This helper is intentionally different:
// it is a pure mock that does NOT invoke subscribers, so tests can assert on
// .events directly without flaky goroutine timing (design D8).
type recordingBus struct {
	mu     sync.Mutex
	events []notify.Event
	onEmit func(notify.Event) // optional hook for early assertions
}

func (b *recordingBus) Emit(ev notify.Event) {
	b.mu.Lock()
	b.events = append(b.events, ev)
	hook := b.onEmit
	b.mu.Unlock()
	if hook != nil {
		hook(ev)
	}
}

func (b *recordingBus) Subscribe(_ func(notify.Event)) {}
func (b *recordingBus) Close()                         {}

// snapshot returns a copy of all recorded events (safe for concurrent use).
func (b *recordingBus) snapshot() []notify.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]notify.Event, len(b.events))
	copy(out, b.events)
	return out
}

// filterByType returns all recorded events with the given type.
func (b *recordingBus) filterByType(evType string) []notify.Event {
	out := []notify.Event{}
	for _, ev := range b.snapshot() {
		if ev.Type == evType {
			out = append(out, ev)
		}
	}
	return out
}
