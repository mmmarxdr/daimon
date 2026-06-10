# Tasks: MiniMax Provider (think-tag stripping)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~260–310 (impl ~130, tests ~130–180) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | `thinkTagFilter` SM + constructor + sync Chat + stream rewire + config/factory | PR 1 | All additive; no existing behavior changed |

---

## Phase 1: Core State Machine — `thinkTagFilter` (RED → GREEN, no HTTP)

- [x] 1.1 **RED** — `internal/provider/minimax_stream_test.go`: write `TestLongestSuffixThatIsPrefixOf` table-driven test (cases: `"</thi"`→5, `"<"`→1, `"abc"`→0, `"</think"`→6, `""`→0). Satisfies MM-5a (buffer bound correctness). Run `make test` — must fail (symbol missing).
- [x] 1.2 **GREEN** — `internal/provider/minimax_stream.go`: implement private `longestSuffixThatIsPrefixOf(work, marker string) int` helper (iterate k from min(len(work), len(marker)-1) down to 1; return first k where `strings.HasPrefix(marker, work[len(work)-k:])`; else 0). `make test` passes 1.1.
- [x] 1.3 **RED** — `internal/provider/minimax_stream_test.go`: write `TestThinkTagFilter_feed` table-driven test; one `t.Run` per design case-table row: no-tag, full-tag-one-chunk, content-before-and-after, split-open, split-close, split-mid-both, multiple-tags, only-think, false-alarm-partial-prefix, tail-equals-marker-minus-1, nested-re-open. Each row feeds `[]string` deltas accumulating (textOut, reasonOut), then calls `flush()` and compares. Also one row for unclosed-`<think>`-at-flush: buf flushed to reasonOut. Satisfies MM-3a/3b/3c/4a/4b/5a and the design case table in ADR-2. `make test` — must fail.
- [x] 1.4 **GREEN** — `internal/provider/minimax_stream.go`: implement `thinkTagFilter` struct (`inThink bool`, `buf string`) + `feed(delta string) (string, string)` + `flush() (string, string)` per ADR-2 algorithm. Export constants `thinkOpen`/`thinkClose` as unexported. `make test` passes 1.3.

## Phase 2: Sync Stripping + `MiniMaxProvider` Struct (RED → GREEN)

- [x] 2.1 **RED** — `internal/provider/minimax_stream_test.go`: write `TestStripThinkContent` table-driven test (no-tag unchanged, full strip, only-think → `""`, content-around, unclosed → `""` since residual routes to reasoning). Satisfies MM-2a/2b/2c and ADR-2.5. `make test` — must fail.
- [x] 2.2 **GREEN** — `internal/provider/minimax_stream.go`: implement `stripThinkContent(s string) string` thin wrapper using `thinkTagFilter`: `feed` whole string, add `textTail` from `flush()`, return concatenated text (reasoning discarded). `make test` passes 2.1.
- [x] 2.3 **RED** — `internal/provider/minimax_test.go`: write `TestNewMiniMaxProvider` (valid config → non-nil, `Name()=="minimax"`, inner model set; nil/bad config propagates error). Write `TestMiniMaxProvider_InterfaceSatisfaction` (type-asserts to `Provider`, `StreamingProvider`, `ModelLister`, `EmbeddingProvider`). Satisfies MM-1a/1b and ADR-1/1.1. `make test` — must fail.
- [x] 2.4 **GREEN** — `internal/provider/minimax.go`: define `MiniMaxProvider` struct (embeds `*OpenAIProvider`), compile-time guards (`var _ Provider = ...`, `var _ StreamingProvider = ...`), `NewMiniMaxProvider(cfg config.ProviderConfig) (*MiniMaxProvider, error)` (calls `NewOpenAIProvider`, wraps in struct), `Name() string { return "minimax" }`. `make test` passes 2.3.
- [x] 2.5 **RED** — `internal/provider/minimax_test.go`: write `TestMiniMaxProvider_Chat_StripsThink` using `httptest.Server` returning a non-streaming JSON body with `<think>cot</think>The answer` in `message.content`; assert `resp.Content == "The answer"` and `resp.ToolCalls` unchanged. Also one sub-test with empty api_key hitting `NewMiniMaxProvider` directly to confirm construction fails (MM-1b). Satisfies MM-2a/2c/MM-6a. `make test` — must fail.
- [x] 2.6 **GREEN** — `internal/provider/minimax.go`: implement `Chat(ctx, req) (*ChatResponse, error)` (delegate to `p.OpenAIProvider.Chat`, set `resp.Content = stripThinkContent(resp.Content)`, return). `make test` passes 2.5.

## Phase 3: Streaming Rewire (RED → GREEN, httptest SSE)

- [x] 3.1 **RED** — `internal/provider/minimax_stream_test.go`: write `TestMiniMaxProvider_ChatStream_StripsThink` with `httptest.Server` emitting OpenAI-format SSE frames whose delta.content tokens deliberately split `<think>`/`</think>` across frames (e.g. `"<thi"`, `"nk>cot</thi"`, `"nk> answer"`). Assert: collected `TextDelta` == `"answer"`, collected `ReasoningDelta` == `"cot"`, `sr.Response().Content == " answer"` (no think text). Satisfies MM-3a/3b. `make test` — must fail.
- [x] 3.2 **RED** — Same test file: add sub-test `TestMiniMaxProvider_ChatStream_UnclosedThink`: stream ends with `<think>partial cot` and no closing tag; assert buffered `"partial cot"` is emitted as `ReasoningDelta` after Done, no hang. Satisfies MM-4a.
- [x] 3.3 **RED** — Same test file: add sub-test `TestMiniMaxProvider_ChatStream_ContentOnlyThink`: stream is `"<think>reasoning only</think>"` end-to-end; assert zero `TextDelta` events, final `Content == ""`. Satisfies MM-3c.
- [x] 3.4 **RED** — Same test file: add sub-test `TestMiniMaxProvider_ChatStream_ToolCallAfterThink`: stream emits `ToolCallDelta` events after `</think>`; assert tool events pass through unmodified to caller. Satisfies MM-6b.
- [x] 3.5 **GREEN** — `internal/provider/minimax_stream.go`: implement `filterStreamResult(upstream *StreamResult) *StreamResult` goroutine per ADR-3 (range upstream.Events, switch on StreamEventTextDelta/Done/default, `defer close(events)`, call `upstream.Response()` after range, `SetResponse` with stripped Content). Implement `MiniMaxProvider.ChatStream()` in `minimax_stream.go` (calls `p.OpenAIProvider.ChatStream`, wraps with `filterStreamResult`). `make test` passes 3.1–3.4.

## Phase 4: Config + Factory Wiring (RED → GREEN)

- [x] 4.1 **RED** — `internal/config/config_test.go`: extend `IsKnownProvider` table row `{"minimax", true}`. Extend validate table: (a) `type: minimax` + api_key → valid (CM-1b/CM-2a); (b) `type: minimax` + empty api_key → error containing `api_key` (CM-2b / ADR-5.1 intentional no-op); (c) v1 legacy `provider.type: minimax` + api_key → valid (CM-1c); (d) `fallback.type: minimax` + api_key → valid (CM-1d). Satisfies CONFIG-MM-1/2. `make test` — must fail (IsKnownProvider + switch cases missing).
- [x] 4.2 **GREEN** — `internal/config/config.go`: add `"minimax"` to `KnownProviders` slice (line 83). Add `"minimax"` to switch at line 1117 (v2 active-provider), line 1127 (v1 legacy), line 1142 (Fallback.Type). Do NOT touch api_key gate at line 1113. `make test` passes 4.1.
- [x] 4.3 **GREEN** — `internal/provider/factory.go`: add `case "minimax":` block mirroring the `"ollama"` case (calls `NewMiniMaxProvider(cfg)`, wraps error). `make test` still passes (factory covered transitively by constructor tests).

## Phase 5: Concrete-Type Assertion Grep + Final Gate

- [x] 5.1 **Grep** — Search entire repo for `.(*OpenAIProvider)` concrete type assertions. If found outside `internal/provider/` (e.g. in agent loop), raise as a blocker; if found only in test files or factory, document as acceptable. Satisfies design open question / ADR-1.1 risk. File: any — read-only grep, no edit expected.
- [x] 5.2 **Verify** — Run `make test` clean from repo root. All new and existing tests must pass. Confirm `internal/provider/ollama*` and `internal/provider/openai*` tests are green (zero regression).
- [x] 5.3 **Note** — Document that real MiniMax-M2 smoke test (requires `sk-cp-...` key) is manual-only; any test gating on that key must `t.Skip` under `-short` or an env var — NOT part of CI.
