package agent

import (
	"fmt"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// WU1: CommandRegistry thread-safety + RegisterIfFree + source tracking
// ---------------------------------------------------------------------------

func TestCommandRegistry_RegisterIfFree_FreeName_RegistersAndReturnsTrue(t *testing.T) {
	reg := NewCommandRegistry()
	called := false
	ok, err := reg.RegisterIfFree("myskill", "does something", func(cc CommandContext) error {
		called = true
		return nil
	}, SourceSkill)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected RegisterIfFree to return true for a free name")
	}

	h, found := reg.Lookup("myskill")
	if !found {
		t.Fatal("expected Lookup to find 'myskill' after RegisterIfFree")
	}
	if err := h(CommandContext{}); err != nil {
		t.Fatalf("unexpected handler error: %v", err)
	}
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestCommandRegistry_RegisterIfFree_TakenName_DoesNotOverwrite_ReturnsFalse(t *testing.T) {
	reg := NewCommandRegistry()
	original := func(cc CommandContext) error { return fmt.Errorf("original") }
	shadow := func(cc CommandContext) error { return fmt.Errorf("shadow") }

	reg.Register("help", "builtin help", original, SourceBuiltin)

	ok, err := reg.RegisterIfFree("help", "shadow", shadow, SourceSkill)
	if err != nil {
		t.Fatalf("unexpected error on collision: %v", err)
	}
	if ok {
		t.Error("expected RegisterIfFree to return false when name is taken")
	}

	// Verify original handler still in place.
	h, found := reg.Lookup("help")
	if !found {
		t.Fatal("expected 'help' to still be registered")
	}
	if err := h(CommandContext{}); err == nil || err.Error() != "original" {
		t.Errorf("expected original handler to still be registered, got error: %v", err)
	}
}

func TestCommandRegistry_RegisterIfFree_ConcurrentCalls_NoRace(t *testing.T) {
	reg := NewCommandRegistry()
	const workers = 20
	results := make([]bool, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		go func(i int) {
			defer wg.Done()
			ok, _ := reg.RegisterIfFree("shared", "desc", func(cc CommandContext) error { return nil }, SourceSkill)
			results[i] = ok
		}(i)
	}
	wg.Wait()

	// Exactly one registration should succeed.
	count := 0
	for _, ok := range results {
		if ok {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 successful registration, got %d", count)
	}
}

func TestCommandRegistry_UnregisterAllBySource_RemovesOnlyMatchingSource(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register("ping", "builtin", func(cc CommandContext) error { return nil }, SourceBuiltin)
	reg.Register("tasks", "cron", func(cc CommandContext) error { return nil }, SourceCron)
	_, _ = reg.RegisterIfFree("researcher", "skill", func(cc CommandContext) error { return nil }, SourceSkill)
	_, _ = reg.RegisterIfFree("coder", "skill", func(cc CommandContext) error { return nil }, SourceSkill)

	n := reg.UnregisterAllBySource(SourceSkill)
	if n != 2 {
		t.Errorf("expected 2 entries removed, got %d", n)
	}

	// Builtin and cron should still be present.
	if _, ok := reg.Lookup("ping"); !ok {
		t.Error("expected 'ping' (builtin) to still be registered")
	}
	if _, ok := reg.Lookup("tasks"); !ok {
		t.Error("expected 'tasks' (cron) to still be registered")
	}
	// Skills should be gone.
	if _, ok := reg.Lookup("researcher"); ok {
		t.Error("expected 'researcher' (skill) to be unregistered")
	}
	if _, ok := reg.Lookup("coder"); ok {
		t.Error("expected 'coder' (skill) to be unregistered")
	}
}

func TestCommandRegistry_UnregisterAllBySource_NoOp_WhenNoMatches(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register("ping", "builtin", func(cc CommandContext) error { return nil }, SourceBuiltin)

	n := reg.UnregisterAllBySource(SourceSkill)
	if n != 0 {
		t.Errorf("expected 0 entries removed, got %d", n)
	}
	if _, ok := reg.Lookup("ping"); !ok {
		t.Error("expected 'ping' to still be registered after no-op unregister")
	}
}

func TestCommandRegistry_EntriesWithSource_IncludesSourceField(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register("ping", "builtin ping", func(cc CommandContext) error { return nil }, SourceBuiltin)
	reg.Register("tasks", "cron tasks", func(cc CommandContext) error { return nil }, SourceCron)
	_, _ = reg.RegisterIfFree("researcher", "skill researcher", func(cc CommandContext) error { return nil }, SourceSkill)

	entries := reg.EntriesWithSource()
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	byName := make(map[string]CommandEntryInfo)
	for _, e := range entries {
		byName[e.Name] = e
	}

	if byName["ping"].Source != SourceBuiltin {
		t.Errorf("expected ping source=%q, got %q", SourceBuiltin, byName["ping"].Source)
	}
	if byName["tasks"].Source != SourceCron {
		t.Errorf("expected tasks source=%q, got %q", SourceCron, byName["tasks"].Source)
	}
	if byName["researcher"].Source != SourceSkill {
		t.Errorf("expected researcher source=%q, got %q", SourceSkill, byName["researcher"].Source)
	}
	if byName["ping"].Desc != "builtin ping" {
		t.Errorf("expected ping desc=%q, got %q", "builtin ping", byName["ping"].Desc)
	}
}

func TestCommandRegistry_Lookup_ThreadSafe_UnderRace(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register("stable", "a stable command", func(cc CommandContext) error { return nil }, SourceBuiltin)

	var wg sync.WaitGroup
	const writers = 5
	const readers = 20

	// Writers register new commands concurrently.
	for i := range writers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("dynamic%d", i)
			_, _ = reg.RegisterIfFree(name, "dynamic", func(cc CommandContext) error { return nil }, SourceSkill)
		}(i)
	}

	// Readers look up the stable command concurrently.
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = reg.Lookup("stable")
		}()
	}

	wg.Wait()
}

func TestCommandRegistry_Source_Constants_Values(t *testing.T) {
	if SourceBuiltin != "builtin" {
		t.Errorf("SourceBuiltin must be %q, got %q", "builtin", SourceBuiltin)
	}
	if SourceCron != "cron" {
		t.Errorf("SourceCron must be %q, got %q", "cron", SourceCron)
	}
	if SourceSkill != "skill" {
		t.Errorf("SourceSkill must be %q, got %q", "skill", SourceSkill)
	}
}

// TestCommandRegistry_ExistingRegister_StillOverwrites confirms backward compatibility:
// Register() keeps silent-overwrite semantics (used only for builtins).
func TestCommandRegistry_ExistingRegister_StillOverwrites(t *testing.T) {
	reg := NewCommandRegistry()
	reg.Register("foo", "first", func(cc CommandContext) error { return fmt.Errorf("first") }, SourceBuiltin)
	reg.Register("foo", "second", func(cc CommandContext) error { return fmt.Errorf("second") }, SourceBuiltin)

	h, ok := reg.Lookup("foo")
	if !ok {
		t.Fatal("expected 'foo' to be registered")
	}
	err := h(CommandContext{})
	if err == nil || err.Error() != "second" {
		t.Errorf("expected second handler to win, got: %v", err)
	}
}
