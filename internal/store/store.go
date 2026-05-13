package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"daimon/internal/provider"
)

// ErrNotFound is returned when a requested conversation does not exist.
var ErrNotFound = errors.New("not found")

// ErrNameConflict is returned by CreateUserSkill when the name already exists
// in the user_skills table (UNIQUE constraint violation).
var ErrNameConflict = errors.New("name conflict")

// ErrEncryptionKeyNotConfigured is returned when a SecretsStore method is called
// but no encryption key has been configured via store.encryption_key or DAIMON_SECRET_KEY.
var ErrEncryptionKeyNotConfigured = errors.New("encryption key not configured")

// ErrInvalidTitle is returned by UpdateConversationTitle when the title is
// empty after trimming. The 1..100 rune bound is enforced at the web-layer
// validator; this sentinel covers the minimum viable invariant at the
// store layer so nothing silently writes empty titles.
var ErrInvalidTitle = errors.New("invalid title")

type Conversation struct {
	ID        string                 `json:"id"`
	ChannelID string                 `json:"channel_id"`
	Messages  []provider.ChatMessage `json:"messages"`
	Metadata  map[string]string      `json:"metadata,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`

	// CompactedAt is non-nil when the conversation has been compacted by the
	// background ConversationCompactor: tool_outputs were summarised into
	// CompactedSummary and the raw rows deleted. New activity in this conv
	// will accumulate fresh outputs that will eventually be re-compacted.
	CompactedAt *time.Time `json:"compacted_at,omitempty"`

	// CompactedSummary holds the LLM-generated summary of the conversation's
	// tool work and key findings. Injected into the system prompt as
	// "## Previous session summary" when the conversation is resumed.
	CompactedSummary string `json:"compacted_summary,omitempty"`

	// ParentConvID, when non-empty, identifies the principal conversation that
	// spawned this sub-conversation. NULL / empty for root (principal) convs.
	// Added in migration v16.
	ParentConvID string `json:"parent_conv_id,omitempty"`

	// Status tracks the conversation lifecycle. Valid values: "active",
	// "running", "completed", "failed", "cancelled". Defaults to "active".
	// Added in migration v16.
	Status string `json:"status,omitempty"`
}

// MemoryEntry represents a single persisted memory item.
type MemoryEntry struct {
	ID        string    `json:"id"`
	ScopeID   string    `json:"scope_id"`
	Topic     string    `json:"topic,omitempty"`
	Type      string    `json:"type,omitempty"`
	Title     string    `json:"title,omitempty"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	Source    string    `json:"source"` // conversation ID
	CreatedAt time.Time `json:"created_at"`

	// Fields added in schema v2 (Layer 1 migration).
	// Zero values are valid defaults for entries created before this migration.
	AccessCount    int        `json:"access_count,omitempty"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`

	// Importance is a 1–10 score assigned by the Curator during classification.
	// Default value is 5 (neutral). Added in schema v8.
	Importance int `json:"importance"`

	// Cluster groups memories into high-level buckets for UI organization:
	// identity, preferences, projects, relationships, technical, general.
	// Assigned by the Curator alongside Importance. Default 'general'. Added in v11.
	Cluster string `json:"cluster,omitempty"`

	// Embedding stores a 256-dimensional float32 vector serialized as
	// little-endian binary (1,024 bytes). Added in schema v3.
	// Not serialized to JSON — internal transport only.
	Embedding []byte `json:"-"`
}

// Store is the primary persistence interface for conversations and memory.
type Store interface {
	SaveConversation(ctx context.Context, conv Conversation) error
	LoadConversation(ctx context.Context, id string) (*Conversation, error)
	ListConversations(ctx context.Context, channelID string, limit int) ([]Conversation, error)
	AppendMemory(ctx context.Context, scopeID string, entry MemoryEntry) error
	SearchMemory(ctx context.Context, scopeID string, query string, limit int) ([]MemoryEntry, error)

	// UpdateMemory updates the topic, type, title, tags, and content of an
	// existing memory entry identified by entry.ID within scopeID.
	// FileStore implements this as a no-op (returns nil).
	UpdateMemory(ctx context.Context, scopeID string, entry MemoryEntry) error

	// ListChildConversations returns every conversation whose parent_conv_id
	// equals parentConvID, ordered by created_at ASC. Returns an empty slice
	// (not an error) when none exist.
	ListChildConversations(ctx context.Context, parentConvID string) ([]Conversation, error)

	// SetConversationStatus updates conversations.status for the given convID.
	// Valid values: "active", "running", "completed", "failed", "cancelled".
	// Returns ErrNotFound if the convID does not exist.
	// Returns an error if status is not one of the valid values.
	SetConversationStatus(ctx context.Context, convID, status string) error

	// ListUserSkills returns all user_skills rows ordered by name ASC.
	// FileStore returns an empty slice (no-op). SQLiteStore queries user_skills.
	ListUserSkills(ctx context.Context) ([]UserSkill, error)

	// GetUserSkill returns the user_skill row with the given name.
	// Returns ErrNotFound when no row matches.
	GetUserSkill(ctx context.Context, name string) (UserSkill, error)

	// CreateUserSkill inserts a new user_skill row.
	// Returns ErrNameConflict when a row with the same name already exists.
	CreateUserSkill(ctx context.Context, skill UserSkill) (UserSkill, error)

	// UpdateUserSkill replaces all mutable fields on the row identified by
	// skill.Name. Returns ErrNotFound when no matching row exists.
	UpdateUserSkill(ctx context.Context, skill UserSkill) (UserSkill, error)

	// DeleteUserSkill removes the row with the given name.
	// Returns ErrNotFound when no row matches.
	DeleteUserSkill(ctx context.Context, name string) error

	Close() error
}

// CronJob represents a scheduled recurring task.
type CronJob struct {
	ID            string
	Schedule      string // 5-field cron expression
	ScheduleHuman string // human-readable description
	Prompt        string
	ChannelID     string
	Description   string
	Enabled       bool
	CreatedAt     time.Time
	LastRunAt     *time.Time
	NextRunAt     *time.Time
	NotifyChannel      string `json:"notify_channel"`       // per-job notification channel override; empty = use rule default
	NotifyOnCompletion bool   `json:"notify_on_completion"` // opt-in echo-back without needing a rule
}

// CronResult is the output of a single cron job execution.
type CronResult struct {
	ID       string
	JobID    string
	RanAt    time.Time
	Output   string
	ErrorMsg string
}

// CronStore is an optional extension to Store for scheduling support.
// Only SQLiteStore implements this; FileStore does not.
type CronStore interface {
	CreateJob(ctx context.Context, job CronJob) (CronJob, error)
	ListJobs(ctx context.Context) ([]CronJob, error)
	GetJob(ctx context.Context, id string) (CronJob, error)
	DeleteJob(ctx context.Context, id string) error
	SaveResult(ctx context.Context, result CronResult) error
	ListResults(ctx context.Context, jobID string, limit int) ([]CronResult, error)
	PruneResults(ctx context.Context, retentionDays, maxPerJob int) error
	CountResults(ctx context.Context, jobID string) (int, error)

	// UpdateJobRunTimes sets last_run_at and next_run_at for a cron job.
	// Best-effort: called after each job fire. No-op if job is absent.
	UpdateJobRunTimes(ctx context.Context, id string, lastRunAt, nextRunAt time.Time) error
}

// WebStore is an optional extension of Store for web dashboard operations.
// Only SQLiteStore implements this interface. Callers type-assert:
//
//	ws, ok := myStore.(store.WebStore)
type WebStore interface {
	// ListConversationsPaginated returns conversations filtered by channelID prefix
	// (or all if empty), ordered by updated_at descending, with pagination.
	// Returns the page slice, total count across all pages, and any error.
	ListConversationsPaginated(ctx context.Context, channelID string, limit, offset int) ([]Conversation, int, error)

	// CountConversations returns the total number of conversations, optionally
	// filtered by channelID prefix. Pass "" for all channels.
	CountConversations(ctx context.Context, channelID string) (int, error)

	// DeleteConversation performs a SOFT delete — sets deleted_at on the row.
	// Returns ErrNotFound (wrapped) if no conversation with that ID exists.
	// No-op (returns nil, not an error) when the conv is already soft-deleted.
	DeleteConversation(ctx context.Context, scopeID string) error

	// RestoreConversation clears deleted_at on a previously soft-deleted conv.
	// Returns ErrNotFound (wrapped) if the conv does not exist OR is already
	// live (two cases with identical observable behavior for the caller).
	RestoreConversation(ctx context.Context, scopeID string) error

	// DeleteConversationsOlderThan physically removes conversations that were
	// soft-deleted before cutoff. Returns the number of rows removed.
	// Intended for the background ConversationPruner.
	DeleteConversationsOlderThan(ctx context.Context, cutoff time.Time) (int, error)

	// GetConversationMessages returns a window of messages from a single
	// conversation without having to load and serialize the entire blob.
	// `beforeIndex = -1` (or any value >= total) means "load the most recent
	// `limit` messages". `limit` is clamped to [1, 200]; 0 → 50.
	// Returns: the window slice (defensive copy), hasMore=true when
	// oldestIndex > 0, oldestIndex is the absolute index of the first
	// returned message (useful as the next cursor for paging upward).
	// Returns ErrNotFound on a missing or soft-deleted conv.
	GetConversationMessages(ctx context.Context, id string, beforeIndex, limit int) ([]provider.ChatMessage, bool, int, error)

	// UpdateConversationTitle sets metadata["title"] for a conversation. The
	// title is validated by the caller (1..100 runes, newlines stripped).
	// Returns ErrNotFound if the conv is missing or soft-deleted.
	UpdateConversationTitle(ctx context.Context, id string, title string) error

	// DeleteMemory removes a single memory entry by its rowid within scopeID.
	// Returns ErrNotFound (wrapped) if no matching entry exists.
	DeleteMemory(ctx context.Context, scopeID string, entryID int64) error
}

// CostRecord represents a single LLM call cost record.
type CostRecord struct {
	ID            string
	SessionID     string
	ChannelID     string
	Model         string
	InputTokens   int
	OutputTokens  int
	InputCostUSD  float64
	OutputCostUSD float64
	TotalCostUSD  float64
	Timestamp     time.Time

	// Fields added in migration v17 for subagent cost attribution.

	// ConvID links this cost record to a specific conversation (distinct from
	// SessionID which is a legacy alias). Added in migration v17.
	ConvID string `json:"conv_id,omitempty"`

	// ParentConvID is set when this cost record belongs to a subagent
	// conversation; it points to the principal conversation. Added in v17.
	ParentConvID string `json:"parent_conv_id,omitempty"`

	// AttributionKind describes how this cost is attributed. "self" means the
	// cost was incurred directly by this conversation's LLM calls. Added in v17.
	AttributionKind string `json:"attribution_kind,omitempty"`
}

// CostFilter allows filtering cost records by dimension.
type CostFilter struct {
	SessionID string
	ChannelID string
	Model     string
	Since     time.Time
	Until     time.Time
}

// CostModelCost represents aggregated costs for a single model.
type CostModelCost struct {
	Model        string
	InputTokens  int
	OutputTokens int
	TotalCostUSD float64
	CallCount    int
}

// CostSummary represents aggregated cost data across records.
type CostSummary struct {
	TotalInputTokens  int
	TotalOutputTokens int
	TotalCostUSD      float64
	RecordCount       int
	ByModel           []CostModelCost

	// ConversationCount is the number of distinct conversations included in
	// this summary (populated by CostSummaryForTree).
	ConversationCount int
}

// DailyCost is one calendar day worth of aggregated LLM-call cost data.
// Used by the metrics endpoint to drive the dashboard charts.
// Conversations is the number of distinct sessions that exchanged at least
// one LLM call that day; Messages is the raw count of LLM calls (a turn that
// fans out to N tool-iterations contributes N).
type DailyCost struct {
	Date          string  // "2026-04-12"
	InputTokens   int64
	OutputTokens  int64
	TotalCostUSD  float64
	Conversations int
	Messages      int
}

// CostStore is an optional extension for cost tracking.
// Only SQLiteStore implements this; callers type-assert.
type CostStore interface {
	RecordCost(ctx context.Context, record CostRecord) error
	GetCostSummary(ctx context.Context, filter CostFilter) (CostSummary, error)
	// GetDailyCostHistory returns one DailyCost per calendar day (UTC) for
	// the last `days` days inclusive of today. Missing days are zero-filled
	// so charts have a continuous x-axis. The last entry in the returned
	// slice is always today.
	GetDailyCostHistory(ctx context.Context, days int) ([]DailyCost, error)
	// GetLastCallTokens returns the input_tokens and model of the most
	// recently recorded LLM call across all conversations. Returns
	// (0, "", nil) when no records exist (not an error). Used by the
	// sidebar's "last turn context" indicator.
	GetLastCallTokens(ctx context.Context) (inputTokens int64, model string, err error)
	// CostSummaryForTree returns aggregated cost for rootConvID and all of
	// its descendant conversations (via parent_conv_id). ConversationCount
	// reflects the number of distinct conversations in the tree.
	CostSummaryForTree(ctx context.Context, rootConvID string) (CostSummary, error)
}

// BudgetJSON is the wire/DB representation of a skill's execution budget.
// TimeoutMin stores minutes (integer) so the DB column is human-readable.
// Converted to BudgetConfig at runtime via skill.userSkillToParts.
type BudgetJSON struct {
	MaxCostUSD float64 `json:"max_cost_usd"`
	MaxTurns   int     `json:"max_turns"`
	TimeoutMin int     `json:"timeout_min"`
}

// UserSkill mirrors a single user_skills row 1:1.
// Budget and ToolsAllowlist use pointer / slice semantics to distinguish
// NULL (inherit / unlimited) from empty (no tools / zero budget).
type UserSkill struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Description    string      `json:"description"`
	Prose          string      `json:"prose"`
	Executable     bool        `json:"executable"`
	Model          string      `json:"model"`
	Provider       string      `json:"provider"`
	ToolsAllowlist []string    `json:"tools_allowlist"`  // nil = inherit all; []string{} = no tools
	Budget         *BudgetJSON `json:"budget,omitempty"` // nil = unlimited
	Version        int         `json:"version"`
	Source         string      `json:"source"` // "user" for rows inserted via REST
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// encodeAllowlist converts a []string to a sql.NullString JSON blob.
// nil input → NULL (Valid=false). Empty or non-empty slice → JSON string.
func encodeAllowlist(v []string) sql.NullString {
	if v == nil {
		return sql.NullString{}
	}
	data, _ := json.Marshal(v)
	return sql.NullString{String: string(data), Valid: true}
}

// decodeAllowlist converts a sql.NullString JSON blob back to []string.
// NULL (Valid=false) → nil. Valid JSON → parsed slice (may be empty non-nil).
func decodeAllowlist(ns sql.NullString) []string {
	if !ns.Valid {
		return nil
	}
	var v []string
	if err := json.Unmarshal([]byte(ns.String), &v); err != nil {
		return nil
	}
	return v
}

// encodeBudget converts a *BudgetJSON to a sql.NullString JSON blob.
// nil input → NULL (Valid=false).
func encodeBudget(b *BudgetJSON) sql.NullString {
	if b == nil {
		return sql.NullString{}
	}
	data, _ := json.Marshal(b)
	return sql.NullString{String: string(data), Valid: true}
}

// decodeBudget converts a sql.NullString JSON blob back to *BudgetJSON.
// NULL (Valid=false) → nil.
func decodeBudget(ns sql.NullString) *BudgetJSON {
	if !ns.Valid {
		return nil
	}
	var b BudgetJSON
	if err := json.Unmarshal([]byte(ns.String), &b); err != nil {
		return nil
	}
	return &b
}

// UserSkillStore is an optional extension of Store for user-defined skill CRUD.
// Only *SQLiteStore implements this; FileStore does not.
// Callers type-assert:
//
//	uss, ok := myStore.(store.UserSkillStore)
type UserSkillStore interface {
	// ListUserSkills returns all user_skills rows ordered by name ASC.
	// Returns an empty non-nil slice when no rows exist.
	ListUserSkills(ctx context.Context) ([]UserSkill, error)

	// GetUserSkill returns the row with the given name.
	// Returns ErrNotFound (wrapped) when no row matches.
	GetUserSkill(ctx context.Context, name string) (UserSkill, error)

	// CreateUserSkill inserts a new row. Returns ErrNameConflict (wrapped)
	// when a row with the same name already exists.
	CreateUserSkill(ctx context.Context, skill UserSkill) (UserSkill, error)

	// UpdateUserSkill replaces all mutable fields on the row identified by
	// skill.Name. updated_at is set to now(). Returns ErrNotFound (wrapped)
	// when no matching row exists.
	UpdateUserSkill(ctx context.Context, skill UserSkill) (UserSkill, error)

	// DeleteUserSkill removes the row with the given name. Returns ErrNotFound
	// (wrapped) when no row matches.
	DeleteUserSkill(ctx context.Context, name string) error
}

// SecretsStore is an optional extension of Store for encrypted key-value secrets.
// Only SQLiteStore implements this interface. Callers type-assert:
//
//	ss, ok := myStore.(store.SecretsStore)
type SecretsStore interface {
	// GetSecret retrieves and decrypts the secret for key.
	// Returns ErrNotFound (wrapped) if key does not exist.
	// Returns ErrEncryptionKeyNotConfigured if no key is configured.
	GetSecret(ctx context.Context, key string) (string, error)

	// SetSecret encrypts and persists value under key (upsert semantics).
	// Returns ErrEncryptionKeyNotConfigured if no key is configured.
	// Returns an error if key is empty.
	SetSecret(ctx context.Context, key string, value string) error

	// DeleteSecret removes the secret for key. Idempotent — no error if key is absent.
	DeleteSecret(ctx context.Context, key string) error

	// ListSecretKeys returns all stored secret key names (never values).
	// Returns an empty non-nil slice if no secrets exist.
	ListSecretKeys(ctx context.Context) ([]string, error)
}
