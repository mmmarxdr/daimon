package agent

// commands_help_grouped_test.go — WU11.2 (RED) grouped /help output tests.
//
// After WU11.4 implements grouped rendering, cmdHelp should output three sections:
//   - "Built-in commands:"  for source=builtin
//   - "Cron commands:"      for source=cron
//   - "Skill commands:"     for source=skill
//
// Commands within each section appear in alphabetical order.
// Existing TestCmdHelp (substring-only) continues to pass.

import (
	"context"
	"strings"
	"testing"
)

// TestCmdHelp_GroupedOutput_SectionHeadersPresent verifies that when commands
// from all three sources exist, the reply contains the three section headers.
func TestCmdHelp_GroupedOutput_SectionHeadersPresent(t *testing.T) {
	cr := &capturedReply{}
	reg := NewCommandRegistry()
	reg.Register("ping", "check alive", func(cc CommandContext) error { return nil }, SourceBuiltin)
	reg.Register("task-cancel", "cancel a task", func(cc CommandContext) error { return nil }, SourceCron)
	reg.Register("researcher", "research subagent", func(cc CommandContext) error { return nil }, SourceSkill)

	cc := CommandContext{
		Ctx:      context.Background(),
		Reply:    cr.reply,
		Registry: reg,
	}
	if err := cmdHelp(cc); err != nil {
		t.Fatalf("cmdHelp returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]

	for _, header := range []string{"Built-in commands:", "Cron commands:", "Skill commands:"} {
		if !strings.Contains(reply, header) {
			t.Errorf("expected reply to contain section header %q, got:\n%s", header, reply)
		}
	}
}

// TestCmdHelp_GroupedOutput_CommandsInCorrectSection verifies that each command
// appears under the correct source section.
func TestCmdHelp_GroupedOutput_CommandsInCorrectSection(t *testing.T) {
	cr := &capturedReply{}
	reg := NewCommandRegistry()
	reg.Register("ping", "builtin cmd", func(cc CommandContext) error { return nil }, SourceBuiltin)
	reg.Register("task-cancel", "cron cmd", func(cc CommandContext) error { return nil }, SourceCron)
	reg.Register("researcher", "skill cmd", func(cc CommandContext) error { return nil }, SourceSkill)

	cc := CommandContext{
		Ctx:      context.Background(),
		Reply:    cr.reply,
		Registry: reg,
	}
	if err := cmdHelp(cc); err != nil {
		t.Fatalf("cmdHelp returned error: %v", err)
	}
	reply := cr.messages[0]

	// Verify section order: builtin section appears before cron section, cron before skill.
	builtinIdx := strings.Index(reply, "Built-in commands:")
	cronIdx := strings.Index(reply, "Cron commands:")
	skillIdx := strings.Index(reply, "Skill commands:")

	if builtinIdx < 0 || cronIdx < 0 || skillIdx < 0 {
		t.Fatalf("missing one or more section headers in:\n%s", reply)
	}
	if builtinIdx > cronIdx {
		t.Errorf("expected Built-in section before Cron section")
	}
	if cronIdx > skillIdx {
		t.Errorf("expected Cron section before Skill section")
	}

	// Verify commands appear after their respective headers.
	pingPos := strings.Index(reply, "/ping")
	taskCancelPos := strings.Index(reply, "/task-cancel")
	researcherPos := strings.Index(reply, "/researcher")

	if pingPos < builtinIdx || pingPos > cronIdx {
		t.Errorf("expected /ping to appear in the Built-in section, got pos %d (builtin:%d, cron:%d)", pingPos, builtinIdx, cronIdx)
	}
	if taskCancelPos < cronIdx || taskCancelPos > skillIdx {
		t.Errorf("expected /task-cancel to appear in the Cron section, got pos %d (cron:%d, skill:%d)", taskCancelPos, cronIdx, skillIdx)
	}
	if researcherPos < skillIdx {
		t.Errorf("expected /researcher to appear in the Skill section, got pos %d (skill:%d)", researcherPos, skillIdx)
	}
}

// TestCmdHelp_GroupedOutput_AlphabeticalWithinSection verifies that commands within
// a section appear in alphabetical order.
func TestCmdHelp_GroupedOutput_AlphabeticalWithinSection(t *testing.T) {
	cr := &capturedReply{}
	reg := NewCommandRegistry()
	// Register in reverse alphabetical order to verify sorting.
	reg.Register("whoami", "who am I", func(cc CommandContext) error { return nil }, SourceBuiltin)
	reg.Register("help", "list commands", func(cc CommandContext) error { return nil }, SourceBuiltin)
	reg.Register("ping", "check alive", func(cc CommandContext) error { return nil }, SourceBuiltin)

	cc := CommandContext{
		Ctx:      context.Background(),
		Reply:    cr.reply,
		Registry: reg,
	}
	if err := cmdHelp(cc); err != nil {
		t.Fatalf("cmdHelp returned error: %v", err)
	}
	reply := cr.messages[0]

	helpPos := strings.Index(reply, "/help")
	pingPos := strings.Index(reply, "/ping")
	whoamiPos := strings.Index(reply, "/whoami")

	if helpPos < 0 || pingPos < 0 || whoamiPos < 0 {
		t.Fatalf("missing one or more commands in reply:\n%s", reply)
	}
	// Alphabetical order: /help < /ping < /whoami
	if helpPos > pingPos {
		t.Errorf("expected /help before /ping in alphabetical order: help=%d, ping=%d", helpPos, pingPos)
	}
	if pingPos > whoamiPos {
		t.Errorf("expected /ping before /whoami in alphabetical order: ping=%d, whoami=%d", pingPos, whoamiPos)
	}
}

// TestCmdHelp_GroupedOutput_OmitsEmptySections verifies that sections with no
// commands are not included in the output.
func TestCmdHelp_GroupedOutput_OmitsEmptySections(t *testing.T) {
	cr := &capturedReply{}
	reg := NewCommandRegistry()
	// Only builtin commands — no cron or skill.
	reg.Register("ping", "check alive", func(cc CommandContext) error { return nil }, SourceBuiltin)

	cc := CommandContext{
		Ctx:      context.Background(),
		Reply:    cr.reply,
		Registry: reg,
	}
	if err := cmdHelp(cc); err != nil {
		t.Fatalf("cmdHelp returned error: %v", err)
	}
	reply := cr.messages[0]

	if !strings.Contains(reply, "Built-in commands:") {
		t.Errorf("expected Built-in section to be present, got:\n%s", reply)
	}
	if strings.Contains(reply, "Cron commands:") {
		t.Errorf("expected Cron section to be absent when no cron commands, got:\n%s", reply)
	}
	if strings.Contains(reply, "Skill commands:") {
		t.Errorf("expected Skill section to be absent when no skill commands, got:\n%s", reply)
	}
}
