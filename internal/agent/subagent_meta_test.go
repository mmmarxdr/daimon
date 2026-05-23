package agent

import (
	"testing"

	"daimon/internal/store"
)

// TestMergeSubagentMeta_TopLevel_NoKeys (REQ-10.1) — top-level conversation
// (no ParentConvID) returns base map unchanged.
func TestMergeSubagentMeta_TopLevel_NoKeys(t *testing.T) {
	conv := &store.Conversation{
		ID:           "conv-root",
		ParentConvID: "", // top-level
	}
	base := map[string]string{"conv_id": "conv-root"}
	got := mergeSubagentMeta(conv, base)

	if _, ok := got["subagent_id"]; ok {
		t.Error("top-level conv should not have subagent_id in meta")
	}
	if got["conv_id"] != "conv-root" {
		t.Errorf("base key conv_id lost: got %q", got["conv_id"])
	}
}

// TestMergeSubagentMeta_Subagent_AllFourKeys (REQ-10.2) — subagent conv carries
// all 4 attribution keys merged into base.
func TestMergeSubagentMeta_Subagent_AllFourKeys(t *testing.T) {
	conv := &store.Conversation{
		ID:           "sub-abc",
		ParentConvID: "conv-xyz",
		Metadata: map[string]string{
			"subagent_id": "sub-abc",
			"batch_id":    "batch-7",
			"skill":       "code-review",
		},
	}
	base := map[string]string{"conv_id": "sub-abc"}
	got := mergeSubagentMeta(conv, base)

	checks := map[string]string{
		"subagent_id":    "sub-abc",
		"parent_conv_id": "conv-xyz",
		"batch_id":       "batch-7",
		"skill":          "code-review",
		"conv_id":        "sub-abc",
	}
	for k, want := range checks {
		if got[k] != want {
			t.Errorf("key %q: got %q, want %q", k, got[k], want)
		}
	}
}

// TestMergeSubagentMeta_NilConv_Safe — nil conv returns base without panic.
func TestMergeSubagentMeta_NilConv_Safe(t *testing.T) {
	base := map[string]string{"x": "y"}
	got := mergeSubagentMeta(nil, base)
	if got["x"] != "y" {
		t.Errorf("base key lost: got %q", got["x"])
	}
	if _, ok := got["subagent_id"]; ok {
		t.Error("nil conv should not produce subagent_id")
	}
}

// TestMergeSubagentMeta_PartialMetadata_OnlyKnownKeys — conv has subagent_id
// but no batch_id → only present keys merged.
func TestMergeSubagentMeta_PartialMetadata_OnlyKnownKeys(t *testing.T) {
	conv := &store.Conversation{
		ID:           "sub-partial",
		ParentConvID: "parent-1",
		Metadata: map[string]string{
			"subagent_id": "sub-partial",
			// no batch_id, no skill
		},
	}
	got := mergeSubagentMeta(conv, nil)

	if got["subagent_id"] != "sub-partial" {
		t.Errorf("subagent_id: got %q, want sub-partial", got["subagent_id"])
	}
	if got["parent_conv_id"] != "parent-1" {
		t.Errorf("parent_conv_id: got %q, want parent-1", got["parent_conv_id"])
	}
	// batch_id and skill should be absent (empty string not inserted).
	if v, ok := got["batch_id"]; ok && v != "" {
		t.Errorf("batch_id should be absent when not in Metadata, got %q", v)
	}
	if v, ok := got["skill"]; ok && v != "" {
		t.Errorf("skill should be absent when not in Metadata, got %q", v)
	}
}
