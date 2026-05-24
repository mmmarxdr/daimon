package agent

// hot_reload_skill_commands_test.go — WU9 RED tests for skill-command rotation
// in ReplaceExecutableSkills (REQ-18).
//
// Tests cover:
//   (a) ReplaceExecutableSkills registers new skill commands after empty initial state
//   (b) ReplaceExecutableSkills removes old skill commands not in new defs
//   (c) collision with builtin/cron is preserved across reload
//   (d) race-clean under concurrent ReplaceExecutableSkills + command Lookup

import (
	"sync"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/skill"
)

// newAgentForHotReloadSkillCommands builds a minimal *Agent suitable for WU9
// tests. Unlike newAgentForHotReload (which skips New()), this uses the full
// New() constructor so built-in commands are registered and commands.mu is
// properly initialised.
func newAgentForHotReloadSkillCommands(t *testing.T) *Agent {
	t.Helper()
	return New(
		config.AgentConfig{},
		config.LimitsConfig{},
		config.FilterConfig{},
		&mockChannel{},
		&mockProvider{},
		&mockStore{},
		audit.NoopAuditor{},
		nil,
		nil,
		skill.SkillIndex{},
		4,
		false,
	)
}

// ---------------------------------------------------------------------------
// (a) ReplaceExecutableSkills registers new skill commands after empty initial state
// ---------------------------------------------------------------------------

// TestReplaceExecutableSkills_RegistersSkillCommands verifies that calling
// ReplaceExecutableSkills with new defs auto-mounts those defs as slash commands
// even when no commands were previously registered as skills.
func TestReplaceExecutableSkills_RegistersSkillCommands(t *testing.T) {
	a := newAgentForHotReloadSkillCommands(t)

	defs := []skill.ExecutableSkillDef{
		{Name: "researcher", Description: "Research assistant"},
		{Name: "code-review", Description: "Code review"},
	}
	a.ReplaceExecutableSkills(defs)

	if _, found := a.commands.Lookup("researcher"); !found {
		t.Error("expected /researcher to be registered after ReplaceExecutableSkills")
	}
	if _, found := a.commands.Lookup("code_review"); !found {
		t.Error("expected /code_review (hyphen→underscore) to be registered after ReplaceExecutableSkills")
	}

	// Verify source tags.
	for _, e := range a.commands.EntriesWithSource() {
		if e.Name == "researcher" || e.Name == "code_review" {
			if e.Source != SourceSkill {
				t.Errorf("expected source=%q for /%s, got %q", SourceSkill, e.Name, e.Source)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// (b) ReplaceExecutableSkills removes old skill commands not in new defs
// ---------------------------------------------------------------------------

// TestReplaceExecutableSkills_RemovesOldSkillCommands verifies that skill
// commands registered by a previous ReplaceExecutableSkills call are removed
// when they are absent from the new defs.
func TestReplaceExecutableSkills_RemovesOldSkillCommands(t *testing.T) {
	a := newAgentForHotReloadSkillCommands(t)

	// First call: register "old-skill".
	oldDefs := []skill.ExecutableSkillDef{
		{Name: "old-skill", Description: "Old skill"},
	}
	a.ReplaceExecutableSkills(oldDefs)

	if _, found := a.commands.Lookup("old_skill"); !found {
		t.Fatal("precondition: expected /old_skill to be registered after first ReplaceExecutableSkills")
	}

	// Second call: replace with "new-skill" only.
	newDefs := []skill.ExecutableSkillDef{
		{Name: "new-skill", Description: "New skill"},
	}
	a.ReplaceExecutableSkills(newDefs)

	// Old skill command must be gone.
	if _, found := a.commands.Lookup("old_skill"); found {
		t.Error("expected /old_skill to be removed after ReplaceExecutableSkills with new defs")
	}

	// New skill command must be present.
	if _, found := a.commands.Lookup("new_skill"); !found {
		t.Error("expected /new_skill to be registered after ReplaceExecutableSkills")
	}
}

// TestReplaceExecutableSkills_EmptyDefs_RemovesAllSkillCommands verifies that
// passing empty defs removes all previously mounted skill commands.
func TestReplaceExecutableSkills_EmptyDefs_RemovesAllSkillCommands(t *testing.T) {
	a := newAgentForHotReloadSkillCommands(t)

	// Register two skills.
	a.ReplaceExecutableSkills([]skill.ExecutableSkillDef{
		{Name: "skill-a", Description: "A"},
		{Name: "skill-b", Description: "B"},
	})

	// Verify they are mounted.
	if _, found := a.commands.Lookup("skill_a"); !found {
		t.Fatal("precondition: /skill_a not registered")
	}
	if _, found := a.commands.Lookup("skill_b"); !found {
		t.Fatal("precondition: /skill_b not registered")
	}

	// Replace with empty slice.
	a.ReplaceExecutableSkills(nil)

	// Both skill commands must be gone.
	if _, found := a.commands.Lookup("skill_a"); found {
		t.Error("expected /skill_a to be removed after empty ReplaceExecutableSkills")
	}
	if _, found := a.commands.Lookup("skill_b"); found {
		t.Error("expected /skill_b to be removed after empty ReplaceExecutableSkills")
	}
}

// ---------------------------------------------------------------------------
// (c) collision with builtin/cron is preserved across reload
// ---------------------------------------------------------------------------

// TestReplaceExecutableSkills_BuiltinPreservedAcrossReload verifies that a
// builtin command is not shadowed by a skill that shares its normalized name
// across a hot-reload cycle.
func TestReplaceExecutableSkills_BuiltinPreservedAcrossReload(t *testing.T) {
	a := newAgentForHotReloadSkillCommands(t)

	// "help" is a builtin registered in New().
	// Attempt to mount a skill named "help" via hot-reload.
	a.ReplaceExecutableSkills([]skill.ExecutableSkillDef{
		{Name: "help", Description: "Skill that clashes with /help"},
	})

	// The /help command must still be a builtin.
	entries := a.commands.EntriesWithSource()
	var src string
	for _, e := range entries {
		if e.Name == "help" {
			src = e.Source
			break
		}
	}
	if src != SourceBuiltin {
		t.Errorf("expected /help to remain %q after hot-reload with colliding skill, got %q", SourceBuiltin, src)
	}
}

// TestReplaceExecutableSkills_CronPreservedAcrossReload verifies that a cron
// command is not replaced by a skill with the same name during hot-reload.
func TestReplaceExecutableSkills_CronPreservedAcrossReload(t *testing.T) {
	a := newAgentForHotReloadSkillCommands(t)

	// Register a cron command manually.
	a.commands.Register("mytask", "Cron my-task", func(cc CommandContext) error {
		return nil
	}, SourceCron)

	// Hot-reload with a skill that would collide.
	a.ReplaceExecutableSkills([]skill.ExecutableSkillDef{
		{Name: "mytask", Description: "Skill mytask"},
	})

	// Cron command must survive.
	entries := a.commands.EntriesWithSource()
	var src string
	for _, e := range entries {
		if e.Name == "mytask" {
			src = e.Source
			break
		}
	}
	if src != SourceCron {
		t.Errorf("expected /mytask to remain %q after hot-reload, got %q", SourceCron, src)
	}
}

// ---------------------------------------------------------------------------
// (d) race-clean: concurrent ReplaceExecutableSkills + processMessage Lookup
// ---------------------------------------------------------------------------

// TestReplaceExecutableSkills_RaceSafe verifies that concurrent
// ReplaceExecutableSkills calls alongside concurrent command Lookups do not
// trigger a data race. Run with: go test -race ./internal/agent/...
func TestReplaceExecutableSkills_RaceSafe(t *testing.T) {
	a := newAgentForHotReloadSkillCommands(t)

	// Seed initial skills.
	a.ReplaceExecutableSkills([]skill.ExecutableSkillDef{
		{Name: "skill-x", Description: "X"},
	})

	const goroutines = 8
	const iterations = 50

	var wg sync.WaitGroup

	// Writers: alternate between two def sets.
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if j%2 == 0 {
					a.ReplaceExecutableSkills([]skill.ExecutableSkillDef{
						{Name: "skill-x", Description: "X"},
					})
				} else {
					a.ReplaceExecutableSkills([]skill.ExecutableSkillDef{
						{Name: "skill-y", Description: "Y"},
					})
				}
			}
		}(i)
	}

	// Readers: concurrent Lookup operations (simulating processMessage dispatch).
	for i := 0; i < goroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// These lookups may or may not find the commands — we only care
				// that there is no data race.
				_ = a.commands.Names()
				_, _ = a.commands.Lookup("skill_x")
				_, _ = a.commands.Lookup("skill_y")
				_ = a.commands.EntriesWithSource()
			}
		}()
	}

	wg.Wait()
}
