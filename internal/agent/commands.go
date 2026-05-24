package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/store"
)

// Command source constants — the authoritative string values used in the registry,
// GET /api/commands responses, and UnregisterAllBySource calls.
const (
	// SourceBuiltin marks commands registered by registerBuiltinCommands at agent init.
	SourceBuiltin = "builtin"
	// SourceCron marks commands registered by registerCronCommands via WithCronCommands.
	SourceCron = "cron"
	// SourceSkill marks commands auto-mounted from ExecutableSkillDef via ReplaceExecutableSkills.
	SourceSkill = "skill"
)

// CommandContext carries everything a command handler needs to operate.
type CommandContext struct {
	Ctx          context.Context
	ChannelID    string
	SenderID     string
	Args         string
	Store        store.Store
	Config       *config.AgentConfig
	Reply        func(string)
	Registry     *CommandRegistry
	ProviderName string
	ChannelName  string
	StartedAt    time.Time
	Inbox        chan<- channel.IncomingMessage
}

// CommandHandler is the function signature for all slash command handlers.
type CommandHandler func(cc CommandContext) error

type commandEntry struct {
	handler CommandHandler
	desc    string
	source  string // SourceBuiltin | SourceCron | SourceSkill
}

// CommandEntryInfo is the exported view of a registry entry, used by
// EntriesWithSource and by the REST handler layer (GET /api/commands).
type CommandEntryInfo struct {
	Name   string
	Desc   string
	Source string
}

// CommandInfo is the public REST view of a registered command.
// Returned by Agent.Commands() and serialized in GET /api/commands responses.
type CommandInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Destructive bool   `json:"destructive"`
}

// RunCommandRequest is the body of POST /api/commands/run.
type RunCommandRequest struct {
	Name             string `json:"name"`
	Args             string `json:"args"`
	ChannelID        string `json:"channel_id"`
	SenderID         string `json:"sender_id"`
	AllowDestructive bool   `json:"allow_destructive"`
}

// CommandResult is the response for a successful POST /api/commands/run.
type CommandResult struct {
	Reply string `json:"reply"`
}

// CommandRegistry holds registered slash commands.
//
// Concurrency: mu protects commands and all derived reads/writes.
// Lock acquisition order when nesting with agent-level locks:
//
//	toolsMu (agent.go) MUST be acquired BEFORE commandsMu.
//	commandsMu MUST NOT be acquired while holding a lock tied to the
//	agent goroutine's inbox processing.
type CommandRegistry struct {
	// mu is an RWMutex because Lookup happens on every incoming message
	// (read-heavy) while Register/RegisterIfFree/UnregisterAllBySource
	// happen rarely (boot-time or hot-reload). RWMutex gives correct
	// concurrent semantics with lower contention than a plain Mutex.
	mu       sync.RWMutex
	commands map[string]commandEntry
}

// NewCommandRegistry creates an empty CommandRegistry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{commands: make(map[string]commandEntry)}
}

// Register adds or overwrites a command. Name is stored lowercase.
// Intended for built-in registration only (silent overwrite semantics are
// acceptable at startup when ordering is deterministic). Cron and skill
// callers must use RegisterIfFree instead.
func (r *CommandRegistry) Register(name, desc string, handler CommandHandler, source string) {
	key := strings.ToLower(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[key] = commandEntry{handler: handler, desc: desc, source: source}
}

// RegisterIfFree registers a command only if the name is not already taken.
// Returns (true, nil) on success, (false, nil) when the name is already
// registered (the caller decides whether to log or skip — not an error).
// Returns (false, err) only for invalid name format.
func (r *CommandRegistry) RegisterIfFree(name, desc string, handler CommandHandler, source string) (bool, error) {
	key := strings.ToLower(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.registerIfFreeLocked(key, desc, handler, source)
}

// registerIfFreeLocked is the lock-free variant for callers that already
// hold r.mu (e.g. hot-reload under commandsMu). Must be called with mu held.
func (r *CommandRegistry) registerIfFreeLocked(name, desc string, handler CommandHandler, source string) (bool, error) {
	if _, exists := r.commands[name]; exists {
		return false, nil
	}
	r.commands[name] = commandEntry{handler: handler, desc: desc, source: source}
	return true, nil
}

// UnregisterAllBySource atomically removes all entries whose source matches
// the given string. Returns the number of entries removed. Safe to call
// concurrently with Lookup and RegisterIfFree.
func (r *CommandRegistry) UnregisterAllBySource(source string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.unregisterAllBySourceLocked(source)
}

// unregisterAllBySourceLocked is the lock-free variant. Must be called with mu held.
func (r *CommandRegistry) unregisterAllBySourceLocked(source string) int {
	n := 0
	for name, e := range r.commands {
		if e.source == source {
			delete(r.commands, name)
			n++
		}
	}
	return n
}

// Lookup returns the handler for a command name (case-insensitive).
func (r *CommandRegistry) Lookup(name string) (CommandHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.commands[strings.ToLower(name)]
	if !ok {
		return nil, false
	}
	return e.handler, true
}

// Entries returns a map of command name → description.
func (r *CommandRegistry) Entries() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m := make(map[string]string, len(r.commands))
	for name, e := range r.commands {
		m[name] = e.desc
	}
	return m
}

// EntriesWithSource returns a slice of CommandEntryInfo containing name,
// description, and source for every registered command. The slice is a
// snapshot — callers may mutate it freely. Used by GET /api/commands and
// the grouped /help output.
func (r *CommandRegistry) EntriesWithSource() []CommandEntryInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]CommandEntryInfo, 0, len(r.commands))
	for name, e := range r.commands {
		out = append(out, CommandEntryInfo{
			Name:   name,
			Desc:   e.desc,
			Source: e.source,
		})
	}
	return out
}

// Names returns sorted command names.
func (r *CommandRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.commands))
	for name := range r.commands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// parseCommand parses a slash command from raw text.
// Returns (name, args, true) on a valid command, ("", "", false) otherwise.
// Valid command: first token matches /[a-zA-Z][a-zA-Z0-9_]{0,31}
func parseCommand(text string) (name, args string, isCommand bool) {
	if text == "" || text[0] != '/' {
		return "", "", false
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return "", "", false
	}
	cmd := fields[0] // e.g. "/help"
	if len(cmd) < 2 {
		return "", "", false
	}
	raw := cmd[1:] // strip leading /
	if len(raw) > 32 {
		return "", "", false
	}
	// First char must be a letter.
	if !isLetter(raw[0]) {
		return "", "", false
	}
	// Remaining chars: letter, digit, or underscore.
	for i := 1; i < len(raw); i++ {
		c := raw[i]
		if !isLetter(c) && !isDigit(c) && c != '_' {
			return "", "", false
		}
	}
	name = strings.ToLower(raw)
	// Args is everything after the first whitespace-delimited token.
	if idx := strings.IndexByte(text, ' '); idx >= 0 {
		args = strings.TrimSpace(text[idx+1:])
	}
	return name, args, true
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// cmdPing replies with a liveness message.
func cmdPing(cc CommandContext) error {
	cc.Reply("pong — daimon is alive")
	return nil
}

// cmdHelp lists all registered commands grouped by source.
// Output sections: "Built-in commands:", "Cron commands:", "Skill commands:".
// Empty sections are omitted. Within each section commands are alphabetically ordered.
func cmdHelp(cc CommandContext) error {
	all := cc.Registry.EntriesWithSource()

	// Bucket commands by source.
	bySource := map[string][]CommandEntryInfo{
		SourceBuiltin: {},
		SourceCron:    {},
		SourceSkill:   {},
	}
	for _, e := range all {
		bySource[e.Source] = append(bySource[e.Source], e)
	}

	// Sort each bucket alphabetically.
	for src := range bySource {
		bucket := bySource[src]
		sort.Slice(bucket, func(i, j int) bool {
			return bucket[i].Name < bucket[j].Name
		})
		bySource[src] = bucket
	}

	var sb strings.Builder
	sections := []struct {
		source  string
		heading string
	}{
		{SourceBuiltin, "Built-in commands:"},
		{SourceCron, "Cron commands:"},
		{SourceSkill, "Skill commands:"},
	}
	for _, sec := range sections {
		bucket := bySource[sec.source]
		if len(bucket) == 0 {
			continue
		}
		sb.WriteString(sec.heading + "\n")
		for _, e := range bucket {
			sb.WriteString(fmt.Sprintf("  /%s — %s\n", e.Name, e.Desc))
		}
	}
	cc.Reply(sb.String())
	return nil
}

// cmdReset clears the conversation history for the current channel.
func cmdReset(cc CommandContext) error {
	conv := store.Conversation{
		ID:        cc.ChannelID,
		ChannelID: cc.ChannelID,
		Messages:  nil,
	}
	if err := cc.Store.SaveConversation(cc.Ctx, conv); err != nil {
		return fmt.Errorf("failed to reset conversation: %w", err)
	}
	cc.Reply("Conversation cleared. Starting fresh.")
	return nil
}

// cmdStatus reports basic agent runtime information.
func cmdStatus(cc CommandContext) error {
	uptime := time.Since(cc.StartedAt).Round(time.Second)
	msg := fmt.Sprintf(
		"Agent Status:\n"+
			"  Provider: %s\n"+
			"  Channel: %s\n"+
			"  Uptime: %s\n"+
			"  Scope: %s",
		cc.ProviderName, cc.ChannelName, uptime, cc.ChannelID)
	cc.Reply(msg)
	return nil
}

// cmdWhoami reports the caller's identity.
func cmdWhoami(cc CommandContext) error {
	msg := fmt.Sprintf(
		"You are:\n"+
			"  Sender ID: %s\n"+
			"  Channel: %s\n"+
			"  Scope: %s",
		cc.SenderID, cc.ChannelName, cc.ChannelID)
	cc.Reply(msg)
	return nil
}

// cmdRetry re-enqueues the last user message for a fresh LLM response.
// It trims the conversation back to just before the last user turn and sends
// the message text as a synthetic IncomingMessage.
func cmdRetry(cc CommandContext) error {
	conv, err := cc.Store.LoadConversation(cc.Ctx, cc.ChannelID)
	if err != nil {
		return fmt.Errorf("failed to load conversation: %w", err)
	}

	// Find the index of the last user message.
	lastUserIdx := -1
	for i := len(conv.Messages) - 1; i >= 0; i-- {
		if conv.Messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx < 0 {
		cc.Reply("Nothing to retry.")
		return nil
	}

	lastUserMsg := conv.Messages[lastUserIdx]
	lastText := lastUserMsg.Content.TextOnly()

	// Trim history: remove the last user turn and everything after it.
	conv.Messages = conv.Messages[:lastUserIdx]
	if err := cc.Store.SaveConversation(cc.Ctx, *conv); err != nil {
		return fmt.Errorf("failed to save trimmed conversation: %w", err)
	}

	// Build a synthetic IncomingMessage with the original text.
	synthetic := channel.IncomingMessage{
		ID:        fmt.Sprintf("retry-%d", time.Now().UnixNano()),
		ChannelID: cc.ChannelID,
		SenderID:  cc.SenderID,
		Content:   content.Blocks{{Type: content.BlockText, Text: lastText}},
		Timestamp: time.Now(),
	}

	select {
	case cc.Inbox <- synthetic:
		cc.Reply("Retrying: " + lastText)
	case <-cc.Ctx.Done():
		return cc.Ctx.Err()
	default:
		cc.Reply("Inbox full, cannot retry right now.")
	}
	return nil
}

// registerBuiltinCommands registers the built-in slash commands on the registry.
func registerBuiltinCommands(reg *CommandRegistry) {
	reg.Register("ping", "Check if the agent is alive", cmdPing, SourceBuiltin)
	reg.Register("help", "List available commands", cmdHelp, SourceBuiltin)
	reg.Register("reset", "Clear conversation history", cmdReset, SourceBuiltin)
	reg.Register("retry", "Re-send last message for a new response", cmdRetry, SourceBuiltin)
	reg.Register("status", "Show agent status", cmdStatus, SourceBuiltin)
	reg.Register("whoami", "Show your identity", cmdWhoami, SourceBuiltin)
}

// cmdCompact implements the /compact command: force-compacts the current
// conversation via the ContextManager and reports token counts.
func (a *Agent) cmdCompact(cc CommandContext) error {
	scope := userScope(cc.ChannelID, cc.SenderID)
	convID := "conv_" + scope

	conv, err := cc.Store.LoadConversation(cc.Ctx, convID)
	if err != nil || len(conv.Messages) == 0 {
		cc.Reply("Nothing to compact")
		return nil
	}

	// Build system prompt and tool defs for token estimation.
	var memories []store.MemoryEntry // skip memory search for /compact
	systemPrompt := a.buildSystemPrompt(memories, nil, "")
	toolDefs := a.buildToolDefs()

	// Count tokens before compaction.
	before := EstimateMessagesTokens(conv.Messages)

	// Force-compact.
	conv.Messages = a.contextMgr.ForceCompact(cc.Ctx, systemPrompt, toolDefs, conv.Messages)

	// Count tokens after compaction.
	after := EstimateMessagesTokens(conv.Messages)

	// Save the compacted conversation.
	conv.UpdatedAt = time.Now()
	if saveErr := cc.Store.SaveConversation(cc.Ctx, *conv); saveErr != nil {
		return fmt.Errorf("failed to save compacted conversation: %w", saveErr)
	}

	cc.Reply(fmt.Sprintf("Compacted: %d → %d tokens", before, after))
	return nil
}

// makeReply returns a function that sends a text reply to the given channel.
func (a *Agent) makeReply(ctx context.Context, channelID string) func(string) {
	return func(text string) {
		out := channel.OutgoingMessage{ChannelID: channelID, Text: text}
		if err := a.channel.Send(ctx, out); err != nil {
			slog.Error("failed to send command reply", "error", err)
		}
	}
}
