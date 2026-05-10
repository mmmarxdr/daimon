package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ─── ListChildConversations ───────────────────────────────────────────────────

// TestSQLiteStore_ListChildConversations_ReturnsChildren verifies that children
// are returned ordered by created_at ascending. Satisfies OUTPUT-STORE-REQ-7.
func TestSQLiteStore_ListChildConversations_ReturnsChildren(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	// Insert parent.
	now := time.Now().UTC()
	parent := Conversation{
		ID: "parent-abc", ChannelID: "subagent",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.SaveConversation(ctx, parent); err != nil {
		t.Fatalf("SaveConversation parent: %v", err)
	}

	// Insert two children with distinct creation times (use explicit updated_at
	// because SQLite DATETIME precision may coalesce identical timestamps).
	child1Time := now.Add(1 * time.Second)
	child2Time := now.Add(2 * time.Second)

	child1 := Conversation{
		ID: "child-1", ChannelID: "subagent",
		ParentConvID: "parent-abc",
		Status:       "running",
		CreatedAt:    child1Time, UpdatedAt: child1Time,
	}
	child2 := Conversation{
		ID: "child-2", ChannelID: "subagent",
		ParentConvID: "parent-abc",
		Status:       "completed",
		CreatedAt:    child2Time, UpdatedAt: child2Time,
	}
	if err := s.SaveConversation(ctx, child1); err != nil {
		t.Fatalf("SaveConversation child1: %v", err)
	}
	if err := s.SaveConversation(ctx, child2); err != nil {
		t.Fatalf("SaveConversation child2: %v", err)
	}

	children, err := s.ListChildConversations(ctx, "parent-abc")
	if err != nil {
		t.Fatalf("ListChildConversations: %v", err)
	}
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}
	if children[0].ID != "child-1" {
		t.Errorf("expected first child 'child-1', got %q", children[0].ID)
	}
	if children[1].ID != "child-2" {
		t.Errorf("expected second child 'child-2', got %q", children[1].ID)
	}
}

// TestSQLiteStore_ListChildConversations_EmptySliceNotError verifies that an
// empty slice (not an error) is returned when no children exist.
func TestSQLiteStore_ListChildConversations_EmptySliceNotError(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	children, err := s.ListChildConversations(ctx, "lone-conv")
	if err != nil {
		t.Fatalf("ListChildConversations: unexpected error: %v", err)
	}
	if children == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(children) != 0 {
		t.Errorf("expected 0 children, got %d", len(children))
	}
}

// TestSQLiteStore_ListChildConversations_UnknownParent verifies that an unknown
// parentConvID returns an empty slice, not an error.
func TestSQLiteStore_ListChildConversations_UnknownParent(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	children, err := s.ListChildConversations(ctx, "ghost-parent-id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("expected 0 children for unknown parent, got %d", len(children))
	}
}

// ─── CostSummaryForTree ────────────────────────────────────────────────────────

// TestSQLiteStore_CostSummaryForTree_SumsAll verifies that CostSummaryForTree
// aggregates costs from root + all direct children. Satisfies OUTPUT-STORE-REQ-8.
func TestSQLiteStore_CostSummaryForTree_SumsAll(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Insert root conv and two children.
	for _, conv := range []Conversation{
		{ID: "root", ChannelID: "cli", CreatedAt: now, UpdatedAt: now},
		{ID: "child-a", ChannelID: "subagent", ParentConvID: "root", CreatedAt: now, UpdatedAt: now},
		{ID: "child-b", ChannelID: "subagent", ParentConvID: "root", CreatedAt: now, UpdatedAt: now},
	} {
		if err := s.SaveConversation(ctx, conv); err != nil {
			t.Fatalf("SaveConversation %s: %v", conv.ID, err)
		}
	}

	// Insert cost records.
	for _, rec := range []CostRecord{
		{ID: "cr-1", SessionID: "root", ConvID: "root", ChannelID: "cli", Model: "m1",
			InputTokens: 100, OutputTokens: 50, TotalCostUSD: 0.10, Timestamp: now},
		{ID: "cr-2", SessionID: "child-a", ConvID: "child-a", ParentConvID: "root", ChannelID: "subagent", Model: "m1",
			InputTokens: 50, OutputTokens: 25, TotalCostUSD: 0.05, Timestamp: now},
		{ID: "cr-3", SessionID: "child-b", ConvID: "child-b", ParentConvID: "root", ChannelID: "subagent", Model: "m1",
			InputTokens: 30, OutputTokens: 15, TotalCostUSD: 0.03, Timestamp: now},
	} {
		if err := s.RecordCost(ctx, rec); err != nil {
			t.Fatalf("RecordCost %s: %v", rec.ID, err)
		}
	}

	summary, err := s.CostSummaryForTree(ctx, "root")
	if err != nil {
		t.Fatalf("CostSummaryForTree: %v", err)
	}

	const wantTotal = 0.18
	const tolerance = 1e-9
	if diff := summary.TotalCostUSD - wantTotal; diff > tolerance || diff < -tolerance {
		t.Errorf("TotalCostUSD: got %.4f, want %.4f", summary.TotalCostUSD, wantTotal)
	}
	if summary.ConversationCount != 3 {
		t.Errorf("ConversationCount: got %d, want 3", summary.ConversationCount)
	}
}

// TestSQLiteStore_CostSummaryForTree_NoChildren verifies that a solo conversation
// returns only its own cost. Satisfies OUTPUT-STORE-REQ-8 (no children case).
func TestSQLiteStore_CostSummaryForTree_NoChildren(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := s.SaveConversation(ctx, Conversation{
		ID: "solo", ChannelID: "cli", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}
	if err := s.RecordCost(ctx, CostRecord{
		ID: "cr-solo", SessionID: "solo", ConvID: "solo",
		ChannelID: "cli", Model: "m1",
		InputTokens: 70, OutputTokens: 35, TotalCostUSD: 0.07, Timestamp: now,
	}); err != nil {
		t.Fatalf("RecordCost: %v", err)
	}

	summary, err := s.CostSummaryForTree(ctx, "solo")
	if err != nil {
		t.Fatalf("CostSummaryForTree: %v", err)
	}
	const wantTotal = 0.07
	const tolerance = 1e-9
	if diff := summary.TotalCostUSD - wantTotal; diff > tolerance || diff < -tolerance {
		t.Errorf("TotalCostUSD: got %.4f, want %.4f", summary.TotalCostUSD, wantTotal)
	}
	if summary.ConversationCount != 1 {
		t.Errorf("ConversationCount: got %d, want 1", summary.ConversationCount)
	}
}

// ─── SetConversationStatus ────────────────────────────────────────────────────

// TestSQLiteStore_SetConversationStatus_Updates verifies status can be changed.
// Satisfies OUTPUT-STORE-REQ-9.
func TestSQLiteStore_SetConversationStatus_Updates(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := s.SaveConversation(ctx, Conversation{
		ID: "child-1", ChannelID: "subagent", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}

	if err := s.SetConversationStatus(ctx, "child-1", "running"); err != nil {
		t.Fatalf("SetConversationStatus: %v", err)
	}

	var status string
	if err := s.db.QueryRow(
		`SELECT status FROM conversations WHERE id='child-1'`,
	).Scan(&status); err != nil {
		t.Fatalf("reading status: %v", err)
	}
	if status != "running" {
		t.Errorf("expected status='running', got %q", status)
	}
}

// TestSQLiteStore_SetConversationStatus_InvalidValue verifies that an unknown
// status returns an error without updating the row.
func TestSQLiteStore_SetConversationStatus_InvalidValue(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := s.SaveConversation(ctx, Conversation{
		ID: "conv-x", ChannelID: "cli", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}

	err := s.SetConversationStatus(ctx, "conv-x", "unknown_status")
	if err == nil {
		t.Fatal("expected error for invalid status, got nil")
	}

	// Row must NOT be updated.
	var status string
	if err2 := s.db.QueryRow(
		`SELECT status FROM conversations WHERE id='conv-x'`,
	).Scan(&status); err2 != nil {
		t.Fatalf("reading status: %v", err2)
	}
	if status != "active" {
		t.Errorf("status must remain 'active' on invalid update, got %q", status)
	}
}

// TestSQLiteStore_SetConversationStatus_NonExistent verifies that a missing
// conv ID returns an error.
func TestSQLiteStore_SetConversationStatus_NonExistent(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	err := s.SetConversationStatus(ctx, "ghost-id", "completed")
	if err == nil {
		t.Fatal("expected error for non-existent convID, got nil")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestSQLiteStore_SetConversationStatus_Idempotent verifies that setting the
// same status twice is not an error.
func TestSQLiteStore_SetConversationStatus_Idempotent(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := s.SaveConversation(ctx, Conversation{
		ID: "conv-idem", ChannelID: "subagent", Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}

	for range 2 {
		if err := s.SetConversationStatus(ctx, "conv-idem", "completed"); err != nil {
			t.Fatalf("SetConversationStatus (idempotent): %v", err)
		}
	}
}

// ─── Compactor status guard ────────────────────────────────────────────────────

// TestSQLiteStore_ListCompactableConversations_ExcludesRunning verifies that
// a conversation with status='running' is NOT returned by ListCompactableConversations
// even when its updated_at is old. Satisfies OUTPUT-STORE-REQ-10.
func TestSQLiteStore_ListCompactableConversations_ExcludesRunning(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	old := time.Now().UTC().Add(-48 * time.Hour)

	// Insert a running subagent conv with old updated_at.
	if err := s.SaveConversation(ctx, Conversation{
		ID: "sub-running", ChannelID: "subagent",
		CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}
	if _, err := s.db.Exec(
		`UPDATE conversations SET status='running', updated_at=? WHERE id='sub-running'`, old,
	); err != nil {
		t.Fatalf("forcing status=running: %v", err)
	}

	idleBefore := time.Now().UTC().Add(-1 * time.Hour) // 1h ago — captures the 48h-old conv
	ids, err := s.ListCompactableConversations(ctx, idleBefore, 100)
	if err != nil {
		t.Fatalf("ListCompactableConversations: %v", err)
	}
	for _, id := range ids {
		if id == "sub-running" {
			t.Error("running subagent conv must NOT appear in compactable list")
		}
	}
}

// TestSQLiteStore_ListCompactableConversations_IncludesCompleted verifies that
// a conv with status='completed' IS returned by ListCompactableConversations.
func TestSQLiteStore_ListCompactableConversations_IncludesCompleted(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	old := time.Now().UTC().Add(-48 * time.Hour)
	if err := s.SaveConversation(ctx, Conversation{
		ID: "sub-completed", ChannelID: "subagent",
		CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}
	if _, err := s.db.Exec(
		`UPDATE conversations SET status='completed', updated_at=? WHERE id='sub-completed'`, old,
	); err != nil {
		t.Fatalf("forcing status=completed: %v", err)
	}

	idleBefore := time.Now().UTC().Add(-1 * time.Hour)
	ids, err := s.ListCompactableConversations(ctx, idleBefore, 100)
	if err != nil {
		t.Fatalf("ListCompactableConversations: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == "sub-completed" {
			found = true
		}
	}
	if !found {
		t.Error("completed subagent conv should appear in compactable list")
	}
}

// TestSQLiteStore_ListCompactableConversations_IncludesActive verifies that
// principal (status='active') conversations with old updated_at are still returned.
func TestSQLiteStore_ListCompactableConversations_IncludesActive(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	old := time.Now().UTC().Add(-48 * time.Hour)
	if err := s.SaveConversation(ctx, Conversation{
		ID: "principal", ChannelID: "cli",
		CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}

	idleBefore := time.Now().UTC().Add(-1 * time.Hour)
	ids, err := s.ListCompactableConversations(ctx, idleBefore, 100)
	if err != nil {
		t.Fatalf("ListCompactableConversations: %v", err)
	}
	found := false
	for _, id := range ids {
		if id == "principal" {
			found = true
		}
	}
	if !found {
		t.Error("active principal conv with old updated_at must appear in compactable list")
	}
}
