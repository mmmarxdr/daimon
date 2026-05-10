package skill

import "time"

// SkillContent holds the parsed behavioral content from a skill file.
// Used by the agent to inject behavioural instructions into the system prompt.
type SkillContent struct {
	Name        string // from frontmatter; fallback: filename stem (no extension)
	Description string // from frontmatter; optional, informational only
	Prose       string // everything except frontmatter and ```yaml tool blocks
	Autoload    bool   // from frontmatter; if true, skill is loaded automatically

	// V1 additions for executable (spawnable subagent) skills.
	// All fields default to zero values — existing non-executable skill files
	// are unaffected (Executable=false, Budget=BudgetFrontmatter{}, etc.).

	// Version is the skill schema version; defaults to 1 when absent.
	Version int

	// Executable, when true, marks this skill as a spawnable subagent definition.
	// A corresponding ExecutableSkillDef is produced by the loader.
	Executable bool

	// Model is the LLM model the subagent runs on (e.g. "claude-haiku-4-5").
	// Empty means inherit the parent's model.
	Model string

	// ProviderName is the provider the subagent uses (e.g. "anthropic").
	// Empty means inherit the parent's provider.
	ProviderName string

	// ProviderConfig holds raw provider-specific options from frontmatter.
	ProviderConfig map[string]any

	// SystemAddendum is extra system-prompt text appended to the base system
	// prompt for this subagent only. Separate from Prose so it does not leak
	// into the principal's prompt-injection path.
	SystemAddendum string

	// ToolsAllowlist is the set of exact tool names the subagent may use.
	// nil or empty means "inherit all parent tools" (no filtering).
	// Set to an explicit empty slice when tools_allowlist: [] in frontmatter.
	ToolsAllowlist []string

	// Budget holds the parsed frontmatter budget block. Only valid when
	// Executable=true. Zero value means "not set".
	Budget BudgetFrontmatter
}

// BudgetFrontmatter is the frontmatter representation of a skill's budget.
// After parsing, it is validated and converted to a BudgetConfig (with
// time.Duration) in the ExecutableSkillDef.
type BudgetFrontmatter struct {
	// Defaults is true when frontmatter contains `budget: defaults` (the
	// shorthand literal). When true the canonical defaults are applied:
	// MaxCostUSD=0.50, MaxTurns=20, TimeoutMin=10.
	Defaults bool `yaml:"-"`

	MaxCostUSD float64 `yaml:"max_cost_usd"`
	MaxTurns   int     `yaml:"max_turns"`
	TimeoutMin int     `yaml:"timeout_min"`
}

// BudgetConfig is the resolved budget for an ExecutableSkillDef. Durations are
// already converted; this is what SubagentManager consumes.
type BudgetConfig struct {
	MaxCostUSD float64
	MaxTurns   int
	Timeout    time.Duration
}

// ExecutableSkillDef is a fully resolved, type-safe subagent spawn definition.
// It is produced by the skill loader for each skill with executable:true and
// consumed by agent.New() to register SubagentSpawnTools.
//
// Kept separate from SkillContent so the principal's prompt-injection path
// cannot accidentally include SystemAddendum or budget constraints.
type ExecutableSkillDef struct {
	Name           string
	Description    string
	Version        int
	Model          string
	ProviderName   string
	ProviderConfig map[string]any
	SystemAddendum string
	ToolsAllowlist []string
	Budget         BudgetConfig
}

// ToolDef is the parsed representation of a ```yaml tool fenced block.
// All fields map 1:1 to YAML keys inside the fenced block.
type ToolDef struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Command     string            `yaml:"command"`
	Timeout     time.Duration     `yaml:"timeout"`     // 0 = inherit limits.tool_timeout
	WorkingDir  string            `yaml:"working_dir"` // "" = inherit tools.shell.working_dir
	Env         map[string]string `yaml:"env"`         // values expanded at load time
}
