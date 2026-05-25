package web

import (
	"encoding/json"
	"net/http"

	"daimon/internal/tool"
)

// toolInfo is the per-tool entry returned by GET /api/tools. The four metadata
// fields (risk, category, permission, source) come from the tool's ToolMeta
// via BuildToolMeta. Permission is DESCRIPTIVE ONLY — it is never enforced.
type toolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
	Risk        string          `json:"risk"`
	Category    string          `json:"category"`
	Permission  string          `json:"permission"`
	Source      string          `json:"source"`
}

// toolSummary holds aggregated counts derived from the per-tool ToolMeta.
// It is computed at request time from the live registry — no precomputed state.
type toolSummary struct {
	Total      int            `json:"total"`
	ByCategory map[string]int `json:"by_category"`
	ByRisk     map[string]int `json:"by_risk"`
}

// toolsResponse is the top-level shape returned by GET /api/tools (spec REQ-6).
// It wraps the tool list and aggregated summary in a single JSON object instead
// of the former bare array.
type toolsResponse struct {
	Tools   []toolInfo  `json:"tools"`
	Summary toolSummary `json:"summary"`
}

func (s *Server) handleListTools(w http.ResponseWriter, _ *http.Request) {
	// Derive metadata for every tool in the live registry in a single pass.
	// BuildToolMeta is pure and cheap over ~20 tools.
	meta := tool.BuildToolMeta(s.deps.Tools)

	tools := make([]toolInfo, 0, len(s.deps.Tools))
	summary := toolSummary{
		ByCategory: make(map[string]int),
		ByRisk:     make(map[string]int),
	}

	for name, t := range s.deps.Tools {
		// Join by the registry key — the same key BuildToolMeta indexes by — so
		// the metadata stays correct even if a tool is ever registered under a
		// key that differs from its Name().
		m := meta[name]
		tools = append(tools, toolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Schema(),
			Risk:        string(m.Risk),
			Category:    string(m.Category),
			Permission:  string(m.Permission),
			Source:      string(m.Source),
		})
		summary.Total++
		summary.ByRisk[string(m.Risk)]++
		summary.ByCategory[string(m.Category)]++
	}

	writeJSON(w, http.StatusOK, toolsResponse{
		Tools:   tools,
		Summary: summary,
	})
}
