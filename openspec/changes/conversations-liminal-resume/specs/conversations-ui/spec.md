# Conversations UI Specification

## Purpose

Defines frontend behavior for the Liminal redesign of the Conversations list + detail pages, the Resume flow in ChatPage, and the supporting hooks. Only behavior is specified — visual details (exact spacing, colors, iconography) are a design-doc concern.

## ADDED Requirements

### Requirement: ConversationsPage — Liminal rewrite

`src/pages/ConversationsPage.tsx` is rewritten to render the Liminal design language. The page MUST include:

1. A `ConversationsPreamble` component at the top (voice-forward header, parallel to `MemoryPreamble`).
2. A `ConversationsToolbar` with a client-side search input and filter controls.
3. Time-clustered sections, each with a `TimeClusterHeader` and a grid/list of `ConversationCard`s.
4. Pagination controls (load-more button, server-side pagination via existing `useConversations`).
5. Undo toast on delete (visible for 10 seconds; clicking "Deshacer" calls `POST /api/conversations/{id}/restore`).

#### Scenario: Empty state

- GIVEN no conversations exist
- WHEN the page renders
- THEN `ConversationsPreamble` is shown with empty-state voice text
- AND no cluster headers are rendered
- AND no cards are rendered
- AND no empty-state illustration is shown (out of scope for v1)

#### Scenario: Conversations render into their time buckets

- GIVEN 5 convs with updated_at spread across: now-2h, now-3d, now-20d, now-60d, now-200d
- WHEN the page renders
- THEN the grouping is:
  - "Hoy" → 1 card (the now-2h one)
  - "Esta semana" → 1 card (now-3d)
  - "Este mes" → 1 card (now-20d)
  - "Últimos meses" → 1 card (now-60d)
  - "Más antiguas" → 1 card (now-200d)
- AND bucket order from top to bottom is: Hoy, Esta semana, Este mes, Últimos meses, Más antiguas

#### Scenario: Search filters client-side

- GIVEN 20 convs loaded
- WHEN the user types "RAG" in the search input
- THEN only convs whose title OR preview OR channel contains "rag" (case-insensitive) are visible
- AND cluster headers with zero visible cards are hidden
- AND the search does NOT trigger a network request

#### Scenario: Delete with undo

- GIVEN a visible conv
- WHEN the user clicks delete on the card
- THEN `DELETE /api/conversations/{id}` fires
- AND the card is removed from the page immediately
- AND a toast appears for 10 seconds with "Deshacer"
- IF the user clicks "Deshacer" within 10 seconds
- THEN `POST /api/conversations/{id}/restore` fires
- AND the card reappears in its original bucket

#### Scenario: Clicking a card navigates to detail

- WHEN the user clicks a card
- THEN the router navigates to `/conversations/{id}`

### Requirement: Time bucket assignment

A pure helper `bucketForTimestamp(updatedAt: Date, now: Date): TimeBucket` MUST be extracted in `src/utils/timeBuckets.ts` with exhaustive mapping:

| Condition | Bucket |
|---|---|
| `now - updatedAt <= 24h` | `"today"` |
| `now - updatedAt <= 7d` | `"thisWeek"` |
| `now - updatedAt <= 30d` | `"thisMonth"` |
| `now - updatedAt <= 90d` | `"lastMonths"` |
| else | `"older"` |

Bucket labels are localized in a single `BUCKET_LABELS` constant in Spanish for v1.

#### Scenario: Boundary exactly 24h

- GIVEN `updatedAt = now - 24h - 1ms`
- THEN `bucketForTimestamp(...)` returns `"thisWeek"` (strictly greater than 24h)

#### Scenario: `updatedAt` in the future (clock skew)

- GIVEN `updatedAt = now + 10min`
- THEN the function returns `"today"` (future timestamps are treated as now)

### Requirement: `ConversationCard` component

A new `src/components/liminal/conversations/ConversationCard.tsx` MUST:

- Accept props: `{ conv: ConversationSummary, density: 'sparse' | 'normal' | 'dense', onDelete: (id) => void, onClick: (id) => void }`.
- Render with a left-border accent, title, 2-line preview truncation, channel pill, relative "last active" label, and a hover-revealed delete icon.
- Match the visual pattern of `MemoryCard` without importing its types.

#### Scenario: Title fallback when metadata.title is empty

- GIVEN a conv with `metadata.title = ""` and first user-msg `"quiero entender el RAG..."`
- THEN the card renders the truncated first user-msg as the title
- AND a subtle "auto" or italic styling indicates the title is derived (implementation detail — design decides)

#### Scenario: Click-to-navigate vs click-to-delete

- GIVEN a card rendered with `onClick` and `onDelete`
- WHEN the user clicks the card body
- THEN `onClick(id)` fires
- WHEN the user clicks the delete icon (stopPropagation applied)
- THEN `onDelete(id)` fires
- AND `onClick` does NOT fire

### Requirement: ConversationDetailPage — Liminal rewrite + Resume

`src/pages/ConversationDetailPage.tsx` is rewritten to:

1. Show a Liminal preamble with conv metadata (title, channel pill, "created X days ago", message count).
2. Render messages via `LiminalThread` + `LiminalUserMsg` / `LiminalAssistantMsg` — same primitives as ChatPage.
3. Support message pagination via `useInfiniteConversationMessages` — initial load is the most recent 50 messages; a "Cargar anteriores" control at the TOP loads older pages.
4. Provide a prominent **Resume** button that navigates to `/chat?conversation_id={id}`.
5. Provide a title rename control (inline edit on click of the title, saves via `PATCH /api/conversations/{id}`).
6. Keep JSON / MD export controls (unchanged API behavior — fetches full conv via `GET /api/conversations/{id}`).
7. Keep delete (soft) control, with the same undo toast as the list page.

#### Scenario: Initial load shows latest 50 messages

- GIVEN a conv with 200 messages
- WHEN the page loads
- THEN the most recent 50 are rendered
- AND a "Cargar anteriores" button is visible at the top
- AND older messages are NOT in the DOM

#### Scenario: Load-more prepends older messages

- GIVEN initial load of 50 most recent
- WHEN the user clicks "Cargar anteriores"
- THEN the next 50 are fetched via `useInfiniteConversationMessages`
- AND they are prepended (older above newer) in the thread
- AND scroll position is preserved so the user stays at the same visual message

#### Scenario: Rename title

- WHEN the user clicks the title
- THEN the title becomes an editable input
- WHEN the user submits via Enter or blur with valid text
- THEN `PATCH /api/conversations/{id}` fires
- AND the page re-fetches summary to reflect the normalized title

#### Scenario: Resume button

- WHEN the user clicks "Resume"
- THEN the router navigates to `/chat?conversation_id={convID}`

### Requirement: ChatPage — Resume flow

`src/pages/ChatPage.tsx` MUST read `?conversation_id=` (alias `?resume=` accepted for UX flexibility; prefer `conversation_id`) from the URL on mount. When present:

1. The WebSocket connection path includes the query param: `/ws/chat?conversation_id={id}`.
2. History is loaded via `useInfiniteConversationMessages(id)`; initial 50 messages rendered in the thread.
3. New messages arriving via WS are appended to the thread (existing behavior).
4. The `useResumeSession(id | null)` hook exposes `canContinue: boolean` — true when `lastMessage.role === "user"`. When true, a "Continuar" button appears that sends a `continue_turn` WS message.

ChatPage MUST NOT grow beyond ~1000 LOC as a result of this change. Resume logic extraction into `useResumeSession` is required, not optional.

#### Scenario: Resume with history

- GIVEN `/chat?conversation_id=conv_web:x:u1` for a conv with 75 messages
- WHEN ChatPage mounts
- THEN the WS connects to `/ws/chat?conversation_id=conv_web:x:u1`
- AND the thread is prefilled with the most recent 50 messages
- AND the user can send a new message normally (appends to the same conv)

#### Scenario: Resume without conversation_id param

- WHEN ChatPage mounts with no query params
- THEN behavior is identical to pre-change (fresh conversation on a new uuid-generated channelID)
- AND `useResumeSession` returns `canContinue: false`

#### Scenario: canContinue is true when last msg is user

- GIVEN a loaded history ending with `{ role: "user", content: "..." }`
- THEN `useResumeSession.canContinue === true`
- AND the "Continuar" button is visible

#### Scenario: canContinue is false when last msg is assistant

- GIVEN a loaded history ending with `{ role: "assistant", content: "..." }`
- THEN `useResumeSession.canContinue === false`
- AND the "Continuar" button is hidden

### Requirement: `useInfiniteConversationMessages` hook

`src/hooks/useInfiniteConversationMessages.ts` MUST:

- Build on `@tanstack/react-query`'s `useInfiniteQuery`.
- First page: `{ before: null, limit: 50 }`.
- Subsequent pages: `{ before: oldest_index, limit: 50 }`.
- `getNextPageParam = (lastPage) => lastPage.has_more ? lastPage.oldest_index : undefined`.

#### Scenario: Initial load

- GIVEN a conv with 200 messages
- WHEN the hook is first rendered with `enabled: true`
- THEN a single page is fetched with `before=null, limit=50`
- AND the returned data includes `oldest_index` and `has_more: true`

#### Scenario: fetchNextPage advances cursor

- WHEN `fetchNextPage()` is called after the initial load
- THEN the next request uses `before=150, limit=50`

### Requirement: `useWebSocket` preserves query params on reconnect

`src/hooks/useWebSocket.ts` MUST accept a `path` that includes a query string, OR a new optional `searchParams: Record<string, string>` argument, and preserve it across auto-reconnects.

#### Scenario: Reconnect after network blip

- GIVEN `useWebSocket({ path: '/ws/chat?conversation_id=conv_x' })`
- AND the connection drops
- WHEN the backoff timer fires a reconnect
- THEN the new connection ALSO includes `?conversation_id=conv_x`

#### Scenario: No query string — unchanged behavior

- GIVEN `useWebSocket({ path: '/ws/chat' })`
- AND reconnect fires
- THEN the new connection uses `/ws/chat` with no query params

### Requirement: Toast undo component

A reusable `src/components/Toast.tsx` (or extension of existing toast infra if any) MUST support an undo-style toast: a message, a button with a label, a countdown, and onAction / onExpire callbacks. If an existing toast utility covers this, extend it minimally rather than creating a new one.

#### Scenario: Undo toast expires after 10s

- GIVEN a toast is shown with 10s duration
- WHEN 10 seconds elapse without user interaction
- THEN `onExpire` fires and the toast is dismissed

#### Scenario: Undo button fires action and dismisses

- WHEN the user clicks "Deshacer" within the window
- THEN `onAction` fires
- AND the toast is dismissed immediately
- AND `onExpire` is NOT called

## Non-requirements

- No empty-state illustration in v1.
- No infinite scroll on ConversationsPage list (server-side pagination via load-more button).
- No backend FTS5 search integration (client-side filter is spec'd; FTS5 is follow-up).
- No real-time list updates (if a new conv is created in another tab, the page must be reloaded to see it; SSE / polling is out of scope).
- No bulk operations (multi-select + delete); single-card delete only.
- No bifurcation ("start new thread from this message").
- No "continuar" fallback when canContinue is false — the button is strictly conditional on last-msg-is-user.
