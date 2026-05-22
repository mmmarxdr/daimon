package agent

import (
	"context"
	"encoding/json"
	"testing"

	"daimon/internal/skill"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fakeTool is a minimal tool.Tool implementation for hot_reload tests.
type fakeTool struct{ name string }

func (f *fakeTool) Name() string        { return f.name }
func (f *fakeTool) Description() string { return "fake tool" }
func (f *fakeTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (tool.ToolResult, error) {
	return tool.ToolResult{}, nil
}

// countSpawnTools counts *SubagentSpawnTool entries in the tool map.
func countSpawnTools(tools map[string]tool.Tool) int {
	n := 0
	for _, t := range tools {
		if _, ok := t.(*SubagentSpawnTool); ok {
			n++
		}
	}
	return n
}

// spawnToolNames returns the names of all *SubagentSpawnTool entries.
func spawnToolNames(tools map[string]tool.Tool) []string {
	var names []string
	for name, t := range tools {
		if _, ok := t.(*SubagentSpawnTool); ok {
			names = append(names, name)
		}
	}
	return names
}

// newAgentForHotReload creates a minimal *Agent suitable for hot_reload tests.
// It does NOT start an event loop — only the tool map and subMgr fields matter.
func newAgentForHotReload() *Agent {
	return &Agent{
		tools:        make(map[string]tool.Tool),
		mcpToolNames: map[string][]string{},
		mcpClients:   map[string]interface{ Close() error }{},
	}
}

// ---------------------------------------------------------------------------
// Task 3.1 — ReplaceExecutableSkills removes old *SubagentSpawnTool entries;
//             non-spawn tools are untouched. (REQ-19)
// ---------------------------------------------------------------------------

func TestReplaceExecutableSkills_RemovesOldSpawnToolsRegistersNew(t *testing.T) {
	a := newAgentForHotReload()

	// Pre-populate: one native tool + two spawn tools.
	a.tools["native-tool"] = &fakeTool{name: "native-tool"}
	a.tools["old-skill-a"] = &SubagentSpawnTool{def: skill.ExecutableSkillDef{Name: "old-skill-a"}}
	a.tools["old-skill-b"] = &SubagentSpawnTool{def: skill.ExecutableSkillDef{Name: "old-skill-b"}}

	// Replace with one new def.
	newDefs := []skill.ExecutableSkillDef{
		{Name: "new-skill", Description: "new", ToolsAllowlist: nil},
	}
	a.ReplaceExecutableSkills(newDefs)

	// Old spawn tools must be gone.
	if _, ok := a.tools["old-skill-a"]; ok {
		t.Error("old-skill-a should have been removed")
	}
	if _, ok := a.tools["old-skill-b"]; ok {
		t.Error("old-skill-b should have been removed")
	}

	// Native tool must be untouched.
	if _, ok := a.tools["native-tool"]; !ok {
		t.Error("native-tool should not be removed")
	}

	// New spawn tool must be registered.
	if _, ok := a.tools["new-skill"]; !ok {
		t.Error("new-skill should be registered")
	}
	if _, ok := a.tools["new-skill"].(*SubagentSpawnTool); !ok {
		t.Error("new-skill should be a *SubagentSpawnTool")
	}

	// Total spawn tools must be exactly 1.
	if n := countSpawnTools(a.tools); n != 1 {
		t.Errorf("expected 1 spawn tool, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// Task 3.2 — Lazy subMgr init: no prior subMgr + non-empty defs → initialized.
// (REQ-19)
// ---------------------------------------------------------------------------

func TestReplaceExecutableSkills_LazySubMgrInit(t *testing.T) {
	a := newAgentForHotReload()

	// Verify subMgr is nil before the call.
	if a.subMgr != nil {
		t.Fatal("precondition: subMgr should be nil")
	}

	defs := []skill.ExecutableSkillDef{
		{Name: "my-skill", Description: "desc", ToolsAllowlist: nil},
	}
	a.ReplaceExecutableSkills(defs)

	// subMgr must now be non-nil.
	if a.subMgr == nil {
		t.Error("subMgr should have been initialized on first non-empty ReplaceExecutableSkills call")
	}
}

// ---------------------------------------------------------------------------
// Task 3.3 — Empty defs → all spawn tools removed; subMgr NOT nilled out.
// (REQ-19)
// ---------------------------------------------------------------------------

func TestReplaceExecutableSkills_EmptyDefsRemovesSpawnTools(t *testing.T) {
	a := newAgentForHotReload()

	// Pre-install an existing subMgr and spawn tools.
	mgr := NewSubagentManager(nil, nil)
	a.subMgr = mgr
	a.tools["skill-x"] = &SubagentSpawnTool{def: skill.ExecutableSkillDef{Name: "skill-x"}}
	a.tools["native"] = &fakeTool{name: "native"}

	// Replace with empty defs.
	a.ReplaceExecutableSkills(nil)

	// All spawn tools must be gone.
	if n := countSpawnTools(a.tools); n != 0 {
		t.Errorf("expected 0 spawn tools, got %d: %v", n, spawnToolNames(a.tools))
	}

	// Native tool must survive.
	if _, ok := a.tools["native"]; !ok {
		t.Error("native tool should remain after empty ReplaceExecutableSkills")
	}

	// subMgr must NOT be nilled (in-flight spawns continue to drain).
	if a.subMgr == nil {
		t.Error("subMgr should not be nilled when ReplaceExecutableSkills is called with empty defs")
	}
	if a.subMgr != mgr {
		t.Error("subMgr instance should be unchanged when called with empty defs")
	}
}

// ---------------------------------------------------------------------------
// Task 3.4 — Unknown tool in tools_allowlist → dropped with slog.Warn, no error.
// (REQ-19; CONFIG-REQ-5 warn-not-block at hot-reload)
// ---------------------------------------------------------------------------

func TestReplaceExecutableSkills_UnknownAllowlistToolDropped(t *testing.T) {
	a := newAgentForHotReload()

	// Only "known-tool" exists in the tool map.
	a.tools["known-tool"] = &fakeTool{name: "known-tool"}

	defs := []skill.ExecutableSkillDef{
		{
			Name:           "skill-allowlist",
			Description:    "desc",
			ToolsAllowlist: []string{"known-tool", "nonexistent-tool"},
		},
	}

	// Must not panic or return an error (the function is void).
	// The unknown tool should be silently dropped with slog.Warn.
	a.ReplaceExecutableSkills(defs)

	// The spawn tool must be registered.
	spawnTool, ok := a.tools["skill-allowlist"]
	if !ok {
		t.Fatal("skill-allowlist should be registered")
	}

	st, ok := spawnTool.(*SubagentSpawnTool)
	if !ok {
		t.Fatal("skill-allowlist should be a *SubagentSpawnTool")
	}

	// The allowlist on the registered def must only contain the known tool.
	allowlist := st.def.ToolsAllowlist
	if len(allowlist) != 1 {
		t.Errorf("expected allowlist len=1, got %d: %v", len(allowlist), allowlist)
	}
	if len(allowlist) == 1 && allowlist[0] != "known-tool" {
		t.Errorf("allowlist[0] = %q, want %q", allowlist[0], "known-tool")
	}
}
