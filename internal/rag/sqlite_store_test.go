package rag_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"daimon/internal/rag"
)

// openTestDB opens an in-memory SQLite DB, enables foreign keys, and runs
// migrateV9 via a helper exposed by the rag package for testing.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := rag.MigrateV9(db); err != nil {
		t.Fatalf("MigrateV9: %v", err)
	}
	return db
}

func makeDoc(id, namespace, title string) rag.Document {
	return rag.Document{
		ID:        id,
		Namespace: namespace,
		Title:     title,
		MIME:      "text/plain",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// T2.1: tables exist after migration.
func TestMigrateV9_TablesExist(t *testing.T) {
	db := openTestDB(t)

	tables := []string{"documents", "document_chunks"}
	for _, tbl := range tables {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migration: %v", tbl, err)
		}
	}
}

func TestMigrateV9_FTS5Works(t *testing.T) {
	db := openTestDB(t)

	// Insert a chunk directly (bypasses the store) and verify FTS5 triggers fire.
	_, err := db.Exec(`INSERT INTO documents (id, namespace, title, mime) VALUES (?, ?, ?, ?)`,
		"doc-fts", "global", "FTS Test", "text/plain")
	if err != nil {
		t.Fatalf("insert doc: %v", err)
	}

	_, err = db.Exec(`INSERT INTO document_chunks (id, doc_id, idx, content) VALUES (?, ?, ?, ?)`,
		"chunk-fts", "doc-fts", 0, "hello world fts test")
	if err != nil {
		t.Fatalf("insert chunk: %v", err)
	}

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM document_chunks_fts WHERE document_chunks_fts MATCH ?`, "hello").Scan(&count)
	if err != nil {
		t.Fatalf("FTS5 query: %v", err)
	}
	if count == 0 {
		t.Error("expected FTS5 to find 'hello' in document_chunks_fts")
	}
}

// T2.3: CRUD tests for SQLiteDocumentStore.

func TestSQLiteDocumentStore_AddAndList(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	doc := makeDoc("doc-1", "global", "First Doc")
	if err := store.AddDocument(ctx, doc); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	docs, err := store.ListDocuments(ctx, "global")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 document, got %d", len(docs))
	}
	if docs[0].ID != "doc-1" {
		t.Errorf("expected doc ID 'doc-1', got %q", docs[0].ID)
	}
}

func TestSQLiteDocumentStore_ListByNamespace(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	_ = store.AddDocument(ctx, makeDoc("doc-global", "global", "Global Doc"))
	_ = store.AddDocument(ctx, makeDoc("doc-ns1", "ns1", "NS1 Doc"))

	globalDocs, err := store.ListDocuments(ctx, "global")
	if err != nil {
		t.Fatalf("ListDocuments global: %v", err)
	}
	if len(globalDocs) != 1 || globalDocs[0].ID != "doc-global" {
		t.Errorf("unexpected global docs: %+v", globalDocs)
	}

	// Empty namespace = list all.
	allDocs, err := store.ListDocuments(ctx, "")
	if err != nil {
		t.Fatalf("ListDocuments all: %v", err)
	}
	if len(allDocs) != 2 {
		t.Errorf("expected 2 docs for empty namespace, got %d", len(allDocs))
	}
}

func TestSQLiteDocumentStore_AddChunksAndSearch_FTS5(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	doc := makeDoc("doc-search", "global", "Searchable Doc")
	if err := store.AddDocument(ctx, doc); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	chunks := []rag.DocumentChunk{
		{ID: "c1", DocID: "doc-search", Index: 0, Content: "golang programming language"},
		{ID: "c2", DocID: "doc-search", Index: 1, Content: "python scripting automation"},
	}
	if err := store.AddChunks(ctx, "doc-search", chunks); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}

	results, err := store.SearchChunks(ctx, "golang", nil, rag.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result for 'golang'")
	}
	if results[0].Chunk.Content != "golang programming language" {
		t.Errorf("unexpected top result: %q", results[0].Chunk.Content)
	}
}

func TestSQLiteDocumentStore_SearchWithCosineRerank(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	doc := makeDoc("doc-vec", "global", "Vector Doc")
	if err := store.AddDocument(ctx, doc); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	// Chunk with embedding pointing in [1,0,...] direction.
	emb1 := make([]float32, 256)
	emb1[0] = 1.0

	// Chunk with embedding pointing in [0,1,...] direction.
	emb2 := make([]float32, 256)
	emb2[1] = 1.0

	chunks := []rag.DocumentChunk{
		{ID: "cv1", DocID: "doc-vec", Index: 0, Content: "vector search cosine", Embedding: emb1},
		{ID: "cv2", DocID: "doc-vec", Index: 1, Content: "vector search cosine", Embedding: emb2},
	}
	if err := store.AddChunks(ctx, "doc-vec", chunks); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}

	// Query vector pointing toward emb2 direction.
	queryVec := make([]float32, 256)
	queryVec[1] = 1.0

	results, err := store.SearchChunks(ctx, "vector search", queryVec, rag.SearchOptions{Limit: 2})
	if err != nil {
		t.Fatalf("SearchChunks with cosine: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// The chunk with emb2 (matching queryVec direction) should rank first.
	if results[0].Chunk.ID != "cv2" {
		t.Errorf("expected cv2 to rank first (cosine match), got %q", results[0].Chunk.ID)
	}
}

func TestSQLiteDocumentStore_DeleteCascadesChunks(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	doc := makeDoc("doc-del", "global", "Delete Me")
	if err := store.AddDocument(ctx, doc); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}
	chunks := []rag.DocumentChunk{
		{ID: "del-c1", DocID: "doc-del", Index: 0, Content: "content to delete"},
	}
	if err := store.AddChunks(ctx, "doc-del", chunks); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}

	// Verify chunk exists.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM document_chunks WHERE doc_id = ?`, "doc-del").Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 chunk before delete, got %d", count)
	}

	// Delete document — should cascade.
	if err := store.DeleteDocument(ctx, "doc-del"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM document_chunks WHERE doc_id = ?`, "doc-del").Scan(&count); err != nil {
		t.Fatalf("count chunks after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 chunks after cascade delete, got %d", count)
	}
}

func TestSQLiteDocumentStore_MaxDocsGuard(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 2, 100000) // limit to 2 docs
	ctx := context.Background()

	_ = store.AddDocument(ctx, makeDoc("d1", "global", "Doc 1"))
	_ = store.AddDocument(ctx, makeDoc("d2", "global", "Doc 2"))

	err := store.AddDocument(ctx, makeDoc("d3", "global", "Doc 3"))
	if err == nil {
		t.Fatal("expected error when exceeding max documents, got nil")
	}
}

func TestSQLiteDocumentStore_AddChunks_UpdatesChunkCount(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	doc := makeDoc("doc-cc", "global", "Chunk Count Doc")
	if err := store.AddDocument(ctx, doc); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	chunks := []rag.DocumentChunk{
		{ID: "cc1", DocID: "doc-cc", Index: 0, Content: "chunk one"},
		{ID: "cc2", DocID: "doc-cc", Index: 1, Content: "chunk two"},
		{ID: "cc3", DocID: "doc-cc", Index: 2, Content: "chunk three"},
	}
	if err := store.AddChunks(ctx, "doc-cc", chunks); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}

	docs, err := store.ListDocuments(ctx, "global")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].ChunkCount != 3 {
		t.Errorf("expected ChunkCount=3, got %d", docs[0].ChunkCount)
	}
}

// TestSQLiteDocumentStore_V12Fields_RoundTrip verifies that AccessCount,
// LastAccessedAt, Summary, and PageCount round-trip through Add/List.
func TestSQLiteDocumentStore_V12Fields_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 0, 0)
	ctx := context.Background()

	pages := 42
	lastAccessed := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	doc := makeDoc("v12-doc", "global", "v12 round-trip")
	doc.AccessCount = 7
	doc.LastAccessedAt = &lastAccessed
	doc.Summary = "LLM-generated summary"
	doc.PageCount = &pages

	if err := store.AddDocument(ctx, doc); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	got, err := store.GetDocument(ctx, "v12-doc")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got.AccessCount != 7 {
		t.Errorf("AccessCount: expected 7, got %d", got.AccessCount)
	}
	if got.Summary != "LLM-generated summary" {
		t.Errorf("Summary: expected set, got %q", got.Summary)
	}
	if got.PageCount == nil || *got.PageCount != 42 {
		t.Errorf("PageCount: expected *42, got %v", got.PageCount)
	}
	if got.LastAccessedAt == nil || !got.LastAccessedAt.Equal(lastAccessed) {
		t.Errorf("LastAccessedAt: expected %v, got %v", lastAccessed, got.LastAccessedAt)
	}
}

// TestSQLiteDocumentStore_SearchBumpsAccessCount verifies that a successful
// SearchChunks call increments access_count and sets last_accessed_at on the
// parent document (best-effort, not required for the search to succeed).
func TestSQLiteDocumentStore_SearchBumpsAccessCount(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 0, 0)
	ctx := context.Background()

	if err := store.AddDocument(ctx, makeDoc("bump-doc", "global", "Bump target")); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}
	if err := store.AddChunks(ctx, "bump-doc", []rag.DocumentChunk{
		{ID: "c1", DocID: "bump-doc", Index: 0, Content: "alpha beta gamma delta", TokenCount: 4},
	}); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}

	results, err := store.SearchChunks(ctx, "alpha", nil, rag.SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result for FTS match")
	}

	got, err := store.GetDocument(ctx, "bump-doc")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if got.AccessCount != 1 {
		t.Errorf("expected AccessCount=1 after search, got %d", got.AccessCount)
	}
	if got.LastAccessedAt == nil {
		t.Error("expected LastAccessedAt to be set after search")
	}
}

func TestSQLiteDocumentStore_SumTokensByDoc(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 0, 0)
	ctx := context.Background()

	if err := store.AddDocument(ctx, makeDoc("d1", "global", "D1")); err != nil {
		t.Fatalf("AddDocument d1: %v", err)
	}
	if err := store.AddDocument(ctx, makeDoc("d2", "global", "D2")); err != nil {
		t.Fatalf("AddDocument d2: %v", err)
	}
	if err := store.AddChunks(ctx, "d1", []rag.DocumentChunk{
		{ID: "d1-0", Index: 0, Content: "a", TokenCount: 10},
		{ID: "d1-1", Index: 1, Content: "b", TokenCount: 20},
	}); err != nil {
		t.Fatalf("AddChunks d1: %v", err)
	}
	if err := store.AddChunks(ctx, "d2", []rag.DocumentChunk{
		{ID: "d2-0", Index: 0, Content: "c", TokenCount: 5},
	}); err != nil {
		t.Fatalf("AddChunks d2: %v", err)
	}

	sums, err := store.SumTokensByDoc(ctx, []string{"d1", "d2", "missing"})
	if err != nil {
		t.Fatalf("SumTokensByDoc: %v", err)
	}
	if sums["d1"] != 30 {
		t.Errorf("d1: expected 30 tokens, got %d", sums["d1"])
	}
	if sums["d2"] != 5 {
		t.Errorf("d2: expected 5 tokens, got %d", sums["d2"])
	}
	if _, ok := sums["missing"]; ok {
		t.Error("missing doc should be absent from result map")
	}
}

func TestSQLiteDocumentStore_GetDocument_NotFound(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 0, 0)
	ctx := context.Background()

	_, err := store.GetDocument(ctx, "no-such-doc")
	if err == nil {
		t.Fatal("expected error for missing doc")
	}
	if !errors.Is(err, rag.ErrDocNotFound) {
		t.Errorf("expected ErrDocNotFound, got %v", err)
	}
}
