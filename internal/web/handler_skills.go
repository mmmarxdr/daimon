package web

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"daimon/internal/agent"
	"daimon/internal/skill"
	"daimon/internal/store"
)

// skillNameRE is the allowed pattern for user-defined skill names.
// Matches ^[a-z][a-z0-9_-]*$ and length must be ≤ 64 characters.
var skillNameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// userSkillReq is the POST/PUT JSON body shape.
// ToolsAllowlist uses a pointer-to-slice so JSON null (omit) →
// nil (inherit all parent tools) is distinct from [] (no tools).
type userSkillReq struct {
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Prose          string            `json:"prose"`
	Executable     bool              `json:"executable"`
	Model          string            `json:"model,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	ToolsAllowlist *[]string         `json:"tools_allowlist,omitempty"`
	Budget         *store.BudgetJSON `json:"budget,omitempty"`
	Version        int               `json:"version,omitempty"`
}

// skillWithCommandStatus wraps a UserSkill with the command_status field
// (spec REQ-13). Values: "active" (skill owns the registered command),
// "shadowed_by_builtin" (builtin or cron holds the slot), "" (non-executable
// or not registered at snapshot time — JSON-omitted via omitempty).
type skillWithCommandStatus struct {
	store.UserSkill
	CommandStatus string `json:"command_status,omitempty"`
}

// listSkillsResp is the envelope for GET /api/skills.
type listSkillsResp struct {
	Skills []skillWithCommandStatus `json:"skills"`
}

// commandStatusForSkill derives the command_status value for an executable skill
// given a snapshot of registered commands from a CommandProvider. The normalized
// name (hyphen → underscore) is used for lookup, matching the auto-mount
// normalization rule (design D3). Values are normative per spec REQ-13:
// "active" when the skill owns the registered command, "shadowed_by_builtin"
// when a builtin or cron entry holds the slot, "" otherwise (non-executable or
// not registered at snapshot time — JSON-omitted via omitempty).
func commandStatusForSkill(sk store.UserSkill, cmds []agent.CommandInfo) string {
	if !sk.Executable {
		return ""
	}
	normalized := strings.ReplaceAll(sk.Name, "-", "_")
	for _, c := range cmds {
		if c.Name == normalized {
			if c.Source == agent.SourceSkill {
				return "active"
			}
			return "shadowed_by_builtin"
		}
	}
	return ""
}

// enrichSkills wraps a slice of UserSkill entries with command_status values,
// using a CommandProvider snapshot. When cp is nil, all statuses are empty.
func enrichSkills(skills []store.UserSkill, cp CommandProvider) []skillWithCommandStatus {
	out := make([]skillWithCommandStatus, 0, len(skills))
	var cmds []agent.CommandInfo
	if cp != nil {
		cmds = cp.Commands()
	}
	for _, sk := range skills {
		out = append(out, skillWithCommandStatus{
			UserSkill:     sk,
			CommandStatus: commandStatusForSkill(sk, cmds),
		})
	}
	return out
}

// skillErrorResp is the single-error envelope (404, 403, 409, 500).
type skillErrorResp struct {
	Error string `json:"error"`
}

// skillValidationResp is the multi-error envelope for 422 responses.
type skillValidationResp struct {
	Errors []string `json:"errors"`
}

// writeSkillError writes a JSON error response with a single "error" key.
func writeSkillError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(skillErrorResp{Error: msg})
}

// writeSkillValidation writes a 422 JSON response with an "errors" array.
func writeSkillValidation(w http.ResponseWriter, errs []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(skillValidationResp{Errors: errs})
}

// validateUserSkill checks the request payload against business rules.
// Returns a non-nil slice of error messages when validation fails.
// At REST write time, unknown tools_allowlist entries are a HARD 422 error
// (CONFIG-REQ-5 / task 3.10 / integration test 3.14). During hot-reload
// (reloadSkills → ReplaceExecutableSkills), unknown entries are warn-and-dropped
// by the agent layer — that distinction is intentional (see design §2.8.3 note).
func validateUserSkill(req userSkillReq, knownTools map[string]interface{}) []string {
	var errs []string

	if !skillNameRE.MatchString(req.Name) || len(req.Name) > 64 {
		errs = append(errs, "name must match ^[a-z][a-z0-9_-]*$ and be ≤ 64 chars")
	}
	if len(req.Prose) > 8*1024 {
		errs = append(errs, "prose must be ≤ 8 KB")
	}
	if len(req.Description) > 8*1024 {
		errs = append(errs, "description must be ≤ 8 KB")
	}
	if req.Budget != nil {
		if req.Budget.MaxCostUSD <= 0 && req.Budget.MaxTurns <= 0 && req.Budget.TimeoutMin <= 0 {
			errs = append(errs, "budget: at least one of max_cost_usd, max_turns, timeout_min must be > 0")
		}
	}
	if req.ToolsAllowlist != nil {
		for _, name := range *req.ToolsAllowlist {
			if _, ok := knownTools[name]; !ok {
				errs = append(errs, "tools_allowlist: unknown tool "+name)
			}
		}
	}
	return errs
}

// knownToolNames converts map[string]tool.Tool to map[string]interface{} for
// the validation helper (avoids importing tool package in the validation fn).
func (s *Server) knownToolNames() map[string]interface{} {
	out := make(map[string]interface{}, len(s.deps.Tools))
	for k := range s.deps.Tools {
		out[k] = struct{}{}
	}
	return out
}

// curatedSkillToUserSkill converts a skill.SkillContent from the bundled
// curated catalog into the store.UserSkill response shape with source="curated".
// Timestamps are zero — curated skills are immutable and have no DB row.
func curatedSkillToUserSkill(sc skill.SkillContent) store.UserSkill {
	return store.UserSkill{
		Name:        sc.Name,
		Description: sc.Description,
		Prose:       sc.Prose,
		Executable:  sc.Executable,
		Model:       sc.Model,
		Provider:    sc.ProviderName,
		Version:     sc.Version,
		Source:      "curated",
	}
}

// --------------------------------------------------------------------------
// Task 3.17 — handleListSkills
// --------------------------------------------------------------------------

// handleListSkills serves GET /api/skills.
// Query param ?source=user|curated|all controls filtering.
//
// source=user   — DB rows only (source="user")
// source=curated — bundled curated catalog only (from deps.CuratedSkills)
// source=all or "" — merged: curated + DB; DB wins on name collision
//
// (CONFIG-REQ-9; AGENT-LOOP-REQ-8; spec-gap fix tasks 3.8 + 6.13)
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	sourceFilter := r.URL.Query().Get("source")

	switch sourceFilter {
	case "curated":
		// Return the bundled curated catalog directly — no DB needed.
		raw := make([]store.UserSkill, 0, len(s.deps.CuratedSkills))
		for _, sc := range s.deps.CuratedSkills {
			raw = append(raw, curatedSkillToUserSkill(sc))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(listSkillsResp{Skills: enrichSkills(raw, s.deps.CommandProvider)}); err != nil {
			slog.Warn("handleListSkills: encode error", "error", err)
		}
		return

	case "user":
		// DB rows only — current V1 behavior.
		var raw []store.UserSkill
		if s.deps.UserSkillStore != nil {
			rows, err := s.deps.UserSkillStore.ListUserSkills(r.Context())
			if err != nil {
				slog.Error("handleListSkills: list error", "error", err)
				writeSkillError(w, http.StatusInternalServerError, "internal error")
				return
			}
			for _, sk := range rows {
				if sk.Source == "user" {
					raw = append(raw, sk)
				}
			}
		}
		if raw == nil {
			raw = []store.UserSkill{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(listSkillsResp{Skills: enrichSkills(raw, s.deps.CommandProvider)}); err != nil {
			slog.Warn("handleListSkills: encode error", "error", err)
		}
		return
	}

	// "" or "all" — merged result: start with curated, then DB wins on collision.
	// Build index from curated catalog (lowest precedence).
	type entry struct {
		sk store.UserSkill
	}
	index := make(map[string]*entry, len(s.deps.CuratedSkills))
	order := make([]string, 0, len(s.deps.CuratedSkills))
	for _, sc := range s.deps.CuratedSkills {
		us := curatedSkillToUserSkill(sc)
		index[sc.Name] = &entry{sk: us}
		order = append(order, sc.Name)
	}

	// DB rows override curated on name collision.
	if s.deps.UserSkillStore != nil {
		rows, err := s.deps.UserSkillStore.ListUserSkills(r.Context())
		if err != nil {
			slog.Error("handleListSkills: list error", "error", err)
			writeSkillError(w, http.StatusInternalServerError, "internal error")
			return
		}
		for _, sk := range rows {
			if _, exists := index[sk.Name]; !exists {
				order = append(order, sk.Name)
			}
			index[sk.Name] = &entry{sk: sk}
		}
	}

	merged := make([]store.UserSkill, 0, len(order))
	for _, name := range order {
		if e, ok := index[name]; ok {
			merged = append(merged, e.sk)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(listSkillsResp{Skills: enrichSkills(merged, s.deps.CommandProvider)}); err != nil {
		slog.Warn("handleListSkills: encode error", "error", err)
	}
}

// --------------------------------------------------------------------------
// Task 3.18 — handleGetSkill
// --------------------------------------------------------------------------

// handleGetSkill serves GET /api/skills/{name}.
// Lookup precedence: DB first (source="user") → curated catalog fallback.
// Returns 200 + UserSkill JSON on success, 404 if not found in either source.
// (AGENT-LOOP-REQ-8; CONFIG-REQ-9; spec-gap fix task 6.13)
func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	// Pass 1: try DB.
	if s.deps.UserSkillStore != nil {
		sk, err := s.deps.UserSkillStore.GetUserSkill(r.Context(), name)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(sk); err != nil {
				slog.Warn("handleGetSkill: encode error", "error", err)
			}
			return
		}
		if !errors.Is(err, store.ErrNotFound) {
			slog.Error("handleGetSkill: get error", "name", name, "error", err)
			writeSkillError(w, http.StatusInternalServerError, "internal error")
			return
		}
		// ErrNotFound — fall through to curated lookup below.
	}

	// Pass 2: curated catalog fallback.
	for _, sc := range s.deps.CuratedSkills {
		if sc.Name == name {
			us := curatedSkillToUserSkill(sc)
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(us); err != nil {
				slog.Warn("handleGetSkill: encode error", "error", err)
			}
			return
		}
	}

	writeSkillError(w, http.StatusNotFound, "skill '"+name+"' not found")
}

// --------------------------------------------------------------------------
// Task 3.19 — handleCreateSkill
// --------------------------------------------------------------------------

// handleCreateSkill serves POST /api/skills.
// Validates the payload, inserts into the store, then hot-reloads the agent.
// (CONFIG-REQ-6; OUTPUT-STORE-REQ-12)
//
// Status codes:
//   - 201 Created + Location header on success
//   - 400 Bad Request on malformed JSON
//   - 409 Conflict on name collision (ErrNameConflict)
//   - 422 Unprocessable Entity on validation failure (name, prose, allowlist, budget)
//   - 500 Internal Server Error on store failure
func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	if s.deps.UserSkillStore == nil {
		writeSkillError(w, http.StatusInternalServerError, "skill store not configured")
		return
	}

	var req userSkillReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSkillError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if errs := validateUserSkill(req, s.knownToolNames()); len(errs) > 0 {
		writeSkillValidation(w, errs)
		return
	}

	var allowlist []string
	if req.ToolsAllowlist != nil {
		allowlist = *req.ToolsAllowlist
	}

	sk := store.UserSkill{
		Name:           req.Name,
		Description:    req.Description,
		Prose:          req.Prose,
		Executable:     req.Executable,
		Model:          req.Model,
		Provider:       req.Provider,
		ToolsAllowlist: allowlist,
		Budget:         req.Budget,
		Source:         "user",
		Version:        1,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	created, err := s.deps.UserSkillStore.CreateUserSkill(r.Context(), sk)
	if err != nil {
		if errors.Is(err, store.ErrNameConflict) {
			writeSkillError(w, http.StatusConflict, "name '"+req.Name+"' already exists")
			return
		}
		slog.Error("handleCreateSkill: create error", "name", req.Name, "error", err)
		writeSkillError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.reloadSkills(r.Context())

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/skills/"+created.Name)
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(created); err != nil {
		slog.Warn("handleCreateSkill: encode error", "error", err)
	}
}

// --------------------------------------------------------------------------
// Task 3.20 — handleUpdateSkill
// --------------------------------------------------------------------------

// handleUpdateSkill serves PUT /api/skills/{name}.
// Rejects writes to curated rows (403). Returns 200 + updated body on success.
// (CONFIG-REQ-9)
func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.deps.UserSkillStore == nil {
		writeSkillError(w, http.StatusNotFound, "skill '"+name+"' not found")
		return
	}

	// Fetch existing row to enforce curated guard and confirm it exists.
	existing, err := s.deps.UserSkillStore.GetUserSkill(r.Context(), name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeSkillError(w, http.StatusNotFound, "skill '"+name+"' not found")
			return
		}
		slog.Error("handleUpdateSkill: get error", "name", name, "error", err)
		writeSkillError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing.Source == "curated" {
		writeSkillError(w, http.StatusForbidden, "curated skills are read-only")
		return
	}

	var req userSkillReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeSkillError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// Use the path param name (not the body name) as the canonical key.
	req.Name = name

	if errs := validateUserSkill(req, s.knownToolNames()); len(errs) > 0 {
		writeSkillValidation(w, errs)
		return
	}

	var allowlist []string
	if req.ToolsAllowlist != nil {
		allowlist = *req.ToolsAllowlist
	} else {
		allowlist = existing.ToolsAllowlist
	}

	updated := store.UserSkill{
		ID:             existing.ID,
		Name:           name,
		Description:    req.Description,
		Prose:          req.Prose,
		Executable:     req.Executable,
		Model:          req.Model,
		Provider:       req.Provider,
		ToolsAllowlist: allowlist,
		Budget:         req.Budget,
		Source:         existing.Source,
		Version:        existing.Version,
		CreatedAt:      existing.CreatedAt,
	}

	result, err := s.deps.UserSkillStore.UpdateUserSkill(r.Context(), updated)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeSkillError(w, http.StatusNotFound, "skill '"+name+"' not found")
			return
		}
		if errors.Is(err, store.ErrNameConflict) {
			writeSkillError(w, http.StatusConflict, "name '"+name+"' already exists")
			return
		}
		slog.Error("handleUpdateSkill: update error", "name", name, "error", err)
		writeSkillError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.reloadSkills(r.Context())

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Warn("handleUpdateSkill: encode error", "error", err)
	}
}

// --------------------------------------------------------------------------
// Task 3.21 — handleDeleteSkill
// --------------------------------------------------------------------------

// handleDeleteSkill serves DELETE /api/skills/{name}.
// Rejects deletes on curated rows (403). Returns 204 on success, 404 if missing.
// (CONFIG-REQ-9)
func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.deps.UserSkillStore == nil {
		writeSkillError(w, http.StatusNotFound, "skill '"+name+"' not found")
		return
	}

	// Fetch the existing row to enforce the curated guard.
	existing, err := s.deps.UserSkillStore.GetUserSkill(r.Context(), name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeSkillError(w, http.StatusNotFound, "skill '"+name+"' not found")
			return
		}
		slog.Error("handleDeleteSkill: get error", "name", name, "error", err)
		writeSkillError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing.Source == "curated" {
		writeSkillError(w, http.StatusForbidden, "curated skills are read-only")
		return
	}

	if err := s.deps.UserSkillStore.DeleteUserSkill(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeSkillError(w, http.StatusNotFound, "skill '"+name+"' not found")
			return
		}
		slog.Error("handleDeleteSkill: delete error", "name", name, "error", err)
		writeSkillError(w, http.StatusInternalServerError, "internal error")
		return
	}

	s.reloadSkills(r.Context())

	w.WriteHeader(http.StatusNoContent)
}

// --------------------------------------------------------------------------
// Task 3.22 — reloadSkills helper
// --------------------------------------------------------------------------

// reloadSkills re-runs LoadSkillsUnified and pushes the merged skill set into
// the running agent. Called after every successful CRUD write to /api/skills.
// Idempotent; safe to call when Agent is nil (returns silently).
// (AGENT-LOOP-REQ-8; design §2.8.4)
func (s *Server) reloadSkills(ctx context.Context) {
	if s.deps.Agent == nil {
		return
	}

	contents, _, execs, warns := skill.LoadSkillsUnified(
		ctx,
		s.config().Skills,
		s.deps.UserSkillStore,
		skill.CuratedFS,
		s.config().Tools.Shell,
		s.config().Limits,
	)
	for _, w := range warns {
		slog.Warn("reloadSkills warning", "error", w)
	}

	s.deps.Agent.ReplaceExecutableSkills(execs)
	autoload, idx := agent.InitSkillInjection(contents, s.config().Agent.MaxContextTokens)
	s.deps.Agent.ReplaceSkills(autoload, idx)
}
