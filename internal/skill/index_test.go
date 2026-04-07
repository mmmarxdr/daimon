package skill

import (
	"strings"
	"testing"
)

func TestBuildIndex(t *testing.T) {
	tests := []struct {
		name       string
		skills     []SkillContent
		wantCount  int
		wantNames  []string
		wantRender string // substring check; empty = skip
	}{
		{
			name: "filters autoload skills",
			skills: []SkillContent{
				{Name: "cron", Description: "scheduling", Autoload: false},
				{Name: "git-helper", Description: "git workflows", Autoload: true},
			},
			wantCount: 1,
			wantNames: []string{"cron"},
		},
		{
			name: "sorts alphabetically",
			skills: []SkillContent{
				{Name: "zeta", Description: "last", Autoload: false},
				{Name: "alpha", Description: "first", Autoload: false},
				{Name: "mu", Description: "middle", Autoload: false},
			},
			wantCount: 3,
			wantNames: []string{"alpha", "mu", "zeta"},
		},
		{
			name:      "empty input returns empty index",
			skills:    []SkillContent{},
			wantCount: 0,
		},
		{
			name: "all autoload returns empty index",
			skills: []SkillContent{
				{Name: "a", Description: "desc", Autoload: true},
				{Name: "b", Description: "desc", Autoload: true},
			},
			wantCount: 0,
		},
		{
			name: "empty description gets default",
			skills: []SkillContent{
				{Name: "nodesc", Description: "", Autoload: false},
			},
			wantCount:  1,
			wantNames:  []string{"nodesc"},
			wantRender: "No description provided.",
		},
		{
			name: "mixed autoload and non-autoload",
			skills: []SkillContent{
				{Name: "always", Description: "always on", Autoload: true},
				{Name: "beta", Description: "on demand", Autoload: false},
				{Name: "gamma", Description: "also on demand", Autoload: false},
				{Name: "delta", Description: "always too", Autoload: true},
			},
			wantCount: 2,
			wantNames: []string{"beta", "gamma"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			idx := BuildIndex(tc.skills)
			if len(idx.Entries) != tc.wantCount {
				t.Errorf("got %d entries, want %d", len(idx.Entries), tc.wantCount)
			}
			for i, wantName := range tc.wantNames {
				if i >= len(idx.Entries) {
					t.Errorf("missing entry at index %d: want %q", i, wantName)
					continue
				}
				if idx.Entries[i].Name != wantName {
					t.Errorf("entry[%d].Name = %q, want %q", i, idx.Entries[i].Name, wantName)
				}
			}
			if tc.wantRender != "" {
				rendered := idx.Render()
				if !strings.Contains(rendered, tc.wantRender) {
					t.Errorf("Render() does not contain %q:\n%s", tc.wantRender, rendered)
				}
			}
		})
	}
}

func TestSkillIndex_Render(t *testing.T) {
	t.Run("empty index renders empty string", func(t *testing.T) {
		idx := SkillIndex{}
		if got := idx.Render(); got != "" {
			t.Errorf("got %q, want empty string", got)
		}
	})

	t.Run("renders header and entries", func(t *testing.T) {
		idx := SkillIndex{
			Entries: []IndexEntry{
				{Name: "alpha", Description: "first skill"},
				{Name: "beta", Description: "second skill"},
			},
		}
		got := idx.Render()
		if !strings.HasPrefix(got, "## Available Skills\n") {
			t.Errorf("missing header, got:\n%s", got)
		}
		if !strings.Contains(got, "load_skill") {
			t.Error("missing load_skill instruction")
		}
		if !strings.Contains(got, "- **alpha**: first skill") {
			t.Errorf("missing alpha entry:\n%s", got)
		}
		if !strings.Contains(got, "- **beta**: second skill") {
			t.Errorf("missing beta entry:\n%s", got)
		}
	})
}

func TestSkillIndex_TokenEstimate(t *testing.T) {
	t.Run("empty index returns 0", func(t *testing.T) {
		idx := SkillIndex{}
		if got := idx.TokenEstimate(); got != 0 {
			t.Errorf("got %d, want 0", got)
		}
	})

	t.Run("non-empty index returns positive", func(t *testing.T) {
		idx := SkillIndex{
			Entries: []IndexEntry{
				{Name: "cron", Description: "scheduling tasks"},
			},
		}
		if got := idx.TokenEstimate(); got <= 0 {
			t.Errorf("got %d, want > 0", got)
		}
	})
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"abcd", 1},
		{"abcdefgh", 2},
		{"a", 0}, // 1/4 = 0
	}
	for _, tc := range tests {
		got := EstimateTokens(tc.input)
		if got != tc.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}
