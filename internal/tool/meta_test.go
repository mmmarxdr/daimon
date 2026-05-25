package tool

import (
	"context"
	"encoding/json"
	"testing"

	"daimon/internal/config"
)

// ---------------------------------------------------------------------------
// Fake tools for testing BuildToolMeta
// ---------------------------------------------------------------------------

// fakeTool implements tool.Tool but NOT ToolInspector. Used to assert the
// default fallback path in BuildToolMeta.
type fakeTool struct {
	name string
}

func (f *fakeTool) Name() string            { return f.name }
func (f *fakeTool) Description() string     { return "fake tool" }
func (f *fakeTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}

// fakeInspectable implements BOTH tool.Tool and ToolInspector. Used to assert
// that BuildToolMeta returns the declared metadata verbatim.
type fakeInspectable struct {
	name string
	meta ToolMeta
}

func (f *fakeInspectable) Name() string            { return f.name }
func (f *fakeInspectable) Description() string     { return "inspectable fake tool" }
func (f *fakeInspectable) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *fakeInspectable) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}
func (f *fakeInspectable) Inspect() ToolMeta { return f.meta }

// ---------------------------------------------------------------------------
// Task 1.1 — TestBuildToolMeta_DefaultFallback
// ---------------------------------------------------------------------------

// TestBuildToolMeta_DefaultFallback asserts that a tool that implements only
// Tool (NOT ToolInspector) receives the safe default metadata.
func TestBuildToolMeta_DefaultFallback(t *testing.T) {
	registry := map[string]Tool{
		"fake": &fakeTool{name: "fake"},
	}
	result := BuildToolMeta(registry)

	got, ok := result["fake"]
	if !ok {
		t.Fatal("expected entry for 'fake' in BuildToolMeta result")
	}
	if got.Risk != RiskSideEffects {
		t.Errorf("Risk = %q, want %q", got.Risk, RiskSideEffects)
	}
	if got.Category != CatUnknown {
		t.Errorf("Category = %q, want %q", got.Category, CatUnknown)
	}
	if got.Permission != PermNone {
		t.Errorf("Permission = %q, want %q", got.Permission, PermNone)
	}
	if got.Source != SourceBuiltin {
		t.Errorf("Source = %q, want %q", got.Source, SourceBuiltin)
	}
}

// ---------------------------------------------------------------------------
// Task 1.2 — TestBuildToolMeta_HonorsInspector
// ---------------------------------------------------------------------------

// TestBuildToolMeta_HonorsInspector asserts that a tool implementing
// ToolInspector has its declared metadata passed through verbatim.
func TestBuildToolMeta_HonorsInspector(t *testing.T) {
	declared := ToolMeta{
		Risk:       RiskReadOnly,
		Category:   CatMemory,
		Permission: PermNone,
		Source:     SourceBuiltin,
	}
	registry := map[string]Tool{
		"mem_reader": &fakeInspectable{name: "mem_reader", meta: declared},
	}
	result := BuildToolMeta(registry)

	got, ok := result["mem_reader"]
	if !ok {
		t.Fatal("expected entry for 'mem_reader' in BuildToolMeta result")
	}
	if got != declared {
		t.Errorf("meta = %+v, want %+v", got, declared)
	}
}

// ---------------------------------------------------------------------------
// Task 1.3 — TestBuildToolMeta_Cardinality
// ---------------------------------------------------------------------------

// TestBuildToolMeta_Cardinality asserts that the result map has the same
// cardinality as the input registry regardless of tool type.
func TestBuildToolMeta_Cardinality(t *testing.T) {
	const n = 5
	registry := make(map[string]Tool, n)
	for i := range n {
		name := "tool_" + string(rune('a'+i))
		if i%2 == 0 {
			registry[name] = &fakeTool{name: name}
		} else {
			registry[name] = &fakeInspectable{
				name: name,
				meta: ToolMeta{Risk: RiskReadOnly, Category: CatMemory, Permission: PermNone, Source: SourceBuiltin},
			}
		}
	}
	result := BuildToolMeta(registry)
	if len(result) != n {
		t.Errorf("len(result) = %d, want %d", len(result), n)
	}
}

// ---------------------------------------------------------------------------
// Task 1.4 — TestToolMetaJSON
// ---------------------------------------------------------------------------

// TestToolMetaJSON asserts that ToolMeta marshals to JSON with string values.
func TestToolMetaJSON(t *testing.T) {
	m := ToolMeta{
		Risk:       RiskReadOnly,
		Category:   CatMemory,
		Permission: PermNone,
		Source:     SourceBuiltin,
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	checks := map[string]string{
		"risk":       "read-only",
		"category":   "memory",
		"permission": "none",
		"source":     "builtin",
	}
	for field, want := range checks {
		got, ok := raw[field]
		if !ok {
			t.Errorf("JSON missing field %q", field)
			continue
		}
		if got != want {
			t.Errorf("JSON[%q] = %q, want %q", field, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 1.5 — TestConstants
// ---------------------------------------------------------------------------

// TestConstants asserts that every typed constant resolves to the correct
// string literal defined by the spec.
func TestConstants(t *testing.T) {
	t.Run("RiskLevel", func(t *testing.T) {
		if RiskReadOnly != "read-only" {
			t.Errorf("RiskReadOnly = %q, want %q", RiskReadOnly, "read-only")
		}
		if RiskSideEffects != "side-effects" {
			t.Errorf("RiskSideEffects = %q, want %q", RiskSideEffects, "side-effects")
		}
		if RiskDestructive != "destructive" {
			t.Errorf("RiskDestructive = %q, want %q", RiskDestructive, "destructive")
		}
	})
	t.Run("PermissionLevel", func(t *testing.T) {
		if PermNone != "none" {
			t.Errorf("PermNone = %q, want %q", PermNone, "none")
		}
		if PermUser != "user" {
			t.Errorf("PermUser = %q, want %q", PermUser, "user")
		}
		if PermAdmin != "admin" {
			t.Errorf("PermAdmin = %q, want %q", PermAdmin, "admin")
		}
	})
	t.Run("ToolCategory", func(t *testing.T) {
		if CatShell != "shell" {
			t.Errorf("CatShell = %q, want %q", CatShell, "shell")
		}
		if CatFile != "file" {
			t.Errorf("CatFile = %q, want %q", CatFile, "file")
		}
		if CatNetwork != "network" {
			t.Errorf("CatNetwork = %q, want %q", CatNetwork, "network")
		}
		if CatMemory != "memory" {
			t.Errorf("CatMemory = %q, want %q", CatMemory, "memory")
		}
		if CatScheduling != "scheduling" {
			t.Errorf("CatScheduling = %q, want %q", CatScheduling, "scheduling")
		}
		if CatKnowledge != "knowledge" {
			t.Errorf("CatKnowledge = %q, want %q", CatKnowledge, "knowledge")
		}
		if CatMeta != "meta" {
			t.Errorf("CatMeta = %q, want %q", CatMeta, "meta")
		}
		if CatMCP != "mcp" {
			t.Errorf("CatMCP = %q, want %q", CatMCP, "mcp")
		}
		if CatUnknown != "unknown" {
			t.Errorf("CatUnknown = %q, want %q", CatUnknown, "unknown")
		}
	})
	t.Run("ToolSource", func(t *testing.T) {
		if SourceBuiltin != "builtin" {
			t.Errorf("SourceBuiltin = %q, want %q", SourceBuiltin, "builtin")
		}
		if SourceMCP != "mcp" {
			t.Errorf("SourceMCP = %q, want %q", SourceMCP, "mcp")
		}
		if SourceSkill != "skill" {
			t.Errorf("SourceSkill = %q, want %q", SourceSkill, "skill")
		}
	})
}

// ---------------------------------------------------------------------------
// Task 2.1 — TestBuiltinInspect (high-risk built-ins, Phase 2)
// ---------------------------------------------------------------------------

// TestBuiltinInspect is a table-driven test asserting that real built-in tool
// constructors return the exact ToolMeta declared in AD-5. Phase 2 covers the
// high-risk subset included in PR1; Phase 5 (WU-2) extends with the remaining
// read-only / low-risk built-ins.
func TestBuiltinInspect(t *testing.T) {
	memDeps := MemoryToolDeps{}
	todoDeps := TodoToolDeps{}

	tests := []struct {
		name     string
		tool     Tool
		wantMeta ToolMeta
	}{
		// Phase 2 (PR1) — high-risk built-ins
		{
			name:     "shell_exec",
			tool:     NewShellTool(defaultShellCfg()),
			wantMeta: ToolMeta{Risk: RiskDestructive, Category: CatShell, Permission: PermAdmin, Source: SourceBuiltin},
		},
		{
			name:     "batch_exec",
			tool:     NewBatchExecTool(nil, BatchExecToolConfig{}),
			wantMeta: ToolMeta{Risk: RiskDestructive, Category: CatShell, Permission: PermAdmin, Source: SourceBuiltin},
		},
		{
			name:     "write_file",
			tool:     NewWriteFileTool(defaultFileCfg()),
			wantMeta: ToolMeta{Risk: RiskSideEffects, Category: CatFile, Permission: PermUser, Source: SourceBuiltin},
		},
		{
			name:     "http_fetch",
			tool:     NewHTTPFetchTool(defaultHTTPCfg()),
			wantMeta: ToolMeta{Risk: RiskSideEffects, Category: CatNetwork, Permission: PermUser, Source: SourceBuiltin},
		},
		{
			name:     "web_fetch",
			tool:     NewWebFetchTool(defaultWebCfg()),
			wantMeta: ToolMeta{Risk: RiskSideEffects, Category: CatNetwork, Permission: PermUser, Source: SourceBuiltin},
		},

		// Phase 5 (PR2) — fileops read-only tools (task 5.2)
		{
			name:     "read_file",
			tool:     NewReadFileTool(defaultFileCfg()),
			wantMeta: ToolMeta{Risk: RiskReadOnly, Category: CatFile, Permission: PermNone, Source: SourceBuiltin},
		},
		{
			name:     "list_files",
			tool:     NewListFilesTool(defaultFileCfg()),
			wantMeta: ToolMeta{Risk: RiskReadOnly, Category: CatFile, Permission: PermNone, Source: SourceBuiltin},
		},

		// Phase 5 (PR2) — memory tools (task 5.3)
		{
			name:     "search_memory",
			tool:     &searchMemoryTool{deps: memDeps},
			wantMeta: ToolMeta{Risk: RiskReadOnly, Category: CatMemory, Permission: PermNone, Source: SourceBuiltin},
		},
		{
			name:     "save_memory",
			tool:     &saveMemoryTool{deps: memDeps},
			wantMeta: ToolMeta{Risk: RiskSideEffects, Category: CatMemory, Permission: PermUser, Source: SourceBuiltin},
		},
		{
			name:     "update_memory",
			tool:     &updateMemoryTool{deps: memDeps},
			wantMeta: ToolMeta{Risk: RiskSideEffects, Category: CatMemory, Permission: PermUser, Source: SourceBuiltin},
		},
		{
			name:     "forget_memory",
			tool:     &forgetMemoryTool{deps: memDeps},
			wantMeta: ToolMeta{Risk: RiskDestructive, Category: CatMemory, Permission: PermUser, Source: SourceBuiltin},
		},

		// Phase 5 (PR2) — todo tools (task 5.4)
		{
			name:     "todo_list",
			tool:     &todoListTool{deps: todoDeps},
			wantMeta: ToolMeta{Risk: RiskReadOnly, Category: CatMeta, Permission: PermNone, Source: SourceBuiltin},
		},
		{
			name:     "todo_create",
			tool:     &todoCreateTool{deps: todoDeps},
			wantMeta: ToolMeta{Risk: RiskSideEffects, Category: CatMeta, Permission: PermUser, Source: SourceBuiltin},
		},
		{
			name:     "todo_update",
			tool:     &todoUpdateTool{deps: todoDeps},
			wantMeta: ToolMeta{Risk: RiskSideEffects, Category: CatMeta, Permission: PermUser, Source: SourceBuiltin},
		},

		// Phase 5 (PR2) — cron tools (task 5.5)
		{
			name:     "list_crons",
			tool:     &listCronsTool{},
			wantMeta: ToolMeta{Risk: RiskReadOnly, Category: CatScheduling, Permission: PermNone, Source: SourceBuiltin},
		},
		{
			name:     "schedule_task",
			tool:     &scheduleTaskTool{},
			wantMeta: ToolMeta{Risk: RiskSideEffects, Category: CatScheduling, Permission: PermUser, Source: SourceBuiltin},
		},
		{
			name:     "delete_cron",
			tool:     &deleteCronTool{},
			wantMeta: ToolMeta{Risk: RiskDestructive, Category: CatScheduling, Permission: PermUser, Source: SourceBuiltin},
		},

		// Phase 5 (PR2) — search_output (task 5.6)
		{
			name:     "search_output",
			tool:     NewSearchOutputTool(nil),
			wantMeta: ToolMeta{Risk: RiskReadOnly, Category: CatMeta, Permission: PermNone, Source: SourceBuiltin},
		},

		// Phase 5 (PR2) — skill_loader (task 5.7)
		{
			name:     "load_skill",
			tool:     NewSkillLoaderTool(nil),
			wantMeta: ToolMeta{Risk: RiskReadOnly, Category: CatMeta, Permission: PermNone, Source: SourceBuiltin},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inspector, ok := tc.tool.(ToolInspector)
			if !ok {
				t.Fatalf("%s does not implement ToolInspector", tc.name)
			}
			got := inspector.Inspect()
			if got != tc.wantMeta {
				t.Errorf("Inspect() = %+v, want %+v", got, tc.wantMeta)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 3.2 — TestBuildToolMeta_MCPDefaultsThroughRegistry
// ---------------------------------------------------------------------------

// fakeInspectableMCP is a minimal stand-in for an MCP tool that implements
// ToolInspector with mcp-tagged metadata. We cannot import internal/mcp here
// (cycle), so we use a local fake that returns the same values MCPToolAdapter will.
type fakeInspectableMCP struct {
	name string
}

func (f *fakeInspectableMCP) Name() string            { return f.name }
func (f *fakeInspectableMCP) Description() string     { return "mcp fake" }
func (f *fakeInspectableMCP) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *fakeInspectableMCP) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}
func (f *fakeInspectableMCP) Inspect() ToolMeta {
	return ToolMeta{Risk: RiskSideEffects, Category: CatMCP, Permission: PermUser, Source: SourceMCP}
}

// TestBuildToolMeta_MCPDefaultsThroughRegistry asserts that when a tool whose
// Inspect() returns mcp-tagged metadata is put in the registry, BuildToolMeta
// returns the expected {side-effects, mcp, user, mcp} entry.
func TestBuildToolMeta_MCPDefaultsThroughRegistry(t *testing.T) {
	registry := map[string]Tool{
		"some_mcp_tool": &fakeInspectableMCP{name: "some_mcp_tool"},
	}
	result := BuildToolMeta(registry)

	got, ok := result["some_mcp_tool"]
	if !ok {
		t.Fatal("expected entry for 'some_mcp_tool' in result")
	}
	want := ToolMeta{Risk: RiskSideEffects, Category: CatMCP, Permission: PermUser, Source: SourceMCP}
	if got != want {
		t.Errorf("meta = %+v, want %+v", got, want)
	}
}

// ---------------------------------------------------------------------------
// Zero-value config helpers (used by TestBuiltinInspect)
// ---------------------------------------------------------------------------

func defaultShellCfg() config.ShellToolConfig {
	return config.ShellToolConfig{}
}

func defaultFileCfg() config.FileToolConfig {
	return config.FileToolConfig{}
}

func defaultHTTPCfg() config.HTTPToolConfig {
	return config.HTTPToolConfig{}
}

func defaultWebCfg() config.WebFetchConfig {
	return config.WebFetchConfig{}
}
