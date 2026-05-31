# Exploration: tui-render-architecture

> SDD explore phase. Goal: rearchitect the embedded TUI around a hard invariant —
> **View is a pure function of Model; all side effects / live-object access live
> in Update** — and bound the chat thread's per-frame cost and memory.

## Current state

Single-root Bubble Tea model (`Model`, `internal/tui/model.go`) with 7 screen
states and an imperative component pattern (structs with `Render` methods, not
nested `tea.Model`). `View()` → `renderLayout(m)` on every frame, delegating to
screen render functions and panel `Render` methods. Bubble Tea v1 recomputes the
full `View` string per message (every keystroke); the framework diffs the string
to minimize terminal writes, but the string-building cost is ours.

## Impurity inventory (complete, all 7 screens)

| Location                                     | Impurity                                                                              | Verdict                                 |
| -------------------------------------------- | ------------------------------------------------------------------------------------- | --------------------------------------- |
| `layout.go:70`                               | `m.modeAgent.CurrentMode()` — RLock + LookupMode into the live agent every frame      | **MUST FIX**                            |
| `rail_panels.go:615`                         | `relativeTime(conv.UpdatedAt)` in `resumeListPanel.Render` — `time.Since()` in Render | **MUST FIX**                            |
| `screen_sessions.go:134`                     | `relativeTime(conv.UpdatedAt)` in `renderSessions` — `time.Since()` in Render         | **MUST FIX**                            |
| `screen_chat.go:128`                         | `m.ag.CurrentMode()` in `applyDenial`                                                 | CLEAN — Update path                     |
| `screen_chat.go:268`                         | `relativeTime(ev.Timestamp)` in `handleBusEvent`                                      | CLEAN — Update path, cached in `bc.ago` |
| `components_thread.go:501`                   | `nowHHMM()` = `time.Now()`                                                            | CLEAN — only called from Update         |
| `model.go:410,421`; `screen_chat.go:380,393` | `m.ag.Commands()`                                                                     | CLEAN — key handlers (Update path)      |

## Thread cost

- `thread.Render(width)` iterates ALL items and joins → **O(n)** string allocation per frame.
- `spinnerTickMsg` copy-on-write (`screen_chat.go:56-59`): `make + copy` of the full items slice every 100ms per running tool → **O(n)** per tick.
- `thread.items` (`components_thread.go:47`) is only appended, never trimmed → unbounded memory.

## Leak audit — verdict: NO leaks

- Bus subscription (`events.go:65-73`): registered once, process-lifetime, GC'd on exit.
- `pumpEvents` (`events.go:39-43`): a one-shot `tea.Cmd`, blocks for one msg then returns; re-armed by Update. No persistent goroutine.
- Mux reply goroutine (`events.go:77-98`): bounded — exits on `ch.done` (closed by `TUIChannel.Stop()` via `sync.Once`) or `ctx.Done()`.
- Spinner `tea.Tick` (`components_thread.go:292-296`): self-cancels when `state != toolRunning`; orphaned ticks fall through to a no-op.

## bubbles/viewport

Already present in `go.mod` (`bubbles v1.0.0`). Provides `SetContent`, `View()`
(renders only the visible window), `GotoBottom()`, `AtBottom()`. No new dependency.

## Approaches

**A — Minimal purity fix (no viewport).** Cache `mode` as a Model field; pre-compute
relativeTime ago strings in Update. ~3 files, ~30 lines, zero test risk. Does NOT
fix O(n) frame cost or unbounded memory.

**B — Viewport + mode caching + relativeTime fix (RECOMMENDED).** All of A, plus
`bubbles/viewport` in `renderChat` and a cap on `thread.items` (e.g. 500). Render
becomes O(viewport-height) instead of O(n); memory bounded. Deps already present.
Separable into 3 atomic commits. Cost: viewport adds `YOffset` state to Model;
golden files need `-update`; test helpers need viewport init.

**C — Dirty-flag incremental rendering.** Cache the rendered thread string, re-render
only when dirty. Best per-frame cost, but cache-invalidation/resize complexity. High effort.

## Recommendation

**Approach B**, three independent commits:

1. Cache `m.mode` as a Model field; remove `modeAgent.CurrentMode()` from `renderLayout`.
2. Pre-compute relativeTime ago strings on `sessionsLoadedMsg`; remove `relativeTime` from `resumeListPanel.Render` and `renderSessions`.
3. Integrate `bubbles/viewport` into `renderChat`; cap `thread.items`.

Each commit is independently green on `make test`.

## Risks / open questions for the proposal

- Viewport `YOffset` must reset on screen transitions (welcome→chat→sessions) to avoid stale scroll bleed.
- `tea.WindowSizeMsg` must propagate width/height to `m.viewport` in the global handler.
- `resumeListPanel` has no Update path — pre-compute ago strings in `setSessions` (add an `ago []string` field).
- **Spinner COW O(n) is NOT fixed by viewport** (viewport changes only the render path, not the Update COW) — track as a follow-up.
- relativeTime staleness between events — may need a periodic refresh Cmd, or accept "as of last event".
- Auto-scroll policy (stick-to-bottom vs freeze when scrolled up) and the items cap value (500?) need a decision.
- Golden chat-screen tests require `-update` regeneration.

## Status

Ready for proposal. Engram: `sdd/tui-render-architecture/explore` (ID 563).
