package store

import (
	"database/sql"
	"testing"

	"daimon/internal/config"
)

// TestMigration_V18_TableAndIndexesExist verifies that migration v18 creates the
// user_skills table with all required columns and both indexes, advances
// schema_version to 18, and starts with zero rows.
func TestMigration_V18_TableAndIndexesExist(t *testing.T) {
	s := newTestSQLiteStore(t)

	// Table must exist and all columns must be selectable.
	rows, err := s.db.Query(
		`SELECT id, name, description, prose, executable, model, provider,
		        tools_allowlist, budget, version, source, created_at, updated_at
		 FROM user_skills WHERE 1=0`,
	)
	if err != nil {
		t.Fatalf("user_skills table / columns do not exist: %v", err)
	}
	rows.Close()

	// Row count must be zero on a fresh DB.
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM user_skills`).Scan(&count); err != nil {
		t.Fatalf("counting user_skills: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows in user_skills, got %d", count)
	}

	// schema_version must be 18.
	var version int
	if err := s.db.QueryRow(`SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if version != 18 {
		t.Errorf("expected schema_version=18, got %d", version)
	}

	// Both indexes must be present.
	for _, idx := range []string{"idx_user_skills_source", "idx_user_skills_executable"} {
		var name string
		err := s.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, idx,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected index %q to exist: %v", idx, err)
		}
	}
}

// TestMigration_V18_Idempotent verifies that running v18 on an already-v18 DB
// returns no error and does not create duplicate rows.
func TestMigration_V18_Idempotent(t *testing.T) {
	path := t.TempDir()

	// First open — v18 runs.
	s1 := openSQLiteStoreAt(t, path)

	// Insert one row to check no duplication after idempotent re-open.
	_, err := s1.db.Exec(`
		INSERT INTO user_skills
			(id, name, description, prose, executable, model, provider,
			 tools_allowlist, budget, version, source, created_at, updated_at)
		VALUES
			('id-1', 'my-skill', 'desc', '', 0, '', '', NULL, NULL, 1, 'user',
			 datetime('now'), datetime('now'))
	`)
	if err != nil {
		t.Fatalf("inserting test row: %v", err)
	}
	s1.Close()

	// Second open — initSchema runs again; because version == 18, migrateV18 is
	// NOT called (version < 18 guard is false). Row count stays at 1.
	s2 := openSQLiteStoreAt(t, path)
	defer s2.Close()

	var count int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM user_skills`).Scan(&count); err != nil {
		t.Fatalf("counting user_skills: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row after idempotent re-open, got %d", count)
	}
}

// TestMigration_V18_FromV17 verifies that opening a v17 database runs migrateV18
// and leaves the schema in a valid v18 state.
func TestMigration_V18_FromV17(t *testing.T) {
	path := t.TempDir()

	// Fully open once (reaches v18).
	s1 := openSQLiteStoreAt(t, path)
	s1.Close()

	// Wind back schema_version to 17 to force v18 to re-run on next open.
	dbPath := path + "/daimon.db"
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("opening raw db: %v", err)
	}
	rawDB.Exec(`DROP TABLE IF EXISTS user_skills`)              //nolint:errcheck
	rawDB.Exec(`DROP INDEX IF EXISTS idx_user_skills_source`)   //nolint:errcheck
	rawDB.Exec(`DROP INDEX IF EXISTS idx_user_skills_executable`) //nolint:errcheck
	rawDB.Exec(`UPDATE schema_version SET version = 17`)        //nolint:errcheck
	rawDB.Close()

	// Re-open — must succeed and reach v18.
	s2, err := NewSQLiteStore(config.StoreConfig{Path: path})
	if err != nil {
		t.Fatalf("NewSQLiteStore on v17 db: %v", err)
	}
	defer s2.Close()

	var version int
	if err := s2.db.QueryRow(`SELECT version FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("reading schema_version: %v", err)
	}
	if version != 18 {
		t.Errorf("expected schema_version=18 after v17 upgrade, got %d", version)
	}
}
