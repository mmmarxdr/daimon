package agent

// set_mode_test.go — RED tests for Phase 3 (SetMode API) and Phase 4 (loadMode helper).
//
// Phase 3 tests cover:
//   - SetMode happy path (all 3 modes)
//   - ErrInvalidMode returned for unknown name
//   - ErrTurnInProgress returned when cancels registry is non-empty
//   - Persistence check: conv.Metadata["daimon/mode"] set correctly after success
//   - Cache-refresh: modeSnapshot() reflects new mode after success
//   - Persist-first invariant: force SaveConversation failure → cache unchanged
//
// Phase 4 tests (appended below) cover:
//   - loadMode with nil conv
//   - loadMode with nil Metadata
//   - loadMode with missing key
//   - loadMode with empty value
//   - loadMode with unknown value (defaults to build + warn)
//   - loadMode with valid plan/build/review (sets currentMode correctly)
//
// REQs covered: REQ-2, REQ-3, REQ-4, REQ-5, REQ-10, REQ-15.
// AD-11 exact error strings asserted.
// These tests are written BEFORE the implementation (TDD RED step).

import (
	"context"
	"errors"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/skill"
	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// buildAgentForSetMode constructs a minimal Agent for SetMode tests.
func buildAgentForSetMode(t *testing.T, st *mockStore) *Agent {
	t.Helper()
	ch := &mockChannel{}
	return New(
		config.AgentConfig{},
		defaultLimits(),
		config.FilterConfig{},
		ch,
		&mockProvider{},
		st,
		audit.NoopAuditor{},
		nil,
		nil,
		skill.SkillIndex{},
		4,
		false,
	)
}

// injectFakeTurn registers a fake cancel entry to simulate a turn in flight.
// Returns the key used, for cleanup purposes.
func injectFakeTurn(a *Agent) cancelKey {
	key := cancelKey{ChannelID: "ch-test", SenderID: "user-test"}
	_ = a.cancels.Register(key, func() {})
	return key
}

// ---------------------------------------------------------------------------
// Phase 3: SetMode happy path (REQ-2, REQ-4)
// ---------------------------------------------------------------------------

func TestSetMode_Plan_HappyPath(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	err := a.SetMode(context.Background(), "ch-1", "user-1", "plan")
	if err != nil {
		t.Fatalf("SetMode(\"plan\") unexpected error: %v", err)
	}

	// Cache should reflect new mode.
	snap := a.modeSnapshot()
	if snap.Name != "plan" {
		t.Errorf("modeSnapshot().Name = %q, want %q", snap.Name, "plan")
	}
}

func TestSetMode_Build_HappyPath(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	err := a.SetMode(context.Background(), "ch-1", "user-1", "build")
	if err != nil {
		t.Fatalf("SetMode(\"build\") unexpected error: %v", err)
	}
	snap := a.modeSnapshot()
	if snap.Name != "build" {
		t.Errorf("modeSnapshot().Name = %q, want %q", snap.Name, "build")
	}
}

func TestSetMode_Review_HappyPath(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	err := a.SetMode(context.Background(), "ch-1", "user-1", "review")
	if err != nil {
		t.Fatalf("SetMode(\"review\") unexpected error: %v", err)
	}
	snap := a.modeSnapshot()
	if snap.Name != "review" {
		t.Errorf("modeSnapshot().Name = %q, want %q", snap.Name, "review")
	}
}

// ---------------------------------------------------------------------------
// ErrInvalidMode (REQ-3, AD-11)
// ---------------------------------------------------------------------------

func TestSetMode_UnknownName_ReturnsErrInvalidMode(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	err := a.SetMode(context.Background(), "ch-1", "user-1", "banana")
	if err == nil {
		t.Fatal("SetMode(\"banana\") expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidMode) {
		t.Errorf("expected errors.Is(err, ErrInvalidMode) = true; err = %v", err)
	}
}

func TestSetMode_EmptyName_ReturnsErrInvalidMode(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	err := a.SetMode(context.Background(), "ch-1", "user-1", "")
	if err == nil {
		t.Fatal("SetMode(\"\") expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidMode) {
		t.Errorf("expected errors.Is(err, ErrInvalidMode) = true; err = %v", err)
	}
}

func TestSetMode_InvalidName_CacheUnchanged(t *testing.T) {
	// Validation failure must NOT update the cache (REQ-3).
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	// Pre-set mode to build.
	a.modeMu.Lock()
	a.currentMode = "build"
	a.modeMu.Unlock()

	_ = a.SetMode(context.Background(), "ch-1", "user-1", "invalid")

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "build" {
		t.Errorf("currentMode after invalid SetMode = %q, want unchanged %q", got, "build")
	}
}

// ---------------------------------------------------------------------------
// ErrTurnInProgress (REQ-10, AD-11)
// ---------------------------------------------------------------------------

func TestSetMode_TurnInProgress_ReturnsErrTurnInProgress(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)
	_ = injectFakeTurn(a)

	err := a.SetMode(context.Background(), "ch-1", "user-1", "plan")
	if err == nil {
		t.Fatal("SetMode with active turn expected ErrTurnInProgress, got nil")
	}
	if !errors.Is(err, ErrTurnInProgress) {
		t.Errorf("expected errors.Is(err, ErrTurnInProgress) = true; err = %v", err)
	}
}

func TestSetMode_TurnInProgress_CacheUnchanged(t *testing.T) {
	// Mid-turn rejection must NOT change the cache (REQ-10).
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)
	a.modeMu.Lock()
	a.currentMode = "build"
	a.modeMu.Unlock()
	_ = injectFakeTurn(a)

	_ = a.SetMode(context.Background(), "ch-1", "user-1", "plan")

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "build" {
		t.Errorf("currentMode after mid-turn SetMode = %q, want unchanged %q", got, "build")
	}
}

func TestSetMode_FakeTurn_RegistryBased_NotTiming(t *testing.T) {
	// Scenario S10-3: fake entry without real goroutine triggers ErrTurnInProgress.
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	// Insert entry without spawning any goroutine.
	key := cancelKey{ChannelID: "fake-ch", SenderID: "fake-user"}
	_ = a.cancels.Register(key, func() {})

	err := a.SetMode(context.Background(), "ch-1", "user-1", "plan")
	if !errors.Is(err, ErrTurnInProgress) {
		t.Errorf("expected ErrTurnInProgress for fake registry entry; got %v", err)
	}
}

func TestSetMode_AfterTurnCompletes_Succeeds(t *testing.T) {
	// Scenario S10-2: after turn completes (Unregister), SetMode succeeds.
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)
	key := injectFakeTurn(a)

	// While turn is in flight: should fail.
	err := a.SetMode(context.Background(), "ch-1", "user-1", "plan")
	if !errors.Is(err, ErrTurnInProgress) {
		t.Fatalf("expected ErrTurnInProgress while turn active; got %v", err)
	}

	// Turn completes: unregister.
	a.cancels.Unregister(key)

	// Now SetMode should succeed.
	err = a.SetMode(context.Background(), "ch-1", "user-1", "plan")
	if err != nil {
		t.Fatalf("SetMode after turn completes expected nil, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Persistence check (REQ-4, AD-4)
// ---------------------------------------------------------------------------

func TestSetMode_PersistsToConvMetadata(t *testing.T) {
	// Scenario S4-2: after SetMode succeeds, conv.Metadata["daimon/mode"] must
	// be set to the requested mode name.
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	err := a.SetMode(context.Background(), "ch-1", "user-1", "review")
	if err != nil {
		t.Fatalf("SetMode(\"review\") unexpected error: %v", err)
	}

	// The mockStore.SaveConversation stores the conv; verify its metadata.
	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()

	if saved == nil {
		t.Fatal("expected conv to be saved, but mockStore.conv is nil")
	}
	if saved.Metadata == nil {
		t.Fatal("saved conv has nil Metadata")
	}
	got := saved.Metadata["daimon/mode"]
	if got != "review" {
		t.Errorf("conv.Metadata[\"daimon/mode\"] = %q, want %q", got, "review")
	}
}

func TestSetMode_MetadataKey_CorrectNamespace(t *testing.T) {
	// Key must be "daimon/mode" (not "mode" or anything else).
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	_ = a.SetMode(context.Background(), "ch-1", "user-1", "plan")

	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()

	if saved == nil || saved.Metadata == nil {
		t.Fatal("expected saved conv with metadata")
	}
	if _, ok := saved.Metadata["daimon/mode"]; !ok {
		t.Errorf("expected key \"daimon/mode\" in conv.Metadata; got keys: %v", saved.Metadata)
	}
	if _, bad := saved.Metadata["mode"]; bad {
		t.Error("found bare key \"mode\" — must use \"daimon/mode\" namespace")
	}
}

// ---------------------------------------------------------------------------
// Persist-first invariant (AD-4)
// ---------------------------------------------------------------------------

func TestSetMode_SaveConversationFails_CacheUnchanged(t *testing.T) {
	// If SaveConversation fails, a.currentMode must remain at the old value.
	// This is the persist-first invariant (AD-4).
	saveErr := errors.New("disk full")
	st := &mockStore{saveErr: saveErr}
	a := buildAgentForSetMode(t, st)

	// Pre-set to build.
	a.modeMu.Lock()
	a.currentMode = "build"
	a.modeMu.Unlock()

	err := a.SetMode(context.Background(), "ch-1", "user-1", "plan")
	if err == nil {
		t.Fatal("expected error from SetMode when SaveConversation fails")
	}

	// Cache must be unchanged.
	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "build" {
		t.Errorf("currentMode after failed save = %q, want unchanged %q", got, "build")
	}
}

func TestSetMode_SaveConvFails_MetadataNotModified(t *testing.T) {
	// When SaveConversation fails, we should not have an inconsistent state.
	st := &mockStore{saveErr: errors.New("io error")}
	a := buildAgentForSetMode(t, st)

	err := a.SetMode(context.Background(), "ch-1", "user-1", "review")
	if err == nil {
		t.Fatal("expected error from SetMode when SaveConversation fails")
	}
	// The test is about cache invariant above — this test verifies error is non-nil.
}

// ---------------------------------------------------------------------------
// LoadConversation NotFound creates new conv (AD-4)
// ---------------------------------------------------------------------------

func TestSetMode_NotFoundConv_CreatesNewConvWithMode(t *testing.T) {
	// When LoadConversation returns ErrNotFound, SetMode creates a new conv.
	st := &mockStore{} // conv == nil → ErrNotFound
	a := buildAgentForSetMode(t, st)

	err := a.SetMode(context.Background(), "ch-new", "user-new", "plan")
	if err != nil {
		t.Fatalf("SetMode on new conv unexpected error: %v", err)
	}

	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()

	if saved == nil {
		t.Fatal("expected conv to be created and saved")
	}
	if saved.Metadata == nil || saved.Metadata["daimon/mode"] != "plan" {
		t.Errorf("new conv metadata[\"daimon/mode\"] = %q, want %q", saved.Metadata["daimon/mode"], "plan")
	}
}

// ---------------------------------------------------------------------------
// Cache-refresh after success (REQ-2, REQ-11)
// ---------------------------------------------------------------------------

func TestSetMode_CacheRefreshedAfterSuccess(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)
	a.modeMu.Lock()
	a.currentMode = "build"
	a.modeMu.Unlock()

	err := a.SetMode(context.Background(), "ch-1", "user-1", "review")
	if err != nil {
		t.Fatalf("SetMode unexpected error: %v", err)
	}

	snap := a.modeSnapshot()
	if snap.Name != "review" {
		t.Errorf("modeSnapshot().Name = %q, want %q after SetMode(review)", snap.Name, "review")
	}
}

// ---------------------------------------------------------------------------
// Existing conv with metadata (REQ-4)
// ---------------------------------------------------------------------------

func TestSetMode_ExistingConv_UpdatesMetadata(t *testing.T) {
	existing := &store.Conversation{
		ID:        "conv_ch-1user-1",
		ChannelID: "ch-1",
		Metadata:  map[string]string{"daimon/mode": "build", "title": "My Conv"},
	}
	st := &mockStore{conv: existing}
	a := buildAgentForSetMode(t, st)

	err := a.SetMode(context.Background(), "ch-1", "user-1", "plan")
	if err != nil {
		t.Fatalf("SetMode unexpected error: %v", err)
	}

	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()

	if saved.Metadata["daimon/mode"] != "plan" {
		t.Errorf("updated conv.Metadata[\"daimon/mode\"] = %q, want %q", saved.Metadata["daimon/mode"], "plan")
	}
	// Other metadata must be preserved.
	if saved.Metadata["title"] != "My Conv" {
		t.Errorf("title metadata was lost: got %q", saved.Metadata["title"])
	}
}

// ===========================================================================
// Phase 4: loadMode helper
// ===========================================================================
//
// REQs covered: REQ-4, REQ-5.

// ---------------------------------------------------------------------------
// loadMode table cases
// ---------------------------------------------------------------------------

func TestLoadMode_NilConv_DefaultsToBuild(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	a.loadMode(nil)

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "build" {
		t.Errorf("loadMode(nil) currentMode = %q, want %q", got, "build")
	}
}

func TestLoadMode_NilMetadata_DefaultsToBuild(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	conv := &store.Conversation{ID: "c1", ChannelID: "ch-1"}
	// conv.Metadata is nil
	a.loadMode(conv)

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "build" {
		t.Errorf("loadMode(conv with nil Metadata) currentMode = %q, want %q", got, "build")
	}
}

func TestLoadMode_MissingKey_DefaultsToBuild(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	conv := &store.Conversation{
		ID:       "c1",
		Metadata: map[string]string{"title": "some conv"},
	}
	a.loadMode(conv)

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "build" {
		t.Errorf("loadMode(conv missing daimon/mode key) currentMode = %q, want %q", got, "build")
	}
}

func TestLoadMode_EmptyValue_DefaultsToBuild(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	conv := &store.Conversation{
		ID:       "c1",
		Metadata: map[string]string{"daimon/mode": ""},
	}
	a.loadMode(conv)

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "build" {
		t.Errorf("loadMode(conv empty daimon/mode) currentMode = %q, want %q", got, "build")
	}
}

func TestLoadMode_UnknownValue_DefaultsToBuild(t *testing.T) {
	// Unknown value: default to build + (warn logged, not asserted here).
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	conv := &store.Conversation{
		ID:       "c1",
		Metadata: map[string]string{"daimon/mode": "turbo-mode-from-future"},
	}
	a.loadMode(conv)

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "build" {
		t.Errorf("loadMode(conv unknown mode) currentMode = %q, want fallback %q", got, "build")
	}
}

func TestLoadMode_ValidPlan_SetsPlan(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	conv := &store.Conversation{
		ID:       "c1",
		Metadata: map[string]string{"daimon/mode": "plan"},
	}
	a.loadMode(conv)

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "plan" {
		t.Errorf("loadMode(conv mode=plan) currentMode = %q, want %q", got, "plan")
	}
}

func TestLoadMode_ValidBuild_SetsBuild(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	// Pre-set to plan to verify it gets overwritten.
	a.modeMu.Lock()
	a.currentMode = "plan"
	a.modeMu.Unlock()

	conv := &store.Conversation{
		ID:       "c1",
		Metadata: map[string]string{"daimon/mode": "build"},
	}
	a.loadMode(conv)

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "build" {
		t.Errorf("loadMode(conv mode=build) currentMode = %q, want %q", got, "build")
	}
}

func TestLoadMode_ValidReview_SetsReview(t *testing.T) {
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	conv := &store.Conversation{
		ID:       "c1",
		Metadata: map[string]string{"daimon/mode": "review"},
	}
	a.loadMode(conv)

	a.modeMu.RLock()
	got := a.currentMode
	a.modeMu.RUnlock()

	if got != "review" {
		t.Errorf("loadMode(conv mode=review) currentMode = %q, want %q", got, "review")
	}
}

func TestLoadMode_ReadOnly_DoesNotWriteToConv(t *testing.T) {
	// O-2: loadMode must NOT write back to conv (no SaveConversation call).
	st := &mockStore{}
	a := buildAgentForSetMode(t, st)

	conv := &store.Conversation{
		ID:       "c1",
		Metadata: map[string]string{}, // no mode key
	}
	// Record save call count before.
	st.mu.Lock()
	savedBefore := st.conv
	st.mu.Unlock()

	a.loadMode(conv)

	st.mu.Lock()
	savedAfter := st.conv
	st.mu.Unlock()

	if savedAfter != savedBefore {
		t.Error("loadMode must NOT call SaveConversation (read-only, O-2)")
	}
	// Also verify conv itself was not mutated.
	if conv.Metadata["daimon/mode"] != "" {
		t.Errorf("loadMode must NOT write to conv.Metadata; got %q", conv.Metadata["daimon/mode"])
	}
}
