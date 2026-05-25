package agent

// modes_test.go — RED tests for Phase 1 of mode-system PR1.
//
// Tests cover:
//   - LookupMode: valid names (plan/build/review), unknown name, empty string
//   - ModeNames: order and contents
//   - filterAllowedTools: nil allowlist (all pass), empty (none pass), populated (subset)
//   - isToolAllowed: nil/empty/hit/miss
//
// REQs covered: REQ-3, REQ-7, REQ-8.
// These tests are written BEFORE the implementation (TDD RED step).

import (
	"errors"
	"testing"

	"daimon/internal/provider"
)

// ---------------------------------------------------------------------------
// LookupMode tests
// ---------------------------------------------------------------------------

func TestLookupMode_ValidNames(t *testing.T) {
	cases := []struct {
		name     string
		wantName string
	}{
		{"plan", "plan"},
		{"build", "build"},
		{"review", "review"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def, err := LookupMode(tc.name)
			if err != nil {
				t.Fatalf("LookupMode(%q) unexpected error: %v", tc.name, err)
			}
			if def.Name != tc.wantName {
				t.Errorf("def.Name = %q, want %q", def.Name, tc.wantName)
			}
		})
	}
}

func TestLookupMode_BuildHasNilAllowlist(t *testing.T) {
	def, err := LookupMode("build")
	if err != nil {
		t.Fatalf("LookupMode(\"build\") error: %v", err)
	}
	if def.ToolAllowlist != nil {
		t.Errorf("build mode ToolAllowlist should be nil (AllowAllTools), got %v", def.ToolAllowlist)
	}
}

func TestLookupMode_PlanHasNonEmptyAllowlist(t *testing.T) {
	def, err := LookupMode("plan")
	if err != nil {
		t.Fatalf("LookupMode(\"plan\") error: %v", err)
	}
	if len(def.ToolAllowlist) == 0 {
		t.Error("plan mode ToolAllowlist should be non-empty")
	}
}

func TestLookupMode_ReviewHasNonEmptyAllowlist(t *testing.T) {
	def, err := LookupMode("review")
	if err != nil {
		t.Fatalf("LookupMode(\"review\") error: %v", err)
	}
	if len(def.ToolAllowlist) == 0 {
		t.Error("review mode ToolAllowlist should be non-empty")
	}
}

func TestLookupMode_UnknownName_WrapsErrInvalidMode(t *testing.T) {
	_, err := LookupMode("banana")
	if err == nil {
		t.Fatal("LookupMode(\"banana\") expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidMode) {
		t.Errorf("expected errors.Is(err, ErrInvalidMode) = true, got false; err = %v", err)
	}
}

func TestLookupMode_EmptyString_WrapsErrInvalidMode(t *testing.T) {
	_, err := LookupMode("")
	if err == nil {
		t.Fatal("LookupMode(\"\") expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidMode) {
		t.Errorf("expected errors.Is(err, ErrInvalidMode) = true, got false; err = %v", err)
	}
}

func TestErrInvalidMode_ErrorString(t *testing.T) {
	// AD-11: exact error string is contract-locked.
	want := "invalid mode name (expected: plan, build, review)"
	if ErrInvalidMode.Error() != want {
		t.Errorf("ErrInvalidMode.Error() = %q, want %q", ErrInvalidMode.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// ModeNames tests
// ---------------------------------------------------------------------------

func TestModeNames_ContainsAllModes(t *testing.T) {
	names := ModeNames()
	wantSet := map[string]bool{"plan": true, "build": true, "review": true}
	for _, n := range names {
		if !wantSet[n] {
			t.Errorf("unexpected mode name %q in ModeNames()", n)
		}
		delete(wantSet, n)
	}
	for missing := range wantSet {
		t.Errorf("mode %q missing from ModeNames()", missing)
	}
}

func TestModeNames_StableOrder(t *testing.T) {
	// ModeNames must return the same stable order every call.
	first := ModeNames()
	second := ModeNames()
	if len(first) != len(second) {
		t.Fatalf("ModeNames() length differs between calls: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Errorf("ModeNames()[%d]: first call = %q, second call = %q", i, first[i], second[i])
		}
	}
}

func TestModeNames_DesignOrder(t *testing.T) {
	// Design specifies: []string{"build", "plan", "review"} (alphabetical).
	got := ModeNames()
	want := []string{"build", "plan", "review"}
	if len(got) != len(want) {
		t.Fatalf("len(ModeNames()) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ModeNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// ---------------------------------------------------------------------------
// filterAllowedTools tests
// ---------------------------------------------------------------------------

// makeDefs returns a slice of ToolDefinitions with the given names.
func makeDefs(names ...string) []provider.ToolDefinition {
	out := make([]provider.ToolDefinition, len(names))
	for i, n := range names {
		out[i] = provider.ToolDefinition{Name: n}
	}
	return out
}

func TestFilterAllowedTools_NilAllowlist_PassesAll(t *testing.T) {
	// nil allowlist = AllowAllTools: all tools pass through unchanged.
	defs := makeDefs("Read", "Edit", "Bash", "Write")
	got := filterAllowedTools(defs, nil)
	if len(got) != len(defs) {
		t.Fatalf("filterAllowedTools(defs, nil) len = %d, want %d", len(got), len(defs))
	}
}

func TestFilterAllowedTools_EmptyAllowlist_BlocksAll(t *testing.T) {
	// []string{} (non-nil but empty) = NONE allowed.
	// This is the OPPOSITE of filterParentTools semantics (discovery #413).
	defs := makeDefs("Read", "Edit", "Bash")
	got := filterAllowedTools(defs, []string{})
	if len(got) != 0 {
		t.Errorf("filterAllowedTools(defs, []string{}) should return empty, got %v", got)
	}
}

func TestFilterAllowedTools_PopulatedAllowlist_ReturnsSubset(t *testing.T) {
	defs := makeDefs("Read", "Edit", "Bash", "Write", "Grep")
	allowlist := []string{"Read", "Grep"}
	got := filterAllowedTools(defs, allowlist)
	if len(got) != 2 {
		t.Fatalf("filterAllowedTools len = %d, want 2; got %v", len(got), got)
	}
	names := map[string]bool{}
	for _, td := range got {
		names[td.Name] = true
	}
	if !names["Read"] {
		t.Error("expected \"Read\" in result")
	}
	if !names["Grep"] {
		t.Error("expected \"Grep\" in result")
	}
	if names["Edit"] || names["Bash"] || names["Write"] {
		t.Error("unexpected tool in result: only Read and Grep should be present")
	}
}

func TestFilterAllowedTools_NilDefs_NilAllowlist(t *testing.T) {
	// Edge case: nil defs with nil allowlist should return nil (or empty).
	got := filterAllowedTools(nil, nil)
	// nil is acceptable here since allowlist==nil means pass-through
	if got != nil {
		// accepting nil or empty slice
		if len(got) != 0 {
			t.Errorf("expected nil or empty slice, got %v", got)
		}
	}
}

func TestFilterAllowedTools_AllowlistEntryNotInDefs(t *testing.T) {
	// Allowlist names that don't exist in defs are simply not matched.
	defs := makeDefs("Read", "Edit")
	allowlist := []string{"Read", "NonExistent"}
	got := filterAllowedTools(defs, allowlist)
	if len(got) != 1 || got[0].Name != "Read" {
		t.Errorf("expected only [Read], got %v", got)
	}
}

// ---------------------------------------------------------------------------
// isToolAllowed tests
// ---------------------------------------------------------------------------

func TestIsToolAllowed_NilAllowlist_AlwaysTrue(t *testing.T) {
	if !isToolAllowed("AnyTool", nil) {
		t.Error("isToolAllowed(\"AnyTool\", nil) should return true")
	}
}

func TestIsToolAllowed_EmptyAllowlist_AlwaysFalse(t *testing.T) {
	if isToolAllowed("AnyTool", []string{}) {
		t.Error("isToolAllowed(\"AnyTool\", []string{}) should return false")
	}
}

func TestIsToolAllowed_HitInAllowlist_ReturnsTrue(t *testing.T) {
	if !isToolAllowed("Read", []string{"Read", "Grep"}) {
		t.Error("isToolAllowed(\"Read\", [\"Read\", \"Grep\"]) should return true")
	}
}

func TestIsToolAllowed_MissInAllowlist_ReturnsFalse(t *testing.T) {
	if isToolAllowed("Bash", []string{"Read", "Grep"}) {
		t.Error("isToolAllowed(\"Bash\", [\"Read\", \"Grep\"]) should return false")
	}
}

// ---------------------------------------------------------------------------
// Mode definitions content tests (REQ-6, REQ-7)
// ---------------------------------------------------------------------------

func TestPlanMode_SystemPromptNonEmpty(t *testing.T) {
	def, _ := LookupMode("plan")
	if def.SystemPrompt == "" {
		t.Error("plan mode SystemPrompt must not be empty")
	}
}

func TestBuildMode_SystemPromptEmpty(t *testing.T) {
	// Build mode: no extra system prompt (behavior is default, spec REQ-6 S6-2).
	def, _ := LookupMode("build")
	if def.SystemPrompt != "" {
		t.Errorf("build mode SystemPrompt should be empty, got %q", def.SystemPrompt)
	}
}

func TestReviewMode_SystemPromptNonEmpty(t *testing.T) {
	def, _ := LookupMode("review")
	if def.SystemPrompt == "" {
		t.Error("review mode SystemPrompt must not be empty")
	}
}

func TestPlanMode_DoesNotIncludeEdit(t *testing.T) {
	// REQ-7 S7-1: plan allowlist should not include "Edit".
	def, _ := LookupMode("plan")
	defs := makeDefs("Read", "Edit", "Bash", "Write")
	filtered := filterAllowedTools(defs, def.ToolAllowlist)
	for _, td := range filtered {
		if td.Name == "Edit" {
			t.Error("plan mode should not allow \"Edit\" tool")
		}
	}
}

func TestBuildMode_NilAllowlistPassesAll(t *testing.T) {
	// REQ-7 S7-2: build mode nil allowlist = all tools pass.
	def, _ := LookupMode("build")
	defs := makeDefs("Read", "Edit", "Bash", "Write", "Grep")
	filtered := filterAllowedTools(defs, def.ToolAllowlist)
	if len(filtered) != len(defs) {
		t.Errorf("build mode should pass all tools; got %d, want %d", len(filtered), len(defs))
	}
}

// ---------------------------------------------------------------------------
// ArgAllowlists data-model tests (C2: AD-1, REQ-7)
// ---------------------------------------------------------------------------

// TestModeDefinitionArgAllowlists asserts the ArgAllowlists field is wired
// correctly on all three mode definitions. Review gets the read-only git set;
// plan and build leave ArgAllowlists nil (no arg restriction).
func TestModeDefinitionArgAllowlists(t *testing.T) {
	wantShellExec := []string{"git diff", "git log", "git show", "git status", "git blame"}

	t.Run("review ArgAllowlists non-nil", func(t *testing.T) {
		def, err := LookupMode("review")
		if err != nil {
			t.Fatalf("LookupMode(review): %v", err)
		}
		if def.ArgAllowlists == nil {
			t.Fatal("review ArgAllowlists must not be nil")
		}
	})

	t.Run("review ArgAllowlists[shell_exec] exact", func(t *testing.T) {
		def, _ := LookupMode("review")
		got, ok := def.ArgAllowlists["shell_exec"]
		if !ok {
			t.Fatal("review ArgAllowlists must have entry for shell_exec")
		}
		if len(got) != len(wantShellExec) {
			t.Fatalf("ArgAllowlists[shell_exec] len=%d, want %d; got %v", len(got), len(wantShellExec), got)
		}
		for i, w := range wantShellExec {
			if got[i] != w {
				t.Errorf("ArgAllowlists[shell_exec][%d] = %q, want %q", i, got[i], w)
			}
		}
	})

	t.Run("plan ArgAllowlists nil", func(t *testing.T) {
		def, _ := LookupMode("plan")
		if def.ArgAllowlists != nil {
			t.Errorf("plan ArgAllowlists should be nil, got %v", def.ArgAllowlists)
		}
	})

	t.Run("build ArgAllowlists nil", func(t *testing.T) {
		def, _ := LookupMode("build")
		if def.ArgAllowlists != nil {
			t.Errorf("build ArgAllowlists should be nil, got %v", def.ArgAllowlists)
		}
	})
}
