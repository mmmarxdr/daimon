package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// Helpers — fakeUserSkillStore
// ---------------------------------------------------------------------------

// fakeUserSkillStore is a programmable in-memory implementation of
// store.UserSkillStore used across handler_skills tests.
type fakeUserSkillStore struct {
	skills     []store.UserSkill
	failGet    bool // when true, GetUserSkill returns a generic error
	failCreate error
	failUpdate error
	failDelete error
}

func (f *fakeUserSkillStore) ListUserSkills(_ context.Context) ([]store.UserSkill, error) {
	out := make([]store.UserSkill, len(f.skills))
	copy(out, f.skills)
	return out, nil
}

func (f *fakeUserSkillStore) GetUserSkill(_ context.Context, name string) (store.UserSkill, error) {
	if f.failGet {
		return store.UserSkill{}, fmt.Errorf("db error")
	}
	for _, s := range f.skills {
		if s.Name == name {
			return s, nil
		}
	}
	return store.UserSkill{}, fmt.Errorf("get user_skill %q: %w", name, store.ErrNotFound)
}

func (f *fakeUserSkillStore) CreateUserSkill(_ context.Context, s store.UserSkill) (store.UserSkill, error) {
	if f.failCreate != nil {
		return store.UserSkill{}, f.failCreate
	}
	// check uniqueness
	for _, existing := range f.skills {
		if existing.Name == s.Name {
			return store.UserSkill{}, fmt.Errorf("create user_skill %q: %w", s.Name, store.ErrNameConflict)
		}
	}
	if s.ID == "" {
		s.ID = "test-id-" + s.Name
	}
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = s.CreatedAt
	}
	if s.Source == "" {
		s.Source = "user"
	}
	if s.Version == 0 {
		s.Version = 1
	}
	f.skills = append(f.skills, s)
	return s, nil
}

func (f *fakeUserSkillStore) UpdateUserSkill(_ context.Context, s store.UserSkill) (store.UserSkill, error) {
	if f.failUpdate != nil {
		return store.UserSkill{}, f.failUpdate
	}
	for i, existing := range f.skills {
		if existing.Name == s.Name {
			s.UpdatedAt = time.Now().UTC()
			s.Version = existing.Version + 1
			f.skills[i] = s
			return s, nil
		}
	}
	return store.UserSkill{}, fmt.Errorf("update user_skill %q: %w", s.Name, store.ErrNotFound)
}

func (f *fakeUserSkillStore) DeleteUserSkill(_ context.Context, name string) error {
	if f.failDelete != nil {
		return f.failDelete
	}
	for i, s := range f.skills {
		if s.Name == name {
			f.skills = append(f.skills[:i], f.skills[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("delete user_skill %q: %w", name, store.ErrNotFound)
}

// ---------------------------------------------------------------------------
// Helpers — fakeAgentReloader (for 3.13 and integration tests)
// ---------------------------------------------------------------------------

// fakeAgentReloader is a programmable mock of AgentReloader that records calls.
type fakeAgentReloader struct {
	replaceSkillsCalls           int
	replaceExecutableSkillsCalls int
	lastExecDefs                 []skill.ExecutableSkillDef
	lastSkillContents            []skill.SkillContent
}

func (f *fakeAgentReloader) RegisterMCPServer(_ string, _ map[string]tool.Tool, _ interface{ Close() error }) {
}
func (f *fakeAgentReloader) UnregisterMCPServer(_ string) error { return nil }
func (f *fakeAgentReloader) ReplaceSkills(skills []skill.SkillContent, _ skill.SkillIndex) {
	f.replaceSkillsCalls++
	f.lastSkillContents = skills
}
func (f *fakeAgentReloader) ReplaceExecutableSkills(defs []skill.ExecutableSkillDef) {
	f.replaceExecutableSkillsCalls++
	f.lastExecDefs = defs
}

// ---------------------------------------------------------------------------
// Helper — newSkillsTestServer
// ---------------------------------------------------------------------------

func newSkillsTestServer(t *testing.T, uss store.UserSkillStore, agent AgentReloader, knownTools map[string]tool.Tool) *Server {
	t.Helper()
	if knownTools == nil {
		knownTools = map[string]tool.Tool{}
	}
	s := &Server{
		deps: ServerDeps{
			Store:          &fakeWebStore{},
			Config:         minimalConfig(),
			UserSkillStore: uss,
			Agent:          agent,
			Tools:          knownTools,
		},
		mux:        http.NewServeMux(),
		wsUpgrader: newWSUpgrader(nil),
	}
	s.routes()
	return s
}

// ---------------------------------------------------------------------------
// Task 3.8 — GET /api/skills (list)
// ---------------------------------------------------------------------------

func TestHandleListSkills_SourceUserFilter(t *testing.T) {
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "1", Name: "alpha", Source: "user", Version: 1},
			{ID: "2", Name: "beta", Source: "curated", Version: 1},
		},
	}
	srv := newSkillsTestServer(t, uss, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/skills?source=user", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Skills []store.UserSkill `json:"skills"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Skills) != 1 {
		t.Fatalf("expected 1 user skill, got %d", len(resp.Skills))
	}
	if resp.Skills[0].Source != "user" {
		t.Errorf("expected source=user, got %q", resp.Skills[0].Source)
	}
}

func TestHandleListSkills_SourceCuratedFilter(t *testing.T) {
	// curated source is not in DB in V1; filter returns empty.
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "1", Name: "alpha", Source: "user", Version: 1},
		},
	}
	srv := newSkillsTestServer(t, uss, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/skills?source=curated", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Skills []store.UserSkill `json:"skills"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// V1: curated rows are not in DB — filter returns empty slice.
	if len(resp.Skills) != 0 {
		t.Errorf("expected 0 curated skills in V1, got %d", len(resp.Skills))
	}
}

func TestHandleListSkills_AllOrNoParam(t *testing.T) {
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "1", Name: "alpha", Source: "user", Version: 1},
			{ID: "2", Name: "beta", Source: "user", Version: 1},
		},
	}

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"no param", "/api/skills", 2},
		{"source=all", "/api/skills?source=all", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newSkillsTestServer(t, uss, nil, nil)
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			w := httptest.NewRecorder()
			srv.mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			ct := w.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			var resp struct {
				Skills []store.UserSkill `json:"skills"`
			}
			if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(resp.Skills) != tt.want {
				t.Errorf("expected %d skills, got %d", tt.want, len(resp.Skills))
			}
		})
	}
}

func TestHandleListSkills_NilStoreReturnsEmpty(t *testing.T) {
	srv := newSkillsTestServer(t, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/skills", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Skills []store.UserSkill `json:"skills"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Skills == nil {
		t.Error("skills must be non-nil (empty array, not null)")
	}
	if len(resp.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(resp.Skills))
	}
}

// ---------------------------------------------------------------------------
// Task 3.9 — GET /api/skills/{name}
// ---------------------------------------------------------------------------

func TestHandleGetSkill_Existing(t *testing.T) {
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "1", Name: "my-skill", Source: "user", Prose: "hello", Version: 1},
		},
	}
	srv := newSkillsTestServer(t, uss, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/skills/my-skill", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var s store.UserSkill
	if err := json.NewDecoder(w.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Name != "my-skill" {
		t.Errorf("name = %q, want 'my-skill'", s.Name)
	}
}

func TestHandleGetSkill_Missing(t *testing.T) {
	uss := &fakeUserSkillStore{}
	srv := newSkillsTestServer(t, uss, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/skills/no-such", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Task 3.10 — POST /api/skills (create)
// ---------------------------------------------------------------------------

func postSkill(t *testing.T, srv *Server, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/skills", bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	return w
}

func TestHandleCreateSkill_Valid201(t *testing.T) {
	uss := &fakeUserSkillStore{}
	srv := newSkillsTestServer(t, uss, nil, nil)

	w := postSkill(t, srv, map[string]any{
		"name":  "my-skill",
		"prose": "Do stuff.",
	})

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if loc != "/api/skills/my-skill" {
		t.Errorf("Location = %q, want '/api/skills/my-skill'", loc)
	}
	var s store.UserSkill
	if err := json.NewDecoder(w.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Name != "my-skill" {
		t.Errorf("name = %q, want 'my-skill'", s.Name)
	}
	if s.Source != "user" {
		t.Errorf("source = %q, want 'user'", s.Source)
	}
}

func TestHandleCreateSkill_NameRegexViolation422(t *testing.T) {
	uss := &fakeUserSkillStore{}
	srv := newSkillsTestServer(t, uss, nil, nil)

	tests := []struct {
		name string
		body map[string]any
	}{
		{"starts with digit", map[string]any{"name": "1bad", "prose": "x"}},
		{"starts with uppercase", map[string]any{"name": "Bad", "prose": "x"}},
		{"has space", map[string]any{"name": "bad skill", "prose": "x"}},
		{"too long", map[string]any{"name": strings.Repeat("a", 65), "prose": "x"}},
		{"empty name", map[string]any{"name": "", "prose": "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := postSkill(t, srv, tt.body)
			if w.Code != http.StatusUnprocessableEntity {
				t.Errorf("expected 422, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleCreateSkill_UniqueConflict409(t *testing.T) {
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "1", Name: "existing", Source: "user", Version: 1},
		},
	}
	srv := newSkillsTestServer(t, uss, nil, nil)

	w := postSkill(t, srv, map[string]any{
		"name":  "existing",
		"prose": "duplicate",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateSkill_ProseOver8KB_422(t *testing.T) {
	uss := &fakeUserSkillStore{}
	srv := newSkillsTestServer(t, uss, nil, nil)

	bigProse := strings.Repeat("x", 8*1024+1)
	w := postSkill(t, srv, map[string]any{
		"name":  "prose-test",
		"prose": bigProse,
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateSkill_UnknownTool422(t *testing.T) {
	uss := &fakeUserSkillStore{}
	// knownTools has "shell_exec" but NOT "unknown_tool"
	knownTools := map[string]tool.Tool{
		"shell_exec": &fakeTool{name: "shell_exec"},
	}
	srv := newSkillsTestServer(t, uss, nil, knownTools)

	w := postSkill(t, srv, map[string]any{
		"name":            "my-skill",
		"prose":           "stuff",
		"tools_allowlist": []string{"unknown_tool"},
	})
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for unknown tool, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateSkill_MalformedJSON400(t *testing.T) {
	uss := &fakeUserSkillStore{}
	srv := newSkillsTestServer(t, uss, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/skills", strings.NewReader("{not valid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Task 3.11 — PUT /api/skills/{name}
// ---------------------------------------------------------------------------

func putSkill(t *testing.T, srv *Server, name string, body any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/skills/"+name, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	return w
}

func TestHandleUpdateSkill_UserSource200(t *testing.T) {
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "1", Name: "my-skill", Source: "user", Prose: "old", Version: 1},
		},
	}
	srv := newSkillsTestServer(t, uss, nil, nil)

	w := putSkill(t, srv, "my-skill", map[string]any{
		"name":  "my-skill",
		"prose": "updated prose",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var s store.UserSkill
	if err := json.NewDecoder(w.Body).Decode(&s); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if s.Prose != "updated prose" {
		t.Errorf("prose = %q, want 'updated prose'", s.Prose)
	}
}

func TestHandleUpdateSkill_CuratedSource403(t *testing.T) {
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "2", Name: "curated-skill", Source: "curated", Version: 1},
		},
	}
	srv := newSkillsTestServer(t, uss, nil, nil)

	w := putSkill(t, srv, "curated-skill", map[string]any{
		"name":  "curated-skill",
		"prose": "hacked",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Task 3.12 — DELETE /api/skills/{name}
// ---------------------------------------------------------------------------

func deleteSkill(t *testing.T, srv *Server, name string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/api/skills/"+name, nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	return w
}

func TestHandleDeleteSkill_UserSource204(t *testing.T) {
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "1", Name: "my-skill", Source: "user", Version: 1},
		},
	}
	srv := newSkillsTestServer(t, uss, nil, nil)

	w := deleteSkill(t, srv, "my-skill")
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteSkill_CuratedSource403(t *testing.T) {
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "2", Name: "curated-skill", Source: "curated", Version: 1},
		},
	}
	srv := newSkillsTestServer(t, uss, nil, nil)

	w := deleteSkill(t, srv, "curated-skill")
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeleteSkill_Missing404(t *testing.T) {
	uss := &fakeUserSkillStore{}
	srv := newSkillsTestServer(t, uss, nil, nil)

	w := deleteSkill(t, srv, "no-such")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Task 3.13 — s.reloadSkills() helper
// ---------------------------------------------------------------------------

// TestReloadSkills verifies that reloadSkills calls ReplaceExecutableSkills
// and ReplaceSkills on the agent once per invocation. LoadSkillsUnified is not
// available yet (Phase 4); reloadSkills uses a DB-only stub that calls
// ListUserSkills and converts results without FS or curated passes.
func TestReloadSkills_CallsAgentMethods(t *testing.T) {
	execSkill := store.UserSkill{
		ID:         "1",
		Name:       "my-skill",
		Source:     "user",
		Prose:      "do stuff",
		Executable: true,
		Version:    1,
	}
	uss := &fakeUserSkillStore{skills: []store.UserSkill{execSkill}}
	reloader := &fakeAgentReloader{}
	srv := newSkillsTestServer(t, uss, reloader, nil)

	srv.reloadSkills(context.Background())

	if reloader.replaceExecutableSkillsCalls != 1 {
		t.Errorf("ReplaceExecutableSkills called %d times, want 1", reloader.replaceExecutableSkillsCalls)
	}
	if reloader.replaceSkillsCalls != 1 {
		t.Errorf("ReplaceSkills called %d times, want 1", reloader.replaceSkillsCalls)
	}
	if len(reloader.lastExecDefs) != 1 {
		t.Errorf("exec defs = %d, want 1", len(reloader.lastExecDefs))
	}
	if reloader.lastExecDefs[0].Name != "my-skill" {
		t.Errorf("exec def name = %q, want 'my-skill'", reloader.lastExecDefs[0].Name)
	}
}

func TestReloadSkills_NilAgent_NoOp(t *testing.T) {
	uss := &fakeUserSkillStore{}
	srv := newSkillsTestServer(t, uss, nil, nil) // no agent

	// Must not panic
	srv.reloadSkills(context.Background())
}

// ---------------------------------------------------------------------------
// Task 3.14 — Integration: allowlist TWO modes
// ---------------------------------------------------------------------------

// TestAllowlistTwoModes verifies:
// (a) REST write with unknown tool → 422 hard error
// (b) hot-reload via reloadSkills() with unknown tool in stored allowlist → warn-and-drop (no panic)
func TestAllowlistTwoModes(t *testing.T) {
	knownTools := map[string]tool.Tool{
		"shell_exec": &fakeTool{name: "shell_exec"},
	}

	t.Run("(a) REST write unknown tool => 422", func(t *testing.T) {
		uss := &fakeUserSkillStore{}
		srv := newSkillsTestServer(t, uss, nil, knownTools)

		w := postSkill(t, srv, map[string]any{
			"name":            "bad-skill",
			"prose":           "stuff",
			"tools_allowlist": []string{"not_a_real_tool"},
		})
		if w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("(b) hot-reload stored allowlist with unknown tool => no error", func(t *testing.T) {
		// Simulate a skill already stored in DB with an unknown tool in allowlist.
		// This can happen if the tool was removed from the env after the skill was created.
		uss := &fakeUserSkillStore{
			skills: []store.UserSkill{
				{
					ID:             "1",
					Name:           "stored-skill",
					Source:         "user",
					Executable:     true,
					ToolsAllowlist: []string{"tool_no_longer_present"},
					Version:        1,
				},
			},
		}
		reloader := &fakeAgentReloader{}
		srv := newSkillsTestServer(t, uss, reloader, knownTools)

		// reloadSkills must not return an error or panic for unknown stored tools.
		// (ReplaceExecutableSkills itself does the warn-and-drop for stored skills.)
		srv.reloadSkills(context.Background())

		if reloader.replaceExecutableSkillsCalls != 1 {
			t.Errorf("ReplaceExecutableSkills called %d times, want 1", reloader.replaceExecutableSkillsCalls)
		}
	})
}

// ---------------------------------------------------------------------------
// Task 3.15 — Integration: POST create → reload triggered
// ---------------------------------------------------------------------------

// TestCreateSkillTriggersReload verifies that after a successful POST /api/skills,
// reloadSkills is called (ReplaceExecutableSkills is invoked on the agent).
func TestCreateSkillTriggersReload(t *testing.T) {
	uss := &fakeUserSkillStore{}
	reloader := &fakeAgentReloader{}
	srv := newSkillsTestServer(t, uss, reloader, nil)

	w := postSkill(t, srv, map[string]any{
		"name":       "spawnable",
		"prose":      "run stuff",
		"executable": true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if reloader.replaceExecutableSkillsCalls == 0 {
		t.Error("expected ReplaceExecutableSkills to be called after successful create")
	}
}

// ---------------------------------------------------------------------------
// Task 3.16 — Integration: DELETE skill → reload triggered
// ---------------------------------------------------------------------------

// TestDeleteSkillTriggersReload verifies that after DELETE /api/skills/{name},
// reloadSkills is called (agent is updated to remove the skill).
func TestDeleteSkillTriggersReload(t *testing.T) {
	uss := &fakeUserSkillStore{
		skills: []store.UserSkill{
			{ID: "1", Name: "doomed-skill", Source: "user", Executable: true, Version: 1},
		},
	}
	reloader := &fakeAgentReloader{}
	srv := newSkillsTestServer(t, uss, reloader, nil)

	w := deleteSkill(t, srv, "doomed-skill")
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if reloader.replaceExecutableSkillsCalls == 0 {
		t.Error("expected ReplaceExecutableSkills to be called after successful delete")
	}
}

// ---------------------------------------------------------------------------
// Helpers — fakeTool (satisfies tool.Tool interface)
// ---------------------------------------------------------------------------

// fakeTool is a minimal tool.Tool implementation for testing allowlist validation.
type fakeTool struct {
	name string
}

func (f *fakeTool) Name() string            { return f.name }
func (f *fakeTool) Description() string     { return "fake tool for testing" }
func (f *fakeTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (f *fakeTool) Execute(_ context.Context, _ json.RawMessage) (tool.ToolResult, error) {
	return tool.ToolResult{Content: "ok"}, nil
}
