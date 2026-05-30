package tui

// purity_test.go — TDD guard tests for View purity (WU-a + WU-b).
//
// These tests enforce the invariant: Model.View() is a deterministic, pure
// function of the receiver's fields alone. No clock reads, no live-agent
// access, no mutation, no IO.
//
// Test order follows the TDD cycle per task list 1.1–1.9:
//   1.1  RED/GREEN: viewport field present in newTestModel (harness unblock for WU-c)
//   1.2  RED: TestView_Deterministic — screenChat, fails until WU-a+WU-b green
//   1.3  RED: TestView_Deterministic_Sessions + TestView_Deterministic_Rail variants
//   1.4  RED: mode-cache unit tests (TestMode_CachedField, TestLayout_ReadsCachedMode,
//              TestMode_SlashCommandRefreshes)
//   1.7  RED: ago pre-compute unit tests (TestSessions_PrecomputedAgo)

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// failingModeAgent is a modeAgent stub that calls t.Fatal if CurrentMode is
// invoked. Used to prove that View/renderLayout does NOT call CurrentMode
// after WU-a — the render path must read m.mode instead.
type failingModeAgent struct {
	t    *testing.T
	mode string // field updated by SetModeImmediate
}

func (f *failingModeAgent) CurrentMode() string {
	if f.t != nil {
		f.t.Helper()
		f.t.Fatalf("CurrentMode() called during View/Render — live agent read detected; render must read m.mode instead")
	}
	return f.mode
}

func (f *failingModeAgent) SetModeImmediate(name string) { f.mode = name }
func (f *failingModeAgent) ReconcileMode(string)         {}

// simpleModeStub is a modeAgent that returns a fixed mode from CurrentMode()
// and records the latest value passed to SetModeImmediate.
type simpleModeStub struct{ mode string }

func newSimpleModeStub(mode string) modeAgent       { return &simpleModeStub{mode: mode} }
func (s *simpleModeStub) CurrentMode() string       { return s.mode }
func (s *simpleModeStub) SetModeImmediate(n string) { s.mode = n }
func (s *simpleModeStub) ReconcileMode(string)      {}

// overridingModeStub faithfully mirrors agentModeAdapter's optimistic-override
// semantics (used by the reconciliation regression tests): SetModeImmediate
// sets an override that shadows the base mode until ReconcileMode clears it.
type overridingModeStub struct {
	base     string // authoritative mode (what the "agent" would report)
	override string // optimistic Tab override; shadows base while non-empty
}

func (s *overridingModeStub) CurrentMode() string {
	if s.override != "" {
		return s.override
	}
	return s.base
}
func (s *overridingModeStub) SetModeImmediate(n string) { s.override = n }
func (s *overridingModeStub) ReconcileMode(confirmed string) {
	if s.override == confirmed {
		s.override = ""
	}
}

// ---------------------------------------------------------------------------
// Task 1.1 — viewport harness unblock
// ---------------------------------------------------------------------------

// TestNewTestModel_ViewportFieldExists verifies that newTestModel() initializes
// the viewport field to a valid viewport.Model so non-chat tests never panic
// when PR-2 adds viewport operations. The test will fail to compile until the
// viewport field is added to Model struct (RED → GREEN = add field + init).
func TestNewTestModel_ViewportFieldExists(t *testing.T) {
	m := newTestModel()
	// AtBottom() on a zero-height viewport must not panic.
	_ = m.viewport.AtBottom()
	// View() on an unsized, empty viewport must return "".
	if got := m.viewport.View(); got != "" {
		t.Errorf("viewport.View() on unsized model = %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// Task 1.2 — TestView_Deterministic: screenChat purity guard
// ---------------------------------------------------------------------------

// TestView_Deterministic populates a Model in screenChat and calls View() 50
// times, asserting byte-identical output. Fails while layout.go still calls
// m.modeAgent.CurrentMode() (which can vary) or while relativeTime() is called
// in any Render path.
func TestView_Deterministic(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	m.screen = screenChat

	// WU-a: set cached mode field. Attach a failingModeAgent so that ANY
	// CurrentMode() call from the render path fails the test — this makes the
	// determinism check a real regression guard against re-introducing a live
	// agent read in renderLayout/View.
	m.mode = "plan"
	m.modeAgent = &failingModeAgent{t: t, mode: "plan"}

	// Thread items exercising the render path.
	m.thread.append(&MsgUser{text: "hi", time: "10:00", styles: m.styles})
	tl := &ToolLine{callID: "c1", name: "bash", state: toolRunning, styles: m.styles}
	m.thread.append(tl)

	// WU-b: pre-computed ago strings (must NOT be recalculated in View).
	m.sessions = []store.Conversation{
		{ID: "abc12345", UpdatedAt: time.Now().Add(-2 * time.Minute)},
	}
	m.sessionsAgo = []string{"2m ago"}

	first := m.View()
	for i := 0; i < 50; i++ {
		got := m.View()
		if got != first {
			t.Fatalf("View() not deterministic at call %d\nfirst:\n%s\ngot:\n%s", i+1, first, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 1.3 — Session + rail determinism variants
// ---------------------------------------------------------------------------

// TestView_Deterministic_Sessions exercises the screenSessions render path.
// Fails while renderSessions calls relativeTime() live.
func TestView_Deterministic_Sessions(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	m.screen = screenSessions

	m.sessions = []store.Conversation{
		{
			ID:        "abc12345",
			Status:    "active",
			UpdatedAt: time.Now().Add(-5 * time.Minute),
			Metadata:  map[string]string{"title": "Test session"},
		},
	}
	// WU-b: pre-computed; renderSessions must read this, not call relativeTime.
	m.sessionsAgo = []string{"5m ago"}

	first := m.View()
	for i := 0; i < 50; i++ {
		got := m.View()
		if got != first {
			t.Fatalf("View() not deterministic (sessions) at call %d", i+1)
		}
	}
}

// TestView_Deterministic_Rail exercises the resumeListPanel.Render path.
// Fails while resumeListPanel.Render calls relativeTime() live.
func TestView_Deterministic_Rail(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	m.screen = screenWelcome

	convs := []store.Conversation{
		{
			ID:        "abc12345",
			Status:    "active",
			UpdatedAt: time.Now().Add(-3 * time.Minute),
			Metadata:  map[string]string{"title": "Rail session"},
		},
	}
	// Populate via Update so the resumeListPanel and sessionsAgo are both set.
	updated, _ := m.Update(sessionsLoadedMsg{convs: convs})
	m = updated.(Model)

	first := m.View()
	for i := 0; i < 50; i++ {
		got := m.View()
		if got != first {
			t.Fatalf("View() not deterministic (rail) at call %d", i+1)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 1.4 — WU-a mode-cache unit tests
// ---------------------------------------------------------------------------

// TestMode_CachedField verifies that cycleMode() writes m.mode and that a
// subsequent View() does NOT call CurrentMode() on the modeAgent.
//
// Two-phase approach:
//  1. Use simpleModeStub for cycleMode (cycleMode computes the next mode from
//     m.mode and calls only SetModeImmediate — the stub is here to satisfy the
//     interface, not to guard a CurrentMode() call).
//  2. Swap to failingModeAgent before View() — any CurrentMode call from the
//     render path calls t.Fatal, proving View reads m.mode, not the live agent.
func TestMode_CachedField(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	m.modeAgent = newSimpleModeStub("build") // safe stub for cycleMode
	m.mode = "build"

	// cycleMode reads modeAgent.CurrentMode() once (to compute next mode) then
	// sets m.mode = "plan". This call is Update-path behavior — expected.
	updated, _ := m.cycleMode()
	m = updated

	if m.mode != "plan" {
		t.Errorf("after cycleMode(), m.mode = %q, want %q", m.mode, "plan")
	}

	// Phase 2: swap to a failing stub. View() must NOT call CurrentMode().
	// If it does, failingModeAgent calls t.Fatal — the test fails clearly.
	m.modeAgent = &failingModeAgent{t: t, mode: "plan"}
	_ = m.View()
}

// TestLayout_ReadsCachedMode verifies renderLayout reads m.mode, not
// modeAgent.CurrentMode(). Sets m.mode = "review" and checks the rendered
// output contains "REVIEW". The failingModeAgent ensures no live call occurs.
func TestLayout_ReadsCachedMode(t *testing.T) {
	m := newTestModel()
	m.width, m.height = 80, 24
	m.modeAgent = &failingModeAgent{t: t, mode: "review"}
	m.mode = "review" // the cached field renderLayout must read

	out := m.View()
	const wantLabel = "REVIEW"
	if !containsCI(out, wantLabel) {
		t.Errorf("renderLayout output missing %q — mode pill not reading cached m.mode\noutput:\n%s", wantLabel, out)
	}
}

// TestMode_SlashCommandRefreshes verifies that a commandResultMsg with
// name "mode" refreshes m.mode from modeAgent.CurrentMode().
func TestMode_SlashCommandRefreshes(t *testing.T) {
	m := newTestModel()
	m.mode = "build"
	m.modeAgent = newSimpleModeStub("plan") // returns "plan" from CurrentMode

	updated, _ := m.Update(commandResultMsg{name: "mode", reply: "mode set"})
	m = updated.(Model)

	if m.mode != "plan" {
		t.Errorf("after commandResultMsg{name:mode}, m.mode = %q, want %q", m.mode, "plan")
	}
}

// TestCycleMode_UsesCachedModeNotStaleOverride is the regression guard for the
// "Tab after /mode" bug: cycleMode must compute the next mode from the cached
// m.mode (ground truth, refreshed by /mode), NOT from the adapter's optimistic
// override, which can be stale after a non-Tab mode change.
func TestCycleMode_UsesCachedModeNotStaleOverride(t *testing.T) {
	m := newTestModel()
	// Simulate state right after `/mode build`: m.mode is the ground truth, but
	// the adapter still carries a stale override "plan" from an earlier Tab.
	m.mode = "build"
	m.modeAgent = &overridingModeStub{base: "build", override: "plan"}

	nm, _ := m.cycleMode()

	// Correct: build → plan. Bug would read the stale override "plan" → review.
	if nm.mode != "plan" {
		t.Errorf("cycleMode after /mode: m.mode = %q, want %q (must use cached mode, not stale override)", nm.mode, "plan")
	}
}

// TestSwitchModeMsg_ReconcilesOverride verifies a landed Tab switch clears the
// adapter's optimistic override so CurrentMode() resumes delegating to truth.
func TestSwitchModeMsg_ReconcilesOverride(t *testing.T) {
	stub := &overridingModeStub{base: "plan", override: "plan"}
	m := newTestModel()
	m.modeAgent = stub
	m.mode = "plan"

	updated, _ := m.Update(switchModeMsg{mode: "plan"})
	nm := updated.(Model)

	if stub.override != "" {
		t.Errorf("switchModeMsg{plan} must clear a matching override, got override=%q", stub.override)
	}
	if nm.mode != "plan" {
		t.Errorf("switchModeMsg must refresh m.mode from ground truth, got %q want %q", nm.mode, "plan")
	}
}

// TestSwitchModeMsg_ReconcileRaceSafe verifies a stale (superseded) confirmation
// does NOT clear a newer override — rapid Tab must not flicker to ground truth.
func TestSwitchModeMsg_ReconcileRaceSafe(t *testing.T) {
	stub := &overridingModeStub{base: "build", override: "review"} // newer Tab → review pending
	m := newTestModel()
	m.modeAgent = stub
	m.mode = "review"

	updated, _ := m.Update(switchModeMsg{mode: "plan"}) // older confirmation for a superseded switch
	nm := updated.(Model)

	if stub.override != "review" {
		t.Errorf("stale switchModeMsg{plan} must NOT clear newer override; got override=%q want %q", stub.override, "review")
	}
	if nm.mode != "review" {
		t.Errorf("m.mode must reflect the pending newer override, got %q want %q", nm.mode, "review")
	}
}

// ---------------------------------------------------------------------------
// Task 1.7 — WU-b ago pre-compute unit tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Task 2.13 — TestView_Deterministic_Viewport: viewport determinism guard
// ---------------------------------------------------------------------------

// TestView_Deterministic_Viewport is the viewport-specific extension of the
// determinism guard. It builds a model with a running tool, drives a WindowSizeMsg
// (which sizes the viewport and pushes content via refreshThreadViewport), then
// calls View() 50 times and asserts byte-identical output.
//
// This guards against any future regression that would re-introduce a clock
// read or live-object access in the viewport render path.
func TestView_Deterministic_Viewport(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	// Attach a failing mode agent so ANY live CurrentMode() call from View
	// causes t.Fatal — same pattern as TestView_Deterministic.
	m.mode = "plan"
	m.modeAgent = &failingModeAgent{t: t, mode: "plan"}

	// Populate thread with a running tool (exercises the spinner render path).
	m.thread.append(&MsgUser{text: "hello viewport", time: "10:00", styles: m.styles})
	tl := &ToolLine{callID: "vp-c1", name: "bash", state: toolRunning, styles: m.styles}
	m.thread.append(tl)

	// Drive WindowSizeMsg to size the viewport and populate its content.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Viewport must have non-zero dimensions after the size message.
	if m.viewport.Width == 0 || m.viewport.Height == 0 {
		t.Fatalf("viewport dimensions not set after WindowSizeMsg: %dx%d",
			m.viewport.Width, m.viewport.Height)
	}

	// Call View() 50 times; every call must return the same bytes.
	first := m.View()
	if first == "" {
		t.Fatal("View() returned empty string after WindowSizeMsg — viewport content not pushed")
	}
	for i := 0; i < 50; i++ {
		got := m.View()
		if got != first {
			t.Fatalf("View() not deterministic (viewport) at call %d\nfirst:\n%s\ngot:\n%s",
				i+1, first, got)
		}
	}
}

// TestSessions_PrecomputedAgo verifies that sessionsLoadedMsg populates
// m.sessionsAgo in parallel with m.sessions (one entry per conversation).
func TestSessions_PrecomputedAgo(t *testing.T) {
	m := newTestModel()

	convs := []store.Conversation{
		{
			ID:        "c1",
			Status:    "active",
			UpdatedAt: time.Now().Add(-5 * time.Minute),
			Metadata:  map[string]string{"title": "Alpha"},
		},
		{
			ID:        "c2",
			Status:    "active",
			UpdatedAt: time.Now().Add(-30 * time.Minute),
			Metadata:  map[string]string{"title": "Beta"},
		},
	}

	updated, _ := m.Update(sessionsLoadedMsg{convs: convs})
	m = updated.(Model)

	if len(m.sessionsAgo) != len(m.sessions) {
		t.Fatalf("sessionsAgo len = %d, want %d (must be parallel to sessions)", len(m.sessionsAgo), len(m.sessions))
	}
	for i, ago := range m.sessionsAgo {
		if ago == "" {
			t.Errorf("sessionsAgo[%d] is empty, want non-empty ago string", i)
		}
	}
}
