package tui

// panels.go — panelID type, rail panel constants, and the panelsFor() dispatch
// table. This is the PANEL CONTRACT (AD-6): the authoritative mapping from
// screenState to the ordered list of right-rail panels.
//
// PR1 declares ALL panel IDs so later PRs only implement the panel structs
// themselves. The panelsFor() switch is the single source of truth — never
// inline panel lists in screen handlers.

// panelID is a string-typed identifier for a right-rail panel.
// String type (not int) so test output and logs are self-describing.
type panelID string

// Rail panel identifiers — authoritative names, transcribed from components.md §Matrix.
const (
	panelEnvironment   panelID = "environment"    // welcome
	panelResumeList    panelID = "resume-list"    // welcome, sessions
	panelTodolist      panelID = "todolist"       // chat
	panelContextMeter  panelID = "context-meter"  // chat
	panelTelemetry     panelID = "telemetry"      // chat, diff, error
	panelMemoryPeek    panelID = "memory-peek"    // chat (PR-c: memory entries)
	panelHunksNav      panelID = "hunks-nav"      // diff
	panelRationale     panelID = "rationale"      // diff
	panelImpact        panelID = "impact"         // diff
	panelToolDetail    panelID = "tool-detail"    // tools
	panelModelPicker   panelID = "model-picker"   // sessions
	panelActivePolicy  panelID = "active-policy"  // error
	panelRecentDenials panelID = "recent-denials" // error
)

// panelsFor returns the ordered list of right-rail panel IDs for a given screen.
// This is the PANEL CONTRACT table (AD-6 / components.md §Matrix).
// Do NOT add or reorder entries without updating the matrix and const_test.go.
func panelsFor(screen screenState) []panelID {
	switch screen {
	case screenWelcome:
		return []panelID{panelEnvironment, panelResumeList}
	case screenChat:
		return []panelID{panelTodolist, panelContextMeter, panelTelemetry, panelMemoryPeek}
	case screenDiff:
		return []panelID{panelHunksNav, panelRationale, panelImpact, panelTelemetry}
	case screenSlash:
		// Slash is an overlay over dimmed chat; no rail panels of its own.
		return []panelID{}
	case screenTools:
		return []panelID{panelToolDetail}
	case screenSessions:
		return []panelID{panelResumeList, panelModelPicker}
	case screenError:
		return []panelID{panelTelemetry, panelActivePolicy, panelRecentDenials}
	default:
		return []panelID{}
	}
}
