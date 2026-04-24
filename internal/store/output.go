package store

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors returned by IndexOutput when required fields are missing.
var (
	ErrOutputMissingID       = errors.New("ToolOutput.ID is required")
	ErrOutputMissingToolName = errors.New("ToolOutput.ToolName is required")
)

// ToolOutput represents the output of a tool execution for indexing and search.
type ToolOutput struct {
	ID        string    `json:"id"`
	ToolName  string    `json:"tool_name"`
	Command   string    `json:"command,omitempty"`
	Content   string    `json:"content"`
	Truncated bool      `json:"truncated"`
	ExitCode  int       `json:"exit_code"`
	Timestamp time.Time `json:"timestamp"`

	// ConversationID scopes the output to a single conversation so the
	// compactor can find and delete it once the conversation is summarised.
	// Empty string for outputs not bound to a conversation (e.g. cron jobs).
	ConversationID string `json:"conversation_id,omitempty"`
}

// OutputStore is an interface for indexing and searching tool outputs.
// This extends the Store interface with output-specific operations.
type OutputStore interface {
	// IndexOutput stores a tool output for later search.
	IndexOutput(ctx context.Context, output ToolOutput) error

	// SearchOutputs searches indexed tool outputs using FTS5.
	// Returns matching outputs sorted by relevance.
	SearchOutputs(ctx context.Context, query string, limit int) ([]ToolOutput, error)
}
