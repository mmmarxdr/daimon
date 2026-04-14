package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func makeTestCostRecord(sessionID, channelID, model string, inTokens, outTokens int) CostRecord {
	return CostRecord{
		ID:            uuid.New().String(),
		SessionID:     sessionID,
		ChannelID:     channelID,
		Model:         model,
		InputTokens:   inTokens,
		OutputTokens:  outTokens,
		InputCostUSD:  float64(inTokens) * 0.0000025,
		OutputCostUSD: float64(outTokens) * 0.00001,
		TotalCostUSD:  float64(inTokens)*0.0000025 + float64(outTokens)*0.00001,
		Timestamp:     time.Now().UTC(),
	}
}

// TestCostStore_RecordAndSummarize verifies basic record-and-aggregate flow.
func TestCostStore_RecordAndSummarize(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	records := []CostRecord{
		makeTestCostRecord("sess-1", "chan-1", "gpt-4o", 1000, 500),
		makeTestCostRecord("sess-1", "chan-1", "gpt-4o", 2000, 1000),
		makeTestCostRecord("sess-1", "chan-1", "deepseek-v3", 3000, 1500),
	}

	for _, r := range records {
		if err := s.RecordCost(ctx, r); err != nil {
			t.Fatalf("RecordCost(%s): %v", r.ID, err)
		}
	}

	summary, err := s.GetCostSummary(ctx, CostFilter{})
	if err != nil {
		t.Fatalf("GetCostSummary: %v", err)
	}

	if summary.RecordCount != 3 {
		t.Errorf("RecordCount = %d, want 3", summary.RecordCount)
	}
	if summary.TotalInputTokens != 6000 {
		t.Errorf("TotalInputTokens = %d, want 6000", summary.TotalInputTokens)
	}
	if summary.TotalOutputTokens != 3000 {
		t.Errorf("TotalOutputTokens = %d, want 3000", summary.TotalOutputTokens)
	}
	if len(summary.ByModel) != 2 {
		t.Errorf("ByModel count = %d, want 2", len(summary.ByModel))
	}
}

// TestCostStore_FilterByModel verifies model filtering.
func TestCostStore_FilterByModel(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	records := []CostRecord{
		makeTestCostRecord("sess-1", "chan-1", "gpt-4o", 1000, 500),
		makeTestCostRecord("sess-1", "chan-1", "deepseek-v3", 2000, 1000),
		makeTestCostRecord("sess-1", "chan-1", "gpt-4o", 500, 250),
	}

	for _, r := range records {
		if err := s.RecordCost(ctx, r); err != nil {
			t.Fatalf("RecordCost: %v", err)
		}
	}

	summary, err := s.GetCostSummary(ctx, CostFilter{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("GetCostSummary: %v", err)
	}

	if summary.RecordCount != 2 {
		t.Errorf("RecordCount = %d, want 2", summary.RecordCount)
	}
	if summary.TotalInputTokens != 1500 {
		t.Errorf("TotalInputTokens = %d, want 1500", summary.TotalInputTokens)
	}
}

// TestCostStore_FilterByChannel verifies channel filtering.
func TestCostStore_FilterByChannel(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	records := []CostRecord{
		makeTestCostRecord("sess-1", "chan-1", "gpt-4o", 1000, 500),
		makeTestCostRecord("sess-2", "chan-2", "gpt-4o", 2000, 1000),
	}

	for _, r := range records {
		if err := s.RecordCost(ctx, r); err != nil {
			t.Fatalf("RecordCost: %v", err)
		}
	}

	summary, err := s.GetCostSummary(ctx, CostFilter{ChannelID: "chan-1"})
	if err != nil {
		t.Fatalf("GetCostSummary: %v", err)
	}

	if summary.RecordCount != 1 {
		t.Errorf("RecordCount = %d, want 1", summary.RecordCount)
	}
}

// TestCostStore_EmptyResult verifies that GetCostSummary returns a zero-value
// summary (not an error) when no records match.
func TestCostStore_EmptyResult(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	summary, err := s.GetCostSummary(ctx, CostFilter{})
	if err != nil {
		t.Fatalf("GetCostSummary on empty store: %v", err)
	}
	if summary.RecordCount != 0 {
		t.Errorf("RecordCount = %d, want 0", summary.RecordCount)
	}
	if summary.TotalCostUSD != 0 {
		t.Errorf("TotalCostUSD = %g, want 0", summary.TotalCostUSD)
	}
	if len(summary.ByModel) != 0 {
		t.Errorf("ByModel count = %d, want 0", len(summary.ByModel))
	}
}

// TestCostStore_ByModelOrderedByCostDesc verifies that the per-model breakdown
// is ordered by total cost descending.
func TestCostStore_ByModelOrderedByCostDesc(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	// Insert records where gpt-4o costs more than deepseek-v3.
	records := []CostRecord{
		makeTestCostRecord("sess-1", "chan-1", "deepseek-v3", 100, 50),
		makeTestCostRecord("sess-1", "chan-1", "gpt-4o", 10000, 5000),
	}

	for _, r := range records {
		if err := s.RecordCost(ctx, r); err != nil {
			t.Fatalf("RecordCost: %v", err)
		}
	}

	summary, err := s.GetCostSummary(ctx, CostFilter{})
	if err != nil {
		t.Fatalf("GetCostSummary: %v", err)
	}

	if len(summary.ByModel) != 2 {
		t.Fatalf("ByModel count = %d, want 2", len(summary.ByModel))
	}
	if summary.ByModel[0].Model != "gpt-4o" {
		t.Errorf("first ByModel model = %q, want gpt-4o (highest cost)", summary.ByModel[0].Model)
	}
	if summary.ByModel[1].Model != "deepseek-v3" {
		t.Errorf("second ByModel model = %q, want deepseek-v3", summary.ByModel[1].Model)
	}
}
