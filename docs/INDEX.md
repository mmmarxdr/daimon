# Daimon Documentation — Index

> Single entry point for both **humans** and **AI agents** reading this codebase.
> Every doc follows a fixed structure so you can predict where to look.

## How to use this index

- **First time?** → Start at [ARCHITECTURE.md](ARCHITECTURE.md) for the system map and message flows.
- **Working on a specific module?** → Go directly to [`modules/<name>.md`](modules/).
- **Looking for cross-cutting health & refactor priorities?** → [VERDICT.md](VERDICT.md).
- **Need to set up, deploy, or run locally?** → [INSTALL.md](INSTALL.md), [DEPLOY.md](DEPLOY.md), [DEVELOPMENT.md](DEVELOPMENT.md).

All module docs follow the canonical template at [`modules/_TEMPLATE.md`](modules/_TEMPLATE.md). If you author or edit one, keep the section order and headings — automated tooling and LLM agents rely on it.

---

## Top-level documents

| Doc                                        | Purpose                                                                         |
| ------------------------------------------ | ------------------------------------------------------------------------------- |
| [ARCHITECTURE.md](ARCHITECTURE.md)         | System map, layering rules, all major message flows (Mermaid sequence diagrams) |
| [VERDICT.md](VERDICT.md)                   | Cross-cutting health: coupling heatmap, prioritized refactors, smells           |
| [INSTALL.md](INSTALL.md)                   | Install daimon on a fresh machine                                               |
| [DEPLOY.md](DEPLOY.md)                     | Production deployment patterns                                                  |
| [DEVELOPMENT.md](DEVELOPMENT.md)           | Local dev workflow, test/run loop                                               |
| [release-pipeline.md](release-pipeline.md) | Release & versioning process                                                    |
| [design-system.md](design-system.md)       | Frontend design system reference                                                |
| [dashboard-design.md](dashboard-design.md) | Web dashboard UX brief                                                          |

---

## Per-module reference

Each entry links to a doc that covers: purpose, public API, dependencies, component & flow diagrams, and a refactor verdict.

### Core (the agent loop and its contracts)

| Module     | Doc                                        | Role                                                                                 |
| ---------- | ------------------------------------------ | ------------------------------------------------------------------------------------ |
| `agent`    | [modules/agent.md](modules/agent.md)       | The message-processing loop: context build → provider call → tool exec → response    |
| `channel`  | [modules/channel.md](modules/channel.md)   | Transport abstraction: CLI, web (WebSocket), heartbeat, future Telegram/Discord      |
| `provider` | [modules/provider.md](modules/provider.md) | LLM API clients: Anthropic, OpenAI, OpenRouter, Ollama. Streaming + tool-use mapping |

### Capabilities (what the agent can do)

| Module   | Doc                                    | Role                                                                        |
| -------- | -------------------------------------- | --------------------------------------------------------------------------- |
| `tool`   | [modules/tool.md](modules/tool.md)     | Built-in tool implementations + registry (shell, fileops, http, browser, …) |
| `mcp`    | [modules/mcp.md](modules/mcp.md)       | MCP client; wraps remote MCP tools as `tool.Tool`                           |
| `skill`  | [modules/skill.md](modules/skill.md)   | Markdown skills loader: behavioral instructions + executable subagents      |
| `filter` | [modules/filter.md](modules/filter.md) | Content filtering / safety policies applied to inbound or tool output       |

### Persistence

| Module    | Doc                                      | Role                                                              |
| --------- | ---------------------------------------- | ----------------------------------------------------------------- |
| `store`   | [modules/store.md](modules/store.md)     | Conversation + memory persistence (FileStore, SQLite)             |
| `content` | [modules/content.md](modules/content.md) | Document content storage backing RAG ingestion                    |
| `rag`     | [modules/rag.md](modules/rag.md)         | Retrieval pipeline (BM25 + cosine + HyDE + RRF + neighbor expand) |

### Subsystems (cross-cutting features)

| Module   | Doc                                    | Role                                             |
| -------- | -------------------------------------- | ------------------------------------------------ |
| `cost`   | [modules/cost.md](modules/cost.md)     | Token & dollar accounting per turn / per channel |
| `audit`  | [modules/audit.md](modules/audit.md)   | Append-only audit log of agent decisions         |
| `notify` | [modules/notify.md](modules/notify.md) | Internal event bus + external notifications      |
| `cron`   | [modules/cron.md](modules/cron.md)     | Scheduled / heartbeat jobs                       |
| `setup`  | [modules/setup.md](modules/setup.md)   | First-launch wizard, env detection               |

### Transports (human-facing surfaces)

| Module | Doc                              | Role                                              |
| ------ | -------------------------------- | ------------------------------------------------- |
| `web`  | [modules/web.md](modules/web.md) | HTTP API + WebSocket `/ws/chat` + static frontend |
| `tui`  | [modules/tui.md](modules/tui.md) | Bubbletea terminal UI                             |

### Cross-cutting

| Module   | Doc                                    | Role                                                             |
| -------- | -------------------------------------- | ---------------------------------------------------------------- |
| `config` | [modules/config.md](modules/config.md) | YAML schema, env-var resolution, validation, hot-reload contract |

---

## Authoring & conventions

- **Source of truth**: code → codegraph → docs. Docs cite the code, never the other way around.
- **Diagrams**: Mermaid only (rendered by GitHub, IDEs, and most LLM tools). For diagrams too dense for Mermaid, drop a `.svg` in `diagrams/` and link.
- **Verdicts**: every per-module doc closes with a verdict (health, coupling, bloat, smells, suggested refactors). Cross-cutting analysis aggregates in [VERDICT.md](VERDICT.md).
- **Slug links**: across module docs, refer to peers as `[[slug]]` (e.g. `[[provider]]`). Pre-merge into proper `modules/<slug>.md` links before publishing.
- **Staleness**: each module doc carries a `Last reviewed` date in its frontmatter. Anything older than 60 days is considered drift and should be re-validated against codegraph.
