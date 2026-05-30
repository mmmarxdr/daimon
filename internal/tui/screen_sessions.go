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
//
// NOTE: sessionsLoadedMsg is handled GLOBALLY in model.go Update (PR4b) so
// both the welcome resume-list panel and this screen are updated regardless
// of which screen is active. This handler no longer needs to process it.
func (m Model) updateSessions(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

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
				sel := m.sessions[m.sessionIdx]
				m.activeConvID = sel.ID
				// V1: resume rebinds activeConvID and clears the thread; prior history is not replayed.
				m.thread = thread{}
				// Reset the breadcrumb too: turns/tokens/label belong to the prior
				// session and must not bleed into the resumed one.
				m.breadcrumb = breadcrumb{styles: m.styles}
				shortID := sel.ID
				if len([]rune(shortID)) > 8 {
					shortID = string([]rune(shortID)[:8])
				}
				m.thread.append(&MsgDaimon{text: "↩ resumed session " + shortID, time: nowHHMM(), styles: m.styles})
				m.screen = screenChat
				m.focus = focusEditor
				m.footer = footerHints{screen: screenChat}
				// WU-c §C.6: reset viewport scroll on session resume + transition to chat.
				// Stale content/offset from the prior session must not bleed through.
				m.viewport.SetContent("")
				m.viewport.GotoTop()
				m = m.refreshThreadViewport()
			}
			return m, nil

		case "esc":
			m.screen = m.prevScreen
			m.footer = footerHints{screen: m.prevScreen}
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
	// Surface a load error instead of the misleading "no sessions yet" placeholder.
	if m.sessionsErr != nil {
		errMsg := "error loading sessions: " + m.sessionsErr.Error()
		return centerText(m.styles.errStyle.Render(errMsg), width)
	}
	if len(m.sessions) == 0 {
		msg := m.styles.dimLabel.Render("no sessions yet — start chatting to create one")
		return centerText(msg, width)
	}

	inner := width
	if inner < 8 {
		inner = 8
	}

	var rows []string

	header := m.styles.panelHeader("sessions")
	rows = append(rows, ansi.Truncate(header, inner, "…"))
	rows = append(rows, "")

	for i, conv := range m.sessions {
		// Short ID: first 8 runes (IDs are ASCII today, but guard rune-safely per
		// the project's ANSI-width rule — no byte-slicing of display strings).
		shortID := conv.ID
		if len([]rune(shortID)) > 8 {
			shortID = string([]rune(shortID)[:8])
		}

		// WU-b: read pre-computed ago string so renderSessions never calls
		// relativeTime (time.Since) from the View path.
		ago := ""
		if i < len(m.sessionsAgo) {
			ago = m.sessionsAgo[i]
		}

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
			// ANSI-safe truncation: ansi.Truncate measures visible columns and never
			// splits a multi-byte rune or ANSI escape sequence (unlike byte-slicing).
			const maxSummaryLen = 120
			summary := ansi.Truncate(sel.CompactedSummary, maxSummaryLen, "…")
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
