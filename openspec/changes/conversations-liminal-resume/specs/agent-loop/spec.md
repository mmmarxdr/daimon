# Agent Loop (Delta) Specification

## Purpose

Extends the agent loop to accept an explicit `ConversationID` on incoming messages (bypassing `userScope` computation), and to invoke an async title-generation hook after the third completed turn.

## MODIFIED Requirements

### Requirement: Conversation identity resolution

`processMessage` MUST derive the conversation ID as follows:

1. If `msg.ConversationID` is non-empty, use it verbatim as the convID.
2. Otherwise, compute `convID = "conv_" + userScope(msg.ChannelID, msg.SenderID)` — existing behavior.

(Previously: convID was ALWAYS computed from `userScope(channelID, senderID)`, preventing clients from resuming a specific conversation across sessions.)

#### Scenario: Explicit ConversationID is respected

- GIVEN an `IncomingMessage` with `ConversationID = "conv_web:abc123:user42"`
- WHEN `processMessage` runs
- THEN `LoadConversation` is called with exactly `"conv_web:abc123:user42"`
- AND `userScope(msg.ChannelID, msg.SenderID)` is NOT invoked for convID purposes
- AND memory search / RAG scope are derived by stripping the `"conv_"` prefix from the provided convID

#### Scenario: Missing ConversationID falls back to userScope

- GIVEN an `IncomingMessage` with `ConversationID = ""`
- AND `ChannelID = "web:newconn"`, `SenderID = ""`
- WHEN `processMessage` runs
- THEN convID is computed as `"conv_" + userScope("web:newconn", "")` = `"conv_web:newconn"`
- AND behavior is identical to pre-change

#### Scenario: ConversationID on a new conversation creates it fresh

- GIVEN `ConversationID = "conv_web:resumed:u1"` for a conv NOT in the store
- WHEN `processMessage` runs
- THEN `LoadConversation` returns `ErrNotFound`
- AND a fresh `Conversation{ID: "conv_web:resumed:u1", ChannelID: msg.ChannelID, CreatedAt: now}` is created
- AND the user's message is appended and a new turn runs normally

### Requirement: Concurrency (documented limitation)

The agent MAY allow multiple concurrent `processMessage` goroutines for the same convID. Last-write-wins on `SaveConversation` is acceptable behavior.

(This is pre-existing; surfaced here because Resume makes it marginally more likely.)

#### Scenario: Concurrent turns on the same conv

- GIVEN two clients send messages targeting the same `ConversationID` within 100ms
- WHEN both trigger `processMessage`
- THEN both goroutines run to completion
- AND the later `SaveConversation` call wins — the other's appended messages may be lost
- AND `slog.Warn` is emitted IF the lost goroutine's `conv.UpdatedAt` was more recent than the winner's starting point

(Emitting the warning is a DEFENSIVE ADD — it lets operators detect collision events. Not required for acceptance but strongly recommended.)

## ADDED Requirements

### Requirement: Post-turn title generation hook

After `SaveConversation` completes in `processMessage`, the loop MUST check whether this turn qualifies for async title generation and enqueue a job if so.

Eligibility:
- `len(conv.Messages) >= 6` (3 user + 3 assistant turns, minimum 3 complete turns)
- `conv.Metadata["title"]` is absent or empty
- The first user-role message has at least 20 runes of text content
- `config.AI.TitleGeneration.Enabled` is `true` (default `true`)

When eligible, the hook enqueues a job on the `TitleGenerator` worker (spec: `title-generator`). The hook MUST NOT block the turn-end path for the caller — it returns immediately after enqueueing.

#### Scenario: Eligible conv triggers title generation

- GIVEN a conv with 6 messages, `metadata["title"]` empty, first user-msg `"quiero entender cómo funciona el RAG"` (>20 runes)
- AND `ai.title_generation.enabled = true`
- WHEN `processMessage` finishes saving the conv
- THEN a title job is enqueued on the `TitleGenerator`
- AND `processMessage` returns without waiting for the job to complete

#### Scenario: Short first message does not trigger

- GIVEN a conv with 6 messages but first user-msg `"hola"` (4 runes)
- WHEN `processMessage` finishes
- THEN no title job is enqueued

#### Scenario: Existing title is not overwritten

- GIVEN a conv with 6 messages and `metadata["title"] = "manually renamed"`
- WHEN `processMessage` finishes
- THEN no title job is enqueued (the condition is non-empty title blocks re-generation)

#### Scenario: Title generation disabled via config

- GIVEN `ai.title_generation.enabled = false`
- WHEN an otherwise-eligible conv saves
- THEN no title job is enqueued

#### Scenario: Feature hook is a no-op when TitleGenerator is not wired

- GIVEN `a.titler == nil`
- WHEN a conv saves
- THEN no crash; no-op; no enqueue attempt

## Non-requirements

- The agent MUST NOT pre-allocate a title at conversation creation.
- The agent MUST NOT retry a failed title job inline; retries are the worker's responsibility.
- The agent MUST NOT change the semantics of `continue_turn` — that code path is untouched.
