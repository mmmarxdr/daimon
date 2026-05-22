package skill

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"daimon/internal/config"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// LoadSkillsUnified merges three sources into a single skill set with explicit
// precedence: DB > FS > Curated. It wraps LoadSkills for the FS pass and does
// not replace it. cmd/daimon/main.go and cmd/daimon/web_cmd.go switch to this
// entrypoint in Phase 4.
//
// curatedFS may be the zero value of embed.FS (curated load is skipped).
// dbStore may be nil (DB load is skipped — for tests or non-SQLite backends).
//
// Returns the same shape as LoadSkills: prose contents, tools map (FS tools only),
// executable defs, and accumulated non-fatal warns/errors.
//
// (AGENT-LOOP-REQ-7; design §2.4)
func LoadSkillsUnified(
	ctx context.Context,
	fsPaths []string,
	dbStore store.UserSkillStore,
	curatedFS embed.FS,
	shellCfg config.ShellToolConfig,
	limits config.LimitsConfig,
) ([]SkillContent, map[string]tool.Tool, []ExecutableSkillDef, []error) {
	var warns []error

	// index: name → (SkillContent, *ExecutableSkillDef, source label)
	type entry struct {
		content SkillContent
		exec    *ExecutableSkillDef
		source  string // "curated" | "fs" | "db"
	}
	index := make(map[string]entry)

	// Pass 1: Curated (lowest precedence).
	curatedContents, curatedExecs, curatedWarns := loadCurated(curatedFS, shellCfg, limits)
	warns = append(warns, curatedWarns...)
	for _, c := range curatedContents {
		index[c.Name] = entry{content: c, source: "curated"}
	}
	for i := range curatedExecs {
		e := index[curatedExecs[i].Name]
		e.exec = &curatedExecs[i]
		index[curatedExecs[i].Name] = e
	}

	// Pass 2: FS (overrides curated by name).
	fsContents, fsTools, fsExecs, fsWarns := LoadSkills(fsPaths, shellCfg, limits)
	warns = append(warns, fsWarns...)
	for _, c := range fsContents {
		if existing, hit := index[c.Name]; hit {
			slog.Warn("skill name collision", "name", c.Name, "winner", "fs", "loser", existing.source)
		}
		index[c.Name] = entry{content: c, source: "fs"}
	}
	for i := range fsExecs {
		e := index[fsExecs[i].Name]
		e.exec = &fsExecs[i]
		index[fsExecs[i].Name] = e
	}

	// Pass 3: DB (highest — DB always wins).
	if dbStore != nil {
		dbSkills, err := dbStore.ListUserSkills(ctx)
		if err != nil {
			warns = append(warns, fmt.Errorf("loader_unified: list db skills: %w", err))
		} else {
			for _, ds := range dbSkills {
				if existing, hit := index[ds.Name]; hit {
					slog.Warn("skill name collision", "name", ds.Name, "winner", "db", "loser", existing.source)
				}
				content, exec := userSkillToParts(ds)
				e := entry{content: content, source: "db"}
				if exec != nil {
					e.exec = exec
				}
				index[ds.Name] = e
			}
		}
	}

	// Flatten index back into output slices. Tools come exclusively from FS.
	contents := make([]SkillContent, 0, len(index))
	execs := make([]ExecutableSkillDef, 0, len(index))
	for _, e := range index {
		contents = append(contents, e.content)
		if e.exec != nil {
			execs = append(execs, *e.exec)
		}
	}
	return contents, fsTools, execs, warns
}

// userSkillToParts converts a store.UserSkill to the loader's runtime types.
// When us.Executable is true an ExecutableSkillDef is also returned.
// Budget == nil → BudgetConfig{} (zero values; all guards in budgetMonitor
// check > 0, so zero means "no limit"). Spawn also branches on Timeout > 0
// after the Phase 5 fix (REQ-16).
//
// (AGENT-LOOP-REQ-7; design §2.4)
func userSkillToParts(us store.UserSkill) (SkillContent, *ExecutableSkillDef) {
	var bf BudgetFrontmatter
	if us.Budget != nil {
		bf = BudgetFrontmatter{
			MaxCostUSD: us.Budget.MaxCostUSD,
			MaxTurns:   us.Budget.MaxTurns,
			TimeoutMin: us.Budget.TimeoutMin,
		}
	}

	sc := SkillContent{
		Name:           us.Name,
		Description:    us.Description,
		Prose:          us.Prose,
		Version:        us.Version,
		Executable:     us.Executable,
		Model:          us.Model,
		ProviderName:   us.Provider,
		SystemAddendum: us.Prose, // exec skills use Prose as SystemAddendum
		ToolsAllowlist: us.ToolsAllowlist,
		Budget:         bf,
	}

	if !us.Executable {
		return sc, nil
	}

	var bcfg BudgetConfig
	if us.Budget != nil {
		bcfg = BudgetConfig{
			MaxCostUSD: us.Budget.MaxCostUSD,
			MaxTurns:   us.Budget.MaxTurns,
			Timeout:    time.Duration(us.Budget.TimeoutMin) * time.Minute,
		}
	}

	exec := &ExecutableSkillDef{
		Name:           us.Name,
		Description:    us.Description,
		Version:        us.Version,
		Model:          us.Model,
		ProviderName:   us.Provider,
		SystemAddendum: us.Prose,
		ToolsAllowlist: us.ToolsAllowlist,
		Budget:         bcfg,
	}
	return sc, exec
}

// loadCurated walks curatedFS and parses each .skill.md file.
// Returns empty slices (not nil) when curatedFS is the zero value or empty.
// Does NOT register tool.Tool entries — curated templates reference environment
// tools only; tool definitions are local to the user's environment.
//
// (design §2.10)
func loadCurated(
	curatedFS embed.FS,
	shellCfg config.ShellToolConfig,
	limits config.LimitsConfig,
) ([]SkillContent, []ExecutableSkillDef, []error) {
	_ = shellCfg
	_ = limits

	entries, err := curatedFS.ReadDir("curated")
	if err != nil {
		// Zero-value embed.FS or empty directory — no curated load, no error.
		return nil, nil, nil
	}

	var contents []SkillContent
	var execs []ExecutableSkillDef
	var warns []error

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".skill.md") {
			continue
		}
		data, err := curatedFS.ReadFile("curated/" + e.Name())
		if err != nil {
			warns = append(warns, fmt.Errorf("curated: read %q: %w", e.Name(), err))
			continue
		}
		c, _, parseErrs := parseSkillContent("curated/"+e.Name(), string(data))
		warns = append(warns, parseErrs...)
		contents = append(contents, c)
		if c.Executable && len(parseErrs) == 0 {
			execs = append(execs, ExecutableSkillDef{
				Name:           c.Name,
				Description:    c.Description,
				Version:        c.Version,
				Model:          c.Model,
				ProviderName:   c.ProviderName,
				ProviderConfig: c.ProviderConfig,
				SystemAddendum: c.SystemAddendum,
				ToolsAllowlist: c.ToolsAllowlist,
				Budget: BudgetConfig{
					MaxCostUSD: c.Budget.MaxCostUSD,
					MaxTurns:   c.Budget.MaxTurns,
					Timeout:    time.Duration(c.Budget.TimeoutMin) * time.Minute,
				},
			})
		}
	}
	return contents, execs, warns
}
