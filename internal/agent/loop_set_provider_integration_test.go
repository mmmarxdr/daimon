package agent

// loop_set_provider_integration_test.go — integration test for the PR2 mid-turn
// safety contract (R-PR2-1 from tasks doc).
//
// Verifies that SetProvider called concurrently with a fake in-flight turn
// is correctly rejected via the cancel registry (not timing-based).
//
// Run with -race: go test -race ./internal/agent/...

import (
	"context"
	"errors"
	"sync"
	"testing"

	"daimon/internal/config"
)

// TestSetProvider_MidTurnIntegration_RaceClean injects a fake cancel entry
// (simulating an in-flight processMessage turn), then calls SetProvider from a
// second goroutine. The second goroutine MUST receive ErrTurnInProgress.
// No real Chat calls or goroutines needed — the registry-based check is
// synchronous.
func TestSetProvider_MidTurnIntegration_RaceClean(t *testing.T) {
	initial := newConfigurableProviderWithModels("claude-haiku-4-5", []string{"claude-sonnet-4-6"})
	a := buildAgentForSetProvider(t, initial, config.ProviderCredentials{})

	// Register a fake "in-flight turn" entry.
	turnKey := cancelKey{ChannelID: "integration-ch", SenderID: "integration-user"}
	if err := a.cancels.Register(turnKey, func() {}); err != nil {
		t.Fatalf("failed to register fake turn: %v", err)
	}

	var (
		swapErr error
		wg      sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		swapErr = a.SetProvider(context.Background(), "claude-sonnet-4-6")
	}()
	wg.Wait()

	if !errors.Is(swapErr, ErrTurnInProgress) {
		t.Errorf("expected ErrTurnInProgress from concurrent SetProvider, got %v", swapErr)
	}

	// Provider unchanged.
	if a.providerSnapshot().Model() != "claude-haiku-4-5" {
		t.Errorf("provider changed despite turn in flight")
	}

	// Clean up.
	a.cancels.Unregister(turnKey)

	// Now SetProvider should succeed.
	if err := a.SetProvider(context.Background(), "claude-sonnet-4-6"); err != nil {
		t.Errorf("SetProvider after turn end returned error: %v", err)
	}
}
