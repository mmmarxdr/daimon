# Proposal: Conversations → Liminal redesign + Resume + paginación + soft delete + LLM title

## 1. Why

- **La sección Conversations está visualmente años atrás del resto del frontend.** `MemoryPage.tsx` estableció el lenguaje Liminal (serif italic aspiracional, voz en primera persona, microcopy colaborativo, border-left accents, density controls). `ConversationsPage.tsx` sigue siendo una tabla admin-style con grid-cols y channel badge — no comparte nada con el sistema vivo del resto de la app.
- **Hoy el usuario pierde contexto cuando cierra el browser.** A nivel UX no hay manera de reanudar una conversación: cada load de `/chat` genera un `channelID = "web:" + uuid[:8]` nuevo (`internal/channel/web.go:199`). La historia sigue en SQLite pero el front no la encuentra — el usuario percibe que "perdió" la conversación aunque los datos estén intactos.
- **Hay un bug silencioso pre-existente que este cambio cierra de regalo.** `useWebSocket` (`src/hooks/useWebSocket.ts:49-53`) auto-reconecta con backoff exponencial en pérdida de red. Cada reconexión abre un nuevo upgrade → nuevo `channelID` → nuevo `convID` → orphana el thread anterior. Dar al cliente una identidad estable (`conversation_id` en el handshake WS) lo fixea como efecto secundario, sin código extra.
- **El delete actual es destructivo e irreversible.** `handleDeleteConversation` (`internal/web/handler_conversations.go:107-120`) hace `DELETE FROM conversations`. Un click accidental = datos perdidos sin recuperación. Soft delete + ventana de retención + undo toast es higiénico.
- **Los títulos hoy son feos.** No hay columna `title` en el schema; se derivan del primer user-msg truncado a 60 runes. Cuando el primer mensaje es "hola" o "tengo una pregunta", el título es inútil. LLM genera títulos usables a costo despreciable (un call por conv, cheap tier).

Este change agrupa los 5 frentes en un único SDD porque comparten: (a) los mismos endpoints de `/api/conversations/*`, (b) la misma migración de schema (v14), (c) los mismos primitivos Liminal nuevos (`ConversationCard`, `TimeClusterHeader`), (d) el mismo WS contract extendido con `conversation_id`. Separarlos multiplicaría los merges sin reducir complejidad efectiva.

## 2. Architectural Decisions (frozen)

Explicaciones en la exploración (`explore.md`) y en memoria #1167 v2. Esta sección es contrato para el implementador.

### Identity model

1. **`conversation_id` es la identidad canónica al reanudar.** El frontend envía `?conversation_id=<id>` en el upgrade WS. El backend, si el param está presente, usa ese ID directamente y omite `userScope(channelID, senderID)`. Descartamos `senderID` en localStorage — era deuda técnica (acoplaba storage del cliente a identidad backend, frágil a "limpié cookies"). **Why:** `userScope` ya trata `senderID` como opcional (`loop.go:34-39`), y `convID = "conv_" + scope` hace la reconstrucción trivial. Inyección directa en `IncomingMessage.ConversationID` es backward-compat.

2. **Ownership check: diferido.** Hoy daimon es single-user local y no valida que el cliente sea "dueño" del convID. Aceptable para v1. Si se activa la auth layer (`internal/web/auth.go` existe), gatear por session ID antes de aceptar el `conversation_id`. **No bloquea este SDD** — el flag queda documentado en specs/web-channel/spec.md.

3. **Auto-reconnect preserva `conversation_id`.** `useWebSocket` debe guardar el query string del `path` original y adjuntarlo en cada reconexión. Sin esto, el bug pre-existente sobrevive al cambio.

### Turn lifetime & "continuar" button

4. **Diseñar para el peor caso: el turno muere con el WS disconnect.** `SaveConversation` corre UNA sola vez al final de `processMessage` (`loop.go:682`). Si el agente cancela su trabajo con el cliente, el turno se pierde. Si sobrevive, el turno se guarda completo. **Sin conocer aún cuál de los dos pasa**, la UX del Resume debe cubrir ambos: si el último msg cargado es del user (sin respuesta del assistant) → mostrar botón "continuar" que envía un `continue_turn` al WS. Si el último es del assistant → ocultarlo. El spec phase resuelve la pregunta con un trace de 15 min y lo documenta, pero el botón es barato y va sí o sí.

5. **`continue_turn` hoy ya existe.** Se reutiliza tal cual (`web.go:253-258`, `loop.go` con `IsContinuation=true`). No es feature nueva — es la misma mecánica que usa la UI cuando se acaba el budget de tokens.

### Persistence

6. **Schema migration v14: `ALTER TABLE conversations ADD COLUMN deleted_at TIMESTAMP NULL`.** Index parcial `CREATE INDEX idx_conversations_deleted_at ON conversations(deleted_at) WHERE deleted_at IS NOT NULL` para que el pruner sea barato. **Why:** schema version actual = 13 (verificado en `migration_v10_test.go:78`); el patrón versionado idempotente ya existe y se extiende sin discusión arquitectural.

7. **Todas las queries de read filtran `WHERE deleted_at IS NULL`** — `LoadConversation`, `ListConversationsPaginated`, `GetMessages`, cualquier FTS futuro. El `DeleteConversation` pasa a UPDATE. Nuevo endpoint `POST /api/conversations/{id}/restore` (undelete) para el toast undo.

8. **Pruner como goroutine del `Server`, ticker cada 6h, retention default 30 días.** Config: `conversations.prune.retention_days` (default 30), `conversations.prune.interval_hours` (default 6). Graceful shutdown integrado al `Stop()` del server existente. **No es un cron separado** — vive dentro del proceso daimon.

9. **Título: nueva columna NO.** Usamos `metadata["title"]` — la columna `metadata TEXT` (JSON) ya existe (`store/store.go:22`). Evita migración extra, evita una columna que puede quedar NULL indefinidamente.

### Title generation

10. **LLM-generated, async, post-turn-3.** Hook en `loop.go` después de `SaveConversation`: si `turn_count == 3 && metadata["title"] == ""`, encolar job. Worker async corre en una goroutine pool (`internal/agent/titler.go` nuevo).

11. **Trigger cubre mensajes triviales.** Post-turn-3 AND primer user-msg ≥20 runes (lo que ocurra último). Evita títulos generados sobre 3 "hola"/"ok"/"gracias".

12. **Modelo: reuse del provider principal para v1.** Plan v2 decía "haiku equivalente", pero cada provider tiene su propio tier barato — generalizar eso es scope-creep para este SDD. v1 usa `provider.Chat()` con el modelo configurado. Cost = ~$0.0005/conv en tiers estándar. Follow-up separado agrega `ai.title_generation.model` override.

13. **Prompt (fijo, no configurable en v1):**
    ```
    Generate a 3-8 word title summarising this conversation. Respond with ONLY the title, no quotes, no preamble.

    {first_3_turns_serialized}
    ```

14. **Failure policy: silent fallback.** Cualquier error (provider down, timeout 30s, respuesta vacía, >100 runes de output) → skip, dejar `metadata["title"]` vacío, reintentar en el próximo turno completo. Config flag `ai.title_generation.enabled` (default `true`). **Nunca bloquear el turno principal.**

15. **Rename manual opcional**. Endpoint `PATCH /api/conversations/{id}` acepta `{title: "..."}`. Valida: 1-100 runes, strip newlines, no rechaza markdown (el frontend renderiza como text simple). Sobrescribe el título generado. **No confirmación de undo** — rename es idempotente y el usuario puede renombrar de nuevo.

### Pagination

16. **Nuevo endpoint `GET /api/conversations/{id}/messages?before={message_index}&limit=N`.** Cursor-based por índice de mensaje (los mensajes son un JSON array; índice es estable). Default `limit=50`, max 200. Response: `{messages: [...], has_more: bool, oldest_index: int}`.

17. **`ChatPage` y `ConversationDetailPage` comparten el mismo endpoint y el mismo hook `useInfiniteConversationMessages(id)` basado en `useInfiniteQuery` de `@tanstack/react-query`.** Carga últimos 50 al montar, "cargar más arriba" via scroll-to-top sentinel o botón (diseñado en el design doc).

18. **Export JSON/MD NO paginado.** El endpoint `GET /api/conversations/{id}` sigue devolviendo la conv completa para export. Es ruta separada del path paginado. **Why:** export típicamente se hace una vez, performance no justifica complicar la API.

### Liminal UI

19. **Componentes paralelos, no refactor de Memory.** Nuevos:
    - `src/components/liminal/conversations/ConversationCard.tsx` — paralelo a `MemoryCard`, tipos propios (preview, channel, lastActivity, messageCount, title)
    - `src/components/liminal/conversations/TimeClusterHeader.tsx` — paralelo a `ClusterHeader`, labels relativos ("Hace horas", "Hace días", "Hace semanas", "Último mes", "Más antiguas")
    - `src/components/liminal/conversations/ConversationsPreamble.tsx` — header de la página con voz Liminal
    - `src/components/liminal/conversations/ConversationsToolbar.tsx` — search input + filtros; basado en `MemoryToolbar`
   
   **Why:** `MemoryCard` tipa su prop `mem` a la shape de memoria; `ClusterHeader` tipa `cluster` al enum `identity/preferences/projects/...`. Refactorizarlos a generics expande el blast radius a MemoryPage sin upside. Componentes paralelos mantienen el lenguaje visual compartido sin acoplar tipos.

20. **Time buckets fijos, anchor = timestamp de fetch.**
    | Bucket | Criterio | Label |
    |---|---|---|
    | hoy | `updated_at > now - 24h` | "Hoy" |
    | días | `updated_at > now - 7d` | "Esta semana" |
    | semanas | `updated_at > now - 30d` | "Este mes" |
    | mes+ | `updated_at > now - 90d` | "Últimos meses" |
    | antiguas | else | "Más antiguas" |
   
   **Why:** "Hace X horas" como label variable es visualmente ruidoso y cambia entre renders. Buckets fijos con label estable son más fáciles de escanear y más consistentes con `MemoryPage`.

21. **ChatPage se limita a leer `?resume=<id>` + cargar historia.** NO meter lógica de resume como rama condicional dentro del componente (está en 934 LOC, límite manejable). Extraer `useResumeSession(id | null)` hook que encapsula: fetch history paginada, detect "last msg is user" → expose `canContinue: bool`, resume continue_turn. ChatPage consume el hook y muestra el botón si `canContinue`.

22. **Sin empty state custom en v1.** Si no hay convs, el preamble dice algo neutral tipo "Nada aún — las conversaciones aparecen acá una vez que empiezan". No se crea un empty-state illustration dedicado.

### Search

23. **v1 client-side filter sobre `{title, channel, preview}`.** Sin endpoint backend nuevo. Aplica sobre la página cargada; si el usuario pagina, busca entre lo cargado + hits de futuras páginas (accept: search puede "descubrir" resultados según usuario paginea). **Why:** FTS5 sobre `messages TEXT` es feature separada con costo de migración + index rebuild. v2 queda como follow-up — no bloquea este SDD.

## 3. Scope

### In
- Backend:
  - `IncomingMessage.ConversationID` field + wire en todos los channels
  - WS upgrade query-param `conversation_id` en `HandleWebSocket`
  - Migration v14 `deleted_at` + index parcial
  - `DeleteConversation` → soft, `ListConversationsPaginated` filtra, nuevo `RestoreConversation`
  - Pruner goroutine + config
  - Título auto-generado post-turno-3 + manual rename endpoint
  - Paginación endpoint `GET /api/conversations/{id}/messages`
- Frontend:
  - 4 componentes nuevos en `liminal/conversations/`
  - Reescribir `ConversationsPage.tsx` y `ConversationDetailPage.tsx`
  - `useResumeSession` hook + integración en `ChatPage`
  - `useInfiniteConversationMessages` hook
  - `useWebSocket` acepta query params y los preserva en reconnect
  - Soft delete undo toast

### Out (follow-ups, fuera de este SDD)
- FTS5 backend search sobre messages
- Bifurcación (resume-from-specific-message)
- Empty state illustration dedicado
- `ai.title_generation.model` override per-provider
- Ownership check vía auth layer (gate cuando auth esté activa)
- Multi-dispositivo sync (requiere session identity real)

## 4. Non-goals

- No reestructuramos la carpeta `src/components/liminal/`.
- No tocamos `MemoryPage.tsx` ni sus primitivos.
- No agregamos "títulos editados por LLM al vuelo" — el título se genera UNA vez post-turn-3; rename es manual.
- No cambiamos el contract de `AgentSession` (sigue sin existir en memoria — todo vive en la conv en SQLite).
- No migramos hard-deletes históricos — el pruner solo aplica a convs borradas después de v14.

## 5. Test plan (obligatorio — Strict TDD)

Los tests se detallan en `tasks.md` con correspondencia 1:1 al apply phase. Bullets de alto nivel:

- **Backend**: tests para `userScope` con ConversationID override, migration v14 up+down idempotencia, soft-delete filtering en List/Load, pruner con clock inyectable, título worker con provider mockeado (success, timeout, empty response), paginación de mensajes con cursor.
- **Frontend**: tests para `useResumeSession` (lastMsg=user → canContinue=true; lastMsg=assistant → false), `useInfiniteConversationMessages` (initial load + next page), `TimeClusterHeader` bucket assignment, `ConversationCard` rendering con/sin título, `useWebSocket` preservando query params en reconnect.
- **E2E-ish integration**: `HandleWebSocket` con `?conversation_id=` carga el conv correcto; sin el param, genera uno nuevo (backward compat).

## 6. Risks & mitigations

| Risk | Prob | Impact | Mitigation |
|---|---|---|---|
| ctx del agente muere mid-turn → turnos perdidos | Med | Med | Botón "continuar" diseñado para el peor caso; verificación temprana en spec phase |
| Pruner borra convs queridas por el usuario (retention 30d muy corto) | Baja | Alto | Config expuesta; docs destacadas; restore endpoint disponible durante la ventana de retención |
| Title generation dispara cost imprevisto | Med | Bajo | Flag default-ON pero documentado; follow-up añade modelo override |
| `useWebSocket` cambio rompe otros canales WS | Baja | Med | Sólo `/ws/chat` usa query params nuevos; otros paths (`/ws/telemetry` si existe) pasan string vacío — API backward compat |
| Identity spoofing (cliente pide convID ajeno) | Alta | Bajo (single-user local) | Documentado; gate cuando auth layer se active |

## 7. Open questions (no bloquean proposal)

- **Q1**: ¿El ctx del agente sobrevive al `conn.Close()`? (se resuelve en spec phase con un trace)
- **Q2**: ¿El `ConversationDetailPage` pagina mensajes o carga todos? Decisión del proposal: **pagina**, reutilizando el mismo endpoint que ChatPage.
- **Q3**: ¿Retention default 30 días o 90? Decisión del proposal: **30**, porque es la ventana típica de "me arrepentí del delete" y el usuario puede configurarla. Si feedback muestra que es corto, subimos default en patch.

## 8. Acceptance criteria

- [ ] Un usuario puede cerrar el browser a mitad de un thread, volver a `/conversations`, clickear una tarjeta, ver el detail Liminal, hacer click en "Resume" y continuar el chat sin perder historia.
- [ ] Una conv auto-reconnecta (pérdida de red) sin perder identidad: el siguiente mensaje sigue apendiendo al mismo `conversation_id`.
- [ ] Borrar una conv muestra un toast con "Deshacer"; durante los 10s del toast, restore la devuelve. Pasados 30 días, el pruner la borra físicamente.
- [ ] Un conv con ≥3 turnos y primer user-msg ≥20 runes tiene `metadata["title"]` poblado por LLM async sin impactar la latencia del turno.
- [ ] El usuario puede renombrar manualmente el título desde el detail page.
- [ ] Un conv con 500 mensajes abre en <1s en el detail page (solo carga los últimos 50) y ChatPage Resume tampoco laguea.
- [ ] `ConversationsPage` usa el lenguaje Liminal visualmente equivalente a `MemoryPage`: preamble, time clusters, cards con border-left accent, toolbar.
- [ ] Todos los tests (backend + frontend) pasan. `go vet ./...` y `go test -race ./...` limpios. `tsc --noEmit` y `vitest run` limpios.

## 9. Next phase

Proceed to `/sdd-spec`: escribir delta specs sobre `agent-loop`, `conversation-store`, `web-channel`, y crear specs nuevos para `conversations-ui`, `soft-delete-pruner`, `title-generator`. Spec phase también resuelve Q1 con el trace.
