package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"daimon/internal/store"
)

// memoryEntryResponse is the API shape the frontend expects for a MemoryEntry.
// Fields beyond ID/Content/Tags were added when the Memory page shifted from a
// static list to the Liminal trust surface (confidence / cluster / source conv).
type memoryEntryResponse struct {
	ID                      string   `json:"id"`
	Content                 string   `json:"content"`
	Tags                    []string `json:"tags"`
	Type                    string   `json:"type,omitempty"`
	Cluster                 string   `json:"cluster"`
	Importance              int      `json:"importance"`
	AccessCount             int      `json:"access_count"`
	LastAccessedAt          string   `json:"last_accessed_at,omitempty"`
	SourceConversationID    string   `json:"source_conversation_id"`
	SourceConversationTitle string   `json:"source_conversation_title,omitempty"`
	CreatedAt               string   `json:"created_at"`
}

func toMemoryEntryResponse(e store.MemoryEntry, convTitle string) memoryEntryResponse {
	tags := e.Tags
	if tags == nil {
		tags = []string{}
	}
	cluster := e.Cluster
	if cluster == "" {
		cluster = "general"
	}
	var lastAccessed string
	if e.LastAccessedAt != nil && !e.LastAccessedAt.IsZero() {
		lastAccessed = e.LastAccessedAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return memoryEntryResponse{
		ID:                      e.ID,
		Content:                 e.Content,
		Tags:                    tags,
		Type:                    e.Type,
		Cluster:                 cluster,
		Importance:              e.Importance,
		AccessCount:             e.AccessCount,
		LastAccessedAt:          lastAccessed,
		SourceConversationID:    e.Source,
		SourceConversationTitle: convTitle,
		CreatedAt:               e.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// resolveConvTitle derives a human-readable label for a conversation by
// trimming its first user message to 60 runes. Uses a per-request cache so a
// list of N memories referencing K conversations hits the store K times at most.
// Returns "" on any failure (missing conv, no user messages, etc.).
func resolveConvTitle(ctx context.Context, st store.Store, convID string, cache map[string]string) string {
	if convID == "" {
		return ""
	}
	if title, ok := cache[convID]; ok {
		return title
	}
	conv, err := st.LoadConversation(ctx, convID)
	if err != nil {
		cache[convID] = ""
		return ""
	}
	for _, m := range conv.Messages {
		if m.Role != "user" {
			continue
		}
		text := strings.TrimSpace(m.Content.TextOnly())
		if text == "" {
			continue
		}
		title := truncate(text, 60)
		cache[convID] = title
		return title
	}
	cache[convID] = ""
	return ""
}

func (s *Server) handleListMemory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	scopeID := r.URL.Query().Get("scope")

	entries, err := s.deps.Store.SearchMemory(r.Context(), scopeID, q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	titleCache := make(map[string]string, 8)
	items := make([]memoryEntryResponse, 0, len(entries))
	for _, e := range entries {
		convTitle := resolveConvTitle(r.Context(), s.deps.Store, e.Source, titleCache)
		items = append(items, toMemoryEntryResponse(e, convTitle))
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePostMemory(w http.ResponseWriter, r *http.Request) {
	var entry store.MemoryEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if entry.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}
	if entry.ScopeID == "" {
		writeError(w, http.StatusBadRequest, "scope_id is required")
		return
	}

	// Assign a server-generated ID — never trust caller-supplied IDs.
	entry.ID = uuid.New().String()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}

	if err := s.deps.Store.AppendMemory(r.Context(), entry.ScopeID, entry); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Direct POST path does not resolve a conv title — caller knows its own source.
	writeJSON(w, http.StatusCreated, toMemoryEntryResponse(entry, ""))
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	ws, ok := s.deps.Store.(store.WebStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "not supported")
		return
	}

	rawID := pathParam(r, "id")
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid memory id: must be a number")
		return
	}
	scopeID := r.URL.Query().Get("scope")

	if err := ws.DeleteMemory(r.Context(), scopeID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
