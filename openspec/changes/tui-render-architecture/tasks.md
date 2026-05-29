# Tasks: tui-render-architecture

## Review Workload Forecast

| Field                   | Value                                  |
| ----------------------- | -------------------------------------- |
| Estimated changed lines | ~460 (PR-1 ~110, PR-2 ~230, PR-3 ~120) |
| 400-line budget risk    | High                                   |
| Chained PRs recommended | Yes                                    |
| Suggested split         | PR-1 → PR-2 → PR-3 (stacked to main)   |
| Delivery strategy       | ask-on-risk                            |
| Chain strategy          | stacked-to-main                        |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal                                                 | Likely PR | Notes                                       |
| ---- | ---------------------------------------------------- | --------- | ------------------------------------------- |
| WU-a | Mode caching — remove live CurrentMode() from render | PR-1      | base: main; goldens must be byte-identical  |
| WU-b | relativeTime pre-compute — remove clock from render  | PR-1      | base: main; stacks with WU-a                |
| WU-c | Viewport + thread cap                                | PR-2      | base: PR-1; golden regen after human review |
| WU-d | Spinner COW batch ticker                             | PR-3      | base: PR-2; snapshot-isolation test         |

---

## PR-1 — Purity: Mode Caching (WU-a) + ago Pre-compute (WU-b) + Determinism Guard

Est. ~110 lines. Base: `main`. Goldens MUST remain byte-identical — verify, do NOT `-update`.

### [x] 1.1 Test harness: viewport.New(0,0) in newTestModel [RED unblock]

- **RED**: confirm non-chat tests that call `newTestModel()` do not panic after WU-c lands
  (viewport nil-deref). Write a placeholder assertion or note in `internal/tui/run_test.go`
  that `newTestModel()` returns a model whose `viewport` field is the zero `viewport.Model`.
- **GREEN**: in `internal/tui/run.go` `newTestModel()`, add `viewport: viewport.New(0, 0)`.
- Files: `internal/tui/run.go`, `internal/tui/run_test.go`

> Note: this task is listed first in PR-1 because it must land before WU-c's golden
> adaptation; placing it here means PR-1 already ships the safe default.

### [x] 1.2 Determinism guard test: TestView_Deterministic [RED]

- **RED**: create `internal/tui/purity_test.go`; implement `TestView_Deterministic` exactly
  as spec §D.5/design §Determinism: build a populated model (screenChat, mode, running
  ToolLine, sessionsAgo, refreshThreadViewport), call `View()` 50 times, assert byte-identical.
  Test must FAIL at this point (mode is still live).
- Files: `internal/tui/purity_test.go` (new)

### [x] 1.3 Determinism guard: sessions + rail variants [RED]

- **RED**: in `purity_test.go`, add `TestView_Deterministic_Sessions` (screenSessions,
  m.sessionsAgo populated) and `TestView_Deterministic_Rail` (resumeListPanel.ago populated).
  Both fail before WU-a/b are green.
- Files: `internal/tui/purity_test.go`

### [x] 1.4 WU-a RED: mode-cache unit tests

- **RED**: in `internal/tui/model_test.go` (or new `purity_test.go` section):
  - `TestMode_CachedField`: call `cycleMode`, assert `m.mode` updated without calling
    `ag.CurrentMode()` from View. Inject a modeAgent stub that `t.Fatal`s if `CurrentMode`
    is called after Update returns.
  - `TestLayout_ReadsCachedMode`: call `renderLayout` with a model whose `modeAgent` is a
    failing stub; assert no panic/no stub call; rendered output contains expected mode label.
  - `TestMode_SlashCommandRefreshes`: send a `commandResultMsg` with command name `"mode"`;
    assert `m.mode` is updated to `ag.CurrentMode()` return value.
- Files: `internal/tui/model_test.go` or `internal/tui/purity_test.go`

### [x] 1.5 WU-a GREEN: add mode field + wire it

- **GREEN** for 1.4:
  - `internal/tui/model.go`: add `mode string` to `Model` struct.
  - `internal/tui/run.go`: set `mode: ag.CurrentMode()` in the production `Model{}` literal.
  - `internal/tui/model.go` `cycleMode()`: after `SetModeImmediate(next)`, add `m.mode = next`.
  - `internal/tui/model.go` `commandResultMsg` handler: when dispatched command == `"mode"`,
    set `m.mode = m.ag.CurrentMode()` (guard `m.ag != nil`).
  - `internal/tui/layout.go`: delete the `modeAgent.CurrentMode()` block (lines 66–71);
    replace with `currentMode := m.mode`; remove any remaining live agent read in layout.
- Run `make test` (must be GREEN). Run `make test-race`.
- Files: `internal/tui/model.go`, `internal/tui/run.go`, `internal/tui/layout.go`

### [x] 1.6 WU-a: verify golden byte-identical

- Confirm `TestModel_View_ChatScreen_Golden` still passes without `-update`.
  No mode change should affect the golden because `mode:""` → "BUILD" default is unchanged.
  If it fails, investigate rather than blind-update.
- Files: `internal/tui/golden_test.go` (read-only verify)

### [x] 1.7 WU-b RED: ago pre-compute unit tests

- **RED**:
  - `TestResumeListPanel_PrecomputedAgo` in `internal/tui/rail_panels_test.go`:
    call `setSessions` with convs; assert `p.ago` slice is populated and `Render` output
    contains the pre-stored string. Introduce a test helper that patches `relativeTime`
    or freezes time — no clock call allowed in Render after this WU.
  - `TestSessions_PrecomputedAgo` in `internal/tui/screen_sessions_test.go`:
    send `sessionsLoadedMsg`; assert `m.sessionsAgo` is populated in returned model.
  - `TestView_Deterministic` variants from 1.2/1.3 must now also become GREEN as ago
    is no longer computed in View. Confirm they pass after this step.
- Files: `internal/tui/rail_panels_test.go`, `internal/tui/screen_sessions_test.go`

### [x] 1.8 WU-b GREEN: pre-compute ago in Update

- **GREEN** for 1.7:
  - `internal/tui/rail_panels.go`: add `ago []string` field to `resumeListPanel`; in
    `setSessions` allocate `p.ago = make([]string, len(convs))` and fill via `relativeTime`.
  - `internal/tui/rail_panels.go` `Render`: replace `ago := relativeTime(conv.UpdatedAt)`
    with index into `p.ago` (guarded `i < len(p.ago)`).
  - `internal/tui/model.go`: add `sessionsAgo []string` to `Model`.
  - `internal/tui/model.go` `sessionsLoadedMsg` handler: after setting `m.sessions`, fill
    `m.sessionsAgo` with `relativeTime` calls.
  - `internal/tui/screen_sessions.go` `renderSessions`: replace `relativeTime(conv.UpdatedAt)`
    with `m.sessionsAgo[i]` (guarded by `i < len(m.sessionsAgo)`).
- Run `make test`, `make test-race`, `make lint`.
- Files: `internal/tui/rail_panels.go`, `internal/tui/model.go`, `internal/tui/screen_sessions.go`

### [x] 1.9 PR-1 determinism tests GREEN + commit

- All three `TestView_Deterministic*` variants must be GREEN at this point.
  Run `make test-race`. Confirm no race on View or any Render path.
- Commit: work-unit commit per WU-a then WU-b, tests with code.
  PR-1 total: ~110 lines, independently green.

---

## PR-2 — Viewport + Thread Memory Cap (WU-c)

Est. ~230 lines. Base: PR-1 branch. After merge PR-1 → main, rebase/stack on main.
Chat golden MUST be regenerated after human diff review — do NOT blind `-update`.

### 2.1 Test harness: WindowSizeMsg in golden test [RED unblock]

- **RED**: update `TestModel_View_ChatScreen_Golden` in `internal/tui/golden_test.go` to
  drive `m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})` before calling `View()`.
  Without the viewport sized, `viewport.View()` returns empty — the test currently passes
  with direct field access; after WU-c it will fail with blank content. Mark this test as
  expected-to-fail (or skip) until WU-c GREEN is in place.
- Files: `internal/tui/golden_test.go`

### 2.2 WU-c RED: viewport unit tests (sizing + transitions)

- **RED** — new tests in `internal/tui/screen_chat_test.go` or new `viewport_test.go`:
  - `TestViewport_WindowSizeMsg_Propagates`: send `WindowSizeMsg{80,24}`; assert
    `m.viewport.Width` and `m.viewport.Height` are set to expected values via `chatViewportSize`.
  - `TestViewport_StickToBottom`: populate thread, size viewport; assert `viewport.AtBottom()`;
    append item; assert still at bottom.
  - `TestViewport_FreezeWhenScrolledUp`: scroll up (`YOffset > 0`); append item; assert
    `YOffset` unchanged.
  - `TestViewport_ResetOnTransition`: set `YOffset = 10`; trigger a chat screen transition;
    assert `YOffset == 0`.
- Files: `internal/tui/screen_chat_test.go` (or new `internal/tui/viewport_test.go`)

### 2.3 WU-c RED: thread cap tests

- **RED** — in `internal/tui/components_thread_test.go`:
  - `TestThreadCap_DropsOldest`: append 501 items; assert `len(items) == 500` and first
    item is the second item originally appended (oldest dropped).
  - `TestThreadCap_TruncationMarker`: append 501 items; call `Render(80)`; assert output
    contains truncation marker string.
  - `TestThreadCap_NilStylesSafe`: zero-value `thread{}` with truncated=true; `Render(80)` must
    not panic.
- Files: `internal/tui/components_thread_test.go`

### 2.4 WU-c RED: scroll key routing tests

- **RED** — in `internal/tui/screen_chat_test.go`:
  - `TestScrollKeys_DoNotStealEditor`: focusEditor + PgDown; assert viewport YOffset changes
    AND input field is unchanged.
  - `TestScrollKeys_ArrowsScrollWhenFocusMain`: focusMai + Down; assert viewport advances.
  - `TestScrollKeys_ArrowsNoopWhenFocusEditor`: focusEditor + Down; assert viewport unchanged.
- Files: `internal/tui/screen_chat_test.go`

### 2.5 WU-c GREEN: add viewport field + constructors

- **GREEN** (unblocks 2.2):
  - `internal/tui/model.go`: add `viewport viewport.Model` to `Model` struct.
  - `internal/tui/run.go`: set `viewport: viewport.New(0, 0)` in production constructor
    (also confirms task 1.1 is properly wired).
  - `internal/tui/run.go` `newTestModel()`: set `viewport: viewport.New(0, 0)` (task 1.1 GREEN).
- Files: `internal/tui/model.go`, `internal/tui/run.go`

### 2.6 WU-c GREEN: chatViewportSize + layout math extraction

- **GREEN**:
  - `internal/tui/layout.go`: extract chrome-reservation math into `chatViewportSize(m Model)
(vw, vh int)` helper. Used by both `renderLayout` and the WindowSizeMsg handler.
  - Ensure existing layout tests pass; `make test`.
- Files: `internal/tui/layout.go`, `internal/tui/layout_test.go`

### 2.7 WU-c GREEN: refreshThreadViewport + WindowSizeMsg propagation

- **GREEN** (resolves 2.2):
  - `internal/tui/model.go`: implement `refreshThreadViewport() Model` per design §C.2
    (AtBottom-gated GotoBottom; bake breadcrumb into content).
  - `internal/tui/model.go` WindowSizeMsg handler: call `chatViewportSize`, set viewport
    dimensions, call `m.refreshThreadViewport()`.
  - Wire `refreshThreadViewport()` at all thread-mutation call sites in `updateChat` and
    `handleBusEvent` (EventToolStart, EventSubagentSpawned, EventReasoningStart, EventToolEnd,
    EventReasoningEnd, spinnerTickMsg placeholder, r-toggle, agentReplyMsg, optimistic MsgUser,
    breadcrumb/token updates).
- Run `make test`.
- Files: `internal/tui/model.go`, `internal/tui/screen_chat.go`

### 2.8 WU-c GREEN: renderChat delegates to viewport

- **GREEN**:
  - `internal/tui/screen_chat.go` `renderChat`: replace full thread render with
    `m.viewport.View()` (guard nil/empty items → placeholder).
- Files: `internal/tui/screen_chat.go`

### 2.9 WU-c GREEN: YOffset reset on screen transitions

- **GREEN** (resolves 2.2 `TestViewport_ResetOnTransition`):
  - In each transition-into-chat site (`updateWelcome` Enter, `updateSessions` Enter,
    session resume in `screen_sessions.go`): add `m.viewport.SetContent(""); m.viewport.GotoTop();
m = m.refreshThreadViewport()`.
- Files: `internal/tui/model.go`, `internal/tui/screen_sessions.go`

### 2.10 WU-c GREEN: scroll key routing

- **GREEN** (resolves 2.4):
  - `internal/tui/screen_chat.go` `handleChatKey`: add PgUp/PgDn/ctrl+u/ctrl+d → always
    forward to viewport.Update; Up/Down → forward only when `m.focus != focusEditor`.
- Files: `internal/tui/screen_chat.go`

### 2.11 WU-c GREEN: thread cap + truncation marker

- **GREEN** (resolves 2.3):
  - `internal/tui/components_thread.go`: add `truncated bool` and `styles tuiStyles` fields
    to `thread`; add `const maxThreadItems = 500`; update `append` with drop-oldest +
    `truncated = true` logic.
  - `internal/tui/components_thread.go` `Render`: prepend truncation marker when `t.truncated`.
  - `internal/tui/run.go`: set `thread.styles` in production constructor and `newTestModel()`.
- Files: `internal/tui/components_thread.go`, `internal/tui/run.go`

### 2.12 WU-c: golden regen (human-reviewed)

- Run `make test` — `TestModel_View_ChatScreen_Golden` fails (viewport changes output).
- Inspect the diff against the existing golden file at
  `internal/tui/testdata/golden/chat_screen*.txt` (or equivalent path).
- Review for unintended visual drift (missing content, broken layout, missing breadcrumb).
- Only after human sign-off: `go test ./internal/tui/... -update` to regenerate.
- Confirm `make test` passes GREEN with new golden.
- Files: `internal/tui/golden_test.go`, `internal/tui/testdata/golden/` (golden snapshots)

### 2.13 WU-c: extend TestView_Deterministic with viewport variant

- Add `TestView_Deterministic_Viewport` in `internal/tui/purity_test.go`: build model with
  running tool, send WindowSizeMsg, call `View()` 50 times, assert byte-identical.
- Run `make test-race`. Confirm GREEN.
- Files: `internal/tui/purity_test.go`

### 2.14 PR-2 lint + commit

- `make lint` clean. Work-unit commits. PR-2 independently green on `make test` +
  `make test-race`. ~230 lines.

---

## PR-3 — Spinner COW Batch Ticker (WU-d)

Est. ~120 lines. Base: PR-2 branch. Stack on PR-2.

### 3.1 WU-d RED: snapshot isolation test

- **RED** in `internal/tui/components_thread_test.go` (or new `internal/tui/spinner_test.go`):
  - `TestSpinner_SnapshotIsolation`: build model A with k=1 running ToolLine; call
    `spinnerTickMsg` handler to get model B; call `A.View()` and assert spinner frame is
    unchanged from pre-tick frame (D.6 property). Must FAIL while old per-ToolLine code runs.
  - `TestSpinner_BatchAdvance_SingleCopy`: build model with k=3 running ToolLines; send one
    `spinnerTickMsg{}` (no callID); assert all three lines advanced AND prior snapshot shows
    no mutation.
  - `TestSpinner_TickerSelfStops`: with zero running tools, send `spinnerTickMsg{}`; assert
    returned Cmd is nil (no re-arm).
  - `TestSpinner_ArmingDedupe`: arm spinner twice (two EventToolStart); assert only one
    ticker is running (`m.spinnerActive == true` after first; second arm is no-op).
- Files: `internal/tui/components_thread_test.go` or `internal/tui/spinner_test.go` (new)

### 3.2 WU-d GREEN: thread.own() + runningToolIdxs()

- **GREEN**:
  - `internal/tui/components_thread.go`: add `func (t *thread) own()` (make+copy as per D.3,
    WITHOUT `ownedGen` — D.7 commits to dropping it). Add `func (t *thread) runningToolIdxs()
[]int` (iterate items, return indices where item is `*ToolLine` with `state == toolRunning`).
- Files: `internal/tui/components_thread.go`

### 3.3 WU-d GREEN: single model-level spinner ticker

- **GREEN** (resolves 3.1 batch + self-stop + dedup tests):
  - `internal/tui/model.go`: add `spinnerActive bool` to `Model`.
  - `internal/tui/screen_chat.go` (or `model.go`) `handleBusEvent` EventToolStart: arm
    single `spinnerTickCmd()` only when `!m.spinnerActive`; set `m.spinnerActive = true`.
  - `internal/tui/model.go`: replace `spinnerTickMsg{callID}` handler with batch handler
    per design §D.5: call `runningToolIdxs()`; if empty return `m, nil`; else `own()`, loop
    advance, `refreshThreadViewport()`, return `m, spinnerTickCmd()`.
  - Remove per-`ToolLine.Tick()` fan-out from EventToolStart and any other call sites.
  - `internal/tui/components_thread.go`: remove (or keep stub) `ToolLine.Tick()` if it was
    the per-line ticker; or repurpose as `AdvanceSpinner()` only.
- Files: `internal/tui/model.go`, `internal/tui/screen_chat.go`, `internal/tui/components_thread.go`

### 3.4 WU-d GREEN: refactor EventToolEnd / EventReasoningEnd / r-toggle onto own()

- **GREEN** (behavior-identical, proven by D.6):
  - Replace each bespoke `make + copy` in EventToolEnd, EventReasoningEnd, and the `r`-toggle
    handler with a call to `m.thread.own()` followed by the in-place slot write.
  - Run `make test` to confirm behavior-identical refactor. No new behavior, no new tests
    needed beyond what already covers these handlers.
- Files: `internal/tui/screen_chat.go` (or wherever these handlers live)

### 3.5 WU-d: drop ownedGen / global counter (verify absent)

- Confirm no `ownedGen`, `threadGenSeq`, or `nextThreadGen` exists in the diff. Per D.7,
  these were explicitly rejected. If any leaked in from earlier drafts, remove them.
  `grep -r ownedGen internal/tui/` must return empty.
- Files: `internal/tui/components_thread.go`, `internal/tui/model.go`

### 3.6 PR-3 test-race + lint + commit

- `make test-race` clean. `make lint` clean.
- Work-unit commit: `thread.own() + runningToolIdxs()` together, then batch ticker + refactor.
  PR-3 independently green. ~120 lines.

---

## Summary

| PR        | Work units  | Tasks                 | Est. lines |
| --------- | ----------- | --------------------- | ---------- |
| PR-1      | WU-a + WU-b | 1.1 – 1.9 (9 tasks)   | ~110       |
| PR-2      | WU-c        | 2.1 – 2.14 (14 tasks) | ~230       |
| PR-3      | WU-d        | 3.1 – 3.6 (6 tasks)   | ~120       |
| **Total** |             | **29 tasks**          | **~460**   |

All tasks follow RED → GREEN → REFACTOR. Test runner: `make test` (race: `make test-race`, lint: `make lint`).
