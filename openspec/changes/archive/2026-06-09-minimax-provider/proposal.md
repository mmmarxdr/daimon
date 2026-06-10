# Proposal: MiniMax Provider (first-class, think-tag stripping)

> Change: `minimax-provider` (DAIM-13). Chosen approach: **A** (confirmed by user).

## Intent

MiniMax-M2 always emits its chain-of-thought inline as `<think>...</think>`
inside `message.content`; no request param disables it and there is no separate
`reasoning_content` field. Routed today through the generic `openai` provider
type, that raw `<think>` block leaks into the chat UI. We add a first-class
`minimax` provider that strips/routes `<think>` so it never reaches the
assistant's main message — surfacing MiniMax cleanly, reusing daimon's existing
reasoning channel, and letting users spend their paid MiniMax coding-plan sub.

## Scope

### In Scope
- New `minimax` provider type wrapping `OpenAIProvider` (Ollama-style wrapper).
- Stateful `thinkTagFilter` stripping `<think>...</think>` in BOTH sync (`Chat`) and streaming (`ChatStream`) paths; in-think content is re-emitted as `ReasoningDelta`.
- Config: add `minimax` to `KnownProviders` and the 4 validation switch sites; enforce required `api_key` (no openai custom-base exemption).
- Factory dispatch: `case "minimax"`.
- Table-driven tests for sync strip, constructor, and `thinkTagFilter.feed()` (split/nested/tool/unclosed) per strict-TDD.

### Out of Scope
- Pricing tables for M2/M3 (follow-up).
- M3 1M-context tuning (follow-up).
- Disabling think via request params (proven impossible — verified in explore).
- The zero-code `openai` + custom `base_url` path stays working, untouched.

## Capabilities

> Researched `openspec/specs/`. Existing `reasoning-stream` covers the
> transport-level reasoning channel; this change adds a new provider on top.

### New Capabilities
- `minimax-provider`: dedicated `minimax` provider type that wraps OpenAI-compatible transport and strips/routes inline `<think>...</think>` from M2 content into the reasoning channel, in sync and streaming.

### Modified Capabilities
- `config`: `KnownProviders` and the 4 provider-type validation switches MUST accept `"minimax"`.

## Approach

Approach A. New `MiniMaxProvider` embeds `*OpenAIProvider`, overriding `Name()`,
`Chat()` (sync strip via `stripThinkContent()`), and `ChatStream()`.
`ChatStream()` does NOT re-parse SSE: it calls `p.OpenAIProvider.ChatStream(...)`,
then a goroutine routes each upstream delta through `thinkTagFilter.feed()` and
re-emits `TextDelta` vs `ReasoningDelta` on a fresh channel. On `Done`:
force-flush the filter, read `upstream.Response()`, `SetResponse()` with stripped
`Content`. `thinkTagFilter` is a pure state machine (`inThink`, ≤8-byte `buf` for
split-marker detection) — fully unit-testable without HTTP. Latency-invisible
(buffer max = `len("</think>")`).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/provider/minimax.go` | New | Provider + constructor + sync `Chat()` strip |
| `internal/provider/minimax_stream.go` | New | `thinkTagFilter` + `ChatStream()` rewire |
| `internal/provider/minimax_test.go` / `minimax_stream_test.go` | New | Table-driven tests + SSE integration |
| `internal/provider/factory.go:15` | Modified | Add `case "minimax"` |
| `internal/config/config.go:83,1117,1127,1142` | Modified | Add `"minimax"` to known + 3 validate switches |
| `internal/config/config_test.go` | Modified | Add `minimax` validation cases |
| existing `openai` / `ollama` paths | Untouched | Zero regression risk by isolation |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Tool call emitted inside `<think>` would be dropped | Low | M2 wraps CoT only; tool JSON follows `</think>`. Document as spec assumption. |
| Unclosed/malformed `<think>` | Low | `Done` handler force-flushes `buf` and resets; in-think-at-Done → `ReasoningDelta`. |
| Content-only think (`<think>...</think>` w/ no answer) | Low | Yields `Content=""`; agent loop already handles empty content via `StopReason`. |

## Rollback Plan

Revert the change branch. Existing configs using `provider: openai` + MiniMax
`base_url` continue to work (with the `<think>` leak), so no user is stranded.
No schema migration to undo; `minimax` is purely additive to known providers.

## Dependencies

- Existing reasoning channel (`StreamEventReasoningDelta` → `WriteReasoning()`) — already present.
- A real M2 coding-plan key (`sk-cp-...`) for the smoke test.

## Success Criteria

- [ ] `<think>` never appears in assistant `Content` in sync responses.
- [ ] `<think>` never appears in assistant `Content` in streaming (incl. split markers).
- [ ] `provider.type: minimax` passes config validation at all 4 sites; missing `api_key` rejected.
- [ ] Boot health check passes against MiniMax.
- [ ] A real MiniMax-M2 chat smoke test routes CoT to the reasoning channel only.
- [ ] `make test` green; existing `openai`/`ollama` tests unaffected.
