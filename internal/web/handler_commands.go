package web

import (
	"encoding/json"
	"net/http"

	"daimon/internal/agent"
)

// handleListCommands serves GET /api/commands.
// Returns the full list of registered commands with name, description, source, and destructive flag.
// 503 when no CommandProvider is wired.
func (s *Server) handleListCommands(w http.ResponseWriter, r *http.Request) {
	if s.deps.CommandProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "agent not available")
		return
	}
	cmds := s.deps.CommandProvider.Commands()
	if cmds == nil {
		cmds = []agent.CommandInfo{}
	}
	writeJSON(w, http.StatusOK, map[string][]agent.CommandInfo{"commands": cmds})
}

// handleRunCommand serves POST /api/commands/run.
// Dispatches a registered command with the provided arguments.
// 400 for invalid JSON or missing name.
// 403 when the command is destructive and allow_destructive is not set.
// 404 when the command is not registered.
// 503 when no CommandProvider is wired.
func (s *Server) handleRunCommand(w http.ResponseWriter, r *http.Request) {
	if s.deps.CommandProvider == nil {
		writeError(w, http.StatusServiceUnavailable, "agent not available")
		return
	}

	var req agent.RunCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "missing name")
		return
	}

	// Look up the command in the registered list to check existence and destructive flag.
	var found *agent.CommandInfo
	for _, c := range s.deps.CommandProvider.Commands() {
		c := c // capture loop variable
		if c.Name == req.Name {
			found = &c
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "command not found")
		return
	}

	// Authorization gate: destructive commands require allow_destructive=true.
	if found.Destructive && !req.AllowDestructive {
		writeError(w, http.StatusForbidden, "command requires allow_destructive=true")
		return
	}

	result, err := s.deps.CommandProvider.RunCommand(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "command failed")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
