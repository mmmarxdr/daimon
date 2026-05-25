package tui

// thread.go — thread component stub (PR2 implements the full thread).
// PR1 declares the type so Model compiles; Render returns empty string.

// thread holds the list of conversation thread items (MsgUser, MsgDaimon,
// ToolLine, Reasoning, Subagent). Populated in PR2 via bus events.
type thread struct {
	items []threadItem
}

// threadItem is the interface implemented by all thread components.
// Concrete types (MsgUser, MsgDaimon, ToolLine, Reasoning, Subagent) land in PR2.
type threadItem interface {
	Render(width int) string
}

// Render renders the thread into a vertical stack of item strings.
// Returns empty string when there are no items (welcome/initial state).
func (t *thread) Render(width int) string {
	if len(t.items) == 0 {
		return ""
	}
	out := ""
	for _, item := range t.items {
		out += item.Render(width) + "\n"
	}
	return out
}
