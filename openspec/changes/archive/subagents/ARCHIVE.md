# Archive Report — Subagents

**Change**: `subagents`
**Archived**: 2026-05-10
**Final phase outcome**: passed verification (Phase 1 + Phase 2 + Phase 3+4)
**PRs**: #4 (Phase 1), #5 (Phase 2), #6 (Phase 3+4) — chain stacked-to-main

## Summary

Subagents enables spawnable, specialized agent loops. The principal agent can delegate bounded tasks to subagents that run with independent provider/model selection, budget enforcement (cost/turns/timeout), and hierarchical cost attribution. Four implementation phases shipped: Phase 1 schema + migrations (v16/v17) + compactor guard, Phase 2 core runtime + lifecycle events, Phase 3 REST/WS visibility, Phase 4 soft warnings + batch_id + edge case coverage. All verification gates passed (4/4 phases). Three chained PRs remain open for human review; no merge blockages.

## Capabilities

### New
- `subagents` — full spec at `openspec/specs/subagents/spec.md` (15 REQs)

### Modified
- `agent-loop` — added REQs 5-6 (SubagentManager wiring, independent Agent instances)
- `output-store` — added REQs 5-10 (migrations v16+v17, three new Store methods, compactor guard)
- `config` — added REQs 4-8 (skill frontmatter parsing, budget validation, backward compat)

## Migrations Shipped

- **v16** (conversations): adds `parent_conv_id` + `status` columns with index; backfill `status='active'`; compactor guard active immediately
- **v17** (cost_records): adds `conv_id` + `parent_conv_id` + `attribution_kind` columns; backfill `conv_id=session_id`, `attribution_kind='self'`; two new indexes

## Implementation Footprint

**Phase 1** (Foundation):
- Files: 7 modified (migrations, store interface/impl, skill parsing, SubagentChannel)
- Lines: ~550 (struct extensions, SQL migrations, parsing logic)
- Tests: 22 tasks (all PASS, Strict TDD)

**Phase 2** (Core Runtime):
- Files: 5 new (SubagentManager, SubagentSpawnTool, event types, agent wiring)
- Lines: ~900 (manager lifecycle, spawn/cancel/budget, cost attribution, event emission)
- Tests: 20 tasks (all PASS, 2 deferred to later)

**Phase 2 Fixes** (CRITICAL findings + Phase 1/2 spec drift):
- 5 CRITICAL findings fixed (cost tree query, provider Config interface fallback, soft warning race, event order, depth limit validation)
- 2 SUGGESTION findings (W4: provider.Config interface not universal; V2 batch grouping comment)
- 0 blockers remaining

**Phase 3** (Visibility):
- Files: 2 new (REST handler, WS stream)
- Lines: ~250 (handler, channel buffer + drop-warn, WS marshaling)
- Tests: 5 tasks (all PASS)

**Phase 4** (Polish):
- Files: 5 modified (injectSoftWarning, batch_id UUID, edge case guards, test fixtures)
- Lines: ~200 (warning injection, batchID generation, race protections)
- Tests: 7 tasks + 5 fixtures (all PASS, Strict TDD clean)

**Total LoC changed**: ~1,900 (across 6 new files + 11 modified files)
**Test/Impl ratio**: 54 test tasks : 1,900 LoC ≈ 28 tests per 1,000 lines

## Verify Outcomes

| Phase | CRITICAL | WARNING | SUGGESTION | Race | Status |
|---|---|---|---|---|---|
| 1 | 0 | 1 (W1: spec text gap, fixed) | 0 | clean | pass |
| 2 (initial) | 2 | 3 | 2 | clean | fail-with-criticals |
| 2 (re-verify after fix) | 0 | 1 (W4: provider Config interface, follow-up) | 0 | clean | pass-with-warnings |
| 3+4 | 0 | 2 (REST field names + WS wire format) | 2 (soft warn scope, fixture comment) | clean | pass-with-warnings |

**Overall**: PASS-WITH-WARNINGS (0 CRITICAL final, 4 total WARNINGs documented as known, ready for PR review)

## Outstanding WARNINGs (Non-Blocking)

1. **W1 + fix**: Phase 1 spec text gap in REQ-3 scenario (fixed immediately)
2. **W4**: `provider.Config()` interface — none of the 5 built-in providers implement it; credential inheritance falls back to declarative config. Documented in code comment. Follow-up: V2 or future provider upgrade. Topic: `sdd/subagents/followups/provider-config-interface`
3. **WARNING-1**: REST response field names diverge from spec (e.g., `subagent_id` vs spec's `id`). Tests match implementation. Frontend panel is deferred; reconcile before that PR. Engram: `sdd/subagents/field-name-alignment`
4. **WARNING-2**: WS frame format deviates from design (uses `{"event": "...", "payload": {...}}` not `{"type": "..."}` as design specified). Implementation is cleaner. Update design docs during archive. Engram: same topic

## Open Follow-Ups (V2 / future)

- **EP-3 (soft warning scope)**: Only cost budget triggers 80% warning; turns/timeout do not. Deferred to V2 if desired.
- **EP-2 (batch_id grouping)**: Today `batchID = spawn_id` (1:1). V2 introduces real batch grouping for grouped spawns. Comment in code documents this. (Task 4.3 deferred 2 sub-items here.)
- **Provider credentials**: See W4 above.

## Historical Artifacts (preserved in archive folder)

- `proposal.md` — original intent, scope, extension points, dependencies, rollback plan
- `exploration.md` — pre-proposal codebase investigation (Phase 0)
- `design.md` — architecture decisions, tradeoffs, SubagentProvider interface, pseudocode
- `tasks.md` — phased task breakdown (4 phases × 4-7 tasks each) with REQ traceability
- `specs/subagents/spec.md` — delta spec as authored (merged into `openspec/specs/subagents/`)
- `specs/agent-loop/spec.md` — delta spec (merged into `openspec/specs/agent-loop/`)
- `specs/output-store/spec.md` — delta spec (merged into `openspec/specs/output-store/`)
- `specs/config/spec.md` — delta spec (merged into `openspec/specs/config/`)

## Artifacts Merged to Main Specs

All delta specs copied verbatim into `openspec/specs/` during archive:
- `openspec/specs/subagents/spec.md` — 15 REQs (NEW capability)
- `openspec/specs/agent-loop/spec.md` — 2 REQs ADDED (REQ-5, REQ-6)
- `openspec/specs/output-store/spec.md` — 6 REQs ADDED (OUTPUT-STORE-REQ-5 through -10)
- `openspec/specs/config/spec.md` — 5 REQs ADDED (CONFIG-REQ-4 through -8)

## Engram References

| Artifact | Topic Key | ID |
|----------|-----------|-----|
| SDD Proposal | `sdd/subagents/proposal` | (search in engram) |
| Phase 1 Spec + Design | `sdd/subagents/spec` + `sdd/subagents/design` | (search in engram) |
| Phase 2 Tasks Breakdown | `sdd/subagents/tasks` | (search in engram) |
| Apply Progress (P1+P2+fixes) | `sdd/subagents/apply-progress` | #1327 |
| Verify Report (P3+P4) | `sdd/subagents/verify-report` | #1328 |
| This Archive | `sdd/subagents/archive-report` | (this artifact) |

## Checklist

- [x] All delta specs merged into main `openspec/specs/`
- [x] Change folder moved to `openspec/changes/archive/subagents/`
- [x] ARCHIVE.md written with full traceability
- [x] All verification findings documented (0 CRITICAL, 4 WARNINGs, 4 SUGGESTIONs)
- [x] Historical artifacts preserved in archive folder
- [x] Engram topic keys recorded for audit trail

## Ready for PR Review

All 4 phases passed verification. Three stacked PRs (#4 → #5 → #6) are open on `main`. No blockers remain. Code is ready for human review and merge. If review forces structural changes, this archive can be reopened.
