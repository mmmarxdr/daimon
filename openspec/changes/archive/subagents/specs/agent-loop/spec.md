# Delta for Agent Loop (agent-loop)

## Overview

Extends `agent.New()` to wire a `SubagentManager` and register `SubagentSpawnTool` entries for every `ExecutableSkillDef` produced by the skill loader. Each subagent runs as a fully independent `Agent` instance; the principal's loop behavior is unchanged.

---

## ADDED Requirements

### AGENT-LOOP-REQ-5 — Wire SubagentManager in `agent.New()`

`agent.New()` SHALL accept an `[]ExecutableSkillDef` slice (produced by `skill.LoadSkills`). For each definition it MUST construct a `SubagentSpawnTool` and insert it into `a.tools` under the skill's name. It MUST also instantiate a `SubagentManager` and store a reference on the `Agent` struct so that spawned subagents can be tracked and cancelled via the manager.

(Previously: `agent.New()` had no `ExecutableSkillDef` parameter, no `SubagentManager`, and no synthetic spawn tools.)

#### Scenario: SubagentManager created at agent init

- GIVEN `agent.New()` is called with two `ExecutableSkillDef` entries (`researcher`, `summarizer`)
- WHEN the agent is fully initialized
- THEN `a.subagentManager` is non-nil
- AND `a.tools` contains keys `"researcher"` and `"summarizer"` of type `SubagentSpawnTool`

#### Scenario: No executable skills — no SubagentManager needed

- GIVEN `agent.New()` is called with an empty `ExecutableSkillDef` slice
- WHEN the agent is initialized
- THEN no `SubagentSpawnTool` entries are present in `a.tools`
- AND `a.subagentManager` is either nil or a no-op stub (implementation detail)

---

### AGENT-LOOP-REQ-6 — Subagent independence: own inbox, sem, ctx

Each subagent spawned via `SubagentManager.Spawn()` MUST run `agent.Run(subCtx)` in its own goroutine. The subagent MUST have its own `inbox` (created inside its `agent.Run`), its own `sem` (sized per subagent profile), and a `context.Context` derived from the `SubagentManager`'s cancellable context — NOT from the parent agent's root context directly, to allow individual-subagent cancellation.

(Previously: not applicable — no subagent concept existed.)

#### Scenario: parent loop unaffected while subagent runs

- GIVEN a `researcher` subagent is executing a multi-turn task
- WHEN the subagent's goroutine holds its own `sem`
- THEN the principal agent's `sem` is at its original capacity (no slots consumed)
- AND the principal can continue processing new messages concurrently

#### Scenario: subagent runs own turn loop

- GIVEN a `researcher` subagent is spawned
- THEN a separate `agent.Run()` call is made for the subagent instance in its own goroutine
- AND the subagent's `inbox` is distinct from the principal's `inbox`

---

## Acceptance Criteria

- [ ] `agent.New()` accepts `[]ExecutableSkillDef` and injects exactly one `SubagentSpawnTool` per definition into `a.tools`.
- [ ] `a.subagentManager` is initialized when at least one executable skill definition is present.
- [ ] The principal agent's `sem` is never consumed by a running subagent.
- [ ] Each spawned subagent runs `agent.Run()` in a separate goroutine with its own `inbox`, `sem`, and `ctx`.
- [ ] Cancelling the `SubagentManager`'s context for a specific subagent does NOT cancel the principal's context.
