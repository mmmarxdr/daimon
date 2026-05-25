package tool

// RiskLevel describes the risk posture of a tool call.
// Named string type so it marshals to human-readable JSON and provides
// compile-time guards against typos in Inspect() literals.
type RiskLevel string

const (
	RiskReadOnly    RiskLevel = "read-only"
	RiskSideEffects RiskLevel = "side-effects"
	RiskDestructive RiskLevel = "destructive"
)

// ToolCategory groups tools by the subsystem they operate on.
type ToolCategory string

const (
	CatShell      ToolCategory = "shell"
	CatFile       ToolCategory = "file"
	CatNetwork    ToolCategory = "network"
	CatMemory     ToolCategory = "memory"
	CatScheduling ToolCategory = "scheduling"
	CatKnowledge  ToolCategory = "knowledge"
	CatMeta       ToolCategory = "meta"
	CatMCP        ToolCategory = "mcp"
	CatUnknown    ToolCategory = "unknown"
)

// PermissionLevel is DESCRIPTIVE inspector metadata ONLY — NOT enforced anywhere.
// A tool tagged PermAdmin executes identically to one tagged PermNone when called
// through the normal agent/tool invocation path.
type PermissionLevel string

const (
	PermNone  PermissionLevel = "none"
	PermUser  PermissionLevel = "user"
	PermAdmin PermissionLevel = "admin"
)

// ToolSource identifies where the tool originates.
type ToolSource string

const (
	SourceBuiltin ToolSource = "builtin"
	SourceMCP     ToolSource = "mcp"
	SourceSkill   ToolSource = "skill"
)

// ToolMeta carries the descriptive metadata for a single tool. All fields are
// string-based so they serialize directly to human-readable JSON without any
// mapping layer. The TUI (#9) renders them as-is.
type ToolMeta struct {
	Risk       RiskLevel       `json:"risk"`
	Category   ToolCategory    `json:"category"`
	Permission PermissionLevel `json:"permission"`
	Source     ToolSource      `json:"source"`
}

// ToolInspector is an OPTIONAL interface. Tools that implement it provide their
// own descriptive metadata. Tools that do NOT implement it receive safe defaults
// via BuildToolMeta. The tool.Tool interface is intentionally NOT extended.
type ToolInspector interface {
	Inspect() ToolMeta
}

// defaultToolMeta is the safe fallback applied to any tool that does not
// implement ToolInspector. Risk is side-effects (never under-state) and
// Category is unknown (no subsystem claim).
var defaultToolMeta = ToolMeta{
	Risk:       RiskSideEffects,
	Category:   CatUnknown,
	Permission: PermNone,
	Source:     SourceBuiltin,
}

// BuildToolMeta derives a ToolMeta for every tool in registry. For tools that
// implement ToolInspector, it calls Inspect() and uses the result. For all
// others, it applies defaultToolMeta. The function is pure: it has no side
// effects and the returned map is freshly allocated on each call.
func BuildToolMeta(registry map[string]Tool) map[string]ToolMeta {
	result := make(map[string]ToolMeta, len(registry))
	for name, t := range registry {
		if inspector, ok := t.(ToolInspector); ok {
			result[name] = inspector.Inspect()
		} else {
			result[name] = defaultToolMeta
		}
	}
	return result
}
