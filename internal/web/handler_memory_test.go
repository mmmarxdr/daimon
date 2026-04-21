package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/store"
)

func TestHandleListMemory_returnsItems(t *testing.T) {
	fs := &fakeWebStore{
		memory: []store.MemoryEntry{
			{ID: "1", ScopeID: "s1", Content: "remember this"},
			{ID: "2", ScopeID: "s1", Content: "and this"},
		},
	}
	srv := newTestServerWithStore(t, fs)

	req := httptest.NewRequest(http.MethodGet, "/api/memory", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	items, _ := resp["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestHandleListMemory_limitParam(t *testing.T) {
	fs := &fakeWebStore{}
	for i := range 10 {
		fs.memory = append(fs.memory, store.MemoryEntry{
			ID: string(rune('a' + i)), ScopeID: "s", Content: "x",
		})
	}
	srv := newTestServerWithStore(t, fs)

	req := httptest.NewRequest(http.MethodGet, "/api/memory?limit=3", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	items, _ := resp["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
}

func TestHandlePostMemory_ok(t *testing.T) {
	fs := &fakeWebStore{}
	srv := newTestServerWithStore(t, fs)

	entry := store.MemoryEntry{
		ScopeID: "scope1",
		Content: "test memory",
		Title:   "my note",
	}
	body, _ := json.Marshal(entry)

	req := httptest.NewRequest(http.MethodPost, "/api/memory", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	if len(fs.memory) != 1 {
		t.Fatalf("expected 1 entry in store, got %d", len(fs.memory))
	}
}

func TestHandlePostMemory_badJSON(t *testing.T) {
	fs := &fakeWebStore{}
	srv := newTestServerWithStore(t, fs)

	req := httptest.NewRequest(http.MethodPost, "/api/memory", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleDeleteMemory_noWebStore_returns501(t *testing.T) {
	srv := newTestServerWithStore(t, noWebStore{})

	req := httptest.NewRequest(http.MethodDelete, "/api/memory/1", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", w.Code)
	}
}

func TestHandleDeleteMemory_notFound(t *testing.T) {
	fs := &fakeWebStore{}
	srv := newTestServerWithStore(t, fs)

	req := httptest.NewRequest(http.MethodDelete, "/api/memory/999", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleListMemory_exposesClusterAndTrustFields(t *testing.T) {
	// An entry with cluster + importance + access metadata should round-trip
	// through the API and arrive at the frontend with all trust-surface fields.
	lastAccessed := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	fs := &fakeWebStore{
		memory: []store.MemoryEntry{
			{
				ID:             "m1",
				ScopeID:        "s1",
				Content:        "prefers Go for backend",
				Tags:           []string{"lang"},
				Type:           "preference",
				Cluster:        "preferences",
				Importance:     8,
				AccessCount:    4,
				LastAccessedAt: &lastAccessed,
				Source:         "conv-abc",
				CreatedAt:      time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC),
			},
		},
	}
	srv := newTestServerWithStore(t, fs)

	req := httptest.NewRequest(http.MethodGet, "/api/memory", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Items []memoryEntryResponse `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	got := resp.Items[0]
	if got.Cluster != "preferences" {
		t.Errorf("cluster: expected preferences, got %q", got.Cluster)
	}
	if got.Importance != 8 {
		t.Errorf("importance: expected 8, got %d", got.Importance)
	}
	if got.AccessCount != 4 {
		t.Errorf("access_count: expected 4, got %d", got.AccessCount)
	}
	if got.Type != "preference" {
		t.Errorf("type: expected preference, got %q", got.Type)
	}
	if got.LastAccessedAt != "2026-04-19T12:00:00Z" {
		t.Errorf("last_accessed_at: expected 2026-04-19T12:00:00Z, got %q", got.LastAccessedAt)
	}
	if got.SourceConversationID != "conv-abc" {
		t.Errorf("source_conversation_id: expected conv-abc, got %q", got.SourceConversationID)
	}
}

func TestHandleListMemory_clusterDefaultsToGeneral(t *testing.T) {
	// Entries written before v11 have cluster == "" — the handler must default
	// to "general" so the frontend never sees a blank bucket.
	fs := &fakeWebStore{
		memory: []store.MemoryEntry{
			{ID: "legacy", ScopeID: "s1", Content: "old entry", Cluster: ""},
		},
	}
	srv := newTestServerWithStore(t, fs)

	req := httptest.NewRequest(http.MethodGet, "/api/memory", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var resp struct {
		Items []memoryEntryResponse `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Items[0].Cluster != "general" {
		t.Errorf("expected Cluster default 'general', got %q", resp.Items[0].Cluster)
	}
}

func TestHandleListMemory_derivesConvTitleFromFirstUserMessage(t *testing.T) {
	// source_conversation_title is derived from the first user message of the
	// memory's source conversation, truncated to 60 runes. Cached per request.
	fs := &fakeWebStore{
		conversations: []store.Conversation{
			{
				ID: "conv-1",
				Messages: []provider.ChatMessage{
					{Role: "assistant", Content: content.TextBlock("hi")},
					{Role: "user", Content: content.TextBlock("Payment service anomalies — help me debug this.")},
				},
			},
		},
		memory: []store.MemoryEntry{
			{ID: "m1", ScopeID: "s1", Content: "fixed", Source: "conv-1"},
			{ID: "m2", ScopeID: "s1", Content: "also fixed", Source: "conv-1"}, // same conv → cache hit
			{ID: "m3", ScopeID: "s1", Content: "orphan", Source: "conv-missing"},
		},
	}
	srv := newTestServerWithStore(t, fs)

	req := httptest.NewRequest(http.MethodGet, "/api/memory", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var resp struct {
		Items []memoryEntryResponse `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Items[0].SourceConversationTitle != "Payment service anomalies — help me debug this." {
		t.Errorf("item 0: expected full user msg as title, got %q", resp.Items[0].SourceConversationTitle)
	}
	if resp.Items[1].SourceConversationTitle != resp.Items[0].SourceConversationTitle {
		t.Errorf("item 1: expected cached title, got %q", resp.Items[1].SourceConversationTitle)
	}
	if resp.Items[2].SourceConversationTitle != "" {
		t.Errorf("item 2: expected empty title for missing conv, got %q", resp.Items[2].SourceConversationTitle)
	}
}
