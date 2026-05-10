# Specification: Subagents (New Capability)

**Change**: `subagents`
**Date**: 2026-05-10
**Status**: Draft

---

## Purpose

The subagents capability introduces spawnable, specialized agent loops that the principal agent can delegate bounded sub-tasks to. A subagent is defined declaratively via a `.skill.md` file with `executable: true` in its frontmatter; the loader produces an `ExecutableSkillDef` which `agent.New()` wires into a `SubagentSpawnTool` registered as a synthetic tool in the principal's tool map. At runtime the tool spawns an independent `Agent` instance — with its own inbox, semaphore, context, and conversation — runs it under budget constraints, then injects the output back into the parent conversation as a synthetic `tool_result` message. Three lifecycle events (`EventSubagentSpawned`, `EventSubagentCompleted`, `EventSubagentFailed`) are emitted on the `notify.Bus`; a REST endpoint and a WS event stream expose live subagent status to the frontend.

---

## Requirements

### REQ-1 — Skill-driven tool registration

The system SHALL register exactly one `SubagentSpawnTool` per `ExecutableSkillDef` (skill file with `executable: true`) into `a.tools` during `agent.New()`. Skill files without an `executable` key (or with `executable: false`) MUST NOT produce a `SubagentSpawnTool`.

#### Scenario: executable skill registers spawn tool at agent init

- GIVEN a skill file `researcher.skill.md` with `executable: true` in its frontmatter
- AND the skill loader is called during `agent.New()`
- WHEN the agent is fully initialized
- THEN `a.tools` contains an entry named `"researcher"` whose type is `SubagentSpawnTool`
- AND no other spawn tools are registered for non-executable skills

#### Scenario: non-executable skill does NOT register spawn tool

- GIVEN a skill file `summarizer.skill.md` with no `executable` key
- WHEN the agent is initialized
- THEN `a.tools` does NOT contain a `SubagentSpawnTool` for `"summarizer"`
- AND the skill's prose is still injected into the system prompt normally

---

### REQ-2 — Independent Agent instance per spawn

The system SHALL spawn each subagent as a fully independent `Agent` instance, with its own `inbox` channel, its own semaphore (`sem`), and its own `context.Context` derived from the `SubagentManager`. The spawned instance MUST NOT share the parent's `inbox`, `sem`, or `ctx`.

#### Scenario: subagent runs in isolation from parent loop

- GIVEN a `researcher` subagent is spawned
- WHEN the subagent is processing its turn
- THEN the parent's `sem` counter is unaffected
- AND the parent's `inbox` channel contains no subagent messages
- AND a separate `agent.Run()` goroutine executes for the subagent

---

### REQ-3 — Parent conversation linkage via `parent_conv_id`

The system SHALL create a new `store.Conversation` for each spawned subagent. The `Conversation.ParentConvID` field MUST be set to the parent conversation's ID. Subagent conversations with no parent MUST have `ParentConvID = ""`.

#### Scenario: spawned subagent conversation links to parent

- GIVEN principal conversation with `conv_id = "parent-abc"`
- WHEN a `researcher` subagent is spawned
- THEN the subagent's new conversation has `parent_conv_id = "parent-abc"` in the database

#### Scenario: principal conversation has no parent link

- GIVEN the principal agent creates a new conversation
- THEN `parent_conv_id` is `NULL` (or empty string) in the database

---

### REQ-4 — Turn-granularity budget enforcement

The system SHALL check the subagent's accumulated cost and turn count AFTER each `EventTurnCompleted` event. Budget checks MUST NOT occur mid-turn (while an LLM response is streaming).

#### Scenario: budget check fires after each turn

- GIVEN a `researcher` subagent with `max_turns: 5`
- AND the subagent has completed 3 turns
- WHEN `EventTurnCompleted` fires for turn 4
- THEN `SubagentManager` checks turn count and accumulated cost
- AND the subagent is NOT interrupted mid-streaming

---

### REQ-5 — Budget exceeded triggers cancellation and failure event

The system SHALL cancel the subagent's context and emit `EventSubagentFailed{reason: "budget_exceeded"}` when either `max_cost_usd`, `max_turns`, or `timeout_min` is exceeded.

#### Scenario: cost budget exceeded cancels subagent

- GIVEN a `researcher` subagent with `max_cost_usd: 0.10`
- AND accumulated cost reaches $0.10 after a turn completes
- WHEN the post-turn budget check runs
- THEN the subagent's context is cancelled
- AND `EventSubagentFailed{reason: "budget_exceeded"}` is emitted on `notify.Bus`
- AND the parent conversation receives this event

#### Scenario: turn budget exceeded cancels subagent

- GIVEN a `researcher` subagent with `max_turns: 5`
- AND the subagent completes turn 5
- WHEN the post-turn budget check runs
- THEN the subagent's context is cancelled
- AND `EventSubagentFailed{reason: "budget_exceeded"}` is emitted

#### Scenario: timeout cancels subagent

- GIVEN a `researcher` subagent with `timeout_min: 1`
- AND 60 seconds elapse since spawn without the subagent completing
- THEN the subagent's context is cancelled via `context.WithTimeout`
- AND `EventSubagentFailed{reason: "budget_exceeded"}` is emitted

#### Scenario: 80% soft warning injected into next turn

- GIVEN a `researcher` subagent with `max_cost_usd: 0.50`
- AND accumulated cost reaches $0.40 (80%) after a turn
- WHEN the post-turn budget check runs
- THEN a soft-warning message is injected into the subagent's next turn context
- AND the subagent is NOT cancelled

---

### REQ-6 — Parent context cancellation cascades to children within 1 second

The system SHALL ensure that when the parent agent's context is cancelled, all live subagent contexts derived from it are cancelled within 1 second. The cascade MUST be automatic via Go's `context` hierarchy (no polling required).

#### Scenario: parent ctx cancel cascades to live subagent

- GIVEN a `researcher` subagent is running
- AND the parent agent context is cancelled
- WHEN 1 second has elapsed
- THEN the subagent's context is cancelled
- AND the subagent's `agent.Run()` goroutine exits

#### Scenario: cancelling one subagent does not cancel parent

- GIVEN a running `researcher` subagent
- WHEN `SubagentManager.CancelSub(id)` is called for that subagent
- THEN only that subagent's context is cancelled
- AND the parent context remains active
- AND all other subagents remain running

---

### REQ-7 — `wait()` returns structured result

The system SHALL have `SubagentHandle.wait()` block until the subagent completes or is cancelled, then return a result with fields: `status`, `summary`, `artifacts`, `cost`, `errors`, `metadata`.

#### Scenario: wait returns on subagent completion

- GIVEN a `researcher` subagent spawned via `wait()` mode
- AND the subagent produces a summary string and zero artifacts
- WHEN the subagent completes
- THEN `wait()` returns `{status: "completed", summary: "<summary text>", artifacts: [], cost: <float64>, errors: nil, metadata: {...}}`

#### Scenario: wait returns on budget cancellation

- GIVEN a `researcher` subagent whose budget is exceeded
- WHEN `wait()` unblocks
- THEN the result has `status: "failed"` and `errors` contains `"budget_exceeded"`

---

### REQ-8 — Output injected as synthetic `tool_result` in parent conversation

The system SHALL inject the subagent's result into the parent conversation as a synthetic `tool_result` message after `wait()` completes. The message metadata MUST include `subagent_id` and `batch_id`.

#### Scenario: subagent result injected into parent conversation

- GIVEN a `researcher` subagent completes with summary `"Key findings: X"`
- WHEN the spawning `SubagentSpawnTool.Execute()` returns
- THEN the parent conversation contains a `tool_result` message with content `"Key findings: X"`
- AND the message metadata includes `subagent_id` and `batch_id`
- AND the principal's next turn naturally reads this digest

---

### REQ-9 — Depth limit: subagents cannot spawn subagents (V1)

The system SHALL reject any spawn attempt made from within a subagent context. The depth limit in V1 is exactly 1. Rejection MUST return a clear error to the calling tool execution.

#### Scenario: subagent attempting to spawn is rejected

- GIVEN a `researcher` subagent is running
- AND its tool list includes a `SubagentSpawnTool` (e.g. `summarizer`)
- WHEN the `researcher` subagent calls the `summarizer` spawn tool
- THEN `SubagentManager.Spawn` returns an error: `"subagent depth limit exceeded: max depth is 1"`
- AND the spawn is NOT attempted
- AND the error propagates back to the `researcher` subagent's turn as a tool error

---

### REQ-10 — Three lifecycle events emitted on `notify.Bus`

The system SHALL emit exactly three event types on the `notify.Bus` during a subagent's lifecycle:

- `EventSubagentSpawned` — immediately after the subagent goroutine is started
- `EventSubagentCompleted` — when the subagent finishes successfully
- `EventSubagentFailed` — when the subagent is cancelled or errors

Each event MUST include at minimum: `subagent_id`, `parent_conv_id`, `skill_name`, `batch_id`.

#### Scenario: spawn event emitted on start

- GIVEN a `researcher` subagent is spawned
- WHEN `SubagentManager.Spawn()` starts the goroutine
- THEN `EventSubagentSpawned` is emitted on `notify.Bus`
- AND the event contains `subagent_id`, `parent_conv_id`, `skill_name: "researcher"`, `batch_id`

#### Scenario: completed event emitted on success

- GIVEN a `researcher` subagent runs to completion without error
- THEN `EventSubagentCompleted` is emitted on `notify.Bus`
- AND the event contains `cost`, `turn_count`, and `summary` fields

#### Scenario: failed event emitted on cancellation

- GIVEN a `researcher` subagent is cancelled (budget or parent ctx)
- THEN `EventSubagentFailed` is emitted on `notify.Bus`
- AND the event contains a `reason` field (`"budget_exceeded"` or `"context_cancelled"`)

---

### REQ-11 — `tools_allowlist` validated at skill load time (exact names only)

The system SHALL validate every name in `tools_allowlist` against the set of known tool names at skill load time. Any name not present in the known tool registry MUST cause a load error. Glob patterns are NOT supported in V1.

#### Scenario: allowlist with valid tool names loads successfully

- GIVEN a skill file with `tools_allowlist: ["read_file", "mcp.github.search_code"]`
- AND both `"read_file"` and `"mcp.github.search_code"` exist in the tool registry
- WHEN the skill is loaded
- THEN loading succeeds with no error

#### Scenario: allowlist with unknown tool name causes load error

- GIVEN a skill file with `tools_allowlist: ["read_file", "nonexistent_tool"]`
- AND `"nonexistent_tool"` is NOT in the tool registry
- WHEN the skill is loaded
- THEN loading returns a non-nil error referencing `"nonexistent_tool"`
- AND the skill is NOT registered

#### Scenario: glob pattern in allowlist causes load error

- GIVEN a skill file with `tools_allowlist: ["read_*"]`
- WHEN the skill is loaded
- THEN loading returns a non-nil error (glob patterns not supported in V1)

---

### REQ-12 — Profile MUST declare `budget` block

The system SHALL require every executable skill to declare a `budget` block. A skill file with `executable: true` but no `budget` key MUST fail to load with a descriptive error. The `budget: defaults` shortcut is valid and expands to `max_cost_usd: 0.50`, `max_turns: 20`, `timeout_min: 10`.

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

#### Scenario: missing budget block causes load error

- GIVEN a skill file with `executable: true` and no `budget` key
- WHEN the skill is loaded
- THEN loading returns a non-nil error referencing `"budget"`
- AND the skill is NOT registered as executable

---

### REQ-13 — Cost records always written with `attribution_kind = "self"` in V1

The system SHALL write `attribution_kind = "self"` on every `cost_record` created by a subagent in V1. The schema supports future values (`"advisor_call"`, `"shared_resource"`) but no runtime path for these exists in V1.

#### Scenario: subagent cost record has attribution_kind = "self"

- GIVEN a `researcher` subagent completes a turn that incurs cost
- WHEN the cost record is written to the store
- THEN `cost_records.attribution_kind = "self"`
- AND `cost_records.conv_id` equals the subagent's conversation ID
- AND `cost_records.parent_conv_id` equals the parent conversation's ID

---

### REQ-14 — Subagents inherit parent MCP tools filtered by `tools_allowlist`

The system SHALL pass the parent's already-materialized MCP tool set (a filtered copy) to each subagent. The filter is the `tools_allowlist` from the executable skill definition. No new MCP subprocess is started per subagent.

#### Scenario: subagent receives filtered subset of parent tools

- GIVEN the parent agent has MCP tools `["mcp.github.search_code", "mcp.notion.list_pages", "read_file"]`
- AND a `researcher` skill has `tools_allowlist: ["mcp.github.search_code", "read_file"]`
- WHEN the `researcher` subagent is spawned
- THEN the subagent's `a.tools` contains `"mcp.github.search_code"` and `"read_file"`
- AND `"mcp.notion.list_pages"` is NOT in the subagent's tool map

#### Scenario: no new MCP process spawned for subagent

- GIVEN a `researcher` subagent is spawned
- WHEN the subagent starts
- THEN no new MCP subprocess is created for the subagent
- AND the subagent uses references to tools already connected in the parent

---

### REQ-15 — REST endpoint returns live subagent status

The system SHALL expose `GET /api/subagents/active` that returns a JSON object `{"active": [...]}`. Each entry in `active` MUST include: `subagent_id`, `batch_id`, `skill_name`, `parent_conv_id`, `status`, `cost_usd`, `turn_count`, `started_at` (RFC3339 timestamp).

When the runtime has no `SubagentManager` configured (no executable skills loaded), the endpoint MUST return `{"active": []}` with HTTP 200 — NOT 404 or 500.

WebSocket companion endpoint `GET /api/ws/subagents` SHALL stream lifecycle events as JSON frames with shape `{"event": "<event-name>", "payload": {...}}`. The `event` field uses the full event name (e.g., `"agent.subagent.spawned"`). Each WS connection MUST have a per-connection cap-8 outbound channel; on overflow, oldest frames MUST be dropped with a `slog.Warn` so a slow consumer does not block the bus or other subscribers.

#### Scenario: active subagents returned

- GIVEN two subagents (`researcher`, `summarizer`) are currently running
- WHEN `GET /api/subagents/active` is called
- THEN the response is `{"active": [<entry1>, <entry2>]}`
- AND each entry includes `subagent_id`, `batch_id`, `skill_name`, `parent_conv_id`, `status: "running"`, `cost_usd`, `turn_count`, `started_at`

#### Scenario: no active subagents returns empty array

- GIVEN no subagents are currently running
- WHEN `GET /api/subagents/active` is called
- THEN the response is `{"active": []}`

#### Scenario: WS stream delivers lifecycle events in order

- GIVEN a WebSocket client is connected to `/api/ws/subagents`
- WHEN a subagent is spawned and then completes
- THEN the client receives `{"event": "agent.subagent.spawned", "payload": {...}}` followed by `{"event": "agent.subagent.completed", "payload": {...}}` in that order
- AND a slow consumer dropping frames does NOT block delivery to other subscribers

---

## Acceptance Criteria

- [ ] Spawning a `researcher` subagent does NOT block the principal's `for/select` loop.
- [ ] The spawned `researcher` subagent runs in a fully independent `Agent` instance (own inbox, sem, ctx).
- [ ] Every spawned subagent's `store.Conversation` has `parent_conv_id` set to the principal's conversation ID.
- [ ] Budget exceeded (cost, turns, or timeout) cancels the subagent and emits `EventSubagentFailed{reason: "budget_exceeded"}`.
- [ ] Parent context cancellation cascades to all live children within 1 second.
- [ ] Subagent attempting to spawn another subagent is rejected with a clear error (depth limit = 1).
- [ ] `wait()` returns `{status, summary, artifacts, cost, errors, metadata}`.
- [ ] Subagent output is injected as a synthetic `tool_result` message in the parent conversation with `subagent_id` + `batch_id` in metadata.
- [ ] All three lifecycle events (`EventSubagentSpawned`, `EventSubagentCompleted`, `EventSubagentFailed`) are emitted on `notify.Bus` with required fields.
- [ ] `tools_allowlist` with an unknown tool name produces a load error; no spawn occurs.
- [ ] Executable skill with no `budget` block produces a load error.
- [ ] `budget: defaults` expands to `max_cost_usd: 0.50 / max_turns: 20 / timeout_min: 10`.
- [ ] All cost records written by subagents have `attribution_kind = "self"`.
- [ ] Subagents receive only the parent's MCP tools that appear in `tools_allowlist`; no new MCP processes are created.
- [ ] `GET /api/subagents/active` returns live spawns with correct status, cost, turn count.
- [ ] Existing non-executable skill files load unchanged with no warnings.
