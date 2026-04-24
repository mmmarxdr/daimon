package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"daimon/internal/store"
)

// fakeCostWebStore extends fakeWebStore with the optional CostStore interface
// so the metrics handler can read GetDailyCostHistory. Inherits everything
// else from fakeWebStore (memory, conversations, etc.).
type fakeCostWebStore struct {
	fakeWebStore
	history []store.DailyCost
}

func (f *fakeCostWebStore) RecordCost(_ context.Context, _ store.CostRecord) error {
	return nil
}

func (f *fakeCostWebStore) GetCostSummary(_ context.Context, _ store.CostFilter) (store.CostSummary, error) {
	return store.CostSummary{}, nil
}

func (f *fakeCostWebStore) GetDailyCostHistory(_ context.Context, _ int) ([]store.DailyCost, error) {
	return f.history, nil
}

func (f *fakeCostWebStore) GetLastCallTokens(_ context.Context) (int64, string, error) {
	return 0, "", nil
}

func TestHandleGetMetrics_withCostStore(t *testing.T) {
	today := time.Now().UTC().Format("2006-01-02")
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")

	st := &fakeCostWebStore{
		history: []store.DailyCost{
			{Date: yesterday, InputTokens: 300, OutputTokens: 200, TotalCostUSD: 0.005, Conversations: 1, Messages: 4},
			{Date: today, InputTokens: 700, OutputTokens: 300, TotalCostUSD: 0.01, Conversations: 2, Messages: 6},
		},
	}

	srv := newTestServerWithStore(t, st)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	today2, ok := resp["today"].(map[string]any)
	if !ok {
		t.Fatalf("expected today object, got %T", resp["today"])
	}
	if today2["input_tokens"].(float64) != 700 {
		t.Errorf("today.input_tokens = %v, want 700", today2["input_tokens"])
	}
	if today2["cost_usd"].(float64) != 0.01 {
		t.Errorf("today.cost_usd = %v, want 0.01", today2["cost_usd"])
	}
	if today2["conversations"].(float64) != 2 {
		t.Errorf("today.conversations = %v, want 2", today2["conversations"])
	}
	if today2["messages"].(float64) != 6 {
		t.Errorf("today.messages = %v, want 6", today2["messages"])
	}

	month, ok := resp["month"].(map[string]any)
	if !ok {
		t.Fatalf("expected month object, got %T", resp["month"])
	}
	// month sums all history: 300+700 = 1000 input, 200+300 = 500 output, 0.015 cost
	if month["input_tokens"].(float64) != 1000 {
		t.Errorf("month.input_tokens = %v, want 1000", month["input_tokens"])
	}
	if month["cost_usd"].(float64) != 0.015 {
		t.Errorf("month.cost_usd = %v, want 0.015", month["cost_usd"])
	}

	history, ok := resp["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", resp["history"])
	}
	if len(history) != 2 {
		t.Errorf("history length = %d, want 2", len(history))
	}
}

func TestHandleGetMetrics_noCostStore(t *testing.T) {
	// noWebStore implements store.Store but NOT store.CostStore — handler
	// must degrade gracefully and return zeros.
	srv := newTestServerWithStore(t, &noWebStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	today, ok := resp["today"].(map[string]any)
	if !ok {
		t.Fatalf("expected today object, got %T", resp["today"])
	}
	if today["cost_usd"].(float64) != 0 {
		t.Errorf("today.cost_usd = %v, want 0", today["cost_usd"])
	}
	if today["input_tokens"].(float64) != 0 {
		t.Errorf("today.input_tokens = %v, want 0", today["input_tokens"])
	}
}

func TestHandleGetMetricsHistory_withCostStore(t *testing.T) {
	st := &fakeCostWebStore{
		history: []store.DailyCost{
			{Date: "2026-04-10", InputTokens: 60, OutputTokens: 40, TotalCostUSD: 0.001, Messages: 2},
			{Date: "2026-04-11", InputTokens: 120, OutputTokens: 80, TotalCostUSD: 0.002, Messages: 4},
		},
	}

	srv := newTestServerWithStore(t, st)

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/history?days=7", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var snap map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}

	history, ok := snap["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", snap["history"])
	}
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
}

func TestHandleGetMetricsHistory_noCostStore(t *testing.T) {
	srv := newTestServerWithStore(t, &noWebStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/history", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var snap map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}

	history, ok := snap["history"].([]any)
	if !ok {
		t.Fatalf("expected history array, got %T", snap["history"])
	}
	if len(history) != 0 {
		t.Errorf("history length = %d, want 0", len(history))
	}
}

func TestHandleGetMetricsHistory_daysClamped(t *testing.T) {
	srv := newTestServerWithStore(t, &fakeCostWebStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/metrics/history?days=999", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	// Should not panic and return 200
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
