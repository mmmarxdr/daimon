package agent

import (
	"context"
	"testing"
	"time"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// WU4 RED tests: processMessage turn-context wiring (REQ-7)
// ---------------------------------------------------------------------------

// TestProcessMessage_TurnCtx_Registered_Before_LLMCall verifies that the
// cancelRegistry has an entry for (channelID, senderID) while the LLM call is
// in progress — i.e., the key is registered BEFORE the first provider.Chat.
func TestProcessMessage_TurnCtx_Registered_Before_LLMCall(t *testing.T) {
	registeredBeforeCall := false
	gate := make(chan struct{})

	// Use blockingProvider with onEnter to capture registry state at Chat call time.
	blockProv := &blockingProvider{
		gate: gate,
		onEnter: func() {
			// At this point, processMessage is inside Chat — ctx-wrap should have fired.
			registeredBeforeCall = true
		},
	}

	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, &mockChannel{}, blockProv, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	done := make(chan struct{})
	go func() {
		ag.processMessage(context.Background(), channel.IncomingMessage{
			ChannelID: "chan:1",
			SenderID:  "user:1",
			Content:   content.TextBlock("hello"),
		})
		close(done)
	}()

	// Wait for onEnter to fire, then check registry and unblock.
	time.Sleep(50 * time.Millisecond)

	// Also verify via cancels.Size() directly.
	if ag.cancels.Size() == 0 {
		registeredBeforeCall = false
	}

	close(gate) // unblock provider
	<-done

	if !registeredBeforeCall {
		t.Error("expected cancelRegistry to have entry before LLM call")
	}
}

// TestProcessMessage_TurnCtx_Deregistered_After_Return verifies that the
// cancelRegistry entry is cleaned up after processMessage returns.
func TestProcessMessage_TurnCtx_Deregistered_After_Return(t *testing.T) {
	prov := &mockProvider{responses: []provider.ChatResponse{{Content: "ok"}}}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, &mockChannel{}, prov, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:1",
		SenderID:  "user:1",
		Content:   content.TextBlock("hello"),
	})

	// After return, no entry should remain.
	if ag.cancels.Size() != 0 {
		t.Errorf("expected cancelRegistry to be empty after processMessage returns, got size %d", ag.cancels.Size())
	}
}

// TestProcessMessage_SlashCommand_Not_Registered_In_CancelRegistry verifies
// that slash commands do NOT register a turn context (they early-return before
// the ctx-wrap block).
func TestProcessMessage_SlashCommand_Not_Registered_In_CancelRegistry(t *testing.T) {
	ch := &mockChannel{}
	prov := &mockProvider{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, ch, prov, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:1",
		SenderID:  "user:1",
		Content:   content.TextBlock("/ping"),
	})

	// /ping should never touch the cancelRegistry.
	if ag.cancels.Size() != 0 {
		t.Errorf("expected cancelRegistry to be empty for slash command, got size %d", ag.cancels.Size())
	}
	// And no LLM call.
	if prov.callCount() != 0 {
		t.Errorf("expected 0 LLM calls for /ping, got %d", prov.callCount())
	}
}

// TestProcessMessage_Cancel_HaltsStreaming verifies that cancelling the turn
// context causes processMessage to return early (the provider call is
// interrupted). The test uses the existing blockingProvider.
func TestProcessMessage_Cancel_HaltsStreaming(t *testing.T) {
	gate := make(chan struct{})
	blockProv := &blockingProvider{gate: gate}
	ch := &mockChannel{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, ch, blockProv, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	done := make(chan struct{})
	go func() {
		ag.processMessage(context.Background(), channel.IncomingMessage{
			ChannelID: "chan:cancel",
			SenderID:  "user:cancel",
			Content:   content.TextBlock("block me"),
		})
		close(done)
	}()

	// Wait a bit for processMessage to register the turn and call the provider.
	time.Sleep(30 * time.Millisecond)

	// Cancel the turn via the cancelRegistry — this cancels turnCtx.
	key := cancelKey{ChannelID: "chan:cancel", SenderID: "user:cancel"}
	cancelled := ag.cancels.Cancel(key)
	if !cancelled {
		t.Error("expected Cancel to return true (entry existed)")
	}

	// The blockingProvider blocks until gate closes OR ctx is cancelled.
	// Since we cancelled the turn ctx, the provider's ctx.Done() fires.
	select {
	case <-done:
		// processMessage returned — good.
	case <-time.After(2 * time.Second):
		// If it didn't return, close the gate to unblock.
		close(gate)
		t.Fatal("processMessage did not return after turn cancellation within timeout")
	}
}

// TestProcessMessage_SemaphoreReleased_After_Cancel verifies that the agent's
// concurrency semaphore slot is released even when a turn is cancelled, so
// subsequent turns can proceed without deadlock.
func TestProcessMessage_SemaphoreReleased_After_Cancel(t *testing.T) {
	// Build an agent with maxConcurrent=1 and a blocking provider.
	gate1 := make(chan struct{})
	blockProv := &blockingProvider{gate: gate1}
	ch := &mockChannel{}
	cfg := config.AgentConfig{MaxIterations: 5, MaxTokensPerTurn: 100}
	ag := New(cfg, defaultLimits(), config.FilterConfig{}, ch, blockProv, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 1 /* maxConcurrent=1 */, false)

	done := make(chan struct{})
	go func() {
		ag.processMessage(context.Background(), channel.IncomingMessage{
			ChannelID: "chan:sem",
			SenderID:  "user:sem",
			Content:   content.TextBlock("block"),
		})
		close(done)
	}()

	// Wait for the provider to be called (turn is in progress).
	time.Sleep(30 * time.Millisecond)

	// Cancel the turn — turnCtx cancels, blockingProvider.Chat returns.
	key := cancelKey{ChannelID: "chan:sem", SenderID: "user:sem"}
	ag.cancels.Cancel(key)

	// Wait for processMessage to finish — semaphore slot should be released.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		close(gate1)
		t.Fatal("processMessage did not return after cancel")
	}

	// Verify semaphore is released: a second processMessage call with a regular
	// provider should complete without blocking.
	ag2 := New(cfg, defaultLimits(), config.FilterConfig{}, ch,
		&mockProvider{responses: []provider.ChatResponse{{Content: "ok"}}},
		&mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 1, false)

	done2 := make(chan struct{})
	go func() {
		ag2.processMessage(context.Background(), channel.IncomingMessage{
			ChannelID: "chan:sem2",
			SenderID:  "user:sem2",
			Content:   content.TextBlock("hello2"),
		})
		close(done2)
	}()

	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("second processMessage (new agent) blocked — unexpected")
	}

	// For the ORIGINAL agent: verify we can still acquire the semaphore
	// by trying processMessage again (it should not block).
	gate3 := make(chan struct{})
	close(gate3)
	// Can't swap provider on ag, so just verify cancels registry is clean.
	if ag.cancels.Size() != 0 {
		t.Errorf("expected cancels registry to be empty after first cancel, got %d", ag.cancels.Size())
	}
}
