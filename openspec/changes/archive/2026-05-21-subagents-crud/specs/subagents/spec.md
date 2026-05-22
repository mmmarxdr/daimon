# Delta for Subagents (subagents)

This delta amends the canonical `openspec/specs/subagents/spec.md` with changes from the `subagents-crud` change.

---

## MODIFIED Requirements

### Requirement: REQ-12 — Profile MAY declare `budget` block

The system SHALL accept executable skills that omit the `budget` block. When `budget` is absent (or all fields are zero), the subagent runs with NO BUDGET LIMITS: unlimited cost, unlimited turns, and no timeout. The system MUST NOT inject a default budget at parse or runtime time.

The `budget: defaults` shortcut remains valid and expands to `max_cost_usd: 0.50`, `max_turns: 20`, `timeout_min: 10`. Explicit budget blocks are still valid as before.

The frontend SHOULD warn users when saving an executable skill with no budget configured, but this is a UI concern and is out of scope for this spec.

(Previously: "executable skill MUST declare a budget block; missing → load error". A skill file with `executable: true` but no `budget` key was required to fail loading with a descriptive error.)

#### Scenario: explicit budget block loads successfully

- GIVEN a skill file with:
  ```yaml
  executable: true
  budget:
    max_cost_usd: 0.10
    max_turns: 10
    timeout_min: 5
  ```
- WHEN the skill is loaded
- THEN loading succeeds and budget values are parsed correctly

#### Scenario: `budget: defaults` shortcut loads successfully

- GIVEN a skill file with:
  ```yaml
  executable: true
  budget: defaults
  ```
- WHEN the skill is loaded
- THEN loading succeeds
- AND the budget expands to `max_cost_usd: 0.50`, `max_turns: 20`, `timeout_min: 10`

#### Scenario: missing budget block NOW SUCCEEDS (reversal)

- GIVEN a skill file with `executable: true` and no `budget` key
- WHEN the skill is loaded
- THEN loading succeeds with no error
- AND the skill is registered as executable with `Budget = nil` semantically (unlimited)

#### Scenario: nil budget skill spawns without instant cancellation

- GIVEN an executable skill with no `budget` block (Budget = nil)
- WHEN the skill is spawned
- THEN the runtime allocates `context.WithCancel(ctx)` instead of `context.WithTimeout`
- AND the budgetMonitor's cost and turn guards are no-ops (already guarded by `> 0` checks in existing implementation)
- AND the subagent context remains live until explicit cancellation, parent ctx cancel, or natural completion

---

## ADDED Requirements

### Requirement: REQ-16 — `Spawn` Timeout==0 produces no instant cancel

The system SHALL branch `Spawn`'s context creation: when `Budget.Timeout > 0`, use `context.WithTimeout`; when `Budget.Timeout == 0`, use `context.WithCancel`. The subagent context MUST NOT be subject to instant cancellation when budget timeout is unset.

#### Scenario: timeout zero uses WithCancel

- GIVEN a skill with `Budget.Timeout == 0` (either nil budget or explicitly zero)
- WHEN the skill is spawned
- THEN the subagent ctx is created with `context.WithCancel`
- AND the subagent context remains live until explicit cancellation, parent ctx cancel, or natural completion

#### Scenario: positive timeout uses WithTimeout (unchanged behavior)

- GIVEN a skill with `Budget.Timeout > 0`
- WHEN the skill is spawned
- THEN behavior is unchanged from REQ-5/REQ-6: the timeout fires `EventSubagentFailed{reason: "budget_exceeded"}` when the timeout elapses

---

### Requirement: REQ-17 — REST `POST /api/subagents/{id}/cancel`

The system SHALL expose `POST /api/subagents/{id}/cancel` that calls `Agent.CancelSubagent(id)`.

| Case | HTTP Response |
|------|--------------|
| Successful cancel | 204 No Content |
| ID not found | 404 Not Found |
| Already in terminal state (completed/failed/cancelled) | 200 `{"already_finished": true}` |
| Internal error | 500 |

#### Scenario: cancel running subagent returns 204

- GIVEN a running subagent with id `"sub-123"`
- WHEN `POST /api/subagents/sub-123/cancel` is called
- THEN the subagent is cancelled within 1 second
- AND `EventSubagentFailed{reason: "cancelled"}` is emitted on `notify.Bus`
- AND the response status is 204

#### Scenario: cancel unknown id returns 404

- GIVEN no subagent exists with id `"nope"`
- WHEN `POST /api/subagents/nope/cancel` is called
- THEN the response status is 404

#### Scenario: cancel already-finished subagent returns 200 with flag

- GIVEN a subagent that has already completed or failed
- WHEN `POST /api/subagents/{id}/cancel` is called
- THEN the response status is 200
- AND the body is `{"already_finished": true}`

---

### Requirement: REQ-18 — `Agent.CancelSubagent(id)` nil-safe delegate

The `Agent` SHALL expose `CancelSubagent(id string) error` that delegates to `subMgr.Cancel(id)` when a SubagentManager is present. When no SubagentManager is configured, the method MUST return nil (soft no-op).

#### Scenario: agent with no SubagentManager returns nil

- GIVEN an Agent initialized with no executable skills (subMgr is nil)
- WHEN `CancelSubagent("any")` is called
- THEN no error is returned
- AND no panic occurs

#### Scenario: agent with active subagents cancels target

- GIVEN an Agent with a running subagent `"sub-1"`
- WHEN `CancelSubagent("sub-1")` is called
- THEN the subagent's ctx is cancelled
- AND a lifecycle event is emitted via the SubagentManager

---

### Requirement: REQ-19 — `Agent.ReplaceExecutableSkills(defs)` hot-reload

The `Agent` SHALL expose `ReplaceExecutableSkills(defs []skill.ExecutableSkillDef)`. This method MUST:
1. Acquire `toolsMu.Lock`
2. Remove all existing `*SubagentSpawnTool` entries from `a.tools`
3. Re-validate each def's `tools_allowlist` against the current `a.tools` map (drop unknown names with a WARN log — same semantics as `WithExecutableSkills`)
4. Register a fresh `SubagentSpawnTool` per def
5. Lazily initialize `subMgr` if nil and `len(defs) > 0`

Non-spawn tools in `a.tools` MUST NOT be removed or affected.

#### Scenario: replaces all spawn tools atomically

- GIVEN an Agent with 2 executable skills loaded
- WHEN `ReplaceExecutableSkills` is called with 3 new defs
- THEN `a.tools` contains exactly the 3 new SubagentSpawnTools
- AND the previous 2 spawn tools are gone
- AND all non-spawn tools remain unchanged

#### Scenario: lazy-initializes subMgr when first skills arrive

- GIVEN an Agent with no SubagentManager (no executable skills at boot)
- WHEN `ReplaceExecutableSkills` is called with 1 def
- THEN `subMgr` is initialized
- AND the SubagentSpawnTool for the def is registered in `a.tools`

#### Scenario: unknown allowlist tool is dropped with warn

- GIVEN a def with `tools_allowlist` containing an unknown tool name
- WHEN `ReplaceExecutableSkills` is called
- THEN the unknown name is dropped and a WARN is logged
- AND the rest of the allowlist entries remain on the registered tool

---

### Requirement: REQ-20 — `ConfigurableProvider` interface for credential inheritance

The `provider` package SHALL define:

```go
type ConfigurableProvider interface {
    Provider
    Config() config.ProviderConfig
}
```

Each of the 5 built-in providers (anthropic, openai, openrouter, gemini, ollama) MUST implement `ConfigurableProvider`. The `makeChildAgentFn` closure in `Agent` MUST type-assert the parent provider to `ConfigurableProvider`. If the assertion succeeds, the child provider config inherits from the parent (model overridden by `skill.Model` if non-empty). If the assertion fails, the child relies on `skill.Provider` being fully self-configured; an auth error at first request is the expected failure mode (graceful, not panic).

#### Scenario: child inherits parent API key via Config()

- GIVEN a parent Anthropic provider with an API key set
- AND an executable skill with an empty `provider:` field
- WHEN a child agent is spawned
- THEN the child uses the parent's API key (via `Config()` return value)
- AND no auth error occurs at first request

#### Scenario: non-ConfigurableProvider parent falls back gracefully

- GIVEN a parent provider that does NOT implement `ConfigurableProvider`
- WHEN a child agent is spawned
- THEN the child is constructed from `skill.Provider` config alone
- AND if credentials are absent, an auth error occurs at the first LLM request (not a panic)

---

## Acceptance Criteria Additions

- [ ] All 5 providers (anthropic, openai, openrouter, gemini, ollama) implement `ConfigurableProvider`
- [ ] `POST /api/subagents/{id}/cancel` returns 204 within 1 second for a running subagent
- [ ] `ReplaceExecutableSkills` swaps spawn tools without removing non-spawn tools
- [ ] Executable skill with no `budget` block loads, spawns, and completes normally without instant cancellation
- [ ] Executable skill with no `budget` block does NOT trigger a load error (REQ-12 reversal is enforced)
