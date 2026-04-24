# Web Endpoints & WebSocket Channel (Delta) Specification

## Purpose

Extends the WebSocket channel to accept an explicit `conversation_id` query parameter on upgrade, and extends the REST API with endpoints needed by the Conversations redesign: paginated messages, title rename, restore from soft-delete.

## MODIFIED Requirements

### Requirement: WebSocket upgrade accepts `conversation_id`

`HandleWebSocket` MUST read the `conversation_id` query parameter from `r.URL.Query()` before upgrading. When present and non-empty:

- The connection is bound to the provided convID for the lifetime of the session.
- Every `IncomingMessage` emitted into the inbox from this connection carries the provided convID in a new `ConversationID` field.
- The `channel_id` used for the connection's `sync.Map` key remains the server-generated `"web:" + uuid[:8]` — this is a transport-level handle, not identity.

When absent, behavior is unchanged (server generates a fresh `channelID`, `IncomingMessage.ConversationID` is empty, `userScope` computes convID).

(Previously: every upgrade produced a fresh `channelID` and no mechanism existed to resume a prior conv.)

#### Scenario: Upgrade with conversation_id binds the session

- GIVEN a client connects to `/ws/chat?conversation_id=conv_web:abc:u1`
- WHEN the first message is sent over the socket
- THEN `IncomingMessage.ConversationID = "conv_web:abc:u1"` is emitted into the inbox
- AND downstream, `processMessage` resolves the convID to `"conv_web:abc:u1"` (spec: `agent-loop`)

#### Scenario: Upgrade without conversation_id is unchanged

- GIVEN a client connects to `/ws/chat` with no query params
- WHEN the first message is sent
- THEN `IncomingMessage.ConversationID = ""` (empty)
- AND the server-generated `channelID` drives convID computation via `userScope`

#### Scenario: Upgrade with conversation_id that does not match an existing conv

- GIVEN a client connects to `/ws/chat?conversation_id=conv_web:nonexistent:u1`
- WHEN a message is sent
- THEN the message is dispatched normally
- AND `processMessage` creates a fresh conv with the provided ID (spec: `agent-loop`)
- AND no error is surfaced to the client for "conv not found"

### Requirement: `IncomingMessage` shape extended

The `channel.IncomingMessage` struct MUST add an optional `ConversationID string` field. All existing channels (CLI, Telegram, Discord, WhatsApp, Mux) MUST compile with zero-value fields. Only the web channel populates it in v1.

#### Scenario: Non-web channels send empty ConversationID

- GIVEN a Telegram channel emits a message
- WHEN it lands in the inbox
- THEN `IncomingMessage.ConversationID = ""`
- AND the agent falls back to `userScope` — pre-existing behavior

## ADDED Requirements

### Requirement: `GET /api/conversations/{id}/messages` paginated

Returns a window of messages from a single conversation.

Query params:
- `before` — integer message index; omitted or `-1` = load most recent window
- `limit` — integer; clamped to [1, 200]; default 50

Response 200:
```json
{
  "messages": [ {...provider.ChatMessage...}, ... ],
  "oldest_index": 100,
  "has_more": true
}
```

Response 404 when conv is missing or soft-deleted.

#### Scenario: Initial load fetches the most recent window

- GIVEN a conv with 200 messages
- WHEN `GET /api/conversations/{id}/messages?limit=50`
- THEN response returns messages at indexes 150..199
- AND `oldest_index = 150`, `has_more = true`

#### Scenario: Paging upward

- WHEN `GET /api/conversations/{id}/messages?before=150&limit=50`
- THEN response returns messages at indexes 100..149
- AND `oldest_index = 100`, `has_more = true`

#### Scenario: 404 on soft-deleted conv

- GIVEN a conv with `deleted_at` set
- WHEN `GET /api/conversations/{id}/messages`
- THEN response is 404

### Requirement: `PATCH /api/conversations/{id}` title rename

Request body: `{"title": "<1..100 runes>"}`.

Validation:
- Trim leading/trailing whitespace.
- Reject with 400 if the trimmed title is empty or > 100 runes.
- Strip newlines and carriage returns (replace with space); preserve other characters verbatim.

Response 200:
```json
{"id": "conv_...", "title": "<normalized>"}
```

Response 404 when the conv is missing or soft-deleted.

#### Scenario: Valid rename persists to metadata.title

- GIVEN a live conv
- WHEN `PATCH /api/conversations/{id}` with `{"title": "Mi nuevo hilo"}`
- THEN response is 200 with the normalized title
- AND the conv's `metadata["title"]` is `"Mi nuevo hilo"`

#### Scenario: Empty title is rejected

- WHEN `PATCH` with `{"title": "   "}`
- THEN response is 400

#### Scenario: Oversize title is rejected

- WHEN `PATCH` with a 101-rune title
- THEN response is 400

#### Scenario: Newline is stripped

- WHEN `PATCH` with `{"title": "line1\nline2"}`
- THEN the stored title is `"line1 line2"`

### Requirement: `POST /api/conversations/{id}/restore`

Clears `deleted_at` on a soft-deleted conv.

Response 200: `{"id": "conv_...", "restored": true}`.
Response 404: the conv does not exist OR is already live.

#### Scenario: Restoring a soft-deleted conv

- GIVEN a conv with `deleted_at` set
- WHEN `POST /api/conversations/{id}/restore`
- THEN response is 200
- AND the conv is live and appears in subsequent list results

#### Scenario: Restoring a live conv is 404

- GIVEN a live conv
- WHEN `POST /api/conversations/{id}/restore`
- THEN response is 404

### Requirement: `DELETE /api/conversations/{id}` semantics change

The existing endpoint SHALL now perform a soft delete (via `DeleteConversation` per the `conversation-store` spec). The response contract remains 200 on success, 404 when the conv is missing.

#### Scenario: Delete performs soft delete

- GIVEN a live conv
- WHEN `DELETE /api/conversations/{id}`
- THEN response is 200
- AND the conv is no longer returned by list / load endpoints
- AND the row still exists in SQL with `deleted_at` set

### Requirement: `GET /api/conversations` response includes title

The list endpoint response MUST include a `"title"` field in each summary, derived as:

1. If `metadata["title"]` is non-empty, use it.
2. Otherwise, fall back to the first user-role message text truncated to 60 runes with an ellipsis suffix if truncated.
3. Otherwise (no user message yet), empty string.

(Previously: summaries used the truncated-first-user-msg derivation directly, without the metadata override.)

#### Scenario: List returns metadata title when present

- GIVEN a conv with `metadata["title"] = "Sobre RAG"`
- WHEN `GET /api/conversations`
- THEN the summary's `"title"` is `"Sobre RAG"`

#### Scenario: List falls back to truncated first msg

- GIVEN a conv without `metadata["title"]` and first user-msg `"una pregunta muy larga..."`
- WHEN `GET /api/conversations`
- THEN `"title"` is the truncated first message

## Non-requirements

- The endpoints do NOT paginate by title or search over messages in v1.
- The endpoints do NOT support undo of `restore` (i.e., "re-delete"); the user can simply DELETE again.
- The endpoints do NOT enforce an ownership check against a user session in v1; all authenticated clients on the same server can access any conv.
