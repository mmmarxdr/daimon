package store

import (
	"context"
	"fmt"
	"strings"
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

// Compile-time interface assertion.
var _ CostStore = (*SQLiteStore)(nil)
