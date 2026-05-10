# Design: subagents-crud

**Change**: `subagents-crud`
**Date**: 2026-05-10
**Author**: sdd-design (Opus 4.7)
**artifact_store**: hybrid (engram topic `sdd/subagents-crud/design` + this file)
**Builds on**: `subagents` (PR #4/#5/#6, archived 2026-05-10)
**Reads**: proposal §1-§14, exploration §1-§14, spec deltas (subagents + output-store)

---

## 1. Architecture Overview

### 1.1 The pattern in one sentence

Wrap the existing FS-based `LoadSkills` with a **multi-source unifier** that merges curated `embed.FS` + filesystem paths + DB rows behind explicit precedence (DB > FS > Curated), expose CRUD over the DB source through a thin REST handler, and close the loop by hot-reloading the agent's spawn-tool registry on every write — without touching the existing FS path.

### 1.2 Layering

```
┌──────────────────────────────────────────────────────────────────────┐
│ Frontend (daimon-frontend; out of scope for this change)              │
└──────────────────────────────────────────────────────────────────────┘
                                 │ HTTP
┌──────────────────────────────────────────────────────────────────────┐
│ internal/web                                                          │
│  - handler_skills.go  (NEW: GET / POST / PUT / DELETE /api/skills)    │
│  - handler_subagents.go (EXTEND: POST /api/subagents/{id}/cancel)     │
│  - server.go         (EXTEND: SubagentProvider, AgentReloader, routes)│
└────────────────────────────┬──────────────────────┬──────────────────┘
                             │                      │
                             ▼                      ▼
┌──────────────────────────────────────────────────────────────────────┐
│ internal/agent                                                        │
│  - hot_reload.go  (EXTEND: ReplaceExecutableSkills)                   │
│  - agent.go       (EXTEND: CancelSubagent; reads ConfigurableProvider)│
│  - subagent_manager.go (FIX: Spawn Timeout==0 branch)                 │
└────────────────────────────┬──────────────────────┬──────────────────┘
                             │                      │
                             ▼                      ▼
┌──────────────────────────────────────┐  ┌─────────────────────────────┐
│ internal/skill                        │  │ internal/store              │
│  - loader.go (UNCHANGED)              │  │  - migration.go (NEW v18)   │
│  - parser.go (REMOVE budget hard-err) │  │  - store.go (NEW UserSkill, │
│  - loader_unified.go (NEW wrapper)    │  │     UserSkillStore)         │
│  - curated_embed.go (NEW //go:embed)  │  │  - sqlitestore.go           │
│  - curated/*.skill.md (NEW templates) │  │     (NEW 5 methods)         │
└──────────────────────────────────────┘  └─────────────────────────────┘
                                                    │
                                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│ internal/provider                                                     │
│  - provider.go (NEW: ConfigurableProvider interface, opt-in)          │
│  - anthropic.go / openai.go / openrouter.go / gemini.go / ollama.go   │
│    (NEW: each implements Config() config.ProviderConfig)              │
└──────────────────────────────────────────────────────────────────────┘
```

### 1.3 Sequence — POST /api/skills (create)

```
client            handler_skills           UserSkillStore     loader_unified         Agent
  │ POST /api/skills                                                                   │
  │ ┌─{name, prose, executable:true, budget:null, tools_allowlist:["shell_exec"]}─┐    │
  │ │                  │                                                          │    │
  │ ├─────────────────►│ validateUserSkill(payload, knownTools)                   │    │
  │ │                  │   - name regex ^[a-z][a-z0-9_-]*$ ≤ 64                   │    │
  │ │                  │   - prose ≤ 8 KB                                         │    │
  │ │                  │   - allowlist ⊆ s.deps.Tools (warn-not-block on misses)  │    │
  │ │                  │   - budget != nil → ≥1 positive field                    │    │
  │ │                  │                                                          │    │
  │ │                  ├──────────────────►│ CreateUserSkill(ctx, skill)          │    │
  │ │                  │                   │   INSERT INTO user_skills (...)      │    │
  │ │                  │                   │   ON UNIQUE(name) → ErrNameConflict  │    │
  │ │                  │◄──────────────────┤                                      │    │
  │ │                  │                                                          │    │
  │ │                  │ s.reloadSkills():                                        │    │
  │ │                  ├────────────────────────────────────►│ LoadSkillsUnified  │    │
  │ │                  │                                     │   (fsPaths,        │    │
  │ │                  │                                     │    ListUserSkills, │    │
  │ │                  │                                     │    curatedFS)      │    │
  │ │                  │                                     │  → merged set      │    │
  │ │                  │◄────────────────────────────────────┤                    │    │
  │ │                  │                                                          │    │
  │ │                  ├──────────────────────────────────────────────────────────►│ ReplaceExecutableSkills(execDefs)
  │ │                  │                                                          │    │   toolsMu.Lock
  │ │                  │                                                          │    │   delete *SubagentSpawnTool
  │ │                  │                                                          │    │   filterKnownTools + register
  │ │                  ├──────────────────────────────────────────────────────────►│ ReplaceSkills(prose, idx)
  │ │                  │                                                          │    │   skillsMu.Lock
  │ │                  │                                                          │    │   replace prose state
  │ │◄─201 Created (UserSkill JSON)                                                    │
```

The next time the user invokes the skill name as a tool from the parent agent, the `SubagentSpawnTool` is already registered — no restart. If a child Spawn is in flight at the moment of the write, it continues with the *previous* def (writes win for next spawn; in-flight spawn is bounded by its existing ctx).

### 1.4 How `LoadSkillsUnified` orchestrates

```
       ┌──────────────────────────┐
       │  curated embed.FS        │   (lowest precedence)
       │  internal/skill/curated/ │
       └─────────┬────────────────┘
                 │
                 ▼
       ┌──────────────────────────┐
       │  FS paths (cfg.Skills)   │   (overrides curated by name)
       │  LoadSkills(fsPaths,...)  │
       └─────────┬────────────────┘
                 │
                 ▼
       ┌──────────────────────────┐
       │  DB UserSkill rows       │   (highest — DB always wins)
       │  store.ListUserSkills()  │
       └─────────┬────────────────┘
                 │
                 ▼
       ┌──────────────────────────┐
       │  merged map[name]→entry  │
       │  3 outputs:              │
       │   - []SkillContent       │
       │   - map[string]tool.Tool │
       │   - []ExecutableSkillDef │
       │   - []error (warns)      │
       └──────────────────────────┘
```

Same-name collisions across sources do not error — they downgrade lower-precedence entries to a `slog.Warn` and the higher-precedence entry wins (last-writer-wins with explicit ordering: curated → fs → db).

---

## 2. Component Specifications

### 2.1 — `UserSkill` struct + `UserSkillStore` interface

**File**: `internal/store/store.go` (extend)

```go
// UserSkill is the persisted form of a user-defined or curated skill.
// Maps 1:1 to a user_skills row (migration v18). Source distinguishes
// editable rows ("user") from read-only seeded rows ("curated"); the
// store does not enforce immutability — that is the handler's job.
type UserSkill struct {
    ID             string       `json:"id"`              // UUID v4
    Name           string       `json:"name"`            // unique slug
    Description    string       `json:"description"`
    Prose          string       `json:"prose"`           // body / SystemAddendum
    Executable     bool         `json:"executable"`
    Model          string       `json:"model"`           // empty = inherit
    Provider       string       `json:"provider"`        // empty = inherit
    ToolsAllowlist []string     `json:"tools_allowlist"` // nil = inherit-all; [] = no tools
    Budget         *BudgetJSON  `json:"budget"`          // nil = unlimited
    Version        int          `json:"version"`         // defaults to 1
    Source         string       `json:"source"`          // "user" | "curated"
    CreatedAt      time.Time    `json:"created_at"`
    UpdatedAt      time.Time    `json:"updated_at"`
}

// BudgetJSON is the on-disk JSON shape of a skill budget.
// Distinct from skill.BudgetConfig (which carries time.Duration);
// conversion happens in LoadSkillsUnified.
type BudgetJSON struct {
    MaxCostUSD float64 `json:"max_cost_usd,omitempty"`
    MaxTurns   int     `json:"max_turns,omitempty"`
    TimeoutMin int     `json:"timeout_min,omitempty"` // minutes, integer
}

// ErrNameConflict is returned by CreateUserSkill / UpdateUserSkill when the
// requested name collides with an existing row. Handlers map this to HTTP 409.
var ErrNameConflict = errors.New("skill name already exists")

// UserSkillStore manages the user_skills table. Implementations: SQLiteStore.
// FileStore does NOT implement this interface — CRUD is SQLite-only.
type UserSkillStore interface {
    ListUserSkills(ctx context.Context) ([]UserSkill, error)
    GetUserSkill(ctx context.Context, name string) (*UserSkill, error)
    CreateUserSkill(ctx context.Context, skill UserSkill) error
    UpdateUserSkill(ctx context.Context, skill UserSkill) error
    DeleteUserSkill(ctx context.Context, name string) error
}
```

**Encoding rules**:

| Go value | DB column type | DB stored | Notes |
|---|---|---|---|
| `nil` `*BudgetJSON` | `budget TEXT` | SQL NULL | "unlimited" semantics |
| `&BudgetJSON{...}` | `budget TEXT` | JSON object `{"max_cost_usd":...}` | All zero-fields omitted via `omitempty` |
| `nil` `[]string` allowlist | `tools_allowlist TEXT` | SQL NULL | "inherit all parent tools" |
| `[]string{}` allowlist | `tools_allowlist TEXT` | `"[]"` (JSON empty array) | "no tools" — explicit lockdown |
| `[]string{"a","b"}` allowlist | `tools_allowlist TEXT` | `["a","b"]` | scanned as JSON array |

The nil/empty distinction is **load-bearing** for the spawn semantics: `filterParentTools` treats `len(allowlist)==0` as "inherit all". To preserve the lockdown intent we must persist the empty-array form distinctly from NULL. JSON `[]` round-trips through `database/sql.NullString` cleanly.

### 2.2 — Migration v18

**File**: `internal/store/migration.go` (extend)

```sql
CREATE TABLE IF NOT EXISTS user_skills (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    description     TEXT NOT NULL DEFAULT '',
    prose           TEXT NOT NULL DEFAULT '',
    executable      INTEGER NOT NULL DEFAULT 0,
    model           TEXT NOT NULL DEFAULT '',
    provider        TEXT NOT NULL DEFAULT '',
    tools_allowlist TEXT,                              -- nullable JSON
    budget          TEXT,                              -- nullable JSON
    version         INTEGER NOT NULL DEFAULT 1,
    source          TEXT NOT NULL DEFAULT 'user',     -- 'user' | 'curated'
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_skills_source     ON user_skills(source);
CREATE INDEX IF NOT EXISTS idx_user_skills_executable ON user_skills(executable);
```

**Go migration template** (mirror v16/v17 pattern — single tx, idempotent, advance schema_version):

```go
// migrateV18 creates the user_skills table for user-defined and curated skill
// records exposed via the /api/skills CRUD surface. Idempotent: CREATE
// statements are guarded by IF NOT EXISTS.
//
// Advances schema_version to 18.
func (s *SQLiteStore) migrateV18() error {
    tx, err := s.db.Begin()
    if err != nil {
        return fmt.Errorf("begin: %w", err)
    }
    defer tx.Rollback() //nolint:errcheck

    stmts := []string{
        `CREATE TABLE IF NOT EXISTS user_skills (
            id              TEXT PRIMARY KEY,
            name            TEXT NOT NULL UNIQUE,
            description     TEXT NOT NULL DEFAULT '',
            prose           TEXT NOT NULL DEFAULT '',
            executable      INTEGER NOT NULL DEFAULT 0,
            model           TEXT NOT NULL DEFAULT '',
            provider        TEXT NOT NULL DEFAULT '',
            tools_allowlist TEXT,
            budget          TEXT,
            version         INTEGER NOT NULL DEFAULT 1,
            source          TEXT NOT NULL DEFAULT 'user',
            created_at      DATETIME NOT NULL,
            updated_at      DATETIME NOT NULL
        )`,
        `CREATE INDEX IF NOT EXISTS idx_user_skills_source ON user_skills(source)`,
        `CREATE INDEX IF NOT EXISTS idx_user_skills_executable ON user_skills(executable)`,
    }
    for _, stmt := range stmts {
        if _, err := tx.Exec(stmt); err != nil {
            return fmt.Errorf("user_skills schema: %w", err)
        }
    }

    if _, err := tx.Exec("UPDATE schema_version SET version = 18"); err != nil {
        return fmt.Errorf("updating schema version to 18: %w", err)
    }
    return tx.Commit()
}
```

Wire into `initSchemaVersioned` after the `version < 17` block:

```go
if version < 18 {
    if err := s.migrateV18(); err != nil {
        return fmt.Errorf("migration v18: %w", err)
    }
}
```

### 2.3 — sqlitestore implementation

**File**: `internal/store/sqlitestore.go` (extend; new file `sqlitestore_userskills.go` is acceptable for hygiene but not required).

#### 2.3.1 Helpers — JSON encoding for budget and allowlist

```go
// encodeAllowlist converts the in-memory allowlist to a sql.NullString.
// nil → NULL ("inherit all"); []string{} → "[]" ("no tools"); else JSON array.
func encodeAllowlist(a []string) (sql.NullString, error) {
    if a == nil {
        return sql.NullString{}, nil
    }
    raw, err := json.Marshal(a)
    if err != nil {
        return sql.NullString{}, fmt.Errorf("marshal allowlist: %w", err)
    }
    return sql.NullString{String: string(raw), Valid: true}, nil
}

// decodeAllowlist inverses encodeAllowlist.
func decodeAllowlist(ns sql.NullString) ([]string, error) {
    if !ns.Valid {
        return nil, nil
    }
    var out []string
    if err := json.Unmarshal([]byte(ns.String), &out); err != nil {
        return nil, fmt.Errorf("unmarshal allowlist: %w", err)
    }
    if out == nil {
        out = []string{} // distinguish "[]" from NULL
    }
    return out, nil
}

// encodeBudget converts *BudgetJSON to a sql.NullString.
func encodeBudget(b *BudgetJSON) (sql.NullString, error) {
    if b == nil {
        return sql.NullString{}, nil
    }
    raw, err := json.Marshal(b)
    if err != nil {
        return sql.NullString{}, fmt.Errorf("marshal budget: %w", err)
    }
    return sql.NullString{String: string(raw), Valid: true}, nil
}

// decodeBudget inverses encodeBudget.
func decodeBudget(ns sql.NullString) (*BudgetJSON, error) {
    if !ns.Valid {
        return nil, nil
    }
    var b BudgetJSON
    if err := json.Unmarshal([]byte(ns.String), &b); err != nil {
        return nil, fmt.Errorf("unmarshal budget: %w", err)
    }
    return &b, nil
}
```

#### 2.3.2 Method bodies (sketch — full code in apply phase)

```go
func (s *SQLiteStore) ListUserSkills(ctx context.Context) ([]UserSkill, error) {
    rows, err := s.db.QueryContext(ctx, `
        SELECT id, name, description, prose, executable, model, provider,
               tools_allowlist, budget, version, source, created_at, updated_at
        FROM user_skills
        ORDER BY name ASC`)
    // ... scan into []UserSkill via decodeAllowlist + decodeBudget
}

func (s *SQLiteStore) GetUserSkill(ctx context.Context, name string) (*UserSkill, error) {
    // QueryRowContext → scan single row; sql.ErrNoRows → ErrNotFound
}

func (s *SQLiteStore) CreateUserSkill(ctx context.Context, skill UserSkill) error {
    // INSERT inside a transaction; map UNIQUE(name) violation → ErrNameConflict.
    // Set CreatedAt = UpdatedAt = time.Now().UTC() if zero.
    // Validate skill.ID is a UUID; assign one if empty.
    //
    // SQLite UNIQUE constraint error string match (modernc.org/sqlite):
    //   "constraint failed: UNIQUE constraint failed: user_skills.name"
    // → wrap as ErrNameConflict.
}

func (s *SQLiteStore) UpdateUserSkill(ctx context.Context, skill UserSkill) error {
    // UPDATE WHERE name = ?; if RowsAffected == 0 → ErrNotFound.
    // Bump version + UpdatedAt.
    // If incoming name change collides with another row, return ErrNameConflict.
}

func (s *SQLiteStore) DeleteUserSkill(ctx context.Context, name string) error {
    // DELETE WHERE name = ?; RowsAffected == 0 → ErrNotFound.
}
```

**Transaction boundaries**: Reads are direct `QueryContext`. Writes (Create/Update/Delete) wrap in a single tx for atomicity — though single-statement writes are atomic in SQLite, the tx wrapper makes future composite writes (e.g., audit log row) trivial to add.

**Error mapping**:
- SQLite `UNIQUE constraint failed: user_skills.name` → `ErrNameConflict`
- `sql.ErrNoRows` → `ErrNotFound`
- All others → `fmt.Errorf("user_skills: %w", err)`

### 2.4 — `LoadSkillsUnified` wrapper

**File**: `internal/skill/loader_unified.go` (NEW)

```go
package skill

import (
    "context"
    "embed"
    "fmt"
    "log/slog"
    "time"

    "daimon/internal/config"
    "daimon/internal/store"
    "daimon/internal/tool"
)

// LoadSkillsUnified merges three sources into a single skill set with explicit
// precedence (DB > FS > Curated). It wraps LoadSkills (FS path); does not
// replace it. main.go and web_cmd.go switch to this entrypoint.
//
// curatedFS may be the zero value of embed.FS (skip curated load).
// dbStore may be nil (skip DB load — for tests or non-SQLite backends).
//
// Returns the same shape as LoadSkills: prose, tools map, exec defs, warns.
func LoadSkillsUnified(
    ctx context.Context,
    fsPaths []string,
    dbStore store.UserSkillStore,
    curatedFS embed.FS,
    shellCfg config.ShellToolConfig,
    limits config.LimitsConfig,
) ([]SkillContent, map[string]tool.Tool, []ExecutableSkillDef, []error) {
    var warns []error

    // index: name → (SkillContent, ExecutableSkillDef-or-nil, source)
    type entry struct {
        content SkillContent
        exec    *ExecutableSkillDef
        source  string // "curated" | "fs" | "db"
    }
    index := make(map[string]entry)

    // Pass 1: curated (lowest precedence).
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
        if existing, hit := index[c.Name]; hit && existing.source != "fs" {
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
        }
        for _, ds := range dbSkills {
            if existing, hit := index[ds.Name]; hit && existing.source != "db" {
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

    // Flatten the index back into 3 output slices preserving FS tool map.
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

// userSkillToParts converts a store.UserSkill into the loader's runtime types.
// When us.Executable is true, an ExecutableSkillDef is also returned.
// Budget == nil → BudgetConfig{} (zero values, all guards become no-ops in
// budgetMonitor; Spawn branches on Timeout > 0 — see REQ-16).
func userSkillToParts(us store.UserSkill) (SkillContent, *ExecutableSkillDef) {
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
    }
    if us.Budget != nil {
        sc.Budget = BudgetFrontmatter{
            MaxCostUSD: us.Budget.MaxCostUSD,
            MaxTurns:   us.Budget.MaxTurns,
            TimeoutMin: us.Budget.TimeoutMin,
        }
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
```

**Curated FS empty-value detection**: `embed.FS` is a value type. The zero value's `ReadDir(".")` returns an empty slice without error. `loadCurated` walks the FS; if no `.md` files match, it returns `(nil, nil, nil)` cleanly — no special-case needed at the call site.

### 2.5 — `ReplaceExecutableSkills` (hot_reload.go)

**File**: `internal/agent/hot_reload.go` (extend)

```go
// ReplaceExecutableSkills atomically replaces the agent's spawnable subagent
// definitions. Used by the /api/skills CRUD handler after every successful
// write. Removes ALL existing *SubagentSpawnTool entries from a.tools, then
// re-registers fresh ones from defs (with two-phase allowlist filtering).
//
// Lazy-initializes a.subMgr when defs is non-empty and no manager exists yet
// (covers the case where the agent was constructed without WithExecutableSkills
// but later gains its first executable skill via CRUD).
//
// Lock ordering: acquires only a.toolsMu (write). a.subMgr has its own internal
// mu.RWMutex which is acquired downstream by Spawn/Cancel — no overlap, no
// nested-lock scenarios.
//
// Idempotent: passing the same defs slice twice produces the same registry.
// Empty defs ([] or nil) leaves a.tools without any *SubagentSpawnTool entries
// but does NOT nil out a.subMgr (in-flight spawns continue to drain through it).
func (a *Agent) ReplaceExecutableSkills(defs []skill.ExecutableSkillDef) {
    a.toolsMu.Lock()
    defer a.toolsMu.Unlock()

    // Phase 1: remove existing spawn tools.
    for name, t := range a.tools {
        if _, ok := t.(*SubagentSpawnTool); ok {
            delete(a.tools, name)
        }
    }

    // Phase 2: lazy-init subMgr on first registration.
    if a.subMgr == nil && len(defs) > 0 {
        a.subMgr = NewSubagentManager(a.bus, a.store)
        a.subMgr.installBusSubscription()
        a.subMgr.newChildAgent = a.makeChildAgentFn()
    }

    // Phase 3: re-register from fresh defs (two-phase allowlist filter).
    for _, def := range defs {
        def.ToolsAllowlist = filterKnownTools(def.ToolsAllowlist, a.tools)
        if _, exists := a.tools[def.Name]; exists {
            slog.Warn("hot_reload: subagent tool name collides; subagent wins", "name", def.Name)
        }
        a.tools[def.Name] = &SubagentSpawnTool{def: def, manager: a.subMgr}
    }

    slog.Info("hot_reload: executable skills replaced",
        "count", len(defs),
        "total_tools", len(a.tools))
}
```

**Test seam**: Tests build an `*Agent` via the existing `NewSubagentManager` test pattern (newTestManager pattern in `subagent_manager_test.go`), then call `ReplaceExecutableSkills` directly. Verify by inspecting `a.tools` — count `*SubagentSpawnTool` entries, confirm names match. No need to spin up a real LLM.

### 2.6 — `CancelSubagent` (agent.go)

**File**: `internal/agent/agent.go` (extend)

```go
// CancelSubagent cancels a single running subagent by ID. Idempotent — calling
// twice on the same ID is safe (the second call is a no-op once the first
// fired). Returns nil when the agent has no SubagentManager (no executable
// skills loaded). Returns an error from SubagentManager.Cancel when the ID is
// not registered.
//
// Satisfies web.SubagentProvider.CancelSubagent.
func (a *Agent) CancelSubagent(id string) error {
    if a.subMgr == nil {
        // No executable skills loaded → nothing to cancel.
        // Returning nil rather than an error keeps the handler simple: it can
        // distinguish "unknown ID" from "no manager" via the second branch
        // (Cancel returns ErrNotFound-shaped errors from the manager).
        return nil
    }
    return a.subMgr.Cancel(id)
}
```

**Nil-safety**: First branch handles the case where `WithExecutableSkills` was never called and `ReplaceExecutableSkills` has never been called with a non-empty slice. Once the manager exists, it stays — even if all skills are deleted.

**Error path**: `SubagentManager.Cancel` returns `fmt.Errorf("subagent %q not found", subID)`. Handler maps any non-nil error to HTTP 404 (per spec REQ-17 — the only failure mode in V1 is "not found").

### 2.7 — `ConfigurableProvider` interface

**File**: `internal/provider/provider.go` (extend)

```go
// ConfigurableProvider is implemented by providers that can return their
// resolved config. Used by makeChildAgentFn to inherit credentials when a
// subagent's skill declares a different provider type than the parent.
//
// This is an OPT-IN interface (additive). Providers that do not implement it
// degrade gracefully: child agents fall back to the parent's provider instance
// instead of constructing a fresh one with inherited credentials.
//
// In V2 (EP-4) this method may be hoisted into the base Provider interface
// once all five providers have shipped Config().
type ConfigurableProvider interface {
    Config() config.ProviderConfig
}
```

**Per-provider implementations** (additive, single-method each):

| File | Receiver | Field name | Already present? |
|---|---|---|---|
| `anthropic.go` | `*AnthropicProvider` | `p.config` | Yes (line 76) |
| `gemini.go` | `*GeminiProvider` | `p.config` | Yes (line 19) |
| `openrouter.go` | `*OpenRouterProvider` | `p.config` | Yes (line 116) |
| `openai.go` | `*OpenAIProvider` | none — flat fields | **No — must add** |
| `ollama.go` | `*OllamaProvider` | embeds `*OpenAIProvider` | Inherits via embed once OpenAI ships |

```go
// Anthropic / Gemini / OpenRouter — direct return:
func (p *AnthropicProvider) Config() config.ProviderConfig { return p.config }
func (p *GeminiProvider) Config() config.ProviderConfig    { return p.config }
func (p *OpenRouterProvider) Config() config.ProviderConfig { return p.config }
```

**OpenAI gets a new field**:

```go
type OpenAIProvider struct {
    config         config.ProviderConfig // ADD: store the original config for Config()
    baseURL        string
    apiKey         string
    // ... rest unchanged
}

// In NewOpenAIProvider:
p := &OpenAIProvider{
    config:     cfg,           // ADD
    baseURL:    baseURL,
    apiKey:     cfg.APIKey,
    // ... rest unchanged
}

func (p *OpenAIProvider) Config() config.ProviderConfig { return p.config }
```

**Ollama inherits**: `OllamaProvider` embeds `*OpenAIProvider` via composition. Once OpenAI implements `Config()`, Ollama satisfies `ConfigurableProvider` automatically through method promotion. No additional work.

**`makeChildAgentFn` change**: The closure already does inheritance via an inline `type configurer interface{ Config() config.ProviderConfig }` (see `agent.go:558`). Replace that ad-hoc local interface with the canonical `provider.ConfigurableProvider`:

```go
// in providerConfigForSkill (agent.go:551)
if pc, ok := parent.(provider.ConfigurableProvider); ok {
    parentCfg := pc.Config()
    // ... rest unchanged
}
```

This is purely a cleanup — the runtime behavior is identical to today's local-interface flavor. The win is that `provider.ConfigurableProvider` becomes a documented, importable contract that handlers, tests, and future code can reference by name.

### 2.8 — REST handlers (`handler_skills.go`)

**File**: `internal/web/handler_skills.go` (NEW)

#### 2.8.1 Wire shape (request + response)

```go
// userSkillReq is the POST/PUT JSON shape. budget and tools_allowlist may be
// JSON null to express "unlimited" / "inherit-all" respectively.
type userSkillReq struct {
    Name           string             `json:"name"`
    Description    string             `json:"description,omitempty"`
    Prose          string             `json:"prose"`
    Executable     bool               `json:"executable"`
    Model          string             `json:"model,omitempty"`
    Provider       string             `json:"provider,omitempty"`
    ToolsAllowlist *[]string          `json:"tools_allowlist,omitempty"` // ptr distinguishes nil vs []
    Budget         *store.BudgetJSON  `json:"budget,omitempty"`
    Version        int                `json:"version,omitempty"`
}

// userSkillResp wraps store.UserSkill for the response envelope (adds
// computed source label for curated rows; no transformation otherwise).
type userSkillResp struct {
    *store.UserSkill
}

// listSkillsResp is the GET /api/skills envelope.
type listSkillsResp struct {
    Skills []store.UserSkill `json:"skills"`
}
```

The `*[]string` for `ToolsAllowlist` is critical: `*nil` → field omitted in JSON; `&[]string{}` → `"tools_allowlist": []` (lockdown). Without the pointer indirection, `nil` and `[]` would be indistinguishable post-decode.

#### 2.8.2 Handler signatures

```go
func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request)   // GET /api/skills
func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request)     // GET /api/skills/{name}
func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request)  // POST /api/skills
func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request)  // PUT /api/skills/{name}
func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request)  // DELETE /api/skills/{name}
```

#### 2.8.3 Validation pipeline

```go
var skillNameRE = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func validateUserSkill(req userSkillReq, knownTools map[string]tool.Tool) (httpStatus int, errs []string) {
    if !skillNameRE.MatchString(req.Name) || len(req.Name) > 64 {
        errs = append(errs, "name must match ^[a-z][a-z0-9_-]*$ and be ≤ 64 chars")
    }
    if len(req.Prose) > 8*1024 {
        errs = append(errs, "prose must be ≤ 8 KB")
    }
    if len(req.Description) > 8*1024 {
        errs = append(errs, "description must be ≤ 8 KB")
    }
    if req.Budget != nil {
        if req.Budget.MaxCostUSD <= 0 && req.Budget.MaxTurns <= 0 && req.Budget.TimeoutMin <= 0 {
            errs = append(errs, "budget: at least one of max_cost_usd, max_turns, timeout_min must be > 0")
        }
    }
    if req.ToolsAllowlist != nil {
        for _, name := range *req.ToolsAllowlist {
            if _, ok := knownTools[name]; !ok {
                // WARN, not block — tools may be added later via MCP hot-add (per
                // exploration §8 + risk row "LOW"). Surface as 200/201 with a
                // soft warning header.
                slog.Warn("user_skill: allowlist references unknown tool", "skill", req.Name, "tool", name)
            }
        }
    }
    if len(errs) > 0 {
        return http.StatusUnprocessableEntity, errs
    }
    return 0, nil
}
```

The "warn-not-block" decision for unknown allowlist names is documented in exploration §8 ("Allowlist write-time validation rejects tools added later via MCP" → mitigation: warn). It matters operationally because MCP servers can be added at runtime via the hot-add path; a strict block would force users to redo skill setup after every MCP add.

#### 2.8.4 Hot-reload trigger

After every successful write, the handler must call a small private helper:

```go
// reloadSkills re-runs LoadSkillsUnified and pushes the merged set into the
// running agent. Called after every successful CRUD write to /api/skills.
// Idempotent: safe to call when no agent is wired (returns silently).
func (s *Server) reloadSkills(ctx context.Context) {
    if s.deps.Agent == nil {
        return
    }
    contents, _, execs, warns := skill.LoadSkillsUnified(
        ctx,
        s.config().Skills,
        s.deps.UserSkillStore, // NEW field on ServerDeps (see §2.8.5)
        skill.CuratedFS,       // NEW exported var from curated_embed.go (see §2.10)
        s.config().ShellTool,
        s.config().Limits,
    )
    for _, w := range warns {
        slog.Warn("reloadSkills warning", "error", w)
    }
    s.deps.Agent.ReplaceExecutableSkills(execs)
    autoload, idx := agent.InitSkillInjection(contents, s.config().Limits.MaxContextTokens)
    s.deps.Agent.ReplaceSkills(autoload, idx)
}
```

#### 2.8.5 ServerDeps + AgentReloader changes

Add to `ServerDeps`:
```go
UserSkillStore store.UserSkillStore // NEW; nil disables the /api/skills CRUD surface
```

Extend `AgentReloader`:
```go
type AgentReloader interface {
    RegisterMCPServer(serverName string, tools map[string]tool.Tool, caller interface{ Close() error })
    UnregisterMCPServer(serverName string) error
    ReplaceSkills(skills []skill.SkillContent, idx skill.SkillIndex)
    ReplaceExecutableSkills(defs []skill.ExecutableSkillDef) // NEW
}
```

Extend `SubagentProvider`:
```go
type SubagentProvider interface {
    ActiveSubagents() []agent.SubagentStatus
    SubagentBus() notify.Bus
    CancelSubagent(id string) error // NEW
}
```

#### 2.8.6 Error response shapes

| Condition | HTTP | Body |
|---|---|---|
| Validation failure | 422 | `{"errors":["msg1","msg2"]}` |
| Name already exists (POST or PUT-rename) | 409 | `{"error":"name 'foo' already exists"}` |
| Not found (GET/PUT/DELETE on missing name) | 404 | `{"error":"skill 'foo' not found"}` |
| Curated row write attempt (PUT/DELETE) | 403 | `{"error":"curated skills are read-only"}` |
| Body too large / malformed JSON | 400 | `{"error":"invalid JSON"}` |
| Internal store error | 500 | `{"error":"internal error"}` (log full err with `slog.Error`) |
| Successful create | 201 | full `UserSkill` JSON |
| Successful update / get | 200 | full `UserSkill` JSON |
| Successful delete | 204 | empty body |

Curated check: at the top of PUT/DELETE handlers, after `GetUserSkill`, if `existing.Source == "curated"` → 403.

### 2.9 — REST handler `POST /api/subagents/{id}/cancel`

**File**: `internal/web/handler_subagents.go` (extend)

```go
// handleSubagentCancel cancels a running subagent identified by path param {id}.
// Per spec REQ-17:
//   - 200 + {"cancelled":true} when the cancel was issued
//   - 404 when no SubagentProvider is wired or the ID is unknown
//   - 405 when the method is not POST (handled by the route mux)
func (s *Server) handleSubagentCancel(w http.ResponseWriter, r *http.Request) {
    id := r.PathValue("id") // Go 1.22+ ServeMux path param
    if id == "" {
        http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
        return
    }
    if s.deps.SubagentProvider == nil {
        http.Error(w, `{"error":"subagent not found"}`, http.StatusNotFound)
        return
    }
    if err := s.deps.SubagentProvider.CancelSubagent(id); err != nil {
        // SubagentManager.Cancel returns "subagent %q not found" for unknown
        // IDs. There is no other failure mode in V1.
        http.Error(w, `{"error":"subagent not found"}`, http.StatusNotFound)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    _ = json.NewEncoder(w).Encode(map[string]bool{"cancelled": true})
}
```

**Route registration** in `server.go` `routes()` (mutating endpoint, wrap with `requireOriginIfCrossOrigin`):

```go
s.mux.Handle("POST /api/subagents/{id}/cancel",
    requireOriginIfCrossOrigin(ao, http.HandlerFunc(s.handleSubagentCancel)))
```

### 2.10 — Curated catalog

#### 2.10.1 Directory layout

```
internal/skill/
├── curated/
│   ├── code-reviewer.skill.md
│   ├── email-drafter.skill.md
│   ├── meeting-notes.skill.md
│   ├── researcher.skill.md
│   └── summarizer.skill.md
├── curated_embed.go        # NEW: //go:embed directive + helper
├── loader.go               # UNCHANGED
├── loader_unified.go       # NEW
├── parser.go               # MODIFIED (budget hard-error removed)
└── skill.go                # UNCHANGED
```

#### 2.10.2 Embed directive

**File**: `internal/skill/curated_embed.go` (NEW; isolated from loader_unified.go for hygiene — pulls only the embed dependency, easy to grep + own)

```go
package skill

import "embed"

// CuratedFS holds the curated skill templates shipped with the daimon binary.
// Updated by re-releasing daimon. Files must end in `.skill.md` and live in
// the curated/ subdirectory.
//
//go:embed curated/*.skill.md
var CuratedFS embed.FS
```

#### 2.10.3 Initial templates

Five generic skills, each ≤ 4 KB. Sample shape (`code-reviewer.skill.md`):

```markdown
---
name: code-reviewer
description: Reviews a diff or file for bugs, style, and clarity issues.
executable: true
model: ""
provider: ""
tools_allowlist: []
budget:
  max_cost_usd: 0.50
  max_turns: 20
  timeout_min: 10
---

You are a senior code reviewer. Given a code change or file, identify:
- Bugs (logic errors, race conditions, null derefs)
- Style issues (naming, structure, idioms for the detected language)
- Clarity issues (confusing names, missing docstrings, dead code)

Return a numbered list of issues, each with severity (critical/major/minor)
and a one-line suggested fix. Do not rewrite the code unless asked.
```

The other four (`researcher.skill.md`, `summarizer.skill.md`, `email-drafter.skill.md`, `meeting-notes.skill.md`) follow the same shape with task-specific prose. All ship with `budget: defaults`-equivalent values (0.50 / 20 / 10) so curated installs work out of the box without per-skill UI tuning.

#### 2.10.4 `loadCurated` helper

```go
// loadCurated walks curatedFS and parses each .skill.md file via parseSkillContent.
// Returns SkillContents and ExecutableSkillDefs as if they were FS-loaded, but
// tagged with source="curated" via the surrounding LoadSkillsUnified entry.
//
// Returns (nil, nil, nil) when curatedFS is the zero value (no embed).
func loadCurated(
    curatedFS embed.FS,
    shellCfg config.ShellToolConfig,
    limits config.LimitsConfig,
) ([]SkillContent, []ExecutableSkillDef, []error) {
    entries, err := curatedFS.ReadDir("curated")
    if err != nil {
        // Zero-value embed.FS or empty directory → no curated load.
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
        c, _, errs := parseSkillContent("curated/"+e.Name(), string(data))
        warns = append(warns, errs...)
        contents = append(contents, c)
        if c.Executable && len(errs) == 0 {
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
```

The curated path does NOT register `tool.Tool` entries (no shell-tool blocks in curated templates) — only prose and executable defs. This is by design: tool definitions are local to the user's environment.

### 2.11 — `Spawn` Timeout==0 fix

**File**: `internal/agent/subagent_manager.go` (modify line 233)

```go
// BEFORE (current line 233):
subCtx, cancel := context.WithTimeout(ctx, def.Budget.Timeout)

// AFTER:
var subCtx context.Context
var cancel context.CancelFunc
if def.Budget.Timeout > 0 {
    subCtx, cancel = context.WithTimeout(ctx, def.Budget.Timeout)
} else {
    subCtx, cancel = context.WithCancel(ctx)
}
```

**Affected REQ**: subagents/REQ-16 (Spawn Timeout==0 produces no instant cancel).

**Race detector implications**: None. `context.WithCancel` and `context.WithTimeout` produce contexts with the same surface area; both are goroutine-safe; both register parent-cancellation propagation identically. The branch happens before any goroutine is spawned.

**Why this works for budgetMonitor**: The existing guards (`subagent_manager.go:375-378`) all check `> 0` before evaluating the cap — `BudgetConfig{}` zero values naturally mean "no limit":

```go
softHit := !rec.softWarned && rec.budget.MaxCostUSD > 0 && rec.cost >= 0.8*rec.budget.MaxCostUSD
hardCost := rec.budget.MaxCostUSD > 0 && rec.cost >= rec.budget.MaxCostUSD
hardTurns := rec.budget.MaxTurns > 0 && rec.turns >= rec.budget.MaxTurns
```

No change required to `budgetMonitor`.

### 2.12 — Parser change for budget reversal

**File**: `internal/skill/parser.go` (lines 257-263)

```go
// BEFORE:
// Validate executable constraints.
if fm.Executable {
    // Missing budget block is a hard error.
    if fm.Budget.IsZero() {
        errs = append(errs, fmt.Errorf("skill %q: executable skills must declare a budget block (or 'budget: defaults')", path))
    }
}

// AFTER:
// (block removed entirely. Executable skills MAY omit `budget`; the loader
// produces ExecutableSkillDef with Budget = BudgetConfig{} which the runtime
// treats as "unlimited". See subagents/REQ-12 and REQ-16.)
```

No other validation to add. The `BudgetFrontmatter{}` zero value flows through the loader unchanged; `loader.go:80` constructs `BudgetConfig{0, 0, 0}` from it; `subagent_manager.go` (after §2.11 fix) handles the 0-timeout path correctly.

---

## 3. Cross-Cutting Concerns

### 3.1 Lock ordering

Only one lock is acquired by CRUD writes inside the agent package: `a.toolsMu` (write) inside `ReplaceExecutableSkills`. The chain looks like:

```
handler_skills (no agent lock)
  → store.CreateUserSkill (sql.DB internal locking only)
  → loader_unified (no agent lock; reads dbStore via context)
  → agent.ReplaceExecutableSkills [a.toolsMu.Lock]
  → agent.ReplaceSkills [a.skillsMu.Lock]   (existing, already correct)
```

`a.subMgr` has its own internal `mu sync.RWMutex` used by `Spawn`/`Cancel`/`Active`/`All`/`Get`. There is **no overlap** between `toolsMu` and `subMgr.mu`: `ReplaceExecutableSkills` touches `subMgr` only when constructing a fresh `*SubagentManager` (at which point no other code holds its lock yet). No nested-lock scenario exists. No deadlock potential.

### 3.2 Hot-reload race

A user may submit `PUT /api/skills/foo` while the parent agent is mid-Spawn for the same skill `foo`. The race window is bounded:

- The `Spawn` flow reads its `def` from `*SubagentSpawnTool.def` (a value copy), then proceeds. Once `Spawn` is past the def-read, the in-flight subagent uses the **old** def for its entire lifetime.
- The CRUD write registers a **new** `*SubagentSpawnTool` under the same name. The next call to that tool name picks up the new def.

**Design intent**: writes win for *next spawn*; in-flight spawns continue with the old def. Documented as accepted — V2 may add per-skill version pins if users need reproducibility across long-running spawns.

### 3.3 Curated shadow logic

When a user creates `user_skill{name:"researcher"}` and the curated catalog also ships `researcher.skill.md`, the loader's pass-3 (DB) overwrites the pass-1 (curated) entry in the merge index. The curated row becomes invisible to the loader and to `GET /api/skills` (which lists the merged set with `source` reflecting the winner — `"user"`).

**Design intent**: user customizations always win. A user can "uncustomize" by deleting their row; the curated default reappears on the next reload (handler triggers `reloadSkills()` after DELETE).

The user_skills table has a `source` column with rows where `source="curated"`. **However, the V1 design does NOT seed curated rows into the DB** (per exploration §4 Option A — embed-only). The `source="curated"` column value exists for forward-compat with EP-2 (V2 may pull curated from a remote registry and seed them into DB for staleness tracking). In V1 the column is always `"user"` for any inserted row.

### 3.4 Backward compat

- Existing `cfg.Skills` FS files load unchanged: Phase 4 *wraps* `LoadSkills` (does not replace it). Pass 2 of `LoadSkillsUnified` is a verbatim call to `LoadSkills(fsPaths, ...)`.
- `MockSubagentProvider` in `handler_subagents_test.go` must add `CancelSubagent(id string) error` method to satisfy the extended interface — a one-line addition (return nil or a stub error). This is a Phase-1 pre-task per exploration §14 obstacle #1.
- The `agent.AgentReloader` interface gains `ReplaceExecutableSkills` — `*agent.Agent` satisfies it via duck typing once the method exists. No other implementers exist in the codebase (verified by grep — only mock implementations in tests, which need a one-liner stub).

### 3.5 Auth on CRUD

All mutating endpoints (POST/PUT/DELETE on `/api/skills` + `POST /api/subagents/{id}/cancel`) wrap with `requireOriginIfCrossOrigin(ao, ...)` per the established server.go pattern. GET endpoints (`/api/skills`, `/api/skills/{name}`) use `s.mux.HandleFunc` (no origin check; auth middleware runs globally per server.go:150).

### 3.6 Budget JSON shape: DB vs runtime

| Layer | Type | Fields | Units |
|---|---|---|---|
| Wire (REST + DB) | `store.BudgetJSON` | `MaxCostUSD float64; MaxTurns int; TimeoutMin int` | dollars / turns / **minutes** |
| Runtime (subagent_manager) | `skill.BudgetConfig` | `MaxCostUSD float64; MaxTurns int; Timeout time.Duration` | dollars / turns / **time.Duration** |

**Conversion**: `Timeout = time.Duration(BudgetJSON.TimeoutMin) * time.Minute`. Performed inside `userSkillToParts` (loader_unified.go) and inside the existing `LoadSkills` (loader.go:83). Both paths already converge on the same `BudgetConfig{}` shape — no divergence.

The frontmatter parser uses an intermediate `BudgetFrontmatter` (also minutes), which `LoadSkills` already converts. Behavior is identical between FS and DB sources.

---

## 4. Trade-Offs Considered

### 4.1 ConfigurableProvider as opt-in vs hoisting into Provider

| Choice | Pro | Con |
|---|---|---|
| **Opt-in (chosen)** | Smaller blast radius — no changes to test mocks of `Provider` interface; gradual adoption | One extra type assertion at the call site |
| Hoist into `Provider` | Mandatory contract; no runtime check | Breaks every test mock that implements `Provider`; bigger PR; opts in non-providers (mocks, fallbacks) that don't have a meaningful config |

Chose opt-in because the `agent.go` call site already uses a local interface assertion; promoting it to a package-level interface is purely a documentation win without any compatibility cost.

### 4.2 embed.FS for curated vs DB seed at migration v18

| Choice | Pro | Con |
|---|---|---|
| **embed.FS (chosen)** | Mutation-safe; ships with binary; updates on release; zero migration risk | Updates require binary release |
| DB seed at v18 | Single source at runtime; users could in theory edit | Mutation risk; migration coupling; how do we add a new curated skill in v19 — re-seed only new ones? |

Chose embed because the curated catalog is *vendor content*, not user content. Vendor content shipping with the binary is the established pattern for `internal/web/mcp_skills/*.md` (already in the codebase).

### 4.3 Separate UserSkillStore interface vs extending Store

| Choice | Pro | Con |
|---|---|---|
| **Separate interface (chosen — the spec calls for this; simpler to inject)** | Sub-interface composition; FileStore doesn't need to stub these out | One extra field in ServerDeps |
| Extend `Store` with 5 new methods | Single injection point | FileStore (no SQLite) would need stubs returning `ErrNotImplemented`; pollutes Store with SQLite-only concerns |

Chose separate. Aligns with existing pattern (`CronStore`, `OutputStore`, `MediaStore`, `ConvPruneStore`, `MemoryStore` — all sub-interfaces only `*SQLiteStore` implements). FileStore stays clean.

### 4.4 Hot-reload via full re-merge vs incremental

| Choice | Pro | Con |
|---|---|---|
| **Full re-merge (chosen)** | Simple; single code path covers create/update/delete; no edge cases around "partial write seen" | Re-runs FS load + DB load on every CRUD write |
| Incremental (apply only delta) | Cheaper (no FS re-read) | Three flow shapes (insert, update, delete); cache invalidation; can drift from disk truth if FS changes externally |

Chose full re-merge for V1 — see EP-5 in proposal §7 for V2 incremental path. CRUD volume is human-scale (clicks per minute, not per second); the cost of a full re-load is negligible compared to a single LLM call.

### 4.5 DB > FS > Curated precedence vs reverse

User customizations always win — a user editing a curated skill (by creating a same-name user_skill) expects their version to take effect immediately. The reverse precedence (Curated wins) would silently override user changes, which violates principle of least surprise.

### 4.6 Curated skills shadowable by user vs not

| Choice | Pro | Con |
|---|---|---|
| **Shadowable (chosen)** | User agency; no special "fork" UI required | A user could shadow with a worse version |
| Not shadowable | Predictable curated baseline | Locks users into a vendor decision; users would file bugs asking for shadow capability |

Shadowable wins. If the user creates a worse version, they can delete it and the curated default reappears on next reload.

---

## 5. Testing Strategy

The change is implemented under Strict TDD Mode. Test order: write failing test first, watch it fail, write minimum code to pass, refactor.

### 5.1 Per-component unit tests

| Component | Test file | Approach |
|---|---|---|
| `store.UserSkillStore` impl | `internal/store/sqlitestore_userskills_test.go` | Table-driven CRUD with all field permutations (nil/zero/full budget × nil/[]/values allowlist) |
| Migration v18 | `internal/store/migration_v18_test.go` | Round-trip: NewSQLiteStore → check schema_version=18 → re-open → idempotent |
| `LoadSkillsUnified` | `internal/skill/loader_unified_test.go` | Merge precedence: empty/curated-only/fs-only/db-only/all-three; collision scenarios (same name in all 3) |
| `loadCurated` | `internal/skill/loader_unified_test.go` | Real CuratedFS load + zero-value FS skip |
| `Agent.ReplaceExecutableSkills` | `internal/agent/hot_reload_test.go` | Lock-held window + idempotency + lazy subMgr init + collision warn |
| `Agent.CancelSubagent` | `internal/agent/agent_cancel_test.go` | nil subMgr returns nil; unknown ID returns error from Cancel |
| `ConfigurableProvider` | `internal/provider/configurable_test.go` | Each of 5 providers returns a non-zero ProviderConfig matching the input cfg |
| HTTP handlers | `internal/web/handler_skills_test.go` | All status codes (200/201/204/400/403/404/409/422/500); curated read-only enforcement; mock UserSkillStore + mock AgentReloader |
| Cancel handler | `internal/web/handler_subagents_cancel_test.go` | 200/404 paths with mock provider |
| Spawn Timeout==0 regression | `internal/agent/subagent_manager_test.go` | Spawn def with `Budget.Timeout==0`; verify subagent context is NOT immediately Done |

### 5.2 Coverage targets

- `internal/store` user_skills paths: 90%+ (per golang-pro project standard)
- `internal/skill/loader_unified.go`: 90%+ (small surface, all branches reachable)
- `internal/web/handler_skills.go`: 80%+ (all status codes via table-driven)
- `internal/agent/hot_reload.go` new method: 85%+

### 5.3 Race detector

`go test -race ./internal/agent/... ./internal/web/...` must pass. The hot_reload test should explicitly issue concurrent `ReplaceExecutableSkills` + `Spawn` to validate the lock semantics described in §3.1-§3.2.

---

## 6. Open Design Risks

### 6.1 Curated skill name collision with MCP-installed skill

If a curated skill name (`code-reviewer`) collides with an MCP-installed skill bundled under `internal/web/mcp_skills/code-reviewer.md` and the user has installed it via `SkillService.Add`, the FS path (Pass 2 in `LoadSkillsUnified`) wins over curated. A user-created `user_skill` with the same name then wins over both.

The spec is silent on whether two different sources should produce a hard error vs a warn. We propose: **warn only, never error** — the established pattern from existing collision handling (`hot_reload.go:42`, `loader.go:119`). This is implemented in `LoadSkillsUnified` collision logging.

### 6.2 ReplaceExecutableSkills in-flight inconsistency

Inside the lock-held window in `ReplaceExecutableSkills`, `a.tools` briefly has *no* `*SubagentSpawnTool` entries between Phase 1 (delete) and Phase 3 (re-register). This window is ~microseconds and covered by the write lock — no other code can read `a.tools` during it because all readers acquire `a.toolsMu.RLock`.

**However**: the `processMessage` hot path acquires `RLock` once at the top of a turn (per agent.go:114-115 comment) and uses that snapshot for the duration of the turn. If a CRUD write fires *between* turns, the next turn observes the new state. If a CRUD write fires *during* a turn, the lock blocks the write until the turn's RLock is released. Bounded behavior in either case.

### 6.3 BudgetJSON schema evolution

If V2 adds a field (e.g., `MaxConcurrentSubs`) to `BudgetJSON`, existing DB rows have older JSON without the new key. `json.Unmarshal` populates missing keys with the zero value, which is the safe default for new caps (0 = no limit). Forward-compat is automatic.

If V2 *removes* a field (`MaxCostUSD`), existing DB rows still contain it; `json.Unmarshal` ignores unknown fields by default. Also safe.

The risky case is *renaming* a field. Mitigation: never rename — add a new field, deprecate the old one, write a migration to copy old → new on next read.

### 6.4 Curated skill prose may reference unavailable tools

A curated `code-reviewer.skill.md` may declare `tools_allowlist: ["shell_exec"]`, but the user has not enabled the shell tool. `filterKnownTools` (existing logic in agent.go:595) silently drops it with a `slog.Warn`. The curated skill spawns with no tools — degraded but not broken.

**UX flag**: the `GET /api/skills?source=curated` response should perhaps surface "uses tools that are not enabled on this install" so the frontend can warn the user. Out of scope for this change; flagged for the frontend repo.

---

## 7. Result Snapshot

After this change ships, the architecture supports:

1. **Hot-reload of any skill via REST**: no restart for create/update/delete.
2. **Out-of-the-box curated agents**: a fresh install has 5 spawnable agents.
3. **Optional budgets**: skill files (FS or DB) without a `budget` block load and spawn correctly without instant cancellation.
4. **Cross-provider subagent credentials**: any of the 5 providers can be the parent or child without re-asking the user for an API key.
5. **REST cancel**: the dashboard can cancel a stuck subagent without restarting the daimon process.
6. **Backward compatible**: all existing FS skills continue to load unchanged; existing tests continue to pass after one mock interface stub.

The seven dependency-bound phases (1 standalone + 2→3→4→5→6 chain) keep PR sizes ≤ 350 LoC each, satisfying the project's 400-line review budget.
