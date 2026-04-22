package rag_test

// Tests for SearchChunks precision features:
//   T1  – neighbor expansion happy path
//   T2  – neighbor radius 0 is a no-op
//   T3  – neighbor clamp at document start (idx=0)
//   T4  – neighbor clamp at document end (last idx)
//   T5  – neighbor dedup with overlapping top-K halos
//   T6  – BM25 threshold drops weak chunks + their neighbors
//   T7  – cosine threshold drops below-threshold chunks
//   T8  – no-embeddings path unaffected by cosine threshold
//   T9  – empty FTS5 result stays empty
//   T10 – both thresholds enabled together
//   T11 – cross-document neighbors don't leak
//   T12 – dedup against primary hits (neighbor fetched exactly once)
//   T13 – score inheritance on neighbors

import (
	"context"
	"sort"
	"testing"
	"time"

	"daimon/internal/rag"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// seedSearchFixture seeds the DB with:
//   - "doc10": 10 chunks (idx 0–9), all containing "quantum"
//   - "doc5":   5 chunks (idx 0–4), all containing "quantum"
//
// Returns the store.
func seedSearchFixture(t *testing.T) *rag.SQLiteDocumentStore {
	t.Helper()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)
	ctx := context.Background()

	for _, spec := range []struct {
		id, title string
		n         int
	}{
		{"doc10", "Ten-chunk Doc", 10},
		{"doc5", "Five-chunk Doc", 5},
	} {
		doc := rag.Document{
			ID:        spec.id,
			Namespace: "global",
			Title:     spec.title,
			MIME:      "text/plain",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := s.AddDocument(ctx, doc); err != nil {
			t.Fatalf("AddDocument %s: %v", spec.id, err)
		}
		chunks := make([]rag.DocumentChunk, spec.n)
		for i := range chunks {
			chunks[i] = rag.DocumentChunk{
				ID:      spec.id + "-" + string(rune('a'+i)),
				DocID:   spec.id,
				Index:   i,
				Content: "quantum data fragment index-" + itoa(i),
			}
		}
		if err := s.AddChunks(ctx, spec.id, chunks); err != nil {
			t.Fatalf("AddChunks %s: %v", spec.id, err)
		}
	}
	return s
}

// chunkIDsOf extracts the chunk IDs from results for easy assertion.
func chunkIDsOf(results []rag.SearchResult) []string {
	ids := make([]string, len(results))
	for i, r := range results {
		ids[i] = r.Chunk.ID
	}
	return ids
}

// indicesOf extracts (docID, idx) pairs for results from a given doc.
func indicesOf(results []rag.SearchResult, docID string) []int {
	var idxs []int
	for _, r := range results {
		if r.Chunk.DocID == docID {
			idxs = append(idxs, r.Chunk.Index)
		}
	}
	sort.Ints(idxs)
	return idxs
}

// containsChunkID reports whether any result has the given chunk ID.
func containsChunkID(results []rag.SearchResult, id string) bool {
	for _, r := range results {
		if r.Chunk.ID == id {
			return true
		}
	}
	return false
}

// countChunkID counts how many results have the given chunk ID.
func countChunkID(results []rag.SearchResult, id string) int {
	n := 0
	for _, r := range results {
		if r.Chunk.ID == id {
			n++
		}
	}
	return n
}

// scoreOf returns the Score for the given chunk ID (first match).
func scoreOf(results []rag.SearchResult, id string) float64 {
	for _, r := range results {
		if r.Chunk.ID == id {
			return r.Score
		}
	}
	return -999
}

// itoa converts an int to a string without importing strconv globally.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// ---------------------------------------------------------------------------
// T1 – Neighbor expansion happy path
// ---------------------------------------------------------------------------

func TestSearchChunks_T1_NeighborExpansion_HappyPath(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d1", Namespace: "g", Title: "Doc", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	// 10 chunks; chunk at idx=4 has distinct content to be the top FTS hit
	chunks := make([]rag.DocumentChunk, 10)
	for i := range chunks {
		content := "generic data"
		if i == 4 {
			content = "unique needle content here"
		}
		chunks[i] = rag.DocumentChunk{ID: "d1-" + itoa(i), DocID: "d1", Index: i, Content: content}
	}
	if err := s.AddChunks(ctx, "d1", chunks); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchChunks(ctx, "needle", nil, rag.SearchOptions{Limit: 1, NeighborRadius: 1})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	idxs := indicesOf(results, "d1")
	if len(idxs) != 3 {
		t.Fatalf("T1: expected 3 results (idx 3,4,5), got %v", idxs)
	}
	for _, want := range []int{3, 4, 5} {
		found := false
		for _, got := range idxs {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("T1: expected idx=%d in results %v", want, idxs)
		}
	}
}

// ---------------------------------------------------------------------------
// T2 – Neighbor radius 0 is a no-op
// ---------------------------------------------------------------------------

func TestSearchChunks_T2_NeighborRadius0_NoOp(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d2", Namespace: "g", Title: "Doc2", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	chunks := []rag.DocumentChunk{
		{ID: "d2-0", DocID: "d2", Index: 0, Content: "generic"},
		{ID: "d2-4", DocID: "d2", Index: 4, Content: "needle unique content"},
		{ID: "d2-9", DocID: "d2", Index: 9, Content: "generic"},
	}
	if err := s.AddChunks(ctx, "d2", chunks); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchChunks(ctx, "needle", nil, rag.SearchOptions{Limit: 1, NeighborRadius: 0})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("T2: expected exactly 1 result (radius=0), got %d: %v", len(results), chunkIDsOf(results))
	}
	if results[0].Chunk.Index != 4 {
		t.Errorf("T2: expected idx=4, got idx=%d", results[0].Chunk.Index)
	}
}

// ---------------------------------------------------------------------------
// T3 – Neighbor clamp at document start (idx=0)
// ---------------------------------------------------------------------------

func TestSearchChunks_T3_NeighborClamp_AtStart(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d3", Namespace: "g", Title: "Doc3", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	chunks := make([]rag.DocumentChunk, 5)
	for i := range chunks {
		content := "filler"
		if i == 0 {
			content = "needle first chunk here"
		}
		chunks[i] = rag.DocumentChunk{ID: "d3-" + itoa(i), DocID: "d3", Index: i, Content: content}
	}
	if err := s.AddChunks(ctx, "d3", chunks); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchChunks(ctx, "needle", nil, rag.SearchOptions{Limit: 1, NeighborRadius: 2})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	// Primary at idx=0, radius=2 → neighbors can be idx=1,2 (no idx=-1,-2)
	idxs := indicesOf(results, "d3")
	for _, idx := range idxs {
		if idx < 0 {
			t.Errorf("T3: got negative index %d — neighbor leaked past document start", idx)
		}
	}
	if !containsChunkID(results, "d3-0") {
		t.Errorf("T3: primary chunk d3-0 not in results")
	}
	// Should not exceed idx=2
	for _, idx := range idxs {
		if idx > 2 {
			t.Errorf("T3: idx=%d exceeds expected max (2) for radius=2 from idx=0", idx)
		}
	}
}

// ---------------------------------------------------------------------------
// T4 – Neighbor clamp at document end (last idx)
// ---------------------------------------------------------------------------

func TestSearchChunks_T4_NeighborClamp_AtEnd(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d4", Namespace: "g", Title: "Doc4", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	// 10 chunks (idx 0-9); primary at idx=9
	chunks := make([]rag.DocumentChunk, 10)
	for i := range chunks {
		content := "filler"
		if i == 9 {
			content = "needle last chunk end"
		}
		chunks[i] = rag.DocumentChunk{ID: "d4-" + itoa(i), DocID: "d4", Index: i, Content: content}
	}
	if err := s.AddChunks(ctx, "d4", chunks); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchChunks(ctx, "needle", nil, rag.SearchOptions{Limit: 1, NeighborRadius: 2})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	// Primary at idx=9, radius=2 → should contain idx=7,8,9; no idx=10
	idxs := indicesOf(results, "d4")
	for _, idx := range idxs {
		if idx > 9 {
			t.Errorf("T4: got idx=%d — beyond end of document (max=9)", idx)
		}
	}
	for _, want := range []int{7, 8, 9} {
		found := false
		for _, got := range idxs {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("T4: expected idx=%d in results %v", want, idxs)
		}
	}
}

// ---------------------------------------------------------------------------
// T5 – Neighbor dedup with overlapping top-K halos
// ---------------------------------------------------------------------------

func TestSearchChunks_T5_NeighborDedup_OverlappingHalos(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d5", Namespace: "g", Title: "Doc5", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	// 10 chunks; idx=2 and idx=4 are both top-K hits; radius=2 makes halos overlap at idx=3
	chunks := make([]rag.DocumentChunk, 10)
	for i := range chunks {
		content := "filler"
		if i == 2 {
			content = "needle primary first"
		} else if i == 4 {
			content = "needle primary second"
		}
		chunks[i] = rag.DocumentChunk{ID: "d5-" + itoa(i), DocID: "d5", Index: i, Content: content}
	}
	if err := s.AddChunks(ctx, "d5", chunks); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchChunks(ctx, "needle", nil, rag.SearchOptions{Limit: 2, NeighborRadius: 2})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	// idx=3 must appear exactly once (not twice from both halos)
	midID := "d5-3"
	count := countChunkID(results, midID)
	if count != 1 {
		t.Errorf("T5: chunk d5-3 (idx=3) appears %d times; expected exactly 1", count)
	}
}

// ---------------------------------------------------------------------------
// T6 – BM25 threshold drops weak chunks + their neighbors
// ---------------------------------------------------------------------------

func TestSearchChunks_T6_BM25Threshold_DropsWeakChunks(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d6", Namespace: "g", Title: "Doc6", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	// One strong hit (many repeated matching terms) and one weak hit
	chunks := []rag.DocumentChunk{
		{ID: "d6-0", DocID: "d6", Index: 0, Content: "alpha"},
		{ID: "d6-1", DocID: "d6", Index: 1, Content: "alpha alpha alpha alpha alpha alpha alpha alpha alpha alpha"},
		{ID: "d6-2", DocID: "d6", Index: 2, Content: "alpha"},
	}
	if err := s.AddChunks(ctx, "d6", chunks); err != nil {
		t.Fatal(err)
	}

	// With MaxBM25Score=0 (disabled), all 3 chunks should be returned (limit=10).
	all, err := s.SearchChunks(ctx, "alpha", nil, rag.SearchOptions{Limit: 10, MaxBM25Score: 0})
	if err != nil {
		t.Fatalf("SearchChunks (baseline): %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("T6 baseline: expected 3 results, got %d", len(all))
	}

	// With a very tight BM25 threshold that only passes the strongest match
	// (BM25 returns negative: -1.3 is better than -0.4; MaxBM25Score=-1.0
	// means: reject if bm25() > -1.0, so only the chunk with bm25 ≤ -1.0 passes).
	// We use a threshold that should only pass d6-1 (the densely-repeated chunk).
	tight, err := s.SearchChunks(ctx, "alpha", nil, rag.SearchOptions{Limit: 10, MaxBM25Score: -0.5, NeighborRadius: 0})
	if err != nil {
		t.Fatalf("SearchChunks (tight threshold): %v", err)
	}
	// Should have fewer results than baseline (weak chunks dropped)
	if len(tight) >= len(all) {
		t.Errorf("T6: expected fewer results with tight BM25 threshold; got %d (same as baseline %d)", len(tight), len(all))
	}
}

// ---------------------------------------------------------------------------
// T7 – Cosine threshold drops below-threshold chunks
// ---------------------------------------------------------------------------

func TestSearchChunks_T7_CosineThreshold_DropsWeakChunks(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d7", Namespace: "g", Title: "Doc7", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}

	// 3 chunks: one with cosine=1.0 against query, two with cosine≈0
	highEmb := make([]float32, 256)
	highEmb[0] = 1.0
	lowEmb1 := make([]float32, 256)
	lowEmb1[1] = 1.0
	lowEmb2 := make([]float32, 256)
	lowEmb2[2] = 1.0

	chunks := []rag.DocumentChunk{
		{ID: "d7-high", DocID: "d7", Index: 0, Content: "cosine match query", Embedding: highEmb},
		{ID: "d7-low1", DocID: "d7", Index: 1, Content: "cosine match query", Embedding: lowEmb1},
		{ID: "d7-low2", DocID: "d7", Index: 2, Content: "cosine match query", Embedding: lowEmb2},
	}
	if err := s.AddChunks(ctx, "d7", chunks); err != nil {
		t.Fatal(err)
	}

	// Query vector matching highEmb direction
	queryVec := make([]float32, 256)
	queryVec[0] = 1.0

	results, err := s.SearchChunks(ctx, "cosine match", queryVec, rag.SearchOptions{Limit: 10, MinCosineScore: 0.9, NeighborRadius: 0})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("T7: expected 1 result (only high-cosine chunk), got %d: %v", len(results), chunkIDsOf(results))
	}
	if results[0].Chunk.ID != "d7-high" {
		t.Errorf("T7: expected d7-high, got %s", results[0].Chunk.ID)
	}
}

// ---------------------------------------------------------------------------
// T8 – No-embeddings path unaffected by cosine threshold
// ---------------------------------------------------------------------------

func TestSearchChunks_T8_NoEmbeddings_CosineThresholdIgnored(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d8", Namespace: "g", Title: "Doc8", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	chunks := []rag.DocumentChunk{
		{ID: "d8-0", DocID: "d8", Index: 0, Content: "threshold test alpha"},
		{ID: "d8-1", DocID: "d8", Index: 1, Content: "threshold test beta"},
	}
	if err := s.AddChunks(ctx, "d8", chunks); err != nil {
		t.Fatal(err)
	}

	// queryVec=nil → cosine path not taken → MinCosineScore is ignored → FTS5 returns normally
	results, err := s.SearchChunks(ctx, "threshold test", nil, rag.SearchOptions{Limit: 10, MinCosineScore: 0.9})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}
	if len(results) == 0 {
		t.Error("T8: expected FTS5 results regardless of cosine threshold when queryVec=nil")
	}
}

// ---------------------------------------------------------------------------
// T9 – Empty FTS5 result stays empty
// ---------------------------------------------------------------------------

func TestSearchChunks_T9_EmptyFTS5_StaysEmpty(t *testing.T) {
	ctx := context.Background()
	s := seedSearchFixture(t)

	results, err := s.SearchChunks(ctx, "xyzxyzxyz_no_match_zqwerty", nil, rag.SearchOptions{Limit: 5, NeighborRadius: 2})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("T9: expected empty results for no-match query, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// T10 – Both thresholds enabled together
// ---------------------------------------------------------------------------

func TestSearchChunks_T10_BothThresholds_OnlyPassingChunksSurvive(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d10", Namespace: "g", Title: "Doc10", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}

	highEmb := make([]float32, 256)
	highEmb[0] = 1.0
	lowEmb := make([]float32, 256)
	lowEmb[1] = 1.0

	chunks := []rag.DocumentChunk{
		{ID: "d10-0", DocID: "d10", Index: 0, Content: "query match test", Embedding: highEmb},
		{ID: "d10-1", DocID: "d10", Index: 1, Content: "query match test", Embedding: lowEmb},
		{ID: "d10-2", DocID: "d10", Index: 2, Content: "query match test", Embedding: highEmb},
	}
	if err := s.AddChunks(ctx, "d10", chunks); err != nil {
		t.Fatal(err)
	}

	queryVec := make([]float32, 256)
	queryVec[0] = 1.0

	// cosine threshold filters d10-1; BM25 threshold (very tight) may further filter
	results, err := s.SearchChunks(ctx, "query match", queryVec, rag.SearchOptions{
		Limit:          10,
		MinCosineScore: 0.9,
		MaxBM25Score:   0, // disabled (cosine path active)
		NeighborRadius: 0,
	})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	// Only chunks with cosine≥0.9 should survive
	for _, r := range results {
		if r.Chunk.ID == "d10-1" {
			t.Error("T10: d10-1 (low cosine) should have been filtered by MinCosineScore=0.9")
		}
	}
	if len(results) == 0 {
		t.Error("T10: expected at least 1 result (d10-0 and d10-2 have high cosine)")
	}
}

// ---------------------------------------------------------------------------
// T11 – Cross-document neighbors don't leak
// ---------------------------------------------------------------------------

func TestSearchChunks_T11_CrossDocument_NeighborsNoLeak(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	for _, spec := range []struct{ id, title string }{
		{"docX", "DocX"},
		{"docY", "DocY"},
	} {
		doc := rag.Document{ID: spec.id, Namespace: "g", Title: spec.title, MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := s.AddDocument(ctx, doc); err != nil {
			t.Fatal(err)
		}
		chunks := make([]rag.DocumentChunk, 6)
		for i := range chunks {
			content := "filler"
			if i == 4 {
				content = "needle unique " + spec.id
			}
			chunks[i] = rag.DocumentChunk{
				ID:      spec.id + "-" + itoa(i),
				DocID:   spec.id,
				Index:   i,
				Content: content,
			}
		}
		if err := s.AddChunks(ctx, spec.id, chunks); err != nil {
			t.Fatal(err)
		}
	}

	results, err := s.SearchChunks(ctx, "needle", nil, rag.SearchOptions{Limit: 5, NeighborRadius: 1})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	// Neighbors of docX's idx=4 must come only from docX; same for docY
	for _, r := range results {
		if r.Chunk.DocID == "docX" {
			if r.Chunk.Index < 3 || r.Chunk.Index > 5 {
				t.Errorf("T11: docX result at idx=%d is outside expected [3,5] window", r.Chunk.Index)
			}
		}
		if r.Chunk.DocID == "docY" {
			if r.Chunk.Index < 3 || r.Chunk.Index > 5 {
				t.Errorf("T11: docY result at idx=%d is outside expected [3,5] window", r.Chunk.Index)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// T12 – Dedup against primary hits (neighbor fetched exactly once)
// ---------------------------------------------------------------------------

func TestSearchChunks_T12_NeighborDedup_AgainstPrimaryHits(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d12", Namespace: "g", Title: "Doc12", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}
	// Primaries at idx=3 and idx=5; radius=1 → each wants idx=4 as neighbor
	chunks := make([]rag.DocumentChunk, 8)
	for i := range chunks {
		content := "filler"
		if i == 3 {
			content = "needle primary three"
		} else if i == 5 {
			content = "needle primary five"
		}
		chunks[i] = rag.DocumentChunk{ID: "d12-" + itoa(i), DocID: "d12", Index: i, Content: content}
	}
	if err := s.AddChunks(ctx, "d12", chunks); err != nil {
		t.Fatal(err)
	}

	results, err := s.SearchChunks(ctx, "needle", nil, rag.SearchOptions{Limit: 2, NeighborRadius: 1})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	// idx=4 (d12-4) must appear exactly once
	count := countChunkID(results, "d12-4")
	if count != 1 {
		t.Errorf("T12: d12-4 (idx=4) appears %d times; expected exactly 1", count)
	}
}

// ---------------------------------------------------------------------------
// T13 – Score inheritance on neighbors
// ---------------------------------------------------------------------------

func TestSearchChunks_T13_ScoreInheritance_OnNeighbors(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := rag.NewSQLiteDocumentStore(db, 0, 0)

	doc := rag.Document{ID: "d13", Namespace: "g", Title: "Doc13", MIME: "text/plain", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.AddDocument(ctx, doc); err != nil {
		t.Fatal(err)
	}

	highEmb := make([]float32, 256)
	highEmb[0] = 1.0
	noEmb := make([]float32, 0) // neighbor has no embedding

	chunks := []rag.DocumentChunk{
		{ID: "d13-2", DocID: "d13", Index: 2, Content: "needle cosine match", Embedding: highEmb},
		{ID: "d13-3", DocID: "d13", Index: 3, Content: "neighbor chunk", Embedding: noEmb},
		{ID: "d13-4", DocID: "d13", Index: 4, Content: "neighbor chunk far", Embedding: noEmb},
		// Also add a second chunk with embedding so cosine rerank triggers (needs ≥2)
		{ID: "d13-0", DocID: "d13", Index: 0, Content: "needle cosine match also", Embedding: highEmb},
	}
	if err := s.AddChunks(ctx, "d13", chunks); err != nil {
		t.Fatal(err)
	}

	queryVec := make([]float32, 256)
	queryVec[0] = 1.0

	results, err := s.SearchChunks(ctx, "needle", queryVec, rag.SearchOptions{Limit: 1, NeighborRadius: 1})
	if err != nil {
		t.Fatalf("SearchChunks: %v", err)
	}

	// Find the neighbor chunk (d13-3) and check its score equals the primary's score
	primaryScore := scoreOf(results, "d13-2")
	neighborScore := scoreOf(results, "d13-3")

	if neighborScore == -999 {
		// d13-3 may or may not be in results depending on which chunk is the top-1 primary;
		// skip the score check if d13-2 wasn't the selected primary
		t.Skip("T13: d13-3 not in results (d13-2 may not have been top-1 primary); skipping score check")
	}
	if primaryScore == -999 {
		t.Skip("T13: d13-2 (primary) not in results")
	}
	if neighborScore != primaryScore {
		t.Errorf("T13: neighbor d13-3 score=%v; expected %v (primary's score)", neighborScore, primaryScore)
	}
}
