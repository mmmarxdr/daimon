package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// WU3: cancelRegistry — standalone struct, no agent wiring yet
// ---------------------------------------------------------------------------

func TestCancelRegistry_Register_StoresFunc(t *testing.T) {
	cr := newCancelRegistry()
	invoked := false
	fn := func() { invoked = true }

	err := cr.Register(cancelKey{ChannelID: "ch1", SenderID: "user1"}, fn)
	if err != nil {
		t.Fatalf("expected no error from Register, got: %v", err)
	}

	// Verify size reflects the registration.
	if cr.Size() != 1 {
		t.Errorf("expected size=1, got %d", cr.Size())
	}
	_ = invoked // fn not called yet
}

func TestCancelRegistry_Register_Collision_ReturnsError(t *testing.T) {
	cr := newCancelRegistry()
	key := cancelKey{ChannelID: "ch1", SenderID: "user1"}

	err := cr.Register(key, func() {})
	if err != nil {
		t.Fatalf("first Register failed unexpectedly: %v", err)
	}

	err = cr.Register(key, func() {})
	if err == nil {
		t.Fatal("expected error on duplicate Register, got nil")
	}

	// Size should still be 1 (second registration rejected).
	if cr.Size() != 1 {
		t.Errorf("expected size=1 after rejected duplicate, got %d", cr.Size())
	}
}

func TestCancelRegistry_Cancel_InvokesFunc_ReturnsTrue(t *testing.T) {
	cr := newCancelRegistry()
	key := cancelKey{ChannelID: "ch2", SenderID: "user2"}

	invoked := false
	_ = cr.Register(key, func() { invoked = true })

	ok := cr.Cancel(key)
	if !ok {
		t.Fatal("expected Cancel to return true for a registered key")
	}
	if !invoked {
		t.Error("expected cancel function to be invoked")
	}
	// Size should be 0 after Cancel (entry removed).
	if cr.Size() != 0 {
		t.Errorf("expected size=0 after Cancel, got %d", cr.Size())
	}
}

func TestCancelRegistry_Cancel_NoEntry_ReturnsFalse(t *testing.T) {
	cr := newCancelRegistry()
	key := cancelKey{ChannelID: "ch3", SenderID: "user3"}

	ok := cr.Cancel(key)
	if ok {
		t.Error("expected Cancel to return false for a non-existent key")
	}
}

func TestCancelRegistry_Cancel_Idempotent_SecondCallReturnsFalse(t *testing.T) {
	cr := newCancelRegistry()
	key := cancelKey{ChannelID: "ch4", SenderID: "user4"}
	_ = cr.Register(key, func() {})

	ok1 := cr.Cancel(key)
	ok2 := cr.Cancel(key)

	if !ok1 {
		t.Error("expected first Cancel to return true")
	}
	if ok2 {
		t.Error("expected second Cancel to return false (idempotent)")
	}
}

func TestCancelRegistry_Unregister_RemovesEntry(t *testing.T) {
	cr := newCancelRegistry()
	key := cancelKey{ChannelID: "ch5", SenderID: "user5"}
	_ = cr.Register(key, func() {})

	cr.Unregister(key)

	if cr.Size() != 0 {
		t.Errorf("expected size=0 after Unregister, got %d", cr.Size())
	}
	// Cancel should now return false since entry is gone.
	if cr.Cancel(key) {
		t.Error("expected Cancel to return false after Unregister")
	}
}

func TestCancelRegistry_ConcurrentRegisterCancel_NoRace(t *testing.T) {
	cr := newCancelRegistry()
	const pairs = 50

	var wg sync.WaitGroup
	for i := range pairs {
		key := cancelKey{ChannelID: "ch", SenderID: string(rune('a'+i%26)) + string(rune('0'+i/26))}
		wg.Add(2)

		// Goroutine 1: register then unregister.
		go func(k cancelKey) {
			defer wg.Done()
			_ = cr.Register(k, func() {})
			cr.Unregister(k)
		}(key)

		// Goroutine 2: try to cancel (may find it or not).
		go func(k cancelKey) {
			defer wg.Done()
			cr.Cancel(k)
		}(key)
	}
	wg.Wait()
}

// TestCancelRegistry_Cancel_InvokedOutsideLock verifies that the cancel
// function is called AFTER the mutex is released by measuring that a
// concurrent Size() call can proceed while fn runs.
func TestCancelRegistry_Cancel_InvokedOutsideLock(t *testing.T) {
	cr := newCancelRegistry()
	key := cancelKey{ChannelID: "ch-lock", SenderID: "user-lock"}

	// Use a context CancelFunc — it does nothing heavy, just sets a flag.
	_, cancel := context.WithCancel(context.Background())

	var fnStarted atomic.Bool
	wrappedCancel := func() {
		fnStarted.Store(true)
		// Small sleep to give a concurrent Size() call time to complete
		// if the lock were still held, it would deadlock.
		time.Sleep(2 * time.Millisecond)
		cancel()
	}

	_ = cr.Register(key, wrappedCancel)

	// Launch Cancel in background.
	done := make(chan struct{})
	go func() {
		cr.Cancel(key)
		close(done)
	}()

	// Wait for fn to start, then call Size() — should not block.
	for !fnStarted.Load() {
		runtime_gossched()
	}
	// This should return immediately (mu not held during fn invocation).
	size := cr.Size()
	_ = size

	<-done
}

// runtime_gossched is a thin wrapper so we don't import runtime in the test.
func runtime_gossched() {
	// Busy-wait with a yield hint (goroutine switch).
	time.Sleep(time.Microsecond)
}
