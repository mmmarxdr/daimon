# Design: setup-experience

## Technical Approach

Three-phase fix targeting the first-run and post-setup experience. Phase 1 patches inconsistencies in the existing wizard. Phase 2 adds a `setup` subcommand for re-running the wizard. Phase 3 adds `doctor` for post-setup validation and improves config path resolution.

## Architecture Decisions

### Decision: Fix store.type to "file" (not "sqlite")

**Choice**: Change `buildConfig()` line 724 from `"sqlite"` to `"file"`.
**Alternatives considered**: Change config defaults to match wizard.
**Rationale**: `file` is the documented default. `sqlite` requires cron/embeddings features most users don't need. Config validation already accepts both.

### Decision: WhatsApp uses stepChannelExtra (not new step)

**Choice**: Add `"whatsapp"` to the `nextStep` routing alongside telegram/discord → stepChannelExtra. Add WhatsApp-specific inputs (phone_number_id, access_token, verify_token) to the existing channelExtra step.
**Alternatives considered**: New dedicated step for WhatsApp fields.
**Rationale**: Reuses existing pattern. WhatsApp needs 3 fields (phone_number_id, access_token, verify_token) — same complexity tier as telegram's token+allowed_users. The stepExtra view can conditionally show fields based on channel type.

### Decision: `setup` subcommand follows existing dispatch pattern

**Choice**: Add `os.Args[1] == "setup"` block before `flag.Parse()`, matching `mcp`/`skills`/`cron`/`config` pattern.
**Alternatives considered**: Use cobra or a command router library.
**Rationale**: Project uses manual `os.Args` dispatch everywhere (lines 66-100 of main.go). Adding a dependency for one command breaks consistency.

### Decision: `doctor` as separate file (cmd/microagent/doctor.go)

**Choice**: New file with `runDoctorCommand()`, following the pattern of other subcommand handlers being in separate files.
**Alternatives considered**: Inline in main.go.
**Rationale**: main.go is already 462 lines. Separation keeps concerns clean. Config validation logic already exists in `config.go:validate()` — doctor reuses it.

### Decision: Config path — no XDG in Phase 3

**Choice**: Keep `~/.microagent/config.yaml` as default. Only add XDG if `XDG_CONFIG_HOME` is explicitly set.
**Alternatives considered**: Full XDG-first migration.
**Rationale**: Breaking existing installs for a convention is bad UX. XDG as opt-in is low-risk.

## Data Flow

```
CLI (microagent setup)
  │
  ├──▶ os.Args[1] == "setup" ──▶ setup.RunWizard()
  │                                    │
  │                              WizardModel (Bubbletea TUI)
  │                                    │
  │                              buildConfig() ──▶ *config.Config
  │                                    │
  │                              WriteConfig(path, cfg) ──▶ ~/.microagent/config.yaml
  │
  └──▶ os.Args[1] == "doctor" ──▶ runDoctorCommand()
                                        │
                                   config.Load() ──▶ validate()
                                        │
                                   check env vars, store path, provider connectivity
                                        │
                                   print diagnostic report
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/setup/wizard.go` | Modify | Fix store.type (line 724: sqlite→file), audit.type (line 730: sqlite→file), add "whatsapp" to channel choices (line 438), add whatsapp to `nextStep`/`prevStep` routing, add WhatsApp fields to stepChannelExtra view and buildConfig |
| `internal/setup/configwriter.go` | Modify | No changes needed — already handles any config path |
| `cmd/microagent/main.go` | Modify | Add `os.Args[1] == "setup"` dispatch block (2 lines, mirrors existing pattern) |
| `cmd/microagent/doctor.go` | Create | `runDoctorCommand(cfgPath string) error` — loads config, validates, checks env vars, reports health |
| `README.md` | Modify | Replace `./dev.sh run` references with `go run ./cmd/microagent` |
| `Makefile` | Modify | Fix `dev.sh run` target to use `go run` |

## Interfaces / Contracts

### New: `runDoctorCommand` signature
```go
// cmd/microagent/doctor.go
func runDoctorCommand(cfgPath string) error
```

### Modified: `buildConfig()` changes (wizard.go)
```go
// Line 724: cfg.Store.Type = "file"  (was "sqlite")
// Line 730: cfg.Audit.Type = "file"  (was "sqlite")
```

### Modified: `nextStep` / `prevStep` — whatsapp routing
```go
// In nextStep, stepChannel case:
case "telegram", "discord", "whatsapp":
    return stepChannelExtra

// In prevStep, stepStorePath case:
case "telegram", "discord", "whatsapp":
    return stepChannelExtra
```

### Modified: channel choices (line 438)
```go
choices: []string{"cli", "telegram", "whatsapp", "discord"},
```

### Modified: buildConfig whatsapp handling
```go
if channel == "whatsapp" {
    cfg.Channel.PhoneNumberID = m.whatsappPhoneIDInput.Value()
    cfg.Channel.AccessToken = m.whatsappAccessTokenInput.Value()
    cfg.Channel.VerifyToken = m.whatsappVerifyTokenInput.Value()
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `buildConfig()` produces correct store.type | Table-driven: verify each channel type produces expected config |
| Unit | `nextStep`/`prevStep` routing for whatsapp | Table-driven: all step transitions |
| Unit | `runDoctorCommand` with valid/invalid config | Mock config.Load, assert output |
| Integration | `microagent setup` launches wizard | Manual TTY test |
| Integration | WhatsApp channel created from wizard config | Verify `channel.NewWhatsAppChannel` succeeds with wizard output |

## Migration / Rollout

No migration required. Changes are backwards-compatible:
- Existing `--setup` flag still works
- `microagent setup` is additive
- Changing store.type default only affects NEW configs written by wizard
- Existing configs with `store.type: sqlite` continue to work

## Open Questions

- [ ] WhatsApp webhook_port/webhook_path — should wizard expose these? Default (8080, /webhook) works for most cases. Recommend skip for now.
- [ ] Should doctor check provider API key validity via a lightweight API call? Could leak keys in logs. Recommend env-var-only check for Phase 3.
