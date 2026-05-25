package tui

import (
	"testing"
)

// TestPanelsFor_ContractMatrix verifies the PANEL CONTRACT (AD-6) maps
// each screen to the exact rail panel set. This is the authoritative
// contract check — if this fails, the matrix is broken.
func TestPanelsFor_ContractMatrix(t *testing.T) {
	tests := []struct {
		screen screenState
		want   []panelID
	}{
		{
			screenWelcome,
			[]panelID{panelEnvironment, panelResumeList},
		},
		{
			screenChat,
			[]panelID{panelTodolist, panelContextMeter, panelTelemetry},
		},
		{
			screenDiff,
			[]panelID{panelHunksNav, panelRationale, panelImpact, panelTelemetry},
		},
		{
			screenSlash,
			[]panelID{}, // slash is an overlay over dimmed chat; no rail panels
		},
		{
			screenTools,
			[]panelID{panelToolDetail},
		},
		{
			screenSessions,
			[]panelID{panelResumeList, panelModelPicker},
		},
		{
			screenError,
			[]panelID{panelTelemetry, panelActivePolicy, panelRecentDenials},
		},
	}

	for _, tt := range tests {
		t.Run(tt.screen.String(), func(t *testing.T) {
			got := panelsFor(tt.screen)
			if len(got) != len(tt.want) {
				t.Errorf("panelsFor(%v): got %d panels %v, want %d panels %v",
					tt.screen, len(got), got, len(tt.want), tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("panelsFor(%v)[%d]: got %q, want %q",
						tt.screen, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestModel_InitialScreen_IsWelcome verifies that a Model built via
// newTestModel (the constructor/struct-literal path) starts on screenWelcome.
//
// NOTE: Init() uses a value receiver and intentionally does not set the screen;
// the screen is set in the struct literal. This test verifies the constructor
// contract, NOT the post-Init state (which would be a dead-mutation check).
func TestModel_InitialScreen_IsWelcome(t *testing.T) {
	m := newTestModel()
	if m.screen != screenWelcome {
		t.Errorf("newTestModel().screen = %v, want screenWelcome", m.screen)
	}
}

// TestModel_Init_ReturnsNilCmd verifies that Init() returns nil (no startup Cmd).
// It also confirms that calling Init() does not accidentally change the screen
// on the caller's copy (value receiver — mutation is on the copy, not the caller).
func TestModel_Init_ReturnsNilCmd(t *testing.T) {
	m := newTestModel()
	cmd := m.Init()
	if cmd != nil {
		t.Errorf("Init() returned non-nil Cmd, want nil")
	}
	// The caller's copy must be unchanged after Init.
	if m.screen != screenWelcome {
		t.Errorf("after Init(), caller screen = %v, want screenWelcome (value-receiver must not mutate caller)", m.screen)
	}
}
