package store

import (
	"fmt"
)

// baseSchema contains CREATE TABLE IF NOT EXISTS statements for all non-FTS tables.
// FTS tables and triggers are intentionally EXCLUDED here — they are managed by
// the versioned migration system (migrateV2). This makes the base schema
// idempotent and safe to re-run on any version of the database.
const baseSchema = `
CREATE TABLE IF NOT EXISTS conversations (
	id         TEXT PRIMARY KEY,
	channel_id TEXT NOT NULL,
	messages   TEXT NOT NULL,
	metadata   TEXT,
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_conv_channel
	ON conversations(channel_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS memory (
	id         TEXT PRIMARY KEY,
	scope_id   TEXT NOT NULL,
	topic      TEXT,
	type       TEXT,
	title      TEXT,
	content    TEXT NOT NULL,
	tags       TEXT,
	source     TEXT,
	created_at DATETIME NOT NULL,
	importance INTEGER NOT NULL DEFAULT 5,
	cluster    TEXT NOT NULL DEFAULT 'general'
);

CREATE TABLE IF NOT EXISTS secrets (
	key        TEXT PRIMARY KEY,
	value      TEXT NOT NULL,
	updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS cron_jobs (
	id                   TEXT PRIMARY KEY,
	schedule             TEXT NOT NULL,
	schedule_human       TEXT NOT NULL,
	prompt               TEXT NOT NULL,
	channel_id           TEXT NOT NULL,
	description          TEXT NOT NULL DEFAULT '',
	enabled              INTEGER NOT NULL DEFAULT 1,
	created_at           INTEGER NOT NULL,
	last_run_at          INTEGER,
	next_run_at          INTEGER,
	notify_channel       TEXT NOT NULL DEFAULT '',
	notify_on_completion INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS cron_results (
	id        TEXT PRIMARY KEY,
	job_id    TEXT NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE,
	ran_at    INTEGER NOT NULL,
	output    TEXT,
	error_msg TEXT
);
CREATE INDEX IF NOT EXISTS idx_cron_results_job_ran ON cron_results(job_id, ran_at DESC);
`

// schemaVersionDDL creates the schema_version table and seeds it to version 1
// if it does not already exist. Version 1 represents the original schema
// (before any versioned migrations were added).
const schemaVersionDDL = `
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER NOT NULL
);
-- Seed with version 1 if the table is empty (covers existing v1 databases).
INSERT INTO schema_version (version)
SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM schema_version);
`

// initSchemaVersioned creates the base tables if they do not exist, ensures the
// schema_version table is present, then runs any pending versioned migrations
// in order. It is idempotent — safe to call multiple times on the same database.
func (s *SQLiteStore) initSchemaVersioned() error {
	// 1. Apply base schema (all CREATE TABLE IF NOT EXISTS — idempotent).
	if _, err := s.db.Exec(baseSchema); err != nil {
		return fmt.Errorf("base schema: %w", err)
	}

	// 2. Ensure schema_version exists and is seeded to 1 for existing databases.
	if _, err := s.db.Exec(schemaVersionDDL); err != nil {
		return fmt.Errorf("schema_version: %w", err)
	}

	// 3. Read the current schema version.
	var version int
	if err := s.db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	// 4. Run migrations in ascending order, guarded by version checks.
	if version < 2 {
		if err := s.migrateV2(); err != nil {
			return fmt.Errorf("migration v2: %w", err)
		}
	}
	if version < 3 {
		if err := s.migrateV3(); err != nil {
			return fmt.Errorf("migration v3: %w", err)
		}
	}
	if version < 4 {
		if err := s.migrateV4(); err != nil {
			return fmt.Errorf("migration v4: %w", err)
		}
	}
	if version < 5 {
		if err := s.migrateV5(); err != nil {
			return fmt.Errorf("migration v5: %w", err)
		}
	}
	if version < 6 {
		if err := s.migrateV6(); err != nil {
			return fmt.Errorf("migration v6: %w", err)
		}
	}
	if version < 7 {
		if err := s.migrateV7(); err != nil {
			return fmt.Errorf("migration v7: %w", err)
		}
	}
	if version < 8 {
		if err := s.migrateV8(); err != nil {
			return fmt.Errorf("migration v8: %w", err)
		}
	}
	if version < 9 {
		if err := s.migrateV9(); err != nil {
			return fmt.Errorf("migration v9: %w", err)
		}
	}
	if version < 10 {
		if err := s.migrateV10(); err != nil {
			return fmt.Errorf("migration v10: %w", err)
		}
	}
	if version < 11 {
		if err := s.migrateV11(); err != nil {
			return fmt.Errorf("migration v11: %w", err)
		}
	}
	if version < 12 {
		if err := s.migrateV12(); err != nil {
			return fmt.Errorf("migration v12: %w", err)
		}
	}
	if version < 13 {
		if err := s.migrateV13(); err != nil {
			return fmt.Errorf("migration v13: %w", err)
		}
	}
	if version < 14 {
		if err := s.migrateV14(); err != nil {
			return fmt.Errorf("migration v14: %w", err)
		}
	}
	if version < 15 {
		if err := s.migrateV15(); err != nil {
			return fmt.Errorf("migration v15: %w", err)
		}
	}

	// One-shot orphan cleanup (idempotent; runs every startup). Document chunks
	// can be left behind by databases created before foreign_keys was enabled
	// in the DSN — under that regime, deleting a document did NOT cascade to
	// its chunks. After the FK fix went in, those orphans accumulate ghost
	// hits in FTS5 search and waste embedding budget on dead vectors. The
	// cleanup is a single DELETE; its WHERE clause matches nothing on a clean
	// DB so the cost is a no-op there.
	if _, err := s.db.Exec(
		`DELETE FROM document_chunks WHERE doc_id NOT IN (SELECT id FROM documents)`,
	); err != nil {
		return fmt.Errorf("cleaning orphan document_chunks: %w", err)
	}

	return nil
}

// migrateV2 performs the Layer 1 migration:
//   - Adds access_count, last_accessed_at, archived_at columns to memory.
//   - Replaces the memory_fts virtual table with a Porter stemmer variant using
//     a shadow-table atomic swap strategy.
//   - Adds the memory_au (after-update) trigger.
//   - Updates schema_version to 2.
//
// All steps run inside a single transaction for atomicity.
func (s *SQLiteStore) migrateV2() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// 1. Add pruning columns to the memory base table.
	// ALTER TABLE … ADD COLUMN is safe to attempt; if the column already exists
	// the migration would have been skipped by the version check, but guard
	// defensively by ignoring "duplicate column" errors only if needed.
	alterStmts := []string{
		`ALTER TABLE memory ADD COLUMN access_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE memory ADD COLUMN last_accessed_at DATETIME`,
		`ALTER TABLE memory ADD COLUMN archived_at DATETIME`,
	}
	for _, stmt := range alterStmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("alter memory table: %w", err)
		}
	}

	// 2. Create a new FTS5 virtual table with the Porter stemmer tokenizer.
	//    Using a shadow name so we can atomically swap it with the old table.
	if _, err := tx.Exec(`
		CREATE VIRTUAL TABLE memory_fts_new USING fts5(
			content,
			tags,
			content='memory',
			content_rowid='rowid',
			tokenize="porter unicode61"
		)
	`); err != nil {
		return fmt.Errorf("creating memory_fts_new: %w", err)
	}

	// 3. Populate the new FTS table from the base table.
	if _, err := tx.Exec(`
		INSERT INTO memory_fts_new(rowid, content, tags)
		SELECT rowid, content, tags FROM memory
	`); err != nil {
		return fmt.Errorf("populating memory_fts_new: %w", err)
	}

	// 4. Drop old triggers (must happen before dropping the FTS table they reference).
	for _, trigger := range []string{"memory_ai", "memory_ad", "memory_au"} {
		if _, err := tx.Exec("DROP TRIGGER IF EXISTS " + trigger); err != nil {
			return fmt.Errorf("dropping trigger %s: %w", trigger, err)
		}
	}

	// 5. Drop the old FTS table.
	if _, err := tx.Exec("DROP TABLE IF EXISTS memory_fts"); err != nil {
		return fmt.Errorf("dropping old memory_fts: %w", err)
	}

	// 6. Rename the new FTS table to the canonical name.
	if _, err := tx.Exec("ALTER TABLE memory_fts_new RENAME TO memory_fts"); err != nil {
		return fmt.Errorf("renaming memory_fts_new: %w", err)
	}

	// 7. Recreate triggers for INSERT, DELETE, and UPDATE.
	triggers := []string{
		`CREATE TRIGGER memory_ai
			AFTER INSERT ON memory BEGIN
				INSERT INTO memory_fts(rowid, content, tags)
				VALUES (new.rowid, new.content, new.tags);
			END`,
		`CREATE TRIGGER memory_ad
			AFTER DELETE ON memory BEGIN
				INSERT INTO memory_fts(memory_fts, rowid, content, tags)
				VALUES ('delete', old.rowid, old.content, old.tags);
			END`,
		`CREATE TRIGGER memory_au
			AFTER UPDATE OF content, tags ON memory BEGIN
				INSERT INTO memory_fts(memory_fts, rowid, content, tags)
				VALUES ('delete', old.rowid, old.content, old.tags);
				INSERT INTO memory_fts(rowid, content, tags)
				VALUES (new.rowid, new.content, new.tags);
			END`,
	}
	for _, trigger := range triggers {
		if _, err := tx.Exec(trigger); err != nil {
			return fmt.Errorf("creating trigger: %w", err)
		}
	}

	// 8. Update schema version.
	if _, err := tx.Exec("UPDATE schema_version SET version = 2"); err != nil {
		return fmt.Errorf("updating schema version to 2: %w", err)
	}

	return tx.Commit()
}

// migrateV3 adds the optional embedding BLOB column to the memory table and
// advances schema_version to 3. This column stores 256-dimensional float32
// vectors serialized as little-endian binary (1,024 bytes). Rows without
// embeddings have NULL in this column, which is valid.
func (s *SQLiteStore) migrateV3() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`ALTER TABLE memory ADD COLUMN embedding BLOB`); err != nil {
		return fmt.Errorf("adding embedding column: %w", err)
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 3"); err != nil {
		return fmt.Errorf("updating schema version to 3: %w", err)
	}

	return tx.Commit()
}

// migrateV4 creates the tool_outputs FTS5 virtual table for indexing and
// searching tool execution outputs. The table uses the porter tokenizer for
// improved full-text search. Advances schema_version to 4.
func (s *SQLiteStore) migrateV4() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Create the tool_outputs FTS5 virtual table with porter tokenizer.
	// Metadata columns (id, truncated, exit_code, timestamp) are marked
	// UNINDEXED — they are stored and retrievable but not part of the FTS
	// index, which keeps disk usage down and prevents IDs/timestamps from
	// polluting MATCH search results. Searchable columns are tool_name,
	// command, and content.
	if _, err := tx.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS tool_outputs USING fts5(
			id UNINDEXED,
			tool_name,
			command,
			content,
			truncated UNINDEXED,
			exit_code UNINDEXED,
			timestamp UNINDEXED,
			tokenize="porter unicode61"
		)
	`); err != nil {
		return fmt.Errorf("creating tool_outputs FTS5 table: %w", err)
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 4"); err != nil {
		return fmt.Errorf("updating schema version to 4: %w", err)
	}

	return tx.Commit()
}

// migrateV6 adds the description column to cron_jobs for existing databases.
// New databases get the column via the base schema. Advances schema_version to 6.
func (s *SQLiteStore) migrateV6() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Check if description column already exists (guard for any edge case where
	// base schema was applied fresh and migration version counter was reset).
	rows, err := tx.Query(`PRAGMA table_info(cron_jobs)`)
	if err != nil {
		return fmt.Errorf("reading table_info for cron_jobs: %w", err)
	}
	hasDescription := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scanning table_info row: %w", err)
		}
		if name == "description" {
			hasDescription = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table_info rows: %w", err)
	}

	if !hasDescription {
		if _, err := tx.Exec(`ALTER TABLE cron_jobs ADD COLUMN description TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("adding description column to cron_jobs: %w", err)
		}
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 6"); err != nil {
		return fmt.Errorf("updating schema version to 6: %w", err)
	}

	return tx.Commit()
}

// migrateV7 adds per-job notification fields to cron_jobs.
// Idempotent: checks for column existence before ALTER TABLE.
// Advances schema_version to 7.
func (s *SQLiteStore) migrateV7() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Check which columns already exist (guard for edge cases).
	rows, err := tx.Query(`PRAGMA table_info(cron_jobs)`)
	if err != nil {
		return fmt.Errorf("reading table_info for cron_jobs: %w", err)
	}
	existing := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scanning table_info row: %w", err)
		}
		existing[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table_info rows: %w", err)
	}

	if !existing["notify_channel"] {
		if _, err := tx.Exec(`ALTER TABLE cron_jobs ADD COLUMN notify_channel TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("adding notify_channel column: %w", err)
		}
	}
	if !existing["notify_on_completion"] {
		if _, err := tx.Exec(`ALTER TABLE cron_jobs ADD COLUMN notify_on_completion INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("adding notify_on_completion column: %w", err)
		}
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 7"); err != nil {
		return fmt.Errorf("updating schema version to 7: %w", err)
	}
	return tx.Commit()
}

// migrateV5 creates the media_blobs content-addressable store table and its
// last_referenced_at index. Advances schema_version to 5.
//
// Timestamps are stored as RFC3339 TEXT to match existing conventions in the
// conversations and memory tables.
func (s *SQLiteStore) migrateV5() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS media_blobs (
			sha256             TEXT PRIMARY KEY,
			mime               TEXT NOT NULL,
			size               INTEGER NOT NULL,
			data               BLOB NOT NULL,
			created_at         TEXT NOT NULL,
			last_referenced_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("creating media_blobs table: %w", err)
	}

	if _, err := tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_media_last_referenced
			ON media_blobs(last_referenced_at)
	`); err != nil {
		return fmt.Errorf("creating idx_media_last_referenced: %w", err)
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 5"); err != nil {
		return fmt.Errorf("updating schema version to 5: %w", err)
	}

	return tx.Commit()
}

// migrateV8 adds the importance column to the memory table.
// New databases get the column via the base schema. Existing databases
// get it via this migration. Advances schema_version to 8.
//
// Idempotent: checks for column existence via PRAGMA table_info before
// issuing ALTER TABLE, so re-running on an already-migrated database is safe.
func (s *SQLiteStore) migrateV8() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Check if importance column already exists.
	rows, err := tx.Query(`PRAGMA table_info(memory)`)
	if err != nil {
		return fmt.Errorf("reading table_info for memory: %w", err)
	}
	hasImportance := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scanning table_info row: %w", err)
		}
		if name == "importance" {
			hasImportance = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table_info rows: %w", err)
	}

	if !hasImportance {
		if _, err := tx.Exec(`ALTER TABLE memory ADD COLUMN importance INTEGER NOT NULL DEFAULT 5`); err != nil {
			return fmt.Errorf("adding importance column to memory: %w", err)
		}
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 8"); err != nil {
		return fmt.Errorf("updating schema version to 8: %w", err)
	}

	return tx.Commit()
}

// migrateV10 creates the cost_records table for LLM call cost tracking.
// Indexed by session, channel, model, and created_at for efficient filtering.
// Advances schema_version to 10.
func (s *SQLiteStore) migrateV10() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS cost_records (
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
		return fmt.Errorf("creating cost_records table: %w", err)
	}

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_cost_session ON cost_records(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_channel ON cost_records(channel_id)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_model ON cost_records(model)`,
		`CREATE INDEX IF NOT EXISTS idx_cost_created ON cost_records(created_at)`,
	}
	for _, idx := range indexes {
		if _, err := tx.Exec(idx); err != nil {
			return fmt.Errorf("creating cost index: %w", err)
		}
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 10"); err != nil {
		return fmt.Errorf("updating schema version to 10: %w", err)
	}

	return tx.Commit()
}

// migrateV9 creates the RAG document storage tables: documents,
// document_chunks, and the document_chunks_fts FTS5 virtual table with sync
// triggers (dc_ai, dc_ad, dc_au). Advances schema_version to 9.
//
// All CREATE statements use IF NOT EXISTS so this migration is safe to run on
// a database that already has these tables.
func (s *SQLiteStore) migrateV9() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS documents (
			id            TEXT PRIMARY KEY,
			namespace     TEXT NOT NULL DEFAULT 'global',
			title         TEXT NOT NULL,
			source_sha256 TEXT,
			mime          TEXT,
			chunk_count   INTEGER DEFAULT 0,
			created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS document_chunks (
			id          TEXT PRIMARY KEY,
			doc_id      TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
			idx         INTEGER NOT NULL,
			content     TEXT NOT NULL,
			embedding   BLOB,
			token_count INTEGER DEFAULT 0,
			UNIQUE(doc_id, idx)
		)`,

		`CREATE VIRTUAL TABLE IF NOT EXISTS document_chunks_fts USING fts5(
			content,
			content=document_chunks,
			content_rowid=rowid
		)`,

		`CREATE TRIGGER IF NOT EXISTS dc_ai
			AFTER INSERT ON document_chunks BEGIN
				INSERT INTO document_chunks_fts(rowid, content)
				VALUES (new.rowid, new.content);
			END`,

		`CREATE TRIGGER IF NOT EXISTS dc_ad
			AFTER DELETE ON document_chunks BEGIN
				INSERT INTO document_chunks_fts(document_chunks_fts, rowid, content)
				VALUES ('delete', old.rowid, old.content);
			END`,

		`CREATE TRIGGER IF NOT EXISTS dc_au
			AFTER UPDATE ON document_chunks BEGIN
				INSERT INTO document_chunks_fts(document_chunks_fts, rowid, content)
				VALUES ('delete', old.rowid, old.content);
				INSERT INTO document_chunks_fts(rowid, content)
				VALUES (new.rowid, new.content);
			END`,
	}

	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("rag schema: %w", err)
		}
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 9"); err != nil {
		return fmt.Errorf("updating schema version to 9: %w", err)
	}

	return tx.Commit()
}

// migrateV11 adds the cluster column to the memory table. The cluster groups
// memories into high-level buckets (identity, preferences, projects,
// relationships, technical, general) assigned by the Curator. Existing rows
// default to 'general'. New databases get the column via the base schema.
// Advances schema_version to 11.
//
// Idempotent: checks for column existence via PRAGMA table_info before
// issuing ALTER TABLE, so re-running on an already-migrated database is safe.
func (s *SQLiteStore) migrateV11() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.Query(`PRAGMA table_info(memory)`)
	if err != nil {
		return fmt.Errorf("reading table_info for memory: %w", err)
	}
	hasCluster := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scanning table_info row: %w", err)
		}
		if name == "cluster" {
			hasCluster = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table_info rows: %w", err)
	}

	if !hasCluster {
		if _, err := tx.Exec(`ALTER TABLE memory ADD COLUMN cluster TEXT NOT NULL DEFAULT 'general'`); err != nil {
			return fmt.Errorf("adding cluster column to memory: %w", err)
		}
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 11"); err != nil {
		return fmt.Errorf("updating schema version to 11: %w", err)
	}

	return tx.Commit()
}

// migrateV12 adds trust-surface columns to the RAG documents table so the
// Knowledge tab can render injection counts, last-used timestamps, LLM-
// generated summaries, and PDF/DOCX page counts. All new columns are nullable
// except access_count (defaults to 0) because legacy rows have no signal.
// Advances schema_version to 12.
//
// Idempotent: checks PRAGMA table_info before ALTER so re-running is safe.
func (s *SQLiteStore) migrateV12() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.Query(`PRAGMA table_info(documents)`)
	if err != nil {
		return fmt.Errorf("reading table_info for documents: %w", err)
	}
	existing := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scanning table_info row: %w", err)
		}
		existing[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table_info rows: %w", err)
	}

	type col struct {
		name string
		ddl  string
	}
	adds := []col{
		{"access_count", `ALTER TABLE documents ADD COLUMN access_count INTEGER NOT NULL DEFAULT 0`},
		{"last_accessed_at", `ALTER TABLE documents ADD COLUMN last_accessed_at DATETIME`},
		{"summary", `ALTER TABLE documents ADD COLUMN summary TEXT`},
		{"page_count", `ALTER TABLE documents ADD COLUMN page_count INTEGER`},
	}
	for _, c := range adds {
		if existing[c.name] {
			continue
		}
		if _, err := tx.Exec(c.ddl); err != nil {
			return fmt.Errorf("adding %s column to documents: %w", c.name, err)
		}
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 12"); err != nil {
		return fmt.Errorf("updating schema version to 12: %w", err)
	}

	return tx.Commit()
}

// migrateV13 adds the ingested_at column on documents. The ingestion worker
// stamps this timestamp after processJob completes (regardless of whether any
// chunks were produced) so the API can distinguish three states:
//   - chunk_count > 0          → ready
//   - chunk_count == 0, ingested_at NOT NULL → empty (worker ran, no text)
//   - chunk_count == 0, ingested_at IS NULL  → indexing (worker not done)
//
// Without this column the empty case is indistinguishable from "still
// indexing" and the UI hangs forever on PDFs the extractor cannot read.
//
// Idempotent: skips ALTER when the column already exists.
func (s *SQLiteStore) migrateV13() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.Query(`PRAGMA table_info(documents)`)
	if err != nil {
		return fmt.Errorf("reading table_info for documents: %w", err)
	}
	hasCol := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scanning table_info row: %w", err)
		}
		if name == "ingested_at" {
			hasCol = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table_info rows: %w", err)
	}

	if !hasCol {
		if _, err := tx.Exec(`ALTER TABLE documents ADD COLUMN ingested_at DATETIME`); err != nil {
			return fmt.Errorf("adding ingested_at column to documents: %w", err)
		}
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 13"); err != nil {
		return fmt.Errorf("updating schema version to 13: %w", err)
	}

	return tx.Commit()
}

// migrateV14 adds the deleted_at column to the conversations table and a
// partial index on it, enabling soft delete with pruner-based retention.
// New databases get the column via the v14 migration running against the
// base schema (which does NOT include deleted_at — the column is added
// exclusively here so existing DBs and fresh DBs follow the same path).
//
// Idempotent: checks PRAGMA table_info before ALTER so re-running is safe.
// Advances schema_version to 14.
func (s *SQLiteStore) migrateV14() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rows, err := tx.Query(`PRAGMA table_info(conversations)`)
	if err != nil {
		return fmt.Errorf("reading table_info for conversations: %w", err)
	}
	hasCol := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scanning table_info row: %w", err)
		}
		if name == "deleted_at" {
			hasCol = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating table_info rows: %w", err)
	}

	if !hasCol {
		if _, err := tx.Exec(`ALTER TABLE conversations ADD COLUMN deleted_at TIMESTAMP NULL`); err != nil {
			return fmt.Errorf("adding deleted_at column to conversations: %w", err)
		}
	}

	// Partial index: only non-null rows, keeps the index compact because most
	// rows are live (deleted_at IS NULL). The pruner scans this to find
	// expired soft-deletes; live reads filter by `deleted_at IS NULL` which
	// the query planner can satisfy from the main table.
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_conversations_deleted_at
		ON conversations(deleted_at) WHERE deleted_at IS NOT NULL`); err != nil {
		return fmt.Errorf("creating idx_conversations_deleted_at: %w", err)
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 14"); err != nil {
		return fmt.Errorf("updating schema version to 14: %w", err)
	}

	return tx.Commit()
}

// migrateV15 adds the per-session compaction infrastructure:
//
//  1. tool_outputs gains a conversation_id column so the compactor can find
//     and delete outputs scoped to a single conversation. FTS5 doesn't support
//     ALTER, so we recreate the virtual table preserving existing rows with
//     conversation_id=''.
//  2. conversations gains compacted_at + compacted_summary columns so the
//     pruner/compactor can mark a conversation as "summarised, raw outputs
//     evicted" and the agent can re-hydrate the summary on resume.
//
// Idempotent: PRAGMA-checks before ALTER, and the FTS5 recreate uses an
// intermediate name so re-running is safe.
// Advances schema_version to 15.
func (s *SQLiteStore) migrateV15() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// --- Part 1: extend conversations with compaction fields ---
	convRows, err := tx.Query(`PRAGMA table_info(conversations)`)
	if err != nil {
		return fmt.Errorf("reading conversations columns: %w", err)
	}
	convCols := map[string]bool{}
	for convRows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := convRows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			convRows.Close()
			return fmt.Errorf("scanning conversations columns: %w", err)
		}
		convCols[name] = true
	}
	convRows.Close()
	if err := convRows.Err(); err != nil {
		return fmt.Errorf("iterating conversations columns: %w", err)
	}
	if !convCols["compacted_at"] {
		if _, err := tx.Exec(`ALTER TABLE conversations ADD COLUMN compacted_at TIMESTAMP NULL`); err != nil {
			return fmt.Errorf("adding compacted_at to conversations: %w", err)
		}
	}
	if !convCols["compacted_summary"] {
		if _, err := tx.Exec(`ALTER TABLE conversations ADD COLUMN compacted_summary TEXT`); err != nil {
			return fmt.Errorf("adding compacted_summary to conversations: %w", err)
		}
	}

	// --- Part 2: rebuild tool_outputs with conversation_id ---
	// FTS5 virtual tables cannot be ALTERed; we copy → drop → rename.
	// Skip if conversation_id already present (idempotent re-run).
	hasConvID := false
	rows, err := tx.Query(`PRAGMA table_info(tool_outputs)`)
	if err != nil {
		return fmt.Errorf("reading tool_outputs columns: %w", err)
	}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			rows.Close()
			return fmt.Errorf("scanning tool_outputs columns: %w", err)
		}
		if name == "conversation_id" {
			hasConvID = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating tool_outputs columns: %w", err)
	}

	if !hasConvID {
		// Create new table with conversation_id, copy data over with empty string
		// for legacy rows (NULL would break FTS5 search), then swap names.
		if _, err := tx.Exec(`
			CREATE VIRTUAL TABLE tool_outputs_v15 USING fts5(
				id UNINDEXED,
				conversation_id UNINDEXED,
				tool_name,
				command,
				content,
				truncated UNINDEXED,
				exit_code UNINDEXED,
				timestamp UNINDEXED,
				tokenize="porter unicode61"
			)
		`); err != nil {
			return fmt.Errorf("creating tool_outputs_v15: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO tool_outputs_v15
				(id, conversation_id, tool_name, command, content, truncated, exit_code, timestamp)
			SELECT id, '', tool_name, command, content, truncated, exit_code, timestamp
			FROM tool_outputs
		`); err != nil {
			return fmt.Errorf("copying tool_outputs rows: %w", err)
		}
		if _, err := tx.Exec(`DROP TABLE tool_outputs`); err != nil {
			return fmt.Errorf("dropping old tool_outputs: %w", err)
		}
		if _, err := tx.Exec(`ALTER TABLE tool_outputs_v15 RENAME TO tool_outputs`); err != nil {
			return fmt.Errorf("renaming tool_outputs_v15: %w", err)
		}
	}

	if _, err := tx.Exec("UPDATE schema_version SET version = 15"); err != nil {
		return fmt.Errorf("updating schema version to 15: %w", err)
	}

	return tx.Commit()
}

