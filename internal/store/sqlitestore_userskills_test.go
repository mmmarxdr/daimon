package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// helper: build a minimal UserSkill for insertion in tests.
func makeSkill(name string) UserSkill {
	return UserSkill{
		ID:          "id-" + name,
		Name:        name,
		Description: "desc " + name,
		Prose:       "prose for " + name,
		Executable:  false,
		Model:       "",
		Provider:    "",
		Version:     1,
		Source:      "user",
		CreatedAt:   time.Now().UTC().Truncate(time.Second),
		UpdatedAt:   time.Now().UTC().Truncate(time.Second),
	}
}

// ─── Task 2.6: ListUserSkills ────────────────────────────────────────────────

// TestListUserSkills_Empty verifies that an empty DB returns an empty non-nil slice.
func TestListUserSkills_Empty(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	list, err := s.ListUserSkills(ctx)
	if err != nil {
		t.Fatalf("ListUserSkills: %v", err)
	}
	if list == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(list) != 0 {
		t.Errorf("expected 0 rows, got %d", len(list))
	}
}

// TestListUserSkills_Ordered verifies rows are returned ordered by name ASC.
func TestListUserSkills_Ordered(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	// Insert out of alphabetical order.
	for _, name := range []string{"zebra-skill", "alpha-skill", "middle-skill"} {
		if _, err := s.CreateUserSkill(ctx, makeSkill(name)); err != nil {
			t.Fatalf("CreateUserSkill %q: %v", name, err)
		}
	}

	list, err := s.ListUserSkills(ctx)
	if err != nil {
		t.Fatalf("ListUserSkills: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(list))
	}
	expected := []string{"alpha-skill", "middle-skill", "zebra-skill"}
	for i, want := range expected {
		if list[i].Name != want {
			t.Errorf("index %d: got %q, want %q", i, list[i].Name, want)
		}
	}
}

// ─── Task 2.7: GetUserSkill ──────────────────────────────────────────────────

// TestGetUserSkill_Exists verifies an existing row is returned correctly.
func TestGetUserSkill_Exists(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	created, err := s.CreateUserSkill(ctx, makeSkill("my-skill"))
	if err != nil {
		t.Fatalf("CreateUserSkill: %v", err)
	}

	got, err := s.GetUserSkill(ctx, "my-skill")
	if err != nil {
		t.Fatalf("GetUserSkill: %v", err)
	}
	if got.Name != created.Name {
		t.Errorf("name: got %q, want %q", got.Name, created.Name)
	}
	if got.Description != created.Description {
		t.Errorf("description: got %q, want %q", got.Description, created.Description)
	}
}

// TestGetUserSkill_Missing verifies ErrNotFound for an unknown name.
func TestGetUserSkill_Missing(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	_, err := s.GetUserSkill(ctx, "no-such-skill")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ─── Task 2.8: CreateUserSkill ───────────────────────────────────────────────

// TestCreateUserSkill_Success verifies a created row matches the input.
func TestCreateUserSkill_Success(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	input := makeSkill("new-skill")
	input.Prose = "do something useful"
	input.Executable = true

	got, err := s.CreateUserSkill(ctx, input)
	if err != nil {
		t.Fatalf("CreateUserSkill: %v", err)
	}
	if got.Name != input.Name {
		t.Errorf("Name: got %q, want %q", got.Name, input.Name)
	}
	if got.Prose != input.Prose {
		t.Errorf("Prose: got %q, want %q", got.Prose, input.Prose)
	}
	if got.Executable != input.Executable {
		t.Errorf("Executable: got %v, want %v", got.Executable, input.Executable)
	}
}

// TestCreateUserSkill_NameConflict verifies ErrNameConflict on duplicate name.
func TestCreateUserSkill_NameConflict(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	if _, err := s.CreateUserSkill(ctx, makeSkill("dup-skill")); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := s.CreateUserSkill(ctx, makeSkill("dup-skill"))
	if !errors.Is(err, ErrNameConflict) {
		t.Errorf("expected ErrNameConflict, got %v", err)
	}
}

// ─── Task 2.9: UpdateUserSkill ───────────────────────────────────────────────

// TestUpdateUserSkill_Success verifies fields and updated_at are changed.
func TestUpdateUserSkill_Success(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	orig, err := s.CreateUserSkill(ctx, makeSkill("update-me"))
	if err != nil {
		t.Fatalf("CreateUserSkill: %v", err)
	}

	// Wait a tick so updated_at advances.
	time.Sleep(2 * time.Millisecond)

	updated := orig
	updated.Description = "updated description"
	updated.Prose = "new prose"

	got, err := s.UpdateUserSkill(ctx, updated)
	if err != nil {
		t.Fatalf("UpdateUserSkill: %v", err)
	}
	if got.Description != "updated description" {
		t.Errorf("Description: got %q, want 'updated description'", got.Description)
	}
	if got.Prose != "new prose" {
		t.Errorf("Prose: got %q, want 'new prose'", got.Prose)
	}
	// updated_at must be strictly after original.
	if !got.UpdatedAt.After(orig.UpdatedAt) {
		t.Errorf("UpdatedAt did not advance: orig=%v got=%v", orig.UpdatedAt, got.UpdatedAt)
	}
}

// TestUpdateUserSkill_Missing verifies ErrNotFound for unknown name.
func TestUpdateUserSkill_Missing(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	_, err := s.UpdateUserSkill(ctx, makeSkill("ghost-skill"))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ─── Task 2.10: DeleteUserSkill ──────────────────────────────────────────────

// TestDeleteUserSkill_Success verifies row removal and subsequent ErrNotFound.
func TestDeleteUserSkill_Success(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	if _, err := s.CreateUserSkill(ctx, makeSkill("delete-me")); err != nil {
		t.Fatalf("CreateUserSkill: %v", err)
	}

	if err := s.DeleteUserSkill(ctx, "delete-me"); err != nil {
		t.Fatalf("DeleteUserSkill: %v", err)
	}

	_, err := s.GetUserSkill(ctx, "delete-me")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

// TestDeleteUserSkill_Missing verifies ErrNotFound for unknown name.
func TestDeleteUserSkill_Missing(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	err := s.DeleteUserSkill(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ─── Task 2.11: Budget NULL round-trip ───────────────────────────────────────

// TestUserSkill_BudgetNullRoundTrip verifies nil Budget stays nil after DB round-trip.
func TestUserSkill_BudgetNullRoundTrip(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	skill := makeSkill("nil-budget")
	skill.Budget = nil

	created, err := s.CreateUserSkill(ctx, skill)
	if err != nil {
		t.Fatalf("CreateUserSkill: %v", err)
	}
	if created.Budget != nil {
		t.Errorf("expected nil Budget after create, got %+v", created.Budget)
	}

	got, err := s.GetUserSkill(ctx, "nil-budget")
	if err != nil {
		t.Fatalf("GetUserSkill: %v", err)
	}
	if got.Budget != nil {
		t.Errorf("expected nil Budget after Get, got %+v", got.Budget)
	}
}

// ─── Task 2.12: ToolsAllowlist nil vs empty distinction ──────────────────────

// TestUserSkill_AllowlistNilVsEmpty verifies nil → SQL NULL → nil and
// []string{} → JSON "[]" → non-nil empty slice.
func TestUserSkill_AllowlistNilVsEmpty(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	// nil case.
	nilSkill := makeSkill("allowlist-nil")
	nilSkill.ToolsAllowlist = nil
	cn, err := s.CreateUserSkill(ctx, nilSkill)
	if err != nil {
		t.Fatalf("CreateUserSkill nil-allowlist: %v", err)
	}
	if cn.ToolsAllowlist != nil {
		t.Errorf("expected nil ToolsAllowlist, got %v", cn.ToolsAllowlist)
	}

	fetched, err := s.GetUserSkill(ctx, "allowlist-nil")
	if err != nil {
		t.Fatalf("GetUserSkill nil-allowlist: %v", err)
	}
	if fetched.ToolsAllowlist != nil {
		t.Errorf("expected nil ToolsAllowlist after Get, got %v", fetched.ToolsAllowlist)
	}

	// empty-slice case.
	emptySkill := makeSkill("allowlist-empty")
	emptySkill.ToolsAllowlist = []string{}
	ce, err := s.CreateUserSkill(ctx, emptySkill)
	if err != nil {
		t.Fatalf("CreateUserSkill empty-allowlist: %v", err)
	}
	if ce.ToolsAllowlist == nil {
		t.Error("expected non-nil empty ToolsAllowlist, got nil")
	}
	if len(ce.ToolsAllowlist) != 0 {
		t.Errorf("expected len=0, got %d", len(ce.ToolsAllowlist))
	}

	fetched2, err := s.GetUserSkill(ctx, "allowlist-empty")
	if err != nil {
		t.Fatalf("GetUserSkill empty-allowlist: %v", err)
	}
	if fetched2.ToolsAllowlist == nil {
		t.Error("expected non-nil empty ToolsAllowlist after Get, got nil")
	}
}

// TestUserSkill_AllowlistValues verifies a populated allowlist round-trips via DB.
func TestUserSkill_AllowlistValues(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	skill := makeSkill("allowlist-values")
	skill.ToolsAllowlist = []string{"bash", "read_file"}

	created, err := s.CreateUserSkill(ctx, skill)
	if err != nil {
		t.Fatalf("CreateUserSkill: %v", err)
	}
	if len(created.ToolsAllowlist) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(created.ToolsAllowlist))
	}

	got, err := s.GetUserSkill(ctx, "allowlist-values")
	if err != nil {
		t.Fatalf("GetUserSkill: %v", err)
	}
	if len(got.ToolsAllowlist) != 2 || got.ToolsAllowlist[0] != "bash" {
		t.Errorf("unexpected allowlist: %v", got.ToolsAllowlist)
	}
}
