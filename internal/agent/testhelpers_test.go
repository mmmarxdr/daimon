package agent

// testhelpers_test.go — shared test helpers for the agent package tests.
// These are package-internal (not exported) and only compiled during testing.

// defaultModeForTests returns a ModeDefinition that matches "build" mode:
// no extra system prompt injection, nil allowlist (all tools pass).
// Use this helper in tests that call buildSystemPrompt or buildToolDefs but
// don't care about mode behaviour — it keeps tests stable when the function
// signatures gain a mode parameter.
func defaultModeForTests() ModeDefinition {
	return ModeDefinition{
		Name:          "build",
		SystemPrompt:  "",
		ToolAllowlist: nil,
	}
}
