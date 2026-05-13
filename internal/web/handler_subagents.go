package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"daimon/internal/agent"
	"daimon/internal/notify"
)

// errAlreadyFinished is a sentinel returned by CancelSubagent implementations
// when the target subagent has already reached a terminal state
// (completed / failed / cancelled). The cancel handler maps this to HTTP 200
// with {"already_finished": true} per REQ-17.
var errAlreadyFinished = errors.New("subagent already finished")

// subagentActiveItem is the JSON shape for a single entry in GET /api/subagents/active.
// Field names follow the spec (SUBAGENTS-REQ-15) and the frontend panel contract.
type subagentActiveItem struct {
	SubagentID   string    `json:"subagent_id"`
	BatchID      string    `json:"batch_id"`
	SkillName    string    `json:"skill_name"`
	ParentConvID string    `json:"parent_conv_id"`
	Status       string    `json:"status"`
	CostUSD      float64   `json:"cost_usd"`
	TurnCount    int       `json:"turn_count"`
	StartedAt    time.Time `json:"started_at"`
}

// subagentActiveResponse is the envelope for GET /api/subagents/active.
type subagentActiveResponse struct {
	Active []subagentActiveItem `json:"active"`
}

// handleSubagentsActive returns the list of currently running subagents.
// If no SubagentProvider is configured (no executable skills loaded), it returns
// {"active":[]} rather than 404 — the endpoint is always available.
func (s *Server) handleSubagentsActive(w http.ResponseWriter, r *http.Request) {
	var statuses []agent.SubagentStatus
	if s.deps.SubagentProvider != nil {
		statuses = s.deps.SubagentProvider.ActiveSubagents()
	}

	items := make([]subagentActiveItem, 0, len(statuses))
	for _, st := range statuses {
		items = append(items, subagentActiveItem{
			SubagentID:   st.ID,
			BatchID:      st.BatchID,
			SkillName:    st.SkillName,
			ParentConvID: st.ParentConvID,
			Status:       st.Status,
			CostUSD:      st.Cost,
			TurnCount:    st.Turns,
			StartedAt:    st.SpawnedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(subagentActiveResponse{Active: items}); err != nil {
		slog.Warn("web: /api/subagents/active encode error", "error", err)
	}
}

// subagentWSFrame is a single JSON frame streamed to the WS client.
type subagentWSFrame struct {
	Event   string         `json:"event"`
	Payload map[string]any `json:"payload"`
}

// handleSubagentsWebSocket upgrades the connection and streams subagent
// lifecycle events (agent.subagent.*) from the event bus.
//
// Per-connection channel: cap-8 + drop+warn pattern. Slow consumers lose
// events rather than blocking the bus or other subscribers.
func (s *Server) handleSubagentsWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("web: ws/subagents upgrade error", "error", err)
		return
	}
	defer conn.Close()

	conn.SetReadLimit(4096) // control frames only
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	})

	// Per-connection event channel — bounded cap-8 + drop+warn for slow consumers.
	evCh := make(chan notify.Event, 8)

	// Subscribe to the bus if a provider is wired in.
	if s.deps.SubagentProvider != nil {
		bus := s.deps.SubagentProvider.SubagentBus()
		if bus != nil {
			bus.Subscribe(func(ev notify.Event) {
				if !strings.HasPrefix(ev.Type, "agent.subagent.") {
					return
				}
				select {
				case evCh <- ev:
				default:
					slog.Warn("web: ws/subagents slow consumer, dropping event",
						"event", ev.Type)
				}
			})
		}
	}

	// Pump client reads to detect disconnect.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-done:
			return
		case <-r.Context().Done():
			return
		case <-pingTicker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				return
			}
		case ev, ok := <-evCh:
			if !ok {
				return
			}
			frame := subagentWSFrame{
				Event:   ev.Type,
				Payload: buildSubagentPayload(ev),
			}
			if err := conn.WriteJSON(frame); err != nil {
				return
			}
		}
	}
}

// handleSubagentCancel cancels a running subagent identified by path param {id}.
// Per spec REQ-17:
//   - 204 No Content when the cancel was issued successfully
//   - 200 + {"already_finished": true} when the subagent is already in a terminal state
//   - 404 when no SubagentProvider is wired or the ID is unknown
//   - 400 when the id path parameter is missing
func (s *Server) handleSubagentCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id") // Go 1.22+ ServeMux path param
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}
	if s.deps.SubagentProvider == nil {
		http.Error(w, `{"error":"subagent not found"}`, http.StatusNotFound)
		return
	}

	err := s.deps.SubagentProvider.CancelSubagent(id)
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if errors.Is(err, errAlreadyFinished) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]bool{"already_finished": true})
		return
	}

	// SubagentManager.Cancel returns "subagent %q not found" for unknown IDs.
	// All other failures are also mapped to 404 in V1 (only failure mode is unknown ID).
	http.Error(w, `{"error":"subagent not found"}`, http.StatusNotFound)
}

// buildSubagentPayload converts a notify.Event into the WS payload map.
// All event fields are included; the Meta map is flattened into the payload.
func buildSubagentPayload(ev notify.Event) map[string]any {
	p := make(map[string]any, len(ev.Meta)+4)
	for k, v := range ev.Meta {
		p[k] = v
	}
	p["channel_id"] = ev.ChannelID
	p["timestamp"] = ev.Timestamp.UTC().Format(time.RFC3339)
	if ev.Text != "" {
		p["text"] = ev.Text
	}
	if ev.Error != "" {
		p["error"] = ev.Error
	}
	return p
}
