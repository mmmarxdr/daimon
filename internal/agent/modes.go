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
	// ArgAllowlists gates tool ARGUMENTS keyed by tool name. Values are allowed
	// command prefixes (1-2 leading whitespace-split tokens). Semantics:
	//   nil map        → no arg restriction for ANY tool (plan/build default)
	//   no entry/tool  → no arg restriction for THAT tool
	//   entry present  → only commands whose leading tokens match are allowed,
	//                    and any shell metachar is rejected unconditionally.
	ArgAllowlists map[string][]string
}

// ---------------------------------------------------------------------------
// Mode prompt text (REQ-6, O-3 — verbatim from spec)
// ---------------------------------------------------------------------------

const planPrompt = `You are in PLAN mode. Your job: read, analyze, and propose. You MUST NOT take any EXTERNAL or IRREVERSIBLE action with side effects — no file writes, no shell commands, no API mutations. Maintaining your internal planning state (the todolist via todo_create / todo_update) is explicitly permitted: the todolist IS your planning artifact, not an external side effect. Use Read, Grep, Glob, codegraph_*, and web tools to understand the codebase. When you have a plan, present it and STOP; wait for explicit approval. If asked to implement, respond: "I'm in plan mode — switch to /mode build to implement."`

const buildPrompt = `` // build mode: no extra prompt injection (spec REQ-6 S6-2)

const reviewPrompt = `You are in REVIEW mode. Your job: audit existing code, diffs, and behavior. You may read files and run read-only git commands (git diff, git log, git show, git status, git blame). You MUST NOT modify files or execute mutating shell commands. Produce findings with severity (CRITICAL / WARNING / SUGGESTION). When asked to fix something, respond: "I'm in review mode — switch to /mode build to apply changes."`

// ---------------------------------------------------------------------------
// Tool allowlists (REQ-7)
// ---------------------------------------------------------------------------

// baseReadOnly is the set of read-only tools shared by BOTH plan and review modes.
// todo_list is read-only (no mutation, no event) so it lives here — both modes
// inherit it. (AD-3: extract shared base to prevent todo write-tool leakage into review.)
//
// WARNING: baseReadOnly, planAllowlist, and reviewAllowlist are all initialized
// once at package load via var-block append calls. Appending to baseReadOnly
// AFTER package init will NOT propagate into planAllowlist or reviewAllowlist —
// those were built from a copy at init time. Add new shared tools directly to
// the baseReadOnly literal below.
var baseReadOnly = []string{
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
	// todo_list is read-only: plan and review both get it via this shared base.
	"todo_list",
}

// planAllowlist is baseReadOnly plus the write todo tools. Plan mode may
// maintain its planning artifact (todo_create / todo_update) per REQ-8.
// Build from a copy so future appends don't mutate baseReadOnly's backing array.
var planAllowlist = append(append([]string{}, baseReadOnly...), "todo_create", "todo_update")

// reviewAllowlist is baseReadOnly plus shell_exec. Review is read-only: it inherits
// todo_list via baseReadOnly but NOT todo_create/todo_update (AD-3).
// The argument-level policy (isArgAllowed) further restricts shell_exec to
// read-only git commands (change #6, AD-5).
var reviewAllowlist = append(append([]string{}, baseReadOnly...), "shell_exec")

// reviewArgAllowlists is the argument-level allowlist for review mode.
// Only read-only git sub-commands are permitted for shell_exec.
// Indexed by tool name; each entry is a list of allowed leading-token prefixes
// (1-2 whitespace-split tokens — see isArgAllowed for matching semantics).
var reviewArgAllowlists = map[string][]string{
	"shell_exec": {"git diff", "git log", "git show", "git status", "git blame"},
}

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
		ArgAllowlists: reviewArgAllowlists,
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
