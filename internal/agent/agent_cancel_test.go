package agent

import (
	"context"
	"testing"

	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// Task 1.10 — Agent.CancelSubagent nil-safe delegate (REQ-18)
// ---------------------------------------------------------------------------

// TestAgentCancelSubagent_NilSubMgr verifies that CancelSubagent returns nil
// when the agent has no SubagentManager (no executable skills loaded). (REQ-18)
func TestAgentCancelSubagent_NilSubMgr(t *testing.T) {
	a := &Agent{} // no subMgr set

	err := a.CancelSubagent("any-id")
	if err != nil {
		t.Errorf("CancelSubagent with nil subMgr returned error %v, want nil", err)
	}
}

// TestAgentCancelSubagent_DelegatesToManager verifies that CancelSubagent delegates
// to subMgr.Cancel when a SubagentManager is present. (REQ-18)
func TestAgentCancelSubagent_DelegatesToManager(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	// Spawn a real subagent so we have a valid ID to cancel.
	def := skill.ExecutableSkillDef{
		Name:           "cancel-test",
		Description:    "test",
		Budget:         defaultBudget(),
		ToolsAllowlist: []string{},
	}
	handle, err := m.Spawn(context.Background(), def, "test prompt", SpawnModeAsync, "conv-parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	a := &Agent{subMgr: m}

	// Cancel should succeed (no error).
	err = a.CancelSubagent(handle.ID)
	if err != nil {
		t.Errorf("CancelSubagent(%q) returned error %v, want nil", handle.ID, err)
	}
}

// TestAgentCancelSubagent_UnknownID verifies that CancelSubagent returns an error
// when the ID is not registered. (REQ-18)
func TestAgentCancelSubagent_UnknownID(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	a := &Agent{subMgr: m}

	err := a.CancelSubagent("nonexistent-id")
	if err == nil {
		t.Error("CancelSubagent with unknown ID should return an error, got nil")
	}
}
