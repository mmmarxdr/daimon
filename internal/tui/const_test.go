package tui

import "testing"

// TestFocusRegion_EnumContract verifies the focusRegion enum values introduced
// in PR2 match the AD-3 frozen contract. NEVER renumber.
func TestFocusRegion_EnumContract(t *testing.T) {
	tests := []struct {
		name  string
		value focusRegion
		want  focusRegion
	}{
		{"focusNone is 0", focusNone, 0},
		{"focusEditor is 1", focusEditor, 1},
		{"focusMain is 2", focusMain, 2},
		{"focusRail is 3", focusRail, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("got %d, want %d", tt.value, tt.want)
			}
		})
	}
}

// TestScreenState_EnumContract verifies the exact iota order and count
// of screenState values are FROZEN (AD-3 forward-compat contract).
// Later PRs MUST NOT renumber or add new values without updating this test.
func TestScreenState_EnumContract(t *testing.T) {
	tests := []struct {
		name  string
		value screenState
		want  screenState
	}{
		{"screenWelcome is 0", screenWelcome, 0},
		{"screenChat is 1", screenChat, 1},
		{"screenDiff is 2", screenDiff, 2},
		{"screenSlash is 3", screenSlash, 3},
		{"screenTools is 4", screenTools, 4},
		{"screenSessions is 5", screenSessions, 5},
		{"screenError is 6", screenError, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.want {
				t.Errorf("got %d, want %d", tt.value, tt.want)
			}
		})
	}
}
