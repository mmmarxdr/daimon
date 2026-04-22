package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"daimon/internal/rag"
)

// fakeDocStore is a minimal in-memory rag.DocumentStore used by knowledge
// handler tests. Only the methods hit by the handlers are meaningfully
// implemented — the rest are no-ops that satisfy the interface.
type fakeDocStore struct {
	mu         sync.Mutex
	docs       []rag.Document
	deleted    []string
	deleteErr  error
	listErr    error
	notFound   bool
}

func (f *fakeDocStore) AddDocument(_ context.Context, d rag.Document) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.docs = append(f.docs, d)
	return nil
}
func (f *fakeDocStore) AddChunks(_ context.Context, _ string, _ []rag.DocumentChunk) error {
	return nil
}
func (f *fakeDocStore) SearchChunks(_ context.Context, _ string, _ []float32, _ rag.SearchOptions) ([]rag.SearchResult, error) {
	return nil, nil
}
func (f *fakeDocStore) DeleteDocument(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if f.notFound {
		return rag.ErrDocNotFound
	}
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeDocStore) ListDocuments(_ context.Context, _ string) ([]rag.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]rag.Document, len(f.docs))
	copy(out, f.docs)
	return out, nil
}
func (f *fakeDocStore) GetDocument(_ context.Context, id string) (rag.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.docs {
		if d.ID == id {
			return d, nil
		}
	}
	return rag.Document{}, rag.ErrDocNotFound
}

func newTestServerWithDocStore(t *testing.T, docs *fakeDocStore) *Server {
	t.Helper()
	s := &Server{
		deps: ServerDeps{
			Store:     &fakeWebStore{},
			StartedAt: time.Now(),
			Config:    minimalConfig(),
			DocStore:  docs,
		},
		mux: http.NewServeMux(),
	}
	s.routes()
	return s
}

func TestHandleListKnowledge_returnsDocsWithTrustFields(t *testing.T) {
	accessed := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	pages := 12
	docs := &fakeDocStore{
		docs: []rag.Document{
			{
				ID: "d1", Namespace: "global", Title: "Payments arch",
				MIME: "text/markdown", ChunkCount: 3,
				AccessCount: 5, LastAccessedAt: &accessed,
				Summary: "webhook pipeline",
				PageCount: &pages,
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			},
		},
	}
	srv := newTestServerWithDocStore(t, docs)

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []knowledgeDocResponse `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	got := resp.Items[0]
	if got.KindHint != "markdown" {
		t.Errorf("kind_hint: expected markdown, got %q", got.KindHint)
	}
	if got.Status != "ready" {
		t.Errorf("status: expected ready (chunks>0), got %q", got.Status)
	}
	if got.AccessCount != 5 {
		t.Errorf("access_count: expected 5, got %d", got.AccessCount)
	}
	if got.Summary != "webhook pipeline" {
		t.Errorf("summary: expected set, got %q", got.Summary)
	}
	if got.PageCount == nil || *got.PageCount != 12 {
		t.Errorf("page_count: expected *12, got %v", got.PageCount)
	}
	if got.LastAccessedAt != "2026-04-01T10:00:00Z" {
		t.Errorf("last_accessed_at: expected 2026-04-01T10:00:00Z, got %q", got.LastAccessedAt)
	}
}

func TestHandleListKnowledge_statusIndexingBeforeWorkerRuns(t *testing.T) {
	docs := &fakeDocStore{
		docs: []rag.Document{
			{ID: "d1", Title: "fresh upload", ChunkCount: 0, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		},
	}
	srv := newTestServerWithDocStore(t, docs)

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var resp struct {
		Items []knowledgeDocResponse `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Items[0].Status != "indexing" {
		t.Errorf("expected status=indexing when IngestedAt is nil, got %q", resp.Items[0].Status)
	}
}

func TestHandleListKnowledge_statusEmptyWhenWorkerProducedNoChunks(t *testing.T) {
	// Worker ran (IngestedAt is set) but text extraction yielded zero chunks —
	// e.g. a PDF with image-only or LaTeX-CID-encoded text. Must surface as
	// "empty" so the UI doesn't claim "indexing" forever.
	ingested := time.Now()
	docs := &fakeDocStore{
		docs: []rag.Document{
			{
				ID: "d1", Title: "untextable.pdf", ChunkCount: 0,
				IngestedAt: &ingested,
				CreatedAt:  time.Now(), UpdatedAt: time.Now(),
			},
		},
	}
	srv := newTestServerWithDocStore(t, docs)

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var resp struct {
		Items []knowledgeDocResponse `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Items[0].Status != "empty" {
		t.Errorf("expected status=empty when worker ran with no chunks, got %q", resp.Items[0].Status)
	}
}

func TestHandleListKnowledge_notImplementedWhenDocStoreNil(t *testing.T) {
	srv := &Server{
		deps: ServerDeps{
			Store: &fakeWebStore{}, StartedAt: time.Now(), Config: minimalConfig(),
		},
		mux: http.NewServeMux(),
	}
	srv.routes()

	req := httptest.NewRequest(http.MethodGet, "/api/knowledge", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 when DocStore nil, got %d", w.Code)
	}
}

func TestHandleDeleteKnowledge_removesDoc(t *testing.T) {
	docs := &fakeDocStore{
		docs: []rag.Document{{ID: "d1", Title: "to delete"}},
	}
	srv := newTestServerWithDocStore(t, docs)

	req := httptest.NewRequest(http.MethodDelete, "/api/knowledge/d1", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if len(docs.deleted) != 1 || docs.deleted[0] != "d1" {
		t.Errorf("expected DeleteDocument called with d1, got %v", docs.deleted)
	}
}

func TestHandleDeleteKnowledge_notFound(t *testing.T) {
	docs := &fakeDocStore{notFound: true}
	srv := newTestServerWithDocStore(t, docs)

	req := httptest.NewRequest(http.MethodDelete, "/api/knowledge/missing", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMimeToKindHint(t *testing.T) {
	cases := map[string]string{
		"application/pdf":  "pdf",
		"text/markdown":    "markdown",
		"text/html":        "html",
		"application/zip":  "zip",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document": "docx",
		"text/plain":       "plain",
		"":                 "plain",
		"APPLICATION/PDF":  "pdf",
	}
	for mime, want := range cases {
		if got := mimeToKindHint(mime); got != want {
			t.Errorf("mime %q: expected %q, got %q", mime, want, got)
		}
	}
}
