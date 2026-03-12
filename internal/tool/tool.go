package tool

import (
	"context"
	"encoding/json"
)

type ToolResult struct {
	Content string // text result returned to the LLM
	IsError bool   // if true, content is an error message
}

type Tool interface {
	Name() string
	Description() string
	Schema() json.RawMessage
	Execute(ctx context.Context, params json.RawMessage) (ToolResult, error)
}
