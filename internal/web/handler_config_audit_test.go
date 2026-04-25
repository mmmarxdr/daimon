package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"daimon/internal/config"
)

// TestHandlePutConfig_Audit_AcceptsSubtree verifies that PUT /api/config
// accepts the audit sub-tree and the values persist (regression: same bug
// class as the rag-hyde T16 — patchBody allow-list previously dropped audit).
func TestHandlePutConfig_Audit_AcceptsSubtree(t *testing.T) {
	cfg := minimalConfig()
	cfg.Web.AuthToken = "secret-token"

	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	s := newConfigTestServer(cfg, cfgPath)

	body := []byte(`{"audit":{"enabled":true,"type":"sqlite","path":"/tmp/test-audit"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader("secret-token"))
	rec := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	got := s.deps.Config.Audit
	if !config.BoolVal(got.Enabled) {
		t.Error("Audit.Enabled: want true, got false/nil")
	}
	if got.Type != "sqlite" {
		t.Errorf("Audit.Type: want %q, got %q", "sqlite", got.Type)
	}
	if got.Path != "/tmp/test-audit" {
		t.Errorf("Audit.Path: want %q, got %q", "/tmp/test-audit", got.Path)
	}
}

// TestHandlePutConfig_Audit_ExplicitDisablePersists verifies the *bool shape:
// when the user toggles audit OFF via the UI, the explicit `false` survives
// the PUT and is not overwritten by ApplyDefaults on the next reload.
func TestHandlePutConfig_Audit_ExplicitDisablePersists(t *testing.T) {
	cfg := minimalConfig()
	cfg.Web.AuthToken = "secret-token"
	enabled := true
	cfg.Audit = config.AuditConfig{Enabled: &enabled, Type: "sqlite", Path: "/tmp/audit"}

	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	s := newConfigTestServer(cfg, cfgPath)

	body := []byte(`{"audit":{"enabled":false}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader("secret-token"))
	rec := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	got := s.deps.Config.Audit
	if got.Enabled == nil {
		t.Fatal("Audit.Enabled: want non-nil pointer to false, got nil")
	}
	if *got.Enabled {
		t.Error("Audit.Enabled: want false (explicit opt-out), got true")
	}
	// Sibling fields must be preserved.
	if got.Type != "sqlite" {
		t.Errorf("Audit.Type: want preserved %q, got %q", "sqlite", got.Type)
	}
	if got.Path != "/tmp/audit" {
		t.Errorf("Audit.Path: want preserved %q, got %q", "/tmp/audit", got.Path)
	}
}

// TestHandlePutConfig_Audit_AbsentKeyPreserves verifies that PUT bodies that
// don't include the audit subtree leave the stored audit config untouched.
func TestHandlePutConfig_Audit_AbsentKeyPreserves(t *testing.T) {
	cfg := minimalConfig()
	cfg.Web.AuthToken = "secret-token"
	enabled := true
	cfg.Audit = config.AuditConfig{Enabled: &enabled, Type: "sqlite", Path: "/tmp/audit"}

	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	s := newConfigTestServer(cfg, cfgPath)

	body := []byte(`{"providers":{"anthropic":{"api_key":"sk-ant-new"}}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader("secret-token"))
	rec := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	got := s.deps.Config.Audit
	if !config.BoolVal(got.Enabled) {
		t.Error("Audit.Enabled: want preserved true, got false/nil")
	}
	if got.Type != "sqlite" {
		t.Errorf("Audit.Type: want preserved %q, got %q", "sqlite", got.Type)
	}
	if got.Path != "/tmp/audit" {
		t.Errorf("Audit.Path: want preserved %q, got %q", "/tmp/audit", got.Path)
	}

	// Also verify the response body does not omit the audit field on read-back.
	var respCfg config.Config
	if err := json.NewDecoder(rec.Body).Decode(&respCfg); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !config.BoolVal(respCfg.Audit.Enabled) {
		t.Error("response Audit.Enabled: want true, got false/nil")
	}
}
