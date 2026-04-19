# Changelog

All notable changes to Daimon are documented here.

---

## [v0.4.0] — BREAKING: Product renamed microagent → daimon

**Release date**: 2026-04-19

This release completes the product rename from `microagent` to `daimon`.
It is a breaking change for all users. No backward compatibility is provided.

### Breaking changes

| What changed | Old | New |
|-------------|-----|-----|
| Binary name | `microagent` | `daimon` |
| Config directory | `~/.microagent/` | `~/.daimon/` |
| Database filename | `microagent.db` | `daimon.db` |
| Web token env var | `MICROAGENT_WEB_TOKEN` | `DAIMON_WEB_TOKEN` |
| Jina API key env var | `MICROAGENT_JINA_API_KEY` | `DAIMON_JINA_API_KEY` |
| Secret key env var | `MICROAGENT_SECRET_KEY` | `DAIMON_SECRET_KEY` |
| Go module path | `module microagent` | `module daimon` |
| GitHub repository | `github.com/mmmarxdr/micro-claw` | `github.com/mmmarxdr/daimon` |

### Migration steps (manual — no automatic migration)

1. **Move config directory:**
   ```bash
   mv ~/.microagent ~/.daimon
   ```

2. **Rename the database file:**
   ```bash
   mv ~/.daimon/data/microagent.db ~/.daimon/data/daimon.db
   ```

3. **Update environment variables** in your shell profile or secrets manager:
   - `MICROAGENT_WEB_TOKEN` → `DAIMON_WEB_TOKEN`
   - `MICROAGENT_JINA_API_KEY` → `DAIMON_JINA_API_KEY`
   - `MICROAGENT_SECRET_KEY` → `DAIMON_SECRET_KEY`

4. **Update any systemd service files** or scripts that reference the old
   binary name or env vars.

5. **Go consumers** (if you use `go install`): the module path is now
   `github.com/mmmarxdr/daimon/cmd/daimon`. Update your `go.mod` accordingly.

### What does NOT change

- Configuration file format — YAML structure is unchanged
- API endpoints — all REST and WebSocket routes are unchanged
- Cookie name (`auth`) — unchanged
- Data format — existing conversations and memory entries are compatible
  after the db rename above

---

*Older pre-0.4.0 entries are not documented here (pre-public-release history).*
