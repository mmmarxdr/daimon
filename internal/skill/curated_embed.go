package skill

import (
	"embed"

	"daimon/internal/config"
)

// CuratedFS holds the curated skill templates shipped with the daimon binary.
// It is populated at compile time via the go:embed directive and passed to
// LoadSkillsUnified as the lowest-precedence source (design §2.10).
//
// The embedded path must match the directory walked by loadCurated ("curated/").
// Files without the .skill.md suffix are silently ignored by loadCurated.
//
//go:embed curated/*.md
var CuratedFS embed.FS

// CuratedCatalog parses and returns the full curated skill catalog from the
// embedded CuratedFS. It is the public entry point for callers (e.g., boot
// wiring, web handlers) that need the curated list without going through
// LoadSkillsUnified. Non-fatal parse warnings are returned in errs; fatal
// problems are silently dropped (same policy as loadCurated).
//
// Returned slices are never nil — callers may range over them unconditionally.
// (design §2.10; spec-gap fix for tasks 3.8 + 6.13)
func CuratedCatalog(shellCfg config.ShellToolConfig, limits config.LimitsConfig) ([]SkillContent, []ExecutableSkillDef, []error) {
	contents, execs, errs := loadCurated(CuratedFS, shellCfg, limits)
	if contents == nil {
		contents = []SkillContent{}
	}
	if execs == nil {
		execs = []ExecutableSkillDef{}
	}
	return contents, execs, errs
}
