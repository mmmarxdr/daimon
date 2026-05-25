package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// Fake tools for handler tests
// ---------------------------------------------------------------------------

// fakeMetaTool implements tool.Tool AND tool.ToolInspector.
// Used to inject known metadata into the deps.Tools map.
type fakeMetaTool struct {
	name string
	meta tool.ToolMeta
}

func (f *fakeMetaTool) Name() string            { return f.name }
func (f *fakeMetaTool) Description() string     { return "fake meta tool: " + f.name }
func (f *fakeMetaTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *fakeMetaTool) Execute(_ context.Context, _ json.RawMessage) (tool.ToolResult, error) {
	return tool.ToolResult{Content: "ok"}, nil
}
func (f *fakeMetaTool) Inspect() tool.ToolMeta { return f.meta }

// fakeNoInspectTool implements tool.Tool but NOT ToolInspector.
// Used to assert the default-fallback path survives the handler.
type fakeNoInspectTool struct {
	name string
}

func (f *fakeNoInspectTool) Name() string            { return f.name }
func (f *fakeNoInspectTool) Description() string     { return "no-inspect: " + f.name }
func (f *fakeNoInspectTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *fakeNoInspectTool) Execute(_ context.Context, _ json.RawMessage) (tool.ToolResult, error) {
	return tool.ToolResult{Content: "ok"}, nil
}

// ---------------------------------------------------------------------------
// Helper — newToolsTestServer
// ---------------------------------------------------------------------------

func newToolsTestServer(t *testing.T, tools map[string]tool.Tool) *Server {
	t.Helper()
	if tools == nil {
		tools = map[string]tool.Tool{}
	}
	s := &Server{
		deps: ServerDeps{
			Store:  &fakeWebStore{},
			Config: minimalConfig(),
			Tools:  tools,
		},
		mux:        http.NewServeMux(),
		wsUpgrader: newWSUpgrader(nil),
	}
	s.routes()
	return s
}

// getTools performs GET /api/tools and returns the recorder.
func getTools(t *testing.T, srv *Server) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/tools", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// Task 4.1 — TestHandleListTools_ResponseShape
// ---------------------------------------------------------------------------

// TestHandleListTools_ResponseShape asserts that GET /api/tools returns a JSON
// object with keys "tools" (array) and "summary" (object with total, by_risk,
// by_category), and that total equals len(tools).
func TestHandleListTools_ResponseShape(t *testing.T) {
	tools := map[string]tool.Tool{
		"alpha": &fakeMetaTool{name: "alpha", meta: tool.ToolMeta{
			Risk: tool.RiskReadOnly, Category: tool.CatMemory,
			Permission: tool.PermNone, Source: tool.SourceBuiltin,
		}},
		"beta": &fakeMetaTool{name: "beta", meta: tool.ToolMeta{
			Risk: tool.RiskSideEffects, Category: tool.CatNetwork,
			Permission: tool.PermUser, Source: tool.SourceMCP,
		}},
	}
	srv := newToolsTestServer(t, tools)
	w := getTools(t, srv)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tools   []json.RawMessage `json:"tools"`
		Summary struct {
			Total      int            `json:"total"`
			ByRisk     map[string]int `json:"by_risk"`
			ByCategory map[string]int `json:"by_category"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Tools) != 2 {
		t.Errorf("tools count = %d, want 2", len(resp.Tools))
	}
	if resp.Summary.Total != len(resp.Tools) {
		t.Errorf("summary.total = %d, want %d (len of tools array)", resp.Summary.Total, len(resp.Tools))
	}
	if resp.Summary.ByRisk == nil {
		t.Error("summary.by_risk must not be nil")
	}
	if resp.Summary.ByCategory == nil {
		t.Error("summary.by_category must not be nil")
	}
}

// ---------------------------------------------------------------------------
// Task 4.2 — TestHandleListTools_MetaJoinCorrectness
// ---------------------------------------------------------------------------

// TestHandleListTools_MetaJoinCorrectness asserts that each tool entry in the
// response has risk, category, permission, and source fields that match the
// tool's declared ToolMeta.
func TestHandleListTools_MetaJoinCorrectness(t *testing.T) {
	tools := map[string]tool.Tool{
		"shell_tool": &fakeMetaTool{name: "shell_tool", meta: tool.ToolMeta{
			Risk: tool.RiskDestructive, Category: tool.CatShell,
			Permission: tool.PermAdmin, Source: tool.SourceBuiltin,
		}},
	}
	srv := newToolsTestServer(t, tools)
	w := getTools(t, srv)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Risk        string `json:"risk"`
			Category    string `json:"category"`
			Permission  string `json:"permission"`
			Source      string `json:"source"`
		} `json:"tools"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(resp.Tools))
	}

	ti := resp.Tools[0]
	if ti.Name == "" {
		t.Error("tool.name must not be empty")
	}
	if ti.Description == "" {
		t.Error("tool.description must not be empty")
	}
	if ti.Risk != "destructive" {
		t.Errorf("risk = %q, want %q", ti.Risk, "destructive")
	}
	if ti.Category != "shell" {
		t.Errorf("category = %q, want %q", ti.Category, "shell")
	}
	if ti.Permission != "admin" {
		t.Errorf("permission = %q, want %q", ti.Permission, "admin")
	}
	if ti.Source != "builtin" {
		t.Errorf("source = %q, want %q", ti.Source, "builtin")
	}
}

// ---------------------------------------------------------------------------
// Task 4.3 — TestHandleListTools_SummaryCounts
// ---------------------------------------------------------------------------

// TestHandleListTools_SummaryCounts asserts that by_risk and by_category counts
// in the summary exactly match the per-tool tally.
func TestHandleListTools_SummaryCounts(t *testing.T) {
	// 3 read-only, 2 side-effects, 1 destructive = 6 total
	// 2 memory, 3 shell, 1 network
	tools := map[string]tool.Tool{
		"ro1": &fakeMetaTool{name: "ro1", meta: tool.ToolMeta{Risk: tool.RiskReadOnly, Category: tool.CatMemory, Permission: tool.PermNone, Source: tool.SourceBuiltin}},
		"ro2": &fakeMetaTool{name: "ro2", meta: tool.ToolMeta{Risk: tool.RiskReadOnly, Category: tool.CatMemory, Permission: tool.PermNone, Source: tool.SourceBuiltin}},
		"ro3": &fakeMetaTool{name: "ro3", meta: tool.ToolMeta{Risk: tool.RiskReadOnly, Category: tool.CatShell, Permission: tool.PermNone, Source: tool.SourceBuiltin}},
		"se1": &fakeMetaTool{name: "se1", meta: tool.ToolMeta{Risk: tool.RiskSideEffects, Category: tool.CatShell, Permission: tool.PermUser, Source: tool.SourceBuiltin}},
		"se2": &fakeMetaTool{name: "se2", meta: tool.ToolMeta{Risk: tool.RiskSideEffects, Category: tool.CatNetwork, Permission: tool.PermUser, Source: tool.SourceMCP}},
		"d1":  &fakeMetaTool{name: "d1", meta: tool.ToolMeta{Risk: tool.RiskDestructive, Category: tool.CatShell, Permission: tool.PermAdmin, Source: tool.SourceBuiltin}},
	}
	srv := newToolsTestServer(t, tools)
	w := getTools(t, srv)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tools   []json.RawMessage `json:"tools"`
		Summary struct {
			Total      int            `json:"total"`
			ByRisk     map[string]int `json:"by_risk"`
			ByCategory map[string]int `json:"by_category"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Summary.Total != 6 {
		t.Errorf("total = %d, want 6", resp.Summary.Total)
	}
	if resp.Summary.ByRisk["read-only"] != 3 {
		t.Errorf("by_risk[read-only] = %d, want 3", resp.Summary.ByRisk["read-only"])
	}
	if resp.Summary.ByRisk["side-effects"] != 2 {
		t.Errorf("by_risk[side-effects] = %d, want 2", resp.Summary.ByRisk["side-effects"])
	}
	if resp.Summary.ByRisk["destructive"] != 1 {
		t.Errorf("by_risk[destructive] = %d, want 1", resp.Summary.ByRisk["destructive"])
	}
	if resp.Summary.ByCategory["memory"] != 2 {
		t.Errorf("by_category[memory] = %d, want 2", resp.Summary.ByCategory["memory"])
	}
	if resp.Summary.ByCategory["shell"] != 3 {
		t.Errorf("by_category[shell] = %d, want 3", resp.Summary.ByCategory["shell"])
	}
	if resp.Summary.ByCategory["network"] != 1 {
		t.Errorf("by_category[network] = %d, want 1", resp.Summary.ByCategory["network"])
	}
	// total must also equal len(tools array)
	if resp.Summary.Total != len(resp.Tools) {
		t.Errorf("summary.total = %d, want %d (len of tools array)", resp.Summary.Total, len(resp.Tools))
	}
}

// ---------------------------------------------------------------------------
// Task 4.4 — TestHandleListTools_PermissionNonEnforcement
// ---------------------------------------------------------------------------

// TestHandleListTools_PermissionNonEnforcement asserts that a tool tagged
// permission="admin" appears in the response and is not blocked or rejected.
// Permission is descriptive-only (spec REQ-8).
func TestHandleListTools_PermissionNonEnforcement(t *testing.T) {
	tools := map[string]tool.Tool{
		"admin_tool": &fakeMetaTool{name: "admin_tool", meta: tool.ToolMeta{
			Risk: tool.RiskDestructive, Category: tool.CatShell,
			Permission: tool.PermAdmin, Source: tool.SourceBuiltin,
		}},
		"none_tool": &fakeMetaTool{name: "none_tool", meta: tool.ToolMeta{
			Risk: tool.RiskReadOnly, Category: tool.CatMemory,
			Permission: tool.PermNone, Source: tool.SourceBuiltin,
		}},
	}
	srv := newToolsTestServer(t, tools)
	w := getTools(t, srv)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tools []struct {
			Name       string `json:"name"`
			Permission string `json:"permission"`
		} `json:"tools"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d — admin tool must NOT be filtered", len(resp.Tools))
	}

	var foundAdmin bool
	for _, ti := range resp.Tools {
		if ti.Name == "admin_tool" {
			foundAdmin = true
			if ti.Permission != "admin" {
				t.Errorf("admin_tool permission = %q, want %q", ti.Permission, "admin")
			}
		}
	}
	if !foundAdmin {
		t.Error("admin_tool must appear in response — Permission is descriptive only, not enforced")
	}
}

// ---------------------------------------------------------------------------
// TestHandleListTools_DefaultFallback — non-inspector tools surface the default
// ---------------------------------------------------------------------------

// TestHandleListTools_DefaultFallback asserts that a tool which does NOT implement
// tool.ToolInspector surfaces the BuildToolMeta default (side-effects / unknown /
// none / builtin) through the handler, rather than empty or missing metadata.
func TestHandleListTools_DefaultFallback(t *testing.T) {
	tools := map[string]tool.Tool{
		"plain": &fakeNoInspectTool{name: "plain"},
	}
	srv := newToolsTestServer(t, tools)
	w := getTools(t, srv)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tools []struct {
			Name       string `json:"name"`
			Risk       string `json:"risk"`
			Category   string `json:"category"`
			Permission string `json:"permission"`
			Source     string `json:"source"`
		} `json:"tools"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(resp.Tools))
	}

	ti := resp.Tools[0]
	if ti.Risk != string(tool.RiskSideEffects) {
		t.Errorf("default risk = %q, want %q", ti.Risk, tool.RiskSideEffects)
	}
	if ti.Category != string(tool.CatUnknown) {
		t.Errorf("default category = %q, want %q", ti.Category, tool.CatUnknown)
	}
	if ti.Permission != string(tool.PermNone) {
		t.Errorf("default permission = %q, want %q", ti.Permission, tool.PermNone)
	}
	if ti.Source != string(tool.SourceBuiltin) {
		t.Errorf("default source = %q, want %q", ti.Source, tool.SourceBuiltin)
	}
}

// ---------------------------------------------------------------------------
// TestHandleListTools_MetaKeyedByRegistryKey — join by registry key, not Name()
// ---------------------------------------------------------------------------

// TestHandleListTools_MetaKeyedByRegistryKey pins that the handler joins each
// tool to its metadata by the REGISTRY KEY (the same key BuildToolMeta indexes
// by), not by the tool's Name(). It registers a tool under a key that differs
// from its Name() and asserts the metadata still resolves — guarding against a
// regression to `meta[t.Name()]`, which would silently yield empty metadata.
func TestHandleListTools_MetaKeyedByRegistryKey(t *testing.T) {
	tools := map[string]tool.Tool{
		"registry_key": &fakeMetaTool{name: "self_name", meta: tool.ToolMeta{
			Risk: tool.RiskDestructive, Category: tool.CatShell,
			Permission: tool.PermAdmin, Source: tool.SourceBuiltin,
		}},
	}
	srv := newToolsTestServer(t, tools)
	w := getTools(t, srv)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Tools []struct {
			Name string `json:"name"`
			Risk string `json:"risk"`
		} `json:"tools"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(resp.Tools))
	}
	if resp.Tools[0].Risk != "destructive" {
		t.Errorf("risk = %q, want %q — meta must be keyed by registry key, not Name()", resp.Tools[0].Risk, "destructive")
	}
}
