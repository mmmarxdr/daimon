package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"daimon/internal/agent"
)

// ---------------------------------------------------------------------------
// fakeCommandProvider — mock for CommandProvider
// ---------------------------------------------------------------------------

// fakeCommandProvider is a programmable mock of CommandProvider for handler tests.
type fakeCommandProvider struct {
	commands   []agent.CommandInfo
	runReply   string
	runErr     error
	runCalled  bool
	runLastReq agent.RunCommandRequest
}

func (f *fakeCommandProvider) Commands() []agent.CommandInfo {
	return f.commands
}

func (f *fakeCommandProvider) RunCommand(_ context.Context, req agent.RunCommandRequest) (agent.CommandResult, error) {
	f.runCalled = true
	f.runLastReq = req
	if f.runErr != nil {
		return agent.CommandResult{}, f.runErr
	}
	return agent.CommandResult{Reply: f.runReply}, nil
}

// ---------------------------------------------------------------------------
// Helper — newCommandsTestServer
// ---------------------------------------------------------------------------

func newCommandsTestServer(t *testing.T, cp CommandProvider) *Server {
	t.Helper()
	s := &Server{
		deps: ServerDeps{
			Config:          minimalConfig(),
			CommandProvider: cp,
		},
		mux:        http.NewServeMux(),
		wsUpgrader: newWSUpgrader(nil),
	}
	s.routes()
	return s
}

// ---------------------------------------------------------------------------
// WU10 tests: GET /api/commands
// ---------------------------------------------------------------------------

func TestHandleListCommands_Returns200AndShape(t *testing.T) {
	cp := &fakeCommandProvider{
		commands: []agent.CommandInfo{
			{Name: "ping", Description: "Check if alive", Source: "builtin", Destructive: false},
			{Name: "reset", Description: "Clear conversation", Source: "builtin", Destructive: true},
		},
	}
	srv := newCommandsTestServer(t, cp)

	req := httptest.NewRequest(http.MethodGet, "/api/commands", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Commands []agent.CommandInfo `json:"commands"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(resp.Commands))
	}
}

func TestHandleListCommands_SourceTagsCorrect(t *testing.T) {
	cp := &fakeCommandProvider{
		commands: []agent.CommandInfo{
			{Name: "ping", Description: "builtin cmd", Source: "builtin", Destructive: false},
			{Name: "task-cancel", Description: "cron cmd", Source: "cron", Destructive: true},
			{Name: "researcher", Description: "skill cmd", Source: "skill", Destructive: false},
		},
	}
	srv := newCommandsTestServer(t, cp)

	req := httptest.NewRequest(http.MethodGet, "/api/commands", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Commands []agent.CommandInfo `json:"commands"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	byName := make(map[string]agent.CommandInfo)
	for _, c := range resp.Commands {
		byName[c.Name] = c
	}

	if byName["ping"].Source != "builtin" {
		t.Errorf("expected ping source=builtin, got %q", byName["ping"].Source)
	}
	if byName["task-cancel"].Source != "cron" {
		t.Errorf("expected task-cancel source=cron, got %q", byName["task-cancel"].Source)
	}
	if byName["researcher"].Source != "skill" {
		t.Errorf("expected researcher source=skill, got %q", byName["researcher"].Source)
	}
	if byName["ping"].Destructive {
		t.Error("expected ping to be non-destructive")
	}
	if !byName["task-cancel"].Destructive {
		t.Error("expected task-cancel to be destructive")
	}
}

func TestHandleListCommands_NilProvider_Returns503(t *testing.T) {
	srv := newCommandsTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/commands", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// WU10 tests: POST /api/commands/run
// ---------------------------------------------------------------------------

func TestHandleRunCommand_NonDestructiveHappyPath(t *testing.T) {
	cp := &fakeCommandProvider{
		commands: []agent.CommandInfo{
			{Name: "ping", Description: "Check alive", Source: "builtin", Destructive: false},
		},
		runReply: "pong — daimon is alive",
	}
	srv := newCommandsTestServer(t, cp)

	body := agent.RunCommandRequest{
		Name:      "ping",
		ChannelID: "web:u1",
		SenderID:  "u1",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/commands/run", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp agent.CommandResult
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Reply != "pong — daimon is alive" {
		t.Errorf("expected pong reply, got %q", resp.Reply)
	}
}

func TestHandleRunCommand_DestructiveWithoutFlag_Returns403(t *testing.T) {
	cp := &fakeCommandProvider{
		commands: []agent.CommandInfo{
			{Name: "reset", Description: "Clear conversation", Source: "builtin", Destructive: true},
		},
		runReply: "Conversation cleared.",
	}
	srv := newCommandsTestServer(t, cp)

	body := agent.RunCommandRequest{
		Name:             "reset",
		ChannelID:        "web:u1",
		SenderID:         "u1",
		AllowDestructive: false,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/commands/run", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	// Provider should NOT have been called
	if cp.runCalled {
		t.Error("expected RunCommand to not be called for destructive without flag")
	}
}

func TestHandleRunCommand_DestructiveWithFlag_Returns200(t *testing.T) {
	cp := &fakeCommandProvider{
		commands: []agent.CommandInfo{
			{Name: "reset", Description: "Clear conversation", Source: "builtin", Destructive: true},
		},
		runReply: "Conversation cleared.",
	}
	srv := newCommandsTestServer(t, cp)

	body := agent.RunCommandRequest{
		Name:             "reset",
		ChannelID:        "web:u1",
		SenderID:         "u1",
		AllowDestructive: true,
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/commands/run", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp agent.CommandResult
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if resp.Reply != "Conversation cleared." {
		t.Errorf("expected cleared reply, got %q", resp.Reply)
	}
}

func TestHandleRunCommand_UnknownCommand_Returns404(t *testing.T) {
	cp := &fakeCommandProvider{
		commands: []agent.CommandInfo{
			{Name: "ping", Description: "Check alive", Source: "builtin", Destructive: false},
		},
	}
	srv := newCommandsTestServer(t, cp)

	body := agent.RunCommandRequest{
		Name:      "nonexistent",
		ChannelID: "web:u1",
		SenderID:  "u1",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/commands/run", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunCommand_MalformedJSON_Returns400(t *testing.T) {
	cp := &fakeCommandProvider{
		commands: []agent.CommandInfo{
			{Name: "ping", Description: "Check alive", Source: "builtin", Destructive: false},
		},
	}
	srv := newCommandsTestServer(t, cp)

	req := httptest.NewRequest(http.MethodPost, "/api/commands/run", bytes.NewReader([]byte(`{not valid json`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunCommand_MissingName_Returns400(t *testing.T) {
	cp := &fakeCommandProvider{
		commands: []agent.CommandInfo{
			{Name: "ping", Description: "Check alive", Source: "builtin", Destructive: false},
		},
	}
	srv := newCommandsTestServer(t, cp)

	body := agent.RunCommandRequest{
		Name:      "",
		ChannelID: "web:u1",
		SenderID:  "u1",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/commands/run", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleRunCommand_NilProvider_Returns503(t *testing.T) {
	srv := newCommandsTestServer(t, nil)

	body := agent.RunCommandRequest{
		Name:      "ping",
		ChannelID: "web:u1",
		SenderID:  "u1",
	}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/commands/run", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
