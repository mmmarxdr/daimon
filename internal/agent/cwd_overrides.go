package agent

import (
	"os"
	"sync"
)

// cwdOverrides stores per-(channel, sender) shell working-directory overrides.
// The key is a cancelKey (same shape as the turn-cancel registry, reused).
// Read-heavy (every shell invocation reads), write-rare (only /cd writes).
//
// Thread-safety: all public methods are safe for concurrent use via RWMutex.
type cwdOverrides struct {
	mu          sync.RWMutex
	overrides   map[cancelKey]string
	defaultCwd  string // fallback when no per-(channel,sender) override is set
	sandboxRoot string // if non-empty, /cd rejects paths outside this prefix
}

// newCwdOverrides returns an initialized cwdOverrides with empty defaults.
// Call WithDefaultCwd and/or WithSandboxRoot on the result when needed.
func newCwdOverrides() *cwdOverrides {
	return &cwdOverrides{
		overrides: make(map[cancelKey]string),
	}
}

// WithDefaultCwd sets the fallback cwd returned by DefaultCwd(). Returns co
// for fluent chaining.
func (co *cwdOverrides) WithDefaultCwd(dir string) *cwdOverrides {
	co.defaultCwd = dir
	return co
}

// WithSandboxRoot sets the sandbox boundary enforced by cmdCd. Returns co for
// fluent chaining.
func (co *cwdOverrides) WithSandboxRoot(root string) *cwdOverrides {
	co.sandboxRoot = root
	return co
}

// DefaultCwd returns the configured default working directory. Falls back to
// os.Getwd() when none was configured, and "." on error.
func (co *cwdOverrides) DefaultCwd() string {
	if co.defaultCwd != "" {
		return co.defaultCwd
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

// SandboxRoot returns the sandbox boundary root, or "" when no sandbox is set.
func (co *cwdOverrides) SandboxRoot() string {
	return co.sandboxRoot
}

// Get returns the override path for key and true, or ("", false) when none is set.
func (co *cwdOverrides) Get(k cancelKey) (string, bool) {
	co.mu.RLock()
	defer co.mu.RUnlock()
	v, ok := co.overrides[k]
	return v, ok
}

// Set stores path under key. Validation (sandbox checks, directory existence)
// is the caller's responsibility (/cd handler does it before calling Set).
func (co *cwdOverrides) Set(k cancelKey, path string) error {
	co.mu.Lock()
	defer co.mu.Unlock()
	co.overrides[k] = path
	return nil
}

// Reset removes any override for key, reverting to the default cwd.
func (co *cwdOverrides) Reset(k cancelKey) {
	co.mu.Lock()
	defer co.mu.Unlock()
	delete(co.overrides, k)
}

// EffectiveCwd returns the effective working directory for (channelID, senderID).
// If an override exists it is returned; otherwise defaultCwd is returned.
func (co *cwdOverrides) EffectiveCwd(channelID, senderID, defaultCwd string) string {
	k := cancelKey{ChannelID: channelID, SenderID: senderID}
	if v, ok := co.Get(k); ok {
		return v
	}
	return defaultCwd
}
