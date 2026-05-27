package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"daimon/internal/channel"
	"daimon/internal/notify"
	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ErrSubagentDepthExceeded is returned when a subagent tries to spawn another
// subagent. Depth limit is 1 in V1 — recursive spawning is deferred.
var ErrSubagentDepthExceeded = errors.New("subagents may not spawn other subagents in this version")

// SpawnMode controls whether the spawn tool blocks the parent turn or returns immediately.
type SpawnMode string

const (
	SpawnModeSync  SpawnMode = "sync"
	SpawnModeAsync SpawnMode = "async"
)

// subRecord holds mutable runtime state for a single spawned subagent.
// Protected by rec.mu for cost/turns/status/softWarned; the outer SubagentManager.mu
// protects subs and callerIsSub maps.
type subRecord struct {
	id           string
	batchID      string
	skillName    string
	convID       string
	parentConvID string

	subChannel *channel.SubagentChannel
	ctx        context.Context
	cancel     context.CancelFunc

	budget     skill.BudgetConfig
	cost       float64 // cumulative USD
	turns      int
	softWarned bool

	status     string // "running" | "completed" | "failed" | "cancelled"
	failReason string
	result     *SubagentResult
	spawnedAt  time.Time

	done   chan struct{}     // closed when finalized
	events chan notify.Event // buffered cap-8 for budgetMonitor fan-out

	mu sync.Mutex

	// REQ-11, D3: per-handle filtered subscription fields.
	// bus is the shared bus set at Spawn time; subMu protects subs and subInstalled.
	bus          notify.Bus
	subMu        sync.Mutex
	subs         []chan notify.Event
	subInstalled bool
}

// BudgetConfig is a resolved budget consumed by SubagentManager.
// Separate from skill.BudgetConfig so the agent package can reference it
// without an import cycle if needed; currently aliased from skill.BudgetConfig.
// (Re-exported so tests outside the package can build SubagentHandle.Status().)

// SubagentResult is the final output of a completed subagent.
type SubagentResult struct {
	Status    string            `json:"status"`  // "completed" | "failed" | "cancelled"
	Summary   string            `json:"summary"` // final assistant text
	Artifacts map[string]string `json:"artifacts,omitempty"`
	Cost      float64           `json:"cost_usd"`
	Turns     int               `json:"turns"`
	Errors    []string          `json:"errors,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SubagentStatus is a read-only snapshot returned by Active()/All()/Get().
type SubagentStatus struct {
	ID, BatchID, SkillName, ConvID, ParentConvID string
	Status                                       string
	Cost                                         float64
	Turns                                        int
	SpawnedAt                                    time.Time
	Budget                                       skill.BudgetConfig
}

// SubagentHandle is returned by Spawn. The caller uses Wait() for sync mode.
type SubagentHandle struct {
	ID      string
	BatchID string
	rec     *subRecord
}

// Wait blocks until the subagent finishes or ctx is done.
func (h *SubagentHandle) Wait(ctx context.Context) (*SubagentResult, error) {
	select {
	case <-h.rec.done:
		return h.rec.result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Cancel cancels this specific subagent. Idempotent.
func (h *SubagentHandle) Cancel() { h.rec.cancel() }

// Subscribe returns a channel that yields only bus events attributed to this
// subagent (filtered by Meta["subagent_id"] == h.ID). The channel is closed
// when ctx is cancelled or the subagent finishes (h.rec.done is closed).
//
// Implementation (D3): a single bus.Subscribe handler is installed lazily on
// the first Subscribe call. It fans out to all active per-Subscribe channels
// using a non-blocking send (REQ-18: thin-dispatcher contract). Slow consumers
// receive a WARN log and their event is dropped.
//
// If h.rec.bus is nil, a closed channel is returned immediately (REQ-11
// nil-bus safety).
func (h *SubagentHandle) Subscribe(ctx context.Context) (<-chan notify.Event, error) {
	h.rec.subMu.Lock()
	defer h.rec.subMu.Unlock()

	// Nil bus — return a closed channel immediately so range loops exit.
	if h.rec.bus == nil {
		ch := make(chan notify.Event)
		close(ch)
		return ch, nil
	}

	// Lazy: install the shared bus subscription on the first Subscribe call.
	if !h.rec.subInstalled {
		id := h.rec.id
		rec := h.rec
		h.rec.bus.Subscribe(func(ev notify.Event) {
			// Fast path: filter by subagent_id.
			if ev.Meta["subagent_id"] != id {
				return
			}
			// Hold subMu for the entire fan-out loop so that no cleanup
			// goroutine can close a channel while we are (or are about to
			// be) sending on it. This is deadlock-free ONLY BECAUSE all
			// sends are non-blocking (select/default) and no other blocking
			// call occurs while subMu is held. WARNING: adding any blocking
			// call here would allow the callWithTimeout abandoned-goroutine
			// path in bus.go to stall cleanup goroutines waiting on subMu.
			rec.subMu.Lock()
			for _, sub := range rec.subs {
				select {
				case sub <- ev: // non-blocking
				default:
					slog.Warn("subagent subscriber lagging, dropping event",
						"subagent_id", id, "type", ev.Type)
				}
			}
			rec.subMu.Unlock()
		})
		h.rec.subInstalled = true
	}

	// Per-Subscribe channel with a generous buffer (REQ-18: absorbs ~3s of TUI
	// render hiccup at worst-case bus throughput).
	ch := make(chan notify.Event, 32)
	h.rec.subs = append(h.rec.subs, ch)

	// Cleanup goroutine: close ch when ctx cancels OR rec.done closes.
	// Removes ch from rec.subs AND closes it while still holding subMu so
	// the fan-out handler can never send on a channel that has been closed.
	go func() {
		select {
		case <-ctx.Done():
		case <-h.rec.done:
		}
		h.rec.subMu.Lock()
		for i, c := range h.rec.subs {
			if c == ch {
				h.rec.subs = append(h.rec.subs[:i], h.rec.subs[i+1:]...)
				break
			}
		}
		close(ch) // inside the lock: mutual exclusion with the fan-out loop
		h.rec.subMu.Unlock()
	}()

	return ch, nil
}

// Status returns a read-only snapshot of the subagent's current state.
func (h *SubagentHandle) Status() SubagentStatus {
	h.rec.mu.Lock()
	defer h.rec.mu.Unlock()
	return SubagentStatus{
		ID:           h.rec.id,
		BatchID:      h.rec.batchID,
		SkillName:    h.rec.skillName,
		ConvID:       h.rec.convID,
		ParentConvID: h.rec.parentConvID,
		Status:       h.rec.status,
		Cost:         h.rec.cost,
		Turns:        h.rec.turns,
		SpawnedAt:    h.rec.spawnedAt,
		Budget:       h.rec.budget,
	}
}

// SubagentManager owns the full spawn lifecycle, budget polling, status
// tracking, and context cascade for all child agents.
type SubagentManager struct {
	bus   notify.Bus
	store store.Store

	mu          sync.RWMutex
	subs        map[string]*subRecord
	callerIsSub map[string]bool // convID → true for depth guard

	// softWarnFn is the callback for 80% budget warning.
	// Overridable by tests to track calls without triggering channel delivery.
	softWarnFn func(*subRecord)

	// newChildAgent is the production factory for child Agents and the test seam.
	// In production it is wired by Agent.WithExecutableSkills via makeChildAgentFn.
	// In tests it is replaced by newTestManager's fake closure to avoid starting
	// real LLM-connected Agents.
	newChildAgent func(
		def skill.ExecutableSkillDef,
		prompt string,
		subCtx context.Context,
		subCh *channel.SubagentChannel,
		parentTools map[string]tool.Tool,
		st store.Store,
	) (*Agent, error)
}

// NewSubagentManager constructs a SubagentManager. parentTools is the
// snapshot of the parent agent's tools at spawn time.
func NewSubagentManager(
	bus notify.Bus,
	st store.Store,
) *SubagentManager {
	m := &SubagentManager{
		bus:         bus,
		store:       st,
		subs:        make(map[string]*subRecord),
		callerIsSub: make(map[string]bool),
	}
	m.softWarnFn = m.injectSoftWarning
	return m
}

// installBusSubscription registers the single bus subscriber that fans out
// EventTurnCompleted events to the per-record events channel (cap-8 + drop+warn).
// Must be called once after construction.
func (m *SubagentManager) installBusSubscription() {
	if m.bus == nil {
		return
	}
	m.bus.Subscribe(func(ev notify.Event) {
		if ev.Type != notify.EventTurnCompleted {
			return
		}
		m.mu.RLock()
		var target *subRecord
		for _, rec := range m.subs {
			if rec.subChannel.ID() == ev.ChannelID {
				target = rec
				break
			}
		}
		m.mu.RUnlock()
		if target == nil {
			return
		}
		select {
		case target.events <- ev:
		default:
			slog.Warn("subagent budget monitor lagging, dropping event", "id", target.id)
		}
	})
}

// Spawn creates and starts a child agent for the given skill definition.
// callerConvID is the parent's conversation ID used for the depth guard.
func (m *SubagentManager) Spawn(
	ctx context.Context,
	def skill.ExecutableSkillDef,
	prompt string,
	mode SpawnMode,
	callerConvID string,
) (*SubagentHandle, error) {
	// Depth-1 guard: reject if the caller is itself a sub.
	m.mu.RLock()
	isSub := m.callerIsSub[callerConvID]
	m.mu.RUnlock()
	if isSub {
		return nil, ErrSubagentDepthExceeded
	}

	id := uuid.New().String()
	batchID := id // V1: 1:1 mapping; V2 will introduce real batch grouping
	childConvID := "sub_" + id

	// REQ-16: only wrap with a timeout when Budget.Timeout > 0. A zero Timeout
	// means "no time limit" — context.WithTimeout(ctx, 0) cancels instantly.
	var subCtx context.Context
	var cancel context.CancelFunc
	if def.Budget.Timeout > 0 {
		subCtx, cancel = context.WithTimeout(ctx, def.Budget.Timeout)
	} else {
		subCtx, cancel = context.WithCancel(ctx)
	}

	// Persist the child conversation before starting the child agent.
	now := time.Now()
	conv := store.Conversation{
		ID:           childConvID,
		ChannelID:    "subagent",
		ParentConvID: callerConvID,
		Status:       "running",
		Metadata: map[string]string{
			"subagent_id": id,
			"batch_id":    batchID,
			"skill":       def.Name,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := m.store.SaveConversation(ctx, conv); err != nil {
		cancel()
		return nil, fmt.Errorf("subagent: persist conversation: %w", err)
	}

	// Check if parent ctx was cancelled between save and run.
	if ctx.Err() != nil {
		cancel()
		_ = m.store.SetConversationStatus(context.Background(), childConvID, "cancelled")
		m.bus.Emit(notify.Event{
			Type:      notify.EventSubagentFailed,
			Origin:    notify.OriginAgent,
			ChannelID: "sub:" + id,
			Timestamp: time.Now(),
			Meta: map[string]string{
				"subagent_id":    id,
				"batch_id":       batchID,
				"skill":          def.Name,
				"parent_conv_id": callerConvID,
				"reason":         "cancelled_during_spawn",
			},
		})
		return nil, ctx.Err()
	}

	subCh := channel.NewSubagentChannel(id)

	rec := &subRecord{
		id:           id,
		batchID:      batchID,
		skillName:    def.Name,
		convID:       childConvID,
		parentConvID: callerConvID,
		subChannel:   subCh,
		ctx:          subCtx,
		cancel:       cancel,
		budget:       def.Budget,
		status:       "running",
		spawnedAt:    now,
		done:         make(chan struct{}),
		events:       make(chan notify.Event, 8),
		bus:          m.bus, // REQ-11, D3: stored for lazy Subscribe bus subscription
	}

	m.mu.Lock()
	m.subs[id] = rec
	m.callerIsSub[childConvID] = true
	m.mu.Unlock()

	// Start the child agent via the seam.
	// parentTools is nil here in unit tests; agent.New() wiring passes real tools.
	childTools := filterParentTools(nil, def.ToolsAllowlist)
	if m.newChildAgent != nil {
		if _, err := m.newChildAgent(def, prompt, subCtx, subCh, childTools, m.store); err != nil {
			cancel()
			m.mu.Lock()
			delete(m.subs, id)
			delete(m.callerIsSub, childConvID)
			m.mu.Unlock()
			return nil, fmt.Errorf("subagent: create child agent: %w", err)
		}
	}

	// Deliver the prompt once the channel is started.
	// newChildAgent is responsible for calling subCh.Start before returning.
	if err := subCh.Deliver(prompt); err != nil {
		slog.Warn("subagent: failed to deliver prompt", "id", id, "error", err)
	}

	// Budget monitor goroutine.
	go m.budgetMonitor(rec)

	// Emit EventSubagentSpawned.
	m.bus.Emit(notify.Event{
		Type:      notify.EventSubagentSpawned,
		Origin:    notify.OriginAgent,
		ChannelID: subCh.ID(),
		Text:      truncate(prompt, 200),
		Timestamp: time.Now(),
		Meta: map[string]string{
			"subagent_id":    id,
			"batch_id":       batchID,
			"skill":          def.Name,
			"parent_conv_id": callerConvID,
			"max_cost_usd":   fmt.Sprintf("%.4f", def.Budget.MaxCostUSD),
			"max_turns":      strconv.Itoa(def.Budget.MaxTurns),
			"timeout_sec":    strconv.FormatInt(int64(def.Budget.Timeout.Seconds()), 10),
		},
	})

	handle := &SubagentHandle{ID: id, BatchID: batchID, rec: rec}
	return handle, nil
}

// budgetMonitor runs in a goroutine per spawn. It listens on rec.events
// (EventTurnCompleted forwarded by the bus subscription) and enforces
// cost/turn caps. On ctx cancellation it finalizes with "cancelled".
func (m *SubagentManager) budgetMonitor(rec *subRecord) {
	for {
		select {
		case <-rec.ctx.Done():
			// Sub context cancelled (timeout or explicit cancel).
			rec.mu.Lock()
			currentStatus := rec.status
			rec.mu.Unlock()
			if currentStatus == "running" {
				reason := "cancelled"
				if errors.Is(rec.ctx.Err(), context.DeadlineExceeded) {
					reason = "budget_exceeded" // timeout_min cap — spec REQ-5 mandates "budget_exceeded" for all budget types
				}
				m.finalize(rec, "failed", reason)
			}
			return

		case ev, ok := <-rec.events:
			if !ok {
				return
			}

			// Parse cost from event meta.
			turnCost, _ := strconv.ParseFloat(ev.Meta["cost_usd"], 64)

			rec.mu.Lock()
			rec.cost += turnCost
			rec.turns++

			softHit := !rec.softWarned && rec.budget.MaxCostUSD > 0 &&
				rec.cost >= 0.8*rec.budget.MaxCostUSD
			hardCost := rec.budget.MaxCostUSD > 0 && rec.cost >= rec.budget.MaxCostUSD
			hardTurns := rec.budget.MaxTurns > 0 && rec.turns >= rec.budget.MaxTurns
			if softHit {
				rec.softWarned = true
			}
			currentStatus := rec.status
			rec.mu.Unlock()

			if currentStatus != "running" {
				return // already finalized
			}

			if softHit && m.softWarnFn != nil {
				m.softWarnFn(rec)
			}

			if hardCost {
				m.finalize(rec, "failed", "budget_exceeded")
				rec.cancel()
				return
			}
			if hardTurns {
				m.finalize(rec, "failed", "budget_exceeded")
				rec.cancel()
				return
			}

			// Check if child completed naturally (final assistant message observed).
			if final := rec.subChannel.FinalAssistant(); final != "" {
				m.finalize(rec, "completed", "")
				return
			}
		}
	}
}

// finalize sets the terminal status on rec, persists it, emits the lifecycle
// event, and closes rec.done. Idempotent — only the first call takes effect.
func (m *SubagentManager) finalize(rec *subRecord, status, reason string) {
	rec.mu.Lock()
	if rec.status != "running" {
		rec.mu.Unlock()
		return // already finalized
	}
	rec.status = status
	rec.failReason = reason
	cost := rec.cost
	turns := rec.turns
	summary := rec.subChannel.FinalAssistant()
	rec.result = &SubagentResult{
		Status:  status,
		Summary: summary,
		Cost:    cost,
		Turns:   turns,
		Metadata: map[string]string{
			"subagent_id": rec.id,
			"batch_id":    rec.batchID,
		},
	}
	if reason != "" && status != "completed" {
		rec.result.Errors = []string{reason}
	}
	rec.mu.Unlock()

	close(rec.done)

	// Persist status.
	storeStatus := status
	if status == "failed" || status == "cancelled" {
		storeStatus = status
	}
	if err := m.store.SetConversationStatus(context.Background(), rec.convID, storeStatus); err != nil {
		slog.Warn("subagent: failed to persist status", "id", rec.id, "status", status, "error", err)
	}

	// Emit lifecycle event.
	if m.bus == nil {
		return
	}
	meta := map[string]string{
		"subagent_id":    rec.id,
		"batch_id":       rec.batchID,
		"skill":          rec.skillName,
		"parent_conv_id": rec.parentConvID,
		"cost_usd":       fmt.Sprintf("%.4f", cost),
		"turns":          strconv.Itoa(turns),
	}

	evType := notify.EventSubagentCompleted
	if status == "failed" || status == "cancelled" {
		evType = notify.EventSubagentFailed
		meta["reason"] = reason
	}

	m.bus.Emit(notify.Event{
		Type:      evType,
		Origin:    notify.OriginAgent,
		ChannelID: rec.subChannel.ID(),
		Text:      truncate(summary, 500),
		Timestamp: time.Now(),
		Meta:      meta,
	})
}

// injectSoftWarning delivers a synthetic user message into the child's inbox
// warning it to wrap up, as it has consumed 80% of its cost budget.
// The budgetMonitor guarantees this is called at most once per record (via
// the softWarned flag), so this function never needs to re-check the flag.
//
// Message text follows the format specified by SUBAGENTS-REQ-5:
//
//	⚠ Budget warning: you have used 80% of your cost cap
//	(current: 0.XXXX, max: Y.YYYY). Wrap up and produce a final summary.
func (m *SubagentManager) injectSoftWarning(rec *subRecord) {
	rec.mu.Lock()
	cost := rec.cost
	capUSD := rec.budget.MaxCostUSD
	skillName := rec.skillName
	id := rec.id
	rec.mu.Unlock()

	text := fmt.Sprintf(
		"⚠ Budget warning: you have used 80%% of your cost cap "+
			"(current: %.4f, max: %.4f). "+
			"Wrap up your work and produce a final summary.",
		cost, capUSD,
	)

	if err := rec.subChannel.Deliver(text); err != nil {
		slog.Warn("subagent: failed to inject soft warning",
			"id", id,
			"skill", skillName,
			"error", err,
		)
	}
}

// Cancel cancels a single subagent by ID. Idempotent.
// Returns an error if the ID is not found.
func (m *SubagentManager) Cancel(subID string) error {
	m.mu.RLock()
	rec, ok := m.subs[subID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("subagent %q not found", subID)
	}
	rec.cancel()
	return nil
}

// Active returns a snapshot of currently running subagents.
func (m *SubagentManager) Active() []SubagentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []SubagentStatus
	for _, rec := range m.subs {
		rec.mu.Lock()
		if rec.status == "running" {
			out = append(out, SubagentStatus{
				ID:           rec.id,
				BatchID:      rec.batchID,
				SkillName:    rec.skillName,
				ConvID:       rec.convID,
				ParentConvID: rec.parentConvID,
				Status:       rec.status,
				Cost:         rec.cost,
				Turns:        rec.turns,
				SpawnedAt:    rec.spawnedAt,
				Budget:       rec.budget,
			})
		}
		rec.mu.Unlock()
	}
	return out
}

// All returns a snapshot of all subagents (running + finished).
func (m *SubagentManager) All() []SubagentStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SubagentStatus, 0, len(m.subs))
	for _, rec := range m.subs {
		rec.mu.Lock()
		out = append(out, SubagentStatus{
			ID:           rec.id,
			BatchID:      rec.batchID,
			SkillName:    rec.skillName,
			ConvID:       rec.convID,
			ParentConvID: rec.parentConvID,
			Status:       rec.status,
			Cost:         rec.cost,
			Turns:        rec.turns,
			SpawnedAt:    rec.spawnedAt,
			Budget:       rec.budget,
		})
		rec.mu.Unlock()
	}
	return out
}

// Get returns the status for a single subagent.
func (m *SubagentManager) Get(subID string) (SubagentStatus, bool) {
	m.mu.RLock()
	rec, ok := m.subs[subID]
	m.mu.RUnlock()
	if !ok {
		return SubagentStatus{}, false
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return SubagentStatus{
		ID:           rec.id,
		BatchID:      rec.batchID,
		SkillName:    rec.skillName,
		ConvID:       rec.convID,
		ParentConvID: rec.parentConvID,
		Status:       rec.status,
		Cost:         rec.cost,
		Turns:        rec.turns,
		SpawnedAt:    rec.spawnedAt,
		Budget:       rec.budget,
	}, true
}

// filterParentTools builds a child tool map by filtering the parent's tools
// against the allowlist. An empty allowlist means inherit all parent tools.
// Unknown names in the allowlist are silently dropped (two-phase validation:
// the loader already warned at New() time). A nil parent returns an empty map.
func filterParentTools(parent map[string]tool.Tool, allowlist []string) map[string]tool.Tool {
	if parent == nil {
		return make(map[string]tool.Tool)
	}
	if len(allowlist) == 0 {
		// Empty allowlist = inherit all parent tools.
		out := make(map[string]tool.Tool, len(parent))
		for k, v := range parent {
			out[k] = v
		}
		return out
	}
	out := make(map[string]tool.Tool, len(allowlist))
	for _, name := range allowlist {
		if t, ok := parent[name]; ok {
			out[name] = t
		}
		// Unknown names already warned at agent.New() — silently drop here.
	}
	return out
}
