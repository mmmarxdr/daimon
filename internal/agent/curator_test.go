package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"daimon/internal/config"
	"daimon/internal/provider"
	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// Mock provider for curator tests
// ---------------------------------------------------------------------------

// curatorMockProvider returns a configurable Chat response or error.
// It records every call so tests can assert call counts.
type curatorMockProvider struct {
	mu       sync.Mutex
	response string
	err      error
	calls    int
}

func (p *curatorMockProvider) Name() string                                  { return "mock" }
func (p *curatorMockProvider) Model() string                                 { return "mock-model" }
func (p *curatorMockProvider) SupportsTools() bool                           { return false }
func (p *curatorMockProvider) SupportsMultimodal() bool                      { return false }
func (p *curatorMockProvider) SupportsAudio() bool                           { return false }
func (p *curatorMockProvider) HealthCheck(_ context.Context) (string, error) { return "ok", nil }
func (p *curatorMockProvider) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return &provider.ChatResponse{Content: p.response}, nil
}

func (p *curatorMockProvider) chatCalls() int { //nolint:unused // kept for future test assertions
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// ---------------------------------------------------------------------------
// Mock store for curator tests
// ---------------------------------------------------------------------------

// curatorMockStore records AppendMemory and UpdateMemory calls.
// SearchMemory returns a configurable set of candidates.
type curatorMockStore struct {
	mu           sync.Mutex
	appendCalls  []store.MemoryEntry
	updateCalls  []store.MemoryEntry
	candidates   []store.MemoryEntry // returned by SearchMemory
	searchErr    error
	appendErr    error
}

func (s *curatorMockStore) SaveConversation(_ context.Context, _ store.Conversation) error {
	return nil
}
func (s *curatorMockStore) LoadConversation(_ context.Context, _ string) (*store.Conversation, error) {
	return nil, store.ErrNotFound
}
func (s *curatorMockStore) ListConversations(_ context.Context, _ string, _ int) ([]store.Conversation, error) {
	return nil, nil
}
func (s *curatorMockStore) AppendMemory(_ context.Context, _ string, entry store.MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.appendErr != nil {
		return s.appendErr
	}
	s.appendCalls = append(s.appendCalls, entry)
	return nil
}
func (s *curatorMockStore) SearchMemory(_ context.Context, _ string, _ string, _ int) ([]store.MemoryEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.searchErr != nil {
		return nil, s.searchErr
	}
	return s.candidates, nil
}
func (s *curatorMockStore) UpdateMemory(_ context.Context, _ string, entry store.MemoryEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCalls = append(s.updateCalls, entry)
	return nil
}
func (s *curatorMockStore) Close() error { return nil }

func (s *curatorMockStore) appendCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.appendCalls)
}

func (s *curatorMockStore) updateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.updateCalls)
}

func (s *curatorMockStore) lastAppend() store.MemoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.appendCalls) == 0 {
		return store.MemoryEntry{}
	}
	return s.appendCalls[len(s.appendCalls)-1]
}

func (s *curatorMockStore) lastUpdate() store.MemoryEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.updateCalls) == 0 {
		return store.MemoryEntry{}
	}
	return s.updateCalls[len(s.updateCalls)-1]
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func enabledCurationCfg() config.MemoryCurationConfig {
	return config.MemoryCurationConfig{
		Enabled:          true,
		MinImportance:    5,
		MinResponseChars: 50,
	}
}

func enabledDedupCfg() config.DeduplicationConfig { //nolint:unused // kept for future test assertions
	return config.DeduplicationConfig{
		Enabled:         true,
		CosineThreshold: 0.85,
		MaxCandidates:   5,
	}
}

func disabledDedupCfg() config.DeduplicationConfig {
	return config.DeduplicationConfig{Enabled: false}
}

// classifyJSON returns a well-formed JSON classification string for tests.
// Includes a non-empty memorable_fact so the Curator's empty-fact gate
// doesn't drop the entry — tests that want to exercise the gate should use
// classifyJSONNoFact instead.
func classifyJSON(importance int, typ, topic, title string) string {
	return fmt.Sprintf(`{"importance":%d,"type":%q,"topic":%q,"title":%q,"memorable_fact":%q}`,
		importance, typ, topic, title, "User stated a memorable fact about themselves.")
}

// classifyJSONWithCluster returns a well-formed JSON classification string
// including the cluster field. Includes a non-empty memorable_fact for the
// same reason as classifyJSON.
func classifyJSONWithCluster(importance int, typ, cluster, topic, title string) string {
	return fmt.Sprintf(`{"importance":%d,"type":%q,"cluster":%q,"topic":%q,"title":%q,"memorable_fact":%q}`,
		importance, typ, cluster, topic, title, "User stated a memorable fact about themselves.")
}

// classifyJSONNoFact returns a classification with an empty memorable_fact —
// used to verify the Curator drops the entry instead of persisting the raw
// response.
func classifyJSONNoFact(importance int, typ, title string) string {
	return fmt.Sprintf(`{"importance":%d,"type":%q,"cluster":"general","topic":"","title":%q,"memorable_fact":""}`,
		importance, typ, title)
}

// longResponse returns a string of n repeated characters — useful to exceed
// the MinResponseChars threshold without crafting real content.
func longResponse(n int) string {
	return strings.Repeat("x", n)
}

// ---------------------------------------------------------------------------
// Test 1: NewCurator returns nil when Enabled = false
// ---------------------------------------------------------------------------

func TestCurator_NewCurator_Disabled(t *testing.T) {
	prov := &curatorMockProvider{}
	st := &curatorMockStore{}
	cfg := config.MemoryCurationConfig{Enabled: false}

	c := NewCurator(prov, st, nil, nil, cfg, disabledDedupCfg())
	if c != nil {
		t.Fatal("expected nil Curator when Enabled=false, got non-nil")
	}
}

// ---------------------------------------------------------------------------
// Test 2: shouldSkip — short response
// ---------------------------------------------------------------------------

func TestCurator_ShouldSkip_ShortResponse(t *testing.T) {
	prov := &curatorMockProvider{response: classifyJSON(8, "fact", "topic", "title")}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	// 49 chars — below MinResponseChars (50).
	if !c.shouldSkip(strings.Repeat("a", 49)) {
		t.Fatal("expected shouldSkip=true for 49-char response")
	}

	// Exactly 50 chars should NOT be skipped.
	if c.shouldSkip(strings.Repeat("a", 50)) {
		t.Fatal("expected shouldSkip=false for 50-char response")
	}
}

// ---------------------------------------------------------------------------
// Test 3: shouldSkip — refusal prefix
// ---------------------------------------------------------------------------

func TestCurator_ShouldSkip_Refusal(t *testing.T) {
	prov := &curatorMockProvider{}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	refusals := []string{
		"I'm sorry, I cannot help with that request. " + longResponse(50),
		"I cannot assist with that. " + longResponse(50),
		"I can't do that. " + longResponse(50),
		"I don't have access to that information. " + longResponse(50),
		"Lo siento, no puedo ayudarte con eso. " + longResponse(20),
		"No puedo responder esa pregunta. " + longResponse(30),
	}

	for _, r := range refusals {
		if !c.shouldSkip(r) {
			t.Errorf("expected shouldSkip=true for refusal: %q", r[:40])
		}
	}
}

// ---------------------------------------------------------------------------
// Test 4: shouldSkip — filler
// ---------------------------------------------------------------------------

func TestCurator_ShouldSkip_Filler(t *testing.T) {
	prov := &curatorMockProvider{}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	fillers := []string{"ok", "sure", "done", "understood", "got it", "Great!", "Thanks.", "OK!"}
	for _, f := range fillers {
		if !c.shouldSkip(f) {
			t.Errorf("expected shouldSkip=true for filler: %q", f)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 5: shouldSkip — valid non-skippable response
// ---------------------------------------------------------------------------

func TestCurator_ShouldSkip_ValidResponse(t *testing.T) {
	prov := &curatorMockProvider{}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	valid := "The user prefers Go for all backend services and wants to avoid Python in new projects."
	if c.shouldSkip(valid) {
		t.Fatal("expected shouldSkip=false for substantive response")
	}
}

// ---------------------------------------------------------------------------
// Test 6: classify — success with mock provider
// ---------------------------------------------------------------------------

func TestCurator_Classify_Success(t *testing.T) {
	prov := &curatorMockProvider{
		response: classifyJSON(8, "preference", "technology", "User prefers Go for backend"),
	}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	result, err := c.classify(context.Background(), "what language do you prefer?", longResponse(100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Importance != 8 {
		t.Errorf("expected importance=8, got %d", result.Importance)
	}
	if result.Type != "preference" {
		t.Errorf("expected type=preference, got %q", result.Type)
	}
	if result.Topic != "technology" {
		t.Errorf("expected topic=technology, got %q", result.Topic)
	}
	if result.Title != "User prefers Go for backend" {
		t.Errorf("expected title set correctly, got %q", result.Title)
	}
}

// ---------------------------------------------------------------------------
// Test 7: classify — parse error falls back to defaults
// ---------------------------------------------------------------------------

func TestCurator_Classify_ParseError_Fallback(t *testing.T) {
	prov := &curatorMockProvider{response: "this is not json at all"}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	response := longResponse(100)
	result, err := c.classify(context.Background(), "user msg", response)
	// Parse error → nil error (fallback is used silently). The new fallback
	// returns importance=0/type=skip with empty memorable_fact so the
	// downstream persistence path drops the entry rather than dumping raw
	// text into memory under a guessed-importance.
	if err != nil {
		t.Fatalf("expected nil error on parse fallback, got %v", err)
	}
	if result.Importance != 0 {
		t.Errorf("expected fallback importance=0 (skip), got %d", result.Importance)
	}
	if result.Type != "skip" {
		t.Errorf("expected fallback type=skip, got %q", result.Type)
	}
	if result.MemorableFact != "" {
		t.Errorf("expected fallback memorable_fact empty, got %q", result.MemorableFact)
	}
}

// ---------------------------------------------------------------------------
// Test 8: Curate — low importance → not saved
// ---------------------------------------------------------------------------

func TestCurator_Curate_LowImportance_NotSaved(t *testing.T) {
	prov := &curatorMockProvider{
		response: classifyJSON(2, "skip", "irrelevant", "trivial"),
	}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	// 100+ char response so it passes the fast-path skip.
	err := c.Curate(context.Background(), "scope-1", "hello", longResponse(100), "conv-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.appendCount() != 0 {
		t.Errorf("expected 0 appends for low-importance response, got %d", st.appendCount())
	}
}

// ---------------------------------------------------------------------------
// Test 9: Curate — high importance → saved with metadata
// ---------------------------------------------------------------------------

func TestCurator_Curate_HighImportance_Saved(t *testing.T) {
	prov := &curatorMockProvider{
		response: classifyJSON(8, "preference", "technology", "Prefers Go for backend"),
	}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	userMsg := "What language do you use for backend?"
	response := "I always use Go for backend services because of its performance and simplicity. Python is reserved for data science tasks."

	err := c.Curate(context.Background(), "scope-1", userMsg, response, "conv-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.appendCount() != 1 {
		t.Fatalf("expected 1 append for high-importance response, got %d", st.appendCount())
	}

	saved := st.lastAppend()
	if saved.Importance != 8 {
		t.Errorf("expected saved importance=8, got %d", saved.Importance)
	}
	if saved.Type != "preference" {
		t.Errorf("expected saved type=preference, got %q", saved.Type)
	}
	if saved.Topic != "technology" {
		t.Errorf("expected saved topic=technology, got %q", saved.Topic)
	}
	if saved.Title != "Prefers Go for backend" {
		t.Errorf("expected saved title='Prefers Go for backend', got %q", saved.Title)
	}
	if saved.ScopeID != "scope-1" {
		t.Errorf("expected ScopeID=scope-1, got %q", saved.ScopeID)
	}
	if saved.ID == "" {
		t.Error("expected non-empty entry ID")
	}
}

// ---------------------------------------------------------------------------
// Test 10: Curate — duplicate found → UpdateMemory called, no AppendMemory
// ---------------------------------------------------------------------------

func TestCurator_Curate_Dedup_UpdatesExisting(t *testing.T) {
	// Dedup compares the distilled memorable_fact against existing entries,
	// not the raw response. The mock returns the canned fact from the
	// classifyJSON helper — so the existing candidate must mirror that text
	// for Jaccard similarity to fire. (This shifted with the memorable_fact
	// refactor: previously dedup compared raw response to existing content.)
	prov := &curatorMockProvider{
		response: classifyJSON(7, "fact", "cooking", "User likes pasta"),
	}

	existingContent := "User stated a memorable fact about themselves."
	existingID := "existing-mem-id"

	st := &curatorMockStore{
		// Pre-seed a candidate with high Jaccard similarity to the incoming fact.
		candidates: []store.MemoryEntry{
			{
				ID:      existingID,
				ScopeID: "scope-1",
				Content: existingContent,
				Topic:   "food",
				Type:    "fact",
			},
		},
	}

	dedupCfg := config.DeduplicationConfig{
		Enabled:         true,
		CosineThreshold: 0.85,
		MaxCandidates:   5,
	}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), dedupCfg)

	// Response is verbose; the classifier-extracted fact (canned via
	// classifyJSON) is what matters for dedup matching.
	verboseResponse := "Yes, the user enjoys cooking pasta dishes and making Italian food at home on weekends regularly. " + longResponse(50)
	err := c.Curate(context.Background(), "scope-1", "do you like pasta?", verboseResponse, "conv-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if st.appendCount() != 0 {
		t.Errorf("expected 0 AppendMemory calls (dedup should update), got %d", st.appendCount())
	}
	if st.updateCount() != 1 {
		t.Fatalf("expected 1 UpdateMemory call for dedup, got %d", st.updateCount())
	}

	updated := st.lastUpdate()
	if updated.ID != existingID {
		t.Errorf("expected UpdateMemory called with existing ID %q, got %q", existingID, updated.ID)
	}
}

// ---------------------------------------------------------------------------
// Cluster: Curator persists cluster from classification output
// ---------------------------------------------------------------------------

func TestCurator_Cluster_PersistedFromLLM(t *testing.T) {
	prov := &curatorMockProvider{
		response: classifyJSONWithCluster(8, "preference", "preferences", "lang", "prefers Go"),
	}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	err := c.Curate(context.Background(), "scope-1", "what do you prefer?", longResponse(100), "conv-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.appendCount() != 1 {
		t.Fatalf("expected 1 AppendMemory call, got %d", st.appendCount())
	}
	if got := st.lastAppend().Cluster; got != "preferences" {
		t.Errorf("expected Cluster=preferences, got %q", got)
	}
}

func TestCurator_Cluster_FallbackOnUnknown(t *testing.T) {
	// Cluster outside the enum must fall back to "general".
	prov := &curatorMockProvider{
		response: classifyJSONWithCluster(8, "fact", "bogus-cluster", "t", "hello"),
	}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	err := c.Curate(context.Background(), "scope-1", "u", longResponse(100), "conv-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := st.lastAppend().Cluster; got != "general" {
		t.Errorf("expected fallback Cluster=general for unknown bucket, got %q", got)
	}
}

func TestCurator_Cluster_FallbackOnMissing(t *testing.T) {
	// Older classifyJSON helper omits cluster entirely — must fall back to "general".
	prov := &curatorMockProvider{
		response: classifyJSON(8, "fact", "t", "hello"),
	}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	err := c.Curate(context.Background(), "scope-1", "u", longResponse(100), "conv-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := st.lastAppend().Cluster; got != "general" {
		t.Errorf("expected fallback Cluster=general when field omitted, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// memorable_fact: persisted Content + skip-on-empty
// ---------------------------------------------------------------------------

func TestCurator_MemorableFact_PersistedAsContent(t *testing.T) {
	// The whole point: even when the LLM response is a 5KB markdown wall, the
	// memory entry should hold the distilled 1-sentence fact, never the raw
	// response.
	prov := &curatorMockProvider{
		response: `{"importance":8,"type":"fact","cluster":"technical","topic":"stack","title":"User runs payments service in Go","memorable_fact":"User runs the payments service at Helix and it is written in Go."}`,
	}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	hugeMarkdown := "## Big tutorial\n\n" + strings.Repeat("blah ", 1000)
	if err := c.Curate(context.Background(), "scope-1", "tell me about Go", hugeMarkdown, "conv-1"); err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if st.appendCount() != 1 {
		t.Fatalf("expected 1 AppendMemory, got %d", st.appendCount())
	}
	got := st.lastAppend()
	if got.Content != "User runs the payments service at Helix and it is written in Go." {
		t.Errorf("Content should be the memorable_fact, got %q", got.Content)
	}
	if strings.Contains(got.Content, "##") || strings.Contains(got.Content, "blah") {
		t.Errorf("Content must not contain raw response markdown, got %q", got.Content)
	}
}

func TestCurator_EmptyMemorableFact_DoesNotPersist(t *testing.T) {
	// Generic explanation/tutorial → classifier returns empty memorable_fact
	// → entry dropped, conversation transcript still has the response.
	prov := &curatorMockProvider{response: classifyJSONNoFact(7, "context", "Generic Kubernetes tutorial")}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	if err := c.Curate(context.Background(), "scope-1", "explain k8s", longResponse(2000), "conv-1"); err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if st.appendCount() != 0 {
		t.Errorf("expected NO Append calls when memorable_fact is empty, got %d", st.appendCount())
	}
	if st.updateCount() != 0 {
		t.Errorf("expected NO Update calls when memorable_fact is empty, got %d", st.updateCount())
	}
}

func TestCurator_ClassifyParseFailure_DoesNotPersist(t *testing.T) {
	// Defensive fallback: when the classifier output cannot be parsed, the
	// Curator must NOT save anything. Previously it would save with a
	// guessed importance=5/type=context, which is how raw markdown started
	// leaking into the dashboard.
	prov := &curatorMockProvider{response: "definitely not valid json"}
	st := &curatorMockStore{}
	c := NewCurator(prov, st, nil, nil, enabledCurationCfg(), disabledDedupCfg())

	if err := c.Curate(context.Background(), "scope-1", "u", longResponse(200), "conv-1"); err != nil {
		t.Fatalf("Curate: %v", err)
	}
	if st.appendCount() != 0 {
		t.Errorf("expected NO Append on parse failure, got %d", st.appendCount())
	}
}
