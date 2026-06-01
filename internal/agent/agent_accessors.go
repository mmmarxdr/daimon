package agent

// agent_accessors.go — additive read-only public accessors for the TUI (AD-8, REQ-14).
//
// These methods expose internal agent state for the TUI without modifying it.
// Both are PR1 additions; they must NOT be called from any existing agent-loop
// path to avoid lock inversion. Callers are the TUI's render path and PR4's
// screen_tools.go (screen 05).

import "daimon/internal/tool"

// ToolRegistry returns a SNAPSHOT copy of the live tool registry map.
//
// Thread-safety: acquires a.toolsMu RLock for the duration of the copy.
// The returned map is independent — mutations by the caller do NOT affect
// the live registry (which may be mutated concurrently by hot-reload,
// hot_reload.go). Callers must not hold the result across agent restarts.
func (a *Agent) ToolRegistry() map[string]tool.Tool {
	a.toolsMu.RLock()
	defer a.toolsMu.RUnlock()
	out := make(map[string]tool.Tool, len(a.tools))
	for k, v := range a.tools {
		out[k] = v
	}
	return out
}

// TodoListForConv returns the current TodoList for the given conversation ID.
//
// This is a thin wrapper over the existing private a.todoRead method
// (todo_bridge.go:145) which is already thread-safe via resolveTodoConv
// (registry-first, store-fallback). No new mutex is introduced.
func (a *Agent) TodoListForConv(convID string) (tool.TodoList, error) {
	return a.todoRead(convID)
}

// CurrentMode returns the name of the active mode ("plan", "build", or "review").
//
// It is a thin public wrapper over modeSnapshot() (PR5 / AD-8 pattern). The
// lock is acquired and released inside modeSnapshot — do NOT add locking here.
// Falls back to "build" when currentMode is empty or corrupt, matching the
// modeSnapshot fallback contract.
func (a *Agent) CurrentMode() string {
	return a.modeSnapshot().Name
}

// ContextWindowSize returns the resolved context window size in tokens as
// determined by the agent's ContextManager at construction time.
//
// Returns 0 when contextMgr is nil (provider unknown or legacy path).
// The returned 0 is the sentinel for "unknown" — callers MUST fall back to
// their own heuristic (e.g. 200 000 tokens) when this returns 0.
//
// This is a static value read once at TUI boot — it does NOT change during
// the agent's lifetime (ADR-6, tui-backend-seams).
func (a *Agent) ContextWindowSize() int {
	if a.contextMgr == nil {
		return 0
	}
	return a.contextMgr.MaxTokens()
}
