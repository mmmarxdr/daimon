package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"daimon/internal/agent"
	"daimon/internal/notify"
)

// ---------------------------------------------------------------------------
// Task 1.13 — handleSubagentCancel unit tests (REQ-17)
// ---------------------------------------------------------------------------

// cancelSubagentProvider is a SubagentProvider test double where CancelSubagent
// is controllable: the cancelFn field determines the return value.
type cancelSubagentProvider struct {
	cancelFn func(id string) error
}

func (c *cancelSubagentProvider) ActiveSubagents() []agent.SubagentStatus { return nil }
func (c *cancelSubagentProvider) SubagentBus() notify.Bus                  { return nil }
func (c *cancelSubagentProvider) CancelSubagent(id string) error {
	if c.cancelFn != nil {
		return c.cancelFn(id)
	}
	return nil
}

// newCancelTestServer creates a test server with the given SubagentProvider.
func newCancelTestServer(t *testing.T, prov SubagentProvider) *Server {
	t.Helper()
	s := &Server{
		deps: ServerDeps{
			Store:            &fakeWebStore{},
			Config:           minimalConfig(),
			SubagentProvider: prov,
		},
		mux:        http.NewServeMux(),
		wsUpgrader: newWSUpgrader(nil),
	}
	s.routes()
	return s
}

// TestHandleSubagentCancel_Success verifies 204 is returned when the subagent
// is running and cancel succeeds. (REQ-17)
func TestHandleSubagentCancel_Success(t *testing.T) {
	prov := &cancelSubagentProvider{
		cancelFn: func(id string) error { return nil },
	}
	srv := newCancelTestServer(t, prov)

	req := httptest.NewRequest(http.MethodPost, "/api/subagents/sub-123/cancel", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleSubagentCancel_UnknownID verifies 404 is returned when the
// subagent ID is not found. (REQ-17)
func TestHandleSubagentCancel_UnknownID(t *testing.T) {
	prov := &cancelSubagentProvider{
		cancelFn: func(id string) error {
			return errors.New("subagent \"" + id + "\" not found")
		},
	}
	srv := newCancelTestServer(t, prov)

	req := httptest.NewRequest(http.MethodPost, "/api/subagents/nope/cancel", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleSubagentCancel_AlreadyFinished verifies 200 + body is returned
// when the subagent has already completed. (REQ-17)
func TestHandleSubagentCancel_AlreadyFinished(t *testing.T) {
	prov := &cancelSubagentProvider{
		cancelFn: func(id string) error {
			return errAlreadyFinished
		},
	}
	srv := newCancelTestServer(t, prov)

	req := httptest.NewRequest(http.MethodPost, "/api/subagents/sub-done/cancel", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for already-finished, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]bool
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body["already_finished"] {
		t.Errorf("expected already_finished=true, got body: %v", body)
	}
}

// TestHandleSubagentCancel_NilProvider verifies 404 is returned when no
// SubagentProvider is wired. (REQ-17)
func TestHandleSubagentCancel_NilProvider(t *testing.T) {
	srv := newCancelTestServer(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/subagents/sub-123/cancel", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
