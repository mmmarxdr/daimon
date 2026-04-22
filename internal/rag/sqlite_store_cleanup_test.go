package rag_test

import (
	"context"
	"fmt"
	"testing"

	_ "modernc.org/sqlite"

	"daimon/internal/rag"
)

// buildJunkChunks builds a sequence that matches the bug pattern:
// starting from prevContent, each subsequent chunk is the previous content
// with the first rune stripped (exactly 1 rune shorter).
// Returns numJunk chunks starting at index startIdx.
func buildJunkChunks(docID, prevContent string, startIdx, numJunk int) []rag.DocumentChunk {
	chunks := make([]rag.DocumentChunk, numJunk)
	prev := []rune(prevContent)
	for i := 0; i < numJunk; i++ {
		prev = prev[1:] // drop first rune
		chunks[i] = rag.DocumentChunk{
			ID:      fmt.Sprintf("junk-%s-%d", docID, startIdx+i),
			Index:   startIdx + i,
			Content: string(prev),
		}
	}
	return chunks
}

// T5 — Cleanup removes exactly the trailing junk suffix while leaving real chunks.
func TestCleanupJunkChunks_RemovesExactPattern(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	if err := store.AddDocument(ctx, makeDoc("doc-junk", "global", "Junk Test")); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	// 3 real chunks.
	realChunks := []rag.DocumentChunk{
		{ID: "r0", Index: 0, Content: "Hello world and friends"},
		{ID: "r1", Index: 1, Content: "world, this is great"},
		{ID: "r2", Index: 2, Content: "this is a test content"},
	}
	if err := store.AddChunks(ctx, "doc-junk", realChunks); err != nil {
		t.Fatalf("AddChunks real: %v", err)
	}

	// 5 junk chunks derived from r2 (each drops 1 rune from the front).
	junk := buildJunkChunks("doc-junk", "this is a test content", 3, 5)
	if err := store.AddChunks(ctx, "doc-junk", junk); err != nil {
		t.Fatalf("AddChunks junk: %v", err)
	}

	docsScanned, chunksDeleted, err := store.CleanupJunkChunks(ctx)
	if err != nil {
		t.Fatalf("CleanupJunkChunks: %v", err)
	}
	if docsScanned != 1 {
		t.Errorf("docsScanned: expected 1, got %d", docsScanned)
	}
	if chunksDeleted != 5 {
		t.Errorf("chunksDeleted: expected 5, got %d", chunksDeleted)
	}

	// Real chunks must still be present.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM document_chunks WHERE doc_id = ?`, "doc-junk").Scan(&count); err != nil {
		t.Fatalf("count remaining chunks: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 real chunks to remain, got %d", count)
	}
	for _, rc := range realChunks {
		var content string
		err := db.QueryRow(`SELECT content FROM document_chunks WHERE id = ?`, rc.ID).Scan(&content)
		if err != nil {
			t.Errorf("real chunk %s missing after cleanup: %v", rc.ID, err)
		}
	}
}

// T6 — Cleanup is idempotent: second run on already-clean data deletes nothing.
func TestCleanupJunkChunks_Idempotent(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	if err := store.AddDocument(ctx, makeDoc("doc-idem", "global", "Idempotent Test")); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}
	realChunks := []rag.DocumentChunk{
		{ID: "i0", Index: 0, Content: "Hello world and friends"},
		{ID: "i1", Index: 1, Content: "world, this is great"},
		{ID: "i2", Index: 2, Content: "this is a test content"},
	}
	if err := store.AddChunks(ctx, "doc-idem", realChunks); err != nil {
		t.Fatalf("AddChunks real: %v", err)
	}
	junk := buildJunkChunks("doc-idem", "this is a test content", 3, 3)
	if err := store.AddChunks(ctx, "doc-idem", junk); err != nil {
		t.Fatalf("AddChunks junk: %v", err)
	}

	// First cleanup.
	_, deleted1, err := store.CleanupJunkChunks(ctx)
	if err != nil {
		t.Fatalf("first CleanupJunkChunks: %v", err)
	}
	if deleted1 != 3 {
		t.Errorf("first cleanup: expected 3 deleted, got %d", deleted1)
	}

	// Second cleanup — must delete nothing.
	_, deleted2, err := store.CleanupJunkChunks(ctx)
	if err != nil {
		t.Fatalf("second CleanupJunkChunks: %v", err)
	}
	if deleted2 != 0 {
		t.Errorf("second cleanup: expected 0 deleted, got %d", deleted2)
	}
}

// T7 — Cleanup does not delete chunks with normal overlap (not junk).
// Normal overlap: consecutive chunks share a prefix/suffix but are NOT
// 1-rune-shorter suffixes of each other.
func TestCleanupJunkChunks_PreservesLegitimateOverlap(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	if err := store.AddDocument(ctx, makeDoc("doc-legit", "global", "Legit Overlap")); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	// Chunks with normal overlap: each starts a few runes before the previous ends.
	chunks := []rag.DocumentChunk{
		{ID: "l0", Index: 0, Content: "The quick brown fox jumps"},
		{ID: "l1", Index: 1, Content: "fox jumps over the lazy dog"},
		{ID: "l2", Index: 2, Content: "over the lazy dog and cat"},
	}
	if err := store.AddChunks(ctx, "doc-legit", chunks); err != nil {
		t.Fatalf("AddChunks: %v", err)
	}

	_, chunksDeleted, err := store.CleanupJunkChunks(ctx)
	if err != nil {
		t.Fatalf("CleanupJunkChunks: %v", err)
	}
	if chunksDeleted != 0 {
		t.Errorf("expected 0 deletions for legitimate overlap, got %d", chunksDeleted)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM document_chunks WHERE doc_id = ?`, "doc-legit").Scan(&count); err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count != 3 {
		t.Errorf("expected all 3 chunks to remain, got %d", count)
	}
}

// T8 — Two docs: one with junk, one clean. Cleanup only touches the dirty one.
func TestCleanupJunkChunks_MultipleDocsIndependent(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	// Doc A: clean.
	if err := store.AddDocument(ctx, makeDoc("doc-a", "global", "Clean Doc")); err != nil {
		t.Fatalf("AddDocument doc-a: %v", err)
	}
	cleanChunks := []rag.DocumentChunk{
		{ID: "a0", Index: 0, Content: "Clean chunk one"},
		{ID: "a1", Index: 1, Content: "Clean chunk two"},
	}
	if err := store.AddChunks(ctx, "doc-a", cleanChunks); err != nil {
		t.Fatalf("AddChunks doc-a: %v", err)
	}

	// Doc B: with junk.
	if err := store.AddDocument(ctx, makeDoc("doc-b", "global", "Dirty Doc")); err != nil {
		t.Fatalf("AddDocument doc-b: %v", err)
	}
	dirtyReal := []rag.DocumentChunk{
		{ID: "b0", Index: 0, Content: "Some real content here"},
		{ID: "b1", Index: 1, Content: "more real content text"},
	}
	if err := store.AddChunks(ctx, "doc-b", dirtyReal); err != nil {
		t.Fatalf("AddChunks doc-b real: %v", err)
	}
	junk := buildJunkChunks("doc-b", "more real content text", 2, 4)
	if err := store.AddChunks(ctx, "doc-b", junk); err != nil {
		t.Fatalf("AddChunks doc-b junk: %v", err)
	}

	docsScanned, chunksDeleted, err := store.CleanupJunkChunks(ctx)
	if err != nil {
		t.Fatalf("CleanupJunkChunks: %v", err)
	}
	if docsScanned != 2 {
		t.Errorf("docsScanned: expected 2, got %d", docsScanned)
	}
	if chunksDeleted != 4 {
		t.Errorf("chunksDeleted: expected 4 (from doc-b only), got %d", chunksDeleted)
	}

	// Doc A must be untouched.
	var countA int
	if err := db.QueryRow(`SELECT COUNT(*) FROM document_chunks WHERE doc_id = ?`, "doc-a").Scan(&countA); err != nil {
		t.Fatalf("count doc-a chunks: %v", err)
	}
	if countA != 2 {
		t.Errorf("doc-a: expected 2 chunks, got %d", countA)
	}

	// Doc B must have exactly its 2 real chunks.
	var countB int
	if err := db.QueryRow(`SELECT COUNT(*) FROM document_chunks WHERE doc_id = ?`, "doc-b").Scan(&countB); err != nil {
		t.Fatalf("count doc-b chunks: %v", err)
	}
	if countB != 2 {
		t.Errorf("doc-b: expected 2 real chunks, got %d", countB)
	}
}

// T9 — Empty DB: CleanupJunkChunks returns (0, 0, nil).
func TestCleanupJunkChunks_EmptyDB(t *testing.T) {
	db := openTestDB(t)
	store := rag.NewSQLiteDocumentStore(db, 500, 100000)
	ctx := context.Background()

	docsScanned, chunksDeleted, err := store.CleanupJunkChunks(ctx)
	if err != nil {
		t.Fatalf("CleanupJunkChunks on empty DB: %v", err)
	}
	if docsScanned != 0 {
		t.Errorf("docsScanned: expected 0, got %d", docsScanned)
	}
	if chunksDeleted != 0 {
		t.Errorf("chunksDeleted: expected 0, got %d", chunksDeleted)
	}
}
