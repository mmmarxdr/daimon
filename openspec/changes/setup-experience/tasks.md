# Tasks: Setup Experience

## Phase 1 — Quick Fixes (store.type, WhatsApp, dev.sh)

- [ ] 1.1 Write failing test: `buildConfig()` produces `store.type: "file"` for all channels (table-driven in `wizard_test.go`)
- [ ] 1.2 Fix `wizard.go:724` — change `cfg.Store.Type = "sqlite"` to `"file"`, fix `audit.type` line 730 similarly
- [ ] 1.3 Write failing test: `nextStep("whatsapp")` returns `stepChannelExtra`, `prevStep` also routes correctly
- [ ] 1.4 Add `"whatsapp"` to channel choices in `wizard.go:438`, add to `nextStep`/`prevStep` routing alongside telegram/discord
- [ ] 1.5 Write failing test: WhatsApp config fields (phone_number_id, access_token, verify_token) present in `buildConfig` output
- [ ] 1.6 Add WhatsApp input fields to `stepChannelExtra` view and `buildConfig()` handler
- [ ] 1.7 Replace `./dev.sh run` references in `README.md` (lines 40, 62, 65, 68) with `go run ./cmd/daimon`
- [ ] 1.8 Fix `Makefile:38` — replace `./dev.sh run $(ARGS)` with `go run ./cmd/daimon $(ARGS)`
- [ ] 1.9 Run `go vet ./...` and `golangci-lint run`, fix all issues; verify `go test -race ./internal/setup/...` passes

## Phase 2 — `daimon setup` Subcommand

- [ ] 2.1 Write failing test: `runSetupCommand()` calls `setup.RunWizard()` and returns nil on success (mock RunWizard)
- [ ] 2.2 Add `os.Args[1] == "setup"` dispatch block in `main.go` before `flag.Parse()`, mirroring existing mcp/skills/cron/config pattern
- [ ] 2.3 Verify `daimon setup` launches wizard identically to `--setup` (manual TTY test)
- [ ] 2.4 Run `go vet ./...` and `golangci-lint run`; verify race detector passes

## Phase 3 — Config Path + Validation

- [x] 3.1 Write failing test: wizard warns when API key is empty for non-ollama provider
- [x] 3.2 Add API key format validation in wizard (non-empty check for non-ollama, warn-only, non-blocking)
- [x] 3.3 Write failing test: wizard detects local `./config.yaml` and offers to write there
- [x] 3.4 Add local config path detection in wizard write step — check `./config.yaml` exists, prompt user for path choice
- [x] 3.5 Write failing test: XDG path used when `XDG_CONFIG_HOME` is set
- [x] 3.6 Add opt-in XDG config path support — only when `XDG_CONFIG_HOME` env var is explicitly set
- [x] 3.7 Run `go vet ./...` and `golangci-lint run`; verify race detector passes

## Phase 4 — `daimon doctor` Command

- [ ] 4.1 Write failing test: `runDoctorCommand()` with valid config reports "OK"; with missing file reports error (table-driven)
- [ ] 4.2 Create `cmd/daimon/doctor.go` with `runDoctorCommand(cfgPath string) error` — load config, call validate()
- [ ] 4.3 Write failing test: doctor reports missing env vars referenced in config placeholders
- [ ] 4.4 Add env var checking logic — parse `${VAR}` placeholders from config, report which are set/missing
- [ ] 4.5 Write failing test: doctor checks store.path directory exists and is writable
- [ ] 4.6 Add store path accessibility check to doctor command
- [ ] 4.7 Add `os.Args[1] == "doctor"` dispatch block in `main.go`
- [ ] 4.8 Run `go vet ./...`, `golangci-lint run`, full test suite `go test -race ./...`, verify all pass

## Phase 5 — Documentation + Verification

- [ ] 5.1 Update README.md — document `daimon setup` and `daimon doctor` commands
- [ ] 5.2 Run full verification: `go vet ./...`, `golangci-lint run`, `go test -race -count=1 ./...`
- [ ] 5.3 Verify all spec scenarios pass (manual + automated)

---

## Total: 27 tasks across 5 phases
| Phase | Tasks | Focus |
|-------|-------|-------|
| Phase 1 | 9 | Quick fixes: store.type, WhatsApp, dev.sh |
| Phase 2 | 4 | setup subcommand wiring |
| Phase 3 | 7 | Config path + API key validation |
| Phase 4 | 8 | doctor command + env checks |
| Phase 5 | 3 | Docs + final verification |

## Implementation Order
Phase 1 first (zero-risk fixes, immediate value). Phase 2 next (trivial wiring). Phase 3 (moderate effort, config polish). Phase 4 (new file, most complex). Phase 5 (docs last, after all code is stable).

## Key Decisions (from design)
- store.type: "file" (not "sqlite") — matches documented default
- WhatsApp reuses `stepChannelExtra` — same complexity as telegram
- `setup` subcommand uses manual `os.Args` dispatch — matches existing pattern
- `doctor` as separate `cmd/daimon/doctor.go` — keeps main.go clean
- XDG is opt-in only when `XDG_CONFIG_HOME` is set — no breaking changes
- Doctor does env-check-only (no API calls) — avoids key leakage in logs
