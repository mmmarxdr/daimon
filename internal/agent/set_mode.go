package agent

// set_mode.go — SetMode method + loadMode helper for mode-system PR1.
//
// Design references:
//   - AD-4: persist-first ordering: validate → check cancels → load/create conv →
//           set Metadata → SaveConversation → THEN modeMu.Lock + update currentMode.
//   - AD-8: loadMode reads conv.Metadata["daimon/mode"], validates via LookupMode,
//           defaults to "build" on absence/invalid. Read-only: does NOT write back.
//   - AD-2: modeMu guards currentMode. Lock ordering: cancels.mu (inside Size()) →
//           [off-lock validation + persistence] → modeMu.Lock (cache refresh).
//           No nesting with providerMu.
//   - AD-11: ErrInvalidMode error string is contract-locked (from modes.go).
//
// Spec coverage: REQ-2, REQ-3, REQ-4, REQ-5, REQ-10, REQ-15.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// SetMode
// ---------------------------------------------------------------------------

// SetMode swaps the active mode for the conversation referenced by (channelID,
// senderID). Persists the new mode to conv.Metadata["daimon/mode"] via
// SaveConversation, then refreshes the agent cache under modeMu.Lock.
//
// Persist-first invariant (AD-4): on any error (validation, mid-turn check, or
// SaveConversation failure), both conv.Metadata and a.currentMode remain
// unchanged. The cache is only updated AFTER a successful persistence.
//
// Returns:
//   - ErrInvalidMode  — name is not "plan", "build", or "review"
//   - ErrTurnInProgress — a.cancels.Size() > 0 (turn is running; retry later)
//   - wrapped store error — SaveConversation or LoadConversation failed
//
// Lock ordering (AD-2): cancels.mu (inside Size()) → [validation off-lock] →
// [persistence off-lock] → modeMu.Lock (cache refresh). No nesting with providerMu.
func (a *Agent) SetMode(ctx context.Context, channelID, senderID, name string) error {
	// Step 1: Validate name (fast, no locks).
	def, err := LookupMode(name)
	if err != nil {
		return err // wraps ErrInvalidMode
	}

	// Step 2: Reject if any turn is in-flight (REQ-10, mirror SetProvider).
	// cancels.Size() takes cancels.mu internally and releases it.
	// Checked BEFORE acquiring modeMu to avoid lock-ordering inversion.
	if a.cancels.Size() > 0 {
		return ErrTurnInProgress
	}

	// Step 3: Resolve target conv, load (or create fresh), mutate metadata, persist.
	convID := a.effectiveConvID(channelID, senderID)
	conv, err := a.store.LoadConversation(ctx, convID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("set_mode: load conv: %w", err)
		}
		// Conv not found: create a fresh one with the new mode set.
		conv = &store.Conversation{
			ID:        convID,
			ChannelID: channelID,
			CreatedAt: time.Now(),
			Metadata:  map[string]string{},
		}
	}
	if conv.Metadata == nil {
		conv.Metadata = map[string]string{}
	}
	conv.Metadata["daimon/mode"] = def.Name
	conv.UpdatedAt = time.Now()

	if err := a.store.SaveConversation(ctx, *conv); err != nil {
		return fmt.Errorf("set_mode: save conv: %w", err)
	}

	// Step 4: Refresh agent cache under Lock (only after persistence succeeds).
	// This is the persist-first invariant: if we reach this line, the store has
	// the new mode. Any failure above returns early without touching the cache.
	a.modeMu.Lock()
	a.currentMode = def.Name
	a.modeMu.Unlock()

	return nil
}

// ---------------------------------------------------------------------------
// loadMode helper (AD-8, O-2)
// ---------------------------------------------------------------------------

// loadMode reconciles a.currentMode with conv.Metadata["daimon/mode"]. Called
// at processMessage start so the per-turn snapshot reflects the conversation's
// stored mode (which may have been set by SetMode or by a previous process
// session via persistence).
//
// Defaults to "build" on any of:
//   - conv is nil
//   - conv.Metadata is nil
//   - key "daimon/mode" is absent
//   - value is empty string
//   - value is an unrecognized mode name (logs warning; silent recovery)
//
// Read-only (O-2 confirmed): does NOT write back to conv.Metadata. The explicit
// initialization at conv-create time (loop.go AD-9) handles writing "build" for
// new conversations; this helper only updates the cache field a.currentMode.
func (a *Agent) loadMode(conv *store.Conversation) {
	name := "build" // safe default (REQ-5)

	if conv != nil && conv.Metadata != nil {
		if v, ok := conv.Metadata["daimon/mode"]; ok && v != "" {
			if _, err := LookupMode(v); err == nil {
				name = v
			} else {
				slog.Warn("load_mode: ignoring unknown mode in conv metadata",
					"conv_id", conv.ID, "mode", v)
			}
		}
	}

	a.modeMu.Lock()
	a.currentMode = name
	a.modeMu.Unlock()
}
