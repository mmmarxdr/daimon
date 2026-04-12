package web

import (
	"encoding/json"
	"net/http"
)

type toolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

func (s *Server) handleListTools(w http.ResponseWriter, _ *http.Request) {
	tools := make([]toolInfo, 0, len(s.deps.Tools))
	for _, t := range s.deps.Tools {
		tools = append(tools, toolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Schema(),
		})
	}
	writeJSON(w, http.StatusOK, tools)
}
