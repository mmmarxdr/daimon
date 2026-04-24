# Title Generator Specification

## Purpose

Defines the async worker that generates LLM-written titles for conversations that have enough real content to warrant one. The worker is triggered from the agent loop's post-turn hook (spec: `agent-loop`), runs off the hot path, never blocks a turn, and fails silently.

## ADDED Requirements

### Requirement: Worker structure

A new type `TitleGenerator` MUST live in `internal/agent/titler.go` with a method `Enqueue(ctx context.Context, convID string)`. Internally it uses a bounded job channel and a pool of worker goroutines.

- Default pool size: 2 workers.
- Default channel buffer: 32 pending jobs.
- Both configurable via `config.AI.TitleGeneration.{WorkerCount, QueueSize}` (optional, with sensible defaults).

#### Scenario: Bounded queue does not block the caller

- GIVEN the queue is full (32 pending jobs)
- WHEN `Enqueue` is called
- THEN the call returns immediately without blocking
- AND a `slog.Warn` message is emitted: "title_generator: queue full, job dropped"
- AND no panic, no error propagated to the agent loop

### Requirement: Job execution

A worker job MUST:

1. Call `store.LoadConversation(ctx, convID)` fresh (do not rely on in-memory state — the conv may have changed since enqueue).
2. Re-verify eligibility: `len(messages) >= 6`, `metadata["title"]` absent/empty, first user-msg ≥ 20 runes, `enabled=true`. If NOT eligible, drop the job silently.
3. Build a prompt from the first 3 turns (6 messages) concatenated as role-tagged text.
4. Call `provider.Chat(ctx, ChatRequest{Model: configured, Messages: [prompt]})` with a 30-second deadline via `context.WithTimeout`.
5. Normalize the response: trim whitespace, strip markdown delimiters `*`, `_`, `` ` ``, strip quotes, clamp to 100 runes.
6. Reject if the normalized string is empty or >= 100 runes (after clamp should not happen but defense-in-depth).
7. `conv.Metadata["title"] = normalized`; `SaveConversation(ctx, *conv)`.

#### Scenario: Successful generation

- GIVEN an eligible conv at enqueue time
- WHEN the worker processes the job
- THEN the provider is called with the title prompt
- AND `metadata["title"]` is set to the normalized response
- AND `SaveConversation` persists the change

#### Scenario: Provider timeout is silent

- GIVEN the provider does not respond within 30s
- WHEN the worker's context deadline fires
- THEN the worker emits `slog.Warn` with the convID and "provider_timeout"
- AND `metadata["title"]` remains empty
- AND NO retry happens for this job

#### Scenario: Provider returns an empty or whitespace-only response

- GIVEN the provider returns `""` or `"   "`
- WHEN the worker normalizes
- THEN the job is dropped (no save)
- AND `slog.Warn` logs "empty_response"

#### Scenario: Conversation was deleted before the worker ran

- GIVEN a job was enqueued for convID X
- AND X was soft-deleted before the worker picked it up
- WHEN the worker calls `LoadConversation`
- THEN `ErrNotFound` is returned
- AND the job is dropped silently (no error logged above debug level)

#### Scenario: Conversation already has a title

- GIVEN a job was enqueued
- AND a manual rename set `metadata["title"]` between enqueue and execution
- WHEN the worker re-checks eligibility
- THEN the job is skipped (manual rename takes precedence)

### Requirement: Prompt template (fixed for v1)

The prompt sent to the provider MUST be:

```
Generate a 3-8 word title summarising this conversation. Respond with ONLY the title — no quotes, no preamble, no explanation.

{serialized_turns}
```

Where `{serialized_turns}` is the first 6 messages formatted as:

```
User: <text content, media blocks omitted>
Assistant: <text content, tool-use blocks omitted>
User: ...
Assistant: ...
User: ...
Assistant: ...
```

Text content is `content.TextOnly()`. Media and tool-use blocks are dropped from the serialized form (the LLM doesn't need them for a title).

#### Scenario: Media blocks are omitted from the prompt

- GIVEN a conv where turn 2 included an image attachment
- WHEN the prompt is built
- THEN the image block is NOT in the prompt text
- AND only the text content of that turn is included

### Requirement: Shutdown behavior

`TitleGenerator.Stop(ctx)` MUST:

1. Stop accepting new jobs (close the input channel, reject new `Enqueue` calls with `slog.Debug` "shutting down").
2. Wait for in-flight jobs to complete, bounded by `ctx` (caller's responsibility to provide a deadline).
3. Discard any pending jobs in the queue on timeout.

#### Scenario: Graceful shutdown

- GIVEN 1 in-flight job and 3 queued jobs
- WHEN `Stop(ctx)` is called with a 5s deadline
- THEN the in-flight job completes
- AND the 3 queued jobs are discarded with a single `slog.Info` summary line
- AND `Stop` returns nil

#### Scenario: Shutdown deadline exceeded

- GIVEN an in-flight job stuck on a provider call
- WHEN `Stop(ctx)` is called with a 100ms deadline
- THEN `Stop` returns `ctx.Err()` after the deadline
- AND the stuck job is abandoned (provider will observe its own context cancellation)

### Requirement: Wiring

`TitleGenerator` MUST be constructed in `cmd/daimon/main.go` (and `web_cmd.go` if applicable) at the same wiring stage as other async workers (Enricher, EmbeddingWorker). It MUST be passed to the agent via a setter `Agent.WithTitler(*TitleGenerator)`.

#### Scenario: Config flag disables wiring

- GIVEN `ai.title_generation.enabled = false`
- WHEN main wires the agent
- THEN `Agent.WithTitler(nil)` is called
- AND the agent loop's hook is a no-op (spec: `agent-loop`)

## Non-requirements

- The worker does NOT support retrying failed jobs in v1. Next successful turn re-enqueues.
- The worker does NOT choose a different / cheaper model than the main chat provider in v1. Model override is a follow-up (`ai.title_generation.model`).
- The worker does NOT attempt streaming. A single blocking `Chat` call is sufficient for ~10 output tokens.
- The worker does NOT update the title after it has been set. One-shot per conv (renames are manual via REST endpoint).
