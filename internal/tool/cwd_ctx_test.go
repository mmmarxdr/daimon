//go:build !windows

package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"daimon/internal/config"
)

// ---------------------------------------------------------------------------
// WU6 RED tests: WithEffectiveCwd / EffectiveCwdFromCtx (REQ-4, REQ-5)
// ---------------------------------------------------------------------------

// TestWithEffectiveCwd_RoundTrip verifies that a path stored via WithEffectiveCwd
// is recovered by EffectiveCwdFromCtx.
func TestWithEffectiveCwd_RoundTrip(t *testing.T) {
	ctx := WithEffectiveCwd(context.Background(), "/home/agent/projects")
	got, ok := EffectiveCwdFromCtx(ctx)
	if !ok {
		t.Fatal("EffectiveCwdFromCtx returned ok=false, expected true")
	}
	if got != "/home/agent/projects" {
		t.Errorf("expected /home/agent/projects, got %q", got)
	}
}

// TestEffectiveCwdFromCtx_NoValue returns ok=false when no cwd is set.
func TestEffectiveCwdFromCtx_NoValue(t *testing.T) {
	_, ok := EffectiveCwdFromCtx(context.Background())
	if ok {
		t.Fatal("expected ok=false for bare context, got ok=true")
	}
}

// TestWithEffectiveCwd_EmptyString allows the empty string as a value —
// the shell tool treats "" as "not set" and falls back to config, but the
// ctx accessor itself must not refuse it.
func TestWithEffectiveCwd_EmptyString(t *testing.T) {
	ctx := WithEffectiveCwd(context.Background(), "")
	got, ok := EffectiveCwdFromCtx(ctx)
	if !ok {
		t.Fatal("expected ok=true even for empty string")
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// TestWithEffectiveCwd_InnerOverridesOuter verifies that inner context wins
// when WithEffectiveCwd is nested.
func TestWithEffectiveCwd_InnerOverridesOuter(t *testing.T) {
	outer := WithEffectiveCwd(context.Background(), "/outer")
	inner := WithEffectiveCwd(outer, "/inner")
	got, ok := EffectiveCwdFromCtx(inner)
	if !ok {
		t.Fatal("expected ok=true for nested ctx")
	}
	if got != "/inner" {
		t.Errorf("expected /inner, got %q", got)
	}
}

// TestShellTool_UsesCwdFromCtx verifies that the shell tool uses the effective
// cwd injected via context instead of the static config.WorkingDir.
func TestShellTool_UsesCwdFromCtx(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shell execution in short mode")
	}
	dir := t.TempDir()

	// Shell tool with allow_all and no static WorkingDir.
	st := NewShellTool(config.ShellToolConfig{Enabled: true, AllowAll: true})

	params, err := json.Marshal(shellParams{Command: "pwd"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Inject the temp dir as the effective cwd via context.
	ctx := WithEffectiveCwd(context.Background(), dir)
	res, err := st.Execute(ctx, params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("Execute returned error: %v", res.Content)
	}
	if !strings.Contains(res.Content, dir) {
		t.Errorf("expected output to contain %q, got %q", dir, res.Content)
	}
}

// TestShellTool_FallsBackToConfigCwd verifies that when no effective cwd is
// injected, the shell tool uses the static config.WorkingDir.
func TestShellTool_FallsBackToConfigCwd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping shell execution in short mode")
	}
	dir := t.TempDir()

	// Shell tool with static WorkingDir set, no ctx override.
	st := NewShellTool(config.ShellToolConfig{
		Enabled:    true,
		AllowAll:   true,
		WorkingDir: dir,
	})

	params, err := json.Marshal(shellParams{Command: "pwd"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	res, err := st.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("Execute returned error: %v", res.Content)
	}
	if !strings.Contains(res.Content, dir) {
		t.Errorf("expected output to contain config WorkingDir %q, got %q", dir, res.Content)
	}
}
