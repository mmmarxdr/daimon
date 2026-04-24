package store

import (
	"strings"
	"testing"
)

// TestMigration_V10_CostRecordsTableExists verifies that migration v10 creates
// the cost_records table with the expected schema.
func TestMigration_V10_CostRecordsTableExists(t *testing.T) {
	s := newTestSQLiteStore(t)

	var count int
	if err := s.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='cost_records'",
	).Scan(&count); err != nil {
		t.Fatalf("querying cost_records table: %v", err)
	}
	if count != 1 {
		t.Errorf("expected cost_records table to exist, count=%d", count)
	}

	// Verify schema includes expected columns.
	var schema string
	if err := s.db.QueryRow(
		"SELECT sql FROM sqlite_master WHERE type='table' AND name='cost_records'",
	).Scan(&schema); err != nil {
		t.Fatalf("querying cost_records schema: %v", err)
	}

	expectedCols := []string{
		"id", "session_id", "channel_id", "model",
		"input_tokens", "output_tokens",
		"input_cost_usd", "output_cost_usd", "total_cost_usd",
		"created_at",
	}
	for _, col := range expectedCols {
		if !strings.Contains(schema, col) {
			t.Errorf("expected column %q in cost_records schema: %s", col, schema)
		}
	}
}

// TestMigration_V10_CostRecordsIndexes verifies that migration v10 creates
// the expected indexes on cost_records.
func TestMigration_V10_CostRecordsIndexes(t *testing.T) {
	s := newTestSQLiteStore(t)

	expectedIndexes := []string{
		"idx_cost_session",
		"idx_cost_channel",
		"idx_cost_model",
		"idx_cost_created",
	}

	for _, idx := range expectedIndexes {
		var name string
		err := s.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?",
			idx,
		).Scan(&name)
		if err != nil {
			t.Errorf("index %q not found: %v", idx, err)
		}
	}
}

// TestMigration_V10_FreshDBSchemaVersion verifies that a fresh database
// reaches schema version 10 after all migrations.
func TestMigration_V10_FreshDBSchemaVersion(t *testing.T) {
	s := newTestSQLiteStore(t)

	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if version != 14 {
		t.Errorf("expected schema_version=14, got %d", version)
	}
}

// TestMigration_V10_RerunIsNoOp verifies that calling initSchema a second time
// on an already-migrated database is safe and leaves schema_version unchanged.
func TestMigration_V10_RerunIsNoOp(t *testing.T) {
	path := t.TempDir()
	s := openSQLiteStoreAt(t, path)

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
	s.Close()
}
