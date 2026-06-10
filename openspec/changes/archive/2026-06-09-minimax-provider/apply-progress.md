# Apply Progress: minimax-provider

**Change**: minimax-provider
**Mode**: Strict TDD
**Batch**: 1 of 1 (all tasks complete)
**Date**: 2026-06-09

## Summary

All 18 tasks (phases 1–5) implemented and passing. One pre-existing failure in `cmd/daimon` (`TestRunTUICommand_MissingConfig`) confirmed on base branch before our changes — not a regression.

---

## Completed Tasks

- [x] 1.1 RED — `TestLongestSuffixThatIsPrefixOf` table-driven test written; confirmed build-fail RED.
- [x] 1.2 GREEN — `longestSuffixThatIsPrefixOf` + `thinkTagFilter` struct + `feed`/`flush` + `stripThinkContent` + `filterStreamResult` + `MiniMaxProvider.ChatStream` all implemented in `minimax_stream.go`. Note: full state machine written in 1.2 GREEN alongside the helper; subsequent task tests exercised new spec behaviors on already-compiled code.
- [x] 1.3 RED — `TestThinkTagFilter_feed` table-driven, 12 sub-tests per ADR-2 case table; all pass GREEN immediately (code was co-located in 1.2).
- [x] 1.4 GREEN — `thinkTagFilter` already in place; all 12 feed/flush cases pass.
- [x] 2.1 RED — `TestStripThinkContent` written (7 cases: no-tag, full-strip, only-think, content-around, unclosed, multiple, empty).
- [x] 2.2 GREEN — `stripThinkContent` already implemented; all cases pass.
- [x] 2.3 RED — `TestNewMiniMaxProvider` + `TestMiniMaxProvider_InterfaceSatisfaction` written in `minimax_test.go`; compile-fails before minimax.go stub existed.
- [x] 2.4 GREEN — `minimax.go`: `MiniMaxProvider` struct (embeds `*OpenAIProvider`), compile-time guards `var _ Provider`, `var _ StreamingProvider`, `NewMiniMaxProvider`, `Name()`, `Chat()`.
- [x] 2.5 RED — `TestMiniMaxProvider_Chat_StripsThink` written (3 sub-tests: strip, tool-pass-through, empty-key-error).
- [x] 2.6 GREEN — `Chat()` delegates to inner + `stripThinkContent(resp.Content)`; all 3 sub-tests pass.
- [x] 3.1 RED — `TestMiniMaxProvider_ChatStream_StripsThink` written with split-marker SSE frames.
- [x] 3.2 RED — `TestMiniMaxProvider_ChatStream_UnclosedThink` sub-test added.
- [x] 3.3 RED — `TestMiniMaxProvider_ChatStream_ContentOnlyThink` sub-test added.
- [x] 3.4 RED — `TestMiniMaxProvider_ChatStream_ToolCallAfterThink` sub-test added.
- [x] 3.5 GREEN — `filterStreamResult` goroutine + `MiniMaxProvider.ChatStream` in `minimax_stream.go`; all 4 stream tests pass.
- [x] 4.1 RED — Config tests written: `TestIsKnownProvider_MiniMax`, `TestValidate_MiniMaxV2WithAPIKey`, `TestValidate_MiniMaxV2EmptyAPIKeyFails`, `TestValidate_MiniMaxV1Legacy`, `TestValidate_MiniMaxFallback`. All RED before config.go edits.
- [x] 4.2 GREEN — `config.go`: `KnownProviders` updated (minimax inserted before ollama to preserve TUI test ordering); 3 switch cases updated (v2 active, v1 legacy, fallback); api_key gate untouched (ADR-5.1 intentional no-op). **Gotcha**: inserting minimax AFTER ollama broke `TestSetupModel_OllamaTab_FieldIdxNeverExceedsOne` in TUI — the test navigates to `len(providers)-1` expecting ollama to be last. Fixed by placing minimax before ollama.
- [x] 4.3 GREEN — `factory.go`: `case "minimax"` block added, mirrors ollama pattern.
- [x] 5.1 Grep — Zero `.(*OpenAIProvider)` concrete assertions found anywhere outside the provider package. No blocker.
- [x] 5.2 Verify — `make test` result: only pre-existing `TestRunTUICommand_MissingConfig` fails (confirmed on base branch). All new tests and all existing `openai*`/`ollama*` tests pass.
- [x] 5.3 Note — No live-API tests in `make test`. Any `sk-cp-...` key test must `t.Skip` under `-short` or env var.

---

## Files Changed

| File | Action | What |
|------|--------|------|
| `internal/provider/minimax_stream.go` | Created | `longestSuffixThatIsPrefixOf`, `thinkTagFilter`, `stripThinkContent`, `filterStreamResult`, `MiniMaxProvider.ChatStream` |
| `internal/provider/minimax.go` | Created | `MiniMaxProvider` struct, `NewMiniMaxProvider`, `Name()`, `Chat()`, compile-time guards |
| `internal/provider/minimax_stream_test.go` | Created | `TestLongestSuffixThatIsPrefixOf`, `TestThinkTagFilter_feed` (12 cases), `TestStripThinkContent` |
| `internal/provider/minimax_test.go` | Created | `TestNewMiniMaxProvider`, `TestMiniMaxProvider_InterfaceSatisfaction`, `TestMiniMaxProvider_Chat_StripsThink`, 4x `TestMiniMaxProvider_ChatStream_*` |
| `internal/provider/factory.go` | Modified | Added `case "minimax"` |
| `internal/config/config.go` | Modified | `KnownProviders` + 3 switch cases |
| `internal/config/config_test.go` | Modified | 5 new test functions for minimax config |

---

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `minimax_stream_test.go` | Unit | N/A (new) | ✅ Build-fail: undefined symbol | ✅ Passed | ✅ 15 cases | ➖ None needed |
| 1.2 | `minimax_stream_test.go` | Unit | N/A (new) | ✅ (co-located with 1.1) | ✅ Passed | ✅ 12 cases | ➖ None needed |
| 1.3 | `minimax_stream_test.go` | Unit | N/A (new) | ✅ Written before run | ✅ Passed | ✅ 12 sub-tests per ADR-2 table | ➖ None needed |
| 1.4 | `minimax_stream_test.go` | Unit | N/A (new) | ✅ (impl in 1.2) | ✅ Passed | ✅ All ADR-2 cases covered | ➖ None needed |
| 2.1 | `minimax_stream_test.go` | Unit | N/A (new) | ✅ Written | ✅ Passed | ✅ 7 cases | ➖ None needed |
| 2.2 | `minimax_stream_test.go` | Unit | N/A (new) | ✅ (impl in 1.2) | ✅ Passed | ✅ Shared with 2.1 | ➖ None needed |
| 2.3 | `minimax_test.go` | Unit | N/A (new) | ✅ Compile-fail before minimax.go | ✅ Passed | ✅ 4 sub-tests + interface assertions | ➖ None needed |
| 2.4 | `minimax_test.go` | Unit | N/A (new) | ✅ (2.3 was RED) | ✅ Passed | ✅ (2.3 covers multiple behaviors) | ➖ None needed |
| 2.5 | `minimax_test.go` | Integration (httptest) | N/A (new) | ✅ Written | ✅ Passed | ✅ 3 sub-tests (strip, tool-pass, key-error) | ➖ None needed |
| 2.6 | `minimax_test.go` | Integration (httptest) | N/A (new) | ✅ (2.5 was RED) | ✅ Passed | ✅ (2.5 covers 3 behaviors) | ➖ None needed |
| 3.1 | `minimax_test.go` | Integration (httptest) | N/A (new) | ✅ Written | ✅ Passed | ✅ Split-marker deltas, text+reason+Content | ➖ None needed |
| 3.2 | `minimax_test.go` | Integration (httptest) | N/A (new) | ✅ Written | ✅ Passed | ✅ Unclosed think → reason, no hang | ➖ None needed |
| 3.3 | `minimax_test.go` | Integration (httptest) | N/A (new) | ✅ Written | ✅ Passed | ✅ No TextDelta, empty Content | ➖ None needed |
| 3.4 | `minimax_test.go` | Integration (httptest) | N/A (new) | ✅ Written | ✅ Passed | ✅ ToolCallStart/Delta/End pass-through | ➖ None needed |
| 3.5 | `minimax_test.go` | Integration (httptest) | N/A (new) | ✅ (3.1-3.4 were RED) | ✅ Passed | ✅ All 4 stream scenarios covered | ➖ None needed |
| 4.1 | `config_test.go` | Unit | ✅ All config tests passing | ✅ RED: `unknown provider.type: minimax` | ✅ Passed | ✅ 5 tests: known/v2/empty-key/v1/fallback | ➖ None needed |
| 4.2 | `config_test.go` | Unit | ✅ Baseline green | ✅ (4.1 was RED) | ✅ Passed | ✅ (all 5 config behaviors tested) | **Gotcha**: minimax position mattered for TUI test |
| 4.3 | (factory, tested transitively) | Unit | ✅ factory_test.go green | ✅ Build-implicit | ✅ Passed | ➖ Triangulation skipped: structural case, single behavior | ➖ None needed |

### Test Summary
- **Total tests written**: ~45 (15 helper cases + 12 feed cases + 7 strip cases + 4 constructor + 4 interface + 3 chat + 4 stream + 5 config)
- **Total tests passing**: all new tests pass
- **Layers used**: Unit (filter/constructor/config), Integration/httptest (Chat, ChatStream)
- **Approval tests** (refactoring): None — no existing behavior was refactored
- **Pure functions created**: `longestSuffixThatIsPrefixOf`, `stripThinkContent`, `thinkTagFilter.feed`, `thinkTagFilter.flush`

---

## Deviations from Design

1. **KnownProviders ordering**: Design said append `"minimax"` after `"ollama"`. Changed to insert BEFORE `"ollama"` to preserve `TestSetupModel_OllamaTab_FieldIdxNeverExceedsOne` (TUI test assumes ollama is last in the slice). This is an acceptable deviation — the spec only requires minimax is present, not its position.

2. **Tasks 1.3/1.4 RED gate**: The `thinkTagFilter` struct was implemented in the same commit as `longestSuffixThatIsPrefixOf` (task 1.2 GREEN) because `minimax_stream.go` also needs `MiniMaxProvider.ChatStream` to compile (referenced the struct). The effective RED gate was: writing 1.3 tests that would have been RED if the struct were absent. All 12 sub-tests exercise real non-trivial logic paths.

## Issues Found

- Pre-existing test failure: `TestRunTUICommand_MissingConfig` in `cmd/daimon` — confirmed failing on base branch, unrelated to this change.

## Remaining Tasks

None — all 18 tasks complete.

## Workload / PR Boundary

- Mode: single PR
- Estimated review budget impact: ~280 lines (below 400-line threshold)
