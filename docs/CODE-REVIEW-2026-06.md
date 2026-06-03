# Code Review — Arquitectura y áreas de mejora (2026-06)

> Revisión de arquitectura del codebase completo realizada el 2026-06-03 mediante
> análisis por dominios (arquitectura global, motor del agente, capa de datos,
> interfaces, transversales). Las referencias `file:line` reflejan el estado del
> repo en esa fecha y pueden derivar con el tiempo — el código manda.

## Veredicto general

Codebase Go maduro y bien diseñado (~55k LOC fuente / ~99k LOC tests, 311 test
files). Monolito de binario único que convierte un LLM en un agente con "manos,
ojos y voz": 5 providers, múltiples transportes (CLI/TUI/web/Discord/Telegram/
WhatsApp/cron), MCP-native, RAG, skills en Markdown. Lo más distintivo es un
proceso de desarrollo dirigido por specs (SDD, `openspec/`) con TDD estricto.

**Madurez: ~8/10.** Los problemas principales son de higiene de infraestructura
y deuda localizada, no de calidad de fondo.

## Fortalezas transversales

- Documentación excepcional y verificada contra código (`docs/ARCHITECTURE.md`,
  `DAIMON.md`, docs por módulo, invariantes con IDs `AD-`/`REQ-`/`INV-`).
- Disciplina de concurrencia: lock ordering documentado, snapshots bajo RLock,
  canales nunca cerrados con senders en vuelo, fan-out MCP race-free, tests de
  pureza en la TUI.
- Patrón de interfaces opcionales por type-assertion usado idiomáticamente en
  providers, store, channels y tools.
- Convenciones consistentes: `slog` estructurado (74 archivos), error wrapping
  con `%w` (82 archivos), un solo `panic()` en código no-test.
- Binario estático único, SQLite puro-Go, fail-fast bootstrap, panic recovery en
  tools.

---

## Áreas de mejora priorizadas

### 🔴 Seguridad (agente que ejecuta shell)

| #  | Hallazgo | Ref |
|----|----------|-----|
| S1 | `tool.PermissionLevel` es decorativo, NO se aplica — `PermAdmin` ejecuta igual que `PermNone`. La seguridad depende 100% de allowlists de modo. | `internal/tool/meta.go:29-31` |
| S2 | Validación JSON-schema superficial — solo chequea `required`, sin tipos/enums/nested/`additionalProperties`. | `internal/agent/validate.go:10-45` |
| S3 | Skills ejecutables desde URL corren `sh -c` arbitrario con solo un warning impreso — trust boundary = documentación, no enforcement. | `internal/skill/service.go:169` |
| S4 | `.golangci.yml` sin `gosec`/`errorlint` en un agente que ejecuta shell/tools (solo 6 linters básicos). | `.golangci.yml` |
| S5 | `isArgAllowed` solo cubre `shell_exec` → la garantía read-only de review-mode es más estrecha de lo aparente. | `internal/agent/modes.go:287` |

### 🟠 Mantenibilidad (deuda estructural)

| #  | Hallazgo | Ref |
|----|----------|-----|
| M1 | `processMessage` = 945 líneas / ~12 responsabilidades — mayor riesgo de mantenibilidad, no testeable en aislamiento. | `internal/agent/loop.go:121-1057` |
| M2 | Struct `Agent` god-object (~50 campos) mezclando provider/tools/skills/RAG/workers/cron/registries. | `internal/agent/agent.go:87-200` |
| M3 | Dos paths de bootstrap paralelos (`main.go --web` vs `web_cmd.go`) duplican wiring y pueden divergir. | `cmd/daimon/main.go`, `cmd/daimon/web_cmd.go` |
| M4 | `web` es god-module (13 deps internas, incl. `agent`); violaciones de capas: `notify→channel`, `filter→tool`, `tool→cron`. | varios |
| M5 | Paquete `cost` fantasma — solo lo usa `costs_cmd.go`; el accounting real vive en `audit`. | `internal/cost/` |
| M6 | Duplicación: `ChatStream` 170-258 líneas × 5 providers · handler WS 3× · boilerplate copy-on-write TUI ~10×. | varios |

### 🟡 Rendimiento / correctitud de datos

| #  | Hallazgo | Ref |
|----|----------|-----|
| D1 | Store principal sin `SetMaxOpenConns` → riesgo `SQLITE_BUSY` con escritores concurrentes (el audit DB sí lo pinea a 1). | `internal/store/sqlitestore.go:47` |
| D2 | Conversación como blob JSON único → cada turno reserializa todo el array de mensajes; no escala. Fix: tabla hija `messages`. | `internal/store/sqlitestore.go:118` |
| D3 | `pureVectorSearch` = full-table scan sin índice ANN (O(N)/query) + N+1 en `expandNeighbors`. | `internal/rag/sqlite_store.go:283,408` |
| D4 | Faltan índices en `memory.scope_id`/`archived_at` + 3 codificaciones de timestamp que anulan índices vía `substr()`. | `internal/store/migration.go` |
| D5 | Errores tragados inútilmente (`_ = fmt.Errorf(...)` que no loggea) — código muerto disfrazado. | `internal/store/sqlitestore.go:803,757` |
| D6 | Fuga de goroutine "por diseño" en `EventBus.callWithTimeout` (abandona, no cancela) · `/ws/logs` hace polling SQLite cada 2s/conexión en vez de suscribirse al bus. | `internal/notify/bus.go:198` |

### 🔵 Infra / tooling (quick wins)

| #  | Hallazgo | Ref |
|----|----------|-----|
| I1 | Binario `server` de 7.0MB commiteado en git — quitar y añadir a `.gitignore`. | raíz |
| I2 | `gorilla/websocket v1.4.2` (2020, era archivada) en código core → bump a v1.5.x mantenido. | `go.mod` |
| I3 | CI single-platform (solo ubuntu) aunque se publica darwin/windows; sin coverage gate ni matrix. | `.github/workflows/ci.yml` |
| I4 | Tests: filter sub-módulos sin tests dedicados (`git/http/shell/file/listing.go`); `t.Parallel()` solo en 9 archivos; `TESTS.md` con drift vs el suite actual. | varios |

---

## Orden de ataque recomendado

1. **Quick wins inmediatos:** I1 (quitar binario), I2 (bump websocket), S4
   (reforzar golangci) — bajo riesgo, alto valor.
2. **Seguridad real:** S1+S3 (enforcement de permisos / trust boundary) — decidir
   si los tags de permiso deben aplicarse o documentarse como cosméticos.
3. **Refactor estructural mayor:** M1 (descomponer `processMessage`) — mayor
   impacto en mantenibilidad/testabilidad.
4. **Escalabilidad de datos:** D1+D2 cuando las conversaciones crezcan.

## Estado de remediación

- [x] I1 — binario `server` eliminado del tracking + `.gitignore` actualizado
- [x] I2 — `gorilla/websocket` bumpeado a v1.5.3 (build + vet + tests `web`/`channel` OK)
- [ ] S4 — reforzar `.golangci.yml`. **Diferido**: requiere coordinar dos cosas antes
  de tocar el config — (a) el CI pinea golangci-lint v1.64.8 mientras que v2.x usa un
  esquema de config incompatible (`version: "2"`), y (b) `gosec` flageará la ejecución
  de shell intencional (G204) en `tool/shell.go` y `skill/shelltool.go`, por lo que hay
  que añadir exclusiones dirigidas para no romper el lint. Hacerlo como cambio propio
  con verificación, no como quick win a ciegas.
- [ ] resto — pendiente de priorización

> Nota: los tests `TestLogout_DiskFailure_*` y `TestRotateAuthToken_DiskFailure_*` de
> `internal/web` fallan al ejecutarse como `root` (la simulación de fallo de disco por
> permisos read-only no aplica a root). Es pre-existente y ajeno a estos cambios.
</content>
</invoke>
