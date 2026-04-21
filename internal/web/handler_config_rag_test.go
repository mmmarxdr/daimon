package web

// T15 – PUT /api/config accepts retrieval sub-tree and values persist.
// T16 – PUT /api/config preserves unspecified retrieval fields (no reset regression).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"daimon/internal/config"
)

// ---------------------------------------------------------------------------
// T15 – PUT accepts rag.retrieval sub-tree; values survive round-trip
// ---------------------------------------------------------------------------

func TestHandlePutConfig_RAGRetrieval_AcceptsSubtree(t *testing.T) {
	cfg := minimalConfig()
	cfg.Web.AuthToken = "secret-token"

	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	s := newConfigTestServer(cfg, cfgPath)

	body := []byte(`{"rag":{"retrieval":{"neighbor_radius":2,"min_cosine_score":0.7}}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader("secret-token"))
	rec := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("T15: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Verify in-memory config updated.
	got := s.deps.Config.RAG.Retrieval
	if got.NeighborRadius != 2 {
		t.Errorf("T15: NeighborRadius: want 2, got %d", got.NeighborRadius)
	}
	if got.MinCosineScore != 0.7 {
		t.Errorf("T15: MinCosineScore: want 0.7, got %f", got.MinCosineScore)
	}

	// Verify response body also reflects the values.
	var respCfg config.Config
	if err := json.NewDecoder(rec.Body).Decode(&respCfg); err != nil {
		t.Fatalf("T15: decode response: %v", err)
	}
	if respCfg.RAG.Retrieval.NeighborRadius != 2 {
		t.Errorf("T15: response NeighborRadius: want 2, got %d", respCfg.RAG.Retrieval.NeighborRadius)
	}
	if respCfg.RAG.Retrieval.MinCosineScore != 0.7 {
		t.Errorf("T15: response MinCosineScore: want 0.7, got %f", respCfg.RAG.Retrieval.MinCosineScore)
	}
}

// ---------------------------------------------------------------------------
// T16 – Unspecified retrieval fields are preserved (no reset-to-zero regression)
// ---------------------------------------------------------------------------

func TestHandlePutConfig_RAGRetrieval_PreservesUnspecifiedFields(t *testing.T) {
	cfg := minimalConfig()
	cfg.Web.AuthToken = "secret-token"
	// Seed existing retrieval config.
	cfg.RAG.Retrieval = config.RAGRetrievalConf{
		NeighborRadius: 3,
		MaxBM25Score:   1.5,
		MinCosineScore: 0.0,
	}

	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	s := newConfigTestServer(cfg, cfgPath)

	// Only PUT min_cosine_score — the other two must be preserved.
	body := []byte(`{"rag":{"retrieval":{"min_cosine_score":0.8}}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader("secret-token"))
	rec := httptest.NewRecorder()
	s.srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("T16: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	got := s.deps.Config.RAG.Retrieval
	if got.NeighborRadius != 3 {
		t.Errorf("T16: NeighborRadius should be preserved as 3, got %d", got.NeighborRadius)
	}
	if got.MaxBM25Score != 1.5 {
		t.Errorf("T16: MaxBM25Score should be preserved as 1.5, got %f", got.MaxBM25Score)
	}
	if got.MinCosineScore != 0.8 {
		t.Errorf("T16: MinCosineScore should be updated to 0.8, got %f", got.MinCosineScore)
	}
}
