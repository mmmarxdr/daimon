package tui

// rail_panels_cmd.go — tea.Cmd helpers for rail panel data refresh (PR2b).
//
// Cmd discipline: ALL IO (agent accessor calls) MUST happen inside a tea.Cmd
// closure. This file defines the Msg types and Cmd constructors; the model
// state mutation happens in Update when the Msg arrives.

import (
	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/agent"
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
