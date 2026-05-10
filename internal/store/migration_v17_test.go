package store

import (
	"database/sql"
	"testing"
	"time"

	"daimon/internal/config"
)

// TestMigration_V17_ColumnsExist verifies that migration v17 adds
// conv_id, parent_conv_id, attribution_kind to cost_records.
func TestMigration_V17_ColumnsExist(t *testing.T) {
	s := newTestSQLiteStore(t)

	rows, err := s.db.Query(`SELECT conv_id, parent_conv_id, attribution_kind FROM cost_records WHERE 1=0`)
	if err != nil {
		t.Fatalf("v17 columns do not exist on cost_records: %v", err)
	}
	rows.Close()
}

// TestMigration_V17_IndexesExist verifies idx_cost_conv and idx_cost_parent_conv were created.
func TestMigration_V17_IndexesExist(t *testing.T) {
	s := newTestSQLiteStore(t)

	for _, idx := range []string{"idx_cost_conv", "idx_cost_parent_conv"} {
		var name string
		err := s.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx,
		).Scan(&name)
		if err != nil {
			t.Fatalf("expected index %q to exist: %v", idx, err)
		}
	}
}

// TestMigration_V17_BackfillsConvID verifies that existing cost_records rows
// get conv_id = session_id after v17 runs.
func TestMigration_V17_BackfillsConvID(t *testing.T) {
	path := t.TempDir()

	// Open fully migrated DB (v17).
	s1, err := NewSQLiteStore(config.StoreConfig{Path: path})
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	// We need to simulate pre-v17 rows: insert a cost_record without conv_id
	// by using a raw INSERT that skips the conv_id column.
	// However since v17 already ran, the column exists. We simulate old rows
	// by inserting with conv_id=NULL (the pre-migration state) and then
	// manually triggering a backfill check.
	//
	// For the real backfill test: open a v16 DB (manual), insert cost_record
	// without conv_id, then upgrade to v17.
	s1.Close()

	// Rebuild: create a raw v16 DB.
	dbPath := path + "/daimon.db"
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening raw db: %v", err)
	}
	if _, err := rawDB.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=1"); err != nil {
		rawDB.Close()
		t.Fatalf("pragmas: %v", err)
	}
	// Drop and recreate to simulate v16 state.
	// Re-create cost_records without v17 columns.
	rawDB.Exec("DROP TABLE IF EXISTS cost_records")               //nolint:errcheck
	rawDB.Exec("DROP INDEX IF EXISTS idx_cost_session")           //nolint:errcheck
	rawDB.Exec("DROP INDEX IF EXISTS idx_cost_channel")           //nolint:errcheck
	rawDB.Exec("DROP INDEX IF EXISTS idx_cost_model")             //nolint:errcheck
	rawDB.Exec("DROP INDEX IF EXISTS idx_cost_created")           //nolint:errcheck
	rawDB.Exec("DROP INDEX IF EXISTS idx_cost_conv")              //nolint:errcheck
	rawDB.Exec("DROP INDEX IF EXISTS idx_cost_parent_conv")       //nolint:errcheck
	if _, err := rawDB.Exec(`
		CREATE TABLE cost_records (
			id              TEXT PRIMARY KEY,
			session_id      TEXT NOT NULL,
			channel_id      TEXT NOT NULL,
			model           TEXT NOT NULL,
			input_tokens    INTEGER NOT NULL,
			output_tokens   INTEGER NOT NULL,
			input_cost_usd  REAL NOT NULL,
			output_cost_usd REAL NOT NULL,
			total_cost_usd  REAL NOT NULL,
			created_at      DATETIME NOT NULL
		)
	`); err != nil {
		rawDB.Close()
		t.Fatalf("creating cost_records: %v", err)
	}
	// Re-create indexes from v10.
	for _, idx := range []string{
		`CREATE INDEX idx_cost_session ON cost_records(session_id)`,
		`CREATE INDEX idx_cost_channel ON cost_records(channel_id)`,
		`CREATE INDEX idx_cost_model ON cost_records(model)`,
		`CREATE INDEX idx_cost_created ON cost_records(created_at)`,
	} {
		if _, err := rawDB.Exec(idx); err != nil {
			rawDB.Close()
			t.Fatalf("creating index: %v", err)
		}
	}

	// Insert a pre-v17 cost record with a session_id.
	now := time.Now().UTC()
	if _, err := rawDB.Exec(
		`INSERT INTO cost_records
			(id, session_id, channel_id, model, input_tokens, output_tokens,
			 input_cost_usd, output_cost_usd, total_cost_usd, created_at)
		 VALUES ('cost-1', 'session-abc', 'cli', 'claude-3', 100, 50, 0.01, 0.01, 0.02, ?)`,
		now,
	); err != nil {
		rawDB.Close()
		t.Fatalf("inserting cost record: %v", err)
	}
	// Set schema_version to 16 so only v17 runs on next open.
	rawDB.Exec(`UPDATE schema_version SET version = 16`) //nolint:errcheck
	rawDB.Close()

	// Open through NewSQLiteStore — v17 must run and backfill conv_id.
	s2, err := NewSQLiteStore(config.StoreConfig{Path: path})
	if err != nil {
		t.Fatalf("NewSQLiteStore on v16 db: %v", err)
	}
	defer s2.Close()

	var convID sql.NullString
	var parentConvID sql.NullString
	var attrKind string
	err = s2.db.QueryRow(
		`SELECT conv_id, parent_conv_id, attribution_kind FROM cost_records WHERE id='cost-1'`,
	).Scan(&convID, &parentConvID, &attrKind)
	if err != nil {
		t.Fatalf("reading cost record: %v", err)
	}
	if !convID.Valid || convID.String != "session-abc" {
		t.Errorf("expected conv_id='session-abc', got %v", convID)
	}
	if parentConvID.Valid {
		t.Errorf("expected parent_conv_id=NULL, got %v", parentConvID.String)
	}
	if attrKind != "self" {
		t.Errorf("expected attribution_kind='self', got %q", attrKind)
	}
}

// TestMigration_V17_AttributionKindDefault verifies new cost_records rows
// get attribution_kind='self' by default.
func TestMigration_V17_AttributionKindDefault(t *testing.T) {
	s := newTestSQLiteStore(t)

	now := time.Now().UTC()
	if _, err := s.db.Exec(
		`INSERT INTO cost_records
			(id, session_id, channel_id, model, input_tokens, output_tokens,
			 input_cost_usd, output_cost_usd, total_cost_usd, created_at)
		 VALUES ('cost-default', 'sess-1', 'cli', 'claude-3', 10, 5, 0.001, 0.001, 0.002, ?)`,
		now,
	); err != nil {
		t.Fatalf("inserting cost record: %v", err)
	}

	var attrKind string
	if err := s.db.QueryRow(
		`SELECT attribution_kind FROM cost_records WHERE id='cost-default'`,
	).Scan(&attrKind); err != nil {
		t.Fatalf("reading attribution_kind: %v", err)
	}
	if attrKind != "self" {
		t.Errorf("expected attribution_kind='self' by default, got %q", attrKind)
	}
}
