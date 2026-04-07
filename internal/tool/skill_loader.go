package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// SkillContent mirrors skill.SkillContent to avoid import cycle.
// Only the fields needed for loading are included.
type SkillContent struct {
	Name  string
	Prose string
}

// SkillLoaderTool implements Tool for on-demand skill prose loading.
type SkillLoaderTool struct {
	skills map[string]SkillContent
}

// NewSkillLoaderTool creates a SkillLoaderTool with the given skill index.
func NewSkillLoaderTool(skills map[string]SkillContent) *SkillLoaderTool {
	return &SkillLoaderTool{skills: skills}
}

func (t *SkillLoaderTool) Name() string { return "load_skill" }

func (t *SkillLoaderTool) Description() string {
	return "Load the full instructions for a skill by name. " +
		"Call this when you need detailed guidance from a skill listed in the Available Skills index. " +
		"Returns the complete skill prose as tool output."
}

func (t *SkillLoaderTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "required": ["name"],
  "properties": {
    "name": {
      "type": "string",
      "description": "The skill name to load (e.g. 'cron_scheduler', 'git-helper')"
    }
  }
}`)
}

type loadSkillParams struct {
	Name string `json:"name"`
}

func (t *SkillLoaderTool) Execute(_ context.Context, params json.RawMessage) (ToolResult, error) {
	var input loadSkillParams
	if err := json.Unmarshal(params, &input); err != nil {
		return ToolResult{IsError: true, Content: fmt.Sprintf("invalid parameters: %v", err)}, nil
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ToolResult{IsError: true, Content: "skill name cannot be empty"}, nil
	}

	sk, ok := t.skills[name]
	if !ok {
		available := make([]string, 0, len(t.skills))
		for k := range t.skills {
			available = append(available, k)
		}
		sort.Strings(available)
		return ToolResult{
			IsError: true,
			Content: fmt.Sprintf("unknown skill %q. Available skills: %s", name, strings.Join(available, ", ")),
		}, nil
	}

	if sk.Prose == "" {
		return ToolResult{Content: fmt.Sprintf("Skill %q has no prose content.", name)}, nil
	}

	return ToolResult{
		Content: fmt.Sprintf("## Skill: %s\n%s", sk.Name, sk.Prose),
	}, nil
}
