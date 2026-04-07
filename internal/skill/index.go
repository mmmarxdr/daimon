package skill

import (
	"sort"
	"strings"
)

// IndexEntry represents a single skill in the compact skill index.
type IndexEntry struct {
	Name        string
	Description string
}

// SkillIndex holds the compiled index of non-autoload skills,
// ready for injection into the system prompt.
type SkillIndex struct {
	Entries []IndexEntry
}

// BuildIndex creates a SkillIndex from non-autoload skills.
// Only includes skills with Autoload == false.
// Entries are sorted alphabetically by Name.
// Skills with empty Description get "No description provided."
func BuildIndex(skills []SkillContent) SkillIndex {
	var entries []IndexEntry
	for _, s := range skills {
		if s.Autoload {
			continue
		}
		desc := s.Description
		if desc == "" {
			desc = "No description provided."
		}
		entries = append(entries, IndexEntry{
			Name:        s.Name,
			Description: desc,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return SkillIndex{Entries: entries}
}

// Render returns the prompt-ready index block.
// Returns empty string if no entries exist.
func (idx SkillIndex) Render() string {
	if len(idx.Entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Available Skills\n")
	sb.WriteString("Use the `load_skill` tool with the skill name to read its full instructions.\n\n")
	for _, e := range idx.Entries {
		sb.WriteString("- **")
		sb.WriteString(e.Name)
		sb.WriteString("**: ")
		sb.WriteString(e.Description)
		sb.WriteString("\n")
	}
	return sb.String()
}

// EstimateTokens approximates token count for a string (bytes/4 heuristic).
// Local to skill package to avoid import cycle with agent.
func EstimateTokens(s string) int {
	return len(s) / 4
}

// TokenEstimate returns approximate token count for the rendered index.
func (idx SkillIndex) TokenEstimate() int {
	return EstimateTokens(idx.Render())
}
