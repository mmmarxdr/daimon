package agent

import (
	"context"
	"testing"
	"time"

	"daimon/internal/provider"
	"daimon/internal/store"
)

// compactorMockStore implements compactorStoreAPI for accessor tests.
type compactorMockStore struct{}

func (s *compactorMockStore) LoadConversation(_ context.Context, _ string) (*store.Conversation, error) {
	return nil, store.ErrNotFound
}
func (s *compactorMockStore) SaveConversation(_ context.Context, _ store.Conversation) error {
	return nil
}
func (s *compactorMockStore) ListCompactableConversations(_ context.Context, _ time.Time, _ int) ([]string, error) {
	return nil, nil
}
func (s *compactorMockStore) DeleteToolOutputsForConversation(_ context.Context, _ string) (int, error) {
	return 0, nil
}

// compactorFullMockProvider implements provider.Provider for accessor tests.
// It also satisfies compactorProviderAPI (has Chat).
type compactorFullMockProvider struct {
	modelName string
	response  string
}

func (p *compactorFullMockProvider) Name() string                                  { return "mock-compactor" }
func (p *compactorFullMockProvider) Model() string                                 { return p.modelName }
func (p *compactorFullMockProvider) SupportsTools() bool                           { return false }
func (p *compactorFullMockProvider) SupportsMultimodal() bool                      { return false }
func (p *compactorFullMockProvider) SupportsAudio() bool                           { return false }
func (p *compactorFullMockProvider) HealthCheck(_ context.Context) (string, error) { return "ok", nil }
func (p *compactorFullMockProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	return &provider.ChatResponse{Content: p.response}, nil
}

// TestCompactor_NoChatFnField asserts REQ-D2/S8-2:
// The ConversationCompactor struct MUST hold chatFn func() compactorProviderAPI, NOT a bare provider compactorProviderAPI.
// This test accesses cc.chatFn to confirm the field exists and is callable.
func TestCompactor_NoChatFnField(t *testing.T) {
	prov := &compactorFullMockProvider{modelName: "m1", response: "summary text"}
	cfg := CompactorConfig{Enabled: true}
	st := &compactorMockStore{}

	var captured provider.Provider = prov
	fn := func() provider.Provider { return captured }

	cc := NewConversationCompactor(st, fn, cfg)
	if cc == nil {
		t.Fatal("NewConversationCompactor returned nil with Enabled=true")
	}

	// cc.chatFn must exist and return a non-nil compactorProviderAPI.
	got := cc.chatFn()
	if got == nil {
		t.Fatal("cc.chatFn() returned nil")
	}
	// Verify the returned API wraps the original provider by calling Chat.
	resp, err := got.Chat(context.Background(), provider.ChatRequest{})
	if err != nil {
		t.Fatalf("chatFn().Chat() returned unexpected error: %v", err)
	}
	if resp.Content != prov.response {
		t.Errorf("chatFn().Chat() content = %q, want %q", resp.Content, prov.response)
	}
}

// TestCompactor_SeesSwappedProvider asserts REQ-D2/S8-1:
// After the closure variable is updated, the next call to cc.chatFn() returns the new provider.
// This simulates what happens after SetProvider updates a.provider under providerMu.Lock.
func TestCompactor_SeesSwappedProvider(t *testing.T) {
	prov1 := &compactorFullMockProvider{modelName: "m1", response: "summary v1"}
	prov2 := &compactorFullMockProvider{modelName: "m2", response: "summary v2"}

	var current provider.Provider = prov1
	fn := func() provider.Provider { return current }

	st := &compactorMockStore{}
	cfg := CompactorConfig{Enabled: true}

	cc := NewConversationCompactor(st, fn, cfg)
	if cc == nil {
		t.Fatal("NewConversationCompactor returned nil")
	}

	// Before swap: chatFn() routes to prov1.
	before, err := cc.chatFn().Chat(context.Background(), provider.ChatRequest{})
	if err != nil {
		t.Fatalf("before swap: Chat() error: %v", err)
	}
	if before.Content != prov1.response {
		t.Errorf("before swap: Chat() content = %q, want %q", before.Content, prov1.response)
	}

	// Simulate SetProvider updating the live provider (direct field update under Lock).
	current = prov2

	// After swap: chatFn() routes to prov2 — post-swap visibility confirmed.
	after, err := cc.chatFn().Chat(context.Background(), provider.ChatRequest{})
	if err != nil {
		t.Fatalf("after swap: Chat() error: %v", err)
	}
	if after.Content != prov2.response {
		t.Errorf("after swap: Chat() content = %q, want %q", after.Content, prov2.response)
	}
}
