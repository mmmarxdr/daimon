package store

import (
	"strings"
	"testing"
)

// TestMigration_V14_DeletedAtColumnExists verifies migration v14 adds the
// deleted_at column to the conversations table.
func TestMigration_V14_DeletedAtColumnExists(t *testing.T) {
	s := newTestSQLiteStore(t)

	var schema string
	if err := s.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='conversations'`,
	).Scan(&schema); err != nil {
		t.Fatalf("reading conversations schema: %v", err)
	}
	if !strings.Contains(strings.ToLower(schema), "deleted_at") {
		t.Errorf("expected deleted_at column in conversations schema, got: %s", schema)
	}
}

// TestMigration_V14_PartialIndexExists verifies the partial index on
// deleted_at IS NOT NULL was created.
func TestMigration_V14_PartialIndexExists(t *testing.T) {
	s := newTestSQLiteStore(t)

	var name string
	err := s.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_conversations_deleted_at'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("expected idx_conversations_deleted_at to exist: %v", err)
	}
}

// TestMigration_V14_IdempotentOnRerun verifies re-running initSchema on a v14
// DB leaves schema_version at 14 and does not throw a duplicate-column error.
func TestMigration_V14_IdempotentOnRerun(t *testing.T) {
	path := t.TempDir()
	s := openSQLiteStoreAt(t, path)

	// First run already completed during openSQLiteStoreAt — version=14.
	// Re-run initSchema explicitly.
	if err := s.initSchema(); err != nil {
		t.Fatalf("second initSchema: %v", err)
	}

	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if version != 14 {
		t.Errorf("expected schema_version=14 after re-run, got %d", version)
	}

	// Guard: the column is still singular (no duplicate-add error silently
	// swallowed by a broken guard).
	var colCount int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('conversations') WHERE name='deleted_at'`,
	).Scan(&colCount); err != nil {
		t.Fatalf("counting deleted_at columns: %v", err)
	}
	if colCount != 1 {
		t.Errorf("expected exactly 1 deleted_at column, got %d", colCount)
	}
	s.Close()
}
