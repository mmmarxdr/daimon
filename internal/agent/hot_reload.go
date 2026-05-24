package agent

import (
	"fmt"
	"log/slog"

	"daimon/internal/skill"
	"daimon/internal/tool"
)

// ReplaceExecutableSkills atomically replaces the agent's spawnable subagent
// definitions. Used by the /api/skills CRUD handler after every successful
// write. Removes ALL existing *SubagentSpawnTool entries from a.tools, then
// re-registers fresh ones from defs (with two-phase allowlist filtering).
//
// Lazy-initializes a.subMgr when defs is non-empty and no manager exists yet
// (covers the case where the agent was constructed without WithExecutableSkills
// but later gains its first executable skill via CRUD).
//
// Lock ordering: acquires only a.toolsMu (write). a.subMgr has its own internal
// mu.RWMutex which is acquired downstream by Spawn/Cancel — no overlap, no
// nested-lock scenarios.
//
// Idempotent: passing the same defs slice twice produces the same registry.
// Empty defs ([] or nil) leaves a.tools without any *SubagentSpawnTool entries
// but does NOT nil out a.subMgr (in-flight spawns continue to drain through it).
// (REQ-19; design §2.5)
func (a *Agent) ReplaceExecutableSkills(defs []skill.ExecutableSkillDef) {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()

	// Phase 1: remove existing spawn tools.
	for name, t := range a.tools {
		if _, ok := t.(*SubagentSpawnTool); ok {
			delete(a.tools, name)
		}
	}

	// Phase 2: lazy-init subMgr on first registration.
	if a.subMgr == nil && len(defs) > 0 {
		a.subMgr = NewSubagentManager(a.bus, a.store)
		a.subMgr.installBusSubscription()
		a.subMgr.newChildAgent = a.makeChildAgentFn()
	}

	// Phase 3: re-register from fresh defs (two-phase allowlist filter).
	for _, def := range defs {
		def.ToolsAllowlist = filterKnownTools(def.ToolsAllowlist, a.tools)
		if _, exists := a.tools[def.Name]; exists {
			slog.Warn("hot_reload: subagent tool name collides; subagent wins", "name", def.Name)
		}
		a.tools[def.Name] = &SubagentSpawnTool{def: def, manager: a.subMgr}
	}

	// Phase 4 (WU9, REQ-18): rotate skill-source commands atomically.
	// UnregisterAllBySource and registerSkillCommands each acquire commands.mu
	// independently. The two calls here are not inside a single transaction, but
	// because this function holds toolsMu (write), no concurrent ReplaceExecutableSkills
	// call can interleave. Concurrent Lookups (reads on commands.mu) see either the
	// fully-unregistered state or the fully-registered state — both are consistent
	// snapshots.
	// Guard: a.commands may be nil in minimal test agents that bypass New().
	if a.commands != nil {
		a.commands.UnregisterAllBySource(SourceSkill)
		registerSkillCommands(a, defs)
	}

	slog.Info("hot_reload: executable skills replaced",
		"count", len(defs),
		"total_tools", len(a.tools))
}

// RegisterMCPServer adds a freshly-connected MCP server's tools to the
// running agent without requiring a restart. Used by the dashboard after
// the user adds a server from the catalog: handler connects the server,
// pulls its tool list, then calls this to make those tools usable on the
// next agent turn.
//
// caller is the live MCPCaller for the server; it stays open for the
// lifetime of the daimon process or until UnregisterMCPServer is called.
// The agent stores it so a future Unregister can Close() it cleanly.
//
// Tool name collisions are resolved first-writer-wins (existing tool keeps
// its slot, new one is dropped with a WARN). This mirrors the boot-time
// merge policy in BuildMCPTools so behaviour is consistent.
func (a *Agent) RegisterMCPServer(serverName string, tools map[string]tool.Tool, caller interface{ Close() error }) {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()

	// If this server name was registered before (re-add after Unregister),
	// drop its old tools first.
	if old, ok := a.mcpToolNames[serverName]; ok {
		for _, n := range old {
			delete(a.tools, n)
		}
	}
	if oldCaller, ok := a.mcpClients[serverName]; ok {
		_ = oldCaller.Close()
	}

	registered := make([]string, 0, len(tools))
	for name, t := range tools {
		if _, exists := a.tools[name]; exists {
			slog.Warn("hot_reload: tool name collision, keeping existing",
				"tool", name, "incoming_server", serverName)
			continue
		}
		a.tools[name] = t
		registered = append(registered, name)
	}
	a.mcpToolNames[serverName] = registered
	a.mcpClients[serverName] = caller

	slog.Info("hot_reload: mcp server registered",
		"server", serverName,
		"tools_registered", len(registered),
		"total_tools", len(a.tools))
}

// UnregisterMCPServer removes all tools that came from a given MCP server
// and closes the underlying client. Returns an error only when the server
// is unknown (not previously registered via the hot-add path); a not-found
// is non-fatal for the caller (the server may have been added at boot,
// which we don't track here).
func (a *Agent) UnregisterMCPServer(serverName string) error {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()

	names, ok := a.mcpToolNames[serverName]
	if !ok {
		return fmt.Errorf("hot_reload: mcp server %q not registered via hot-add path", serverName)
	}
	for _, n := range names {
		delete(a.tools, n)
	}
	delete(a.mcpToolNames, serverName)

	if caller, ok := a.mcpClients[serverName]; ok {
		if err := caller.Close(); err != nil {
			slog.Warn("hot_reload: error closing mcp client", "server", serverName, "error", err)
		}
		delete(a.mcpClients, serverName)
	}

	slog.Info("hot_reload: mcp server unregistered",
		"server", serverName,
		"tools_removed", len(names),
		"total_tools", len(a.tools))
	return nil
}

// CloseHotMCPServers closes every MCP client registered via the hot-add
// path. The boot-time MCP Manager owns its own connections and is
// independent. Call this in the daimon shutdown defer so hot-added
// servers' subprocess children get cleanly terminated.
func (a *Agent) CloseHotMCPServers() {
	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()
	for name, caller := range a.mcpClients {
		if err := caller.Close(); err != nil {
			slog.Warn("hot_reload: error closing mcp client during shutdown",
				"server", name, "error", err)
		}
	}
	a.mcpClients = map[string]interface{ Close() error }{}
}

// ReplaceSkills atomically swaps the agent's autoload-skills slice and
// skill index. Used by the dashboard after a recipe skill is installed
// (installRecipeSkill writes the markdown then triggers this). The next
// turn's system prompt picks up the new content with no restart.
func (a *Agent) ReplaceSkills(skills []skill.SkillContent, idx skill.SkillIndex) {
	a.skillsMu.Lock()
	a.skills = skills
	a.skillIndex = idx
	a.skillsMu.Unlock()
	slog.Info("hot_reload: skills replaced", "autoload_count", len(skills))
}
