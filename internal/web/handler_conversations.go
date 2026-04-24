package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"daimon/internal/store"
)

// apiMessage is the wire shape for a single conversation message sent to the frontend.
type apiMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

// apiConversation is the wire shape for a full conversation sent to the frontend.
type apiConversation struct {
	ID        string       `json:"id"`
	ChannelID string       `json:"channel_id"`
	Messages  []apiMessage `json:"messages"`
	CreatedAt string       `json:"created_at"`
	UpdatedAt string       `json:"updated_at"`
}

func toAPIConversation(c *store.Conversation) apiConversation {
	msgs := make([]apiMessage, 0, len(c.Messages))
	for _, m := range c.Messages {
		msgs = append(msgs, apiMessage{
			Role:    m.Role,
			Content: m.Content.TextOnly(),
		})
	}
	return apiConversation{
		ID:        c.ID,
		ChannelID: c.ChannelID,
		Messages:  msgs,
		CreatedAt: c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	channel := r.URL.Query().Get("channel")

	ws, ok := s.deps.Store.(store.WebStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "conversation listing not supported by current store backend")
		return
	}

	convs, total, err := ws.ListConversationsPaginated(r.Context(), channel, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	type convSummary struct {
		ID           string `json:"id"`
		ChannelID    string `json:"channel_id"`
		Title        string `json:"title"`
		MessageCount int    `json:"message_count"`
		LastMessage  string `json:"last_message,omitempty"`
		UpdatedAt    string `json:"updated_at,omitempty"`
	}

	items := make([]convSummary, 0, len(convs))
	for _, c := range convs {
		summary := convSummary{
			ID:           c.ID,
			ChannelID:    c.ChannelID,
			Title:        deriveTitle(&c),
			MessageCount: len(c.Messages),
			UpdatedAt:    c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if len(c.Messages) > 0 {
			last := c.Messages[len(c.Messages)-1]
			summary.LastMessage = truncate(last.Content.TextOnly(), 100)
		}
		items = append(items, summary)
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	id := pathParam(r, "id")

	conv, err := s.deps.Store.LoadConversation(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "conversation not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, toAPIConversation(conv))
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	ws, ok := s.deps.Store.(store.WebStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "not supported")
		return
	}

	id := pathParam(r, "id")
	if err := ws.DeleteConversation(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// truncate shortens s to at most n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}

	return string(runes[:n]) + "…"
}

// deriveTitle resolves the conv's display title with the precedence:
//  1. metadata["title"] when non-empty (LLM-generated or manual rename)
//  2. first user message truncated to 60 runes + "…" when truncated
//  3. empty string when neither is available
func deriveTitle(c *store.Conversation) string {
	if c == nil {
		return ""
	}
	if t := strings.TrimSpace(c.Metadata["title"]); t != "" {
		return t
	}
	for _, m := range c.Messages {
		if m.Role == "user" {
			text := strings.TrimSpace(m.Content.TextOnly())
			if text == "" {
				continue
			}
			runes := []rune(text)
			if len(runes) <= 60 {
				return text
			}
			return string(runes[:60]) + "…"
		}
	}
	return ""
}

// --- Phase 2–4 endpoints: paginated messages, rename, restore ---

// handleGetConversationMessages returns a window of messages from a single
// conversation. Query params: `before` (int, -1 or omitted = most recent),
// `limit` (int, clamped to [1, 200], default 50).
func (s *Server) handleGetConversationMessages(w http.ResponseWriter, r *http.Request) {
	ws, ok := s.deps.Store.(store.WebStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "paginated messages not supported by current store backend")
		return
	}

	id := pathParam(r, "id")

	beforeRaw := r.URL.Query().Get("before")
	before := -1
	if beforeRaw != "" {
		v, err := strconv.Atoi(beforeRaw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid 'before' param")
			return
		}
		before = v
	}

	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid 'limit' param")
			return
		}
		limit = v
	}

	msgs, hasMore, oldest, err := ws.GetConversationMessages(r.Context(), id, before, limit)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	apiMsgs := make([]apiMessage, 0, len(msgs))
	for _, m := range msgs {
		apiMsgs = append(apiMsgs, apiMessage{
			Role:    m.Role,
			Content: m.Content.TextOnly(),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"messages":     apiMsgs,
		"oldest_index": oldest,
		"has_more":     hasMore,
	})
}

// handlePatchConversation accepts {"title": "..."} and persists it to
// metadata.title. Validates: 1..100 runes after trimming, newlines stripped.
func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	ws, ok := s.deps.Store.(store.WebStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "title rename not supported")
		return
	}

	id := pathParam(r, "id")

	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Replace newlines with spaces, then trim + collapse whitespace.
	normalized := strings.TrimSpace(body.Title)
	normalized = strings.ReplaceAll(normalized, "\r\n", " ")
	normalized = strings.ReplaceAll(normalized, "\n", " ")
	normalized = strings.ReplaceAll(normalized, "\r", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")

	if normalized == "" {
		writeError(w, http.StatusBadRequest, "title must be non-empty")
		return
	}
	if utf8.RuneCountInString(normalized) > 100 {
		writeError(w, http.StatusBadRequest, "title must be at most 100 runes")
		return
	}

	if err := ws.UpdateConversationTitle(r.Context(), id, normalized); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		if errors.Is(err, store.ErrInvalidTitle) {
			writeError(w, http.StatusBadRequest, "invalid title")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": id, "title": normalized})
}

// handleRestoreConversation clears deleted_at on a soft-deleted conv.
func (s *Server) handleRestoreConversation(w http.ResponseWriter, r *http.Request) {
	ws, ok := s.deps.Store.(store.WebStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "restore not supported")
		return
	}

	id := pathParam(r, "id")

	if err := ws.RestoreConversation(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": id, "restored": true})
}
