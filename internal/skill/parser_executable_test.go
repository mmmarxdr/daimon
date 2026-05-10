package skill

import (
	"strings"
	"testing"
)

// buildSkillFile is a helper that wraps frontmatter in --- delimiters.
func buildSkillFile(frontmatter, body string) string {
	return "---\n" + frontmatter + "\n---\n" + body
}

// TestParseSkillContent_FullExecutableFrontmatter verifies that a skill with
// all executable fields parses correctly. Satisfies CONFIG-REQ-4.
func TestParseSkillContent_FullExecutableFrontmatter(t *testing.T) {
	content := buildSkillFile(`
name: researcher
executable: true
version: 1
model: claude-haiku-4-5
provider: anthropic
system_prompt_addendum: "Focus only on technical documentation."
tools_allowlist:
  - read_file
  - mcp.github.search_code
budget:
  max_cost_usd: 0.10
  max_turns: 10
  timeout_min: 5
`, "Researcher skill prose.")

	sc, _, errs := parseSkillContent("researcher.skill.md", content)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if sc.Model != "claude-haiku-4-5" {
		t.Errorf("Model: got %q, want %q", sc.Model, "claude-haiku-4-5")
	}
	if sc.ProviderName != "anthropic" {
		t.Errorf("ProviderName: got %q, want %q", sc.ProviderName, "anthropic")
	}
	if sc.SystemAddendum != "Focus only on technical documentation." {
		t.Errorf("SystemAddendum: got %q", sc.SystemAddendum)
	}
	if len(sc.ToolsAllowlist) != 2 || sc.ToolsAllowlist[0] != "read_file" || sc.ToolsAllowlist[1] != "mcp.github.search_code" {
		t.Errorf("ToolsAllowlist: got %v", sc.ToolsAllowlist)
	}
	if sc.Budget.MaxCostUSD != 0.10 {
		t.Errorf("Budget.MaxCostUSD: got %v, want 0.10", sc.Budget.MaxCostUSD)
	}
	if sc.Budget.MaxTurns != 10 {
		t.Errorf("Budget.MaxTurns: got %d, want 10", sc.Budget.MaxTurns)
	}
	if sc.Budget.TimeoutMin != 5 {
		t.Errorf("Budget.TimeoutMin: got %d, want 5", sc.Budget.TimeoutMin)
	}
	if sc.Version != 1 {
		t.Errorf("Version: got %d, want 1", sc.Version)
	}
	if !sc.Executable {
		t.Error("Executable: want true")
	}
}

// TestParseSkillContent_BudgetDefaultsShortcut verifies that `budget: defaults`
// expands to 0.50/20/10. Satisfies CONFIG-REQ-4.
func TestParseSkillContent_BudgetDefaultsShortcut(t *testing.T) {
	content := buildSkillFile(`
executable: true
budget: defaults
`, "Prose.")

	sc, _, errs := parseSkillContent("cheap.skill.md", content)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if sc.Budget.MaxCostUSD != 0.50 {
		t.Errorf("Budget.MaxCostUSD: got %v, want 0.50", sc.Budget.MaxCostUSD)
	}
	if sc.Budget.MaxTurns != 20 {
		t.Errorf("Budget.MaxTurns: got %d, want 20", sc.Budget.MaxTurns)
	}
	if sc.Budget.TimeoutMin != 10 {
		t.Errorf("Budget.TimeoutMin: got %d, want 10", sc.Budget.TimeoutMin)
	}
	if !sc.Budget.Defaults {
		t.Error("Budget.Defaults: want true when budget: defaults used")
	}
}

// TestParseSkillContent_InvalidBudgetLiteral verifies that `budget: random_value`
// is a parse error. Satisfies CONFIG-REQ-7.
func TestParseSkillContent_InvalidBudgetLiteral(t *testing.T) {
	content := buildSkillFile(`
executable: true
budget: random_value
`, "Prose.")

	_, _, errs := parseSkillContent("bad.skill.md", content)
	if len(errs) == 0 {
		t.Fatal("expected parse error for invalid budget literal, got none")
	}
	errMsg := errs[0].Error()
	if !strings.Contains(errMsg, "random_value") && !strings.Contains(errMsg, "budget") {
		t.Errorf("error should mention 'random_value' or 'budget', got: %q", errMsg)
	}
}

// TestParseSkillContent_ExecutableWithNoBudgetIsError verifies that
// `executable: true` without a budget block is a load error. Satisfies CONFIG-REQ-6.
func TestParseSkillContent_ExecutableWithNoBudgetIsError(t *testing.T) {
	content := buildSkillFile(`
executable: true
`, "Prose.")

	_, _, errs := parseSkillContent("nobud.skill.md", content)
	if len(errs) == 0 {
		t.Fatal("expected parse error for missing budget on executable skill, got none")
	}
	errMsg := errs[0].Error()
	if !strings.Contains(errMsg, "budget") {
		t.Errorf("error should mention 'budget', got: %q", errMsg)
	}
}

// TestParseSkillContent_VersionDefaultsToOne verifies that when version is absent,
// SkillContent.Version is 1. Satisfies CONFIG-REQ-4.
func TestParseSkillContent_VersionDefaultsToOne(t *testing.T) {
	// Non-executable skill — no version field.
	content := buildSkillFile(`
name: simple
`, "Prose.")

	sc, _, errs := parseSkillContent("simple.skill.md", content)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if sc.Version != 1 {
		t.Errorf("Version: got %d, want 1 (default)", sc.Version)
	}
}

// TestParseSkillContent_NonExecutableProducesNoExecutableDef verifies that
// a skill without executable:true has Executable=false. Satisfies CONFIG-REQ-8.
func TestParseSkillContent_NonExecutableProducesNoExecutableDef(t *testing.T) {
	content := buildSkillFile(`
name: prose-skill
executable: false
`, "Just some prose.")

	sc, _, errs := parseSkillContent("prose-skill.skill.md", content)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if sc.Executable {
		t.Error("Executable: want false for non-executable skill")
	}
}

// TestParseSkillContent_EmptyToolsAllowlistIsValid verifies that
// tools_allowlist: [] is valid and produces an empty (non-nil) slice.
// Satisfies CONFIG-REQ-4 empty-allowlist case.
func TestParseSkillContent_EmptyToolsAllowlistIsValid(t *testing.T) {
	content := buildSkillFile(`
executable: true
budget: defaults
tools_allowlist: []
`, "Prose.")

	sc, _, errs := parseSkillContent("nolist.skill.md", content)
	if len(errs) != 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if sc.ToolsAllowlist == nil {
		t.Error("ToolsAllowlist: want non-nil empty slice, got nil")
	}
	if len(sc.ToolsAllowlist) != 0 {
		t.Errorf("ToolsAllowlist: want empty, got %v", sc.ToolsAllowlist)
	}
}

// TestParseSkillContent_BackwardCompatNoNewKeys verifies that an existing
// skill file without any new keys loads with zero warnings. Satisfies CONFIG-REQ-8.
func TestParseSkillContent_BackwardCompatNoNewKeys(t *testing.T) {
	content := buildSkillFile(`
name: summarizer
description: Summarizes documents.
autoload: true
`, "This skill summarizes things.")

	sc, _, errs := parseSkillContent("summarizer.skill.md", content)
	if len(errs) != 0 {
		t.Errorf("unexpected warnings/errors for non-executable skill: %v", errs)
	}
	if sc.Executable {
		t.Error("Executable must be false for backward-compat skill")
	}
	if sc.Budget.MaxCostUSD != 0 {
		t.Errorf("Budget must be zero for non-executable skill, got %v", sc.Budget.MaxCostUSD)
	}
}
