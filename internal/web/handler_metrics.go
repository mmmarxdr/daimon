package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"daimon/internal/audit"
	"daimon/internal/store"
)

// metricsDay matches the frontend MetricsSnapshot.today / month shape.
type metricsDay struct {
	InputTokens   int64   `json:"input_tokens"`
	OutputTokens  int64   `json:"output_tokens"`
	CostUSD       float64 `json:"cost_usd"`
	Conversations int     `json:"conversations"`
	Messages      int     `json:"messages"`
	// LastCallInputTokens is the input_tokens of the most recent LLM call
	// across all conversations. Surfaced to the dashboard sidebar as a
	// "last turn context" indicator — distinct from InputTokens which is
	// today's running total across every conv (and would otherwise be
	// mistaken for context-window utilisation).
	LastCallInputTokens int64  `json:"last_call_input_tokens"`
	LastCallModel       string `json:"last_call_model,omitempty"`
	// LastCallContextLength is the max context window (in tokens) of the
	// model that produced LastCallInputTokens. Used by the sidebar as the
	// denominator for the context-utilisation bar so the fill matches the
	// model's true capacity (e.g. 1M for deepseek-v4-pro), not the agent's
	// self-imposed cap. 0 when the model is unknown to the runtime lookup.
	LastCallContextLength int `json:"last_call_context_length,omitempty"`
}

// metricsMonth matches MetricsSnapshot.month (no conversations/messages).
type metricsMonth struct {
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// metricsHistoryEntry matches MetricsSnapshot.history[].
type metricsHistoryEntry struct {
	Date         string  `json:"date"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// metricsSnapshot is the full shape the frontend expects for MetricsSnapshot.
type metricsSnapshot struct {
	Today   metricsDay            `json:"today"`
	Month   metricsMonth          `json:"month"`
	History []metricsHistoryEntry `json:"history"`
}

// buildMetricsSnapshot constructs the full MetricsSnapshot for the given context.
//
// Reads per-day token/cost/usage from store.CostStore (populated by the agent
// loop after every LLM call). The audit subsystem is intentionally NOT a
// dependency here — metrics must work with the default config where audit is
// disabled. Audit is for security/compliance, cost is for the dashboard.
func (s *Server) buildMetricsSnapshot(ctx context.Context) metricsSnapshot {
	snap := metricsSnapshot{
		History: []metricsHistoryEntry{},
	}

	cs, ok := s.deps.Store.(store.CostStore)
	if !ok {
		return snap
	}

	if last, lastModel, err := cs.GetLastCallTokens(ctx); err == nil {
		snap.Today.LastCallInputTokens = last
		snap.Today.LastCallModel = lastModel
		if ctxLen, ok := audit.LookupContextLength(lastModel); ok {
			snap.Today.LastCallContextLength = ctxLen
		}
	}

	const monthDays = 30
	history, err := cs.GetDailyCostHistory(ctx, monthDays)
	if err != nil {
		return snap
	}

	todayKey := time.Now().UTC().Format("2006-01-02")
	for _, d := range history {
		snap.Month.InputTokens += d.InputTokens
		snap.Month.OutputTokens += d.OutputTokens
		snap.Month.CostUSD += d.TotalCostUSD
		snap.History = append(snap.History, metricsHistoryEntry{
			Date:         d.Date,
			InputTokens:  d.InputTokens,
			OutputTokens: d.OutputTokens,
			CostUSD:      d.TotalCostUSD,
		})
		if d.Date == todayKey {
			snap.Today.InputTokens = d.InputTokens
			snap.Today.OutputTokens = d.OutputTokens
			snap.Today.CostUSD = d.TotalCostUSD
			snap.Today.Conversations = d.Conversations
			snap.Today.Messages = d.Messages
		}
	}

	return snap
}

func (s *Server) handleGetMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.buildMetricsSnapshot(r.Context()))
}

func (s *Server) handleGetMetricsHistory(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}

	snap := metricsSnapshot{
		History: []metricsHistoryEntry{},
	}

	cs, ok := s.deps.Store.(store.CostStore)
	if !ok {
		writeJSON(w, http.StatusOK, snap)
		return
	}

	history, err := cs.GetDailyCostHistory(r.Context(), days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, d := range history {
		snap.Month.InputTokens += d.InputTokens
		snap.Month.OutputTokens += d.OutputTokens
		snap.Month.CostUSD += d.TotalCostUSD
		snap.History = append(snap.History, metricsHistoryEntry{
			Date:         d.Date,
			InputTokens:  d.InputTokens,
			OutputTokens: d.OutputTokens,
			CostUSD:      d.TotalCostUSD,
		})
	}

	// today is the last entry in the history if present (GetDailyCostHistory
	// returns one entry per day with today as the last).
	if len(history) > 0 {
		last := history[len(history)-1]
		snap.Today.InputTokens = last.InputTokens
		snap.Today.OutputTokens = last.OutputTokens
		snap.Today.CostUSD = last.TotalCostUSD
		snap.Today.Conversations = last.Conversations
		snap.Today.Messages = last.Messages
	}

	writeJSON(w, http.StatusOK, snap)
}
