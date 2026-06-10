package provider

import (
	"testing"
)

// --------------------------------------------------------------------------
// Task 1.1 — TestLongestSuffixThatIsPrefixOf (helper bound correctness, MM-5a)
// --------------------------------------------------------------------------

func TestLongestSuffixThatIsPrefixOf(t *testing.T) {
	cases := []struct {
		name   string
		work   string
		marker string
		want   int
	}{
		{"empty work", "", "</think>", 0},
		{"no match abc", "abc", "</think>", 0},
		{"single lt", "<", "</think>", 1},
		{"lt slash", "</", "</think>", 2},
		{"lt slash t", "</t", "</think>", 3},
		{"lt slash th", "</th", "</think>", 4},
		{"lt slash thi", "</thi", "</think>", 5},
		{"lt slash thin", "</thin", "</think>", 6},
		{"lt slash think", "</think", "</think>", 7},
		{"open tag partial thi", "<thi", "<think>", 4},
		{"open tag partial t", "<t", "<think>", 2},
		{"open tag single lt", "<", "<think>", 1},
		{"no overlap xyz", "xyz", "<think>", 0},
		{"work longer no tail", "hello world", "<think>", 0},
		{"partial in middle irrelevant", "abc<thi", "<think>", 4},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := longestSuffixThatIsPrefixOf(tc.work, tc.marker)
			if got != tc.want {
				t.Errorf("longestSuffixThatIsPrefixOf(%q, %q) = %d, want %d", tc.work, tc.marker, got, tc.want)
			}
		})
	}
}

// --------------------------------------------------------------------------
// Task 1.3 — TestThinkTagFilter_feed (state machine, ADR-2 case table)
// Satisfies MM-3a/3b/3c/4a/4b/5a and the design case table in ADR-2.
// --------------------------------------------------------------------------

func TestThinkTagFilter_feed(t *testing.T) {
	// Each case feeds a sequence of deltas and checks accumulated outputs + flush.
	cases := []struct {
		name        string
		deltas      []string
		wantText    string // cumulative text across all feeds + flush text
		wantReason  string // cumulative reason across all feeds + flush reason
		flushText   string // expected flush() textOut
		flushReason string // expected flush() reasonOut
	}{
		{
			name:       "no tag",
			deltas:     []string{"hello world"},
			wantText:   "hello world",
			wantReason: "",
		},
		{
			name:       "full tag one chunk",
			deltas:     []string{"a<think>cot</think>b"},
			wantText:   "ab",
			wantReason: "cot",
		},
		{
			name:       "content before and after",
			deltas:     []string{"pre<think>x</think>post"},
			wantText:   "prepost",
			wantReason: "x",
		},
		{
			// feed1: work="<thi", no full <think>, buf="<thi", textOut=""
			// feed2: work="<thi"+"nk>cot", finds <think>, reasonOut="cot"
			name:       "split open",
			deltas:     []string{"<thi", "nk>cot"},
			wantText:   "",
			wantReason: "cot",
		},
		{
			// Start inThink=true via first delta that completes <think>.
			// Then split </think> across two feeds.
			// feed1: "<think>c</thi" → inThink toggles → reason="c", buf="</thi"
			// feed2: "nk>ans" → finds </think>, textOut="ans"
			name:       "split close",
			deltas:     []string{"<think>c</thi", "nk>ans"},
			wantText:   "ans",
			wantReason: "c",
		},
		{
			// "p<th" → text="p", buf="<th"
			// "ink>r</th" → finds <think>, reason="r", buf="</th"
			// "ink>q" → finds </think>, text="q"
			name:       "split mid both",
			deltas:     []string{"p<th", "ink>r</th", "ink>q"},
			wantText:   "pq",
			wantReason: "r",
		},
		{
			name:       "multiple tags one delta",
			deltas:     []string{"a<think>x</think>b<think>y</think>c"},
			wantText:   "abc",
			wantReason: "xy",
		},
		{
			name:       "only think no answer",
			deltas:     []string{"<think>all</think>"},
			wantText:   "",
			wantReason: "all",
		},
		{
			// Nested re-open: inner <think> is literal inside reasoning (ADR-2.4).
			// <think> → inThink; inner <think> literal → reason gets "a<think>b";
			// </think> → !inThink; "c" → text.
			name:       "nested re-open",
			deltas:     []string{"<think>a<think>b</think>c"},
			wantText:   "c",
			wantReason: "a<think>b",
		},
		{
			// feed1: "a<th" → text="a", buf="<th"
			// feed2: "ing else" → work="<thing else", no <think> found → text="<thing else"
			name:       "partial prefix false alarm",
			deltas:     []string{"a<th", "ing else"},
			wantText:   "a<thing else",
			wantReason: "",
		},
		{
			// feed1: "x<thi" → text="x", buf="<thi" (4 bytes ≤ 6)
			name:        "tail equals marker minus 1",
			deltas:      []string{"x<thi"},
			wantText:    "x",
			wantReason:  "",
			flushText:   "<thi",
			flushReason: "",
		},
		{
			// Unclosed <think> at flush: buf flushed to reasonOut.
			// <think>partial cot → inThink=true, reason="partial cot", buf=""
			// flush: inThink → returns "", "partial cot" but buf="" so flush noop.
			// Let's use a case where buf has residual inside think:
			// "<think>partial</thi" → inThink=true, reason="partial", buf="</thi"
			// flush: inThink=true → reasonOut="</thi"
			name:        "unclosed think at flush",
			deltas:      []string{"<think>partial</thi"},
			wantText:    "",
			wantReason:  "partial",
			flushText:   "",
			flushReason: "</thi",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var f thinkTagFilter
			var totalText, totalReason string

			for _, d := range tc.deltas {
				txt, rsn := f.feed(d)
				totalText += txt
				totalReason += rsn
			}

			if totalText != tc.wantText {
				t.Errorf("feed text = %q, want %q", totalText, tc.wantText)
			}
			if totalReason != tc.wantReason {
				t.Errorf("feed reason = %q, want %q", totalReason, tc.wantReason)
			}

			// Check flush outputs when the test specifies them.
			if tc.flushText != "" || tc.flushReason != "" {
				ft, fr := f.flush()
				if ft != tc.flushText {
					t.Errorf("flush text = %q, want %q", ft, tc.flushText)
				}
				if fr != tc.flushReason {
					t.Errorf("flush reason = %q, want %q", fr, tc.flushReason)
				}
			}
		})
	}
}

// --------------------------------------------------------------------------
// Task 2.1 — TestStripThinkContent (sync wrapper, MM-2a/2b/2c, ADR-2.5)
// --------------------------------------------------------------------------

func TestStripThinkContent(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no tag unchanged",
			input: "Just a plain answer",
			want:  "Just a plain answer",
		},
		{
			name:  "full strip",
			input: "<think>step 1</think>The answer is 42",
			want:  "The answer is 42",
		},
		{
			name:  "only think yields empty",
			input: "<think>only reasoning here</think>",
			want:  "",
		},
		{
			name:  "content around think block",
			input: "pre<think>x</think>post",
			want:  "prepost",
		},
		{
			// Unclosed <think> at end: residual routes to reasoning (discarded).
			name:  "unclosed think yields empty text",
			input: "<think>partial cot",
			want:  "",
		},
		{
			name:  "multiple think blocks stripped",
			input: "a<think>x</think>b<think>y</think>c",
			want:  "abc",
		},
		{
			name:  "empty string unchanged",
			input: "",
			want:  "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := stripThinkContent(tc.input)
			if got != tc.want {
				t.Errorf("stripThinkContent(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
