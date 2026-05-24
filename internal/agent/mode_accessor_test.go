package agent

// mode_accessor_test.go — RED tests for Phase 2 of mode-system PR1.
//
// Tests cover:
//   - modeSnapshot() round-trip for each valid mode name (plan/build/review)
//   - Defensive fallback to "build" when currentMode is corrupt/unknown
//   - Race safety: 100 goroutines calling modeSnapshot() concurrently
//   - Race safety: concurrent modeSnapshot() + direct currentMode writes
//
// REQs covered: REQ-11, REQ-15.
// These tests are written BEFORE the implementation (TDD RED step).

import (
	"sync"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/skill"
)

// buildAgentForModeAccessor constructs a minimal Agent for mode accessor tests.
// Uses the same New() pattern as other accessor tests.
func buildAgentForModeAccessor(t *testing.T) *Agent {
	t.Helper()
	ch := &mockChannel{}
	st := &mockStore{}
	return New(
		config.AgentConfig{},
		defaultLimits(),
		config.FilterConfig{},
		ch,
		&mockProvider{},
		st,
		audit.NoopAuditor{},
		nil,
		nil,
		skill.SkillIndex{},
		4,
		false,
	)
}

// ---------------------------------------------------------------------------
// modeSnapshot round-trip tests
// ---------------------------------------------------------------------------

func TestModeSnapshot_DefaultIsEmpty_FallsBackToBuild(t *testing.T) {
	// New agent has currentMode == "" (zero value). modeSnapshot must fall back to build.
	a := buildAgentForModeAccessor(t)

	snap := a.modeSnapshot()
	if snap.Name != "build" {
		t.Errorf("modeSnapshot() on new agent = %q, want %q", snap.Name, "build")
	}
}

func TestModeSnapshot_RoundTrip_Plan(t *testing.T) {
	a := buildAgentForModeAccessor(t)

	// Directly write currentMode under modeMu to simulate SetMode having succeeded.
	a.modeMu.Lock()
	a.currentMode = "plan"
	a.modeMu.Unlock()

	snap := a.modeSnapshot()
	if snap.Name != "plan" {
		t.Errorf("modeSnapshot() = %q, want %q", snap.Name, "plan")
	}
	if len(snap.ToolAllowlist) == 0 {
		t.Error("plan mode snapshot should have non-empty ToolAllowlist")
	}
}

func TestModeSnapshot_RoundTrip_Build(t *testing.T) {
	a := buildAgentForModeAccessor(t)

	a.modeMu.Lock()
	a.currentMode = "build"
	a.modeMu.Unlock()

	snap := a.modeSnapshot()
	if snap.Name != "build" {
		t.Errorf("modeSnapshot() = %q, want %q", snap.Name, "build")
	}
	if snap.ToolAllowlist != nil {
		t.Errorf("build mode ToolAllowlist should be nil, got %v", snap.ToolAllowlist)
	}
}

func TestModeSnapshot_RoundTrip_Review(t *testing.T) {
	a := buildAgentForModeAccessor(t)

	a.modeMu.Lock()
	a.currentMode = "review"
	a.modeMu.Unlock()

	snap := a.modeSnapshot()
	if snap.Name != "review" {
		t.Errorf("modeSnapshot() = %q, want %q", snap.Name, "review")
	}
}

// ---------------------------------------------------------------------------
// Defensive fallback to build on corrupt currentMode
// ---------------------------------------------------------------------------

func TestModeSnapshot_CorruptCurrentMode_FallsBackToBuild(t *testing.T) {
	a := buildAgentForModeAccessor(t)

	// Inject an invalid/corrupt mode name directly (bypassing SetMode).
	a.modeMu.Lock()
	a.currentMode = "not-a-valid-mode-xyz"
	a.modeMu.Unlock()

	snap := a.modeSnapshot()
	if snap.Name != "build" {
		t.Errorf("modeSnapshot() with corrupt currentMode = %q, want fallback %q", snap.Name, "build")
	}
}

// ---------------------------------------------------------------------------
// Race safety tests (REQ-15)
// ---------------------------------------------------------------------------

func TestModeSnapshot_ConcurrentReads_NoRace(t *testing.T) {
	a := buildAgentForModeAccessor(t)
	a.modeMu.Lock()
	a.currentMode = "plan"
	a.modeMu.Unlock()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			_ = a.modeSnapshot()
		}()
	}
	wg.Wait()
}

func TestModeSnapshot_ConcurrentWritesAndReads_NoRace(t *testing.T) {
	// 50 goroutines writing currentMode under modeMu, 50 goroutines calling modeSnapshot().
	a := buildAgentForModeAccessor(t)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	modes := []string{"plan", "build", "review"}
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			name := modes[i%len(modes)]
			a.modeMu.Lock()
			a.currentMode = name
			a.modeMu.Unlock()
		}(i)
		go func() {
			defer wg.Done()
			_ = a.modeSnapshot()
		}()
	}
	wg.Wait()
	// No assertion: the race detector failure would be the signal.
}
