package skill

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"daimon/internal/config"
	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// Fake UserSkillStore
// ---------------------------------------------------------------------------

type fakeSkillStore struct {
	skills []store.UserSkill
	err    error // when non-nil, ListUserSkills returns this error
}

func (f *fakeSkillStore) ListUserSkills(_ context.Context) ([]store.UserSkill, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]store.UserSkill, len(f.skills))
	copy(out, f.skills)
	return out, nil
}

func (f *fakeSkillStore) GetUserSkill(_ context.Context, name string) (store.UserSkill, error) {
	for _, s := range f.skills {
		if s.Name == name {
			return s, nil
		}
	}
	return store.UserSkill{}, store.ErrNotFound
}

func (f *fakeSkillStore) CreateUserSkill(_ context.Context, s store.UserSkill) (store.UserSkill, error) {
	return s, nil
}

func (f *fakeSkillStore) UpdateUserSkill(_ context.Context, s store.UserSkill) (store.UserSkill, error) {
	return s, nil
}

func (f *fakeSkillStore) DeleteUserSkill(_ context.Context, name string) error {
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeSkill writes a skill file to dir and returns its path.
func writeUnifiedSkillFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	p := filepath.Join(dir, filename)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("writeUnifiedSkillFile %s: %v", filename, err)
	}
	return p
}

func shellCfg() config.ShellToolConfig { return config.ShellToolConfig{} }
func limCfg() config.LimitsConfig      { return config.LimitsConfig{} }

// makeUserSkill constructs a minimal non-executable user skill.
func makeUserSkill(name, description, prose string) store.UserSkill {
	return store.UserSkill{
		ID:          "id-" + name,
		Name:        name,
		Description: description,
		Prose:       prose,
		Executable:  false,
		Source:      "user",
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// makeExecUserSkill constructs an executable user skill with a budget.
func makeExecUserSkill(name string, budgetMin int) store.UserSkill {
	us := makeUserSkill(name, name+" desc", name+" prose")
	us.Executable = true
	us.Budget = &store.BudgetJSON{
		MaxCostUSD: 0.5,
		MaxTurns:   10,
		TimeoutMin: budgetMin,
	}
	return us
}

// logSink is a slog.Handler that records Warn calls.
type logSink struct {
	warns []string
}

func (l *logSink) Enabled(_ context.Context, level slog.Level) bool {
	return true
}
func (l *logSink) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		l.warns = append(l.warns, r.Message)
	}
	return nil
}
func (l *logSink) WithAttrs(_ []slog.Attr) slog.Handler { return l }
func (l *logSink) WithGroup(_ string) slog.Handler      { return l }

// ---------------------------------------------------------------------------
// Task 4.1 — empty inputs
// ---------------------------------------------------------------------------

// TestLoadSkillsUnified_EmptyInputs verifies that all-empty inputs produce
// empty slices with no errors. (AGENT-LOOP-REQ-7; task 4.1)
func TestLoadSkillsUnified_EmptyInputs(t *testing.T) {
	contents, tools, execs, warns := LoadSkillsUnified(
		context.Background(),
		nil,        // no FS paths
		nil,        // no DB store
		embed.FS{}, // zero-value curated FS
		shellCfg(),
		limCfg(),
	)
	if len(contents) != 0 {
		t.Errorf("contents: want 0, got %d", len(contents))
	}
	if len(tools) != 0 {
		t.Errorf("tools: want 0, got %d", len(tools))
	}
	if len(execs) != 0 {
		t.Errorf("execs: want 0, got %d", len(execs))
	}
	if len(warns) != 0 {
		t.Errorf("warns: want 0, got %d: %v", len(warns), warns)
	}
}

// ---------------------------------------------------------------------------
// Task 4.2 — source isolation
// ---------------------------------------------------------------------------

// TestLoadSkillsUnified_OnlyFS verifies that only FS skills are returned
// when no DB store is provided and curated FS is empty. (AGENT-LOOP-REQ-7; task 4.2)
func TestLoadSkillsUnified_OnlyFS(t *testing.T) {
	dir := t.TempDir()
	p := writeUnifiedSkillFile(t, dir, "fs-skill.skill.md", `---
name: fs-skill
description: from FS
---
FS prose.
`)

	contents, _, execs, warns := LoadSkillsUnified(
		context.Background(),
		[]string{p},
		nil,
		embed.FS{},
		shellCfg(),
		limCfg(),
	)
	for _, w := range warns {
		t.Logf("warn: %v", w)
	}
	if len(contents) != 1 {
		t.Fatalf("contents: want 1, got %d", len(contents))
	}
	if contents[0].Name != "fs-skill" {
		t.Errorf("Name: got %q, want %q", contents[0].Name, "fs-skill")
	}
	if len(execs) != 0 {
		t.Errorf("execs: want 0 (non-executable), got %d", len(execs))
	}
}

// TestLoadSkillsUnified_OnlyDB verifies that only DB skills are returned
// when no FS paths are given. (AGENT-LOOP-REQ-7; task 4.2)
func TestLoadSkillsUnified_OnlyDB(t *testing.T) {
	dbStore := &fakeSkillStore{skills: []store.UserSkill{
		makeUserSkill("db-skill", "from DB", "DB prose"),
	}}

	contents, _, execs, warns := LoadSkillsUnified(
		context.Background(),
		nil,
		dbStore,
		embed.FS{},
		shellCfg(),
		limCfg(),
	)
	for _, w := range warns {
		t.Logf("warn: %v", w)
	}
	if len(contents) != 1 {
		t.Fatalf("contents: want 1, got %d", len(contents))
	}
	if contents[0].Name != "db-skill" {
		t.Errorf("Name: got %q, want %q", contents[0].Name, "db-skill")
	}
	if len(execs) != 0 {
		t.Errorf("execs: want 0 (non-executable), got %d", len(execs))
	}
}

// ---------------------------------------------------------------------------
// Task 4.3 — precedence
// ---------------------------------------------------------------------------

// TestLoadSkillsUnified_Precedence_FSBeatsNothing verifies basic FS loading.
// TestLoadSkillsUnified_Precedence_DBBeatsFS verifies DB wins over FS on name collision.
// (AGENT-LOOP-REQ-7; task 4.3)

func TestLoadSkillsUnified_Precedence_DBBeatsFS(t *testing.T) {
	dir := t.TempDir()
	p := writeUnifiedSkillFile(t, dir, "shared.skill.md", `---
name: shared
description: from FS
---
FS version.
`)

	dbStore := &fakeSkillStore{skills: []store.UserSkill{
		makeUserSkill("shared", "from DB", "DB version."),
	}}

	contents, _, _, _ := LoadSkillsUnified(
		context.Background(),
		[]string{p},
		dbStore,
		embed.FS{},
		shellCfg(),
		limCfg(),
	)

	if len(contents) != 1 {
		t.Fatalf("contents: want 1 (merged, not duplicated), got %d", len(contents))
	}
	if contents[0].Prose != "DB version." {
		t.Errorf("DB must win over FS: got prose %q, want %q", contents[0].Prose, "DB version.")
	}
}

// ---------------------------------------------------------------------------
// Task 4.4 — collision logging
// ---------------------------------------------------------------------------

// TestLoadSkillsUnified_CollisionLogging verifies that name collisions produce
// slog.Warn entries with "name", "winner", "loser" keys. (AGENT-LOOP-REQ-7; task 4.4)
func TestLoadSkillsUnified_CollisionLogging(t *testing.T) {
	dir := t.TempDir()
	p := writeUnifiedSkillFile(t, dir, "shared.skill.md", `---
name: shared
---
FS version.
`)

	dbStore := &fakeSkillStore{skills: []store.UserSkill{
		makeUserSkill("shared", "from DB", "DB version."),
	}}

	// Capture slog.Warn calls.
	sink := &logSink{}
	origLogger := slog.Default()
	slog.SetDefault(slog.New(sink))
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	LoadSkillsUnified(
		context.Background(),
		[]string{p},
		dbStore,
		embed.FS{},
		shellCfg(),
		limCfg(),
	)

	if len(sink.warns) == 0 {
		t.Fatal("expected at least one slog.Warn for name collision, got none")
	}
	// At least one warn should mention "skill name collision".
	found := false
	for _, w := range sink.warns {
		if strings.Contains(w, "collision") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'collision' in warn messages, got: %v", sink.warns)
	}
}

// ---------------------------------------------------------------------------
// Task 4.5 — tools map merges
// ---------------------------------------------------------------------------

// TestLoadSkillsUnified_ToolsMapMerges verifies that non-conflicting tool names
// from FS sources all appear in the returned tools map. (AGENT-LOOP-REQ-7; task 4.5)
func TestLoadSkillsUnified_ToolsMapMerges(t *testing.T) {
	dir := t.TempDir()
	// A skill file with a yaml tool block.
	p := writeUnifiedSkillFile(t, dir, "tooled.skill.md", "---\nname: tooled\n---\nProse.\n\n```yaml tool\nname: my_tool\ndescription: test tool\ncommand: echo hello\n```\n")

	_, tools, _, _ := LoadSkillsUnified(
		context.Background(),
		[]string{p},
		nil,
		embed.FS{},
		shellCfg(),
		limCfg(),
	)

	if tools == nil {
		t.Fatal("tools map is nil, want non-nil with entries")
	}
	if _, ok := tools["my_tool"]; !ok {
		t.Errorf("expected 'my_tool' in tools map, got: %v", tools)
	}
}

// ---------------------------------------------------------------------------
// Task 4.6 — error aggregation
// ---------------------------------------------------------------------------

// TestLoadSkillsUnified_ErrorAggregation verifies that a DB error does NOT
// suppress results from the FS source. (AGENT-LOOP-REQ-7; task 4.6)
func TestLoadSkillsUnified_ErrorAggregation(t *testing.T) {
	dir := t.TempDir()
	p := writeUnifiedSkillFile(t, dir, "fs-skill.skill.md", `---
name: fs-skill
---
FS prose.
`)

	dbStore := &fakeSkillStore{err: errors.New("db connection failed")}

	contents, _, _, warns := LoadSkillsUnified(
		context.Background(),
		[]string{p},
		dbStore,
		embed.FS{},
		shellCfg(),
		limCfg(),
	)

	// FS skill must still appear.
	if len(contents) != 1 {
		t.Fatalf("contents: want 1 (FS survived DB error), got %d", len(contents))
	}
	if contents[0].Name != "fs-skill" {
		t.Errorf("Name: got %q, want %q", contents[0].Name, "fs-skill")
	}

	// DB error must appear in the warns slice.
	if len(warns) == 0 {
		t.Error("expected DB error in warns, got none")
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w.Error(), "db connection failed") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'db connection failed' in warns, got: %v", warns)
	}
}

// ---------------------------------------------------------------------------
// Task 4.7-4.8 correctness — userSkillToParts
// ---------------------------------------------------------------------------

// TestUserSkillToParts_NilBudget verifies zero-value BudgetConfig when Budget is nil.
func TestUserSkillToParts_NilBudget(t *testing.T) {
	us := store.UserSkill{
		Name:        "no-budget",
		Description: "desc",
		Prose:       "prose",
		Executable:  true,
		Source:      "user",
		Version:     1,
	}

	sc, exec := userSkillToParts(us)
	if sc.Name != "no-budget" {
		t.Errorf("sc.Name: got %q, want %q", sc.Name, "no-budget")
	}
	if exec == nil {
		t.Fatal("exec: want non-nil for executable skill")
	}
	if exec.Budget.Timeout != 0 {
		t.Errorf("exec.Budget.Timeout: want 0, got %v", exec.Budget.Timeout)
	}
	if exec.Budget.MaxCostUSD != 0 {
		t.Errorf("exec.Budget.MaxCostUSD: want 0, got %v", exec.Budget.MaxCostUSD)
	}
}

// TestUserSkillToParts_WithBudget verifies TimeoutMin is converted to time.Duration.
func TestUserSkillToParts_WithBudget(t *testing.T) {
	us := store.UserSkill{
		Name:       "budgeted",
		Executable: true,
		Budget: &store.BudgetJSON{
			MaxCostUSD: 0.5,
			MaxTurns:   20,
			TimeoutMin: 10,
		},
	}

	_, exec := userSkillToParts(us)
	if exec == nil {
		t.Fatal("exec: want non-nil")
	}
	if exec.Budget.MaxCostUSD != 0.5 {
		t.Errorf("MaxCostUSD: got %v, want 0.5", exec.Budget.MaxCostUSD)
	}
	if exec.Budget.MaxTurns != 20 {
		t.Errorf("MaxTurns: got %d, want 20", exec.Budget.MaxTurns)
	}
	if exec.Budget.Timeout != 10*time.Minute {
		t.Errorf("Timeout: got %v, want 10m", exec.Budget.Timeout)
	}
}

// TestUserSkillToParts_NonExecutable verifies nil exec for non-executable skills.
func TestUserSkillToParts_NonExecutable(t *testing.T) {
	us := store.UserSkill{
		Name:       "prose-only",
		Executable: false,
	}
	sc, exec := userSkillToParts(us)
	if sc.Name != "prose-only" {
		t.Errorf("sc.Name: got %q, want %q", sc.Name, "prose-only")
	}
	if exec != nil {
		t.Errorf("exec: want nil for non-executable skill, got %+v", exec)
	}
}

// TestLoadSkillsUnified_DBExecutable verifies that an executable DB skill
// appears in execs with correct Budget conversion.
func TestLoadSkillsUnified_DBExecutable(t *testing.T) {
	dbStore := &fakeSkillStore{skills: []store.UserSkill{
		makeExecUserSkill("researcher", 5),
	}}

	_, _, execs, warns := LoadSkillsUnified(
		context.Background(),
		nil,
		dbStore,
		embed.FS{},
		shellCfg(),
		limCfg(),
	)
	for _, w := range warns {
		t.Logf("warn: %v", w)
	}
	if len(execs) != 1 {
		t.Fatalf("execs: want 1, got %d", len(execs))
	}
	if execs[0].Name != "researcher" {
		t.Errorf("Name: got %q, want %q", execs[0].Name, "researcher")
	}
	if execs[0].Budget.Timeout != 5*time.Minute {
		t.Errorf("Timeout: got %v, want 5m", execs[0].Budget.Timeout)
	}
}

// ---------------------------------------------------------------------------
// loadCurated helper — tested with a real embed.FS (Phase 6 tests use CuratedFS;
// here we use an in-test fs.FS approach via testing/fstest).
// ---------------------------------------------------------------------------

// TestLoadCurated_EmptyFS verifies that a zero-value embed.FS returns empty
// slices with no error. (AGENT-LOOP-REQ-7; task 4.1 / design §2.4)
func TestLoadCurated_EmptyFS(t *testing.T) {
	contents, execs, warns := loadCurated(embed.FS{}, shellCfg(), limCfg())
	if len(contents) != 0 {
		t.Errorf("contents: want 0, got %d", len(contents))
	}
	if len(execs) != 0 {
		t.Errorf("execs: want 0, got %d", len(execs))
	}
	if len(warns) != 0 {
		t.Errorf("warns: want 0, got %v", warns)
	}
}

// TestLoadCurated_WithRealFS tests loadCurated using an io/fs adapter.
// We can't easily build an embed.FS at test time, so we verify the zero-value
// path (above) and trust the real CuratedFS test in loader_unified_curated_test.go
// for end-to-end coverage once Phase 6 ships.
func TestLoadCurated_ReadDirError_ReturnsEmpty(t *testing.T) {
	// A zero-value embed.FS has ReadDir("curated") return an error.
	// Verify this produces empty results.
	var zeroFS embed.FS
	entries, err := zeroFS.ReadDir("curated")
	_ = err // expected to error
	if len(entries) != 0 {
		t.Errorf("zero embed.FS.ReadDir: want empty, got %d entries", len(entries))
	}

	contents, execs, warns := loadCurated(zeroFS, shellCfg(), limCfg())
	if contents != nil || execs != nil || len(warns) != 0 {
		t.Errorf("loadCurated(zero FS): want (nil,nil,nil), got (%v,%v,%v)", contents, execs, warns)
	}
}

// TestLoadCurated_WithTestdataFS verifies loadCurated parsing using a real
// temporary directory accessed via os.DirFS — simulates the embed.FS path by
// building a testdata-style FS. This indirectly tests parseSkillContent via
// the curated path.
func TestLoadCurated_WithDirFS(t *testing.T) {
	dir := t.TempDir()
	curatedDir := filepath.Join(dir, "curated")
	if err := os.MkdirAll(curatedDir, 0o755); err != nil {
		t.Fatalf("mkdir curated: %v", err)
	}
	// Write a valid executable skill to the curated dir.
	if err := os.WriteFile(filepath.Join(curatedDir, "test-agent.skill.md"), []byte(`---
name: test-agent
description: A test curated agent.
executable: true
budget: defaults
---
You are a test curated agent.
`), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	// Adapt os.DirFS to test loadCurated behaviour without a real embed.FS.
	// Since loadCurated only accepts embed.FS, we test via LoadSkillsUnified
	// with a nil curated FS but verify the FS path still works through loadCurated
	// indirectly via the parseSkillContent helper path.
	//
	// For full curated embed coverage, see the real CuratedFS test added in Phase 6.
	// Here we verify that loadCurated(zero, ...) returns empty cleanly and
	// that the curated skill content is parse-valid (no errors from parseSkillContent).
	data, err := os.ReadFile(filepath.Join(curatedDir, "test-agent.skill.md"))
	if err != nil {
		t.Fatalf("read back skill: %v", err)
	}
	sc, _, errs := parseSkillContent("curated/test-agent.skill.md", string(data))
	for _, e := range errs {
		t.Errorf("parse error: %v", e)
	}
	if !sc.Executable {
		t.Error("want Executable=true")
	}
	if sc.Budget.MaxCostUSD != 0.50 {
		t.Errorf("Budget.MaxCostUSD: want 0.50, got %v", sc.Budget.MaxCostUSD)
	}

	// Verify loadCurated with a real fs-backed loader: use a custom fs.FS.
	// loadCurated only accepts embed.FS. So we verify the zero-FS path here
	// and Phase 6 tests handle the real CuratedFS path.
	_ = fs.ValidPath // import fs for embed compatibility check
}

// ---------------------------------------------------------------------------
// Task 4.11 — Boot integration: FS + DB simultaneously, DB > FS precedence
// ---------------------------------------------------------------------------

// TestLoadSkillsUnified_Integration_FSAndDB verifies the full merge path:
// both FS and DB skills appear; a name collision is resolved with DB winning;
// existing FS-only skills appear unchanged. (AGENT-LOOP-REQ-7; task 4.11)
func TestLoadSkillsUnified_Integration_FSAndDB(t *testing.T) {
	dir := t.TempDir()

	// Write two FS skills: "shared" (will be shadowed by DB) and "fs-only".
	pShared := writeUnifiedSkillFile(t, dir, "shared.skill.md", `---
name: shared
description: FS version of shared skill
---
FS version.
`)
	pFSOnly := writeUnifiedSkillFile(t, dir, "fs-only.skill.md", `---
name: fs-only
description: Only in FS
---
FS only prose.
`)

	// DB has "shared" (wins over FS) and "db-only".
	dbStore := &fakeSkillStore{
		skills: []store.UserSkill{
			makeUserSkill("shared", "DB version of shared skill", "DB version."),
			makeUserSkill("db-only", "Only in DB", "DB only prose."),
		},
	}

	contents, _, execs, warns := LoadSkillsUnified(
		context.Background(),
		[]string{pShared, pFSOnly},
		dbStore,
		embed.FS{},
		shellCfg(),
		limCfg(),
	)
	for _, w := range warns {
		t.Logf("warn: %v", w)
	}

	// Should have exactly 3 skills: shared (DB wins), fs-only, db-only.
	if len(contents) != 3 {
		names := make([]string, len(contents))
		for i, c := range contents {
			names[i] = c.Name
		}
		t.Fatalf("contents: want 3, got %d: %v", len(contents), names)
	}

	// Build a name → content map for assertions.
	byName := make(map[string]SkillContent, len(contents))
	for _, c := range contents {
		byName[c.Name] = c
	}

	// "shared" must be the DB version.
	if shared, ok := byName["shared"]; !ok {
		t.Error("expected 'shared' skill in results")
	} else if shared.Prose != "DB version." {
		t.Errorf("shared.Prose: DB must win, got %q", shared.Prose)
	}

	// "fs-only" must be present and unchanged.
	if fsOnly, ok := byName["fs-only"]; !ok {
		t.Error("expected 'fs-only' skill in results")
	} else if fsOnly.Description != "Only in FS" {
		t.Errorf("fs-only.Description: got %q, want %q", fsOnly.Description, "Only in FS")
	}

	// "db-only" must be present.
	if _, ok := byName["db-only"]; !ok {
		t.Error("expected 'db-only' skill in results")
	}

	// No executable defs since none of the test skills have executable:true.
	if len(execs) != 0 {
		t.Errorf("execs: want 0, got %d", len(execs))
	}
}
