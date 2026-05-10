package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"daimon/internal/skill"
	"daimon/internal/tool"
)

// spawnCaller is the interface SubagentSpawnTool uses to spawn a child.
// Satisfied by *SubagentManager; also lets tests inject a fake.
type spawnCaller interface {
	Spawn(ctx context.Context, def skill.ExecutableSkillDef, prompt string, mode SpawnMode, callerConvID string) (*SubagentHandle, error)
}

// SubagentSpawnTool implements tool.Tool. One instance is registered in a.tools
// per executable skill at agent.New() time. When the LLM emits a tool call
// targeting its Name(), the parent loop calls Execute which drives Spawn and
// (in sync mode) blocks until the subagent completes.
type SubagentSpawnTool struct {
	def     skill.ExecutableSkillDef
	manager spawnCaller
}

// Compile-time assertion: SubagentSpawnTool must implement tool.Tool.
var _ tool.Tool = (*SubagentSpawnTool)(nil)

// Name returns the skill name — this is what the LLM uses as the tool name.
func (t *SubagentSpawnTool) Name() string { return t.def.Name }

// Description returns the skill description (≤200 chars from frontmatter).
func (t *SubagentSpawnTool) Description() string { return t.def.Description }

// Schema returns the JSON Schema for the spawn tool parameters.
// The schema is static — every SubagentSpawnTool accepts the same two params
// regardless of which skill it wraps.
func (t *SubagentSpawnTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "prompt": {
      "type": "string",
      "description": "Task description for the subagent"
    },
    "mode": {
      "type": "string",
      "enum": ["sync", "async"],
      "default": "sync",
      "description": "sync blocks parent until subagent finishes; async returns a handle immediately"
    }
  },
  "required": ["prompt"]
}`)
}

// Execute implements tool.Tool. It spawns a child agent for this skill.
// In sync mode it blocks until the child completes and returns SubagentResult JSON.
// In async mode it returns the handle ID immediately.
func (t *SubagentSpawnTool) Execute(ctx context.Context, params json.RawMessage) (tool.ToolResult, error) {
	var p struct {
		Prompt string `json:"prompt"`
		Mode   string `json:"mode,omitempty"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return tool.ToolResult{IsError: true, Content: "invalid params: " + err.Error()}, nil
	}
	if strings.TrimSpace(p.Prompt) == "" {
		return tool.ToolResult{IsError: true, Content: "prompt is required"}, nil
	}

	mode := SpawnMode(p.Mode)
	if mode == "" {
		mode = SpawnModeSync
	}

	// Extract the caller's conversation ID from context (used for depth guard).
	// ConvIDFromContext returns "" when not set (fresh context); that is safe —
	// depth guard only blocks when the convID is explicitly registered as a sub.
	callerConvID := tool.ConvIDFromContext(ctx)

	handle, err := t.manager.Spawn(ctx, t.def, p.Prompt, mode, callerConvID)
	if err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}

	if mode == SpawnModeAsync {
		payload, _ := json.Marshal(map[string]any{
			"handle_id": handle.ID,
			"batch_id":  handle.BatchID,
			"status":    "running",
		})
		return tool.ToolResult{
			Content: string(payload),
			Meta: map[string]string{
				"subagent_id": handle.ID,
				"batch_id":    handle.BatchID,
				"mode":        "async",
			},
		}, nil
	}

	// Sync: block on Wait. The parent's tool ctx already carries limits from
	// the agent config; Wait returns ctx.Err() on parent timeout.
	res, waitErr := handle.Wait(ctx)
	if waitErr != nil {
		return tool.ToolResult{IsError: true, Content: "subagent wait failed: " + waitErr.Error()}, nil
	}

	payload, _ := json.Marshal(res)
	isError := res.Status != "completed"
	return tool.ToolResult{
		Content: string(payload),
		IsError: isError,
		Meta: map[string]string{
			"subagent_id": handle.ID,
			"batch_id":    handle.BatchID,
			"mode":        "sync",
			"status":      res.Status,
			"cost_usd":    fmt.Sprintf("%.4f", res.Cost),
		},
	}, nil
}
