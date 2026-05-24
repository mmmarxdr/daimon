package agent

// commands_new_builtins.go — New built-in command handlers added in PR2+.
//
// Contains:
//   - cmdCancel   (WU4, REQ-6): cancel the in-progress LLM turn for a (channel, sender)
//   - IsDestructiveCommand (WU4, REQ-17): authoritative destructive-command table
//   - cmdCd       (WU5, REQ-5, REQ-23): per-(channel, sender) shell cwd override
//   - cmdResume   (WU7, REQ-1): list/switch active conversation
//   - cmdSave     (WU7, REQ-2): snapshot the current conversation
//   - cmdFork     (WU7, REQ-3): branch conversation at last user turn
//   - cmdExport   (WU7, REQ-4): render conversation as markdown or JSON

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"daimon/internal/provider"
	"daimon/internal/store"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// convOverrides — per-(channel, sender) active conversation ID overrides
// (WU7, REQ-1: /resume <convID> switches the active conv for the caller)
// ---------------------------------------------------------------------------

// convOverrides stores per-(channel, sender) active conversation ID overrides.
// The key is a cancelKey (reused from the cancel registry for consistency).
// Thread-safe via RWMutex.
type convOverrides struct {
	mu        sync.RWMutex
	overrides map[cancelKey]string
}

// newConvOverrides returns a fresh convOverrides.
func newConvOverrides() *convOverrides {
	return &convOverrides{overrides: make(map[cancelKey]string)}
}

// Get returns the active conv override for key, or ("", false) if not set.
func (co *convOverrides) Get(k cancelKey) (string, bool) {
	co.mu.RLock()
	defer co.mu.RUnlock()
	v, ok := co.overrides[k]
	return v, ok
}

// Set stores the active conv ID for key.
func (co *convOverrides) Set(k cancelKey, convID string) {
	co.mu.Lock()
	defer co.mu.Unlock()
	co.overrides[k] = convID
}

// Reset removes the override for key, reverting to the default conv derivation.
func (co *convOverrides) Reset(k cancelKey) {
	co.mu.Lock()
	defer co.mu.Unlock()
	delete(co.overrides, k)
}

// effectiveConvID returns the convID that command handlers should target for
// (channelID, senderID): the /resume override if set, else the default
// userScope-derived convID. Mirrors the resolution order used in
// processMessage (loop.go) so meta-commands stay consistent with the
// active-conversation invariant after /resume.
func (a *Agent) effectiveConvID(channelID, senderID string) string {
	key := cancelKey{ChannelID: channelID, SenderID: senderID}
	if override, ok := a.activeConv.Get(key); ok {
		return override
	}
	return "conv_" + userScope(channelID, senderID)
}

// ---------------------------------------------------------------------------
// Destructive command table (REQ-17, Decision #12)
//
// Placed in the agent package so *Agent.Commands() can populate
// CommandInfo.Destructive without importing the web layer.
// ---------------------------------------------------------------------------

// destructiveCommands is the authoritative list of commands that require
// allow_destructive=true when invoked via the REST /api/commands/run endpoint.
// The list matches the spec REQ-17 table exactly.
var destructiveCommands = map[string]bool{
	"reset":               true,
	"fork":                true,
	"save":                true,
	"cancel":              true,
	"task-cancel":         true,
	"task-cancel-confirm": true,
	"cd":                  true,
	"export":              true,
	"resume":              true,
	"schedule":            true,
	"retry":               true,
	"compact":             true,
}

// IsDestructiveCommand reports whether name is in the destructive-command set.
// Used by the REST handler layer (handler_commands.go) to gate allow_destructive.
func IsDestructiveCommand(name string) bool {
	return destructiveCommands[strings.ToLower(name)]
}

// ---------------------------------------------------------------------------
// /cancel handler (WU4, REQ-6)
// ---------------------------------------------------------------------------

// cmdCancel is the /cancel built-in command handler. It cancels the in-progress
// LLM turn for the (channel, sender) pair that sent the command.
//
// If a turn is in progress: calls a.cancels.Cancel(key) → replies confirmation.
// If no turn is in progress (idempotent): replies neutral message.
//
// /cancel is registered as a closure (method on *Agent) so it can access
// a.cancels, following the same pattern as cmdCompact.
func (a *Agent) cmdCancel(cc CommandContext) error {
	key := cancelKey{ChannelID: cc.ChannelID, SenderID: cc.SenderID}
	if a.cancels.Cancel(key) {
		cc.Reply("Turn cancellation requested.")
	} else {
		cc.Reply("No turn in progress.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// /cd handler (WU5, REQ-5, REQ-23)
// ---------------------------------------------------------------------------

// cmdCd implements the /cd built-in command for per-(channel, sender) shell
// working directory overrides.
//
// Reply format (Decision #7):
//   - No-arg or "~": reset to default → reply "Working directory: <defaultCwd>"
//   - Valid path set  → reply "Changed working directory to: <path>"
//   - Error           → reply "cd: <description>"; no state change
//
// Sandbox enforcement (REQ-23):
//   - Rejects any path containing ".." after filepath.Clean (traversal guard).
//   - If a.sandboxRoot is non-empty (set at construction time from the
//     deployer-configured file-tool base path), the resolved path must be
//     inside it.
func (a *Agent) cmdCd(cc CommandContext) error {
	key := cancelKey{ChannelID: cc.ChannelID, SenderID: cc.SenderID}

	arg := strings.TrimSpace(cc.Args)

	// No-arg or "~" → reset to default cwd.
	if arg == "" || arg == "~" {
		a.shellCwd.Reset(key)
		defaultCwd := a.shellCwd.DefaultCwd()
		cc.Reply("Working directory: " + defaultCwd)
		return nil
	}

	// Reject paths containing ".." before any resolution (REQ-23).
	// filepath.Clean normalises ".." traversals in-place; if the result
	// still contains ".." the path tried to escape the current dir.
	cleaned := filepath.Clean(arg)
	if strings.Contains(cleaned, "..") {
		cc.Reply("cd: path must not contain '..' traversal components")
		return nil
	}

	// Resolve symlinks for a canonical absolute path.
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			cc.Reply(fmt.Sprintf("cd: %s: no such file or directory", cleaned))
		} else {
			cc.Reply(fmt.Sprintf("cd: %s: %v", cleaned, err))
		}
		return nil
	}

	// Must be a directory.
	info, err := os.Stat(resolved)
	if err != nil {
		cc.Reply(fmt.Sprintf("cd: %s: %v", resolved, err))
		return nil
	}
	if !info.IsDir() {
		cc.Reply(fmt.Sprintf("cd: %s: not a directory", resolved))
		return nil
	}

	// Sandbox check (REQ-23): if a sandbox root is configured, the resolved
	// path must be inside it.
	if sandboxRoot := a.shellCwd.SandboxRoot(); sandboxRoot != "" {
		resolvedSandbox, err := filepath.EvalSymlinks(sandboxRoot)
		if err != nil {
			resolvedSandbox = sandboxRoot
		}
		if !strings.HasPrefix(resolved, resolvedSandbox) {
			cc.Reply(fmt.Sprintf("cd: %s: outside sandbox boundary", resolved))
			return nil
		}
	}

	// All checks passed — store the override.
	if err := a.shellCwd.Set(key, resolved); err != nil {
		cc.Reply(fmt.Sprintf("cd: failed to set working directory: %v", err))
		return nil
	}

	cc.Reply("Changed working directory to: " + resolved)
	return nil
}

// ---------------------------------------------------------------------------
// /resume handler (WU7, REQ-1)
// ---------------------------------------------------------------------------

// cmdResume implements the /resume built-in command.
//
// No args: lists recent conversations for the calling (channelID, senderID).
// With <convID> arg: switches the active conversation for this caller.
func (a *Agent) cmdResume(cc CommandContext) error {
	key := cancelKey{ChannelID: cc.ChannelID, SenderID: cc.SenderID}
	arg := strings.TrimSpace(cc.Args)

	if arg == "" {
		// List recent conversations for this channel.
		convs, err := cc.Store.ListConversations(cc.Ctx, cc.ChannelID, 20)
		if err != nil {
			cc.Reply(fmt.Sprintf("resume: failed to list conversations: %v", err))
			return nil
		}
		if len(convs) == 0 {
			cc.Reply("No conversations found.")
			return nil
		}
		var sb strings.Builder
		sb.WriteString("Recent conversations:\n")
		for _, c := range convs {
			ts := c.CreatedAt.Format(time.RFC3339)
			sb.WriteString(fmt.Sprintf("  %s  (created %s)\n", c.ID, ts))
		}
		cc.Reply(sb.String())
		return nil
	}

	// Switch active conversation: verify conv exists first.
	_, err := cc.Store.LoadConversation(cc.Ctx, arg)
	if err != nil {
		cc.Reply(fmt.Sprintf("resume: no conversation found with ID %q", arg))
		return nil
	}
	a.activeConv.Set(key, arg)
	cc.Reply(fmt.Sprintf("Switched to conversation %s.", arg))
	return nil
}

// ---------------------------------------------------------------------------
// /save handler (WU7, REQ-2)
// ---------------------------------------------------------------------------

// cmdSave implements the /save built-in command.
//
// Creates a lightweight named snapshot of the current conversation by copying
// it with a new UUID and storing metadata["snapshot_name"].
func (a *Agent) cmdSave(cc CommandContext) error {
	convID := a.effectiveConvID(cc.ChannelID, cc.SenderID)
	src, err := cc.Store.LoadConversation(cc.Ctx, convID)
	if err != nil {
		cc.Reply(fmt.Sprintf("save: failed to load conversation: %v", err))
		return nil
	}

	// Generate the snapshot name.
	name := strings.TrimSpace(cc.Args)
	if name == "" {
		name = "snapshot-" + time.Now().UTC().Format("20060102-150405")
	}

	// Copy messages into a fresh slice.
	msgs := make([]provider.ChatMessage, len(src.Messages))
	copy(msgs, src.Messages)

	// Build the snapshot conversation.
	snapID := "snap_" + uuid.New().String()
	snap := store.Conversation{
		ID:           snapID,
		ChannelID:    src.ChannelID,
		Messages:     msgs,
		ParentConvID: src.ID,
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"snapshot_name": name},
	}

	if err := cc.Store.SaveConversation(cc.Ctx, snap); err != nil {
		cc.Reply(fmt.Sprintf("save: failed to persist snapshot: %v", err))
		return nil
	}

	cc.Reply(fmt.Sprintf("Snapshot saved: %s (name: %s)", snapID, name))
	return nil
}

// ---------------------------------------------------------------------------
// /fork handler (WU7, REQ-3)
// ---------------------------------------------------------------------------

// cmdFork implements the /fork built-in command.
//
// Branches the current conversation at the last user turn: creates a new conv
// with a new UUID, ParentConvID set to the current conv, and messages up to
// and including the last user-role message. The fork becomes the active conv.
func (a *Agent) cmdFork(cc CommandContext) error {
	key := cancelKey{ChannelID: cc.ChannelID, SenderID: cc.SenderID}
	convID := a.effectiveConvID(cc.ChannelID, cc.SenderID)
	src, err := cc.Store.LoadConversation(cc.Ctx, convID)
	if err != nil {
		cc.Reply(fmt.Sprintf("fork: failed to load conversation: %v", err))
		return nil
	}

	// Find the last user message.
	lastUserIdx := -1
	for i := len(src.Messages) - 1; i >= 0; i-- {
		if src.Messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		cc.Reply("fork: nothing to fork — no messages in conversation")
		return nil
	}

	// Copy messages up to and including the last user turn.
	msgs := make([]provider.ChatMessage, lastUserIdx+1)
	copy(msgs, src.Messages[:lastUserIdx+1])

	forkID := "fork_" + uuid.New().String()
	fork := store.Conversation{
		ID:           forkID,
		ChannelID:    src.ChannelID,
		Messages:     msgs,
		ParentConvID: src.ID,
		CreatedAt:    time.Now(),
	}

	if err := cc.Store.SaveConversation(cc.Ctx, fork); err != nil {
		cc.Reply(fmt.Sprintf("fork: failed to persist fork: %v", err))
		return nil
	}

	// The fork becomes the active conversation.
	a.activeConv.Set(key, forkID)

	cc.Reply(fmt.Sprintf("Forked to conversation %s (branched from %s at last user turn).", forkID, src.ID))
	return nil
}

// ---------------------------------------------------------------------------
// /export handler (WU7, REQ-4)
// ---------------------------------------------------------------------------

// exportMessage is the JSON shape for /export json.
type exportMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// cmdExport implements the /export built-in command.
//
// Renders the current conversation as markdown (default) or JSON via cc.Reply.
func (a *Agent) cmdExport(cc CommandContext) error {
	convID := a.effectiveConvID(cc.ChannelID, cc.SenderID)
	conv, err := cc.Store.LoadConversation(cc.Ctx, convID)
	if err != nil {
		cc.Reply(fmt.Sprintf("export: failed to load conversation: %v", err))
		return nil
	}

	format := strings.ToLower(strings.TrimSpace(cc.Args))
	if format == "" {
		format = "markdown"
	}

	switch format {
	case "markdown", "md":
		var sb strings.Builder
		for _, m := range conv.Messages {
			role := strings.Title(m.Role) //nolint:staticcheck // simple capitalization, not unicode-sensitive
			sb.WriteString(fmt.Sprintf("## %s\n\n%s\n\n", role, m.Content.TextOnly()))
		}
		cc.Reply(sb.String())
	case "json":
		msgs := make([]exportMessage, 0, len(conv.Messages))
		for _, m := range conv.Messages {
			msgs = append(msgs, exportMessage{
				Role:    m.Role,
				Content: m.Content.TextOnly(),
			})
		}
		data, err := json.Marshal(msgs)
		if err != nil {
			cc.Reply(fmt.Sprintf("export: failed to marshal JSON: %v", err))
			return nil
		}
		cc.Reply(string(data))
	default:
		cc.Reply(fmt.Sprintf("export: unknown format %q — accepted formats: markdown (md), json", format))
	}
	return nil
}
