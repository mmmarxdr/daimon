# tui-render-purity Specification

## Purpose

Define the behavioral contract for the TUI's rendering layer after the
`tui-render-architecture` change. This spec describes what the system MUST
guarantee once purity, bounded cost, and bounded memory are enforced — not how
those guarantees are implemented.

---

## Requirements

### Requirement: View Purity

`Model.View()` and every `Render(width)` it transitively calls MUST be a
deterministic, pure function of the receiver's fields alone. They MUST NOT
read live/shared objects (no `agent.CurrentMode()`, no RLocks into running
services), MUST NOT call `time.Now()` or `time.Since()`, MUST NOT perform IO,
and MUST NOT mutate any state.

#### Scenario: Repeated View calls produce identical output

- GIVEN a fully-populated Model containing running tool lines, session "ago"
  strings, and an active mode
- WHEN `View()` is called N times consecutively with no intervening `Update`
- THEN all N outputs are byte-identical
- AND the determinism guard test asserts this with N ≥ 3 and fails on any
  byte difference

#### Scenario: Race detector finds no concurrent live-object access

- GIVEN the same populated Model
- WHEN the test suite is executed with `-race`
- THEN no data race is reported on `View()` or any Render path

---

### Requirement: Mode Cached in Model

The displayed agent mode MUST reflect the value stored in the Model, not a
live lookup. The Model field MUST be updated via `Update` when a mode-change
event is received.

#### Scenario: Mode display reflects cached field

- GIVEN a Model whose mode field is set to `FOCUSED`
- WHEN `View()` is called
- THEN the rendered output contains the `FOCUSED` mode label

#### Scenario: Mode updates through Update, not View

- GIVEN a Model with mode field `FOCUSED`
- WHEN `Update` receives a mode-change message carrying `AUTONOMOUS`
- THEN the returned Model's mode field is `AUTONOMOUS`
- AND a subsequent `View()` call renders `AUTONOMOUS` without any additional
  message

---

### Requirement: Relative Timestamps Pre-computed

Session "ago" strings MUST be computed in the `Update` path (when the
source data arrives) and stored in the Model. `View()` and all `Render`
methods MUST read the stored strings; they MUST NOT call `time.Since()` or
any clock function.

#### Scenario: Ago string rendered from stored value

- GIVEN a Model whose session list entries carry pre-computed ago strings
  (e.g. `"3m ago"`)
- WHEN `View()` is called
- THEN the rendered output contains those exact stored strings
- AND no `time.Since()` call occurs during the render

#### Scenario: Ago string frozen between events

- GIVEN a session list loaded at time T with ago strings `"1m ago"`
- WHEN no further Update messages arrive for several minutes
- THEN `View()` continues to render `"1m ago"` (staleness accepted per policy)

---

### Requirement: Bounded Render Cost

The chat thread render cost MUST be bounded by the visible window height, not
by the total number of items in thread history. Render cost MUST NOT grow as
the session accumulates messages.

#### Scenario: Render cost does not grow with thread length

- GIVEN a thread with N items where N exceeds the visible viewport height
- WHEN `View()` is called
- THEN only the rows within the visible window are materialized
- AND the render work is O(viewport-height), not O(N)

#### Scenario: Scroll position preserved when user has scrolled up

- GIVEN a thread where the user has scrolled up from the bottom
- WHEN a new message arrives and `Update` processes it
- THEN the viewport's scroll offset is unchanged
- AND the user's reading position is not interrupted

#### Scenario: Auto-scroll to bottom for new messages when at bottom

- GIVEN a thread where the user is at the bottom of the viewport
- WHEN a new message arrives and `Update` processes it
- THEN the viewport scrolls to show the newest message

---

### Requirement: Bounded Thread Memory

Thread history MUST be capped at a finite item limit. When the limit is
reached and a new item arrives, the oldest items MUST be trimmed in the
`Update` path. The UI MUST NOT silently drop items without any indication;
truncation MUST be communicated to the user.

#### Scenario: Items trimmed when cap is exceeded

- GIVEN a thread already at the item cap
- WHEN `Update` processes a new message
- THEN the oldest items are removed to make room
- AND the total item count does not exceed the cap

#### Scenario: Truncation is visible, not silent

- GIVEN a thread that has been trimmed at least once
- WHEN the user scrolls to the top of the visible thread
- THEN a truncation indicator is shown (e.g. a notice that earlier history
  was trimmed), so the user is not misled into thinking the top is the
  conversation start

---

### Requirement: Bounded Spinner Update Cost

Advancing a running tool's spinner animation MUST be O(1) per tick. The
`spinnerTickMsg` handler MUST NOT copy the entire items slice on every tick.
Snapshot isolation between Model values MUST be preserved.

#### Scenario: Spinner tick does not copy full items slice

- GIVEN a thread with N items where exactly one tool line is running
- WHEN a `spinnerTickMsg` is received by `Update`
- THEN only that one tool line is copied and updated
- AND the items slice itself is not fully copied on this tick

#### Scenario: Prior Model snapshot unaffected by current-tick update

- GIVEN two Model values A and B where B is the result of advancing A's
  spinner
- WHEN `A.View()` is called after B was created
- THEN A's rendered spinner frame is unchanged (no retroactive mutation)

---

### Requirement: No Behavioral or Visual Regression

The chat screen's visible structure MUST be preserved across this change.
Speaker headers, tool-line columns, breadcrumb, and reasoning blocks (Inc.2
thread structure) MUST render identically to their pre-change form for an
equivalent Model state. Scrolling support is additive: it extends capability
without removing any previously rendered element.

#### Scenario: Golden render matches pre-change output for static content

- GIVEN a Model with the same content as the existing golden fixture
- WHEN `View()` is called
- THEN the rendered output matches the golden file byte-for-byte
  (modulo any viewport framing that wraps the same content)
- AND golden regeneration via `-update` is reviewed for unintended drift
  before merging

#### Scenario: Screen transition resets scroll state

- GIVEN a user who has scrolled to the middle of the chat thread
- WHEN the user navigates away and then back to the chat screen
- THEN the viewport scroll offset is reset to the bottom
- AND no content from the previous scroll position bleeds through
