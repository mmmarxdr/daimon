package agent

import (
	"testing"
	"time"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/notify"
	"daimon/internal/skill"
	"daimon/internal/tool"
)

// Ensure tool is used.
var _ tool.Tool = (*mockTool)(nil)

// ---------------------------------------------------------------------------
// Mock helpers re-used from loop_test.go (same package)
// ---------------------------------------------------------------------------

// newTestAgent builds a minimal Agent with the given execSkills.
func newTestAgent(t *testing.T, execSkills []skill.ExecutableSkillDef) *Agent {
	t.Helper()
	cfg := config.AgentConfig{MaxIterations: 1}
	prov := &mockProvider{}
	ch := &mockChannel{}
	st := &mockStore{}
	tools := map[string]tool.Tool{
		"existing_tool": &mockTool{name: "existing_tool"},
	}
	bus := notify.NewEventBus(256, 0, 0)
	t.Cleanup(func() { bus.Close() })

	a := New(cfg, defaultLimits(), config.FilterConfig{}, ch, prov, st,
		audit.NoopAuditor{}, tools, nil, skill.SkillIndex{}, 4, false)
	// WithBus must come before WithExecutableSkills so that NewSubagentManager
	// receives a non-nil bus (matches production order in cmd/daimon).
	a.WithBus(bus)
	if len(execSkills) > 0 {
		a.WithExecutableSkills(execSkills)
	}
	return a
}

// ---------------------------------------------------------------------------
// 2.14 — agent.New() wiring tests
// ---------------------------------------------------------------------------

func TestNew_ExecSkills_ProducesSpawnTools(t *testing.T) {
	defs := []skill.ExecutableSkillDef{
		{
			Name:        "researcher",
			Description: "Research a topic",
			Budget:      skill.BudgetConfig{MaxCostUSD: 0.5, MaxTurns: 20, Timeout: 10 * time.Minute},
		},
		{
			Name:        "summarizer",
			Description: "Summarize content",
			Budget:      skill.BudgetConfig{MaxCostUSD: 0.1, MaxTurns: 5, Timeout: 2 * time.Minute},
		},
	}

	a := newTestAgent(t, defs)

	a.toolsMu.RLock()
	researcherTool, hasResearcher := a.tools["researcher"]
	summarizerTool, hasSummarizer := a.tools["summarizer"]
	a.toolsMu.RUnlock()

	if !hasResearcher {
		t.Error("expected a.tools['researcher'] to exist")
	}
	if !hasSummarizer {
		t.Error("expected a.tools['summarizer'] to exist")
	}

	// Both must be SubagentSpawnTool instances.
	if _, ok := researcherTool.(*SubagentSpawnTool); !ok {
		t.Errorf("a.tools['researcher'] is %T, want *SubagentSpawnTool", researcherTool)
	}
	if _, ok := summarizerTool.(*SubagentSpawnTool); !ok {
		t.Errorf("a.tools['summarizer'] is %T, want *SubagentSpawnTool", summarizerTool)
	}
}

func TestNew_ExecSkills_SubMgr_NonNil(t *testing.T) {
	defs := []skill.ExecutableSkillDef{
		{
			Name:        "researcher",
			Description: "Research",
			Budget:      skill.BudgetConfig{MaxCostUSD: 0.5, MaxTurns: 20, Timeout: 10 * time.Minute},
		},
	}
	a := newTestAgent(t, defs)
	if a.subMgr == nil {
		t.Error("expected a.subMgr to be non-nil when execSkills are provided")
	}
}

func TestNew_NoExecSkills_NoSpawnTools_SubMgrNil(t *testing.T) {
	a := newTestAgent(t, nil)

	// No spawn tools should be added.
	a.toolsMu.RLock()
	_, hasResearcher := a.tools["researcher"]
	a.toolsMu.RUnlock()

	if hasResearcher {
		t.Error("expected no spawn tools when execSkills is nil")
	}
	if a.subMgr != nil {
		t.Error("expected a.subMgr to be nil when no execSkills")
	}
}

func TestNew_ExecSkills_ExistingToolPreserved(t *testing.T) {
	defs := []skill.ExecutableSkillDef{
		{
			Name:        "researcher",
			Description: "Research",
			Budget:      skill.BudgetConfig{MaxCostUSD: 0.5, MaxTurns: 20, Timeout: 10 * time.Minute},
		},
	}
	a := newTestAgent(t, defs)

	// The "existing_tool" that was passed in tools must still be present.
	a.toolsMu.RLock()
	_, exists := a.tools["existing_tool"]
	a.toolsMu.RUnlock()

	if !exists {
		t.Error("existing_tool was removed after execSkills wiring")
	}
}

func TestNew_ExecSkills_UnknownAllowlist_DropWithWarn(t *testing.T) {
	// tools_allowlist contains "nonexistent_tool" which is not in a.tools.
	// Per design §2.5.4: unknown names are dropped with a warning (non-fatal).
	defs := []skill.ExecutableSkillDef{
		{
			Name:           "researcher",
			Description:    "Research",
			Budget:         skill.BudgetConfig{MaxCostUSD: 0.5, MaxTurns: 20, Timeout: 10 * time.Minute},
			ToolsAllowlist: []string{"existing_tool", "nonexistent_tool"},
		},
	}

	// This must NOT panic or return an error — just warn.
	a := newTestAgent(t, defs)

	// The spawn tool should still be registered.
	a.toolsMu.RLock()
	_, exists := a.tools["researcher"]
	a.toolsMu.RUnlock()
	if !exists {
		t.Error("expected researcher spawn tool to be registered even with unknown allowlist entry")
	}
}

// ---------------------------------------------------------------------------
// 2.15 — Principal sem unaffected while subagent goroutine runs
// ---------------------------------------------------------------------------

func TestNew_ExecSkills_SemCapUnchanged(t *testing.T) {
	defs := []skill.ExecutableSkillDef{
		{
			Name:        "researcher",
			Description: "Research",
			Budget:      skill.BudgetConfig{MaxCostUSD: 0.5, MaxTurns: 20, Timeout: 10 * time.Minute},
		},
	}

	cfg := config.AgentConfig{MaxIterations: 1}
	prov := &mockProvider{}
	ch := &mockChannel{}
	st := &mockStore{}
	const maxConcurrent = 4

	a := New(cfg, defaultLimits(), config.FilterConfig{}, ch, prov, st,
		audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, maxConcurrent, false)
	a.WithExecutableSkills(defs)

	// The semaphore cap must equal maxConcurrent — not inflated by spawn goroutines.
	if got := cap(a.sem); got != maxConcurrent {
		t.Errorf("sem cap = %d, want %d (spawn goroutines must not inflate the parent sem)", got, maxConcurrent)
	}
}

// ---------------------------------------------------------------------------
// SubagentManager accessor
// ---------------------------------------------------------------------------

func TestAgent_SubagentManager_Accessor(t *testing.T) {
	defs := []skill.ExecutableSkillDef{
		{
			Name:        "researcher",
			Description: "Research",
			Budget:      skill.BudgetConfig{MaxCostUSD: 0.5, MaxTurns: 20, Timeout: 10 * time.Minute},
		},
	}
	a := newTestAgent(t, defs)

	if got := a.SubagentManager(); got == nil {
		t.Error("SubagentManager() returned nil, want non-nil")
	}
	if got := a.SubagentManager(); got != a.subMgr {
		t.Error("SubagentManager() does not return the agent's subMgr")
	}
}

func TestAgent_SubagentManager_NilWhenNoSkills(t *testing.T) {
	a := newTestAgent(t, nil)
	if got := a.SubagentManager(); got != nil {
		t.Errorf("SubagentManager() = %v, want nil", got)
	}
}
