package channel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// F1. HandleWebSocket reads `conversation_id` query param and tags every
// IncomingMessage emitted on that socket.

func dialWSWithPath(t *testing.T, srvURL, path string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srvURL, "http") + path
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %q: %v", url, err)
	}
	return conn
}

func TestHandleWebSocket_ConversationIDBindsSession(t *testing.T) {
	srv, _, inbox := newTestServer(t)
	conn := dialWSWithPath(t, srv.URL, "/ws/chat?conversation_id=conv_web:abc:u1")
	defer conn.Close()

	payload, _ := json.Marshal(wsMsg{Type: "message", Text: "hi", SenderID: "u1"})
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case msg := <-inbox:
		if msg.ConversationID != "conv_web:abc:u1" {
			t.Errorf("ConversationID: got %q, want conv_web:abc:u1", msg.ConversationID)
		}
	case <-time.After(time.Second):
		t.Fatal("no message on inbox")
	}
}

func TestHandleWebSocket_NoConversationIDLeavesEmpty(t *testing.T) {
	srv, _, inbox := newTestServer(t)
	conn := dialWSWithPath(t, srv.URL, "/ws/chat")
	defer conn.Close()

	payload, _ := json.Marshal(wsMsg{Type: "message", Text: "hi", SenderID: "u1"})
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case msg := <-inbox:
		if msg.ConversationID != "" {
			t.Errorf("ConversationID should be empty when param absent, got %q", msg.ConversationID)
		}
	case <-time.After(time.Second):
		t.Fatal("no message on inbox")
	}
}

func TestHandleWebSocket_ConversationIDContinueTurnTagged(t *testing.T) {
	srv, _, inbox := newTestServer(t)
	conn := dialWSWithPath(t, srv.URL, "/ws/chat?conversation_id=conv_web:cont:u9")
	defer conn.Close()

	payload, _ := json.Marshal(wsMsg{Type: "continue_turn", SenderID: "u9"})
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case msg := <-inbox:
		if !msg.IsContinuation {
			t.Errorf("expected IsContinuation=true")
		}
		if msg.ConversationID != "conv_web:cont:u9" {
			t.Errorf("ConversationID on continue_turn: got %q, want conv_web:cont:u9", msg.ConversationID)
		}
	case <-time.After(time.Second):
		t.Fatal("no continue_turn on inbox")
	}
}

func TestHandleWebSocket_OversizeConversationIDDropped(t *testing.T) {
	srv, _, inbox := newTestServer(t)
	long := strings.Repeat("x", 300) // >200 chars, should be ignored
	conn := dialWSWithPath(t, srv.URL, "/ws/chat?conversation_id="+long)
	defer conn.Close()

	payload, _ := json.Marshal(wsMsg{Type: "message", Text: "hi", SenderID: "u1"})
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case msg := <-inbox:
		if msg.ConversationID != "" {
			t.Errorf("oversize conversation_id should be dropped, got %q (%d chars)",
				msg.ConversationID, len(msg.ConversationID))
		}
	case <-time.After(time.Second):
		t.Fatal("no message on inbox")
	}
}
