# Delta for Output Store (output-store)

This delta amends the canonical `openspec/specs/output-store/spec.md` with changes from the `subagents-crud` change.

---

## ADDED Requirements

### Requirement: OUTPUT-STORE-REQ-11 — Migration v18: `user_skills` table

The system SHALL apply migration v18 which creates the `user_skills` table and associated indexes. The schema_version MUST be updated to 18. No backfill is required (the table is brand-new with no existing rows).

```sql
CREATE TABLE IF NOT EXISTS user_skills (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    prose       TEXT NOT NULL DEFAULT '',
    executable  INTEGER NOT NULL DEFAULT 0,
    model       TEXT NOT NULL DEFAULT '',
    provider    TEXT NOT NULL DEFAULT '',
    tools_allowlist TEXT,
    budget      TEXT,
    version     INTEGER NOT NULL DEFAULT 1,
    source      TEXT NOT NULL DEFAULT 'user',
    created_at  DATETIME NOT NULL,
    updated_at  DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_user_skills_source ON user_skills(source);
CREATE INDEX IF NOT EXISTS idx_user_skills_executable ON user_skills(executable);
```

`tools_allowlist` is stored as a JSON array (or NULL). `budget` is stored as a JSON object (or NULL — NULL means unlimited).

#### Scenario: migration v18 applies cleanly on existing DB

- GIVEN a database at migration version 17 with existing rows in other tables
- WHEN migration v18 runs
- THEN the `user_skills` table exists with all specified columns
- AND the row count in `user_skills` is 0
- AND `schema_version` is 18
- AND both `idx_user_skills_source` and `idx_user_skills_executable` indexes exist

#### Scenario: migration v18 is idempotent

- GIVEN a database with migration v18 already applied
- WHEN migration v18 is run again (e.g., after a restart)
- THEN no error is returned
- AND no duplicate tables or indexes are created (`CREATE TABLE IF NOT EXISTS` and `CREATE INDEX IF NOT EXISTS` used)

---

### Requirement: OUTPUT-STORE-REQ-12 — `UserSkillStore` interface and sqlitestore implementation

The `Store` interface (or a `UserSkillStore` interface composed into `Store`) SHALL expose:

```go
ListUserSkills(ctx context.Context) ([]UserSkill, error)
GetUserSkill(ctx context.Context, name string) (*UserSkill, error)
CreateUserSkill(ctx context.Context, skill UserSkill) error
UpdateUserSkill(ctx context.Context, skill UserSkill) error
DeleteUserSkill(ctx context.Context, name string) error
```

The `UserSkill` struct MUST have fields matching the `user_skills` table columns. The `Budget` field MUST be `*BudgetJSON` (nil pointer = unlimited; serializes to SQL NULL). `ToolsAllowlist` MUST be `[]string` (nil or empty slice = inherit all parent tools; serializes to SQL NULL / empty JSON array respectively).

`GetUserSkill` MUST return a sentinel error (e.g., `ErrNotFound`) when no row matches the given name.

#### Scenario: Create and Get round-trip preserves all fields

- GIVEN a `UserSkill` with all fields populated, including a nil `Budget`
- WHEN `CreateUserSkill` is called followed by `GetUserSkill` with the same name
- THEN the returned struct has all fields identical to the original
- AND `Budget` is nil (round-trips as SQL NULL)

#### Scenario: Create and Get round-trip with explicit budget

- GIVEN a `UserSkill` with `Budget = &BudgetJSON{MaxCostUSD: 0.10, MaxTurns: 10, TimeoutMin: 5}`
- WHEN `CreateUserSkill` is called followed by `GetUserSkill`
- THEN the returned `Budget` matches the original values exactly

#### Scenario: GetUserSkill returns ErrNotFound for missing name

- GIVEN no `user_skill` row with name `"ghost"`
- WHEN `GetUserSkill(ctx, "ghost")` is called
- THEN the error is the sentinel `ErrNotFound` (or equivalent)
- AND the returned pointer is nil

#### Scenario: DeleteUserSkill removes the row

- GIVEN a `UserSkill` named `"researcher"` exists in the table
- WHEN `DeleteUserSkill(ctx, "researcher")` is called
- THEN the row is removed
- AND a subsequent `GetUserSkill(ctx, "researcher")` returns `ErrNotFound`

#### Scenario: ListUserSkills returns all rows ordered by name

- GIVEN three skills named `"coder"`, `"analyst"`, `"researcher"` exist
- WHEN `ListUserSkills(ctx)` is called
- THEN the result contains all three rows
- AND they are ordered `"analyst"`, `"coder"`, `"researcher"` (ascending by name)

#### Scenario: ToolsAllowlist nil round-trips as SQL NULL

- GIVEN a `UserSkill` with `ToolsAllowlist = nil`
- WHEN stored and retrieved
- THEN `ToolsAllowlist` is nil (not an empty slice)

#### Scenario: ToolsAllowlist empty slice round-trips correctly

- GIVEN a `UserSkill` with `ToolsAllowlist = []string{}`
- WHEN stored and retrieved
- THEN `ToolsAllowlist` is an empty (non-nil) slice

---

## Acceptance Criteria

- [ ] Migration v18 applies cleanly on a real database and round-trips idempotently
- [ ] All 5 `UserSkillStore` methods are covered by table-driven unit tests
- [ ] `Budget` JSON serializes and deserializes correctly: nil pointer round-trips as SQL NULL; non-nil pointer round-trips with all field values
- [ ] `ToolsAllowlist` nil vs empty-slice distinction is preserved through storage round-trip
