package rag_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"daimon/internal/rag"
)

// mockMediaStore implements rag.MediaStoreReader for testing.
type mockMediaStore struct {
	mu      sync.Mutex
	media   map[string][]byte
	mimes   map[string]string
	getCalls int
}

func newMockMediaStore() *mockMediaStore {
	return &mockMediaStore{
		media: make(map[string][]byte),
		mimes: make(map[string]string),
	}
}

func (m *mockMediaStore) GetMedia(_ context.Context, sha256 string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls++
	data, ok := m.media[sha256]
	if !ok {
		return nil, "", rag.ErrDocNotFound
	}
	return data, m.mimes[sha256], nil
}

// trackingStore wraps mockDocumentStore but records calls.
type trackingStore struct {
	mu       sync.Mutex
	docs     []rag.Document
	chunks   map[string][]rag.DocumentChunk
}

func newTrackingStore() *trackingStore {
	return &trackingStore{chunks: make(map[string][]rag.DocumentChunk)}
}

func (s *trackingStore) AddDocument(_ context.Context, doc rag.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs = append(s.docs, doc)
	return nil
}

func (s *trackingStore) AddChunks(_ context.Context, docID string, chunks []rag.DocumentChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks[docID] = append(s.chunks[docID], chunks...)
	return nil
}

func (s *trackingStore) SearchChunks(_ context.Context, _ string, _ []float32, _ int) ([]rag.SearchResult, error) {
	return nil, nil
}
func (s *trackingStore) DeleteDocument(_ context.Context, _ string) error { return nil }
func (s *trackingStore) ListDocuments(_ context.Context, _ string) ([]rag.Document, error) {
	return nil, nil
}
func (s *trackingStore) GetDocument(_ context.Context, _ string) (rag.Document, error) {
	return rag.Document{}, rag.ErrDocNotFound
}

// T4.1 — DocIngestionWorker

func TestDocIngestionWorker_InlineText(t *testing.T) {
	store := newTrackingStore()
	media := newMockMediaStore()
	embedFn := func(_ context.Context, text string) ([]float32, error) {
		return []float32{0.1, 0.2, 0.3}, nil
	}

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:      store,
		Extractor:  rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:    rag.FixedSizeChunker{},
		EmbedFn:    embedFn,
		MediaStore: media,
		ChunkOpts:  rag.ChunkOptions{Size: 512, Overlap: 64},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	job := rag.IngestionJob{
		DocID:     "doc-001",
		Namespace: "global",
		Title:     "Test Doc",
		Content:   "This is the inline content for the document.",
		MIME:      "text/plain",
	}

	w.Enqueue(job)

	// Wait for processing with timeout
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		docCount := len(store.docs)
		store.mu.Unlock()
		if docCount > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	w.Stop()

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.docs) == 0 {
		t.Fatal("expected at least one document to be stored")
	}
	if len(store.chunks["doc-001"]) == 0 {
		t.Error("expected chunks to be stored for doc-001")
	}
}

func TestDocIngestionWorker_SHA256FetchAndExtract(t *testing.T) {
	store := newTrackingStore()
	media := newMockMediaStore()

	media.mu.Lock()
	media.media["abc123"] = []byte("Content fetched from media store.")
	media.mimes["abc123"] = "text/plain"
	media.mu.Unlock()

	embedFn := func(_ context.Context, text string) ([]float32, error) {
		return []float32{0.5, 0.5}, nil
	}

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:      store,
		Extractor:  rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:    rag.FixedSizeChunker{},
		EmbedFn:    embedFn,
		MediaStore: media,
		ChunkOpts:  rag.ChunkOptions{Size: 512, Overlap: 64},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	job := rag.IngestionJob{
		DocID:     "doc-sha",
		Namespace: "global",
		Title:     "SHA Doc",
		SHA256:    "abc123",
		MIME:      "text/plain",
	}
	w.Enqueue(job)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		docCount := len(store.docs)
		store.mu.Unlock()
		if docCount > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	w.Stop()

	media.mu.Lock()
	getCalls := media.getCalls
	media.mu.Unlock()
	if getCalls == 0 {
		t.Error("expected GetMedia to be called for SHA256 job")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.docs) == 0 {
		t.Fatal("expected document to be stored")
	}
}

func TestDocIngestionWorker_StopIdempotent(t *testing.T) {
	store := newTrackingStore()
	media := newMockMediaStore()

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:      store,
		Extractor:  rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:    rag.FixedSizeChunker{},
		MediaStore: media,
		ChunkOpts:  rag.ChunkOptions{Size: 512, Overlap: 64},
	})

	ctx := context.Background()
	w.Start(ctx)

	// Stop multiple times — should not panic or deadlock
	done := make(chan struct{})
	go func() {
		w.Stop()
		w.Stop()
		w.Stop()
		close(done)
	}()

	select {
	case <-done:
		// pass
	case <-time.After(3 * time.Second):
		t.Error("Stop() timed out — possible deadlock")
	}
}

func TestDocIngestionWorker_FullChannelDrops(t *testing.T) {
	store := newTrackingStore()
	media := newMockMediaStore()

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:      store,
		Extractor:  rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:    rag.FixedSizeChunker{},
		MediaStore: media,
		ChunkOpts:  rag.ChunkOptions{Size: 512, Overlap: 64},
	})

	// Do NOT start the worker — channel will fill up quickly.
	// Enqueue more than cap(5) jobs; the extras should be dropped without blocking.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			w.Enqueue(rag.IngestionJob{
				DocID:   "doc",
				Content: "text",
				MIME:    "text/plain",
			})
		}
		close(done)
	}()

	select {
	case <-done:
		// All enqueues returned without blocking — pass
	case <-time.After(2 * time.Second):
		t.Error("Enqueue blocked when channel is full — should drop")
	}
}

// ── Summary generation tests ────────────────────────────────────────────────

// waitForDoc polls until the tracking store has at least one Document, or fails.
func waitForDoc(t *testing.T, store *trackingStore) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := len(store.docs)
		store.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for document to be stored")
}

func TestDocIngestionWorker_Summary_Persisted(t *testing.T) {
	store := newTrackingStore()
	media := newMockMediaStore()

	var summaryCalls int32
	var receivedText string
	var mu sync.Mutex
	summaryFn := func(_ context.Context, text string) (string, error) {
		mu.Lock()
		summaryCalls++
		receivedText = text
		mu.Unlock()
		return "This is a 1-sentence summary.", nil
	}

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:      store,
		Extractor:  rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:    rag.FixedSizeChunker{},
		SummaryFn:  summaryFn,
		MediaStore: media,
		ChunkOpts:  rag.ChunkOptions{Size: 512, Overlap: 64},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	w.Enqueue(rag.IngestionJob{
		DocID: "sum-1", Namespace: "global", Title: "Summary Target",
		Content: "Some document content here.", MIME: "text/plain",
	})
	waitForDoc(t, store)
	w.Stop()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.docs[0].Summary != "This is a 1-sentence summary." {
		t.Errorf("expected summary persisted, got %q", store.docs[0].Summary)
	}
	mu.Lock()
	defer mu.Unlock()
	if summaryCalls != 1 {
		t.Errorf("expected SummaryFn called once, got %d", summaryCalls)
	}
	if receivedText == "" {
		t.Error("expected SummaryFn to receive extracted text")
	}
}

func TestDocIngestionWorker_Summary_FailureDoesNotBlockIngest(t *testing.T) {
	store := newTrackingStore()
	media := newMockMediaStore()

	summaryFn := func(_ context.Context, _ string) (string, error) {
		return "", context.DeadlineExceeded
	}

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:      store,
		Extractor:  rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:    rag.FixedSizeChunker{},
		SummaryFn:  summaryFn,
		MediaStore: media,
		ChunkOpts:  rag.ChunkOptions{Size: 512, Overlap: 64},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	w.Enqueue(rag.IngestionJob{
		DocID: "sum-fail", Namespace: "global", Title: "Failing Summary",
		Content: "Content", MIME: "text/plain",
	})
	waitForDoc(t, store)
	w.Stop()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.docs[0].Summary != "" {
		t.Errorf("expected empty summary on failure, got %q", store.docs[0].Summary)
	}
	// Ingest still completed — the document row IS persisted.
	if store.docs[0].ID != "sum-fail" {
		t.Errorf("expected doc persisted despite summary failure, got %+v", store.docs[0])
	}
}

func TestDocIngestionWorker_Summary_NilFnSkips(t *testing.T) {
	store := newTrackingStore()
	media := newMockMediaStore()

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:      store,
		Extractor:  rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:    rag.FixedSizeChunker{},
		SummaryFn:  nil,
		MediaStore: media,
		ChunkOpts:  rag.ChunkOptions{Size: 512, Overlap: 64},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	w.Enqueue(rag.IngestionJob{
		DocID: "sum-nil", Namespace: "global", Title: "No Summarizer",
		Content: "content", MIME: "text/plain",
	})
	waitForDoc(t, store)
	w.Stop()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.docs[0].Summary != "" {
		t.Errorf("expected empty summary when SummaryFn nil, got %q", store.docs[0].Summary)
	}
}

func TestDocIngestionWorker_PageCount_FromExtractor(t *testing.T) {
	store := newTrackingStore()
	media := newMockMediaStore()

	// Build a docx with a known page count via app.xml.
	appXML := `<?xml version="1.0"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
  <Pages>4</Pages>
</Properties>`
	docxData := buildDocx(t, minimalDocXML, appXML)

	media.mu.Lock()
	media.media["docx-sha"] = docxData
	media.mimes["docx-sha"] = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	media.mu.Unlock()

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:      store,
		Extractor:  rag.NewSelectExtractor(rag.DocxExtractor{}),
		Chunker:    rag.FixedSizeChunker{},
		MediaStore: media,
		ChunkOpts:  rag.ChunkOptions{Size: 512, Overlap: 64},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	w.Enqueue(rag.IngestionJob{
		DocID: "pg-1", Namespace: "global", Title: "Paginated",
		SHA256: "docx-sha",
		MIME:   "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	})
	waitForDoc(t, store)
	w.Stop()

	store.mu.Lock()
	defer store.mu.Unlock()
	doc := store.docs[0]
	if doc.PageCount == nil {
		t.Fatal("expected PageCount propagated from extractor to Document")
	}
	if *doc.PageCount != 4 {
		t.Errorf("PageCount: expected *4, got %d", *doc.PageCount)
	}
}

// ── EmbedBatch path ─────────────────────────────────────────────────────────

func TestDocIngestionWorker_PrefersEmbedBatchOverEmbedFn(t *testing.T) {
	// When both EmbedFn and EmbedBatchFn are wired, the worker MUST use the
	// batch path. The single-call fn should never fire.
	store := newTrackingStore()
	media := newMockMediaStore()

	var singleCalls int32
	var batchCalls int32
	var sawTexts []string
	var mu sync.Mutex
	embedFn := func(_ context.Context, _ string) ([]float32, error) {
		mu.Lock()
		singleCalls++
		mu.Unlock()
		return []float32{0.5}, nil
	}
	embedBatchFn := func(_ context.Context, texts []string) ([][]float32, error) {
		mu.Lock()
		batchCalls++
		sawTexts = append(sawTexts, texts...)
		mu.Unlock()
		out := make([][]float32, len(texts))
		for i := range texts {
			out[i] = []float32{0.1, 0.2, 0.3}
		}
		return out, nil
	}

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:        store,
		Extractor:    rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:      rag.FixedSizeChunker{},
		EmbedFn:      embedFn,
		EmbedBatchFn: embedBatchFn,
		MediaStore:   media,
		ChunkOpts:    rag.ChunkOptions{Size: 30, Overlap: 0}, // small chunks → multiple
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	// 200+ chars → multiple chunks at size 30.
	w.Enqueue(rag.IngestionJob{
		DocID: "batch-1", Namespace: "global", Title: "Multi-chunk doc",
		Content: "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau upsilon",
		MIME:    "text/plain",
	})
	waitForDoc(t, store)
	w.Stop()

	mu.Lock()
	defer mu.Unlock()
	if singleCalls != 0 {
		t.Errorf("EmbedFn must NOT be called when EmbedBatchFn is wired, got %d calls", singleCalls)
	}
	if batchCalls == 0 {
		t.Error("expected EmbedBatchFn to be called at least once")
	}
	if len(sawTexts) == 0 {
		t.Error("expected at least one batch with texts")
	}
}

func TestDocIngestionWorker_EmbedBatchFailureKeepsChunksWithoutVectors(t *testing.T) {
	// Batch HTTP error must NOT block ingest. Chunks still get persisted, just
	// without embeddings (RAG falls back to FTS5 keyword search).
	store := newTrackingStore()
	media := newMockMediaStore()

	embedBatchFn := func(_ context.Context, _ []string) ([][]float32, error) {
		return nil, errors.New("provider exploded")
	}

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:        store,
		Extractor:    rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:      rag.FixedSizeChunker{},
		EmbedBatchFn: embedBatchFn,
		MediaStore:   media,
		ChunkOpts:    rag.ChunkOptions{Size: 30, Overlap: 0},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	w.Enqueue(rag.IngestionJob{
		DocID: "batch-fail", Namespace: "global", Title: "Will-fail-embed",
		Content: "alpha beta gamma delta epsilon zeta eta theta iota kappa", MIME: "text/plain",
	})
	waitForDoc(t, store)
	w.Stop()

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.docs[0].ChunkCount == 0 {
		t.Error("expected chunks persisted even when embed fails")
	}
	for _, c := range store.chunks["batch-fail"] {
		if c.Embedding != nil {
			t.Errorf("chunk %s: expected nil embedding on batch failure, got %d-dim", c.ID, len(c.Embedding))
		}
	}
}

func TestDocIngestionWorker_FallsBackToEmbedFnWhenNoBatch(t *testing.T) {
	// Provider without EmbedBatch → worker uses single-call EmbedFn.
	store := newTrackingStore()
	media := newMockMediaStore()

	var singleCalls int32
	var mu sync.Mutex
	embedFn := func(_ context.Context, _ string) ([]float32, error) {
		mu.Lock()
		singleCalls++
		mu.Unlock()
		return []float32{0.7}, nil
	}

	w := rag.NewDocIngestionWorker(rag.DocIngestionWorkerConfig{
		Store:      store,
		Extractor:  rag.NewSelectExtractor(rag.PlainTextExtractor{}),
		Chunker:    rag.FixedSizeChunker{},
		EmbedFn:    embedFn,
		MediaStore: media,
		ChunkOpts:  rag.ChunkOptions{Size: 30, Overlap: 0},
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)

	w.Enqueue(rag.IngestionJob{
		DocID: "fallback-1", Namespace: "global", Title: "No batch",
		Content: "alpha beta gamma delta epsilon zeta eta theta", MIME: "text/plain",
	})
	waitForDoc(t, store)
	w.Stop()

	mu.Lock()
	defer mu.Unlock()
	if singleCalls == 0 {
		t.Error("expected EmbedFn to be called when EmbedBatchFn is nil")
	}
}
