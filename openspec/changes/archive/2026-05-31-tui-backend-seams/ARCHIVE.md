# Archive Report — tui-backend-seams

**Change**: `tui-backend-seams`
**Archived**: 2026-05-31
**Delivered to main**: fast-forward `0e58ac2..8666b34`
**Final phase outcome**: judgment-day APPROVED (Round 2) · verify PASS-WITH-WARNINGS (0 CRITICAL)

## Summary

Additive backend data seams that publish the information the Phase-2 TUI rail panels
need (context-meter, telemetry, memory-peek), via the notify bus, without breaking any
existing `notify.Event` consumer and without violating `View=pure(Model)`. **Backend
only — zero `internal/tui/` files touched.** The downstream consumer is the
`tui-rail-panels` change.

## Seams delivered

1. **`ContextManager.LastUsage() (TokenUsage, bool)`** — caches the last per-category
   token breakdown (`SystemPrompt`/`Messages`/`Tools`; new `Tools int` on `TokenUsage`),
   written on all strategy paths before the smartManage early-return, guarded by the
   existing `cm.mu`.
2. **`SysToks`/`MsgToks`/`ToolToks int` (omitempty)** on `notify.Event`, emitted on
   `EventTokensUsage` at the loop turn-end site (nil-guarded read of `LastUsage()`),
   REPLACE/snapshot semantics.
3. **`subRecord.tokens`** accumulated in `budgetMonitor` from `EventTurnCompleted` Meta
   (`input_tokens`+`output_tokens`), published as `tokens` Meta on `EventSubagentCompleted`.
4. **`EventMemoryChanged`** bare signal (Meta `scope_id`+`entry_id`), in `KnownEventTypes`,
   NOT in `StreamingSkipSet`; emitted nil-bus-guarded after every successful `AppendMemory`
   at 5 sites; bus injected into `Curator`/`Consolidator`/`MemoryToolDeps`. Plus
   **`Agent.ContextWindowSize() int`** (nil contextMgr → 0).

## Implementation

- Commits: `4a130ab` (LastUsage cache), `a958f19` (Event category fields + EventMemoryChanged
  const), `17a8253` (subRecord.tokens), `10269b1` (ContextWindowSize), `46176c9`
  (EventMemoryChanged emit + bus injection), `99e2107` (tasks done), `429cefe` (test
  strengthening), `268bc6d` (spec/verify-report), `7c0c363` + `8666b34` (judgment-day
  Round-1 fix).
- Strict TDD throughout; 35 tasks RED→GREEN→REFACTOR. `make test` + `-race` green on
  `internal/agent`, `internal/notify`, `internal/tool`. Known pre-existing
  `cmd/daimon TestRunTUICommand_MissingConfig` failure is out of scope (fails on main too).

## Judgment Day

- **Round 1**: BOTH judges CHANGES REQUESTED — confirmed CRITICAL: `EventMemoryChanged` bus
  injection was DEAD CODE in production (`WithBus` ran before `wireSmartMemory`, so SetBus
  propagation no-op'd on nil curator/consolidator and `MemoryToolDeps.Bus` was never set).
  Green unit tests had bypassed production wiring.
- **Fix** (option C): `WithCurator`/`WithConsolidator` auto-`SetBus(a.bus)`; `wireSmartMemory`
  sets `MemoryToolDeps{Bus: ag.SubagentBus()}` (which returns `a.bus`). Added a
  production-order integration test (`memory_changed_wiring_test.go`, verified RED-on-revert).
- **Round 2**: BOTH judges APPROVED — bus identity proven, both entry points (main + web)
  covered, test honest. Terminal state APPROVED.

## Spec promotion

Delta spec promoted to `openspec/specs/tui-backend-seams/spec.md` (new capability, no prior
spec to merge). 5 requirements / 17 scenarios; 12/17 covered by backend tests (the 5
uncovered are TUI-consumer concerns deferred to `tui-rail-panels`).

## Follow-up

- `tui-rail-panels` — the downstream TUI change that consumes these seams into the
  context-meter / telemetry / memory-peek panels. Exploration already drafted at
  `openspec/changes/tui-rail-panels/exploration.md`.
