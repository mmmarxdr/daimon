package agent

// skill_automount_test.go — WU8 RED tests for skill auto-mount via WithExecutableSkills.
//
// Tests cover:
//   (a) skills with simple names registered as /<name>
//   (b) skills with hyphens registered as /<name_underscored>
//   (c) collision with builtin → builtin wins, skill skipped
//   (d) collision with cron → cron wins, skill skipped
//   (e) no collision → skill registered
//   (f) skill command handler invokes spawn tool and emits a reply

import (
	"context"
	"strings"
	"sync"
	"testing"

	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// (a) simple name registered
// ---------------------------------------------------------------------------

// TestWithExecutableSkills_SimpleNameRegisteredAsCommand verifies that a skill
// with a plain lower-case name is auto-mounted as a /<name> slash command.
func TestWithExecutableSkills_SimpleNameRegisteredAsCommand(t *testing.T) {
	a := newAgentForSkillAutomount(t)
	defs := []skill.ExecutableSkillDef{
		{Name: "researcher", Description: "Research assistant"},
	}
	a.WithExecutableSkills(defs)

	_, found := a.commands.Lookup("researcher")
	if !found {
		t.Error("expected /researcher to be registered after WithExecutableSkills")
	}

	entries := a.commands.EntriesWithSource()
	var src string
	for _, e := range entries {
		if e.Name == "researcher" {
			src = e.Source
			break
		}
	}
	if src != SourceSkill {
		t.Errorf("expected source=%q for /researcher, got %q", SourceSkill, src)
	}
}

// ---------------------------------------------------------------------------
// (b) hyphenated name normalised to underscored
// ---------------------------------------------------------------------------

// TestWithExecutableSkills_HyphenNormalisedToUnderscore verifies that a skill
// named "my-skill" is auto-mounted as /my_skill (D3 from spec).
func TestWithExecutableSkills_HyphenNormalisedToUnderscore(t *testing.T) {
	a := newAgentForSkillAutomount(t)
	defs := []skill.ExecutableSkillDef{
		{Name: "my-skill", Description: "Hyphenated skill"},
	}
	a.WithExecutableSkills(defs)

	if _, found := a.commands.Lookup("my_skill"); !found {
		t.Error("expected /my_skill to be registered (hyphen→underscore normalization)")
	}
	if _, found := a.commands.Lookup("my-skill"); found {
		t.Error("expected /my-skill (unhyphenated) NOT to be registered")
	}
}

// ---------------------------------------------------------------------------
// (c) collision with builtin → builtin wins
// ---------------------------------------------------------------------------

// TestWithExecutableSkills_CollisionWithBuiltin_BuiltinWins verifies that when
// a skill name collides with a built-in command, the built-in is preserved and
// the skill is skipped.
func TestWithExecutableSkills_CollisionWithBuiltin_BuiltinWins(t *testing.T) {
	a := newAgentForSkillAutomount(t)
	// "help" is a built-in registered in New() via registerBuiltinCommands.
	defs := []skill.ExecutableSkillDef{
		{Name: "help", Description: "Skill that collides with /help builtin"},
	}
	a.WithExecutableSkills(defs)

	entries := a.commands.EntriesWithSource()
	var src string
	for _, e := range entries {
		if e.Name == "help" {
			src = e.Source
			break
		}
	}
	if src != SourceBuiltin {
		t.Errorf("expected /help source to remain %q after collision, got %q", SourceBuiltin, src)
	}
}

// ---------------------------------------------------------------------------
// (d) collision with cron → cron wins
// ---------------------------------------------------------------------------

// TestWithExecutableSkills_CollisionWithCron_CronWins verifies that when a
// skill name collides with a cron command, the cron command is preserved.
func TestWithExecutableSkills_CollisionWithCron_CronWins(t *testing.T) {
	a := newAgentForSkillAutomount(t)

	// Manually register a cron command so we control the collision.
	a.commands.Register("tasks", "Cron tasks command", func(cc CommandContext) error {
		return nil
	}, SourceCron)

	// Now try to mount a skill named "tasks".
	defs := []skill.ExecutableSkillDef{
		{Name: "tasks", Description: "Skill that collides with /tasks cron"},
	}
	a.WithExecutableSkills(defs)

	entries := a.commands.EntriesWithSource()
	var src string
	for _, e := range entries {
		if e.Name == "tasks" {
			src = e.Source
			break
		}
	}
	if src != SourceCron {
		t.Errorf("expected /tasks source to remain %q after collision, got %q", SourceCron, src)
	}
}

// ---------------------------------------------------------------------------
// (e) no collision → skill registered cleanly
// ---------------------------------------------------------------------------

// TestWithExecutableSkills_NoCollision_SkillRegistered verifies that a skill
// with a unique name is registered as a slash command with SourceSkill.
func TestWithExecutableSkills_NoCollision_SkillRegistered(t *testing.T) {
	a := newAgentForSkillAutomount(t)
	defs := []skill.ExecutableSkillDef{
		{Name: "summarizer", Description: "Summarization skill"},
		{Name: "code-review", Description: "Code review skill"},
	}
	a.WithExecutableSkills(defs)

	for _, want := range []string{"summarizer", "code_review"} {
		_, found := a.commands.Lookup(want)
		if !found {
			t.Errorf("expected /%s to be registered", want)
		}
	}

	entries := a.commands.EntriesWithSource()
	for _, e := range entries {
		if e.Name == "summarizer" || e.Name == "code_review" {
			if e.Source != SourceSkill {
				t.Errorf("expected source=%q for /%s, got %q", SourceSkill, e.Name, e.Source)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// (f) skill command handler invokes spawn tool and emits a reply
// ---------------------------------------------------------------------------

// TestWithExecutableSkills_HandlerCallsSpawnToolAndReplies verifies that
// invoking a skill command dispatches via the SubagentSpawnTool and returns
// a reply when the subagent finishes.
func TestWithExecutableSkills_HandlerCallsSpawnToolAndReplies(t *testing.T) {
	a := newAgentForSkillAutomount(t)

	defs := []skill.ExecutableSkillDef{
		{Name: "researcher", Description: "Research assistant"},
	}
	a.WithExecutableSkills(defs)

	// Inject a fakeManager into the registered SubagentSpawnTool so Spawn is
	// intercepted without needing a real child agent.
	fm := &fakeManagerForSkill{
		result: &SubagentResult{
			Status:  "completed",
			Summary: "Research complete: OAuth uses tokens",
		},
	}
	spawnTool, ok := a.tools["researcher"].(*SubagentSpawnTool)
	if !ok {
		t.Fatal("expected a.tools[researcher] to be *SubagentSpawnTool after WithExecutableSkills")
	}
	spawnTool.manager = fm

	handler, found := a.commands.Lookup("researcher")
	if !found {
		t.Fatal("expected /researcher to be registered")
	}

	cr := &capturedReply{}
	cc := CommandContext{
		Ctx:       context.Background(),
		ChannelID: "chan:1",
		SenderID:  "user:1",
		Args:      "What is OAuth?",
		Reply:     cr.reply,
		Registry:  a.commands,
	}

	if err := handler(cc); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if !fm.spawnCalled {
		t.Error("expected SubagentManager.Spawn to be called")
	}
	if fm.lastPrompt != "What is OAuth?" {
		t.Errorf("expected prompt=%q, got %q", "What is OAuth?", fm.lastPrompt)
	}
	if len(cr.messages) == 0 {
		t.Fatal("expected at least one reply message")
	}
	if !strings.Contains(cr.messages[0], "OAuth") {
		t.Errorf("expected reply to contain result text, got %q", cr.messages[0])
	}
}

// TestWithExecutableSkills_Handler_NilSubMgr_RepliesUnavailable verifies that
// when subMgr is nil at invocation time, the handler replies gracefully.
func TestWithExecutableSkills_Handler_NilSubMgr_RepliesUnavailable(t *testing.T) {
	a := newAgentForSkillAutomount(t)

	defs := []skill.ExecutableSkillDef{{Name: "nil-skill", Description: "test"}}
	a.WithExecutableSkills(defs)

	// Override the spawn tool's manager to nil to simulate unavailable subMgr.
	spawnTool, ok := a.tools["nil-skill"].(*SubagentSpawnTool)
	if !ok {
		t.Fatal("expected *SubagentSpawnTool for nil-skill")
	}
	spawnTool.manager = nil

	handler, found := a.commands.Lookup("nil_skill")
	if !found {
		t.Fatal("expected /nil_skill to be registered")
	}

	cr := &capturedReply{}
	cc := CommandContext{
		Ctx:       context.Background(),
		ChannelID: "chan:1",
		SenderID:  "user:1",
		Args:      "test",
		Reply:     cr.reply,
		Registry:  a.commands,
	}
	if err := handler(cc); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if len(cr.messages) == 0 {
		t.Fatal("expected a reply when subMgr is nil")
	}
	if !strings.Contains(strings.ToLower(cr.messages[0]), "unavailable") {
		t.Errorf("expected unavailable message, got %q", cr.messages[0])
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newAgentForSkillAutomount creates a minimal *Agent suitable for skill
// auto-mount tests. Uses the full New() constructor so built-in commands are
// pre-registered (needed for collision tests).
func newAgentForSkillAutomount(t *testing.T) *Agent {
	t.Helper()
	return makeAgentWithStore(t, &mockStore{})
}

// fakeManagerForSkill is a test double for spawnCaller used in WU8 tests.
// It records Spawn calls and returns a pre-baked result via the handle's Wait.
type fakeManagerForSkill struct {
	mu          sync.Mutex
	spawnCalled bool
	lastPrompt  string
	lastSkill   string
	result      *SubagentResult
	spawnErr    error
}

func (m *fakeManagerForSkill) Spawn(
	_ context.Context,
	def skill.ExecutableSkillDef,
	prompt string,
	_ SpawnMode,
	_ string,
) (*SubagentHandle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spawnCalled = true
	m.lastPrompt = prompt
	m.lastSkill = def.Name
	if m.spawnErr != nil {
		return nil, m.spawnErr
	}
	// Build a fake SubagentHandle whose Wait returns immediately.
	doneCh := make(chan struct{})
	close(doneCh)
	result := m.result
	if result == nil {
		result = &SubagentResult{Status: "completed", Summary: "done"}
	}
	rec := &subRecord{
		done:   doneCh,
		result: result,
		mu:     sync.Mutex{},
	}
	return &SubagentHandle{rec: rec}, nil
}
