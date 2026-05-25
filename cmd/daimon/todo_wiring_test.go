package main

// todo_wiring_test.go — source-scan tests for wireTodo (task 3.6).
//
// Mirrors the rag_wiring_test.go approach: instead of constructing expensive
// integration mocks, scan the source files for the presence of required calls.
// This guards against the "dual startup path" class of bug (wireTodo missing
// from one of the two entrypoints) documented in PR3 risks.

import (
	"os"
	"strings"
	"testing"
)

// TestWireTodo_RegistersAllThreeTools verifies that todo_wiring.go calls
// BuildTodoTools and registers into the toolsRegistry with first-writer-wins
// guard (mirrors wireSmartMemory pattern).
func TestWireTodo_RegistersAllThreeTools(t *testing.T) {
	src, err := os.ReadFile("todo_wiring.go")
	if err != nil {
		t.Fatalf("read todo_wiring.go: %v", err)
	}
	content := string(src)

	required := []struct {
		call   string
		reason string
	}{
		{"BuildTodoTools(", "constructs the three todo tools from deps"},
		{"TodoToolDeps()", "builds callback deps from the agent"},
		{"exists", "first-writer-wins guard must check for pre-existing entries before registering"},
	}
	for _, req := range required {
		if !strings.Contains(content, req.call) {
			t.Errorf("todo_wiring.go must contain %q to %s", req.call, req.reason)
		}
	}
}

// TestWireTodo_BothStartupPathsCallWireTodo guards the known dual-path risk:
// wireTodo must be called from BOTH main.go (CLI path) and web_cmd.go (web
// server path). Missing one means the tool works in one mode but not the other.
func TestWireTodo_BothStartupPathsCallWireTodo(t *testing.T) {
	for _, path := range []string{"main.go", "web_cmd.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(src), "wireTodo(") {
			t.Errorf("%s must call wireTodo — dual startup paths must stay in sync", path)
		}
	}
}
