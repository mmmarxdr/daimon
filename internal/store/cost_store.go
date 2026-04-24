package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RecordCost inserts a cost record for a single LLM call.
func (s *SQLiteStore) RecordCost(ctx context.Context, record CostRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cost_records
			(id, session_id, channel_id, model,
			 input_tokens, output_tokens,
			 input_cost_usd, output_cost_usd, total_cost_usd,
			 created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.SessionID,
		record.ChannelID,
		record.Model,
		record.InputTokens,
		record.OutputTokens,
		record.InputCostUSD,
		record.OutputCostUSD,
		record.TotalCostUSD,
		record.Timestamp.UTC(),
	)
	if err != nil {
		return fmt.Errorf("recording cost %s: %w", record.ID, err)
	}
	return nil
}

// GetCostSummary returns aggregated cost data matching the filter.
func (s *SQLiteStore) GetCostSummary(ctx context.Context, filter CostFilter) (CostSummary, error) {
	where, args := s.buildCostWhere(filter)

	var summary CostSummary

	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(input_tokens), 0),
		        COALESCE(SUM(output_tokens), 0),
		        COALESCE(SUM(total_cost_usd), 0),
		        COUNT(*)
		 FROM cost_records`+where,
		args...,
	).Scan(&summary.TotalInputTokens, &summary.TotalOutputTokens, &summary.TotalCostUSD, &summary.RecordCount)
	if err != nil {
		return summary, fmt.Errorf("querying cost summary: %w", err)
	}

	// Per-model breakdown.
	byModel, err := s.getCostByModel(ctx, where, args)
	if err != nil {
		return summary, err
	}
	summary.ByModel = byModel

	return summary, nil
}

// getCostByModel returns per-model cost breakdown within the filtered set.
func (s *SQLiteStore) getCostByModel(ctx context.Context, where string, args []any) ([]CostModelCost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT model,
		        SUM(input_tokens),
		        SUM(output_tokens),
		        SUM(total_cost_usd),
		        COUNT(*)
		 FROM cost_records`+where+`
		 GROUP BY model
		 ORDER BY SUM(total_cost_usd) DESC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("querying cost by model: %w", err)
	}
	defer rows.Close()

	var results []CostModelCost
	for rows.Next() {
		var m CostModelCost
		if err := rows.Scan(&m.Model, &m.InputTokens, &m.OutputTokens, &m.TotalCostUSD, &m.CallCount); err != nil {
			return nil, fmt.Errorf("scanning cost model row: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating cost model rows: %w", err)
	}
	if results == nil {
		results = []CostModelCost{}
	}
	return results, nil
}

// buildCostWhere constructs a WHERE clause from CostFilter.
// Returns " WHERE ..." (with leading space) or "" if no filters are set.
func (s *SQLiteStore) buildCostWhere(filter CostFilter) (string, []any) {
	var clauses []string
	var args []any

	if filter.SessionID != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, filter.SessionID)
	}
	if filter.ChannelID != "" {
		clauses = append(clauses, "channel_id = ?")
		args = append(args, filter.ChannelID)
	}
	if filter.Model != "" {
		clauses = append(clauses, "model = ?")
		args = append(args, filter.Model)
	}
	if !filter.Since.IsZero() {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, filter.Since.UTC())
	}
	if !filter.Until.IsZero() {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, filter.Until.UTC())
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// GetDailyCostHistory returns aggregated cost/token/usage data per calendar
// day (UTC) for the last `days` days inclusive. Days with no records are
// zero-filled so chart consumers get a continuous time axis. The last entry
// is always today (UTC).
//
// Daily aggregation runs in two queries — one for token/cost/message counts,
// one for distinct-conversation counts — joined in Go. SQLite doesn't have
// a clean way to express "COUNT(*) and COUNT(DISTINCT session_id) per day"
// in a single grouped query without a subselect that complicates indexing.
func (s *SQLiteStore) GetDailyCostHistory(ctx context.Context, days int) ([]DailyCost, error) {
	if days <= 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	start := now.AddDate(0, 0, -(days - 1))
	startDate := start.Format("2006-01-02")

	type dayAgg struct {
		inputTokens  int64
		outputTokens int64
		totalCostUSD float64
		messages     int
	}

	tokenRows, err := s.db.QueryContext(ctx, `
		SELECT
			substr(created_at, 1, 10) AS day,
			COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0),
			COALESCE(SUM(total_cost_usd), 0),
			COUNT(*)
		FROM cost_records
		WHERE substr(created_at, 1, 10) >= ?
		GROUP BY substr(created_at, 1, 10)
	`, startDate)
	if err != nil {
		return nil, fmt.Errorf("querying daily cost history: %w", err)
	}
	defer tokenRows.Close()

	byDay := make(map[string]dayAgg)
	for tokenRows.Next() {
		var day string
		var a dayAgg
		if err := tokenRows.Scan(&day, &a.inputTokens, &a.outputTokens, &a.totalCostUSD, &a.messages); err != nil {
			return nil, fmt.Errorf("scanning daily cost row: %w", err)
		}
		byDay[day] = a
	}
	if err := tokenRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating daily cost rows: %w", err)
	}

	convoRows, err := s.db.QueryContext(ctx, `
		SELECT
			substr(created_at, 1, 10) AS day,
			COUNT(DISTINCT session_id)
		FROM cost_records
		WHERE substr(created_at, 1, 10) >= ?
		  AND session_id IS NOT NULL AND session_id != ''
		GROUP BY substr(created_at, 1, 10)
	`, startDate)
	if err != nil {
		return nil, fmt.Errorf("querying daily conversation count: %w", err)
	}
	defer convoRows.Close()

	convoByDay := make(map[string]int)
	for convoRows.Next() {
		var day string
		var n int
		if err := convoRows.Scan(&day, &n); err != nil {
			return nil, fmt.Errorf("scanning daily conv row: %w", err)
		}
		convoByDay[day] = n
	}
	if err := convoRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating daily conv rows: %w", err)
	}

	result := make([]DailyCost, 0, days)
	for i := range days {
		day := start.AddDate(0, 0, i).Format("2006-01-02")
		a := byDay[day]
		result = append(result, DailyCost{
			Date:          day,
			InputTokens:   a.inputTokens,
			OutputTokens:  a.outputTokens,
			TotalCostUSD:  a.totalCostUSD,
			Messages:      a.messages,
			Conversations: convoByDay[day],
		})
	}
	return result, nil
}

// GetLastCallTokens returns the input_tokens and model of the most recent
// cost_records row. Used by the sidebar to show last-turn context window
// utilisation (a single-call number, distinct from today's running total).
// Returns (0, "", nil) when the table is empty — an empty agent is not
// an error condition for this caller.
func (s *SQLiteStore) GetLastCallTokens(ctx context.Context) (int64, string, error) {
	var input int64
	var model string
	err := s.db.QueryRowContext(ctx,
		`SELECT input_tokens, model FROM cost_records ORDER BY created_at DESC LIMIT 1`,
	).Scan(&input, &model)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, "", nil
		}
		return 0, "", fmt.Errorf("querying last call tokens: %w", err)
	}
	return input, model, nil
}

// Compile-time interface assertion.
var _ CostStore = (*SQLiteStore)(nil)
