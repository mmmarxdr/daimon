package web

import (
	"context"
	"embed"
	"log/slog"
	"os"
	"path/filepath"

	"daimon/internal/agent"
	"daimon/internal/config"
	"daimon/internal/skill"
)

//go:embed mcp_skills/*.md
var mcpSkillsFS embed.FS

// mcpRecipeSkills maps MCP server names (as configured by the catalog Add
// flow in the dashboard) to their bundled skill filenames. When a user adds
// one of these recipes, installRecipeSkill copies the corresponding markdown
// into ~/.daimon/skills/ and registers it for autoload — the next agent boot
// picks up the guidance and the LLM knows when/how to use the new tools.
//
// To add a new MCP recipe with a skill: drop the .md file in mcp_skills/,
// add the name → filename mapping here. No other wiring needed.
var mcpRecipeSkills = map[string]string{
	"gmail":               "mcp-gmail.md",
	"google-workspace":    "mcp-gmail.md",
	"google-calendar":     "mcp-google-calendar.md",
	"github":              "mcp-github.md",
	"brave-search":        "mcp-brave-search.md",
	"memory":              "mcp-memory.md",
	"obsidian":            "mcp-obsidian.md",
	"filesystem":          "mcp-filesystem.md",
	"scrapling":           "mcp-scrapling.md",
	"fetch":               "mcp-fetch.md",
	"sequential-thinking": "mcp-sequential-thinking.md",
	"time":                "mcp-time.md",
}

// installRecipeSkill copies the bundled skill file for an MCP recipe to the
// user's skills directory and registers it in the config. When `ag` is
// non-nil it ALSO hot-reloads the agent's skill state so the new guidance
// is active on the next turn — no daimon restart required.
//
// Best-effort: failures at any step are logged but never block the MCP add.
func installRecipeSkill(serverName string, cfg *config.Config, cfgPath string, ag AgentReloader) {
	skillFile, ok := mcpRecipeSkills[serverName]
	if !ok {
		return
	}

	content, err := mcpSkillsFS.ReadFile("mcp_skills/" + skillFile)
	if err != nil {
		slog.Warn("mcp: bundled skill not found", "server", serverName, "file", skillFile, "error", err)
		return
	}

	// Resolve skills directory.
	skillsDir := cfg.SkillsDir
	if skillsDir == "" {
		home, _ := os.UserHomeDir()
		skillsDir = filepath.Join(home, ".daimon", "skills")
	} else {
		// Expand tilde.
		if len(skillsDir) > 0 && skillsDir[0] == '~' {
			home, _ := os.UserHomeDir()
			skillsDir = filepath.Join(home, skillsDir[1:])
		}
	}

	// Ensure directory exists.
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		slog.Warn("mcp: failed to create skills dir", "path", skillsDir, "error", err)
		return
	}

	// Write the skill file.
	destPath := filepath.Join(skillsDir, skillFile)
	if err := os.WriteFile(destPath, content, 0o644); err != nil {
		slog.Warn("mcp: failed to write skill file", "path", destPath, "error", err)
		return
	}

	// Register in config via SkillService (adds to skills[] list in YAML).
	svc := skill.NewSkillService(cfgPath, skillsDir, cfg.SkillsRegistryURL)
	if err := svc.Add(context.Background(), destPath, false); err != nil {
		// Already registered is fine.
		slog.Debug("mcp: skill registration note", "file", skillFile, "error", err)
	}

	slog.Info("mcp: installed recipe skill", "server", serverName, "skill", destPath)

	// Hot-reload the agent's skill state so the new file is active on the
	// next turn. We re-read from the freshly-updated config rather than
	// trying to splice incrementally — simpler and matches boot semantics.
	if ag != nil {
		if reloaded, idx, err := loadSkillsForReload(cfgPath, cfg); err == nil {
			ag.ReplaceSkills(reloaded, idx)
		} else {
			slog.Warn("mcp: skill hot-reload failed (file saved, requires restart to use)",
				"server", serverName, "error", err)
		}
	}
}

// loadSkillsForReload re-runs the boot-time skill discovery for hot-reload.
// Reads the persisted config from disk to pick up changes the SkillService
// just made, resolves every registered skill markdown, and rebuilds the
// autoload slice + index. Mirrors what cmd/daimon does at startup minus
// the registry sync (skills are already on disk).
func loadSkillsForReload(cfgPath string, fallbackCfg *config.Config) ([]skill.SkillContent, skill.SkillIndex, error) {
	// Re-read the YAML so we see the entries the SkillService just appended.
	freshCfg, err := config.Load(cfgPath)
	if err != nil {
		// Fall back to the in-memory cfg passed to installRecipeSkill — it
		// will be one revision behind but still reasonable.
		freshCfg = fallbackCfg
	}

	contents, _, warns := skill.LoadSkills(freshCfg.Skills, freshCfg.Tools.Shell, freshCfg.Limits)
	for _, w := range warns {
		slog.Warn("hot_reload: skill load warning", "error", w)
	}

	maxCtx := freshCfg.Agent.MaxContextTokens
	if maxCtx == 0 {
		maxCtx = 100_000
	}
	autoload, idx := agent.InitSkillInjection(contents, maxCtx)
	return autoload, idx, nil
}
