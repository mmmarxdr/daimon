package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
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

// listSkillsResp is the envelope for GET /api/skills.
type listSkillsResp struct {
	Skills []store.UserSkill `json:"skills"`
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

// --------------------------------------------------------------------------
// Task 3.17 — handleListSkills
// --------------------------------------------------------------------------

// handleListSkills serves GET /api/skills.
// Query param ?source=user|curated|all controls filtering.
//
// V1 note: curated skills are loaded from embed.FS at runtime (Phase 6), NOT
// stored in the DB. Until Phase 6 ships, ?source=curated always returns an
// empty slice. The filter parses correctly; the empty result is intentional.
// (CONFIG-REQ-9; AGENT-LOOP-REQ-8)
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	sourceFilter := r.URL.Query().Get("source")

	skills := make([]store.UserSkill, 0)

	if s.deps.UserSkillStore != nil {
		rows, err := s.deps.UserSkillStore.ListUserSkills(r.Context())
		if err != nil {
			slog.Error("handleListSkills: list error", "error", err)
			writeSkillError(w, http.StatusInternalServerError, "internal error")
			return
		}
		for _, sk := range rows {
			switch sourceFilter {
			case "user":
				if sk.Source == "user" {
					skills = append(skills, sk)
				}
			case "curated":
				// V1: curated skills are not stored in DB. Phase 6 will seed them.
				// For now, ?source=curated always returns empty.
				if sk.Source == "curated" {
					skills = append(skills, sk)
				}
			default:
				// "" or "all" — return everything
				skills = append(skills, sk)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(listSkillsResp{Skills: skills}); err != nil {
		slog.Warn("handleListSkills: encode error", "error", err)
	}
}

// --------------------------------------------------------------------------
// Task 3.18 — handleGetSkill
// --------------------------------------------------------------------------

// handleGetSkill serves GET /api/skills/{name}.
// Returns 200 + UserSkill JSON on success, 404 on missing. (AGENT-LOOP-REQ-8)
func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.deps.UserSkillStore == nil {
		writeSkillError(w, http.StatusNotFound, "skill '"+name+"' not found")
		return
	}

	sk, err := s.deps.UserSkillStore.GetUserSkill(r.Context(), name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeSkillError(w, http.StatusNotFound, "skill '"+name+"' not found")
			return
		}
		slog.Error("handleGetSkill: get error", "name", name, "error", err)
		writeSkillError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sk); err != nil {
		slog.Warn("handleGetSkill: encode error", "error", err)
	}
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
//
// curatedFS is the zero-value embed.FS until Phase 6 ships the curated catalog.
// When Phase 6 adds curated_embed.go, this call site switches to skill.CuratedFS.
// (AGENT-LOOP-REQ-8; design §2.8.4)
func (s *Server) reloadSkills(ctx context.Context) {
	if s.deps.Agent == nil {
		return
	}

	contents, _, execs, warns := skill.LoadSkillsUnified(
		ctx,
		s.config().Skills,
		s.deps.UserSkillStore,
		embed.FS{}, // Phase 6: replace with skill.CuratedFS
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
