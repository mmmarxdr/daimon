package agent

// web_memory_scope_test.go — tests for the web-channel memory scope fix.
//
// Bug: scope is derived from convID (conv_web:<uuid8>), making it unique per
// conversation. Web conversations save memory under "web:aaaaaaaa" and a new
// chat searches under "web:bbbbbbbb" → miss. Fix: memScope collapses all
// web:<*> channelIDs to the constant "web".
//
// TDD protocol: RED on current code (single scope variable), GREEN after fix.

import (
	"context"
	"strings"
	"sync"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/skill"
	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// Unit test: webMemScope helper derivation
// ---------------------------------------------------------------------------

// TestWebMemScope_Derivation verifies the scope-to-memScope derivation rules:
//   - web:<uuid> channelID → memScope must be the constant "web"
//   - telegram:<userID> channelID → memScope must equal the per-user scope
//   - empty senderID (CLI/cron) → memScope must equal channelID
//
// On the current (unfixed) code this test FAILS because webMemScope does not
// exist and the derivation produces the per-connection "web:<uuid>" scope.
func TestWebMemScope_Derivation(t *testing.T) {
	cases := []struct {
		name         string
		channelID    string
		senderID     string
		wantMemScope string
	}{
		{
			name:         "web channel first uuid",
			channelID:    "web:aaaaaaaa",
			senderID:     "",
			wantMemScope: "web",
		},
		{
			name:         "web channel second uuid",
			channelID:    "web:bbbbbbbb",
			senderID:     "",
			wantMemScope: "web",
		},
		{
			name:         "telegram stable scope",
			channelID:    "telegram",
			senderID:     "12345678",
			wantMemScope: "telegram:12345678",
		},
		{
			name:         "cli no sender",
			channelID:    "cli",
			senderID:     "",
			wantMemScope: "cli",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := webMemScope(tc.channelID, tc.senderID)
			if got != tc.wantMemScope {
				t.Errorf("webMemScope(%q, %q) = %q; want %q",
					tc.channelID, tc.senderID, got, tc.wantMemScope)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Scope-tracking store mock
// ---------------------------------------------------------------------------

// scopeTrackingStore wraps mockStore and records the scopeID argument passed
// to every SearchMemory and AppendMemory call.
type scopeTrackingStore struct {
	mockStore
	mu           sync.Mutex
	searchScopes []string
	appendScopes []string
	// scopedMemories maps scopeID → entries; SearchMemory returns only those
	// whose ScopeID matches the given scope so cross-scope recall is testable.
	scopedMemories map[string][]store.MemoryEntry
}

func (s *scopeTrackingStore) SearchMemory(ctx context.Context, scopeID string, query string, limit int) ([]store.MemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.searchScopes = append(s.searchScopes, scopeID)
	if s.scopedMemories != nil {
		return s.scopedMemories[scopeID], nil
	}
	return s.mockStore.memories, nil
}

func (s *scopeTrackingStore) AppendMemory(ctx context.Context, scopeID string, entry store.MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendScopes = append(s.appendScopes, scopeID)
	if s.scopedMemories == nil {
		s.scopedMemories = map[string][]store.MemoryEntry{}
	}
	s.scopedMemories[scopeID] = append(s.scopedMemories[scopeID], entry)
	return nil
}

func (s *scopeTrackingStore) lastSearchScope() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.searchScopes) == 0 {
		return ""
	}
	return s.searchScopes[len(s.searchScopes)-1]
}

func (s *scopeTrackingStore) lastAppendScope() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.appendScopes) == 0 {
		return ""
	}
	return s.appendScopes[len(s.appendScopes)-1]
}

// ---------------------------------------------------------------------------
// Integration test: cross-conversation recall on the web channel
// ---------------------------------------------------------------------------

// TestWebChannel_CrossConversationMemoryRecall verifies that memory saved in
// web conversation A is visible when searching from web conversation B.
//
// Scenario:
//  1. Conv A (channelID="web:aaaaaaaa", convID="conv_web:aaaaaaaa") produces a
//     text response → memory appended under scope X.
//  2. Conv B (channelID="web:bbbbbbbb", convID="conv_web:bbbbbbbb") sends a
//     message → memory searched under scope Y.
//  3. PASS condition: X == Y == "web" (shared scope, cross-conv recall works).
//  4. FAIL condition (pre-fix): X="web:aaaaaaaa", Y="web:bbbbbbbb" → scopes
//     diverge and recall returns nothing.
func TestWebChannel_CrossConversationMemoryRecall(t *testing.T) {
	// Provider: turn 1 returns text to trigger AppendMemory; turn 2 is just a
	// plain response so we can observe the SearchMemory scope.
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "I love Go for backend work."}, // conv A turn
			{Content: "Response from conv B."},       // conv B turn
		},
	}

	ch := &mockChannel{}
	st := &scopeTrackingStore{}

	ag := New(
		config.AgentConfig{MaxIterations: 1, MaxTokensPerTurn: 100, MemoryResults: 5},
		defaultLimits(),
		config.FilterConfig{},
		ch, prov, st,
		audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	)

	ctx := context.Background()

	// --- Turn 1: web conversation A saves a memory ---------------------------
	ag.processMessage(ctx, channel.IncomingMessage{
		ChannelID:      "web:aaaaaaaa",
		SenderID:       "",
		ConversationID: "conv_web:aaaaaaaa",
		Content:        content.TextBlock("Tell me about Go"),
	})

	appendScope := st.lastAppendScope()
	if appendScope == "" {
		t.Fatal("no AppendMemory call recorded after conv A turn")
	}

	// --- Turn 2: web conversation B searches memory --------------------------
	ag.processMessage(ctx, channel.IncomingMessage{
		ChannelID:      "web:bbbbbbbb",
		SenderID:       "",
		ConversationID: "conv_web:bbbbbbbb",
		Content:        content.TextBlock("What do you think about Go?"),
	})

	searchScope := st.lastSearchScope()
	if searchScope == "" {
		t.Fatal("no SearchMemory call recorded after conv B turn")
	}

	// Core assertion: both scopes must be the same stable value ("web").
	// PRE-FIX (RED): appendScope="web:aaaaaaaa", searchScope="web:bbbbbbbb" → FAIL.
	// POST-FIX (GREEN): both == "web" → PASS.
	if appendScope != searchScope {
		t.Errorf("cross-conversation recall broken: appendScope=%q != searchScope=%q; "+
			"all web conversations must share the constant memScope \"web\"",
			appendScope, searchScope)
	}
	if !strings.HasPrefix(appendScope, "web") {
		t.Errorf("appendScope %q should start with \"web\"", appendScope)
	}
	// Exact value after fix must be the constant "web", not a UUID-bearing value.
	if appendScope != "web" {
		t.Errorf("appendScope = %q; want \"web\" (constant shared scope for all web conversations)", appendScope)
	}
}

// ---------------------------------------------------------------------------
// Regression guard: Telegram channel scope must be unaffected
// ---------------------------------------------------------------------------

// TestTelegramChannel_MemoryScopeUnchanged asserts that non-web channels still
// use the per-user scope derived from userScope(channelID, senderID).
func TestTelegramChannel_MemoryScopeUnchanged(t *testing.T) {
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "Hello from telegram."},
		},
	}

	ch := &mockChannel{}
	st := &scopeTrackingStore{}

	ag := New(
		config.AgentConfig{MaxIterations: 1, MaxTokensPerTurn: 100, MemoryResults: 5},
		defaultLimits(),
		config.FilterConfig{},
		ch, prov, st,
		audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "telegram",
		SenderID:  "12345678",
		Content:   content.TextBlock("Hi"),
	})

	appendScope := st.lastAppendScope()
	// telegram:12345678 is the expected per-user scope.
	wantScope := "telegram:12345678"
	if appendScope != wantScope {
		t.Errorf("telegram appendScope = %q; want %q (per-user scope must be unchanged)",
			appendScope, wantScope)
	}
}
