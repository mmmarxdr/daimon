# Design: MiniMax Provider (think-tag stripping)

> Change: `minimax-provider` (DAIM-13). Phase: design. Approach **A** (LOCKED).
> Grounded against `internal/provider/` + `internal/config/` on branch `main`, 2026-06-09.

## Scope of this document

The HOW at the architectural level. Defines the `MiniMaxProvider` shape, the
`thinkTagFilter` state machine (core artifact), the streaming and sync rewires,
config/factory wiring, and the strict-TDD test strategy. Task breakdown is the
`sdd-tasks` phase, not here.

## Architecture overview

```
agent loop
   │  (treats MiniMaxProvider identically to OpenAIProvider via interfaces)
   ▼
MiniMaxProvider  ──embeds──▶  *OpenAIProvider   (transport: SSE, tools, embed, list)
   │  Chat()       → OpenAIProvider.Chat() + stripThinkContent(resp.Content)
   │  ChatStream() → OpenAIProvider.ChatStream() + filterStreamResult(...)
   │  Name()       → "minimax"
   ▼
thinkTagFilter   (pure state machine; shared by sync + stream; no HTTP)
```

The wrapper pattern mirrors `OllamaProvider` (`ollama.go:15`,
`ollama_stream.go:67`). The key DIFFERENCE from Ollama: Ollama OVERRIDES
`ChatStream` to hit a different endpoint (`/api/chat`); MiniMax does NOT — it
REUSES `OpenAIProvider.ChatStream` (the MiniMax endpoint is OpenAI-compatible
SSE, verified in explore) and only post-processes the event stream. No SSE
re-parsing.

---

## ADR-1 — `MiniMaxProvider` struct shape: embed `*OpenAIProvider`

**Decision**: Embed `*OpenAIProvider` (not hold as a named field), exactly like
`OllamaProvider` (`ollama.go:15-18`).

```go
// minimax.go
var _ Provider = (*MiniMaxProvider)(nil)
var _ StreamingProvider = (*MiniMaxProvider)(nil) // compile-time guard, see ADR-1.1

type MiniMaxProvider struct {
    *OpenAIProvider
}

func NewMiniMaxProvider(cfg config.ProviderConfig) (*MiniMaxProvider, error) {
    inner, err := NewOpenAIProvider(cfg)
    if err != nil {
        return nil, err
    }
    return &MiniMaxProvider{OpenAIProvider: inner}, nil
}

func (p *MiniMaxProvider) Name() string { return "minimax" }
```

**Why embed, not field**: embedding promotes ALL of `OpenAIProvider`'s methods
automatically. We override ONLY `Name()`, `Chat()`, and `ChatStream()`.
Everything else delegates for free:

| Method / interface | Source | Behaviour |
|---|---|---|
| `Model()` | promoted `openai.go:171` | returns inner `p.model` |
| `SupportsTools()` | promoted `openai.go:172` | `true` (MiniMax supports OpenAI tools, explore:20) |
| `SupportsMultimodal()` | promoted `openai.go:173` | `true` (M2 is multimodal-capable; no override needed, unlike Ollama which forces `false`) |
| `SupportsAudio()` | promoted `openai.go:174` | `true` |
| `HealthCheck()` | promoted `openai.go:182` | reuses OpenAI health check against MiniMax base_url |
| `Embed()` / `EmbedBatch()` | promoted `openai.go:512,585` | `EmbeddingProvider` + `BatchEmbeddingProvider` satisfied for free |
| `ListModels()` | promoted `openai.go:651` | `ModelLister` satisfied for free |
| `Config()` | promoted `openai.go:178` | `ConfigurableProvider` satisfied for free |

**Capabilities are NOT overridden** — that is the deliberate difference from
`OllamaProvider`. MiniMax-M2 is a full-capability model; we only intercept the
`<think>` content channel, not the capability surface.

### ADR-1.1 — interface identity (the load-bearing guarantee)

Because Go embedding promotes methods, `MiniMaxProvider` satisfies the SAME set
of interfaces `*OpenAIProvider` does:

- `Provider` (overridden `Name`/`Chat` + promoted rest)
- `StreamingProvider` (`stream.go:126`) — via overridden `ChatStream`
- `EmbeddingProvider`, `BatchEmbeddingProvider`, `ModelLister`, `ConfigurableProvider` — all promoted

This is REQUIRED so the agent loop's type assertions
(`prov.(StreamingProvider)`, `prov.(ModelLister)`, etc.) succeed and the loop
treats MiniMax identically to OpenAI. Add compile-time guards:
`var _ StreamingProvider = (*MiniMaxProvider)(nil)` and
`var _ Provider = (*MiniMaxProvider)(nil)`. (Ollama only asserts `Provider` at
`ollama.go:6`; we add `StreamingProvider` too because our override is the whole
point and a signature drift must fail the build.)

**Open question for tasks**: confirm no agent-loop code does a CONCRETE type
assertion `prov.(*OpenAIProvider)` that would now miss MiniMax. Grep for
`.(*OpenAIProvider)` during tasks. Low risk (interfaces are used everywhere),
but worth one grep.

---

## ADR-2 — `thinkTagFilter` state machine (CORE ARTIFACT)

A pure, allocation-light state machine that strips `<think>...</think>` spans,
routing in-think bytes to a reasoning output and everything else to a text
output. Used by BOTH paths (ADR-2.5). No HTTP, no channels — fully unit-testable.

### Constants

```go
const (
    thinkOpen  = "<think>"   // len 7
    thinkClose = "</think>"  // len 8
)
```

### Fields

```go
type thinkTagFilter struct {
    inThink bool   // true after a complete <think>, false after </think>
    buf     string // retained tail that MIGHT be a partial marker prefix
}
```

`buf` holds at most `len(thinkClose)-1 == 7` bytes (see ADR-2.3). No big
builders are stored — `feed` returns the outputs directly and the caller
decides what to do with them (emit events vs. accumulate a string).

### `feed(delta string) (textOut, reasonOut string)` algorithm

The marker we are hunting depends on state: `<think>` when `!inThink`,
`</think>` when `inThink`. The routing target for non-marker bytes also depends
on state: text when `!inThink`, reason when `inThink`.

```
feed(delta):
    work := buf + delta        // prepend retained tail
    buf = ""
    var textOut, reasonOut strings.Builder
    loop:
        marker := inThink ? thinkClose : thinkOpen
        idx := strings.Index(work, marker)
        if idx >= 0:
            // bytes before the marker belong to the CURRENT state's channel
            emit(work[:idx]) → (inThink ? reasonOut : textOut)
            work = work[idx+len(marker):]
            inThink = !inThink           // toggle state
            continue loop                // re-scan remainder for more/next markers
        else:
            // no full marker. Emit everything EXCEPT a possible partial-marker tail.
            keep := longestSuffixThatIsPrefixOf(work, marker)   // 0..len(marker)-1
            flushable := work[:len(work)-keep]
            emit(flushable) → (inThink ? reasonOut : textOut)
            buf = work[len(work)-keep:]  // retain partial prefix for next feed
            break loop
    return textOut.String(), reasonOut.String()
```

`emit(s)` skips empty strings (no zero-length writes).

This single loop handles every case below by construction.

### Case behaviour (this is the test table — see ADR-6)

| Case | Input sequence | Expected (textOut, reasonOut) cumulative |
|---|---|---|
| **no tag** | `feed("hello world")` | `("hello world", "")` |
| **full tag one chunk** | `feed("a<think>cot</think>b")` | `("ab", "cot")` |
| **content before+after** | `feed("pre<think>x</think>post")` | `("prepost", "x")` |
| **split open** | `feed("<thi")`, `feed("nk>cot")` | feed1 `("","")` (buf=`<thi`), feed2 `("","cot")` |
| **split close** | already inThink; `feed("c</thi")`, `feed("nk>ans")` | feed1 `("","c")` (buf=`</thi`), feed2 `("ans","")` |
| **split mid both** | `feed("p<th")`,`feed("ink>r</th")`,`feed("ink>q")` | `("p","")`,`("","r")`,`("q","")` |
| **multiple tags one delta** | `feed("a<think>x</think>b<think>y</think>c")` | `("abc","xy")` |
| **only think, no answer** | `feed("<think>all</think>")` | `("","all")` → `Content==""` (risk #2 handled) |
| **nested / re-open** | `feed("<think>a<think>b</think>c")` | see ADR-2.4 |
| **partial-prefix false alarm** | `feed("a<th")`,`feed("ing else")` | feed1 `("a","")` buf=`<th`; feed2: `<thing else` has no `<think>` → emit `<thing else` as text. Result feed2 `("<thing else","")` |
| **tail equals marker-minus-1** | `feed("x<thi")` | `("x","")`, buf=`<thi` (4 bytes ≤ 6) |

The "false alarm" case is the critical correctness check: a retained partial
prefix that turns out NOT to be a real marker must be re-emitted verbatim to the
current channel on the next feed. The `buf + delta` prepend guarantees this.

### `flush() (textOut, reasonOut string)` — stream end

```go
func (f *thinkTagFilter) flush() (string, string) {
    if f.buf == "" {
        return "", ""
    }
    out := f.buf
    f.buf = ""
    if f.inThink {
        return "", out   // unclosed <think> at EOF → reasoning (risk #3)
    }
    return out, ""       // dangling partial-<think> prefix → literal text
}
```

`flush` emits whatever is held in `buf` (a partial marker that never completed)
to the current state's channel. If the stream ended mid-think (unclosed
`<think>`), residual goes to REASONING, never to `Content` — satisfying proposal
risk row 3 and explore open-question 3.

### ADR-2.3 — why `buf` max is `len("</think>")` and the retain-tail rule

`longestSuffixThatIsPrefixOf(work, marker)` returns the length of the longest
suffix of `work` that is also a prefix of `marker`. Examples for
`marker="</think>"`: `work` ending in `"</thi"` → 5; ending in `"<"`/`"</"` → also
matches (`<` is a prefix of `</think>`). Max possible retained length is
`len(marker)-1` because a suffix of full marker length would have been caught by
`strings.Index` as a complete match.

- `!inThink`: marker `<think>` (7) → retain ≤ 6 bytes.
- `inThink`: marker `</think>` (8) → retain ≤ 7 bytes.

So `buf` never exceeds **7 bytes**. This bounds memory and proves the filter is
**latency-invisible**: at most 7 bytes of any visible-text chunk are delayed one
feed, and those 7 bytes are exactly the ones that might be the start of a tag we
must not leak. Real text never sits in `buf` longer than one delta.

Implementation note for tasks: `longestSuffixThatIsPrefixOf` is a tiny helper —
iterate `k` from `min(len(work), len(marker)-1)` down to `1`, return first `k`
where `strings.HasPrefix(marker, work[len(work)-k:])`; else `0`. Keep it private
and table-test it directly too.

### ADR-2.4 — nested / re-opened `<think>` (DOCUMENTED behaviour)

The state machine is a 2-state toggle, NOT a counter. Given
`<think>a<think>b</think>c`:

1. `<think>` → enter think.
2. inner `<think>` while `inThink` is TRUE: we are scanning for `</think>`, so
   `<think>` is NOT a marker — its 7 bytes flow to `reasonOut` as literal text
   (`a<think>b`).
3. `</think>` → exit think. `c` → text.

Result: `text="c"`, `reason="a<think>b"`. **Decision**: treat nested `<think>` as
LITERAL inside reasoning. Rationale: M2 does not nest (explore:20-23 — single CoT
block); a counter would add complexity for a case the model never emits, and
"literal inside reasoning" never leaks CoT into `Content` (the actual safety
goal). Documented as a spec assumption.

### ADR-2.5 — sync reuse: `stripThinkContent` shares the SAME state machine

**Decision**: `stripThinkContent` is a thin wrapper that feeds the whole string
once then flushes, reusing `thinkTagFilter`. ONE implementation, ONE set of
correctness guarantees.

```go
// minimax.go (or minimax_stream.go, co-located with the filter)
func stripThinkContent(s string) string {
    var f thinkTagFilter
    text, _ := f.feed(s)        // reason discarded in sync path (no channel)
    textTail, _ := f.flush()    // pick up any unclosed/partial tail as text...
    // NOTE: on unclosed <think>, flush routes residual to reasonOut, so textTail==""
    return text + textTail
}
```

Sync `Chat()` has NO reasoning channel to write to (the `ChatResponse` struct has
only `Content`, `provider.go:92-97`), so the reasoning output is discarded —
which is exactly the desired behaviour: strip `<think>` from `Content`, drop the
CoT. Streaming keeps both outputs because it HAS a reasoning event channel.

**Why share, not duplicate**: a split-marker bug fixed in one path must not
silently persist in the other. Single source of truth. The unit test table
(ADR-6) drives `feed`/`flush` directly, so both callers inherit the proof.

---

## ADR-3 — `ChatStream()` rewire (filterStreamResult)

**Decision**: `MiniMaxProvider.ChatStream` calls
`p.OpenAIProvider.ChatStream(ctx, req)` to get the upstream `*StreamResult`, then
wraps it with `filterStreamResult(upstream)` returning a FRESH `*StreamResult`.

```go
// minimax_stream.go
func (p *MiniMaxProvider) ChatStream(ctx context.Context, req ChatRequest) (*StreamResult, error) {
    upstream, err := p.OpenAIProvider.ChatStream(ctx, req)
    if err != nil {
        return nil, err   // construction/HTTP errors surface synchronously, as today
    }
    return filterStreamResult(upstream), nil
}

func filterStreamResult(upstream *StreamResult) *StreamResult {
    sr, events := NewStreamResult(32)   // same buffer size as openai_stream.go:167
    go func() {
        defer close(events)
        var f thinkTagFilter
        for ev := range upstream.Events {     // ranges until upstream closes its channel
            switch ev.Type {
            case StreamEventTextDelta:
                text, reason := f.feed(ev.Text)
                if reason != "" {
                    events <- StreamEvent{Type: StreamEventReasoningDelta, Text: reason}
                }
                if text != "" {
                    events <- StreamEvent{Type: StreamEventTextDelta, Text: text}
                }
            case StreamEventDone:
                // flush before forwarding Done so trailing buf reaches the channel
                text, reason := f.flush()
                if reason != "" {
                    events <- StreamEvent{Type: StreamEventReasoningDelta, Text: reason}
                }
                if text != "" {
                    events <- StreamEvent{Type: StreamEventTextDelta, Text: text}
                }
                events <- ev   // forward Done
            default:
                // ReasoningDelta, ToolCallStart/Delta/End, Usage, Error → pass through
                events <- ev
            }
        }
        // upstream channel closed. Read its assembled response, strip Content, republish.
        resp, rerr := upstream.Response()   // drains nothing (already drained) + waits done
        if rerr != nil {
            sr.SetResponse(nil, rerr)
            return
        }
        if resp != nil {
            stripped := *resp                       // shallow copy
            stripped.Content = stripThinkContent(resp.Content)
            sr.SetResponse(&stripped, nil)
        } else {
            sr.SetResponse(resp, nil)
        }
    }()
    return sr, nil
}
```

### Event mapping rules

- `StreamEventTextDelta` → `feed()` → emit 0..2 events: ReasoningDelta first (if
  any), then TextDelta (if any). Order: reasoning-before-text within a single
  delta is arbitrary but consistent; pick reasoning-first so the UI shows
  thinking before answer for the chunk that contains the `</think>` transition.
- `StreamEventDone` → `flush()` (emit residual), THEN forward the original Done.
- ALL other events (`ReasoningDelta`, `ToolCallStart`, `ToolCallDelta`,
  `ToolCallEnd`, `Usage`, `Error`) → passed through UNCHANGED. Tool calls are
  NEVER routed through the filter (proposal risk row 1 / explore open-question 1:
  M2 emits tool JSON only AFTER `</think>`, so tool deltas never contain tag
  bytes; even if they did, we deliberately do not touch them).

### ADR-3.1 — goroutine lifecycle & cancellation

- The wrapper goroutine's lifetime is bound to `range upstream.Events`. When the
  caller cancels `ctx`, the UPSTREAM OpenAI goroutine (`openai_stream.go:169`)
  observes it via the HTTP request context, stops, emits its terminal event, and
  closes `upstream.Events`. Our `range` then exits naturally. We do NOT need our
  own `ctx` select — cancellation propagates through the upstream channel close.
- We `defer close(events)` so OUR channel always closes when the goroutine
  returns. No leak.

### ADR-3.2 — error propagation

- Mid-stream errors arrive as `StreamEventError` on `upstream.Events` and are
  passed through unchanged (default case), so the consumer sees them exactly as
  with OpenAI. The upstream sets its own `resp/err` via its `SetResponse`.
- After the range ends, `upstream.Response()` returns the assembled
  `(resp, err)`. If `err != nil` (e.g. parse failure, `openai_stream.go:302`), we
  forward it via `sr.SetResponse(nil, err)`. The consumer's `Response()` then
  yields the same error — error semantics preserved end-to-end.

### ADR-3.3 — `Response()` / `SetResponse()` mechanics (verified)

- `StreamResult.Response()` (`stream.go:88-94`) drains `Events` then blocks on
  `<-done`. Since OUR goroutine already drained `upstream.Events` via `range`,
  the drain loop inside `upstream.Response()` is a no-op and it returns
  immediately with the upstream's assembled result.
- `SetResponse` (`stream.go:112-118`) is `once`-guarded; we call it exactly once
  on the fresh `sr`. The consumer of our `sr` calls `sr.Response()` to get the
  STRIPPED content.
- **Critical ordering**: we forward `StreamEventDone` and then close `events`
  BEFORE calling `sr.SetResponse`. A consumer that does `for range sr.Events {}`
  then `sr.Response()` (the agent-loop pattern) will unblock correctly: our
  `events` close lets their range end, and our `SetResponse` (called right after)
  closes `done`. Matches OpenAI's own ordering (`openai_stream.go:304` sets
  response after `close(events)` deferred — note OpenAI defers the close, we
  defer it too, and call SetResponse after the loop; identical lifecycle).

---

## ADR-4 — `Chat()` sync rewire

**Decision**: delegate to inner `Chat`, strip only `Content`, pass everything
else through untouched.

```go
// minimax.go
func (p *MiniMaxProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
    resp, err := p.OpenAIProvider.Chat(ctx, req)
    if err != nil {
        return nil, err
    }
    resp.Content = stripThinkContent(resp.Content)
    return resp, nil
}
```

- `parseOpenAIResponse` (`openai.go:308`) returns a fresh `*ChatResponse`, so
  mutating `resp.Content` in place is safe (no shared aliasing).
- `ToolCalls`, `Usage`, `StopReason` are PASSED THROUGH untouched
  (`provider.go:92-97`). Tool-call-only responses have `Content==""`;
  `stripThinkContent("")` returns `""` — no-op, tool calls intact (proposal
  success criterion + risk row 1).
- Content-only-think (`<think>...</think>` with no answer, risk row 2) →
  `stripThinkContent` yields `""`; the agent loop already handles empty content
  via `StopReason` (explore open-question 2).

---

## ADR-5 — Config + factory wiring (exact edits)

### Factory (`factory.go:15` switch)

Add a case mirroring the ollama case (`factory.go:28-33`):

```go
case "minimax":
    p, err := NewMiniMaxProvider(cfg)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize minimax provider: %w", err)
    }
    return p, nil
```

### Config — `KnownProviders` (`config.go:83`)

```go
var KnownProviders = []string{"anthropic", "openai", "gemini", "openrouter", "ollama", "minimax"}
```

### Config — validation switches (THREE switch sites, see correction below)

Add `"minimax"` to each `switch` case list:

- `config.go:1117` — v2 active-provider validate.
- `config.go:1127` — v1 legacy `Provider.Type` validate.
- `config.go:1142` — `Fallback.Type` validate.

```go
case "anthropic", "gemini", "openrouter", "openai", "ollama", "minimax", "test", "test_provider":
```

(For the Fallback switch at `:1142`, keep the trailing `""` empty-string case.)

### ADR-5.1 — api_key enforcement (no edit needed; this is the "4th site")

The proposal/explore call out "4 validation sites". Code-grounded reality: there
are **3 `switch` statements** (1117/1127/1142) PLUS the api_key gate at
`config.go:1113`:

```go
openAIWithCustomBase := activeProv == "openai" && creds.BaseURL != ""
if creds.APIKey == "" && activeProv != "ollama" && !openAIWithCustomBase {
    return fmt.Errorf("provider.api_key is required")
}
```

The custom-base exemption applies ONLY when `activeProv == "openai"`. Because
`minimax != "openai"` and `minimax != "ollama"`, MiniMax **automatically requires
api_key** with ZERO code change at `:1113`. This is the desired behaviour
(MiniMax always needs a key, explore:20). The "4th site" is this gate, and the
correct design action is to deliberately NOT touch it — a no-op that must be
covered by a test asserting `provider.type: minimax` + empty `api_key` is
REJECTED (ADR-6, config test case).

**Correction flagged for tasks**: the edit count is "3 switch edits + 0 api_key
edits + KnownProviders". The proposal table listing 4 validate edits should read
3 switch edits; the 4th is an intentional no-op verified by test.

---

## ADR-6 — Test strategy (strict TDD, `make test`)

RED-first ordering. Each test file is table-driven with `t.Run` subtests
(go-testing standard).

### Tier 1 — pure unit, no HTTP (write FIRST, RED → GREEN)

`minimax_stream_test.go` (filter lives here):

1. `TestThinkTagFilter_feed` — table-driven, ONE row per ADR-2 case table row.
   Each row: name, `[]string` input deltas (to exercise splits across feeds),
   expected concatenated `textOut`, expected concatenated `reasonOut`, and
   expected `flush()` outputs. Drive: construct filter, feed each delta
   accumulating outputs, then flush, compare. This is the keystone test — it
   proves split/nested/multiple/false-alarm/unclosed all at once.
2. `TestLongestSuffixThatIsPrefixOf` — table-driven helper test (`work`, `marker`
   → expected k). Explicit success AND boundary cases (`"</thi"`→5, `"<"`→1,
   `"abc"`→0, full marker→handled by Index not here).
3. `TestStripThinkContent` — table-driven, reuses the same conceptual cases at
   the string level (no-tag, full, only-think→"", content-around, unclosed→strips
   to "" because residual goes to reasoning). Proves ADR-2.5 sharing.

`minimax_test.go`:

4. `TestNewMiniMaxProvider` — constructor: valid cfg → non-nil, `Name()=="minimax"`,
   inner model wired; propagates `NewOpenAIProvider` error on bad cfg.
5. `TestMiniMaxProvider_InterfaceSatisfaction` — compile-time guards are enough,
   but add an explicit assert that the value satisfies `StreamingProvider`,
   `ModelLister`, `EmbeddingProvider` (type-assert in test) to lock ADR-1.1.

### Tier 2 — `httptest` SSE server (integration, still in-package)

`minimax_stream_test.go`:

6. `TestMiniMaxProvider_ChatStream_StripsThink` — spin an `httptest.Server`
   emitting an OpenAI-format SSE stream whose `delta.content` tokens, when
   concatenated, contain `<think>cot</think>answer` — AND deliberately SPLIT the
   markers across SSE frames (`"<thi"`, `"nk>"`, ...). Point a MiniMaxProvider's
   base_url at it. Assert: collected `TextDelta` text == `"answer"`,
   collected `ReasoningDelta` text == `"cot"`, and final `sr.Response().Content
   == "answer"`. Also one row with a tool call after `</think>` asserting the
   tool events pass through.

`minimax_test.go`:

7. `TestMiniMaxProvider_Chat_StripsThink` — `httptest` server returning a
   non-streaming JSON body with `<think>...</think>` in `message.content`. Assert
   `resp.Content` has no `<think>` and tool calls (if present) pass through.

### Tier 3 — config (`config_test.go`, modified)

8. Extend existing `IsKnownProvider` table with `{"minimax", true}`.
9. Extend `validate()` table: `provider.type: minimax` + api_key set → valid;
   `provider.type: minimax` + empty api_key → error `provider.api_key is
   required` (locks ADR-5.1). Cover fallback.type `minimax` too.

### Out of scope for automated tests

Real MiniMax-M2 smoke test (needs `sk-cp-...` key) — manual, per success
criteria; mark any such test `t.Skip` under `-short` or gate behind an env var,
NOT part of `make test` CI green.

### RED-first sequence

filter feed test (1) → implement filter → strip test (3) → wire
stripThinkContent → constructor test (4) → struct+constructor → sync Chat test
(7) → Chat override → stream test (6) → ChatStream+filterStreamResult → config
tests (8,9) → config edits → factory case last (covered indirectly by
constructor/integration).

---

## Open questions for the tasks phase

1. **Concrete type assertion grep**: confirm nothing does `.(*OpenAIProvider)` in
   the agent loop that would skip MiniMax (ADR-1.1). One grep; low risk.
2. **Reasoning-vs-text emit order within one delta**: design picks reasoning-first.
   Confirm the agent stream writer (`agent/stream.go:132,154`) does not assume
   text-before-reasoning ordering. Low risk — they are separate sinks
   (`WriteReasoning` vs `WriteChunk`).
3. **Edit-count correction**: tasks must record "3 switch edits + KnownProviders
   + 1 deliberate api_key no-op", not the proposal's "4 validate edits" (ADR-5.1).
4. **Model defaults**: should `minimax` get a default model constant (like
   `openAIDefaultModel`)? Inner `NewOpenAIProvider` defaults to `gpt-4o` which is
   wrong for MiniMax. Decide in tasks whether to require explicit `model` in
   config or add a MiniMax default. Flagged — likely "require explicit model,
   validated by config".

## Decisions summary (ADR index)

- ADR-1: embed `*OpenAIProvider`; override only `Name`/`Chat`/`ChatStream`; no capability overrides.
- ADR-1.1: interface identity via embedding + compile-time `StreamingProvider` guard.
- ADR-2: `thinkTagFilter` 2-state toggle, `buf` ≤7 bytes, `feed`/`flush`, retain-tail rule.
- ADR-2.4: nested `<think>` treated as literal inside reasoning (documented).
- ADR-2.5: `stripThinkContent` reuses the same state machine (single source of truth).
- ADR-3: `filterStreamResult` goroutine wraps upstream; maps TextDelta, flushes on Done, passes others through; cancellation via upstream channel close; error via `upstream.Response()`.
- ADR-4: sync `Chat` strips only `Content`; tool calls untouched.
- ADR-5: factory case + KnownProviders + 3 switch edits.
- ADR-5.1: api_key auto-enforced (no edit), verified by test.
- ADR-6: strict-TDD tiers — pure-unit filter table first, httptest SSE, config.
