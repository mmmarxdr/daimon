package provider

import (
	"context"
	"encoding/json"
)

type ChatMessage struct {
	Role       string     `json:"role"` // "user", "assistant", "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ChatRequest struct {
	SystemPrompt string
	Messages     []ChatMessage
	Tools        []ToolDefinition
	MaxTokens    int
	Temperature  float64
}

type ChatResponse struct {
	Content    string     // text content (may be empty if only tool calls)
	ToolCalls  []ToolCall // tool calls to execute (may be empty if only text)
	Usage      UsageStats
	StopReason string // "end_turn", "tool_use", "max_tokens"
}

type UsageStats struct {
	InputTokens  int
	OutputTokens int
}

type Provider interface {
	Name() string
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	SupportsTools() bool
}
