package skill

import (
	"os"
	"path/filepath"
	"testing"

	"daimon/internal/config"
)

// TestLoadSkills_ExecutableSkillProducesExecDef verifies that an executable
// skill produces an ExecutableSkillDef. Satisfies CONFIG-REQ-8, CONFIG-REQ-4.
func TestLoadSkills_ExecutableSkillProducesExecDef(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
		return p
	}

	execPath := write("researcher.skill.md", `---
name: researcher
executable: true
model: claude-haiku-4-5
provider: anthropic
budget: defaults
tools_allowlist:
  - shell_exec
---
Research skill prose.
`)
	nonExecPath := write("summarizer.skill.md", `---
name: summarizer
autoload: true
---
Summarizer prose.
`)

	contents, tools, execDefs, warns := LoadSkills(
		[]string{execPath, nonExecPath},
		config.ShellToolConfig{},
		config.LimitsConfig{},
	)
	_ = tools

	// Verify no fatal errors.
	for _, w := range warns {
		t.Logf("warn: %v", w)
	}

	// ExecutableSkillDef slice should have exactly one entry.
	if len(execDefs) != 1 {
		t.Fatalf("expected 1 ExecutableSkillDef, got %d", len(execDefs))
	}
	def := execDefs[0]
	if def.Name != "researcher" {
		t.Errorf("Name: got %q, want %q", def.Name, "researcher")
	}
	if def.Model != "claude-haiku-4-5" {
		t.Errorf("Model: got %q, want %q", def.Model, "claude-haiku-4-5")
	}
	if def.Budget.MaxCostUSD != 0.50 {
		t.Errorf("Budget.MaxCostUSD: got %v, want 0.50", def.Budget.MaxCostUSD)
	}

	// SkillContent slice should contain both skills.
	if len(contents) != 2 {
		t.Errorf("expected 2 SkillContent entries, got %d", len(contents))
	}
}

// TestLoadSkills_NonExecutableProducesNoDef verifies that a non-executable
// skill does not appear in the ExecutableSkillDef slice. Satisfies CONFIG-REQ-8.
func TestLoadSkills_NonExecutableProducesNoDef(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "prose.skill.md")
	if err := os.WriteFile(p, []byte(`---
name: prose
---
Just prose.
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, execDefs, _ := LoadSkills([]string{p}, config.ShellToolConfig{}, config.LimitsConfig{})
	if len(execDefs) != 0 {
		t.Errorf("expected 0 ExecutableSkillDef for non-executable skill, got %d", len(execDefs))
	}
}

// TestLoadSkills_FourReturnValueSignatureCompiles verifies that the 4-return
// signature compiles correctly (compile-time test).
func TestLoadSkills_FourReturnValueSignatureCompiles(t *testing.T) {
	contents, tools, defs, warns := LoadSkills(nil, config.ShellToolConfig{}, config.LimitsConfig{})
	// All return values should be nil/empty for no paths.
	if contents != nil || tools != nil || defs != nil || warns != nil {
		t.Error("LoadSkills with no paths should return all nil")
	}
}

// TestLoadSkills_ExistingCallSiteBehaviorUnchanged verifies that the
// 4-return signature produces the same SkillContent slice as before.
func TestLoadSkills_ExistingCallSiteBehaviorUnchanged(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "old.skill.md")
	if err := os.WriteFile(p, []byte(`---
name: old-skill
autoload: true
---
Old skill prose here.
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	contents, _, _, warns := LoadSkills([]string{p}, config.ShellToolConfig{}, config.LimitsConfig{})
	for _, w := range warns {
		t.Logf("warn: %v", w)
	}
	if len(contents) != 1 {
		t.Fatalf("expected 1 SkillContent, got %d", len(contents))
	}
	if contents[0].Name != "old-skill" {
		t.Errorf("Name: got %q, want %q", contents[0].Name, "old-skill")
	}
	if contents[0].Prose != "Old skill prose here." {
		t.Errorf("Prose: got %q", contents[0].Prose)
	}
}
