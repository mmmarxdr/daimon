package main

import (
	"log/slog"

	"daimon/internal/agent"
	"daimon/internal/tool"
)

// wireTodo attaches the todo tool set to the agent by building a TodoToolDeps
// backed by the agent's per-turn registry and registering the three tools into
// toolsRegistry.
//
// First-writer-wins: any tool name that already exists in the registry is
// skipped (mirrors the wireSmartMemory guard at memory_wiring.go:68-70).
//
// Call this after agent.New, alongside wireSmartMemory, in BOTH startup paths
// (cmd/daimon/main.go and cmd/daimon/web_cmd.go).
//
// Event emission uses the bus already wired into ag.bus; no bus parameter
// is needed here.
func wireTodo(ag *agent.Agent, toolsRegistry map[string]tool.Tool) {
	deps := ag.TodoToolDeps()
	todoTools := tool.BuildTodoTools(deps)
	for name, t := range todoTools {
		if _, exists := toolsRegistry[name]; !exists {
			toolsRegistry[name] = t
			slog.Debug("todo tool registered", "tool", name)
		}
	}
}
