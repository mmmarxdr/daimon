package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"daimon/internal/agent"
	"daimon/internal/notify"
)

// ---------------------------------------------------------------------------
// Task 1.16 — Integration test: cancel → EventSubagentFailed on bus (REQ-17)
//
// This test uses a controllable SubagentProvider that emits a real bus event
// when CancelSubagent is called, simulating the full cancel → event flow
// without requiring a live LLM provider.
// ---------------------------------------------------------------------------

// busEmittingCancelProvider is a SubagentProvider that emits an
// EventSubagentFailed event on the bus when CancelSubagent is called.
// Simulates what the real *agent.Agent does: cancel the ctx → budgetMonitor
// fires EventSubagentFailed{reason:"cancelled"}.
type busEmittingCancelProvider struct {
	bus    notify.Bus
	subIDs map[string]bool // registered subagent IDs
}

func (b *busEmittingCancelProvider) ActiveSubagents() []agent.SubagentStatus { return nil }
func (b *busEmittingCancelProvider) SubagentBus() notify.Bus                  { return b.bus }

func (b *busEmittingCancelProvider) CancelSubagent(id string) error {
	if !b.subIDs[id] {
		return nil // unknown IDs: silently succeed for integration test simplicity
	}
	// Emit the lifecycle event that the real SubagentManager would emit.
	b.bus.Emit(notify.Event{
		Type:      notify.EventSubagentFailed,
		Origin:    notify.OriginAgent,
		ChannelID: "sub:" + id,
		Timestamp: time.Now(),
		Meta: map[string]string{
			"subagent_id": id,
			"reason":      "cancelled",
		},
	})
	return nil
}

// TestCancelSubagent_Integration verifies the full HTTP → bus event flow:
// POST /api/subagents/{id}/cancel → 204 → EventSubagentFailed on bus. (REQ-17)
func TestCancelSubagent_Integration(t *testing.T) {
	bus := notify.NewEventBus(256, 0, 0)
	t.Cleanup(func() { bus.Close() })

	subID := "sub-integration-123"
	prov := &busEmittingCancelProvider{
		bus:    bus,
		subIDs: map[string]bool{subID: true},
	}

	// Subscribe to the bus BEFORE cancelling.
	eventCh := make(chan notify.Event, 1)
	bus.Subscribe(func(ev notify.Event) {
		if ev.Type == notify.EventSubagentFailed {
			select {
			case eventCh <- ev:
			default:
			}
		}
	})

	s := &Server{
		deps: ServerDeps{
			Store:            &fakeWebStore{},
			Config:           minimalConfig(),
			SubagentProvider: prov,
		},
		mux:        http.NewServeMux(),
		wsUpgrader: newWSUpgrader(nil),
	}
	s.routes()
	ts := httptest.NewServer(s.mux)
	t.Cleanup(ts.Close)

	// Issue the cancel via HTTP.
	req := httptest.NewRequest(http.MethodPost, "/api/subagents/"+subID+"/cancel", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	// Assert EventSubagentFailed is emitted within 1 second.
	select {
	case ev := <-eventCh:
		if ev.Meta["reason"] != "cancelled" {
			t.Errorf("event reason = %q, want 'cancelled'", ev.Meta["reason"])
		}
		if ev.Meta["subagent_id"] != subID {
			t.Errorf("event subagent_id = %q, want %q", ev.Meta["subagent_id"], subID)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("EventSubagentFailed not received within 1 second after cancel")
	}
}
