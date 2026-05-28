package tui

import (
	"strings"
	"testing"
	"time"
)

// TestBreadcrumb_Render_Empty: a breadcrumb with no turns and no label renders
// "" (zero height), so renderChat can skip it before the first turn lands.
func TestBreadcrumb_Render_Empty(t *testing.T) {
	s := newTuiStyles()
	b := breadcrumb{styles: s}
	if got := b.Render(80); got != "" {
		t.Errorf("empty breadcrumb must render \"\", got %q", got)
	}
}

// TestBreadcrumb_Render_Fields asserts the design fields are all present:
// "~/chat/ <label> · <N> turns · tokens <Xk> in · <Yk> out … autosave · <ago>".
func TestBreadcrumb_Render_Fields(t *testing.T) {
	s := newTuiStyles()
	b := breadcrumb{
		styles: s, label: "payment-anomalies",
		turns: 47, tokensIn: 34210, tokensOut: 8942,
		lastTurn: time.Now(),
	}
	got := b.Render(120)
	for _, want := range []string{"~/chat/", "payment-anomalies", "47 turns", "34.2k in", "8.9k out", "autosave"} {
		if !strings.Contains(got, want) {
			t.Errorf("breadcrumb missing %q\ngot: %q", want, got)
		}
	}
}

// TestBreadcrumb_Render_WidthRespected: the breadcrumb never exceeds its width.
func TestBreadcrumb_Render_WidthRespected(t *testing.T) {
	s := newTuiStyles()
	b := breadcrumb{styles: s, label: "x", turns: 3, tokensIn: 100, tokensOut: 50, lastTurn: time.Now()}
	got := b.Render(60)
	for i, line := range strings.Split(got, "\n") {
		if w := visibleWidth(line); w > 60 {
			t.Errorf("breadcrumb line %d width=%d > 60: %q", i, w, line)
		}
	}
}

// TestFormatTokensK verifies the compact k-suffixed token formatting.
func TestFormatTokensK(t *testing.T) {
	cases := map[int]string{0: "0", 999: "999", 1000: "1.0k", 34210: "34.2k", 8942: "8.9k"}
	for in, want := range cases {
		if got := formatTokensK(in); got != want {
			t.Errorf("formatTokensK(%d)=%q want %q", in, got, want)
		}
	}
}
