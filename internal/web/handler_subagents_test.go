package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"daimon/internal/agent"
	"daimon/internal/notify"
)

// ---------------------------------------------------------------------------
// Helpers — fakeSubagentProvider
// ---------------------------------------------------------------------------

// fakeSubagentProvider is a test double for SubagentProvider.
type fakeSubagentProvider struct {
	mu      sync.Mutex
	active  []agent.SubagentStatus
	bus     notify.Bus
}

func (f *fakeSubagentProvider) ActiveSubagents() []agent.SubagentStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]agent.SubagentStatus, len(f.active))
	copy(out, f.active)
	return out
}

func (f *fakeSubagentProvider) SubagentBus() notify.Bus {
	return f.bus
}

func newSubagentTestServer(t *testing.T, provider SubagentProvider) *Server {
	t.Helper()
	s := &Server{
		deps: ServerDeps{
			Store:            &fakeWebStore{},
			Config:           minimalConfig(),
			SubagentProvider: provider,
		},
		mux:        http.NewServeMux(),
		wsUpgrader: newWSUpgrader(nil),
	}
	s.routes()
	return s
}

// ---------------------------------------------------------------------------
// Task 3.1 — GET /api/subagents/active
// ---------------------------------------------------------------------------

func TestHandleSubagentsActive_EmptyList(t *testing.T) {
	provider := &fakeSubagentProvider{}
	srv := newSubagentTestServer(t, provider)

	req := httptest.NewRequest(http.MethodGet, "/api/subagents/active", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Active []json.RawMessage `json:"active"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Active) != 0 {
		t.Errorf("expected empty array, got %d entries", len(resp.Active))
	}
}

func TestHandleSubagentsActive_WithLiveSubs(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	active := []agent.SubagentStatus{
		{
			ID:           "sub-001",
			BatchID:      "batch-001",
			SkillName:    "researcher",
			ConvID:       "sub_sub-001",
			ParentConvID: "conv_parent",
			Status:       "running",
			Cost:         0.12,
			Turns:        5,
			SpawnedAt:    now,
		},
		{
			ID:           "sub-002",
			BatchID:      "batch-002",
			SkillName:    "summarizer",
			ConvID:       "sub_sub-002",
			ParentConvID: "conv_parent",
			Status:       "running",
			Cost:         0.05,
			Turns:        2,
			SpawnedAt:    now,
		},
	}

	provider := &fakeSubagentProvider{active: active}
	srv := newSubagentTestServer(t, provider)

	req := httptest.NewRequest(http.MethodGet, "/api/subagents/active", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp struct {
		Active []struct {
			SubagentID   string    `json:"subagent_id"`
			BatchID      string    `json:"batch_id"`
			SkillName    string    `json:"skill_name"`
			ParentConvID string    `json:"parent_conv_id"`
			Status       string    `json:"status"`
			CostUSD      float64   `json:"cost_usd"`
			TurnCount    int       `json:"turn_count"`
			StartedAt    time.Time `json:"started_at"`
		} `json:"active"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(resp.Active) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(resp.Active))
	}

	first := resp.Active[0]
	if first.SubagentID == "" {
		t.Error("subagent_id is empty")
	}
	if first.BatchID == "" {
		t.Error("batch_id is empty")
	}
	if first.SkillName == "" {
		t.Error("skill_name is empty")
	}
	if first.Status != "running" {
		t.Errorf("status = %q, want 'running'", first.Status)
	}
	if first.CostUSD == 0 && first.TurnCount == 0 {
		t.Error("expected non-zero cost_usd or turn_count")
	}
	if first.StartedAt.IsZero() {
		t.Error("started_at is zero")
	}
}

func TestHandleSubagentsActive_NilProvider(t *testing.T) {
	// nil SubagentProvider → empty array, not 404
	srv := newSubagentTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/subagents/active", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Active []json.RawMessage `json:"active"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Active) != 0 {
		t.Errorf("expected empty array, got %d entries", len(resp.Active))
	}
}

// ---------------------------------------------------------------------------
// Task 3.3 — WS /api/ws/subagents event stream
// ---------------------------------------------------------------------------

// wsSubagentClient dials the WS endpoint and collects frames.
type wsSubagentClient struct {
	conn   *websocket.Conn
	frames []map[string]any
	mu     sync.Mutex
	done   chan struct{}
}

func newWSSubagentClient(t *testing.T, ts *httptest.Server) *wsSubagentClient {
	t.Helper()
	u := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/subagents"
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("WS dial: %v", err)
	}
	c := &wsSubagentClient{conn: conn, done: make(chan struct{})}
	go c.readLoop()
	return c
}

func (c *wsSubagentClient) readLoop() {
	defer close(c.done)
	for {
		var frame map[string]any
		if err := c.conn.ReadJSON(&frame); err != nil {
			return
		}
		c.mu.Lock()
		c.frames = append(c.frames, frame)
		c.mu.Unlock()
	}
}

func (c *wsSubagentClient) waitForFrame(eventType string, timeout time.Duration) (map[string]any, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		for _, f := range c.frames {
			if f["event"] == eventType {
				c.mu.Unlock()
				return f, true
			}
		}
		c.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	return nil, false
}

func (c *wsSubagentClient) frameCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.frames)
}

func (c *wsSubagentClient) close() { _ = c.conn.Close() }

func TestHandleSubagentsWS_ReceivesSpawnedAndCompletedFrames(t *testing.T) {
	bus := notify.NewEventBus(256, 0, 0)
	t.Cleanup(func() { bus.Close() })

	provider := &fakeSubagentProvider{bus: bus}
	srv := newSubagentTestServer(t, provider)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	client := newWSSubagentClient(t, ts)
	t.Cleanup(client.close)

	// Allow the WS handler to subscribe before emitting.
	time.Sleep(50 * time.Millisecond)

	now := time.Now()
	spawnMeta := map[string]string{
		"subagent_id":    "sub-abc",
		"batch_id":       "batch-abc",
		"skill":          "researcher",
		"parent_conv_id": "conv_parent",
		"max_cost_usd":   "0.5000",
		"max_turns":      "20",
		"timeout_sec":    "600",
	}
	bus.Emit(notify.Event{
		Type:      notify.EventSubagentSpawned,
		Origin:    notify.OriginAgent,
		ChannelID: "sub:sub-abc",
		Timestamp: now,
		Meta:      spawnMeta,
	})

	frame, found := client.waitForFrame("agent.subagent.spawned", 3*time.Second)
	if !found {
		t.Fatal("spawned frame not received within 3s")
	}
	if frame["event"] != "agent.subagent.spawned" {
		t.Errorf("event = %q, want 'agent.subagent.spawned'", frame["event"])
	}
	payload, _ := frame["payload"].(map[string]any)
	if payload == nil {
		t.Fatal("payload is nil or wrong type")
	}

	// Now emit completed.
	completedMeta := map[string]string{
		"subagent_id":    "sub-abc",
		"batch_id":       "batch-abc",
		"skill":          "researcher",
		"parent_conv_id": "conv_parent",
		"cost_usd":       "0.1234",
		"turns":          "3",
	}
	bus.Emit(notify.Event{
		Type:      notify.EventSubagentCompleted,
		Origin:    notify.OriginAgent,
		ChannelID: "sub:sub-abc",
		Timestamp: now,
		Meta:      completedMeta,
	})

	frame2, found2 := client.waitForFrame("agent.subagent.completed", 3*time.Second)
	if !found2 {
		t.Fatal("completed frame not received within 3s")
	}
	if frame2["event"] != "agent.subagent.completed" {
		t.Errorf("event = %q, want 'agent.subagent.completed'", frame2["event"])
	}
}

// ---------------------------------------------------------------------------
// Task 3.4 — 10 subscribers; slow consumer does not block others
// ---------------------------------------------------------------------------

func TestHandleSubagentsWS_SlowConsumerDoesNotBlockOthers(t *testing.T) {
	bus := notify.NewEventBus(256, 0, 0)
	t.Cleanup(func() { bus.Close() })

	provider := &fakeSubagentProvider{bus: bus}
	srv := newSubagentTestServer(t, provider)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	// Create 10 subscribers.
	clients := make([]*wsSubagentClient, 10)
	for i := range clients {
		clients[i] = newWSSubagentClient(t, ts)
	}
	t.Cleanup(func() {
		for _, c := range clients {
			c.close()
		}
	})

	// Allow all to subscribe.
	time.Sleep(100 * time.Millisecond)

	// Close one client to simulate a slow/dead consumer (its connection blocks).
	_ = clients[0].conn.Close()

	// Emit 3 events.
	for i := 0; i < 3; i++ {
		bus.Emit(notify.Event{
			Type:      notify.EventSubagentSpawned,
			Origin:    notify.OriginAgent,
			ChannelID: "sub:test",
			Timestamp: time.Now(),
			Meta: map[string]string{
				"subagent_id": "sub-test",
				"batch_id":    "batch-test",
				"skill":       "researcher",
			},
		})
	}

	// All healthy clients (1..9) should receive at least 1 frame within 3s.
	deadline := time.Now().Add(3 * time.Second)
	for i := 1; i < 10; i++ {
		for time.Now().Before(deadline) {
			if clients[i].frameCount() > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		if clients[i].frameCount() == 0 {
			t.Errorf("client[%d] received no frames — slow consumer may have blocked it", i)
		}
	}
}
