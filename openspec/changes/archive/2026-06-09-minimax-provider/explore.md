# Exploration: minimax-provider

> Phase: explore. Change: `minimax-provider` (DAIM-13). Investigated 2026-06-09.
> Grounded against `internal/provider/` + `internal/config/` on branch `main`.

## Goal

Add a dedicated `minimax` LLM provider so MiniMax (OpenAI-compatible API) is a
first-class provider AND its reasoning leakage is fixed. MiniMax-M2 ALWAYS emits
its chain-of-thought inline as `<think>...</think>` inside `message.content`.
Today MiniMax is routed through the generic `openai` provider type, so the raw
`<think>...</think>` renders in the chat UI. The dedicated provider must
strip/route `<think>` content so it never appears in the assistant's main
message.

## Verified ground truth (do not re-test)

- MiniMax chat API is OpenAI-compatible: `POST https://api.minimax.io/v1/chat/completions`,
  `Authorization: Bearer`, SSE streaming, OpenAI-format tools. Coding-plan key
  (`sk-cp-...`) authenticates against this endpoint (HTTP 200, 2026-06-09).
- MiniMax-M2 always emits `<think>...</think>` in `message.content`. Request
  params `thinking:{type:disabled}` and `reasoning_effort:none` do NOT suppress
  it. No separate `reasoning_content` field is returned — everything is in `content`.
- Wiring via the existing `openai` type + custom `base_url` works end-to-end
  (boot health check passes, chat works) but leaks `<think>`.

## Verified current behavior (code-grounded)

### The leak

- **Sync** — `internal/provider/openai.go:329` (`parseOpenAIResponse`): the whole
  `message.content` (including `<think>...</think>`) is assigned to
  `ChatResponse.Content` with no filtering.
- **Stream** — `internal/provider/openai_stream.go:213-219`: every content delta
  (including `<think>` / `</think>` tokens) is appended to `textContent` and
  emitted as `StreamEventTextDelta`, flowing to `sw.WriteChunk(ev.Text)` at
  `internal/agent/stream.go:154`.

### The reasoning channel ALREADY exists

- `StreamEventReasoningDelta` (`internal/provider/stream.go:28`) routes to
  `sw.WriteReasoning()` (`internal/agent/stream.go:132`) and is NEVER accumulated
  into `ChatResponse.Content`. Infrastructure is ready; only the routing is wrong
  for MiniMax.
- Contrast: OpenRouter exposes reasoning in a separate `delta.reasoning` /
  `delta.reasoning_content` field (`openrouter_stream.go:32`), never mixed into
  `delta.content`. MiniMax embeds it inside `content` — a different protocol.

### Delegation template

- `ollama.go` (+ `ollama_list.go`, `ollama_stream.go`) shows the wrapper pattern
  over a base provider (override `Name()`, capability flags, streaming). This is
  the template for a thin `minimax` provider.

### Dispatch + validation seams

- `internal/provider/factory.go:15` — provider `type` switch/dispatch.
- `internal/provider/registry.go:53-65` — thinking-config type-assertion block;
  no change needed (MiniMax has no structured thinking config; always-on inline tags).
- `internal/config/config.go:83` — `KnownProviders` slice.
- `internal/config/config.go:1117` — v2 active provider validate switch.
- `internal/config/config.go:1127` — v1 legacy provider validate switch.
- `internal/config/config.go:1142` — `Fallback.Type` validate switch.
  (Note: validation has **4** sites, not 3.)

## Approaches

| Approach | Description | Pros | Cons | Effort |
|---|---|---|---|---|
| **A. New `MiniMaxProvider` wrapping `OpenAIProvider`** | Embed `*OpenAIProvider`, override `Name()`, `Chat()` (sync strip), `ChatStream()` (rewire events via stateful `thinkTagFilter`). Follows the `OllamaProvider` pattern. | No change to existing openai/ollama paths; split-tag logic isolated + unit-testable; established pattern. | Needs a `filterStreamResult()` that rewraps the `*StreamResult` channel. ~200 LOC incl. tests. | Medium |
| **B. Flag on `OpenAIProvider`** (`stripThinkTags bool`) | `WithThinkTagStripping()` option filters inline in both paths; factory sets it for minimax. | No new type. | Modifies existing sync+stream paths → regression risk for openai/ollama; Ollama inherits the flag confusingly; harder to test in isolation. | Medium-Low (higher risk) |
| **C. Middleware in agent loop** | Post-process `StreamEventTextDelta` for flagged providers. | Centralized. | Wrong layer; buffering for tag detection breaks streaming latency; can't cleanly filter the sync path. | High |

## Recommendation: Approach A

New `MiniMaxProvider` wrapping `OpenAIProvider`, with stream-event rewiring.
`ChatStream()` does NOT re-implement SSE parsing: it calls
`p.OpenAIProvider.ChatStream(...)`, then a goroutine consumes the upstream
events, routes each delta through `thinkTagFilter.feed()`, and re-emits the
correct event type (`TextDelta` vs `ReasoningDelta`) on a fresh channel. On
`StreamEventDone`: force-flush the filter, read `upstream.Response()`,
`SetResponse()` with `Content` stripped.

`thinkTagFilter` is a pure state machine (`inThink bool`, `buf` up to 8 bytes for
partial `</think>` detection, output builders) — fully table-driven testable
without HTTP.

## Streaming `<think>`-split problem

SSE chunks can split markers: `"<thi"` / `"nk>cot"` / `"</thi"` / `"nk> answer"`.
`thinkTagFilter.feed(delta) (textOut, reasonOut string)`:
- prepend `buf` to `delta`, clear `buf`;
- `!inThink`: scan for `<think>`, flush prefix to `textOut`, keep partial prefix
  (≤7 bytes) in `buf` if no full match;
- `inThink`: scan for `</think>`, flush preceding to `reasonOut`, keep partial
  suffix in `buf`;
- on Done: force-flush `buf` to current state's output; `inThink` at Done →
  `ReasoningDelta`.
Buffer max = `len("</think>")` = 8 bytes → latency-invisible.

## Open questions / risks

1. **Tool calls inside `<think>`** — M2 wraps CoT only; tool JSON appears after
   `</think>`. If wrong, a tool call inside `<think>` would be dropped. Low risk;
   document as a spec assumption.
2. **Content-only-think** (`"<think>...</think>"` with nothing after) — both paths
   produce `Content = ""` with reasoning in the channel; agent loop handles empty
   content via `StopReason`. No issue.
3. **Malformed/unclosed `<think>`** — `Done` handler must force-flush + reset.
4. **Registry** — no change needed (no structured thinking config).
5. **Config api_key** — MiniMax always requires api_key; the `openAIWithCustomBase`
   exemption (`config.go:1112`) only applies to `activeProv == "openai"`, so the
   `minimax` path correctly enforces the key.

## Implementation seams

| File | Change |
|---|---|
| `internal/provider/minimax.go` | NEW: `MiniMaxProvider` + `NewMiniMaxProvider(cfg)` + `Name()` + `Chat()` (sync strip via `stripThinkContent()`) |
| `internal/provider/minimax_stream.go` | NEW: `thinkTagFilter` + `ChatStream()` override (rewire upstream stream) |
| `internal/provider/minimax_test.go` | NEW: table-driven sync tests + constructor validation |
| `internal/provider/minimax_stream_test.go` | NEW: table-driven `thinkTagFilter.feed()` tests (split/nested/tool turns) + SSE-server integration |
| `internal/provider/factory.go:15` | ADD `case "minimax": return NewMiniMaxProvider(cfg)` |
| `internal/config/config.go:83` | ADD `"minimax"` to `KnownProviders` |
| `internal/config/config.go:1117,1127,1142` | ADD `"minimax"` to the 3 validate switches |
| `internal/config/config_test.go` | ADD `"minimax"` cases for `IsKnownProvider` + `validate()` |

**Next recommended**: sdd-propose.
