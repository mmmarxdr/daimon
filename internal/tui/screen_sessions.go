package tui

// screen_sessions.go — sessions screen handler (screen 06, PR3b).
//
// The sessions screen allows browsing and resuming past conversations.
// Navigation: up/down (or k/j) moves selection; enter resumes the selected
// conversation (sets activeConvID and transitions to chat); esc returns to
// the previous screen.
//
// /save, /fork, /export are handled by the PR3a command palette — not here.
//
// RULE: No IO in Update. All store reads happen inside tea.Cmd closures.

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"daimon/internal/store"
)

// sessionsLoadedMsg is returned by loadSessionsCmd when the store list completes.
type sessionsLoadedMsg struct {
	convs []store.Conversation
	err   error
}

// loadSessionsCmd returns a tea.Cmd that reads up to 50 conversations from the
// store and delivers the result as a sessionsLoadedMsg.
// Guard: nil store → return empty sessionsLoadedMsg so tests with nil store don't panic.
func loadSessionsCmd(st store.Store) tea.Cmd {
	return func() tea.Msg {
		if st == nil {
			return sessionsLoadedMsg{}
		}
		convs, err := st.ListConversations(context.Background(), "tui", 50)
		return sessionsLoadedMsg{convs: convs, err: err}
	}
}

// updateSessions is the screenSessions Update handler. It is called from
// Model.Update when m.screen == screenSessions.
func (m Model) updateSessions(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case sessionsLoadedMsg:
		m.sessions = msg.convs
		// Clamp sessionIdx to [0, len-1]; 0 when empty.
		if len(m.sessions) == 0 {
			m.sessionIdx = 0
		} else if m.sessionIdx >= len(m.sessions) {
			m.sessionIdx = len(m.sessions) - 1
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {

		case "up", "k":
			if m.sessionIdx > 0 {
				m.sessionIdx--
			}
			return m, nil

		case "down", "j":
			if len(m.sessions) > 0 && m.sessionIdx < len(m.sessions)-1 {
				m.sessionIdx++
			}
			return m, nil

		case "enter":
			if len(m.sessions) > 0 {
				m.activeConvID = m.sessions[m.sessionIdx].ID
				m.screen = screenChat
				m.focus = focusEditor
			}
			return m, nil

		case "esc":
			m.screen = m.prevScreen
			return m, nil
		}
	}

	return m, nil
}

// renderSessions renders the center column for the sessions screen.
// Each row shows: short ID (8 chars), relative updated-ago, status, and title.
// The selected row is highlighted. A small preview of the selected conv is shown
// below the list. All width math uses ansi.StringWidth/ansi.Truncate (no len/byte).
func renderSessions(m Model, width, height int) string {
	if len(m.sessions) == 0 {
		msg := m.styles.dimLabel.Render("no sessions yet — start chatting to create one")
		return centerText(msg, width)
	}

	inner := width
	if inner < 8 {
		inner = 8
	}

	var rows []string

	header := m.styles.accent.Render("◈ sessions")
	rows = append(rows, ansi.Truncate(header, inner, "…"))
	rows = append(rows, "")

	for i, conv := range m.sessions {
		// Short ID: first 8 chars.
		shortID := conv.ID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		// Relative time from UpdatedAt.
		ago := relativeTime(conv.UpdatedAt)

		// Title: from metadata or "(untitled)".
		title := conv.Metadata["title"]
		if title == "" {
			title = "(untitled)"
		}

		// Branch marker for forked conversations.
		branchMark := ""
		if conv.ParentConvID != "" {
			branchMark = " ⎇"
		}

		line := fmt.Sprintf("%-8s  %-8s  %-9s  %s%s", shortID, ago, conv.Status, title, branchMark)

		if i == m.sessionIdx {
			line = m.styles.selected.Render(ansi.Truncate(line, inner, "…"))
		} else {
			line = m.styles.dimLabel.Render(ansi.Truncate(line, inner, "…"))
		}
		rows = append(rows, line)
	}

	// Preview of selected conv below list.
	if m.sessionIdx < len(m.sessions) {
		sel := m.sessions[m.sessionIdx]
		rows = append(rows, "")

		msgCount := fmt.Sprintf("messages: %d", len(sel.Messages))
		rows = append(rows, m.styles.dimLabel.Render(ansi.Truncate(msgCount, inner, "…")))

		if sel.CompactedSummary != "" {
			summary := sel.CompactedSummary
			const maxSummaryLen = 120
			if len(summary) > maxSummaryLen {
				summary = summary[:maxSummaryLen] + "…"
			}
			rows = append(rows, m.styles.dimLabel.Render(ansi.Truncate("summary: "+summary, inner, "…")))
		}
	}

	// Pad to height to avoid layout gaps.
	for len(rows) < height {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

// relativeTime returns a short human-readable relative time string for t.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
