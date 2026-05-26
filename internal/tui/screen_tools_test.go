package tui

// screen_tools_test.go — STRICT TDD tests for PR4a: tools screen.
//
// Test order follows the spec:
//  1. updateTools: up/down move toolIdx; clamp at bounds; esc → prevScreen
//  2. ctrl+t in chat (handleChatKey) → screenTools + non-nil cmd
//  3. ctrl+t in welcome (updateWelcome) → screenTools + non-nil cmd
//  4. toolsLoadedMsg → m.tools populated (sorted), toolIdx clamped
//  5. toolDetailPanel: setTool + Render contains name + risk + category; empty → ""
//  6. renderTools: empty → "no tools"; with tools → name + risk/category visible
//  7. After navigation, the tool-detail rail reflects the selected tool

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// fakeToolEntries returns a deterministic slice of toolEntry values suitable
// for use in tests. They do NOT require a real agent.
func fakeToolEntries() []toolEntry {
	return []toolEntry{
		{
			name: "bash",
			desc: "Execute bash commands",
			meta: tool.ToolMeta{Risk: tool.RiskSideEffects, Category: tool.CatShell, Permission: tool.PermUser, Source: tool.SourceBuiltin},
		},
		{
			name: "read_file",
			desc: "Read a file from disk",
			meta: tool.ToolMeta{Risk: tool.RiskReadOnly, Category: tool.CatFile, Permission: tool.PermNone, Source: tool.SourceBuiltin},
		},
		{
			name: "write_file",
			desc: "Write content to a file",
			meta: tool.ToolMeta{Risk: tool.RiskDestructive, Category: tool.CatFile, Permission: tool.PermAdmin, Source: tool.SourceBuiltin},
		},
	}
}

// toolsModel returns a Model set up for the tools screen with fake entries.
func toolsModel(entries []toolEntry) Model {
	m := newTestModel()
	m.screen = screenTools
	m.tools = entries
	m.toolIdx = 0
	m.prevScreen = screenWelcome
	return m
}

// ---------------------------------------------------------------------------
// 1. updateTools — key routing
// ---------------------------------------------------------------------------

func TestUpdateTools_Down_IncreasesIndex(t *testing.T) {
	m := toolsModel(fakeToolEntries())
	m.toolIdx = 0

	next, _ := m.updateTools(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	rm := next.(Model)

	if rm.toolIdx != 1 {
		t.Errorf("toolIdx after 'j' = %d, want 1", rm.toolIdx)
	}
}

func TestUpdateTools_Down_Arrow_IncreasesIndex(t *testing.T) {
	m := toolsModel(fakeToolEntries())
	m.toolIdx = 0

	next, _ := m.updateTools(tea.KeyMsg{Type: tea.KeyDown})
	rm := next.(Model)

	if rm.toolIdx != 1 {
		t.Errorf("toolIdx after KeyDown = %d, want 1", rm.toolIdx)
	}
}

func TestUpdateTools_Down_ClampsAtEnd(t *testing.T) {
	entries := fakeToolEntries()
	m := toolsModel(entries)
	m.toolIdx = len(entries) - 1

	next, _ := m.updateTools(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	rm := next.(Model)

	if rm.toolIdx != len(entries)-1 {
		t.Errorf("toolIdx past end = %d, want %d", rm.toolIdx, len(entries)-1)
	}
}

func TestUpdateTools_Up_DecreasesIndex(t *testing.T) {
	m := toolsModel(fakeToolEntries())
	m.toolIdx = 2

	next, _ := m.updateTools(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	rm := next.(Model)

	if rm.toolIdx != 1 {
		t.Errorf("toolIdx after 'k' = %d, want 1", rm.toolIdx)
	}
}

func TestUpdateTools_Up_Arrow_DecreasesIndex(t *testing.T) {
	m := toolsModel(fakeToolEntries())
	m.toolIdx = 2

	next, _ := m.updateTools(tea.KeyMsg{Type: tea.KeyUp})
	rm := next.(Model)

	if rm.toolIdx != 1 {
		t.Errorf("toolIdx after KeyUp = %d, want 1", rm.toolIdx)
	}
}

func TestUpdateTools_Up_ClampsAtZero(t *testing.T) {
	m := toolsModel(fakeToolEntries())
	m.toolIdx = 0

	next, _ := m.updateTools(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	rm := next.(Model)

	if rm.toolIdx != 0 {
		t.Errorf("toolIdx below 0 = %d, want 0", rm.toolIdx)
	}
}

func TestUpdateTools_Esc_GoesToPrevScreen(t *testing.T) {
	m := toolsModel(fakeToolEntries())
	m.prevScreen = screenChat

	next, _ := m.updateTools(tea.KeyMsg{Type: tea.KeyEscape})
	rm := next.(Model)

	if rm.screen != screenChat {
		t.Errorf("screen after esc = %v, want screenChat (prevScreen)", rm.screen)
	}
}

func TestUpdateTools_Esc_UpdatesFooter(t *testing.T) {
	m := toolsModel(fakeToolEntries())
	m.prevScreen = screenChat
	m.footer = footerHints{screen: screenTools}

	next, _ := m.updateTools(tea.KeyMsg{Type: tea.KeyEscape})
	rm := next.(Model)

	if rm.footer.screen != screenChat {
		t.Errorf("footer.screen after esc = %v, want screenChat", rm.footer.screen)
	}
}

// ---------------------------------------------------------------------------
// 2. ctrl+t in chat → screenTools + non-nil cmd
// ---------------------------------------------------------------------------

func TestHandleChatKey_CtrlT_NavigatesToTools(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	rm := next.(Model)

	if rm.screen != screenTools {
		t.Errorf("screen after ctrl+t in chat = %v, want screenTools", rm.screen)
	}
	if rm.prevScreen != screenChat {
		t.Errorf("prevScreen = %v, want screenChat", rm.prevScreen)
	}
	if cmd == nil {
		t.Error("ctrl+t in chat: expected a non-nil cmd (loadToolsCmd), got nil")
	}
}

func TestHandleChatKey_CtrlT_UpdatesFooter(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor
	m.footer = footerHints{screen: screenChat}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	rm := next.(Model)

	if rm.footer.screen != screenTools {
		t.Errorf("footer.screen after ctrl+t from chat = %v, want screenTools", rm.footer.screen)
	}
}

// ---------------------------------------------------------------------------
// 3. ctrl+t in welcome → screenTools + non-nil cmd
// ---------------------------------------------------------------------------

func TestUpdateWelcome_CtrlT_NavigatesToTools(t *testing.T) {
	m := newTestModel()
	m.screen = screenWelcome

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	rm := next.(Model)

	if rm.screen != screenTools {
		t.Errorf("screen after ctrl+t in welcome = %v, want screenTools", rm.screen)
	}
	if rm.prevScreen != screenWelcome {
		t.Errorf("prevScreen = %v, want screenWelcome", rm.prevScreen)
	}
	if cmd == nil {
		t.Error("ctrl+t in welcome: expected a non-nil cmd (loadToolsCmd), got nil")
	}
}

func TestUpdateWelcome_CtrlT_UpdatesFooter(t *testing.T) {
	m := newTestModel()
	m.screen = screenWelcome
	m.footer = footerHints{screen: screenWelcome}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	rm := next.(Model)

	if rm.footer.screen != screenTools {
		t.Errorf("footer.screen after ctrl+t from welcome = %v, want screenTools", rm.footer.screen)
	}
}

// ---------------------------------------------------------------------------
// 4. toolsLoadedMsg → m.tools populated (sorted), toolIdx clamped
// ---------------------------------------------------------------------------

func TestUpdateTools_ToolsLoadedMsg_Populates(t *testing.T) {
	m := toolsModel(nil)
	entries := fakeToolEntries()

	next, _ := m.updateTools(toolsLoadedMsg{tools: entries})
	rm := next.(Model)

	if len(rm.tools) != len(entries) {
		t.Fatalf("tools len = %d, want %d", len(rm.tools), len(entries))
	}
}

func TestUpdateTools_ToolsLoadedMsg_Sorted(t *testing.T) {
	// Load in non-alphabetical order; expect sorted output.
	unsorted := []toolEntry{
		{name: "zebra", desc: "z", meta: tool.ToolMeta{Risk: tool.RiskReadOnly, Category: tool.CatMeta, Source: tool.SourceBuiltin}},
		{name: "alpha", desc: "a", meta: tool.ToolMeta{Risk: tool.RiskReadOnly, Category: tool.CatMeta, Source: tool.SourceBuiltin}},
		{name: "middle", desc: "m", meta: tool.ToolMeta{Risk: tool.RiskReadOnly, Category: tool.CatMeta, Source: tool.SourceBuiltin}},
	}

	m := toolsModel(nil)
	next, _ := m.updateTools(toolsLoadedMsg{tools: unsorted})
	rm := next.(Model)

	if len(rm.tools) != 3 {
		t.Fatalf("tools len = %d, want 3", len(rm.tools))
	}
	if rm.tools[0].name != "alpha" {
		t.Errorf("tools[0].name = %q, want 'alpha' (sorted)", rm.tools[0].name)
	}
	if rm.tools[1].name != "middle" {
		t.Errorf("tools[1].name = %q, want 'middle' (sorted)", rm.tools[1].name)
	}
	if rm.tools[2].name != "zebra" {
		t.Errorf("tools[2].name = %q, want 'zebra' (sorted)", rm.tools[2].name)
	}
}

func TestUpdateTools_ToolsLoadedMsg_ClampsIdx(t *testing.T) {
	m := toolsModel(fakeToolEntries())
	m.toolIdx = 99

	next, _ := m.updateTools(toolsLoadedMsg{tools: fakeToolEntries()[:1]})
	rm := next.(Model)

	if rm.toolIdx != 0 {
		t.Errorf("toolIdx = %d, want 0 (clamped to len-1=0)", rm.toolIdx)
	}
}

func TestUpdateTools_ToolsLoadedMsg_Empty(t *testing.T) {
	m := toolsModel(fakeToolEntries())
	m.toolIdx = 1

	next, _ := m.updateTools(toolsLoadedMsg{tools: nil})
	rm := next.(Model)

	if len(rm.tools) != 0 {
		t.Errorf("tools len = %d, want 0", len(rm.tools))
	}
	if rm.toolIdx != 0 {
		t.Errorf("toolIdx = %d, want 0", rm.toolIdx)
	}
}

// ---------------------------------------------------------------------------
// 5. toolDetailPanel: setTool → Render contains name + risk + category; empty → ""
// ---------------------------------------------------------------------------

func TestToolDetailPanel_ImplementsPanel(t *testing.T) {
	var _ Panel = newToolDetailPanel(newTuiStyles())
}

func TestToolDetailPanel_Empty_ReturnsEmpty(t *testing.T) {
	p := newToolDetailPanel(newTuiStyles())
	got := p.Render(40, 20)
	if got != "" {
		t.Errorf("toolDetailPanel.Render with no data: got %q, want empty string", got)
	}
}

func TestToolDetailPanel_WithTool_ContainsNameRiskCategory(t *testing.T) {
	p := newToolDetailPanel(newTuiStyles())
	p.setTool("read_file", "Read a file from disk",
		tool.ToolMeta{Risk: tool.RiskReadOnly, Category: tool.CatFile, Permission: tool.PermNone, Source: tool.SourceBuiltin})

	got := p.Render(40, 20)
	if got == "" {
		t.Fatal("toolDetailPanel.Render: got empty, want content")
	}
	if !strings.Contains(got, "read_file") {
		t.Errorf("toolDetailPanel.Render: expected 'read_file' in output:\n%s", got)
	}
	if !strings.Contains(got, string(tool.RiskReadOnly)) {
		t.Errorf("toolDetailPanel.Render: expected risk %q in output:\n%s", tool.RiskReadOnly, got)
	}
	if !strings.Contains(got, string(tool.CatFile)) {
		t.Errorf("toolDetailPanel.Render: expected category %q in output:\n%s", tool.CatFile, got)
	}
}

func TestToolDetailPanel_RenderTable(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		toolDesc    string
		meta        tool.ToolMeta
		wantContain []string
		wantEmpty   bool
	}{
		{
			name:      "no tool set returns empty",
			wantEmpty: true,
		},
		{
			name:     "bash tool shows all fields",
			toolName: "bash",
			toolDesc: "Execute bash commands",
			meta:     tool.ToolMeta{Risk: tool.RiskSideEffects, Category: tool.CatShell, Permission: tool.PermUser, Source: tool.SourceBuiltin},
			wantContain: []string{
				"bash",
				string(tool.RiskSideEffects),
				string(tool.CatShell),
				string(tool.SourceBuiltin),
			},
		},
		{
			name:     "write_file shows destructive risk",
			toolName: "write_file",
			toolDesc: "Write content to a file",
			meta:     tool.ToolMeta{Risk: tool.RiskDestructive, Category: tool.CatFile, Permission: tool.PermAdmin, Source: tool.SourceBuiltin},
			wantContain: []string{
				"write_file",
				string(tool.RiskDestructive),
				string(tool.CatFile),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newToolDetailPanel(newTuiStyles())
			if tt.toolName != "" {
				p.setTool(tt.toolName, tt.toolDesc, tt.meta)
			}
			got := p.Render(40, 20)
			if tt.wantEmpty {
				if got != "" {
					t.Errorf("Render: got %q, want empty string", got)
				}
				return
			}
			if got == "" {
				t.Fatal("Render: got empty string, want non-empty")
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("Render: expected %q in output:\n%s", want, got)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. renderTools: empty → "no tools"; with tools → name + risk/category visible
// ---------------------------------------------------------------------------

func TestRenderTools_EmptyList_ShowsNoTools(t *testing.T) {
	m := newTestModel()
	m.screen = screenTools
	m.width = 80
	m.height = 24

	got := renderTools(m, 80, 20)

	if !strings.Contains(got, "no tools") {
		t.Errorf("renderTools with empty list: expected 'no tools' in output:\n%s", got)
	}
}

func TestRenderTools_WithTools_ShowsNamesAndMeta(t *testing.T) {
	entries := fakeToolEntries()
	m := newTestModel()
	m.screen = screenTools
	m.tools = entries
	m.toolIdx = 0
	m.width = 80
	m.height = 24

	got := renderTools(m, 80, 20)

	if got == "" {
		t.Fatal("renderTools: got empty, want content")
	}
	// First entry is "bash"
	if !strings.Contains(got, "bash") {
		t.Errorf("renderTools: expected 'bash' in output:\n%s", got)
	}
	// risk and category for bash
	if !strings.Contains(got, string(tool.RiskSideEffects)) {
		t.Errorf("renderTools: expected risk %q in output:\n%s", tool.RiskSideEffects, got)
	}
	if !strings.Contains(got, string(tool.CatShell)) {
		t.Errorf("renderTools: expected category %q in output:\n%s", tool.CatShell, got)
	}
}

func TestRenderTools_SelectedRowHighlighted(t *testing.T) {
	entries := fakeToolEntries()
	m := newTestModel()
	m.screen = screenTools
	m.tools = entries
	m.toolIdx = 1 // "read_file" is second (index 1) after sorting: bash, read_file, write_file
	m.width = 80
	m.height = 24

	got := renderTools(m, 80, 20)
	// The selected tool name must appear; we can't assert ANSI styles (no inline hex)
	// but the name must be in the output.
	if !strings.Contains(got, "read_file") {
		t.Errorf("renderTools: expected 'read_file' (selected) in output:\n%s", got)
	}
}

// TestRenderTools_MCPNote verifies the MCP deferral note is present.
func TestRenderTools_MCPNote(t *testing.T) {
	m := newTestModel()
	m.screen = screenTools
	m.tools = fakeToolEntries()
	m.width = 80
	m.height = 24

	got := renderTools(m, 80, 20)
	// Should mention MCP is not loaded in the embedded TUI.
	if !strings.Contains(got, "MCP") {
		t.Errorf("renderTools: expected MCP deferral note in output:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// 7. After navigation, the tool-detail rail reflects the selected tool
// ---------------------------------------------------------------------------

// TestUpdateTools_Navigation_UpdatesToolDetailPanel verifies that after navigating
// down, the toolDetailPanel in the rail reflects the newly selected tool.
func TestUpdateTools_Navigation_UpdatesToolDetailPanel(t *testing.T) {
	entries := fakeToolEntries()
	m := toolsModel(entries)
	m.toolIdx = 0
	// Ensure toolDetailPanel is in the rail.
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		panels[panelToolDetail] = newToolDetailPanel(m.styles)
	})

	// Navigate down to index 1.
	next, _ := m.updateTools(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	rm := next.(Model)

	p, ok := rm.rail.panels[panelToolDetail].(*toolDetailPanel)
	if !ok {
		t.Fatal("rail.panels[panelToolDetail] is not a *toolDetailPanel after navigation")
	}
	got := p.Render(40, 20)
	// After down, index 1 = "read_file" (entries are pre-sorted in fakeToolEntries).
	if !strings.Contains(got, entries[1].name) {
		t.Errorf("toolDetailPanel after nav down: expected %q in render:\n%s", entries[1].name, got)
	}
}

// TestUpdateTools_ToolsLoadedMsg_UpdatesRailPanel verifies that toolsLoadedMsg
// sets the toolDetailPanel to the first (index 0) tool in the list.
func TestUpdateTools_ToolsLoadedMsg_UpdatesRailPanel(t *testing.T) {
	m := toolsModel(nil)
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		panels[panelToolDetail] = newToolDetailPanel(m.styles)
	})
	entries := fakeToolEntries()

	next, _ := m.updateTools(toolsLoadedMsg{tools: entries})
	rm := next.(Model)

	p, ok := rm.rail.panels[panelToolDetail].(*toolDetailPanel)
	if !ok {
		t.Fatal("rail.panels[panelToolDetail] is not a *toolDetailPanel after toolsLoadedMsg")
	}
	got := p.Render(40, 20)
	// toolsLoadedMsg sorts entries: bash, read_file, write_file → index 0 = "bash"
	if !strings.Contains(got, "bash") {
		t.Errorf("toolDetailPanel after toolsLoadedMsg: expected 'bash' (first sorted) in render:\n%s", got)
	}
}
