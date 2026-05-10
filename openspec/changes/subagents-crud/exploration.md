# Exploration: subagents-crud

**Change**: `subagents-crud`
**Date**: 2026-05-10
**Author**: sdd-explore sub-agent (Claude Sonnet 4.6)
**artifact_store**: hybrid (engram topic `sdd/subagents-crud/explore` + this file)

---

## 1. Executive Summary

The `subagents-crud` change builds a full CRUD system for agent definitions (executable skills) stored in SQLite (`user_skills` table, migration v18) and exposes them through new REST endpoints (`/api/skills`). The primary architectural impact is a **loader unification**: `LoadSkills` currently reads only filesystem paths from `cfg.Skills`; it must be extended (or wrapped) to also pull from DB and optionally embed curated templates. The change touches `internal/skill` (loader, parser, structs), `internal/store` (migration + new interface), `internal/web` (REST handlers, server.go routes), and the two wiring entrypoints (`cmd/daimon/main.go`, `web_cmd.go`). The W4 follow-up (provider `Config()` interface) and cancel endpoint are orthogonal Phase 1 items that can ship in an isolated PR before the schema exists. The dominant runtime risk is the REQ-12 budget reversal: `context.WithTimeout(ctx, 0)` creates a context that expires immediately — unlimited-budget subagents would be cancelled instantly without a special-case fix in `Spawn`.

---

## 2. Current Skill Loading Flow

### Discovery
Skills are discovered via `cfg.Skills` — a `[]string` of file paths written to `config.yaml`. There is no directory glob; each file must be explicitly listed. Two secondary paths exist:
- `~/.daimon/skills/` (managed store) — populated by `SkillService.Add`, `installRecipeSkill`
- `internal/web/mcp_skills/*.md` — bundled via `embed.FS`, copied to `~/.daimon/skills/` on MCP add, then added to `cfg.Skills` via `SkillService.Add`

### LoadSkills Signature (current)
```go
func LoadSkills(
    paths []string,
    shellCfg config.ShellToolConfig,
    limits config.LimitsConfig,
) ([]SkillContent, map[string]tool.Tool, []ExecutableSkillDef, []error)
```
Called in `main.go` and `web_cmd.go` with `cfg.Skills`. Returns four values; errors are non-fatal warns.

### Flow: disk → Agent.tools
1. `main.go` calls `LoadSkills(cfg.Skills, ...)` → `(skillContents, skillTools, execSkillDefs, warns)`
2. `agent.InitSkillInjection(skillContents, maxCtx)` → `(autoloadSkills, skillIndex)` (prose injection)
3. `agent.New(...)` receives `autoloadSkills, skillIndex` + the tools map
4. `agent.New` calls `.WithExecutableSkills(execSkillDefs)` → registers one `SubagentSpawnTool` per def in `a.tools`
5. Two-phase allowlist validation: `filterKnownTools` drops any allowlist entry that does not exist in `a.tools`

### Hot-Reload (prose only)
`installRecipeSkill` → writes `.md` to disk → `SkillService.Add` updates config → `loadSkillsForReload` re-runs `LoadSkills` → `ag.ReplaceSkills(autoload, idx)` swaps prose/injection state under `skillsMu`. It does NOT re-run `WithExecutableSkills`. Hot-reload for executable spawn tools is currently incomplete — restart is required today.

### Missing for CRUD: ReplaceExecutableSkills
Any CRUD write must call a new `ag.ReplaceExecutableSkills(defs)` that acquires `toolsMu.Lock`, removes old `*SubagentSpawnTool` entries, and registers new ones. This method does not exist yet.

### ToolsAllowlist Cross-Validation
`filterKnownTools` runs at `WithExecutableSkills` call time. DB-sourced skills validated at write time (handler validates against `s.deps.Tools`). Unknown names at spawn time are silently dropped (existing behavior).

---

## 3. SQLite Schema for user_skills (migration v18)

```sql
CREATE TABLE IF NOT EXISTS user_skills (
    id          TEXT PRIMARY KEY,            -- UUID v4
    name        TEXT NOT NULL UNIQUE,        -- slug: ^[a-z][a-z0-9_-]*$, max 64 chars
    description TEXT NOT NULL DEFAULT '',
    prose       TEXT NOT NULL DEFAULT '',    -- body / system_prompt_addendum
    executable  INTEGER NOT NULL DEFAULT 0,  -- 0 = prose-only, 1 = spawnable subagent
    model       TEXT NOT NULL DEFAULT '',    -- empty = inherit parent
    provider    TEXT NOT NULL DEFAULT '',    -- empty = inherit parent
    tools_allowlist TEXT,                    -- JSON array or NULL (nil = inherit all)
    budget      TEXT,                        -- JSON object or NULL (nil = unlimited)
    version     INTEGER NOT NULL DEFAULT 1,
    source      TEXT NOT NULL DEFAULT 'user',-- 'user' | 'curated'
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_skills_source ON user_skills(source);
CREATE INDEX IF NOT EXISTS idx_user_skills_executable ON user_skills(executable);
```

Notes:
- `name` UNIQUE covers both user and curated rows — name conflicts prevented at write time.
- `tools_allowlist` is stored as JSON array `["tool_a"]` or SQL NULL (inherit all).
- `budget` is stored as JSON object `{"max_cost_usd":0.50,"max_turns":20,"timeout_min":10}` or SQL NULL (unlimited).
- `prose` maps to `SkillContent.Prose` for non-executable skills and `ExecutableSkillDef.SystemAddendum` for executable ones.
- No `provider_config` column in Phase 2 — can be added as a follow-up.
- No backfill needed from existing data. Migration is append-only.

---

## 4. Curated Catalog Strategy

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| A: embed.FS in `internal/skill/curated/` | Zero DB impact; versioned with binary; testable | Updates require binary release; read-only by definition | **Recommended** |
| B: Seed into DB at migration v18 | Single source at runtime | Mutation risk; migration coupling; updates need migration bump | Rejected |
| C: Filesystem `internal/skill/curated/` without embed | Familiar | Not portable in installed binary; requires file presence in tests | Rejected |

**Recommendation (Option A)**: Ship `internal/skill/curated/*.md` with `//go:embed curated/*.md` in `loader_unified.go`. `LoadSkillsUnified` calls `fs.WalkDir` over the embedded FS to produce curated `SkillContent` + `ExecutableSkillDef` entries.

**Disable mechanism**: A user can shadow a curated skill by creating a user_skill with the same name. DB wins over curated in precedence, effectively overriding it. No separate `disabled_curated` mechanism needed in Phase 6.

---

## 5. Default Budget Reversal (REQ-12 flip)

### Current enforcement (parser.go lines 258–261)
```go
if fm.Executable {
    if fm.Budget.IsZero() {
        errs = append(errs, fmt.Errorf("skill %q: executable skills must declare a budget block ...", path))
    }
}
```
This hard error is present only for FS-parsed skills. DB-sourced skills bypass the parser (they are constructed directly from DB row → struct).

### What changes in parser.go
Remove the `fm.Budget.IsZero()` hard-error block entirely. An absent budget node produces `BudgetFrontmatter{}` (zero value), mapping to `BudgetConfig{MaxCostUSD:0, MaxTurns:0, Timeout:0}`.

### CRITICAL: Timeout==0 bug in subagent_manager.go
In `Spawn`:
```go
subCtx, cancel := context.WithTimeout(ctx, def.Budget.Timeout)
```
When `Timeout == 0`, `context.WithTimeout` creates a context that expires immediately on the next scheduler yield. This would silently cancel every unlimited-budget subagent instantly.

**Fix required (Phase 5)**:
```go
var subCtx context.Context
var cancel context.CancelFunc
if def.Budget.Timeout > 0 {
    subCtx, cancel = context.WithTimeout(ctx, def.Budget.Timeout)
} else {
    subCtx, cancel = context.WithCancel(ctx)
}
```

### budgetMonitor guard (already correct)
```go
softHit := !rec.softWarned && rec.budget.MaxCostUSD > 0 && rec.cost >= 0.8*rec.budget.MaxCostUSD
hardCost := rec.budget.MaxCostUSD > 0 && rec.cost >= rec.budget.MaxCostUSD
hardTurns := rec.budget.MaxTurns > 0 && rec.turns >= rec.budget.MaxTurns
```
Zero values naturally mean "no limit" — both cost and turn guards are already guarded by `> 0`. No change needed to `budgetMonitor` itself.

---

## 6. REST CRUD Endpoints

### Auth / middleware pattern
`GET` endpoints → `s.mux.HandleFunc(pattern, handler)` (auth middleware runs globally).
Mutating endpoints → `s.mux.Handle(pattern, requireOriginIfCrossOrigin(ao, http.HandlerFunc(handler)))`.

### Proposed routes
```
GET    /api/skills             — list all (?source=user|curated|all)
GET    /api/skills/{name}      — single skill by name
POST   /api/skills             — create user skill
PUT    /api/skills/{name}      — update user skill (403 if source=curated)
DELETE /api/skills/{name}      — delete user skill (403 if source=curated)
POST   /api/subagents/{id}/cancel — cancel running subagent
```

### Cancel endpoint (Phase 1 — no schema deps)
1. Add `CancelSubagent(id string) error` to `SubagentProvider` interface in `server.go`
2. Add `func (a *Agent) CancelSubagent(id string) error` in `agent.go` (delegates to `a.subMgr.Cancel`, nil-safe)
3. Add `handleSubagentCancel` in `handler_subagents.go`
4. Register route in `server.go`

### Validation rules (POST/PUT)
- `name`: required; `^[a-z][a-z0-9_-]*$`; max 64 chars; unique
- `executable`: optional; default false
- `budget`: nullable JSON; if present, at least one of max_cost_usd / max_turns / timeout_min must be > 0
- `prose`, `description`: max 8 KB
- `tools_allowlist`: JSON array of known tool names (validated against `s.deps.Tools`)

---

## 7. Loader Unification Strategy

### Precedence (highest wins on same name)
1. **User DB skills** (`source='user'`)
2. **User FS skills** (`cfg.Skills` paths) — backward-compat
3. **Curated** (embedded templates)

### Approach: wrapper function (keeps LoadSkills unchanged)
New file `internal/skill/loader_unified.go`:
```go
func LoadSkillsUnified(
    fsPaths []string,
    dbSkills []store.UserSkill,
    curatedFS embed.FS,        // pass zero-value embed.FS to skip
    shellCfg config.ShellToolConfig,
    limits config.LimitsConfig,
) ([]SkillContent, map[string]tool.Tool, []ExecutableSkillDef, []error)
```
Internally:
1. Load curated from `curatedFS`
2. Call `LoadSkills(fsPaths, ...)` — FS skills override curated by name
3. Convert `dbSkills` to `SkillContent` + `ExecutableSkillDef` — DB overrides FS by name
4. Merge tools maps (DB skills can define tools too — unlikely, but architecturally sound)

`main.go` and `web_cmd.go` switch from `LoadSkills` to `LoadSkillsUnified`. The `UserSkillStore` is injected into `ServerDeps` for handlers to call.

### UserSkillStore interface (store.go)
```go
type UserSkill struct {
    ID             string
    Name           string
    Description    string
    Prose          string
    Executable     bool
    Model          string
    Provider       string
    ToolsAllowlist []string  // nil = inherit all; []string{} = no tools
    Budget         *BudgetJSON  // nil = unlimited
    Version        int
    Source         string    // "user" | "curated"
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

type UserSkillStore interface {
    ListUserSkills(ctx context.Context) ([]UserSkill, error)
    GetUserSkill(ctx context.Context, name string) (*UserSkill, error)
    CreateUserSkill(ctx context.Context, skill UserSkill) error
    UpdateUserSkill(ctx context.Context, skill UserSkill) error
    DeleteUserSkill(ctx context.Context, name string) error
}
```
`BudgetJSON` is a helper struct matching the DB JSON columns.

---

## 8. Hot-Reload Semantics

### New method: ReplaceExecutableSkills (hot_reload.go)
```go
func (a *Agent) ReplaceExecutableSkills(defs []skill.ExecutableSkillDef) {
    a.toolsMu.Lock()
    defer a.toolsMu.Unlock()
    // Remove all existing SubagentSpawnTools
    for name, t := range a.tools {
        if _, ok := t.(*SubagentSpawnTool); ok {
            delete(a.tools, name)
        }
    }
    // Re-register from fresh defs
    if a.subMgr == nil && len(defs) > 0 {
        a.subMgr = NewSubagentManager(a.bus, a.store)
        a.subMgr.installBusSubscription()
        a.subMgr.newChildAgent = a.makeChildAgentFn()
    }
    for _, def := range defs {
        def.ToolsAllowlist = filterKnownTools(def.ToolsAllowlist, a.tools)
        a.tools[def.Name] = &SubagentSpawnTool{def: def, manager: a.subMgr}
    }
}
```
CRUD handlers call this after every write: `s.deps.Agent.ReplaceExecutableSkills(newDefs)`.

### AgentReloader interface extension
`AgentReloader` in `server.go` currently declares `RegisterMCPServer`, `UnregisterMCPServer`, `ReplaceSkills`. Must add `ReplaceExecutableSkills(defs []skill.ExecutableSkillDef)`. `*Agent` already satisfies it by duck-typing; adding this method makes it explicit.

### Allowlist validation at write time
Handler validates `tools_allowlist` against `s.deps.Tools` (the registry at boot). Unknown names → 422 with list. Names added later via MCP hot-add are not in the registry at write time — this is acceptable; warn, do not block.

---

## 9. Frontend API Contract Requirements

### List response (GET /api/skills)
```json
{
  "skills": [
    {
      "name": "code-reviewer",
      "description": "Reviews code...",
      "executable": true,
      "model": "claude-haiku-4-5",
      "provider": "",
      "tools_allowlist": ["shell_exec"],
      "budget": {"max_cost_usd": 0.50, "max_turns": 20, "timeout_min": 10},
      "source": "user",
      "version": 1,
      "created_at": "2026-05-10T12:00:00Z",
      "updated_at": "2026-05-10T12:00:00Z"
    }
  ]
}
```

Key frontend rules:
- `budget: null` → render "Unlimited" warning badge on the skill card.
- `source: "curated"` → hide Edit / Delete buttons, show read-only indicator.
- `tools_allowlist: null` → "Inherits all parent tools" label; `[]` → "No tools allowed".
- Budget null + executable=true → show confirmation dialog before save.

---

## 10. Architectural Mapping — Packages to Touch

| Package / File | Action | Phase |
|---|---|---|
| `internal/store/migration.go` | Add `migrateV18()` | 2 |
| `internal/store/store.go` | `UserSkill` struct + `UserSkillStore` interface | 2 |
| `internal/store/sqlitestore.go` | Implement `UserSkillStore` methods | 2 |
| `internal/skill/parser.go` | Remove budget hard-error for executable | 5 |
| `internal/skill/loader_unified.go` (NEW) | `LoadSkillsUnified` wrapper | 4 |
| `internal/skill/curated/` (NEW dir) | Embedded `.md` templates | 6 |
| `internal/agent/subagent_manager.go` | Fix `Timeout==0` → `context.WithCancel` | 5 |
| `internal/agent/hot_reload.go` | Add `ReplaceExecutableSkills` | 3 |
| `internal/agent/agent.go` | Add `CancelSubagent(id)`, extend `AgentReloader` interface | 1/3 |
| `internal/web/server.go` | Extend `SubagentProvider` + `AgentReloader`, add routes | 1/3 |
| `internal/web/handler_subagents.go` | Add `handleSubagentCancel` | 1 |
| `internal/web/handler_skills.go` (NEW) | REST CRUD handlers | 3 |
| `cmd/daimon/main.go` | Switch to `LoadSkillsUnified`, inject `UserSkillStore` | 4 |
| `cmd/daimon/web_cmd.go` | Same as main.go | 4 |

---

## 11. Extension Points / Future Compatibility

- **Skill versioning**: `version` column already present. `parent_curated_id TEXT` column for tracking forks from curated templates can be added later.
- **Tags/categories**: Add `tags TEXT` (JSON array) to `user_skills` in a later migration for catalog organization.
- **Export/import**: `GET /api/skills/{name}/export` returns `.skill.md` reconstructed from DB; `POST /api/skills/import` parses uploaded file via `parseSkillContent`.
- **Marketplace**: `SkillsRegistryURL` in config already supports remote registry fetching via `SkillService.Add`. CRUD is the UI surface of the same infrastructure.

---

## 12. Risks

| Risk | Severity | Mitigation |
|---|---|---|
| `context.WithTimeout(ctx, 0)` = instant cancel for unlimited-budget subagents | CRITICAL | Phase 5 must branch on `Timeout==0` → `context.WithCancel` in `Spawn` |
| FS + DB name collision non-deterministic at startup | HIGH | `LoadSkillsUnified` enforces explicit precedence map; logs collision |
| `ReplaceExecutableSkills` removes ALL spawn tools (including FS-loaded at boot) | HIGH | Re-run full `LoadSkillsUnified` on every CRUD op and call with full fresh slice |
| Budget reversal contradicts archived subagents spec REQ-12 | HIGH | Update spec text in `sdd-spec` phase; document reversal explicitly |
| Provider `Config()` interface missing (W4) — cross-provider skills fall back to parent | MEDIUM | Phase 1 W4 item; child agents already gracefully inherit parent provider |
| Allowlist write-time validation rejects tools added later via MCP | LOW | Warn but do not block; document as advisory validation |
| Curated embed adds to binary size | LOW | ~40 KB for 10 templates; negligible |
| `SubagentProvider` interface change breaks existing test mocks | MEDIUM | Update `handler_subagents_test.go` mock when extending interface |

---

## 13. Recommendations for Propose — Implementation Order

### Phase 1 — Foundation (~230 LoC, ships first PR, no schema deps)
- W4: Add `Config() config.ProviderConfig` to `Provider` interface (or `ConfigurableProvider`); implement on all 5 concrete providers; use in `makeChildAgentFn`.
- Cancel endpoint: extend `SubagentProvider` + `Agent.CancelSubagent` + `handleSubagentCancel` + route.
- Testable: cancel endpoint test; provider Config() round-trip test.
- Gates: nothing (fully orthogonal).

### Phase 2 — Schema + Store (~200 LoC)
- Migration v18 + `UserSkill` struct + `UserSkillStore` interface + SQLiteStore impl + table-driven tests.
- Gates Phase 3.

### Phase 3 — REST CRUD (~350 LoC)
- `handler_skills.go` GET/POST/PUT/DELETE + route registration.
- `hot_reload.go` `ReplaceExecutableSkills` + `AgentReloader` extension.
- CRUD handler triggers `ReplaceExecutableSkills` + `ReplaceSkills` after each write.
- Integration tests.
- Gates Phase 4.

### Phase 4 — Loader Unification (~200 LoC)
- `loader_unified.go` wrapper + inject `UserSkillStore` into `main.go`/`web_cmd.go`.
- DB skills now available at boot.
- Gates Phase 5 (budget reversal only affects FS loader path).

### Phase 5 — Default Budget Reversal (~80 LoC)
- Remove parser hard-error for missing budget.
- Fix `Timeout==0` in `Spawn`.
- Update spec references.
- Regression tests: unlimited-budget subagent completes normally.

### Phase 6 — Curated Catalog (~300 LoC + templates)
- `internal/skill/curated/` + embed.FS + integrate into `LoadSkillsUnified`.
- Ship 2–5 initial templates.

---

## 14. Obstacles — What the Plan Doesn't Account For

1. **SubagentProvider interface breaking change**: adding `CancelSubagent` requires updating the mock in `handler_subagents_test.go`. Minor but easy to miss.

2. **ReplaceExecutableSkills vs boot-loaded FS skills**: The naive remove-all approach would kill FS-loaded spawn tools. Correct fix: after every CRUD write, re-run `LoadSkillsUnified` from all three sources (FS paths, fresh DB query, curated) and call `ReplaceExecutableSkills` with the full merged exec defs. This is the path of least surprise.

3. **Phase 1 truly orthogonal — confirmed**: `SubagentManager.Cancel` exists. The HTTP surface and provider Config() interface are purely additive. No migration needed. Can target `main` directly.

4. **SkillService vs UserSkillStore coexistence**: `SkillService` manages FS skills (config.yaml edits, disk writes). `UserSkillStore` manages DB skills. They operate independently with no refactor required in Phase 2–3.

5. **Budget JSON shape in DB**: Must serialize/deserialize `BudgetConfig` as `{"max_cost_usd":X,"max_turns":Y,"timeout_min":Z}`. The `timeout_min` field (integer minutes) aligns with the frontmatter convention; `BudgetConfig.Timeout` (time.Duration) is derived by multiplying by `time.Minute` during conversion.

6. **Out-of-scope items (confirmed excluded)**: multi-spawn batch_id grouping, sibling cancel, mid-turn budget gate, per-sub MCP isolation, `attribution_kind = "advisor_call"`, API rate-limit coordination.
