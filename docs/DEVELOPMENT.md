# Development

Everything you need to build, test, and ship changes to Daimon.

## Build

```bash
make build           # compile binary (TUI-only)
make build-full      # compile with web frontend
make frontend        # download pre-built frontend assets
make copy-frontend   # copy from a local daimon-frontend checkout
```

## Test

```bash
make test            # unit tests
make test-race       # unit tests with race detector
make lint            # golangci-lint
make ci              # vet + lint + test-race (run this before pushing)
```

## Project structure

```
cmd/daimon/           entrypoint, subcommands
internal/
  agent/              agent loop, context builder
  channel/            CLI, Telegram, Discord, WhatsApp, Web
  provider/           OpenRouter, Anthropic, Gemini, OpenAI, Ollama
  tool/               shell, file, HTTP, MCP tools
  store/              SQLite persistence
  web/                HTTP server, REST API, WebSocket, auth
  mcp/                MCP client (stdio + http)
  cron/               scheduler, daemon mode
  skill/              skill loader, parser
  config/             YAML config, validation
  audit/              audit log
configs/              example config + skill files
```

For the full architecture breakdown, see `DAIMON.md` in the repo root.

## Running locally

```bash
# Agent with CLI channel
./bin/daimon

# Agent with web dashboard
./bin/daimon web

# Re-run the setup wizard
./bin/daimon --setup
```

Config search order: `--config` flag → `~/.daimon/config.yaml` →
`./config.yaml`.

## CLI reference

```bash
daimon                            # start the agent (setup wizard if no config)
daimon --web                      # start with web dashboard
daimon --dashboard                # TUI dashboard (read-only)
daimon --setup                    # re-run setup wizard
daimon --daemon                   # cron-only background mode

daimon web [--port N] [--host H]  # web-only mode
daimon setup                      # setup wizard
daimon doctor                     # validate config
daimon config                     # show active config

daimon mcp list|add|remove|test|validate|manage
daimon skills add|list|info|remove
daimon cron list|info|delete
```

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for the full workflow.
