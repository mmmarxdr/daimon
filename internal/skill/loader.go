package skill

import (
	"fmt"
	"os"
	"time"

	"daimon/internal/config"
	"daimon/internal/tool"
)

// LoadSkills loads skill files from the given paths.
// Returns:
//   - loaded prose contents (SkillContent slice)
//   - a map of tool.Tool implementations keyed by tool name
//   - a slice of ExecutableSkillDef for skills with executable:true
//   - a slice of non-fatal warnings/errors
//
// LoadSkills NEVER returns a fatal error; all failures are represented as warnings.
// The caller is responsible for logging returned warnings.
func LoadSkills(
	paths []string,
	shellCfg config.ShellToolConfig,
	limits config.LimitsConfig,
) ([]SkillContent, map[string]tool.Tool, []ExecutableSkillDef, []error) {
	if len(paths) == 0 {
		return nil, nil, nil, nil
	}

	tools := make(map[string]tool.Tool)
	var contents []SkillContent
	var execDefs []ExecutableSkillDef
	var warns []error
	var totalProseBytes int
	var warnedProseBudget bool

	for _, path := range paths {
		// Read file
		data, err := os.ReadFile(path)
		if err != nil {
			warns = append(warns, fmt.Errorf("skills: cannot read %q: %w", path, err))
			continue
		}

		// Size check (per-file limit: 8 KB)
		if len(data) > 8*1024 {
			warns = append(warns, fmt.Errorf("skills: file %q is too large (%d bytes, limit 8192); skipping", path, len(data)))
			continue
		}

		// Parse — use internal helper to avoid double I/O
		content, toolDefs, parseErrs := parseSkillContent(path, string(data))

		// Separate hard executable errors (budget missing/invalid) from soft
		// warnings. For executable skills with errors, skip the ExecutableSkillDef
		// but still collect the SkillContent (prose can still be useful).
		warns = append(warns, parseErrs...)

		// Track total prose bytes
		totalProseBytes += len(content.Prose)
		if totalProseBytes > 32*1024 && !warnedProseBudget {
			warns = append(warns, fmt.Errorf("skills: total skill prose exceeds 32 KB (%d bytes); system prompt will be large", totalProseBytes))
			warnedProseBudget = true
		}

		// Collect prose content.
		contents = append(contents, content)

		// Build ExecutableSkillDef for executable skills that parsed cleanly.
		if content.Executable && len(parseErrs) == 0 {
			def := ExecutableSkillDef{
				Name:           content.Name,
				Description:    content.Description,
				Version:        content.Version,
				Model:          content.Model,
				ProviderName:   content.ProviderName,
				ProviderConfig: content.ProviderConfig,
				SystemAddendum: content.SystemAddendum,
				ToolsAllowlist: content.ToolsAllowlist,
				Budget: BudgetConfig{
					MaxCostUSD: content.Budget.MaxCostUSD,
					MaxTurns:   content.Budget.MaxTurns,
					Timeout:    time.Duration(content.Budget.TimeoutMin) * time.Minute,
				},
			}
			execDefs = append(execDefs, def)
		}

		// Build tool.Tool for each ToolDef, with env expansion
		for _, def := range toolDefs {
			// Expand env values
			expandedEnv := make(map[string]string, len(def.Env))
			for k, v := range def.Env {
				expanded, err := config.ExpandSafeEnv(v)
				if err != nil {
					warns = append(warns, fmt.Errorf("skills: tool %q env[%q]: %w; using unexpanded value", def.Name, k, err))
					expandedEnv[k] = v // use original on error
				} else {
					expandedEnv[k] = expanded
				}
			}
			def.Env = expandedEnv

			// Apply config inheritance for WorkingDir
			if def.WorkingDir == "" {
				def.WorkingDir = shellCfg.WorkingDir
			}

			// Apply config inheritance for Timeout
			if def.Timeout == 0 {
				def.Timeout = limits.ToolTimeout
			}
			if def.Timeout == 0 {
				def.Timeout = 30 * time.Second // absolute fallback
			}

			// Collision check: first skill file wins among skill files
			if _, exists := tools[def.Name]; exists {
				warns = append(warns, fmt.Errorf("skills: tool name %q defined in multiple skill files; first definition wins", def.Name))
				continue
			}

			tools[def.Name] = NewSkillShellTool(def)
		}
	}

	return contents, tools, execDefs, warns
}
