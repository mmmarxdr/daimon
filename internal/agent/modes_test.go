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
	"encoding/json"
	"errors"
	"strings"
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

// ---------------------------------------------------------------------------
// Bug #438 regression: reviewAllowlist must reference real tool name (C3)
// ---------------------------------------------------------------------------

// TestReviewAllowlistUsesRealToolName asserts that reviewAllowlist contains
// "shell_exec" (the real registered name) and does NOT contain "Bash" (the
// dead alias that caused bug #438). Must fail RED before the fix.
func TestReviewAllowlistUsesRealToolName(t *testing.T) {
	def, err := LookupMode("review")
	if err != nil {
		t.Fatalf("LookupMode(review): %v", err)
	}

	hasShellExec := false
	hasBash := false
	for _, name := range def.ToolAllowlist {
		if name == "shell_exec" {
			hasShellExec = true
		}
		if name == "Bash" {
			hasBash = true
		}
	}
	if !hasShellExec {
		t.Error("reviewAllowlist must contain \"shell_exec\" — the real registered shell tool name")
	}
	if hasBash {
		t.Error("reviewAllowlist must NOT contain \"Bash\" — that name matches no registered tool (bug #438)")
	}
}

// TestReviewPromptSyncWithAllowlist asserts the review-mode system prompt lists
// exactly the five allowed git commands and does not reference forbidden ones.
func TestReviewPromptSyncWithAllowlist(t *testing.T) {
	def, _ := LookupMode("review")
	prompt := def.SystemPrompt

	required := []string{"git diff", "git log", "git show", "git status", "git blame"}
	for _, cmd := range required {
		if !strings.Contains(prompt, cmd) {
			t.Errorf("reviewPrompt missing %q — must list all allowed commands", cmd)
		}
	}

	forbidden := []string{"git commit", "git push", "git rebase", "git checkout", "git reset"}
	for _, cmd := range forbidden {
		if strings.Contains(prompt, cmd) {
			t.Errorf("reviewPrompt must NOT reference %q (not in allowlist)", cmd)
		}
	}
}

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

// ---------------------------------------------------------------------------
// isArgAllowed tests (C4: AD-2, AD-4, AD-7, REQ-2–5, REQ-7)
// ---------------------------------------------------------------------------

// reviewDef returns the review-mode ModeDefinition for isArgAllowed tests.
func reviewDef(t *testing.T) ModeDefinition {
	t.Helper()
	def, err := LookupMode("review")
	if err != nil {
		t.Fatalf("LookupMode(review): %v", err)
	}
	return def
}

// planDef returns the plan-mode ModeDefinition for nil-allowlist passthrough tests.
func planDef(t *testing.T) ModeDefinition {
	t.Helper()
	def, err := LookupMode("plan")
	if err != nil {
		t.Fatalf("LookupMode(plan): %v", err)
	}
	return def
}

// shellInput produces a JSON-encoded {"command": cmd} for isArgAllowed tests.
// Uses encoding/json for correct escaping of all characters, including control
// chars (e.g. \n, \r used in metachar test cases).
func shellInput(cmd string) []byte {
	b, _ := json.Marshal(map[string]string{"command": cmd})
	return b
}

// TestIsArgAllowed exercises the isArgAllowed gate with the full scenario matrix
// from the spec (REQ-2 through REQ-5, REQ-7) and design (AD-2, AD-4, AD-7).
func TestIsArgAllowed(t *testing.T) {
	rev := reviewDef(t)
	plan := planDef(t)

	// Helper: a review-mode def but with Read registered instead of shell_exec.
	revReadDef := ModeDefinition{
		Name:          "review",
		ToolAllowlist: rev.ToolAllowlist,
		ArgAllowlists: rev.ArgAllowlists,
	}

	cases := []struct {
		name      string
		toolName  string
		rawParams []byte
		def       ModeDefinition
		wantOK    bool
		wantMsg   string // non-empty only when wantOK=false
	}{
		// --- Allowed (ok=true, REQ-2) ---
		{
			name:     "git diff allowed",
			toolName: "shell_exec", rawParams: shellInput("git diff HEAD"),
			def: rev, wantOK: true,
		},
		{
			name:     "git log allowed",
			toolName: "shell_exec", rawParams: shellInput("git log --oneline -10"),
			def: rev, wantOK: true,
		},
		{
			name:     "git show allowed",
			toolName: "shell_exec", rawParams: shellInput("git show HEAD:file.go"),
			def: rev, wantOK: true,
		},
		{
			name:     "git status allowed",
			toolName: "shell_exec", rawParams: shellInput("git status --short"),
			def: rev, wantOK: true,
		},
		{
			name:     "git blame allowed",
			toolName: "shell_exec", rawParams: shellInput("git blame -L 100,120 f.go"),
			def: rev, wantOK: true,
		},
		{
			name:     "git diff double-space (AD-4: collapse whitespace)",
			toolName: "shell_exec", rawParams: shellInput("git  diff"),
			def: rev, wantOK: true,
		},
		{
			name:     "git status padded (AD-4: trim and collapse)",
			toolName: "shell_exec", rawParams: shellInput("  git status  "),
			def: rev, wantOK: true,
		},

		// --- Rejected non-match (ok=false, REQ-3) ---
		{
			name:     "git commit rejected",
			toolName: "shell_exec", rawParams: shellInput("git commit -m 'wip'"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "git push rejected",
			toolName: "shell_exec", rawParams: shellInput("git push origin main"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "git checkout rejected",
			toolName: "shell_exec", rawParams: shellInput("git checkout -b x"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "git reset rejected",
			toolName: "shell_exec", rawParams: shellInput("git reset --hard HEAD~1"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "git rebase rejected",
			toolName: "shell_exec", rawParams: shellInput("git rebase -i HEAD~3"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "rm rejected",
			toolName: "shell_exec", rawParams: shellInput("rm -rf /tmp"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "curl rejected",
			toolName: "shell_exec", rawParams: shellInput("curl https://example.com"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "cat rejected",
			toolName: "shell_exec", rawParams: shellInput("cat /etc/passwd"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "bare git rejected (REQ-3.9)",
			toolName: "shell_exec", rawParams: shellInput("git"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "empty command rejected (REQ-3.10)",
			toolName: "shell_exec", rawParams: shellInput(""),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "env-prefix injection rejected by allowlist (REQ-4 note)",
			toolName: "shell_exec", rawParams: shellInput("GIT_SSH_COMMAND=evil git log"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},

		// --- Rejected metachar (ok=false, REQ-4) — checked BEFORE allowlist ---
		{
			name:     "semicolon metachar",
			toolName: "shell_exec", rawParams: shellInput("git log; rm -rf /"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "double-ampersand metachar",
			toolName: "shell_exec", rawParams: shellInput("git diff && curl https://evil.com"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "pipe metachar",
			toolName: "shell_exec", rawParams: shellInput("git log | xargs rm"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "dollar-paren metachar",
			toolName: "shell_exec", rawParams: shellInput("git log $(rm -rf /)"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "backtick metachar",
			toolName: "shell_exec", rawParams: shellInput("git log `id`"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "newline metachar",
			toolName: "shell_exec", rawParams: shellInput("git diff\nrm -rf /"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},

		// --- Nil-allowlist passthrough (ok=true, REQ-7) ---
		{
			name:     "plan mode nil ArgAllowlists passthrough",
			toolName: "shell_exec", rawParams: shellInput("git commit -m x"),
			def: plan, wantOK: true,
		},
		{
			name:     "review mode no entry for Read tool passthrough",
			toolName: "Read", rawParams: []byte(`{"file_path":"/etc/passwd"}`),
			def: revReadDef, wantOK: true,
		},

		// --- Malformed params (ok=false, fail-closed) ---
		{
			name:     "malformed JSON fail-closed",
			toolName: "shell_exec", rawParams: []byte("not-json"),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},
		{
			name:     "missing command field fail-closed",
			toolName: "shell_exec", rawParams: []byte(`{"other":"value"}`),
			def: rev, wantOK: false, wantMsg: reviewShellRejectMsg,
		},

		// --- REQ-5: exact rejection message ---
		{
			name:      "exact rejection message for git commit",
			toolName:  "shell_exec",
			rawParams: shellInput("git commit -m 'x'"),
			def:       rev,
			wantOK:    false,
			wantMsg:   "command not allowed in review mode: only read-only git commands are permitted (git diff, git log, git show, git status, git blame)",
		},

		// --- FIX 1: --output file-write bypass (security, adversarial-review finding) ---
		// git {diff,log,show} --output=<file> silently writes/overwrites the target
		// file — violates the read-only contract in review mode. Both forms must be
		// rejected: --output=FILE (inline value) and --output FILE (space-separated).
		// NOTE: the short flag -o is NOT blocked (git rejects it as ambiguous; not a
		// write vector). Common read-only flags like --oneline, --stat, --name-only,
		// --numstat MUST still pass (regression cases immediately below).
		{
			name:      "--output=FILE rejected (git diff)",
			toolName:  "shell_exec",
			rawParams: shellInput("git diff --output=/tmp/x"),
			def:       rev,
			wantOK:    false,
			wantMsg:   reviewShellRejectMsg,
		},
		{
			name:      "--output FILE rejected (space form, git diff)",
			toolName:  "shell_exec",
			rawParams: shellInput("git diff --output /tmp/x"),
			def:       rev,
			wantOK:    false,
			wantMsg:   reviewShellRejectMsg,
		},
		{
			name:      "--output=FILE rejected (git log)",
			toolName:  "shell_exec",
			rawParams: shellInput("git log --output=/tmp/x"),
			def:       rev,
			wantOK:    false,
			wantMsg:   reviewShellRejectMsg,
		},
		{
			name:      "--output=FILE rejected (git show)",
			toolName:  "shell_exec",
			rawParams: shellInput("git show --output=/tmp/x"),
			def:       rev,
			wantOK:    false,
			wantMsg:   reviewShellRejectMsg,
		},

		// --- Regression: FIX 1 must NOT over-block legitimate read-only flags ---
		{
			name:      "git log --oneline still allowed (no over-block)",
			toolName:  "shell_exec",
			rawParams: shellInput("git log --oneline"),
			def:       rev,
			wantOK:    true,
		},
		{
			name:      "git diff --stat still allowed (no over-block)",
			toolName:  "shell_exec",
			rawParams: shellInput("git diff --stat"),
			def:       rev,
			wantOK:    true,
		},
		{
			name:      "git show --name-only still allowed (no over-block)",
			toolName:  "shell_exec",
			rawParams: shellInput("git show --name-only"),
			def:       rev,
			wantOK:    true,
		},
		{
			name:      "git diff --numstat still allowed (no over-block)",
			toolName:  "shell_exec",
			rawParams: shellInput("git diff --numstat"),
			def:       rev,
			wantOK:    true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := isArgAllowed(tc.toolName, tc.rawParams, tc.def)
			if ok != tc.wantOK {
				t.Errorf("isArgAllowed(%q, ...) ok=%v, want %v", tc.toolName, ok, tc.wantOK)
			}
			if !tc.wantOK && reason != tc.wantMsg {
				t.Errorf("isArgAllowed reason = %q, want %q", reason, tc.wantMsg)
			}
			if tc.wantOK && reason != "" {
				t.Errorf("isArgAllowed allowed but returned non-empty reason %q", reason)
			}
		})
	}
}
