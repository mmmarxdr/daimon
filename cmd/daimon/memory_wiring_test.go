package main

// memory_wiring_test.go — source-scan guard for the MemoryToolDeps.Bus fix.
//
// The confirmed production bug (ADR-5): wireSmartMemory built MemoryToolDeps
// without setting .Bus, so saveMemoryTool never emitted EventMemoryChanged.
// This test scans the source to ensure .Bus is wired from the agent's bus.
//
// Mirrors the source-scan approach used by todo_wiring_test.go and
// rag_wiring_test.go — lightweight, no expensive integration mocks needed.

import (
	"os"
	"strings"
	"testing"
)

// TestWireSmartMemory_BusWired verifies that memory_wiring.go sets MemoryToolDeps.Bus
// so that saveMemoryTool emits EventMemoryChanged in production.
func TestWireSmartMemory_BusWired(t *testing.T) {
	src, err := os.ReadFile("memory_wiring.go")
	if err != nil {
		t.Fatalf("read memory_wiring.go: %v", err)
	}
	content := string(src)

	required := []struct {
		needle string
		reason string
	}{
		{
			"MemoryToolDeps{Store: st, Bus:",
			"MemoryToolDeps.Bus must be set so saveMemoryTool emits EventMemoryChanged",
		},
		{
			"SubagentBus()",
			"Bus must come from the agent's SubagentBus() accessor (same bus as WithBus)",
		},
	}

	for _, req := range required {
		if !strings.Contains(content, req.needle) {
			t.Errorf("memory_wiring.go missing %q — %s", req.needle, req.reason)
		}
	}
}
