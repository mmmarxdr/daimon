# Delta for Config (config)

## Overview

Adds optional skill frontmatter fields for executable subagent definitions. All new fields have backward-compatible defaults — existing skill files with none of these keys load exactly as before. Introduces three load-time validation errors for executable skills: unknown tool in `tools_allowlist`, missing `budget` block, and invalid `budget: defaults` usage.

---

## ADDED Requirements

### CONFIG-REQ-4 — New skill frontmatter fields for executable skills

The skill loader SHALL parse the following new YAML frontmatter fields on `.skill.md` files:

| Field | Type | Required for executable | Default (non-executable) | Description |
|---|---|---|---|---|
| `executable` | `bool` | — | `false` | Marks the skill as a spawnable subagent definition |
| `version` | `int` | No | `1` | Profile schema version; reserved for future field evolution |
| `model` | `string` | No | parent's model | LLM model the subagent runs on (e.g. `"claude-haiku-4-5"`) |
| `provider` | `string` | No | parent's provider | Provider name (e.g. `"anthropic"`, `"openrouter"`) |
| `system_prompt_addendum` | `string` | No | `""` | Extra system prompt text appended to the base system prompt for this subagent |
| `tools_allowlist` | `[]string` | No | `[]` (inherit all parent tools) | Exact tool names the subagent may use |
| `budget` | `BudgetConfig \| "defaults"` | YES (if `executable: true`) | N/A — load error if absent | Budget constraints block |
| `budget.max_cost_usd` | `float64` | Yes (if explicit budget block) | — | Max spend in USD |
| `budget.max_turns` | `int` | Yes (if explicit budget block) | — | Max turn count |
| `budget.timeout_min` | `int` | Yes (if explicit budget block) | — | Max wall-clock runtime in minutes |

#### Scenario: full executable skill frontmatter parses correctly

- GIVEN a skill file `researcher.skill.md` with:
  ```yaml
  ---
  name: researcher
  executable: true
  version: 1
  model: claude-haiku-4-5
  provider: anthropic
  system_prompt_addendum: "Focus only on technical documentation."
  tools_allowlist:
    - read_file
    - mcp.github.search_code
  budget:
    max_cost_usd: 0.10
    max_turns: 10
    timeout_min: 5
  ---
  ```
- WHEN the skill loader processes this file
- THEN the returned `ExecutableSkillDef` has `Model: "claude-haiku-4-5"`, `Provider: "anthropic"`, `MaxCostUSD: 0.10`, `MaxTurns: 10`, `TimeoutMin: 5`
- AND `ToolsAllowlist: ["read_file", "mcp.github.search_code"]`
- AND `SystemPromptAddendum: "Focus only on technical documentation."`

#### Scenario: `budget: defaults` shortcut parses and expands

- GIVEN a skill file with:
  ```yaml
  executable: true
  budget: defaults
  ```
- WHEN the skill loader processes this file
- THEN the `ExecutableSkillDef` has `MaxCostUSD: 0.50`, `MaxTurns: 20`, `TimeoutMin: 10`

---

### CONFIG-REQ-5 — Validation: unknown tool in `tools_allowlist` is a load error

The skill loader SHALL validate each name in `tools_allowlist` against the set of tool names available at load time. Any name not found in the registry MUST cause the skill to fail to load with an error identifying the unknown name. The validation uses exact string matching only — no glob or prefix matching in V1.

(Previously: no `tools_allowlist` concept existed.)

#### Scenario: valid allowlist loads successfully

- GIVEN `tools_allowlist: ["read_file", "shell_exec"]`
- AND both `"read_file"` and `"shell_exec"` exist in the tool registry
- WHEN the skill is loaded
- THEN loading succeeds and the allowlist is stored as-is

#### Scenario: unknown tool name causes load error

- GIVEN `tools_allowlist: ["read_file", "does_not_exist"]`
- AND `"does_not_exist"` is NOT in the tool registry
- WHEN the skill is loaded
- THEN loading returns a non-nil error
- AND the error message contains `"does_not_exist"`
- AND the skill is NOT added to the executable skill list

#### Scenario: empty allowlist is valid (inherit all parent tools)

- GIVEN an executable skill with no `tools_allowlist` key (or `tools_allowlist: []`)
- WHEN the skill is loaded
- THEN loading succeeds
- AND the subagent will inherit all parent tools at spawn time (no filtering)

---

### CONFIG-REQ-6 — Validation: missing `budget` block on executable skill is a load error

Every skill file with `executable: true` MUST declare a `budget` block (either explicit values or the `"defaults"` shortcut). Absence of the `budget` key MUST cause a load error.

(Previously: no budget concept existed.)

#### Scenario: executable skill without budget key causes load error

- GIVEN a skill file with `executable: true` and no `budget` key
- WHEN the skill is loaded
- THEN loading returns a non-nil error
- AND the error message references `"budget"` and the skill name

#### Scenario: explicit budget block with all three fields loads successfully

- GIVEN `budget: { max_cost_usd: 0.25, max_turns: 15, timeout_min: 8 }`
- WHEN the skill is loaded
- THEN loading succeeds and all three budget values are accessible on `ExecutableSkillDef`

---

### CONFIG-REQ-7 — Validation: `budget: defaults` must be a literal string

The `budget` field accepts either a mapping (explicit values) or the literal string `"defaults"`. Any other string value MUST be a load error.

#### Scenario: `budget: defaults` is valid

- GIVEN `budget: defaults` in frontmatter
- WHEN the skill is loaded
- THEN loading succeeds with expanded default values

#### Scenario: `budget: something_else` is a load error

- GIVEN `budget: random_value` in frontmatter
- WHEN the skill is loaded
- THEN loading returns a non-nil error
- AND the error references the invalid budget value `"random_value"`

---

### CONFIG-REQ-8 — Backward compatibility: non-executable skill files load unchanged

Skill files that do not include `executable: true` (or include `executable: false`) MUST load exactly as before with no warnings, no errors, and no behavioral change. The new frontmatter fields are silently ignored for non-executable skills.

(Previously: all skill files fell into this category — this requirement preserves that behavior.)

#### Scenario: existing skill file with no new fields loads unchanged

- GIVEN a skill file `summarizer.skill.md` with only `name`, `description`, `autoload`, and prose
- WHEN the skill is loaded
- THEN loading succeeds with no warnings or errors
- AND the skill's prose is injected into the agent system prompt as before
- AND no `ExecutableSkillDef` is produced for this skill

#### Scenario: skill with `executable: false` is treated as non-executable

- GIVEN a skill file with `executable: false` explicitly set
- WHEN the skill is loaded
- THEN the skill is treated identically to a non-executable skill
- AND no `SubagentSpawnTool` is registered for it

---

## Acceptance Criteria

- [ ] All new frontmatter fields parse correctly without breaking existing skill loading.
- [ ] `budget: defaults` expands to `max_cost_usd: 0.50`, `max_turns: 20`, `timeout_min: 10`.
- [ ] Unknown tool name in `tools_allowlist` produces a load error naming the offending tool.
- [ ] `executable: true` skill with no `budget` key produces a load error.
- [ ] Any non-`"defaults"` string value for `budget` that is not a mapping produces a load error.
- [ ] Non-executable skill files (no `executable` key or `executable: false`) load with zero behavioral change.
- [ ] `version` field defaults to `1` when absent; other version values parse but are not acted upon in V1.
