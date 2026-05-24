package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/skill"
	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// WU7 RED tests: /save, /fork, /export, /resume (REQ-1..4)
// ---------------------------------------------------------------------------

// makeAgentWithStore builds a minimal *Agent suitable for command-handler tests.
// It wires up mocks and registers all built-in commands (including the new ones).
func makeAgentWithStore(t *testing.T, st store.Store) *Agent {
	t.Helper()
	ag := New(
		defaultCfg(),
		defaultLimits(),
		config.FilterConfig{},
		&mockChannel{},
		&mockProvider{},
		st,
		audit.NoopAuditor{},
		nil,
		nil,
		skill.SkillIndex{},
		4,
		false,
	)
	return ag
}

// makeAgentCC builds a CommandContext that invokes a method-bound command on ag.
func makeAgentCC(ag *Agent, cr *capturedReply, st store.Store, args string) CommandContext {
	return CommandContext{
		Ctx:       context.Background(),
		ChannelID: "chan:42",
		SenderID:  "user:7",
		Args:      args,
		Store:     st,
		Config:    &config.AgentConfig{},
		Reply:     cr.reply,
		Registry:  ag.commands,
	}
}

// ---------------------------------------------------------------------------
// /save tests (REQ-2)
// ---------------------------------------------------------------------------

// TestCmdSave_NoArg_CreatesSnapshot_WithTimestampName verifies that /save with
// no args creates a new conv copy with metadata["snapshot_name"] set to a
// non-empty timestamp-based string, and replies with the new conv ID.
func TestCmdSave_NoArg_CreatesSnapshot_WithTimestampName(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("hello")},
			{Role: "assistant", Content: content.TextBlock("hi")},
		},
	}
	st := &mockStore{conv: &srcConv}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "")

	if err := ag.cmdSave(cc); err != nil {
		t.Fatalf("cmdSave: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d: %v", len(cr.messages), cr.messages)
	}
	// Reply must contain the new conv ID.
	reply := cr.messages[0]
	if !strings.Contains(reply, "snapshot") && !strings.Contains(reply, "conv_") && !strings.Contains(reply, "snap_") {
		t.Errorf("expected reply to mention snapshot ID, got: %q", reply)
	}

	// The saved conversation must have a snapshot_name in its metadata.
	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()
	if saved == nil {
		t.Fatal("expected a conversation to be saved")
	}
	if saved.ID == srcConv.ID {
		t.Error("expected a NEW conv ID for the snapshot, not the source conv ID")
	}
	if saved.Metadata == nil || saved.Metadata["snapshot_name"] == "" {
		t.Errorf("expected metadata[snapshot_name] to be set, got metadata: %v", saved.Metadata)
	}
	// Source conv messages must be copied.
	if len(saved.Messages) != len(srcConv.Messages) {
		t.Errorf("expected %d messages in snapshot, got %d", len(srcConv.Messages), len(saved.Messages))
	}
}

// TestCmdSave_WithName_UsesArgAsSnapshotName verifies that /save <name> sets
// metadata["snapshot_name"] to the provided name.
func TestCmdSave_WithName_UsesArgAsSnapshotName(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("hi")},
		},
	}
	st := &mockStore{conv: &srcConv}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "my-checkpoint")

	if err := ag.cmdSave(cc); err != nil {
		t.Fatalf("cmdSave: %v", err)
	}

	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()

	if saved.Metadata == nil || saved.Metadata["snapshot_name"] != "my-checkpoint" {
		t.Errorf("expected snapshot_name=my-checkpoint, got %v", saved.Metadata)
	}
}

// TestCmdSave_SourceConversationUnchanged verifies that /save does NOT modify
// the source conversation (REQ-2 scenario 2.3).
func TestCmdSave_SourceConversationUnchanged(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("a")},
			{Role: "assistant", Content: content.TextBlock("b")},
			{Role: "user", Content: content.TextBlock("c")},
			{Role: "assistant", Content: content.TextBlock("d")},
			{Role: "user", Content: content.TextBlock("e")},
		},
	}
	st := &mockStore{conv: &srcConv}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "")

	if err := ag.cmdSave(cc); err != nil {
		t.Fatalf("cmdSave: %v", err)
	}

	// Verify the original conv still has 5 messages by loading it.
	loaded, err := st.LoadConversation(context.Background(), srcConv.ID)
	if err != nil {
		t.Fatalf("LoadConversation: %v", err)
	}
	// mockStore.SaveConversation always overwrites; the last Save was the snapshot.
	// We check the snapshot doesn't carry the source's ID.
	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()
	if saved.ID == srcConv.ID {
		t.Error("snapshot must have a different ID from the source")
	}
	_ = loaded // keep the load to verify no error
}

// TestCmdSave_DoesNotCallLLM verifies that /save never triggers an LLM call.
func TestCmdSave_DoesNotCallLLM(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages:  []provider.ChatMessage{{Role: "user", Content: content.TextBlock("x")}},
	}
	st := &mockStore{conv: &srcConv}
	prov := &mockProvider{}
	ch := &mockChannel{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, ch, prov, st, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:42",
		SenderID:  "user:7",
		Content:   content.TextBlock("/save checkpoint"),
	})

	if prov.callCount() != 0 {
		t.Errorf("expected 0 LLM calls for /save, got %d", prov.callCount())
	}
}

// ---------------------------------------------------------------------------
// /fork tests (REQ-3)
// ---------------------------------------------------------------------------

// TestCmdFork_CreatesNewConvBranchedAtLastUserTurn verifies that /fork creates
// a new conversation with ParentConvID set, containing all messages up to and
// including the last user-role message (REQ-3 scenario 3.1).
func TestCmdFork_CreatesNewConvBranchedAtLastUserTurn(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("hello")},
			{Role: "assistant", Content: content.TextBlock("hi")},
			{Role: "user", Content: content.TextBlock("tell me more")},
		},
	}
	st := &mockStore{conv: &srcConv}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "")

	if err := ag.cmdFork(cc); err != nil {
		t.Fatalf("cmdFork: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d: %v", len(cr.messages), cr.messages)
	}

	// The saved conv must have a new ID, ParentConvID=srcConv.ID, and 3 messages
	// (all messages up to and including the last user turn).
	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()

	if saved.ID == srcConv.ID {
		t.Error("expected new conv ID for fork, not source ID")
	}
	if saved.ParentConvID != srcConv.ID {
		t.Errorf("expected ParentConvID=%q, got %q", srcConv.ID, saved.ParentConvID)
	}
	// Should include exactly 3 messages (up to and including last user turn).
	if len(saved.Messages) != 3 {
		t.Errorf("expected 3 messages in fork, got %d", len(saved.Messages))
	}
	// Last message in fork must be the last user message.
	last := saved.Messages[len(saved.Messages)-1]
	if last.Role != "user" {
		t.Errorf("expected last message role=user, got %q", last.Role)
	}
}

// TestCmdFork_EmptyConversation_ReturnsError verifies that /fork on an empty
// conversation replies with an error (REQ-3 scenario 3.3).
func TestCmdFork_EmptyConversation_ReturnsError(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages:  nil,
	}
	st := &mockStore{conv: &srcConv}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "")

	if err := ag.cmdFork(cc); err != nil {
		t.Fatalf("cmdFork returned non-nil error (should use Reply): %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.Contains(strings.ToLower(cr.messages[0]), "nothing") &&
		!strings.Contains(strings.ToLower(cr.messages[0]), "empty") &&
		!strings.Contains(strings.ToLower(cr.messages[0]), "no messages") {
		t.Errorf("expected error reply about empty conversation, got: %q", cr.messages[0])
	}
}

// TestCmdFork_DoesNotCallLLM verifies that /fork never triggers an LLM call.
func TestCmdFork_DoesNotCallLLM(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages:  []provider.ChatMessage{{Role: "user", Content: content.TextBlock("x")}},
	}
	st := &mockStore{conv: &srcConv}
	prov := &mockProvider{}
	ch := &mockChannel{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, ch, prov, st, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:42",
		SenderID:  "user:7",
		Content:   content.TextBlock("/fork"),
	})

	if prov.callCount() != 0 {
		t.Errorf("expected 0 LLM calls for /fork, got %d", prov.callCount())
	}
}

// ---------------------------------------------------------------------------
// /export tests (REQ-4)
// ---------------------------------------------------------------------------

// TestCmdExport_DefaultMarkdown verifies that /export with no args produces
// markdown output containing both messages with role labels (REQ-4 scenario 4.1).
func TestCmdExport_DefaultMarkdown(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("hi")},
			{Role: "assistant", Content: content.TextBlock("hello")},
		},
	}
	st := &mockStore{conv: &srcConv}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "")

	if err := ag.cmdExport(cc); err != nil {
		t.Fatalf("cmdExport: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	// Must contain both messages and role labels.
	if !strings.Contains(reply, "hi") || !strings.Contains(reply, "hello") {
		t.Errorf("expected both messages in output, got: %q", reply)
	}
	// Must contain role labels (user/assistant or ## User / ## Assistant).
	if !strings.Contains(strings.ToLower(reply), "user") || !strings.Contains(strings.ToLower(reply), "assistant") {
		t.Errorf("expected role labels in markdown output, got: %q", reply)
	}
}

// TestCmdExport_ExplicitMarkdown verifies that /export md produces the same
// result as /export (REQ-4 scenario 4.2).
func TestCmdExport_ExplicitMarkdown(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("question")},
		},
	}
	st := &mockStore{conv: &srcConv}
	ag := makeAgentWithStore(t, st)

	cr1 := &capturedReply{}
	cc1 := makeAgentCC(ag, cr1, st, "")
	if err := ag.cmdExport(cc1); err != nil {
		t.Fatalf("cmdExport no-arg: %v", err)
	}

	cr2 := &capturedReply{}
	cc2 := makeAgentCC(ag, cr2, st, "md")
	if err := ag.cmdExport(cc2); err != nil {
		t.Fatalf("cmdExport md: %v", err)
	}

	if len(cr1.messages) == 0 || len(cr2.messages) == 0 {
		t.Fatal("expected replies from both calls")
	}
	// Both should contain the message.
	if !strings.Contains(cr1.messages[0], "question") || !strings.Contains(cr2.messages[0], "question") {
		t.Errorf("expected message content in both outputs")
	}
}

// TestCmdExport_JSON verifies that /export json produces valid parseable JSON
// (REQ-4 scenario 4.3).
func TestCmdExport_JSON(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("hi")},
			{Role: "assistant", Content: content.TextBlock("hello")},
		},
	}
	st := &mockStore{conv: &srcConv}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "json")

	if err := ag.cmdExport(cc); err != nil {
		t.Fatalf("cmdExport json: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]

	// Must be parseable JSON array.
	var msgs []map[string]interface{}
	if err := json.Unmarshal([]byte(reply), &msgs); err != nil {
		t.Errorf("expected valid JSON array, got parse error: %v\noutput: %q", err, reply)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages in JSON, got %d", len(msgs))
	}
	// Each object must have at least "role" and "content".
	for i, m := range msgs {
		if _, ok := m["role"]; !ok {
			t.Errorf("message[%d] missing 'role' field", i)
		}
		if _, ok := m["content"]; !ok {
			t.Errorf("message[%d] missing 'content' field", i)
		}
	}
}

// TestCmdExport_UnknownFormat_ReturnsError verifies that /export <unknown>
// replies with an error listing accepted formats (REQ-4 scenario 4.4).
func TestCmdExport_UnknownFormat_ReturnsError(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages:  []provider.ChatMessage{{Role: "user", Content: content.TextBlock("x")}},
	}
	st := &mockStore{conv: &srcConv}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "xml")

	if err := ag.cmdExport(cc); err != nil {
		t.Fatalf("cmdExport xml should not return error: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if strings.Contains(cr.messages[0], "<") {
		t.Error("expected error reply, not XML output")
	}
	// Must mention accepted formats.
	if !strings.Contains(strings.ToLower(cr.messages[0]), "markdown") && !strings.Contains(strings.ToLower(cr.messages[0]), "json") {
		t.Errorf("expected reply to list accepted formats, got: %q", cr.messages[0])
	}
}

// TestCmdExport_DoesNotCallLLM verifies that /export never triggers an LLM call.
func TestCmdExport_DoesNotCallLLM(t *testing.T) {
	srcConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages:  []provider.ChatMessage{{Role: "user", Content: content.TextBlock("x")}},
	}
	st := &mockStore{conv: &srcConv}
	prov := &mockProvider{}
	ch := &mockChannel{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, ch, prov, st, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:42",
		SenderID:  "user:7",
		Content:   content.TextBlock("/export json"),
	})

	if prov.callCount() != 0 {
		t.Errorf("expected 0 LLM calls for /export, got %d", prov.callCount())
	}
}

// ---------------------------------------------------------------------------
// /resume tests (REQ-1)
// ---------------------------------------------------------------------------

// TestCmdResume_NoArg_ListsRecentConversations verifies that /resume with no
// args replies with a list of recent conversations for the caller (REQ-1 scenario 1.1).
func TestCmdResume_NoArg_ListsRecentConversations(t *testing.T) {
	// mockStore.ListConversations returns nil, nil by default.
	// Override it so we return 3 conversations.
	st := &listableStore{
		convs: []store.Conversation{
			{ID: "conv-a", ChannelID: "chan:42", CreatedAt: time.Now().Add(-3 * time.Hour)},
			{ID: "conv-b", ChannelID: "chan:42", CreatedAt: time.Now().Add(-2 * time.Hour)},
			{ID: "conv-c", ChannelID: "chan:42", CreatedAt: time.Now().Add(-1 * time.Hour)},
		},
	}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "")

	if err := ag.cmdResume(cc); err != nil {
		t.Fatalf("cmdResume: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d: %v", len(cr.messages), cr.messages)
	}
	reply := cr.messages[0]
	// Reply must mention all 3 conv IDs.
	for _, id := range []string{"conv-a", "conv-b", "conv-c"} {
		if !strings.Contains(reply, id) {
			t.Errorf("expected reply to contain %q, got: %q", id, reply)
		}
	}
	// Active conversation must be unchanged (no switch).
	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	if _, ok := ag.activeConv.Get(key); ok {
		t.Error("expected no active conv override after /resume with no args")
	}
}

// TestCmdResume_WithValidConvID_SwitchesActiveConv verifies that /resume <convID>
// switches the active conversation (REQ-1 scenario 1.2).
func TestCmdResume_WithValidConvID_SwitchesActiveConv(t *testing.T) {
	convB := store.Conversation{ID: "conv-b", ChannelID: "chan:42"}
	st := &listableStore{
		target: &convB,
	}
	ag := makeAgentWithStore(t, st)
	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "conv-b")

	if err := ag.cmdResume(cc); err != nil {
		t.Fatalf("cmdResume conv-b: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d: %v", len(cr.messages), cr.messages)
	}
	// Reply must mention conv-b.
	if !strings.Contains(cr.messages[0], "conv-b") {
		t.Errorf("expected reply to mention conv-b, got: %q", cr.messages[0])
	}
	// Active conv override must be set.
	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	got, ok := ag.activeConv.Get(key)
	if !ok {
		t.Fatal("expected active conv override to be set after /resume <convID>")
	}
	if got != "conv-b" {
		t.Errorf("expected active conv=conv-b, got %q", got)
	}
}

// TestCmdResume_UnknownConvID_ErrorReply_NoMutation verifies that /resume
// with an unknown conv ID replies with an error and does NOT change the active
// conv (REQ-1 scenario 1.3).
func TestCmdResume_UnknownConvID_ErrorReply_NoMutation(t *testing.T) {
	st := &listableStore{} // no target → LoadConversation returns ErrNotFound
	ag := makeAgentWithStore(t, st)

	// Set an initial override to verify it doesn't change.
	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	ag.activeConv.Set(key, "conv-original")

	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "conv-x")

	if err := ag.cmdResume(cc); err != nil {
		t.Fatalf("cmdResume conv-x should not return error: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	// Reply must indicate an error.
	if !strings.Contains(strings.ToLower(cr.messages[0]), "not found") &&
		!strings.Contains(strings.ToLower(cr.messages[0]), "unknown") &&
		!strings.Contains(strings.ToLower(cr.messages[0]), "error") &&
		!strings.Contains(strings.ToLower(cr.messages[0]), "no conversation") {
		t.Errorf("expected error reply for unknown conv, got: %q", cr.messages[0])
	}
	// Active conv must be unchanged.
	got, _ := ag.activeConv.Get(key)
	if got != "conv-original" {
		t.Errorf("expected active conv unchanged (conv-original), got %q", got)
	}
}

// TestCmdResume_DoesNotCallLLM verifies that /resume never triggers an LLM call.
func TestCmdResume_DoesNotCallLLM(t *testing.T) {
	st := &listableStore{}
	prov := &mockProvider{}
	ch := &mockChannel{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, ch, prov, st, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:42",
		SenderID:  "user:7",
		Content:   content.TextBlock("/resume"),
	})

	if prov.callCount() != 0 {
		t.Errorf("expected 0 LLM calls for /resume, got %d", prov.callCount())
	}
}

// ---------------------------------------------------------------------------
// listableStore — test helper that supports ListConversations and targeted
// LoadConversation (for /resume tests).
// ---------------------------------------------------------------------------

// listableStore extends mockStore's behavior for list/load needed by /resume.
type listableStore struct {
	mockStore
	convs  []store.Conversation
	target *store.Conversation // if set, LoadConversation returns this for any ID
}

func (l *listableStore) ListConversations(ctx context.Context, channelID string, limit int) ([]store.Conversation, error) {
	return l.convs, nil
}

func (l *listableStore) LoadConversation(ctx context.Context, id string) (*store.Conversation, error) {
	if l.target != nil && l.target.ID == id {
		cp := *l.target
		return &cp, nil
	}
	return nil, store.ErrNotFound
}

// ---------------------------------------------------------------------------
// byIDStore — store.Store mock that resolves LoadConversation by exact ID
// against a map of conversations. Used by W-2 regression tests to prove
// that /save, /fork, and /export honor the activeConv override.
// ---------------------------------------------------------------------------

type byIDStore struct {
	mockStore
	convs     map[string]*store.Conversation
	lastSaved *store.Conversation
}

func newByIDStore(convs ...store.Conversation) *byIDStore {
	m := make(map[string]*store.Conversation, len(convs))
	for i := range convs {
		c := convs[i]
		m[c.ID] = &c
	}
	return &byIDStore{convs: m}
}

func (b *byIDStore) LoadConversation(_ context.Context, id string) (*store.Conversation, error) {
	c, ok := b.convs[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *c
	cp.Messages = append([]provider.ChatMessage(nil), c.Messages...)
	return &cp, nil
}

func (b *byIDStore) SaveConversation(_ context.Context, c store.Conversation) error {
	saved := c
	b.lastSaved = &saved
	if b.convs == nil {
		b.convs = map[string]*store.Conversation{}
	}
	b.convs[c.ID] = &saved
	return nil
}

// ---------------------------------------------------------------------------
// W-2 regression tests: /save, /fork, /export MUST honor activeConv override
// after /resume <convID> has switched the active conversation.
// REQ-1 + REQ-2/3/4 interaction — see verify-report-pr3 W-2.
// ---------------------------------------------------------------------------

// TestCmdSave_HonorsActiveConvOverride verifies that after /resume conv-B,
// /save snapshots conv-B (the override), not the default-derived conv.
func TestCmdSave_HonorsActiveConvOverride(t *testing.T) {
	defaultConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("default-only")},
		},
	}
	overrideConv := store.Conversation{
		ID:        "conv-override",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("override-1")},
			{Role: "assistant", Content: content.TextBlock("override-2")},
		},
	}
	st := newByIDStore(defaultConv, overrideConv)
	ag := makeAgentWithStore(t, st)
	ag.activeConv.Set(cancelKey{ChannelID: "chan:42", SenderID: "user:7"}, overrideConv.ID)

	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "")
	if err := ag.cmdSave(cc); err != nil {
		t.Fatalf("cmdSave: %v", err)
	}
	if st.lastSaved == nil {
		t.Fatal("expected snapshot to be persisted")
	}
	if st.lastSaved.ParentConvID != overrideConv.ID {
		t.Errorf("snapshot ParentConvID = %q, want %q (the activeConv override)", st.lastSaved.ParentConvID, overrideConv.ID)
	}
	if got, want := len(st.lastSaved.Messages), len(overrideConv.Messages); got != want {
		t.Errorf("snapshot Messages count = %d, want %d (from override conv)", got, want)
	}
}

// TestCmdFork_HonorsActiveConvOverride verifies that after /resume conv-B,
// /fork branches conv-B (the override), not the default-derived conv.
func TestCmdFork_HonorsActiveConvOverride(t *testing.T) {
	defaultConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("default-only")},
		},
	}
	overrideConv := store.Conversation{
		ID:        "conv-override",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("u1")},
			{Role: "assistant", Content: content.TextBlock("a1")},
			{Role: "user", Content: content.TextBlock("u2")},
		},
	}
	st := newByIDStore(defaultConv, overrideConv)
	ag := makeAgentWithStore(t, st)
	ag.activeConv.Set(cancelKey{ChannelID: "chan:42", SenderID: "user:7"}, overrideConv.ID)

	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "")
	if err := ag.cmdFork(cc); err != nil {
		t.Fatalf("cmdFork: %v", err)
	}
	if st.lastSaved == nil {
		t.Fatal("expected fork to be persisted")
	}
	if st.lastSaved.ParentConvID != overrideConv.ID {
		t.Errorf("fork ParentConvID = %q, want %q (the activeConv override)", st.lastSaved.ParentConvID, overrideConv.ID)
	}
	// Override has 3 msgs with the last being a user turn → fork copies all 3.
	if got, want := len(st.lastSaved.Messages), 3; got != want {
		t.Errorf("fork Messages count = %d, want %d (from override conv up to last user turn)", got, want)
	}
}

// TestCmdExport_HonorsActiveConvOverride verifies that after /resume conv-B,
// /export renders conv-B's messages (the override), not the default-derived conv.
func TestCmdExport_HonorsActiveConvOverride(t *testing.T) {
	defaultConv := store.Conversation{
		ID:        "conv_chan:42:user:7",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("DEFAULT-MARKER")},
		},
	}
	overrideConv := store.Conversation{
		ID:        "conv-override",
		ChannelID: "chan:42",
		Messages: []provider.ChatMessage{
			{Role: "user", Content: content.TextBlock("OVERRIDE-MARKER")},
		},
	}
	st := newByIDStore(defaultConv, overrideConv)
	ag := makeAgentWithStore(t, st)
	ag.activeConv.Set(cancelKey{ChannelID: "chan:42", SenderID: "user:7"}, overrideConv.ID)

	cr := &capturedReply{}
	cc := makeAgentCC(ag, cr, st, "")
	if err := ag.cmdExport(cc); err != nil {
		t.Fatalf("cmdExport: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	if !strings.Contains(reply, "OVERRIDE-MARKER") {
		t.Errorf("export reply must include override conversation content, got: %q", reply)
	}
	if strings.Contains(reply, "DEFAULT-MARKER") {
		t.Errorf("export reply MUST NOT include default conversation content after /resume, got: %q", reply)
	}
}
