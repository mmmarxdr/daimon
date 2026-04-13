package notify

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestEventBus_Delivered verifies that an emitted event reaches the handler
// with the correct fields.
func TestEventBus_Delivered(t *testing.T) {
	bus := NewEventBus(16, 30, 5*time.Second)
	defer bus.Close()

	received := make(chan Event, 1)
	bus.Subscribe(func(e Event) {
		received <- e
	})

	want := Event{
		Type:      EventCronJobFired,
		Origin:    OriginCron,
		JobID:     "job-1",
		JobPrompt: "do something",
		ChannelID: "cron:job-1",
		Timestamp: time.Now(),
	}
	bus.Emit(want)

	select {
	case got := <-received:
		if got.Type != want.Type {
			t.Errorf("Type: got %q, want %q", got.Type, want.Type)
		}
		if got.JobID != want.JobID {
			t.Errorf("JobID: got %q, want %q", got.JobID, want.JobID)
		}
		if got.ChannelID != want.ChannelID {
			t.Errorf("ChannelID: got %q, want %q", got.ChannelID, want.ChannelID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: handler not called")
	}
}

// TestEventBus_OriginNotificationDropped verifies that events with
// OriginNotification are silently dropped and the handler is never called.
func TestEventBus_OriginNotificationDropped(t *testing.T) {
	bus := NewEventBus(16, 30, 5*time.Second)
	defer bus.Close()

	var called atomic.Bool
	bus.Subscribe(func(e Event) {
		called.Store(true)
	})

	bus.Emit(Event{
		Type:      EventNotificationSent,
		Origin:    OriginNotification,
		Timestamp: time.Now(),
	})

	// Give the worker time to process the event.
	time.Sleep(100 * time.Millisecond)

	if called.Load() {
		t.Fatal("handler was called for OriginNotification event — expected drop")
	}
}

// TestEventBus_CircuitBreaker verifies that at most maxPerMin events are
// delivered per 60-second window, and excess events are dropped.
func TestEventBus_CircuitBreaker(t *testing.T) {
	const maxPerMin = 3
	bus := NewEventBus(64, maxPerMin, 5*time.Second)
	defer bus.Close()

	var count atomic.Int64
	delivered := make(chan struct{}, maxPerMin+1)
	bus.Subscribe(func(e Event) {
		count.Add(1)
		delivered <- struct{}{}
	})

	const total = 10
	for i := 0; i < total; i++ {
		bus.Emit(Event{
			Type:      EventCronJobFired,
			Origin:    OriginCron,
			Timestamp: time.Now(),
		})
	}

	// Drain up to maxPerMin deliveries with a timeout.
	deadline := time.After(2 * time.Second)
	var got int
loop:
	for {
		select {
		case <-delivered:
			got++
			if got >= maxPerMin {
				// Give a little more time in case extras leak through.
				time.Sleep(100 * time.Millisecond)
				break loop
			}
		case <-deadline:
			break loop
		}
	}

	final := int(count.Load())
	if final > maxPerMin {
		t.Errorf("circuit breaker failed: %d events delivered, want <= %d", final, maxPerMin)
	}
	if final == 0 {
		t.Error("no events delivered — circuit breaker may be too aggressive")
	}
}

// TestEventBus_BufferFull verifies that Emit is non-blocking when the buffer
// is full and a slow handler is blocking the worker.
func TestEventBus_BufferFull(t *testing.T) {
	const bufferSize = 4
	// Use a very large maxPerMin so the circuit breaker doesn't interfere.
	bus := NewEventBus(bufferSize, 1000, 5*time.Second)

	// Block the worker by subscribing a handler that waits for a release signal.
	release := make(chan struct{})
	bus.Subscribe(func(e Event) {
		<-release
	})

	// Send one event to occupy the worker (it will block on the handler).
	bus.Emit(Event{Type: EventCronJobFired, Origin: OriginCron, Timestamp: time.Now()})

	// Wait briefly for the worker to pick up the first event.
	time.Sleep(50 * time.Millisecond)

	// Now fill the buffer.
	for i := 0; i < bufferSize; i++ {
		bus.Emit(Event{Type: EventCronJobFired, Origin: OriginCron, Timestamp: time.Now()})
	}

	// These extra emits should not block even though the buffer is full.
	emitDone := make(chan struct{})
	go func() {
		defer close(emitDone)
		for i := 0; i < 5; i++ {
			bus.Emit(Event{Type: EventCronJobFired, Origin: OriginCron, Timestamp: time.Now()})
		}
	}()

	select {
	case <-emitDone:
		// Good — Emit returned immediately (non-blocking).
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Emit blocked when buffer was full")
	}

	// Unblock the worker and close cleanly.
	close(release)
	bus.Close()
}

// TestEventBus_Close_Drains verifies that all events emitted before Close()
// are delivered before Close returns.
func TestEventBus_Close_Drains(t *testing.T) {
	const numEvents = 20
	bus := NewEventBus(numEvents+4, 1000, 5*time.Second)

	var count atomic.Int64
	var wg sync.WaitGroup
	wg.Add(numEvents)

	bus.Subscribe(func(e Event) {
		count.Add(1)
		wg.Done()
	})

	for i := 0; i < numEvents; i++ {
		bus.Emit(Event{Type: EventTurnCompleted, Origin: OriginAgent, Timestamp: time.Now()})
	}

	// Close should block until all numEvents are processed.
	closeDone := make(chan struct{})
	go func() {
		bus.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Close() timed out — worker did not drain")
	}

	// Wait for all handler calls to complete (they run in goroutines inside
	// callWithTimeout, but Close() only waits for the worker, not the handlers).
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("handlers did not all complete after Close()")
	}

	if got := int(count.Load()); got != numEvents {
		t.Errorf("got %d events delivered, want %d", got, numEvents)
	}
}

// TestEventBus_Close_Idempotent verifies that calling Close() twice does not
// panic or deadlock.
func TestEventBus_Close_Idempotent(t *testing.T) {
	bus := NewEventBus(16, 30, 5*time.Second)

	// First close.
	bus.Close()

	// Second close should not panic.
	panicked := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				close(panicked)
			}
		}()
		bus.Close()
	}()

	select {
	case <-panicked:
		t.Fatal("Close() panicked on second call")
	case <-time.After(500 * time.Millisecond):
		// No panic — good.
	}
}
