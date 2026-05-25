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

// TestModel_InitialScreen_IsWelcome verifies that a freshly initialized Model
// starts on the welcome screen (screenWelcome).
func TestModel_InitialScreen_IsWelcome(t *testing.T) {
	m := newTestModel()
	// Init returns a cmd; we don't need to run it for this test.
	_ = m.Init()
	if m.screen != screenWelcome {
		t.Errorf("initial screen = %v, want screenWelcome", m.screen)
	}
}
