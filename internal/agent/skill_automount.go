package agent

// skill_automount.go — WU8: auto-mount ExecutableSkillDef entries as slash commands.
//
// Design D5 (obs #361): when WithExecutableSkills or ReplaceExecutableSkills is called,
// each skill is also registered as a /<normalized_name> command with source="skill".
//
// Normalization rule D3: hyphens in def.Name become underscores so the resulting
// command name passes parseCommand's [a-zA-Z][a-zA-Z0-9_]{0,31} regex.
//
// Precedence Q3: builtin > cron > skill. RegisterIfFree is used — collisions are
// skipped with slog.Warn. The skill remains registered as a SubagentSpawnTool in
// a.tools; only the slash-command auto-mount is suppressed.
//
// Handler shape: the slash command looks up the *SubagentSpawnTool in a.tools by
// def.Name at invocation time, calls Execute with the user args as JSON {"prompt":…},
// and replies with the SubagentResult.Summary. If the tool is not found or the
// manager is nil, it replies with an informative message.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"daimon/internal/skill"
)

// registerSkillCommands registers each ExecutableSkillDef as a slash command in
// a.commands. Callers must NOT hold a.commands.mu — RegisterIfFree acquires it
// internally. Hyphens in def.Name are replaced with underscores before
// registration (D3). Built-in/cron collisions are silently skipped with WARN.
//
// Called from WithExecutableSkills (initial mount) and ReplaceExecutableSkills
// (hot-reload, after UnregisterAllBySource("skill")).
func registerSkillCommands(a *Agent, defs []skill.ExecutableSkillDef) {
	for _, def := range defs {
		normalized := strings.ReplaceAll(def.Name, "-", "_")
		if normalized != def.Name {
			slog.Info("skill_automount: normalized skill name for slash command",
				"skill", def.Name, "command", "/"+normalized)
		}

		desc := def.Description
		if desc == "" {
			desc = "Subagent: " + def.Name
		}

		handler := makeSkillCommandHandler(a, def.Name)
		ok, err := a.commands.RegisterIfFree(normalized, desc, handler, SourceSkill)
		if err != nil {
			slog.Warn("skill_automount: invalid command name from skill",
				"skill", def.Name, "command", normalized, "error", err)
			continue
		}
		if !ok {
			slog.Warn("skill_command: collision, builtin/cron wins",
				"skill", def.Name, "command", normalized)
		}
	}
}

// makeSkillCommandHandler returns a CommandHandler that invokes the
// SubagentSpawnTool registered under toolName in a.tools. The handler looks up
// the tool at invocation time (not at mount time) so hot-reload replacements are
// picked up transparently.
//
// The handler marshals cc.Args as {"prompt": args} and calls Execute on the tool.
// On success it replies with SubagentResult.Summary. On any error it replies with
// a descriptive message.
func makeSkillCommandHandler(a *Agent, toolName string) CommandHandler {
	return func(cc CommandContext) error {
		// Look up the spawn tool at invocation time.
		a.toolsMu.RLock()
		t, found := a.tools[toolName]
		a.toolsMu.RUnlock()
		if !found {
			cc.Reply(fmt.Sprintf("Skill /%s is no longer available.", strings.ReplaceAll(toolName, "-", "_")))
			return nil
		}

		spawnTool, ok := t.(*SubagentSpawnTool)
		if !ok {
			cc.Reply(fmt.Sprintf("Skill /%s is not a subagent tool.", strings.ReplaceAll(toolName, "-", "_")))
			return nil
		}
		if spawnTool.manager == nil {
			cc.Reply(fmt.Sprintf("Skill /%s: subagent manager not initialized — skill command unavailable.",
				strings.ReplaceAll(toolName, "-", "_")))
			return nil
		}

		// Build params JSON: {"prompt": <user args>}.
		params, err := json.Marshal(map[string]string{"prompt": cc.Args})
		if err != nil {
			cc.Reply(fmt.Sprintf("Skill /%s: failed to build params: %v",
				strings.ReplaceAll(toolName, "-", "_"), err))
			return nil
		}

		ctx := cc.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		result, execErr := spawnTool.Execute(ctx, params)
		if execErr != nil {
			cc.Reply(fmt.Sprintf("Skill /%s failed: %v",
				strings.ReplaceAll(toolName, "-", "_"), execErr))
			return nil
		}
		if result.IsError {
			cc.Reply(fmt.Sprintf("Skill /%s error: %s",
				strings.ReplaceAll(toolName, "-", "_"), result.Content))
			return nil
		}

		// Parse the SubagentResult JSON and reply with the summary.
		var res SubagentResult
		if jsonErr := json.Unmarshal([]byte(result.Content), &res); jsonErr != nil {
			// Fall back to raw content if JSON parsing fails.
			cc.Reply(result.Content)
			return nil
		}
		cc.Reply(res.Summary)
		return nil
	}
}
