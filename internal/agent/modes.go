package agent

// modes.go — Mode definitions for the mode-system feature (change #4).
//
// Exports: ModeDefinition, AllowAllTools, ErrInvalidMode, LookupMode, ModeNames.
// Package-private: modes map, filterAllowedTools, isToolAllowed.
//
// Design references:
//   - AD-1: exported struct, package-private map
//   - AD-2: cache string only, resolve tuple on demand via LookupMode
//   - AD-5: filterAllowedTools has DIFFERENT semantics from filterParentTools
//           (subagent_manager.go:685): nil=all, empty=NONE, non-empty=subset.
//           Do NOT replace this with filterParentTools — see discovery #413.
//   - AD-11: ErrInvalidMode.Error() is contract-locked; tests assert exact wording.
//   - O-3: exact mode prompts from REQ-6 used verbatim as package-level constants.
//
// Spec coverage: REQ-3, REQ-6, REQ-7, REQ-8.

import (
	"errors"
	"fmt"

	"daimon/internal/provider"
)

// ---------------------------------------------------------------------------
// Error sentinels
// ---------------------------------------------------------------------------

// ErrInvalidMode is returned by LookupMode (and SetMode) when the name is not
// one of "plan", "build", or "review". Error string is contract-locked per AD-11.
// Callers MUST use errors.Is to detect this sentinel.
var ErrInvalidMode = errors.New("invalid mode name (expected: plan, build, review)")

// ---------------------------------------------------------------------------
// ModeDefinition
// ---------------------------------------------------------------------------

// AllowAllTools is the sentinel allowlist meaning "every registered tool is
// allowed in this mode". The nil value is chosen deliberately: it is the zero
// value for a []string field, so code that never sets ToolAllowlist gets
// all-tools-pass behaviour automatically. Documented constant for self-documenting
// call sites.
var AllowAllTools []string = nil

// ModeDefinition is the immutable (system prompt, tool allowlist) tuple that
// defines a mode's behaviour. Exported so tests and callers can construct
// synthetic modes for filterAllowedTools unit tests without needing to mutate
// the package-private map.
//
// Callers MUST NOT mutate the returned struct from LookupMode — it is a copy
// of a package-internal literal.
type ModeDefinition struct {
	// Name is the canonical mode identifier: "plan" | "build" | "review".
	Name string
	// SystemPrompt is appended to a.config.Personality in buildSystemPrompt.
	// Empty string means no extra injection (build mode).
	SystemPrompt string
	// ToolAllowlist controls which tools are exposed to the model and which can
	// execute. Semantics:
	//   nil         → AllowAllTools: every tool passes (build mode default).
	//   []string{}  → NONE: all tools blocked (use case: future read-only posture).
	//   non-empty   → only the listed tool names are allowed.
	ToolAllowlist []string
}

// ---------------------------------------------------------------------------
// Mode prompt text (REQ-6, O-3 — verbatim from spec)
// ---------------------------------------------------------------------------

const planPrompt = `You are in PLAN mode. Your job: read, analyze, and propose. You MUST NOT modify files, execute shell commands, or take any action with side effects. Use Read, Grep, Glob, codegraph_*, and web tools to understand the codebase. When you have a plan, present it to the user and STOP. Wait for explicit approval before suggesting next steps. If the user asks you to implement, respond: "I'm in plan mode — switch to /mode build to implement."`

const buildPrompt = `` // build mode: no extra prompt injection (spec REQ-6 S6-2)

const reviewPrompt = `You are in REVIEW mode. Your job: audit existing code, diffs, and behavior. You may read files and run read-only git commands (git diff, git log, git show, git status). You MUST NOT modify files or execute mutating shell commands. Produce findings with severity (CRITICAL / WARNING / SUGGESTION). When asked to fix something, respond: "I'm in review mode — switch to /mode build to apply changes."`

// ---------------------------------------------------------------------------
// Tool allowlists (REQ-7)
// ---------------------------------------------------------------------------

// planAllowlist is the set of tool names permitted in "plan" mode.
// read-only analysis and codegraph tools only. No file-mutating or shell tools.
var planAllowlist = []string{
	"Read",
	"Grep",
	"Glob",
	"WebFetch",
	"WebSearch",
	// mem_* tools (prefix-based; listed by canonical names):
	"mem_save",
	"mem_search",
	"mem_get_observation",
	"mem_context",
	"mem_update",
	"mem_suggest_topic_key",
	"mem_session_start",
	"mem_session_end",
	"mem_session_summary",
	"mem_stats",
	"mem_delete",
	"mem_timeline",
	"mem_capture_passive",
	"mem_merge_projects",
	"mem_current_project",
	"mem_save_prompt",
	// codegraph_* tools:
	"codegraph_search",
	"codegraph_context",
	"codegraph_callers",
	"codegraph_callees",
	"codegraph_impact",
	"codegraph_node",
	"codegraph_explore",
	"codegraph_files",
	"codegraph_status",
}

// reviewAllowlist is planAllowlist plus Bash (name-level only; argument-level
// restriction deferred to change #6 per spec gap note in REQ-7).
var reviewAllowlist = append(append([]string{}, planAllowlist...), "Bash")

// ---------------------------------------------------------------------------
// Mode definitions table (package-private)
// ---------------------------------------------------------------------------

// modes is the package-private definition table. Tests MUST reference entries
// via LookupMode rather than reading the map directly to prevent test mutation.
var modes = map[string]ModeDefinition{
	"plan": {
		Name:          "plan",
		SystemPrompt:  planPrompt,
		ToolAllowlist: planAllowlist,
	},
	"build": {
		Name:          "build",
		SystemPrompt:  buildPrompt,
		ToolAllowlist: AllowAllTools,
	},
	"review": {
		Name:          "review",
		SystemPrompt:  reviewPrompt,
		ToolAllowlist: reviewAllowlist,
	},
}

// ---------------------------------------------------------------------------
// LookupMode
// ---------------------------------------------------------------------------

// LookupMode returns the ModeDefinition for name. Returns a wrapped ErrInvalidMode
// if the name is not a recognized mode. Callers MUST NOT mutate the returned
// struct (it is a copy of package-internal literals, but ToolAllowlist is a
// shared backing array — treat as read-only).
func LookupMode(name string) (ModeDefinition, error) {
	m, ok := modes[name]
	if !ok {
		return ModeDefinition{}, fmt.Errorf("%w: %q; available: plan, build, review", ErrInvalidMode, name)
	}
	return m, nil
}

// ModeNames returns the stable sorted list of valid mode names. Used by the
// /mode no-args list command to enumerate modes in a predictable order.
func ModeNames() []string {
	return []string{"build", "plan", "review"}
}

// ---------------------------------------------------------------------------
// filterAllowedTools (AD-5)
// ---------------------------------------------------------------------------

// filterAllowedTools returns the subset of tools whose Name is in allowlist.
//
// Semantics (intentionally DIFFERENT from filterParentTools in subagent_manager.go):
//
//	allowlist == nil         → return tools unchanged (all allowed, AllowAllTools)
//	len(allowlist) == 0      → return empty slice (NONE allowed)
//	len(allowlist) > 0       → return only ToolDefinitions whose Name appears in allowlist
//
// IMPORTANT: do NOT replace with filterParentTools. That helper treats
// empty allowlist as "inherit all" — the exact opposite of mode semantics.
// See discovery #413 in engram.
//
// Hot-path: O(N*M) where N=len(tools), M=len(allowlist). Both are bounded by
// the registered toolset (typically <50). Acceptable for once-per-turn use.
func filterAllowedTools(tools []provider.ToolDefinition, allowlist []string) []provider.ToolDefinition {
	if allowlist == nil {
		return tools
	}
	if len(allowlist) == 0 {
		return []provider.ToolDefinition{}
	}
	set := make(map[string]struct{}, len(allowlist))
	for _, n := range allowlist {
		set[n] = struct{}{}
	}
	out := make([]provider.ToolDefinition, 0, len(allowlist))
	for _, td := range tools {
		if _, ok := set[td.Name]; ok {
			out = append(out, td)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// isToolAllowed (AD-6)
// ---------------------------------------------------------------------------

// isToolAllowed reports whether toolName passes the allowlist check.
//
//	nil allowlist   → all allowed (AllowAllTools)
//	empty allowlist → none allowed
//	non-empty       → membership check
//
// Used by the execution gate at loop.go to reject tool calls not in the
// modeSnap's allowlist (AD-6).
func isToolAllowed(toolName string, allowlist []string) bool {
	if allowlist == nil {
		return true
	}
	for _, n := range allowlist {
		if n == toolName {
			return true
		}
	}
	return false
}
