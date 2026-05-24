package agent

import (
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// WU5 RED tests: cwdOverrides struct (REQ-5)
// ---------------------------------------------------------------------------

func TestCwdOverrides_Get_NoOverride_ReturnsFalse(t *testing.T) {
	co := newCwdOverrides()
	k := cancelKey{ChannelID: "chan:1", SenderID: "user:1"}

	v, ok := co.Get(k)
	if ok {
		t.Errorf("expected Get to return false when no override set, got true with value %q", v)
	}
	if v != "" {
		t.Errorf("expected empty value when no override, got %q", v)
	}
}

func TestCwdOverrides_Set_And_Get_ReturnsValue(t *testing.T) {
	co := newCwdOverrides()
	k := cancelKey{ChannelID: "chan:1", SenderID: "user:1"}

	if err := co.Set(k, "/tmp/mydir"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	v, ok := co.Get(k)
	if !ok {
		t.Fatal("expected Get to return true after Set")
	}
	if v != "/tmp/mydir" {
		t.Errorf("expected /tmp/mydir, got %q", v)
	}
}

func TestCwdOverrides_Reset_Removes_Override(t *testing.T) {
	co := newCwdOverrides()
	k := cancelKey{ChannelID: "chan:1", SenderID: "user:1"}

	if err := co.Set(k, "/tmp/mydir"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	co.Reset(k)

	_, ok := co.Get(k)
	if ok {
		t.Error("expected Get to return false after Reset")
	}
}

func TestCwdOverrides_EffectiveCwd_WithOverride_ReturnsOverride(t *testing.T) {
	co := newCwdOverrides()
	k := cancelKey{ChannelID: "chan:1", SenderID: "user:1"}

	if err := co.Set(k, "/tmp/override"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got := co.EffectiveCwd("chan:1", "user:1", "/default")
	if got != "/tmp/override" {
		t.Errorf("expected /tmp/override, got %q", got)
	}
}

func TestCwdOverrides_EffectiveCwd_NoOverride_ReturnsDefault(t *testing.T) {
	co := newCwdOverrides()

	got := co.EffectiveCwd("chan:1", "user:1", "/my/default")
	if got != "/my/default" {
		t.Errorf("expected /my/default, got %q", got)
	}
}

func TestCwdOverrides_ConcurrentSetGet_NoRace(t *testing.T) {
	co := newCwdOverrides()
	k := cancelKey{ChannelID: "chan:race", SenderID: "user:race"}

	var wg sync.WaitGroup
	const N = 50

	// Writers
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			_ = co.Set(k, "/tmp/path")
		}(i)
	}

	// Readers
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = co.Get(k)
		}()
	}

	// Resetters
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			co.Reset(k)
		}()
	}

	wg.Wait()
}

func TestCwdOverrides_PerUserIsolation(t *testing.T) {
	co := newCwdOverrides()
	k1 := cancelKey{ChannelID: "chan:1", SenderID: "alice"}
	k2 := cancelKey{ChannelID: "chan:1", SenderID: "bob"}

	if err := co.Set(k1, "/tmp/alice"); err != nil {
		t.Fatalf("Set alice: %v", err)
	}
	if err := co.Set(k2, "/tmp/bob"); err != nil {
		t.Fatalf("Set bob: %v", err)
	}

	v1, ok1 := co.Get(k1)
	v2, ok2 := co.Get(k2)

	if !ok1 || v1 != "/tmp/alice" {
		t.Errorf("alice: expected /tmp/alice (ok=true), got %q (ok=%v)", v1, ok1)
	}
	if !ok2 || v2 != "/tmp/bob" {
		t.Errorf("bob: expected /tmp/bob (ok=true), got %q (ok=%v)", v2, ok2)
	}

	// Reset alice — bob unaffected.
	co.Reset(k1)
	_, stillAlice := co.Get(k1)
	if stillAlice {
		t.Error("expected alice's override to be removed after Reset")
	}
	v2after, ok2after := co.Get(k2)
	if !ok2after || v2after != "/tmp/bob" {
		t.Errorf("bob should still have /tmp/bob after alice reset, got %q (ok=%v)", v2after, ok2after)
	}
}
