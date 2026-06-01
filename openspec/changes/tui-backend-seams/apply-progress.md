# Apply Progress: tui-backend-seams

**Status**: done — 35/35 tasks complete
**Mode**: Strict TDD (RED → GREEN → REFACTOR per seam)
**Branch**: tui-backend-seams

## TDD Cycle Evidence

| Seam | Task        | RED Evidence                                                          | GREEN Evidence                                  | REFACTOR                                       | Commit SHA |
| ---- | ----------- | --------------------------------------------------------------------- | ----------------------------------------------- | ---------------------------------------------- | ---------- |
| 1    | 1.1–1.4     | compile error: `cm.LastUsage undefined`                               | ok 0.006s                                       | pctOf inlined as closure                       | 4a130ab    |
| 1    | 1.5–1.10    | —                                                                     | all agent tests pass 13.4s                      | confirmed, no new exports                      | 4a130ab    |
| 2+6  | 2.1,4.1,6.1 | compile errors: SysToks/MsgToks/ToolToks/EventMemoryChanged undefined | ok notify 6.8s                                  | consumers verified additive                    | a958f19    |
| 2    | 2.2–2.6     | compile error: ev.SysToks undefined                                   | ok 0.006s                                       | grep consumers: TUI/web unaffected             | a958f19    |
| 3    | 3.1–3.2     | compile error: rec.tokens undefined                                   | ok 0.179s                                       | tokens captured under same lock as cost/turns  | 17a8253    |
| 5    | 5.1         | compile error: a.ContextWindowSize undefined                          | ok 0.005s                                       | MaxTokens() already exists                     | 10269b1    |
| 4    | 4.2–4.6     | compile errors: SetBus/Bus field undefined                            | ok agent+tool 13.6s+0.6s                        | dedup UpdateMemory path confirmed out-of-scope | 46176c9    |
| 6    | 6.3         | —                                                                     | race clean: agent 20.3s, notify 7.9s, tool 7.2s | —                                              | —          |

## make test Summary

```
--- FAIL: TestRunTUICommand_MissingConfig (0.00s)  [PRE-EXISTING, out of scope]
FAIL    daimon/cmd/daimon 0.510s
ok      daimon/internal/agent   13.609s
ok      daimon/internal/notify  6.832s
ok      daimon/internal/tool    0.683s
ok      daimon/internal/tui     1.219s
[all other packages: ok or cached]
```

## make test-race Summary

```
--- FAIL: TestRunTUICommand_MissingConfig (0.02s)  [PRE-EXISTING, out of scope]
FAIL    daimon/cmd/daimon 4.183s
ok      daimon/internal/agent   20.951s
ok      daimon/internal/notify  7.892s
ok      daimon/internal/tool    8.218s
ok      daimon/internal/tui     2.712s
[all other packages: ok or cached]
```

## Completed Tasks (35/35)

All tasks marked `[x]` in tasks.md.

## Files Changed

| File                                       | Action                                                                  |
| ------------------------------------------ | ----------------------------------------------------------------------- |
| `internal/agent/context_manager.go`        | Modified — Tools field, lastUsage cache, LastUsage() accessor           |
| `internal/agent/context_manager_test.go`   | Modified — Seam 1 tests                                                 |
| `internal/notify/bus.go`                   | Modified — SysToks/MsgToks/ToolToks fields                              |
| `internal/notify/events.go`                | Modified — EventMemoryChanged constant + KnownEventTypes                |
| `internal/notify/events_test.go`           | Modified — Seam 2+4+6 tests                                             |
| `internal/agent/loop.go`                   | Modified — emit site LastUsage + EventMemoryChanged at 2 fallback paths |
| `internal/agent/loop_tokens_usage_test.go` | Modified — Seam 2 tests                                                 |
| `internal/agent/subagent_manager.go`       | Modified — tokens field, budgetMonitor accumulate, finalize Meta        |
| `internal/agent/subagent_manager_test.go`  | Modified — Seam 3 tests                                                 |
| `internal/agent/agent_accessors.go`        | Modified — ContextWindowSize() added                                    |
| `internal/agent/agent_accessors_test.go`   | Modified — Seam 5 tests                                                 |
| `internal/agent/curator.go`                | Modified — bus field, SetBus(), AppendMemory emit                       |
| `internal/agent/curator_test.go`           | Modified — Seam 4 curator tests                                         |
| `internal/agent/consolidator.go`           | Modified — bus field, SetBus(), AppendMemory emit                       |
| `internal/agent/consolidator_test.go`      | Modified — Seam 4 consolidator tests                                    |
| `internal/agent/agent.go`                  | Modified — WithBus propagates to curator+consolidator                   |
| `internal/tool/memory.go`                  | Modified — Bus field on MemoryToolDeps, saveMemoryTool emit             |
| `internal/tool/memory_test.go`             | Modified — Seam 4 tool tests                                            |

## Deviations from Design

None. All ADRs implemented exactly as specified.

Notable implementation decisions:

- `pctOf` helper: inlined as anonymous closure (no new exported symbol) per task 1.10
- Seam 3 test: used controlled child factory (gate channel) instead of timing-based approach to reliably test multi-turn token accumulation
- Seam 4 consolidator test: called `consolidateScope` directly (same package, white-box) since `consolidateAll` requires SQLiteStore for scope listing
