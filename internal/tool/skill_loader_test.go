package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TestSkillLoaderTool_Interface
// ---------------------------------------------------------------------------

func TestSkillLoaderTool_Interface(t *testing.T) {
	sl := NewSkillLoaderTool(map[string]SkillContent{
		"test": {Name: "test", Prose: "some prose"},
	})

	t.Run("Name returns load_skill", func(t *testing.T) {
		if sl.Name() != "load_skill" {
			t.Errorf("got %q, want %q", sl.Name(), "load_skill")
		}
	})

	t.Run("Description is non-empty", func(t *testing.T) {
		if sl.Description() == "" {
			t.Error("Description() returned empty string")
		}
	})

	t.Run("Schema is valid JSON", func(t *testing.T) {
		schema := sl.Schema()
		if !json.Valid(schema) {
			t.Errorf("Schema() is not valid JSON: %s", schema)
		}
		var m map[string]interface{}
		if err := json.Unmarshal(schema, &m); err != nil {
			t.Errorf("failed to parse schema: %v", err)
		}
		props, ok := m["properties"].(map[string]interface{})
		if !ok {
			t.Error("schema missing 'properties' key")
		} else if _, ok := props["name"]; !ok {
			t.Error("schema missing 'name' property")
		}
		req, ok := m["required"].([]interface{})
		if !ok {
			t.Error("schema missing 'required' key")
		} else {
			found := false
			for _, v := range req {
				if v == "name" {
					found = true
					break
				}
			}
			if !found {
				t.Error("'name' not listed in required")
			}
		}
	})
}

// ---------------------------------------------------------------------------
// TestSkillLoaderTool_Execute
// ---------------------------------------------------------------------------

func TestSkillLoaderTool_Execute(t *testing.T) {
	tests := []struct {
		name      string
		skills    map[string]SkillContent
		params    string
		wantIsErr bool
		wantStr   string
		wantGoErr bool
	}{
		{
			name: "valid name returns skill prose",
			skills: map[string]SkillContent{
				"cron": {Name: "cron", Prose: "This is the cron skill."},
			},
			params:    `{"name":"cron"}`,
			wantIsErr: false,
			wantStr:   "## Skill: cron\nThis is the cron skill.",
		},
		{
			name: "unknown name returns error with available skills",
			skills: map[string]SkillContent{
				"alpha": {Name: "alpha", Prose: "alpha prose"},
				"beta":  {Name: "beta", Prose: "beta prose"},
			},
			params:    `{"name":"gamma"}`,
			wantIsErr: true,
			wantStr:   "unknown skill",
		},
		{
			name: "empty name returns error",
			skills: map[string]SkillContent{
				"cron": {Name: "cron", Prose: "prose"},
			},
			params:    `{"name":""}`,
			wantIsErr: true,
			wantStr:   "skill name cannot be empty",
		},
		{
			name: "whitespace-only name returns error",
			skills: map[string]SkillContent{
				"cron": {Name: "cron", Prose: "prose"},
			},
			params:    `{"name":"   "}`,
			wantIsErr: true,
			wantStr:   "skill name cannot be empty",
		},
		{
			name: "empty prose returns info message",
			skills: map[string]SkillContent{
				"empty": {Name: "empty", Prose: ""},
			},
			params:    `{"name":"empty"}`,
			wantIsErr: false,
			wantStr:   "has no prose content",
		},
		{
			name:      "invalid JSON params returns ToolResult error",
			skills:    map[string]SkillContent{},
			params:    `{bad json`,
			wantIsErr: true,
			wantStr:   "invalid parameters",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sl := NewSkillLoaderTool(tc.skills)
			ctx := context.Background()
			result, err := sl.Execute(ctx, json.RawMessage(tc.params))
			if tc.wantGoErr {
				if err == nil {
					t.Error("expected Go error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected Go error: %v", err)
			}
			if result.IsError != tc.wantIsErr {
				t.Errorf("IsError = %v, want %v; content: %q", result.IsError, tc.wantIsErr, result.Content)
			}
			if tc.wantStr != "" && !strings.Contains(result.Content, tc.wantStr) {
				t.Errorf("content %q does not contain %q", result.Content, tc.wantStr)
			}
		})
	}
}

func TestSkillLoaderTool_UnknownSkillListsAvailable(t *testing.T) {
	skills := map[string]SkillContent{
		"zebra": {Name: "zebra", Prose: "z"},
		"alpha": {Name: "alpha", Prose: "a"},
		"mango": {Name: "mango", Prose: "m"},
	}
	sl := NewSkillLoaderTool(skills)
	ctx := context.Background()

	result, err := sl.Execute(ctx, json.RawMessage(`{"name":"nonexistent"}`))
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for unknown skill")
	}
	// Available skills should be sorted alphabetically
	if !strings.Contains(result.Content, "alpha, mango, zebra") {
		t.Errorf("content does not list available skills in sorted order: %q", result.Content)
	}
}
