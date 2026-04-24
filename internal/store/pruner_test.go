package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Fakes ---

type fakePruneStore struct {
	mu          sync.Mutex
	cutoffs     []time.Time
	err         error
	deletedN    int32
	simulateHit int32 // how many rows to claim deleted on the next call
}

func (f *fakePruneStore) DeleteConversationsOlderThan(_ context.Context, cutoff time.Time) (int, error) {
	f.mu.Lock()
	f.cutoffs = append(f.cutoffs, cutoff)
	f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	n := int(atomic.LoadInt32(&f.simulateHit))
	atomic.StoreInt32(&f.deletedN, atomic.LoadInt32(&f.deletedN)+int32(n))
	return n, nil
}

func (f *fakePruneStore) DeleteToolOutputsBefore(_ context.Context, _ time.Time) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	return 0, nil
}

func (f *fakePruneStore) CutoffCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.cutoffs)
}

// --- E1. Tick behavior ---

func TestPruner_TickDeletesExpired(t *testing.T) {
	now := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)
	clock := &FakeClock{T: now}
	store := &fakePruneStore{}
	atomic.StoreInt32(&store.simulateHit, 3)

	p := NewConversationPruner(store, clock, PrunerConfig{
		Enabled:   true,
		Retention: 30 * 24 * time.Hour,
		Interval:  1 * time.Hour,
	})
	p.Tick(context.Background())

	if store.CutoffCount() != 1 {
		t.Fatalf("expected 1 Tick call, got %d", store.CutoffCount())
	}
	want := now.Add(-30 * 24 * time.Hour)
	got := store.cutoffs[0]
	if !got.Equal(want) {
		t.Errorf("cutoff: got %v, want %v", got, want)
	}
}

func TestPruner_TickWithFakeClockAdvance(t *testing.T) {
	clock := &FakeClock{T: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)}
	store := &fakePruneStore{}
	p := NewConversationPruner(store, clock, PrunerConfig{
		Enabled:   true,
		Retention: 7 * 24 * time.Hour,
		Interval:  1 * time.Hour,
	})

	// Advance the clock manually and run Tick deterministically.
	clock.Advance(8 * 24 * time.Hour)
	p.Tick(context.Background())

	if store.CutoffCount() != 1 {
		t.Fatalf("Tick count: %d", store.CutoffCount())
	}
	wantCutoff := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC).
		Add(8 * 24 * time.Hour).
		Add(-7 * 24 * time.Hour)
	if !store.cutoffs[0].Equal(wantCutoff) {
		t.Errorf("cutoff: got %v, want %v", store.cutoffs[0], wantCutoff)
	}
}

// --- E1.5. DB error does not kill goroutine ---

func TestPruner_DBErrorDoesNotKillGoroutine(t *testing.T) {
	store := &fakePruneStore{err: errors.New("database is locked")}
	clock := SystemClock{}
	p := NewConversationPruner(store, clock, PrunerConfig{
		Enabled: true, Retention: time.Hour, Interval: 20 * time.Millisecond,
	})
	p.Start(context.Background())
	defer p.Stop()

	// Wait long enough for at least 2 ticks with errors.
	time.Sleep(80 * time.Millisecond)
	if store.CutoffCount() < 2 {
		t.Errorf("expected ≥2 Tick calls despite errors, got %d", store.CutoffCount())
	}
}

// --- E2. Lifecycle ---

func TestPruner_StartStopExitsQuickly(t *testing.T) {
	store := &fakePruneStore{}
	p := NewConversationPruner(store, SystemClock{}, PrunerConfig{
		Enabled: true, Retention: time.Hour, Interval: 100 * time.Millisecond,
	})

	p.Start(context.Background())
	stopStart := time.Now()
	p.Stop()
	elapsed := time.Since(stopStart)
	if elapsed > 200*time.Millisecond {
		t.Errorf("Stop took %v, expected <200ms", elapsed)
	}
}

func TestPruner_DisabledDoesNotLaunchGoroutine(t *testing.T) {
	store := &fakePruneStore{}
	p := NewConversationPruner(store, SystemClock{}, PrunerConfig{
		Enabled: false, Retention: time.Hour, Interval: 20 * time.Millisecond,
	})
	p.Start(context.Background())
	time.Sleep(80 * time.Millisecond) // would have ticked multiple times if enabled

	if store.CutoffCount() != 0 {
		t.Errorf("disabled pruner ticked %d times", store.CutoffCount())
	}
	p.Stop() // must be safe even without Start launching anything
}

func TestPruner_StopIsIdempotent(t *testing.T) {
	p := NewConversationPruner(&fakePruneStore{}, SystemClock{}, PrunerConfig{
		Enabled: true, Retention: time.Hour, Interval: time.Hour,
	})
	p.Start(context.Background())
	p.Stop()
	p.Stop() // must not panic
}
