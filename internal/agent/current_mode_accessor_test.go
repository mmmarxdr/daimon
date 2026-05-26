package agent

// current_mode_accessor_test.go — STRICT TDD: tests written RED-first.
//
// Tests cover Agent.CurrentMode(), the public read-only accessor that exposes
// the active mode name (plan/build/review) to the TUI (PR5).

import (
	"testing"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/skill"
)

func buildAgentForCurrentMode(t *testing.T) *Agent {
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

// TC-1: Default agent (currentMode == "") must fall back to "build" (mirrors modeSnapshot).
func TestCurrentMode_DefaultIsBuild(t *testing.T) {
	a := buildAgentForCurrentMode(t)
	got := a.CurrentMode()
	if got != "build" {
		t.Errorf("CurrentMode() on new agent = %q, want \"build\"", got)
	}
}

// TC-2: Round-trip for each valid mode.
func TestCurrentMode_RoundTrip(t *testing.T) {
	for _, want := range []string{"plan", "build", "review"} {
		t.Run(want, func(t *testing.T) {
			a := buildAgentForCurrentMode(t)
			a.modeMu.Lock()
			a.currentMode = want
			a.modeMu.Unlock()

			got := a.CurrentMode()
			if got != want {
				t.Errorf("CurrentMode() = %q, want %q", got, want)
			}
		})
	}
}

// TC-3: Corrupt currentMode falls back to "build".
func TestCurrentMode_CorruptFallsBackToBuild(t *testing.T) {
	a := buildAgentForCurrentMode(t)
	a.modeMu.Lock()
	a.currentMode = "not-a-valid-mode"
	a.modeMu.Unlock()

	got := a.CurrentMode()
	if got != "build" {
		t.Errorf("CurrentMode() with corrupt mode = %q, want \"build\" fallback", got)
	}
}
