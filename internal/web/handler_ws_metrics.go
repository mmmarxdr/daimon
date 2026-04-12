package web

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleMetricsWebSocket upgrades the connection to WebSocket and pushes a
// MetricsSnapshot every 5 seconds until the client disconnects.
func (s *Server) handleMetricsWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("web: ws/metrics upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Send an initial snapshot immediately.
	snap := s.buildMetricsSnapshot(r.Context())
	if err := conn.WriteJSON(snap); err != nil {
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Pump control messages to detect client close.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-r.Context().Done():
			return
		case <-ticker.C:
			snap := s.buildMetricsSnapshot(r.Context())
			if err := conn.WriteJSON(snap); err != nil {
				return
			}
		}
	}
}
