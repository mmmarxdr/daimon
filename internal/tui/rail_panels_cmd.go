package tui

// rail_panels_cmd.go — tea.Cmd helpers for rail panel data refresh (PR2b, PR-c).
//
// Cmd discipline: ALL IO (agent accessor calls, store queries) MUST happen inside
// a tea.Cmd closure. This file defines the Msg types and Cmd constructors; the
// model state mutation happens in Update when the Msg arrives.

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/agent"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// todolistRefreshMsg carries a freshly-read TodoList into the Update loop.
// Emitted by fetchTodolist (a tea.Cmd) after calling Agent.TodoListForConv.
type todolistRefreshMsg struct {
	list tool.TodoList
}

// fetchTodolist returns a tea.Cmd that calls ag.TodoListForConv(convID) on its
// own goroutine (Cmd discipline: no IO in Update) and delivers the result as
// a todolistRefreshMsg. If ag is nil or convID is empty, the Cmd is a no-op
// and returns a zero todolistRefreshMsg so the todolist panel simply shows
// whatever it already holds.
func fetchTodolist(ag *agent.Agent, convID string) tea.Cmd {
	return func() tea.Msg {
		if ag == nil || convID == "" {
			return todolistRefreshMsg{}
		}
		list, _ := ag.TodoListForConv(convID) // errors silently ignored; panel shows stale data
		return todolistRefreshMsg{list: list}
	}
}

// memoryRefreshMsg carries freshly-fetched memory entries into the Update loop.
// Emitted by fetchMemory (a tea.Cmd) after calling store.SearchMemory.
// An empty entries slice means no entries for the scope (panel renders "").
type memoryRefreshMsg struct {
	entries []store.MemoryEntry
}

// fetchMemory returns a tea.Cmd that calls st.SearchMemory on its own goroutine
// (Cmd discipline: no IO in Update) and delivers the result as a memoryRefreshMsg.
//
// When st is nil OR scopeID is empty, the Cmd returns memoryRefreshMsg{} immediately
// without calling SearchMemory (mirrors the fetchTodolist nil/empty guard).
// Errors from SearchMemory are silently ignored; the panel shows stale (or empty) data.
func fetchMemory(st store.Store, scopeID string) tea.Cmd {
	return func() tea.Msg {
		if st == nil || scopeID == "" {
			return memoryRefreshMsg{}
		}
		entries, _ := st.SearchMemory(context.Background(), scopeID, "", 5) // err ignored
		return memoryRefreshMsg{entries: entries}
	}
}
