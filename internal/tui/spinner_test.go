package tui

// spinner_test.go — WU-d RED: snapshot isolation + batch-advance + self-stop + dedup.
//
// Design references:
//   - design.md §D.3: thread.own() copy-on-write helper
//   - design.md §D.5: batch handler (own-once, k in-place writes)
//   - design.md §D.6: aliasing-safety proof (COW property)
//   - design.md §D.7: no ownedGen / global counter

import (
	"testing"

	"daimon/internal/notify"
)

// ---------------------------------------------------------------------------
// 3.1 TestSpinner_SnapshotIsolation
//
// Design §D.6 property: after a spinnerTickMsg is handled, a snapshot of the
// model taken BEFORE the tick must not observe the new frame. The spinnerFrame
// in the snapshot is frozen at its pre-tick value.
//
// RED: fails while the old per-ToolLine Tick() path is live, because the per-line
// handler mutates items[idx] through a freshly-copied slice BUT the items stored
// in the slice are *ToolLine pointers — a pointer in both old and new items
// slices points at the same ToolLine struct. After AdvanceSpinner() on the COPY,
// the old pointer in the snapshot still dereferences the same (now mutated)
// struct. This test catches that pointer-aliasing hazard.
// ---------------------------------------------------------------------------
func TestSpinner_SnapshotIsolation(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.width = 80
	m.height = 24

	// Insert one running ToolLine.
	tl := &ToolLine{callID: "c1", name: "bash", state: toolRunning, styles: m.styles}
	m.thread.append(tl)
	m = m.refreshThreadViewport()

	// Record the spinner frame BEFORE the tick.
	preTick := m.thread.findToolLine("c1")
	if preTick == nil {
		t.Fatal("pre-condition: ToolLine c1 not found")
	}
	frameBefore := preTick.spinnerFrame

	// Send ONE spinnerTickMsg (batch, no callID) to get model B.
	m2, _ := m.Update(spinnerTickMsg{})
	modelB := m2.(Model)

	// Model B must have advanced the frame.
	tlB := modelB.thread.findToolLine("c1")
	if tlB == nil {
		t.Fatal("ToolLine c1 missing from model B after spinnerTickMsg")
	}
	if tlB.spinnerFrame == frameBefore {
		t.Errorf("model B frame unchanged after tick: frame=%d", tlB.spinnerFrame)
	}

	// The original model A's snapshot (m) must show the PRE-TICK frame.
	// If items share a pointer to the same ToolLine struct, m.thread.findToolLine
	// would see the mutated value — that is the aliasing hazard we are preventing.
	tlA := m.thread.findToolLine("c1")
	if tlA == nil {
		t.Fatal("ToolLine c1 missing from snapshot model A")
	}
	if tlA.spinnerFrame != frameBefore {
		t.Errorf("COW violation: snapshot model A frame changed from %d to %d after tick on model B",
			frameBefore, tlA.spinnerFrame)
	}
}

// ---------------------------------------------------------------------------
// 3.1 TestSpinner_BatchAdvance_SingleCopy
//
// Design §D.5: one spinnerTickMsg{} (no callID) advances ALL k running ToolLines
// in a single own() copy. After the tick: all k lines advanced, AND the prior
// snapshot still sees the un-advanced frames (COW property).
// ---------------------------------------------------------------------------
func TestSpinner_BatchAdvance_SingleCopy(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.width = 80
	m.height = 24

	// Insert 3 running ToolLines.
	tls := []*ToolLine{
		{callID: "c1", name: "bash", state: toolRunning, styles: m.styles},
		{callID: "c2", name: "read", state: toolRunning, styles: m.styles},
		{callID: "c3", name: "grep", state: toolRunning, styles: m.styles},
	}
	for _, tl := range tls {
		m.thread.append(tl)
	}
	m = m.refreshThreadViewport()

	// Record pre-tick frames.
	preTick := map[string]int{}
	for _, tl := range tls {
		found := m.thread.findToolLine(tl.callID)
		if found == nil {
			t.Fatalf("pre-condition: ToolLine %s not found", tl.callID)
		}
		preTick[tl.callID] = found.spinnerFrame
	}

	// Send ONE spinnerTickMsg{} — must advance ALL three.
	m2, cmd := m.Update(spinnerTickMsg{})
	modelB := m2.(Model)

	// (a) All k lines advanced in model B.
	for _, tl := range tls {
		tlB := modelB.thread.findToolLine(tl.callID)
		if tlB == nil {
			t.Fatalf("ToolLine %s missing from model B", tl.callID)
		}
		want := (preTick[tl.callID] + 1) % len(brailleSpinner)
		if tlB.spinnerFrame != want {
			t.Errorf("ToolLine %s: frame=%d want=%d", tl.callID, tlB.spinnerFrame, want)
		}
	}

	// (b) Prior snapshot (m) still shows pre-tick frames (COW property).
	for _, tl := range tls {
		tlA := m.thread.findToolLine(tl.callID)
		if tlA == nil {
			t.Fatalf("ToolLine %s missing from snapshot model A", tl.callID)
		}
		if tlA.spinnerFrame != preTick[tl.callID] {
			t.Errorf("COW violation: snapshot ToolLine %s frame changed from %d to %d",
				tl.callID, preTick[tl.callID], tlA.spinnerFrame)
		}
	}

	// (c) Returned cmd must re-arm the ticker (running tools still present).
	if cmd == nil {
		t.Error("spinnerTickMsg with running tools must return a non-nil re-arm cmd")
	}
}

// ---------------------------------------------------------------------------
// 3.1 TestSpinner_TickerSelfStops
//
// Design §D.5: when runningToolIdxs() is empty, the spinnerTickMsg handler
// returns (m, nil) — no re-arm. Self-stop when no tools are running.
// ---------------------------------------------------------------------------
func TestSpinner_TickerSelfStops(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.width = 80
	m.height = 24

	// Insert one DONE tool (not running).
	tl := &ToolLine{callID: "c1", name: "bash", state: toolDone, styles: m.styles}
	m.thread.append(tl)
	m = m.refreshThreadViewport()

	// Send spinnerTickMsg{} with no running tools.
	_, cmd := m.Update(spinnerTickMsg{})

	// Cmd must be nil — self-stop, no re-arm.
	if cmd != nil {
		t.Error("spinnerTickMsg with zero running tools must return nil cmd (self-stop)")
	}
}

// ---------------------------------------------------------------------------
// 3.1 TestSpinner_ArmingDedupe
//
// Design §D.7 ticker arming: arm on the FIRST EventToolStart only when
// !m.spinnerActive; the second EventToolStart must NOT arm a second ticker.
// After first arm: m.spinnerActive == true. Second arm: still true, no new cmd.
// ---------------------------------------------------------------------------
func TestSpinner_ArmingDedupe(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.width = 80
	m.height = 24

	// Wire a no-op events channel so pumpEvents doesn't panic.
	evCh := make(chan interface{ isTeaMsg() }, 1)
	_ = evCh

	// First EventToolStart — should arm the single ticker.
	ev1 := notify.Event{
		Type:       notify.EventToolStart,
		ToolCallID: "c1",
		ToolName:   "bash",
	}
	m2, _ := m.Update(busEventMsg{event: ev1})
	model1 := m2.(Model)

	if !model1.spinnerActive {
		t.Error("after first EventToolStart: m.spinnerActive must be true")
	}

	// Second EventToolStart — must NOT start a second ticker.
	ev2 := notify.Event{
		Type:       notify.EventToolStart,
		ToolCallID: "c2",
		ToolName:   "read",
	}
	m3, _ := model1.Update(busEventMsg{event: ev2})
	model2 := m3.(Model)

	if !model2.spinnerActive {
		t.Error("after second EventToolStart: m.spinnerActive must still be true")
	}
	// spinnerActive stays true; no way to directly count tickers, but the
	// state flag is the guard against stacking. The design §D.7 contract is:
	// arm if and only if !m.spinnerActive.
}
