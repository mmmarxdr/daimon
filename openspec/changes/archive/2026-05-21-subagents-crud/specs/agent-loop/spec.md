# Delta for Agent Loop (agent-loop)

This delta amends the canonical `openspec/specs/agent-loop/spec.md` with changes from the `subagents-crud` change.

---

## ADDED Requirements

### Requirement: AGENT-LOOP-REQ-7 — `LoadSkillsUnified` merges multi-source skills

The `skill` package SHALL expose:

```go
func LoadSkillsUnified(
    fsPaths []string,
    dbSkills []store.UserSkill,
    curatedFS embed.FS,
    shellCfg config.ShellToolConfig,
    limits config.LimitsConfig,
) ([]SkillContent, map[string]tool.Tool, []ExecutableSkillDef, []error)
```

Skills are merged with the following precedence (highest wins on name collision):
1. **Curated** (from `curatedFS`, lowest precedence)
2. **Filesystem** (from `fsPaths`, via existing `LoadSkills`)
3. **DB** (from `dbSkills`, highest precedence)

On any name collision, the higher-precedence source replaces the lower-precedence entry and a `slog.Warn` is logged identifying the collision. The existing `LoadSkills` function MUST remain available and unchanged.

#### Scenario: DB skill wins over same-named curated skill

- GIVEN a curated skill named `"researcher"` (from `curatedFS`)
- AND a `user_skill` row also named `"researcher"` (from `dbSkills`)
- WHEN `LoadSkillsUnified` runs
- THEN the resulting skill slice contains the DB version of `"researcher"`
- AND a WARN is logged for the collision

#### Scenario: DB skill wins over same-named filesystem skill

- GIVEN `fsPaths = ["~/x.skill.md"]` pointing to a skill named `"analyst"`
- AND a `user_skill` row also named `"analyst"` (from `dbSkills`)
- WHEN `LoadSkillsUnified` runs
- THEN the resulting skill slice contains the DB version of `"analyst"`
- AND a WARN is logged for the collision

#### Scenario: empty inputs return empty outputs without error

- GIVEN `fsPaths = []`, `dbSkills = []`, and an empty `curatedFS`
- WHEN `LoadSkillsUnified` runs
- THEN all return slices are empty
- AND the errors slice is nil or empty

#### Scenario: existing FS skills load unchanged

- GIVEN `fsPaths` contains valid existing skill files
- AND `dbSkills` is empty and `curatedFS` is empty
- WHEN `LoadSkillsUnified` runs
- THEN the result is identical to calling `LoadSkills(fsPaths, ...)` directly
- AND no warnings are emitted

---

### Requirement: AGENT-LOOP-REQ-8 — Hot-reload after CRUD writes

After every write operation in the skills REST handler (POST create, PUT update, DELETE), the handler SHALL:

1. Query all user skills via `UserSkillStore.ListUserSkills`
2. Re-run `LoadSkillsUnified(cfg.Skills, freshDBSkills, curatedFS, ...)` to produce the merged skill set
3. Call `Agent.ReplaceExecutableSkills(execDefs)` to update spawn tools
4. Call `Agent.ReplaceSkills(autoloadSkills, skillIndex)` to update the system prompt skill injections

This ensures in-memory agent state always matches database state without a process restart.

#### Scenario: freshly-created executable skill is immediately spawnable

- GIVEN the agent has been running with no executable skills
- WHEN `POST /api/skills` creates a new executable skill
- AND hot-reload runs as part of the handler
- THEN a subsequent spawn request via `SubagentSpawnTool` succeeds (no restart required)

#### Scenario: deleted skill is no longer spawnable

- GIVEN an executable skill `"researcher"` is registered and spawnable
- WHEN `DELETE /api/skills/researcher` is called
- AND hot-reload runs as part of the handler
- THEN a subsequent spawn attempt for `"researcher"` returns a tool-not-found error

#### Scenario: updated skill definition takes effect immediately

- GIVEN an executable skill `"coder"` with `max_turns: 5`
- WHEN `PUT /api/skills/coder` updates the skill to `max_turns: 20`
- AND hot-reload runs
- THEN the next spawn of `"coder"` uses `max_turns: 20`

---

## Unchanged Requirements (cross-reference)

- **AGENT-LOOP-REQ-5** — Wire SubagentManager in `agent.New()`: UNCHANGED.
- **AGENT-LOOP-REQ-6** — Subagent independence (own inbox, sem, ctx): UNCHANGED.
