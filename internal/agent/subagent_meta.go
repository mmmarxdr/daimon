package agent

import "daimon/internal/store"

// mergeSubagentMeta returns base augmented with the 4 subagent attribution
// keys IFF conv is a subagent conversation (ParentConvID non-empty AND
// Metadata["subagent_id"] non-empty). Otherwise base is returned unmodified.
//
// The 4 canonical attribution keys (REQ-10) are:
//   - "subagent_id"    — the handle ID
//   - "parent_conv_id" — the parent conversation ID
//   - "batch_id"       — the batch ID from spawn (if present in Metadata)
//   - "skill"          — the skill name from spawn (if present in Metadata)
//
// For top-level conversations (ParentConvID == "") base is returned unchanged so
// that top-level events never carry the attribution keys (REQ-10.1).
//
// Invariant: SubagentManager.Spawn always populates conv.Metadata with
// "subagent_id", "batch_id", and "skill". The conditional checks below are
// defensive against future code paths that materialize subagent conversations
// outside the Spawn flow; under normal operation all 4 keys are always present.
//
// If base is nil and the function would add keys, a new map is allocated.
func mergeSubagentMeta(conv *store.Conversation, base map[string]string) map[string]string {
	if conv == nil || conv.ParentConvID == "" {
		return base
	}
	subID := conv.Metadata["subagent_id"]
	if subID == "" {
		return base
	}
	if base == nil {
		base = make(map[string]string)
	}
	base["subagent_id"] = subID
	base["parent_conv_id"] = conv.ParentConvID
	if v := conv.Metadata["batch_id"]; v != "" {
		base["batch_id"] = v
	}
	if v := conv.Metadata["skill"]; v != "" {
		base["skill"] = v
	}
	return base
}
