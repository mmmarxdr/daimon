package tui

// screen_tools.go — tools screen handler (screen 05, PR4a).
//
// The tools screen lists all built-in tools registered with the agent,
// displaying each tool's name, Risk, and Category in a scrollable list.
// The right rail shows a tool-detail panel for the selected tool.
//
// MCP servers are NOT wired in the embedded TUI (tui_cmd.go builds the agent
// with built-in tools only). Only built-in tools are shown.
// TODO(PR4+): MCP server list when MCP is wired into the embedded TUI.
//
// Navigation: up/down (or k/j) moves selection; esc returns to prevScreen.
//
// RULE: No IO in Update. ToolRegistry snapshot runs inside tea.Cmd closure.

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"daimon/internal/agent"
	"daimon/internal/tool"
)

// toolEntry is a local snapshot of a single tool's name, description, and metadata.
// Built in loadToolsCmd to keep the Update path IO-free.
type toolEntry struct {
	name string
	desc string
	meta tool.ToolMeta
}

// toolsLoadedMsg is returned by loadToolsCmd when the tool registry snapshot completes.
type toolsLoadedMsg struct {
	tools []toolEntry
}

// loadToolsCmd returns a tea.Cmd that snapshots the agent's tool registry under
// RLock, builds ToolMeta for each tool, and delivers the result as a toolsLoadedMsg.
// The entries are sorted by name for deterministic rendering and golden tests.
// Guard: nil agent → return empty toolsLoadedMsg so tests with nil agent don't panic.
func loadToolsCmd(ag *agent.Agent) tea.Cmd {
	return func() tea.Msg {
		if ag == nil {
			return toolsLoadedMsg{}
		}
		reg := ag.ToolRegistry()
		metas := tool.BuildToolMeta(reg)

		entries := make([]toolEntry, 0, len(reg))
		for name, t := range reg {
			entries = append(entries, toolEntry{
				name: name,
				desc: t.Description(),
				meta: metas[name],
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].name < entries[j].name
		})
		return toolsLoadedMsg{tools: entries}
	}
}

// updateToolDetailPanel updates the toolDetailPanel in the rail for the given
// selection index. Called after navigation or after toolsLoadedMsg populates the list.
func (m Model) updateToolDetailPanel() Model {
	if len(m.tools) == 0 {
		return m
	}
	sel := m.tools[m.toolIdx]
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		if p, ok := panels[panelToolDetail].(*toolDetailPanel); ok {
			cp := *p
			cp.setTool(sel.name, sel.desc, sel.meta)
			panels[panelToolDetail] = &cp
		}
	})
	return m
}

// updateTools is the screenTools Update handler. It is called from
// Model.Update when m.screen == screenTools.
func (m Model) updateTools(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case toolsLoadedMsg:
		m.tools = msg.tools
		// Sort the incoming entries (loadToolsCmd already sorts, but
		// tests may pass unsorted entries directly to updateTools).
		sort.Slice(m.tools, func(i, j int) bool {
			return m.tools[i].name < m.tools[j].name
		})
		// Clamp toolIdx to [0, len-1]; 0 when empty.
		if len(m.tools) == 0 {
			m.toolIdx = 0
		} else if m.toolIdx >= len(m.tools) {
			m.toolIdx = len(m.tools) - 1
		}
		m = m.updateToolDetailPanel()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {

		case "up", "k":
			if m.toolIdx > 0 {
				m.toolIdx--
				m = m.updateToolDetailPanel()
			}
			return m, nil

		case "down", "j":
			if len(m.tools) > 0 && m.toolIdx < len(m.tools)-1 {
				m.toolIdx++
				m = m.updateToolDetailPanel()
			}
			return m, nil

		case "esc":
			m.screen = m.prevScreen
			m.footer = footerHints{screen: m.prevScreen}
			// FIX-3: recompute viewport size when returning to chat.
			// Tools has no input bar; chat reserves 4 rows — without a recompute the
			// viewport retains the tools-screen height and renderChat overflows.
			// enterChatViewport() is called AFTER m.screen is set so chatViewportSize
			// reads the correct geometry. Only applies when landing on screenChat;
			// welcome has the same input-bar geometry as chat so no correction needed.
			if m.screen == screenChat {
				m = m.enterChatViewport()
			}
			return m, nil
		}
	}

	return m, nil
}

// renderTools renders the center column for the tools screen.
// Each row shows: name + Risk badge + Category badge, selected row highlighted.
// A dim footer note clarifies that MCP servers are not loaded in the embedded TUI.
// All width math uses ansi.StringWidth/ansi.Truncate (no len/byte-slice).
func renderTools(m Model, width, height int) string {
	inner := width
	if inner < 8 {
		inner = 8
	}

	if len(m.tools) == 0 {
		msg := m.styles.dimLabel.Render("no tools — agent not connected")
		return centerText(msg, width)
	}

	var rows []string

	header := m.styles.panelHeader("built-in tools")
	rows = append(rows, ansi.Truncate(header, inner, "…"))
	rows = append(rows, "")

	for i, entry := range m.tools {
		risk := string(entry.meta.Risk)
		cat := string(entry.meta.Category)

		line := entry.name + "  " + risk + "  " + cat

		if i == m.toolIdx {
			line = m.styles.selected.Render(ansi.Truncate(line, inner, "…"))
		} else {
			line = m.styles.dimLabel.Render(ansi.Truncate(line, inner, "…"))
		}
		rows = append(rows, line)
	}

	// MCP deferral note — honest, dim.
	rows = append(rows, "")
	mcpNote := m.styles.dimLabel.Render("(MCP servers not loaded in embedded TUI)")
	rows = append(rows, ansi.Truncate(mcpNote, inner, "…"))

	// Pad to height to avoid layout gaps.
	for len(rows) < height {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}
