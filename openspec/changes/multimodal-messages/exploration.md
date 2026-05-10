# Exploration: multimodal-messages

## Current State

### Channel layer — `IncomingMessage` and `OutgoingMessage`

`internal/channel/channel.go` defines the two core wire types:

```go
type IncomingMessage struct {
    ID        string
    ChannelID string
    SenderID  string
    Text      string            // ← only payload field
    Metadata  map[string]string // channel-specific, untyped
    Timestamp time.Time
}

type OutgoingMessage struct {
    ChannelID   string
    RecipientID string
    Text        string            // ← only payload field
    Metadata    map[string]string
}
```

Both types are **text-only**. There is no field for binary payloads, MIME types, or media references.

### Channel implementations — where non-text is dropped

Every channel explicitly gates on text before enqueuing:

| Channel | Guard | Location |
|---------|-------|----------|
| Telegram | `update.Message.Text == ""` → `continue` | `telegram.go:82` |
| WhatsApp | `message.Type != "text"` → `continue` | `whatsapp.go:182` |
| Discord | `strings.TrimSpace(m.Content) == ""` → `return` | `discord.go:74` |
| CLI | Scanner reads only text lines | `cli.go:44–68` |

Telegram's guard is the sharpest because it hard-codes on `.Message.Text`; the Telegram bot library exposes `.Message.Voice`, `.Message.Photo`, `.Message.Document`, `.Message.Caption` on the same struct but they are never read.

Discord has an additional drop path: `inbox <- msg` can fall through to `default` (drop silently) when the inbox is full — identical to Telegram line 129. This is the "drop-on-full inbox bug" flagged as out-of-scope but worth noting since adding larger media messages will worsen it.

### Provider layer — `ChatMessage.Content` is a string

`internal/provider/provider.go`:

```go
type ChatMessage struct {
    Role       string
    Content    string     // ← flat string, no blocks
    ToolCalls  []ToolCall
    ToolCallID string
}
```

The internal `ChatMessage` represents a single turn in the conversation. Providers individually translate this to their wire formats.

**Anthropic** (`anthropic_stream.go:buildAnthropicRequest`): the provider already constructs `[]any` content blocks internally for tool_result and tool_use messages, but plain user/assistant messages collapse back to `Content string`. The wire-level Anthropic API *supports* image blocks natively via `{"type":"image","source":{"type":"base64","media_type":"...","data":"..."}}` — the provider just never uses them.

**OpenAI** (`openai.go`): `openaiMessage.Content` is `any` already (to allow `null` for tool-call-only messages), and the GPT-4o vision API accepts `[{"type":"text","text":"..."}, {"type":"image_url","image_url":{"url":"data:..."}}]` content arrays. The current code always writes a plain string.

**Gemini** (`gemini.go`): uses `geminiPart` which has `Text`, `FunctionCall`, `FunctionResponse` but no inline image part. Gemini's API supports an `inlineData` part with `mimeType`/`data` fields — not currently modelled.

**OpenRouter** (via OpenAI-compatible path): same as OpenAI.

### Agent loop — `loop.go`

`processMessage` (loop.go:30) appends the incoming text verbatim into the conversation:

```go
conv.Messages = append(conv.Messages, provider.ChatMessage{
    Role:    "user",
    Content: msg.Text,  // ← text only
})
```

And also logs `"text_len", len(msg.Text)` — an implicit text assumption.

The semaphore warning in `agent.go:214` also references `m.Text` for the truncation preview.

### Context builder — `context.go`

`buildContext` constructs `provider.ChatRequest` from `conv.Messages` directly — all messages are already `provider.ChatMessage` with string `Content`. No changes needed here unless we add multimodal to the message history.

### Filter — `filter.go`

`filter.Apply` and `filter.PreApply` operate on `tool.ToolResult.Content string` — they are entirely decoupled from `IncomingMessage`. No direct impact, but if a "transcribe voice" or "describe image" tool is introduced, its text output would flow through Apply normally.

### Store — `sqlitestore.go` / `filestore.go`

`SaveConversation` serialises `conv.Messages` (a `[]provider.ChatMessage`) via `json.Marshal`. If `ChatMessage.Content` stays a string, nothing changes in the store. If it becomes `[]ContentBlock`, existing stored conversations would have string Content values that would need forward-migration on read.

The `conversations` table column is `messages TEXT` (JSON blob) — schema stays unchanged, but the shape of that JSON changes, which means deserialization must handle both old `"content":"..."` and new `"content":[...]` shapes.

---

## Affected Areas

### Channel layer
- `internal/channel/channel.go` — `IncomingMessage` and `OutgoingMessage` type definitions
- `internal/channel/telegram.go` — drop guard, message extraction, photo/voice/document fields
- `internal/channel/whatsapp.go` — whatsappPayload struct needs audio/image/document fields; drop guard
- `internal/channel/discord.go` — attachment extraction from `m.Attachments`
- `internal/channel/cli.go` — minor: probably no change needed unless file-path input desired
- `internal/channel/mux.go` / `mux_stream.go` — pass-through; needs audit for any Text assumptions

### Provider layer
- `internal/provider/provider.go` — `ChatMessage.Content` type
- `internal/provider/anthropic_stream.go` — `buildAnthropicRequest`: encode image blocks as `{"type":"image","source":...}`
- `internal/provider/openai.go` / `openai_stream.go` — encode image blocks as `[{"type":"image_url",...}]`
- `internal/provider/gemini.go` / `gemini_stream.go` — encode `inlineData` parts
- `internal/provider/openrouter.go` / `openrouter_stream.go` — likely same as OpenAI path
- `internal/provider/fallback.go` / `fallback_stream.go` — fan-out; needs to forward blocks

### Agent loop
- `internal/agent/loop.go` — `processMessage`: append multimodal `IncomingMessage` to conv, `text_len` log
- `internal/agent/agent.go` — `truncate(m.Text, 80)` warning; semaphore drop log
- `internal/agent/context.go` — `buildContext` may be unaffected if ChatMessage carries blocks natively

### Store
- `internal/store/filestore.go` — forward-compat JSON decode of legacy `"content":string`
- `internal/store/sqlitestore.go` — same; migration logic for existing conversations
- `internal/store/output.go` — unaffected (ToolOutput is separate from ChatMessage)

### Filter
- `internal/filter/filter.go` — unaffected for incoming; voice transcription output would pass through normally
- `internal/filter/filter_test.go` — may need coverage for new tool types

### Tests (churn risk)
- `internal/channel/telegram_test.go`, `discord_test.go`, `whatsapp_test.go` — message construction tests
- `internal/provider/anthropic_test.go`, `openai_test.go`, `gemini_test.go` — ChatMessage encoding tests
- `internal/agent/loop_test.go`, `context_test.go` — message flow tests
- `internal/store/sqlitestore_output_test.go`, `pruning_test.go` — likely unaffected

---

## Approaches

### Approach A — Content blocks on core types (tightest integration)

Evolve `ChatMessage.Content` from `string` to `[]ContentBlock`:

```go
type ContentBlock struct {
    Type       string // "text" | "image" | "audio" | "document"
    Text       string
    MediaBytes []byte
    MediaURL   string
    MIME       string
    Transcript string // populated post-transcription
}

type ChatMessage struct {
    Role       string
    Content    []ContentBlock  // replaces string
    ToolCalls  []ToolCall
    ToolCallID string
}
```

Each provider's builder decides what to do with non-text blocks: Anthropic can use native image blocks; OpenAI can use `image_url`; text-only providers (or Ollama) stringify or skip.

- **Pros**: single canonical representation throughout the pipeline; providers can natively forward binary; no parallel code paths; future audio/document blocks are additive
- **Cons**: **breaks every existing consumer of `.Content`** — loop.go, context.go, all provider builders, all tests; existing stored JSON `"content":"text"` is a schema mismatch on load (needs migration shim); significant test churn; `Content string` is ergonomically convenient, now replaced with a slice
- **Effort**: High

### Approach B — Sidecar attachments (least invasive)

Keep `Content string` on `ChatMessage` unchanged. Add `Attachments []Attachment` alongside:

```go
type Attachment struct {
    Type       string // "image" | "audio" | "document"
    MIME       string
    Bytes      []byte
    URL        string
    Transcript string
}

type IncomingMessage struct {
    // ... existing fields ...
    Attachments []Attachment
}

type ChatMessage struct {
    Role        string
    Content     string
    Attachments []Attachment  // new
    ToolCalls   []ToolCall
    ToolCallID  string
}
```

Providers that support multimodal check `Attachments` and construct mixed content arrays. Text-only providers ignore `Attachments` entirely (or log a warning).

- **Pros**: zero breakage to existing code — all `.Content` reads still work; existing stored JSON round-trips cleanly (new field is omitempty, old records just have no attachments); additive change; easy to ship incrementally
- **Cons**: **two permanent code paths** — content and attachments are parallel forever; providers must remember to check Attachments; risk that attachments get silently dropped by providers that forget to handle them; sidecar pattern is somewhat awkward for mixed text+image messages (e.g. "describe this photo: <image>" where text and image interleave)
- **Effort**: Medium

### Approach C — Pre-processing at the channel edge (simplest, most limited)

Channels call out to transcription/vision services before creating `IncomingMessage`. Telegram channel transcribes `.Message.Voice` via Whisper API, converts `.Message.Photo` via GPT-4o vision to a text description, then sets `msg.Text` to the result. Everything upstream stays text-only.

- **Pros**: zero changes to provider, agent loop, context builder, or store; no migration; easiest to ship fast; no new types
- **Cons**: loses raw media — can't pass actual image bytes to Anthropic's native vision; policy (which provider to call for transcription) is buried in channels; extra latency and API cost at the edge; if user wants to change transcription model they must change channel code; voice notes get transcribed even when the downstream provider could handle audio natively; no ability to show the original image in a future UI
- **Effort**: Low (per channel) but architecturally closes the door on native multimodal

---

## Recommendation

**Approach B (sidecar attachments)** for the initial implementation, with a documented migration path to Approach A.

Approach B keeps the existing `Content string` ergonomics intact — zero breakage to the 30+ existing test assertions on `.Content` and zero migration of stored conversation JSON. Providers gain an opt-in `Attachments` slice they check after the text block. Once the sidecar pattern is in production and validated end-to-end, the proposal phase can plan a follow-up refactor to Approach A (merge `Content` + `Attachments` into `[]ContentBlock`) with a proper JSON migration shim. Approach C is rejected because it sacrifices native multimodal and embeds policy in the wrong layer.

---

## Key Questions for the Proposal Phase

1. **Flatten strategy for text-only providers**: when an Attachment arrives at a provider that doesn't support multimodal (Ollama, older GPT models), should it: (a) silently ignore, (b) auto-transcribe/describe via a sidecar call, or (c) return an error to the user?

2. **Media persistence in SQLiteStore**: store binary blobs inline in the `conversations` JSON (easy, bad for large files), write to `~/.daimon/data/media/<hash>` and store a path reference (good, requires cleanup), or use a content-addressable store (CAS) keyed by SHA256? Voice notes can be several MB; photos up to 10 MB.

3. **Voice note processing point**: transcribe at the channel edge (Telegram downloads + calls Whisper before enqueue) vs. pass raw bytes to the provider (Anthropic's native audio support via multimodal) vs. a separate pre-processing step in the agent loop before the LLM call? Each has different cost, privacy, and latency profiles.

4. **Outgoing multimodal scope**: is this change incoming-only (agent receives photos/voice), or does the agent also need to send images back? Sending images requires `OutgoingMessage` changes and per-channel send paths (Telegram's `sendPhoto`, Discord's attachment upload, WhatsApp's media message API).

5. **Media size limits and cleanup policy**: what's the max attachment size accepted from each channel? How long are raw bytes kept (session-only, 7 days, forever)? Who enforces limits — channel layer (drop oversized messages), or agent loop (trim before persisting)?

6. **Filter behavior with non-text content**: `filter.PreApply` and `filter.Apply` currently only process tool results. If a transcription tool is introduced, its output flows through Apply normally. But if raw image bytes end up in `ChatMessage.Attachments`, the filter layer needs a policy: truncate, ignore, or inspect?

---

## Risks

- **Test churn (Medium)**: even with Approach B, adding `Attachments` to `IncomingMessage` and `ChatMessage` requires updating all channel construction sites and provider builder assertions. Estimate 15–25 test files touched.

- **JSON migration for existing conversations (High)**: `conv.Messages` is stored as JSON text. If `ChatMessage` gains `Attachments []Attachment`, old stored records deserialize fine (field defaults to nil). But if a future migration to Approach A changes `Content string` → `[]ContentBlock`, every saved conversation needs a read-time shim or an explicit migration pass. Should be planned now even if not executed yet.

- **Binary blob growth in SQLiteStore (High)**: storing even one 1 MB voice note inline in the conversations JSON per turn will balloon the database. The store schema and architecture discussion should happen before committing to the persistence design. WAL mode helps concurrent reads but not size.

- **Provider capability mismatch at runtime (Medium)**: Ollama and some OpenRouter models don't support vision. Silently dropping Attachments means the user gets a text reply with no acknowledgement that the image was ignored. Need a `SupportsMultimodal() bool` interface method or capabilities struct.

- **Telegram file download requires an extra API call (Low-Medium)**: Telegram doesn't deliver binary in the update payload — it delivers a `file_id`. The channel must call `bot.GetFileDirectURL(file_id)` and download the bytes, adding latency and a failure mode before the message even enters the inbox.

- **Race in `processMessage` with large attachments (Low)**: the semaphore pool is sized at 4 concurrent messages. Large binary attachments held in-memory across the full LLM round-trip (which can be seconds) will multiply memory pressure. A worker pool or streaming-to-disk approach may be needed.

- **WhatsApp media download via Cloud API (Medium)**: WhatsApp doesn't deliver media bytes in the webhook — it delivers a `media_id`. The channel must call the Graph API media endpoint with the access token to retrieve a download URL, then fetch the bytes. Two extra HTTP calls before enqueue, each with their own failure modes and rate limits.
