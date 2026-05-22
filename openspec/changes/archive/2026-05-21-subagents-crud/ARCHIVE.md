# Archive Report — Subagents CRUD

**Change**: `subagents-crud`
**Archived**: 2026-05-21
**Final phase outcome**: PASS WITH WARNINGS (verify-report)
**PRs**: #9–#14 (Phase 1 through Phase 6 + spec-gap fix), all merged into `main`

## Summary

`subagents-crud` adds REST CRUD + SQLite-backed user skill definitions, a unified 3-pass skill loader (curated → FS → DB), hot-reload of executable skills, optional budgets for executable skills, an embedded curated catalog of 5 subagent templates, and a cancel endpoint for running subagents. All 87 tasks across 6 phases shipped and verified; race detector clean.

Two notable mid-flight adjustments were authorized:

- **Budget reversal (REQ-12 / CONFIG-REQ-6)**: the `budget` block is now OPTIONAL on executable skills. A missing block yields `Budget = nil` (unlimited cost/turns, no timeout) and `Spawn` switches to `context.WithCancel` instead of `context.WithTimeout`.
- **Curated REST exposure (spec-gap fix in Phase 6)**: `GET /api/skills?source=curated` and `GET /api/skills/{name}` return curated entries from a boot-time snapshot; `?source=all` merges curated + DB with DB-wins precedence.

## Capabilities

### Modified

- `subagents` — `openspec/specs/subagents/spec.md`
  - REQ-12 reversed: budget block is OPTIONAL
  - Added REQ-16 (`Spawn` Timeout==0 uses `WithCancel`)
  - Added REQ-17 (`POST /api/subagents/{id}/cancel`)
  - Added REQ-18 (`Agent.CancelSubagent(id)` nil-safe delegate)
  - Added REQ-19 (`Agent.ReplaceExecutableSkills(defs)` hot-reload)
  - Added REQ-20 (`ConfigurableProvider` interface across all 5 providers)
- `config` — `openspec/specs/config/spec.md`
  - CONFIG-REQ-4 frontmatter table: `budget` column reclassified OPTIONAL
  - CONFIG-REQ-6 rewritten: missing `budget` no longer a load error
  - Added CONFIG-REQ-9: skill `source` field is metadata-only; curated REST writes return 403
- `output-store` — added OUTPUT-STORE-REQ-11 (`migrateV18` user_skills table) and OUTPUT-STORE-REQ-12 (full `UserSkillStore` interface)
- `agent-loop` — added AGENT-LOOP-REQ-7 (`LoadSkillsUnified` 3-pass merge) and AGENT-LOOP-REQ-8 (`reloadSkills()` helper called after every CRUD write)

## Migrations Shipped

- **v18** (`user_skills`): adds DB-backed skill definitions with `source` metadata (`"user"` | `"curated"`); JSON columns for budget + tools*allowlist; idempotent migration verified via `TestMigration_V18*\*`.

## Implementation Footprint

87 tasks across 6 phases, all green; 21/21 packages pass `go test`; race detector clean on `internal/agent/...`, `internal/skill/...`, `internal/web/...`, `cmd/daimon/...`.

Files created/modified (highlights):

- `internal/agent/hot_reload.go` — `ReplaceExecutableSkills`
- `internal/agent/subagent_manager.go` — `WithCancel` branch for `Timeout==0`
- `internal/skill/loader_unified.go` — `LoadSkillsUnified` (curated → FS → DB merge)
- `internal/skill/curated_embed.go` — `CuratedFS embed.FS` + `CuratedCatalog()` helper
- `internal/skill/curated/*.skill.md` — 5 embedded templates (researcher, summarizer, code-reviewer, email-drafter, meeting-notes)
- `internal/skill/parser.go` — removed hard-error on missing budget
- `internal/web/handler_skills.go` — full REST CRUD + curated fallback in list/get
- `internal/web/server.go` — `UserSkillStore` + `CuratedSkills` deps + cancel route
- `internal/store/{store,sqlitestore_userskills,migration}.go` — schema + interface + v18 migration
- `cmd/daimon/{main,web_cmd}.go` — boot wiring for `LoadSkillsUnified` + `CuratedSkills`

## Deviations (all authorized)

1. **Budget reversal** (Phase 5) — REQ-12 / CONFIG-REQ-6 flipped from MANDATORY to OPTIONAL. Delta spec, design.md §2.12, and implementation aligned.
2. **Curated REST exposure** (Phase 6 spec-gap fix) — added mid-Phase 6 in PR #14 after handlers had landed without curated data; backfilled with tests `TestHandleListSkills_SourceCurated_ReturnsCatalog`, `TestHandleGetSkill_NotInDB_ButInCurated_Returns200Curated`, `TestHandleListSkills_SourceAll_DBWinsCollision`.
3. **Allowlist asymmetry** (CONFIG-REQ-5) — REST writes return 422 on unknown allowlist tool; hot-reload via `ReplaceExecutableSkills` instead WARN-and-drops to support MCP hot-add. Both paths tested. Canonical CONFIG-REQ-5 retains its single-mode wording; documenting the split formally was tracked as **W-1** in the verify-report and is deferred to a follow-up change.

## Outstanding Follow-ups

- **W-1**: Update canonical `openspec/specs/config/spec.md` CONFIG-REQ-5 to document the REST-vs-hot-reload asymmetry authorized by `design.md §3`. Documentation-only; no code change required.
- **S-1**: Streamline `handleDeleteSkill` to avoid the fetch-then-delete pattern (minor concurrency cleanup, currently safe).
- **S-2**: Add an explicit ordering guarantee for `?source=all` to the spec for frontend predictability.

## Verification Snapshot

| Check                              | Result                |
| ---------------------------------- | --------------------- |
| `go build ./...`                   | PASS                  |
| `go vet ./...`                     | PASS                  |
| `go test -timeout 300s ./...`      | PASS — 21/21 packages |
| `go test -race` (changed packages) | PASS — 0 races        |
| Tasks tildated                     | 87 / 87 (100%)        |
| Scenarios with passing tests       | ~35 / ~35 (100%)      |

Full verify-report archived to engram at topic_key `sdd/subagents-crud/verify-report`.
