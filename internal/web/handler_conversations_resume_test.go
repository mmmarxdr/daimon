package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/store"
)

// seedSummaryConv adds a conv to the fake store with the given title + messages.
func seedSummaryConv(f *fakeWebStore, id string, title string, msgs []provider.ChatMessage) {
	conv := store.Conversation{
		ID:        id,
		ChannelID: "web:t",
		Messages:  msgs,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if title != "" {
		conv.Metadata = map[string]string{"title": title}
	}
	f.conversations = append(f.conversations, conv)
}

// --- F5. Title in list summary ---

func TestHandleListConversations_UsesMetadataTitleWhenPresent(t *testing.T) {
	fs := &fakeWebStore{}
	mk := func(role, text string) provider.ChatMessage {
		return provider.ChatMessage{Role: role, Content: content.Blocks{{Type: content.BlockText, Text: text}}}
	}
	seedSummaryConv(fs, "conv_a", "Sobre RAG", []provider.ChatMessage{
		mk("user", "quiero entender el RAG con muchas palabras aquí"),
		mk("assistant", "ok"),
	})

	srv := newConvHandlerServer(t, fs)
	req := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Title != "Sobre RAG" {
		t.Errorf("title: got %q, want %q", resp.Items[0].Title, "Sobre RAG")
	}
}

func TestHandleListConversations_FallsBackToFirstUserMsg(t *testing.T) {
	fs := &fakeWebStore{}
	mk := func(role, text string) provider.ChatMessage {
		return provider.ChatMessage{Role: role, Content: content.Blocks{{Type: content.BlockText, Text: text}}}
	}
	seedSummaryConv(fs, "conv_b", "", []provider.ChatMessage{
		mk("user", "una pregunta muy larga que debería ser truncada por el derivador"),
		mk("assistant", "respuesta"),
	})

	srv := newConvHandlerServer(t, fs)
	req := httptest.NewRequest(http.MethodGet, "/api/conversations", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	var resp struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(resp.Items))
	}
	if !strings.HasPrefix(resp.Items[0].Title, "una pregunta muy larga") {
		t.Errorf("title should start with first user msg, got %q", resp.Items[0].Title)
	}
}

// --- F2. GET /api/conversations/{id}/messages ---

func TestHandleGetConversationMessages_InitialLoad(t *testing.T) {
	fs := &fakeWebStore{}
	mk := func(i int) provider.ChatMessage {
		return provider.ChatMessage{Role: "user", Content: content.Blocks{{Type: content.BlockText, Text: "msg-" + itoa(i)}}}
	}
	msgs := make([]provider.ChatMessage, 200)
	for i := range msgs {
		msgs[i] = mk(i)
	}
	seedSummaryConv(fs, "conv_big", "", msgs)

	srv := newConvHandlerServer(t, fs)
	req := httptest.NewRequest(http.MethodGet, "/api/conversations/conv_big/messages?limit=50", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Messages    []apiMessage `json:"messages"`
		OldestIndex int          `json:"oldest_index"`
		HasMore     bool         `json:"has_more"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Messages) != 50 {
		t.Errorf("len(messages): got %d, want 50", len(resp.Messages))
	}
	if resp.OldestIndex != 150 {
		t.Errorf("oldest_index: got %d, want 150", resp.OldestIndex)
	}
	if !resp.HasMore {
		t.Errorf("has_more: got false, want true")
	}
}

func TestHandleGetConversationMessages_PagingUpward(t *testing.T) {
	fs := &fakeWebStore{}
	mk := func(i int) provider.ChatMessage {
		return provider.ChatMessage{Role: "user", Content: content.Blocks{{Type: content.BlockText, Text: "msg-" + itoa(i)}}}
	}
	msgs := make([]provider.ChatMessage, 200)
	for i := range msgs {
		msgs[i] = mk(i)
	}
	seedSummaryConv(fs, "conv_pg", "", msgs)

	srv := newConvHandlerServer(t, fs)
	req := httptest.NewRequest(http.MethodGet, "/api/conversations/conv_pg/messages?before=150&limit=50", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	var resp struct {
		OldestIndex int  `json:"oldest_index"`
		HasMore     bool `json:"has_more"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.OldestIndex != 100 {
		t.Errorf("oldest_index: got %d, want 100", resp.OldestIndex)
	}
	if !resp.HasMore {
		t.Errorf("has_more: want true")
	}
}

func TestHandleGetConversationMessages_NotFound(t *testing.T) {
	fs := &fakeWebStore{}
	srv := newConvHandlerServer(t, fs)

	req := httptest.NewRequest(http.MethodGet, "/api/conversations/conv_ghost/messages", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestHandleGetConversationMessages_InvalidBefore(t *testing.T) {
	srv := newConvHandlerServer(t, &fakeWebStore{})

	req := httptest.NewRequest(http.MethodGet, "/api/conversations/x/messages?before=abc", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 on bad before, got %d", rec.Code)
	}
}

// --- F3. PATCH /api/conversations/{id} ---

func TestHandlePatchConversation_ValidRename(t *testing.T) {
	fs := &fakeWebStore{}
	seedSummaryConv(fs, "conv_rn", "", nil)

	srv := newConvHandlerServer(t, fs)
	body := []byte(`{"title":"Mi nuevo hilo"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/conversations/conv_rn", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Title string `json:"title"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Title != "Mi nuevo hilo" {
		t.Errorf("response title: got %q, want %q", resp.Title, "Mi nuevo hilo")
	}
	// Verify store got the update.
	if fs.conversations[0].Metadata["title"] != "Mi nuevo hilo" {
		t.Errorf("store not updated; metadata=%v", fs.conversations[0].Metadata)
	}
}

func TestHandlePatchConversation_EmptyRejected(t *testing.T) {
	fs := &fakeWebStore{}
	seedSummaryConv(fs, "conv_e", "", nil)

	srv := newConvHandlerServer(t, fs)
	body := []byte(`{"title":"   "}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/conversations/conv_e", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePatchConversation_OversizeRejected(t *testing.T) {
	fs := &fakeWebStore{}
	seedSummaryConv(fs, "conv_big", "", nil)

	srv := newConvHandlerServer(t, fs)
	body := []byte(`{"title":"` + strings.Repeat("x", 101) + `"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/conversations/conv_big", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandlePatchConversation_NewlineStripped(t *testing.T) {
	fs := &fakeWebStore{}
	seedSummaryConv(fs, "conv_nl", "", nil)

	srv := newConvHandlerServer(t, fs)
	body := []byte(`{"title":"line1\nline2"}`)
	req := httptest.NewRequest(http.MethodPatch, "/api/conversations/conv_nl", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if fs.conversations[0].Metadata["title"] != "line1 line2" {
		t.Errorf("newline not stripped: got %q", fs.conversations[0].Metadata["title"])
	}
}

// --- F4. POST /api/conversations/{id}/restore ---
// The fake store's RestoreConversation returns ErrNotFound always, which
// matches both "missing" and "already live" semantics. For integration
// coverage with real restore semantics, see sqlitestore_softdelete_test.go.

func TestHandleRestoreConversation_404(t *testing.T) {
	srv := newConvHandlerServer(t, &fakeWebStore{})

	req := httptest.NewRequest(http.MethodPost, "/api/conversations/conv_x/restore", nil)
	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

// --- Helpers ---

// convHandlerHarness is a thin wrapper around http.Handler for the
// conversation handlers under test. Avoids httptest.NewServer because that
// obscures the handler behind a network socket.
type convHandlerHarness struct{ Handler http.Handler }

func newConvHandlerServer(t *testing.T, fs *fakeWebStore) convHandlerHarness {
	t.Helper()
	s := &Server{
		mux:  http.NewServeMux(),
		deps: ServerDeps{Store: fs},
	}
	s.mux.HandleFunc("GET /api/conversations", s.handleListConversations)
	s.mux.HandleFunc("GET /api/conversations/{id}", s.handleGetConversation)
	s.mux.HandleFunc("GET /api/conversations/{id}/messages", s.handleGetConversationMessages)
	s.mux.HandleFunc("PATCH /api/conversations/{id}", s.handlePatchConversation)
	s.mux.HandleFunc("POST /api/conversations/{id}/restore", s.handleRestoreConversation)
	return convHandlerHarness{Handler: s.mux}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
