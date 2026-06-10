# Delta for Config (config)

## Overview

Removes `FileChunkSize` from `ContextModeConfig` entirely. Any config file that sets this field MUST fail to load with a descriptive error — silent ignoring is prohibited. The `preApplyFileRead` dead code (filter side) is removed in the bounded-exec spec.

## MODIFIED Requirements

### Requirement: ContextModeConfig Fields

`ContextModeConfig` MUST NOT contain a `FileChunkSize` field. YAML configs that include `file_chunk_size` MUST produce a load-time error.

(Previously: `FileChunkSize` was present with a comment "reserved for Phase 2 chunking". The proposal originally said to log a warning; user decision overrides this — hard error on load.)

#### Scenario: Config without FileChunkSize loads successfully

- GIVEN a YAML config with `context_mode` fields but no `file_chunk_size` key
- WHEN the config is loaded
- THEN loading succeeds with no error

#### Scenario: Config with FileChunkSize fails to load

- GIVEN a YAML config that includes `file_chunk_size: 4096`
- WHEN the config is loaded
- THEN loading returns a non-nil error
- AND the error message references `file_chunk_size`
- AND the config is NOT used

#### Scenario: Existing fields are unaffected

- GIVEN a YAML config with valid `shell_max_output`, `sandbox_timeout`, and `auto_index_outputs`
- WHEN the config is loaded
- THEN all three fields parse correctly with no error

---

## ADDED Requirements (from subagents change)

### CONFIG-REQ-4 — New skill frontmatter fields for executable skills

The skill loader SHALL parse the following new YAML frontmatter fields on `.skill.md` files:

| Field                    | Type                         | Required for executable        | Default (non-executable)        | Description                                                                   |
| ------------------------ | ---------------------------- | ------------------------------ | ------------------------------- | ----------------------------------------------------------------------------- |
| `executable`             | `bool`                       | —                              | `false`                         | Marks the skill as a spawnable subagent definition                            |
| `version`                | `int`                        | No                             | `1`                             | Profile schema version; reserved for future field evolution                   |
| `model`                  | `string`                     | No                             | parent's model                  | LLM model the subagent runs on (e.g. `"claude-haiku-4-5"`)                    |
| `provider`               | `string`                     | No                             | parent's provider               | Provider name (e.g. `"anthropic"`, `"openrouter"`)                            |
| `system_prompt_addendum` | `string`                     | No                             | `""`                            | Extra system prompt text appended to the base system prompt for this subagent |
| `tools_allowlist`        | `[]string`                   | No                             | `[]` (inherit all parent tools) | Exact tool names the subagent may use                                         |
| `budget`                 | `BudgetConfig \| "defaults"` | **No** — MAY be omitted        | `nil` (unlimited)               | Budget constraints block; absence means unlimited                             |
| `budget.max_cost_usd`    | `float64`                    | Yes (if explicit budget block) | —                               | Max spend in USD                                                              |
| `budget.max_turns`       | `int`                        | Yes (if explicit budget block) | —                               | Max turn count                                                                |
| `budget.timeout_min`     | `int`                        | Yes (if explicit budget block) | —                               | Max wall-clock runtime in minutes                                             |

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

#### Scenario: executable skill with NO budget block loads successfully (reversal)

- GIVEN a skill file with `executable: true` and no `budget` key
- WHEN the skill is loaded
- THEN loading succeeds with no error
- AND `Budget` on the resulting `ExecutableSkillDef` is nil (unlimited semantics)

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

### CONFIG-REQ-6 — Validation: `budget` is OPTIONAL for executable skills

Every skill file with `executable: true` MAY omit the `budget` block. When omitted, the skill loads with `Budget = nil`, meaning unlimited cost, unlimited turns, and no timeout. The parser MUST NOT return an error for a missing `budget` key on an executable skill.

(Previously: CONFIG-REQ-6 required the `budget` block and returned a load error when absent. This requirement is explicitly reversed by the `subagents-crud` change.)

#### Scenario: executable skill without budget key loads successfully

- GIVEN a skill file with `executable: true` and no `budget` key
- WHEN the skill is loaded
- THEN loading succeeds with no error
- AND `Budget` is nil on the resulting `ExecutableSkillDef`

#### Scenario: explicit budget block with all three fields still loads successfully

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

## ADDED Requirements (from minimax-provider change)

### CONFIG-MM-1 — `minimax` is a known provider type

`KnownProviders` MUST include the string `"minimax"`. `IsKnownProvider("minimax")` MUST return `true`. All provider-type validation switches (v2 active provider, v1 legacy provider, `Fallback.Type`) MUST accept `"minimax"` as a valid value without error.

(Previously: `"minimax"` was absent from `KnownProviders` and all four validation switches; using it produced a validation error.)

#### Scenario CM-1a: minimax passes IsKnownProvider

- GIVEN the global `KnownProviders` slice
- WHEN `IsKnownProvider("minimax")` is called
- THEN it returns `true`

#### Scenario CM-1b: minimax passes v2 active-provider validation

- GIVEN a config with `providers.active: minimax` and a valid `api_key`
- WHEN the config is validated
- THEN no error is returned for the provider type

#### Scenario CM-1c: minimax passes v1 legacy provider validation

- GIVEN a v1 config block with `provider.type: minimax` and a valid `api_key`
- WHEN the config is validated
- THEN no error is returned for the provider type

#### Scenario CM-1d: minimax passes Fallback.Type validation

- GIVEN a fallback config with `fallback.type: minimax` and a valid `api_key`
- WHEN the config is validated
- THEN no error is returned for the provider type

---

### CONFIG-MM-2 — api_key is REQUIRED for minimax

A config block with `type: minimax` MUST be rejected at validation time if `api_key` is absent or empty. The standard `openai`-with-custom-base exemption (which allows `openai` to skip `api_key` when `base_url` is custom) MUST NOT apply to `minimax`.

#### Scenario CM-2a: minimax config with api_key passes validation

- GIVEN a provider config `type: minimax` with `api_key: "sk-cp-abc123"`
- WHEN the config is validated
- THEN validation succeeds

#### Scenario CM-2b: minimax config without api_key fails validation

- GIVEN a provider config `type: minimax` with no `api_key` field
- WHEN the config is validated
- THEN a non-nil validation error is returned
- AND the error message references `api_key` or the minimax provider
