package store

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"daimon/internal/config"
)

// TestMigration_V16_ColumnsAndIndexExist verifies that migration v16 adds
// parent_conv_id and status columns plus idx_conv_parent to conversations.
func TestMigration_V16_ColumnsAndIndexExist(t *testing.T) {
	s := newTestSQLiteStore(t)

	// parent_conv_id column must exist.
	rows, err := s.db.Query(`SELECT parent_conv_id FROM conversations WHERE 1=0`)
	if err != nil {
		t.Fatalf("parent_conv_id column does not exist: %v", err)
	}
	rows.Close()

	// status column must exist.
	rows2, err := s.db.Query(`SELECT status FROM conversations WHERE 1=0`)
	if err != nil {
		t.Fatalf("status column does not exist: %v", err)
	}
	rows2.Close()

	// idx_conv_parent index must exist.
	var idxName string
	err = s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_conv_parent'`,
	).Scan(&idxName)
	if err != nil {
		t.Fatalf("expected idx_conv_parent to exist: %v", err)
	}
}

// TestMigration_V16_NewRowsGetDefaultStatus verifies that new rows receive
// status='active' and parent_conv_id=NULL (default values).
func TestMigration_V16_NewRowsGetDefaultStatus(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	conv := Conversation{
		ID:        "new-v16-conv",
		ChannelID: "cli",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.SaveConversation(ctx, conv); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}

	var status sql.NullString
	var parentConvID sql.NullString
	err := s.db.QueryRow(
		`SELECT status, parent_conv_id FROM conversations WHERE id='new-v16-conv'`,
	).Scan(&status, &parentConvID)
	if err != nil {
		t.Fatalf("reading conv: %v", err)
	}
	if !status.Valid || status.String != "active" {
		t.Errorf("expected status='active', got %v", status)
	}
	if parentConvID.Valid {
		t.Errorf("expected parent_conv_id=NULL, got %v", parentConvID.String)
	}
}

// TestMigration_V16_SchemaVersion verifies schema_version advances to 17
// after all migrations run on a fresh DB.
func TestMigration_V16_SchemaVersion(t *testing.T) {
	s := newTestSQLiteStore(t)

	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if version != 18 {
		t.Errorf("expected schema_version=18 on fresh DB after all migrations, got %d", version)
	}
}

// TestMigration_V16_OrphanSweepOnBoot verifies that boot-time orphan sweep
// marks status='running' conversations older than 24h as 'cancelled'.
func TestMigration_V16_OrphanSweepOnBoot(t *testing.T) {
	path := t.TempDir()

	// Open a fresh DB (gets migrated to v17).
	s1, err := NewSQLiteStore(config.StoreConfig{Path: path})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	// Insert a "stuck" running conversation with updated_at 48 hours ago.
	stuckTime := time.Now().UTC().Add(-48 * time.Hour)
	ctx := context.Background()
	if err := s1.SaveConversation(ctx, Conversation{
		ID:        "stuck-running",
		ChannelID: "cli",
		CreatedAt: stuckTime,
		UpdatedAt: stuckTime,
	}); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}
	// Force status='running' and an old updated_at directly.
	if _, err := s1.db.Exec(
		`UPDATE conversations SET status='running', updated_at=? WHERE id='stuck-running'`,
		stuckTime,
	); err != nil {
		t.Fatalf("forcing status=running: %v", err)
	}
	s1.Close()

	// Re-open — boot-time sweep should cancel it.
	s2, err := NewSQLiteStore(config.StoreConfig{Path: path})
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()

	var status string
	if err := s2.db.QueryRow(
		`SELECT status FROM conversations WHERE id='stuck-running'`,
	).Scan(&status); err != nil {
		t.Fatalf("reading stuck conv status: %v", err)
	}
	if status != "cancelled" {
		t.Errorf("expected status='cancelled' after boot sweep, got %q", status)
	}
}

// TestMigration_V16_RecentRunningNotSwept verifies that a 'running' conversation
// updated within the last 24h is NOT swept by the boot-time orphan sweep.
func TestMigration_V16_RecentRunningNotSwept(t *testing.T) {
	path := t.TempDir()

	s1, err := NewSQLiteStore(config.StoreConfig{Path: path})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	now := time.Now().UTC()
	ctx := context.Background()
	if err := s1.SaveConversation(ctx, Conversation{
		ID:        "recent-running",
		ChannelID: "cli",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveConversation: %v", err)
	}
	if _, err := s1.db.Exec(
		`UPDATE conversations SET status='running' WHERE id='recent-running'`,
	); err != nil {
		t.Fatalf("forcing status=running: %v", err)
	}
	s1.Close()

	s2, err := NewSQLiteStore(config.StoreConfig{Path: path})
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()

	var status string
	if err := s2.db.QueryRow(
		`SELECT status FROM conversations WHERE id='recent-running'`,
	).Scan(&status); err != nil {
		t.Fatalf("reading recent conv status: %v", err)
	}
	if status != "running" {
		t.Errorf("expected status='running' (not swept), got %q", status)
	}
}

// TestMigration_V16_IdxConvStatusUpdatedExists verifies the idx_conv_status_updated index.
func TestMigration_V16_IdxConvStatusUpdatedExists(t *testing.T) {
	s := newTestSQLiteStore(t)

	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_conv_status_updated'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("expected idx_conv_status_updated to exist: %v", err)
	}
	if !strings.Contains(name, "status") {
		t.Errorf("unexpected index name %q", name)
	}
}
