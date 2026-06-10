# Spec — minimax-provider

OpenAI-compatible transport to MiniMax API with stateful `<think>...</think>` tag stripping and routing in both sync and streaming chat paths.

## Requirements

### REQ-MM-1 — Provider construction and identity

A `MiniMaxProvider` MUST be constructible from a provider config block with `type: minimax`. It MUST use base URL `https://api.minimax.io/v1` by default, `Authorization: Bearer <api_key>` for auth, and MUST return `"minimax"` from `Name()`.

#### Scenario MM-1a: Default base URL and name

- **GIVEN** a provider config with `type: minimax` and a valid `api_key`
- **WHEN** the factory constructs the provider
- **THEN** `Name()` returns `"minimax"`
- **AND** the HTTP transport targets `https://api.minimax.io/v1`

#### Scenario MM-1b: api_key absent — construction fails

- **GIVEN** a provider config with `type: minimax` and no `api_key`
- **WHEN** the factory constructs or the config is validated
- **THEN** a non-nil error is returned
- **AND** the error indicates `api_key` is required

---

### REQ-MM-2 — Sync path: think-tag stripping

In the sync `Chat()` path, all `<think>...</think>` segments MUST be removed from `ChatResponse.Content`. The text remaining after stripping is the answer. A response whose entire content is a single think block MUST yield `Content == ""`.

#### Scenario MM-2a: Think block stripped from sync response

- **GIVEN** a sync `ChatResponse.Content` of `"<think>step 1</think>The answer is 42"`
- **WHEN** `Chat()` returns
- **THEN** `ChatResponse.Content == "The answer is 42"`

#### Scenario MM-2b: Content-only think yields empty Content

- **GIVEN** a sync `ChatResponse.Content` of `"<think>only reasoning here</think>"`
- **WHEN** `Chat()` returns
- **THEN** `ChatResponse.Content == ""`

#### Scenario MM-2c: Response with no think tag is unchanged

- **GIVEN** a sync `ChatResponse.Content` of `"Just a plain answer"`
- **WHEN** `Chat()` returns
- **THEN** `ChatResponse.Content == "Just a plain answer"`

---

### REQ-MM-3 — Streaming path: split-safe tag detection

In the streaming `ChatStream()` path, `<think>` and `</think>` markers MUST be detected even when split across SSE chunks. Text outside think blocks MUST be emitted as `StreamEvent{Type: TextDelta}`; text inside think blocks MUST be emitted as `StreamEvent{Type: ReasoningDelta}`. The final accumulated `ChatResponse.Content` MUST contain no think text.

#### Scenario MM-3a: Inline think block routed correctly

- **GIVEN** SSE chunks `"<think>cot text</think>"` and `" answer"` arrive in sequence
- **WHEN** the stream is consumed
- **THEN** `ReasoningDelta` events carry `"cot text"`
- **AND** `TextDelta` events carry `" answer"`
- **AND** the final `Content` does NOT contain `"cot text"`

#### Scenario MM-3b: Marker split across chunks

- **GIVEN** SSE chunks `"<thi"`, `"nk>step 1</thi"`, `"nk> answer"` arrive in sequence
- **WHEN** the stream is consumed
- **THEN** `ReasoningDelta` carries `"step 1"`
- **AND** `TextDelta` carries `" answer"`
- **AND** no `<think>` or `</think>` text appears on either channel

#### Scenario MM-3c: Content-only think in stream yields no TextDelta

- **GIVEN** a stream whose entire content is `"<think>reasoning only</think>"`
- **WHEN** the stream is fully consumed
- **THEN** only `ReasoningDelta` events are emitted (no `TextDelta`)
- **AND** the final `Content == ""`

---

### REQ-MM-4 — Streaming: unclosed or malformed think tag

If the stream ends with an unclosed `<think>` (no matching `</think>`), the filter MUST force-flush buffered bytes on the `Done` event, route them as `ReasoningDelta`, and reset cleanly. No goroutine hang, no data loss, no panic.

#### Scenario MM-4a: Stream ends inside think block

- **GIVEN** a stream of `"<think>partial cot"` with no closing `</think>`
- **WHEN** the stream emits `Done`
- **THEN** the buffered `"partial cot"` is emitted as `ReasoningDelta`
- **AND** the stream result channel closes without blocking

#### Scenario MM-4b: Partial closing tag at stream end

- **GIVEN** the last chunk is `"answer</thi"` (partial `</think>` with no matching open)
- **WHEN** the stream emits `Done`
- **THEN** the buffered `"</thi"` is flushed to the current channel state without hanging

---

### REQ-MM-5 — Tag-detection buffer is bounded

The `thinkTagFilter` internal buffer MUST NOT exceed `len("</think>") == 8` bytes at any time. This bound ensures streaming latency is not degraded relative to the underlying OpenAI-compatible transport.

#### Scenario MM-5a: Buffer never exceeds 8 bytes

- **GIVEN** a `thinkTagFilter` processing an arbitrary stream of chunks
- **WHEN** any `feed()` call returns
- **THEN** the internal buffer length is ≤ 8 bytes

---

### REQ-MM-6 — Tool calls are unaffected by stripping

MiniMax-M2 wraps only chain-of-thought in `<think>`. Tool-call JSON appears after `</think>` and MUST NOT be dropped or corrupted by the think-tag filter. This is a documented spec assumption: if a future MiniMax model emits tool calls inside `<think>`, that content would be silently discarded.

#### Scenario MM-6a: Tool call JSON after think block reaches caller intact

- **GIVEN** a sync response whose content is `"<think>cot</think>"` and whose `tool_calls` array is populated
- **WHEN** `Chat()` returns
- **THEN** the `tool_calls` array is unchanged
- **AND** `Content == ""`

#### Scenario MM-6b: Streaming tool call delta after think block is not swallowed

- **GIVEN** a stream that emits `ReasoningDelta` events (inside think) followed by a `ToolCallDelta` event (after `</think>`)
- **WHEN** the stream is consumed
- **THEN** the `ToolCallDelta` is forwarded unchanged to the caller
