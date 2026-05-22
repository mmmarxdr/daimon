# Delta for Config (config)

This delta amends the canonical `openspec/specs/config/spec.md` with changes from the `subagents-crud` change.

---

## MODIFIED Requirements

### Requirement: CONFIG-REQ-4 — New skill frontmatter fields for executable skills (updated budget column)

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

(Previously: `budget` column was marked "YES (if `executable: true`) — N/A — load error if absent". It is now OPTIONAL: omitting the block is valid and results in unlimited runtime behavior.)

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

### Requirement: CONFIG-REQ-6 — Validation: `budget` is OPTIONAL for executable skills

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

## ADDED Requirements

### Requirement: CONFIG-REQ-9 — Skill `source` field is metadata-only

When a skill is materialized from the database (via `UserSkill`), the `source` field (`"user"` | `"curated"`) is metadata for the loader and frontend. The runtime MUST treat all skills equivalently regardless of source. The frontend MUST honor `source` for UI affordances (curated skills are read-only in the UI). The `source` field does NOT appear in `.skill.md` frontmatter — it is a DB-only attribute.

#### Scenario: user-source and curated-source skills behave identically at runtime

- GIVEN a user-source skill and a curated-source skill with identical `executable`, `model`, `provider`, `tools_allowlist`, and `budget` fields
- WHEN both are loaded and spawned
- THEN the runtime behavior is identical for both
- AND no source-based branching occurs in spawn, budget enforcement, or event emission

#### Scenario: curated source prevents write via REST

- GIVEN a skill with `source = "curated"` in the `user_skills` table
- WHEN a `PUT` or `DELETE` request is made for that skill via the REST API
- THEN the response is HTTP 403
- AND no modification is made to the row

---

### Requirement: CONFIG-REQ-5 — `tools_allowlist` unknown tool handling differs between REST writes and hot-reload

The skill loader and REST handlers MUST treat unknown tool names in `tools_allowlist` differently depending on the entry point:

| Entry point                                                           | Behavior      | Status code              | Rationale                                                                                                                                       |
| --------------------------------------------------------------------- | ------------- | ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| REST write (`POST` or `PUT /api/skills`)                              | HARD REJECT   | 422 Unprocessable Entity | API write is a synchronous user action; surfacing the typo immediately prevents persisting bad state                                            |
| Hot-reload of stored allowlist (e.g. `Agent.ReplaceExecutableSkills`) | WARN-AND-DROP | n/a (no HTTP response)   | A stored skill may reference a tool that disappeared (MCP server unregistered, build flag flipped); hard-fail would brick the agent reload path |
| Boot-time skill load from FS or DB                                    | WARN-AND-DROP | n/a                      | Same rationale as hot-reload — never block startup on a stale allowlist                                                                         |

(Previously: CONFIG-REQ-5 said the loader returned an unconditional "load error". The asymmetry was introduced by the `subagents-crud` change because Phase 3 added REST writes — a synchronous validation point that did not exist before. The original "load error" semantics now apply only to the REST write path.)

#### Scenario: REST POST with unknown tool name returns 422

- GIVEN a `POST /api/skills` body with `tools_allowlist: ["nonexistent-tool"]`
- AND `nonexistent-tool` is not registered in `ServerDeps.Tools`
- WHEN the request is processed
- THEN the response is HTTP 422 with a body identifying the invalid tool name
- AND no row is written to `user_skills`

#### Scenario: hot-reload from stored allowlist drops unknown tool

- GIVEN an existing `user_skills` row whose `tools_allowlist` includes `nonexistent-tool`
- AND `nonexistent-tool` is not registered in `ServerDeps.Tools`
- WHEN `Agent.ReplaceExecutableSkills` runs (boot or after a CRUD write)
- THEN `slog.Warn` is emitted with the dropped tool name
- AND the subagent registers with the remaining allowlist entries
- AND no error is returned

---

## Unchanged Requirements (cross-reference)

- **CONFIG-REQ-7** — `budget: defaults` must be a literal string: UNCHANGED.
- **CONFIG-REQ-8** — Non-executable skills load unchanged: UNCHANGED.
