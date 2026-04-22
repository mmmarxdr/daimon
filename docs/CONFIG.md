# Configuration Reference

Daimon reads its config from `~/.daimon/config.yaml` by default. The
search order is:

1. `--config` CLI flag
2. `~/.daimon/config.yaml`
3. `./config.yaml`

All string values support `${ENV_VAR}` interpolation. Paths support `~`
expansion.

## Minimal config

```yaml
agent:
  name: "Micro"
  personality: "You are a concise, helpful assistant."

provider:
  type: openrouter
  model: openrouter/auto
  api_key: "sk-or-v1-..."   # or ${OPENROUTER_API_KEY}

channel:
  type: cli

store:
  type: sqlite
  path: "~/.daimon/data"
```

## `agent`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `name` | string | `"Micro"` | Agent name |
| `personality` | string | — | System prompt injected at every turn |
| `max_iterations` | int | `10` | Max tool-use cycles per message |
| `max_tokens_per_turn` | int | `4096` | Max tokens per LLM call |
| `history_length` | int | `20` | Conversation messages kept in context |
| `memory_results` | int | `5` | Max memory entries injected into context |

## `provider`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `type` | string | — | `openrouter`, `anthropic`, `gemini`, `openai`, `ollama` |
| `model` | string | provider default | Model identifier |
| `api_key` | string | — | API key (supports `${ENV_VAR}`) |
| `timeout` | duration | `60s` | Per-request timeout |
| `max_retries` | int | `3` | Retries on 5xx errors |
| `stream` | bool | `true` | Enable streaming responses |
| `fallback` | object | — | Fallback provider (same fields) — activates on rate-limit or unavailability |

See [docs/PROVIDERS.md](PROVIDERS.md) for the provider table.

## `channel`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `type` | string | — | `cli`, `telegram`, `discord`, `whatsapp` |
| `token` | string | — | Bot API token (Telegram/Discord) |
| `allowed_users` | []int64 | — | User ID whitelist |

See [docs/CHANNELS.md](CHANNELS.md) for per-channel setup.

## `web`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `enabled` | bool | `false` | Enable web dashboard (also via `--web` flag) |
| `port` | int | `8080` | HTTP port |
| `host` | string | `"127.0.0.1"` | Bind address (`0.0.0.0` to expose) |
| `auth_token` | string | auto-generated | Bearer token; also `DAIMON_WEB_TOKEN` env var |

See [docs/WEB_DASHBOARD.md](WEB_DASHBOARD.md) for the full web guide.

## `tools.shell`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `enabled` | bool | `true` | Enable `shell_exec` tool |
| `allowed_commands` | []string | `[ls, cat, ...]` | Whitelisted commands |
| `allow_all` | bool | `false` | Allow any command |
| `working_dir` | string | `"~"` | Working directory |

## `tools.file`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `enabled` | bool | `true` | Enable file tools |
| `base_path` | string | `"~/workspace"` | Sandbox root |
| `max_file_size` | string | `"1MB"` | Max file size |

## `tools.http` / `tools.web_fetch`

See [docs/TOOLS.md](TOOLS.md).

## `tools.mcp`

See [docs/MCP.md](MCP.md).

## `store`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `type` | string | `"file"` | `file` or `sqlite` |
| `path` | string | `"~/.daimon/data"` | Storage directory |

## `logging`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `level` | string | `"info"` | `debug`, `info`, `warn`, `error` |
| `format` | string | `"text"` | `text` or `json` |

## `limits`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `tool_timeout` | duration | `30s` | Max time per tool execution |
| `total_timeout` | duration | `120s` | Max time per agent turn |

## `audit`

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `enabled` | bool | `false` | Enable audit log |
| `path` | string | `"~/.daimon/audit"` | Audit log directory |

## `cron`

See [docs/CRON.md](CRON.md).

## `skills`

See [docs/SKILLS.md](SKILLS.md).

## `notifications`

See [docs/NOTIFICATIONS.md](NOTIFICATIONS.md).

---

## `rag`

The RAG (Retrieval-Augmented Generation) subsystem indexes uploaded documents and
injects relevant chunks into the agent context before each LLM call.

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `enabled` | bool | `false` | Enable the RAG subsystem |
| `chunk_size` | int | `512` | Target token count per chunk |
| `chunk_overlap` | int | `64` | Overlapping tokens between adjacent chunks |
| `top_k` | int | `5` | Maximum chunks injected into context |
| `max_documents` | int | `500` | Maximum stored documents |
| `max_chunks` | int | `100000` | Maximum stored chunks |
| `max_context_tokens` | int | `10000` | Token budget for RAG context injection |
| `summary_model` | string | `""` | Model override for per-document summarization. Empty = use main provider model |

### `rag.embedding`

Configures a dedicated provider for generating vector embeddings. Decouples
embedding from the main chat provider — useful when the chat provider does not
support embeddings (e.g., most OpenRouter models).

**Fallback chain**: `rag.embedding` (if enabled) → main provider (if it supports
embeddings) → FTS5 keyword search only (no cosine reranking).

> **Warning**: changing `model` after data has been indexed silently invalidates
> existing embeddings — they live in a different vector space. Re-upload documents
> after changing the model.

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `enabled` | bool | `false` | Enable dedicated embedding provider |
| `provider` | string | `""` | `openai` or `gemini` (required when enabled) |
| `model` | string | `""` | Embedding model ID. Empty = provider's canonical default |
| `api_key` | string | `""` | API key for the embedding provider (required when enabled) |
| `base_url` | string | `""` | Override the provider's standard endpoint. Empty = default |

```yaml
rag:
  enabled: true
  embedding:
    enabled: true
    provider: openai
    model: text-embedding-3-small
    api_key: ${OPENAI_API_KEY}
```

### `rag.retrieval`

Precision knobs applied after the initial BM25+cosine candidate fetch.
All fields default to zero (disabled); opt in explicitly.

**BM25 vs cosine orientation — read this before setting thresholds:**
FTS5 `bm25()` scores are inverted — lower (more negative) means a better
match. `max_bm25_score` is therefore a ceiling: chunks with a score *above*
(i.e., less negative than) the threshold are rejected. Cosine similarity is
the opposite — higher is better — so `min_cosine_score` is a floor: chunks
below the threshold are rejected. The inverted prefixes (`max` vs `min`) are
intentional and match the underlying score semantics.

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `neighbor_radius` | int | `0` | Expand each retrieved chunk by including up to N adjacent chunks on each side. `0` = disabled |
| `max_bm25_score` | float64 | `0` | BM25 ceiling threshold. Chunks with `bm25() > max_bm25_score` are dropped. `0` = no threshold |
| `min_cosine_score` | float64 | `0` | Cosine floor threshold. Chunks with cosine similarity `< min_cosine_score` are dropped. `0` = no threshold |

```yaml
rag:
  retrieval:
    neighbor_radius: 1
    max_bm25_score: -0.5   # reject poor BM25 matches (less negative = worse)
    min_cosine_score: 0.7  # reject low-similarity cosine matches
```

### `rag.hyde`

HyDE (Hypothetical Document Embeddings) improves semantic recall for queries
that have no keyword overlap with the indexed content. When enabled, the agent
generates a short hypothetical answer to the query, embeds that answer, and
uses it as a second retrieval signal alongside the raw query embedding. The
two signals are merged via Reciprocal Rank Fusion (RRF) before final reranking.

**Default: OFF.** Opt in by setting `enabled: true`. Requires
`rag.embedding.enabled: true` — HyDE has no effect without vector embeddings.

**HyDE model fallback chain**: `rag.hyde.model` → `rag.summary_model` →
main provider model.

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `enabled` | bool | `false` | Enable HyDE retrieval pass |
| `model` | string | `""` | Model used to generate hypothetical answers. Empty = follow fallback chain |
| `hypothesis_timeout` | duration | `10s` | Timeout for hypothesis generation. On timeout, falls through to baseline retrieval without failing |
| `query_weight` | float64 | `0.3` | Weight of the raw query embedding in the final ensemble. HyDE hypothesis weight = `1 - query_weight` |
| `max_candidates` | int | `20` | Maximum candidates fetched from each retrieval pass before RRF merge |

```yaml
rag:
  hyde:
    enabled: true
    model: ""              # uses summary_model or main provider
    hypothesis_timeout: 10s
    query_weight: 0.3
    max_candidates: 20
```

### `rag.metrics`

In-memory ring buffer that records per-query retrieval events. Powers
`GET /api/metrics/rag`. Collection is ON by default whenever the RAG
subsystem is enabled.

| Field | Type | Default | Description |
| ----- | ---- | ------- | ----------- |
| `enabled` | bool | `true` | Enable metrics collection |
| `buffer_size` | int | `200` | Ring buffer capacity (number of recent events retained) |

```yaml
rag:
  metrics:
    enabled: true
    buffer_size: 200
```

### Upgrade notes

**`CleanupJunkChunks` migration (v0.8.0)**: At startup, Daimon runs a
one-shot idempotent migration that deletes chunks matching the "1-rune-drop
suffix" pattern introduced by a chunker bug in earlier versions. The migration
is safe to run multiple times and causes no data loss beyond removing the
malformed chunks. No action required from users.

**`rag.hyde` is opt-in**: HyDE is disabled by default (`enabled: false`).
To activate it, add `rag.hyde.enabled: true` to your config. HyDE requires
`rag.embedding.enabled: true` — without vector embeddings it has no effect.

**`auto_index_outputs`**: This field (under `agent.context_mode`) now defaults
to `true` whenever the store is SQLite, independent of `context_mode`. Tool
outputs are indexed to FTS5 so the agent can search them later via
`search_output` without re-running the command.
