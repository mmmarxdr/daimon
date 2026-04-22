package rag_test

import (
	"math"
	"strings"
	"testing"

	"daimon/internal/rag"
)

// T1 — No junk tail chunks after the final chunk.
// The pre-fix chunker re-enters the loop after emitting the last chunk
// (where end==total) and produces `overlap` suffix-only junk chunks.
func TestFixedSizeChunker_NoJunkTailChunks(t *testing.T) {
	c := rag.FixedSizeChunker{}
	opts := rag.ChunkOptions{Size: 100, Overlap: 20}

	// Build text long enough to produce several real chunks.
	text := strings.Repeat("abcdefghij", 50) // 500 runes
	chunks := c.Chunk(text, opts)

	total := len([]rune(text))

	// The last chunk must end exactly at total (cover the tail).
	last := chunks[len(chunks)-1]
	lastRunes := []rune(last.Content)
	lastEnd := 0
	// Walk chunks to recover start positions isn't easy without internals,
	// so verify via content: last chunk's content must be the suffix of text.
	suffix := string([]rune(text)[total-len(lastRunes):])
	if last.Content != suffix {
		t.Errorf("last chunk content is not a suffix of text: got %q … (len=%d)", last.Content[:min(20, len(last.Content))], len(last.Content))
	}
	_ = lastEnd

	// No chunk may be a strict 1-rune-drop suffix of its predecessor (junk pattern).
	for i := 1; i < len(chunks); i++ {
		prev := []rune(chunks[i-1].Content)
		curr := []rune(chunks[i].Content)
		if len(curr) == len(prev)-1 && string(curr) == string(prev[1:]) {
			t.Errorf("chunk[%d] is a 1-rune-drop suffix of chunk[%d] — junk tail chunk detected", i, i-1)
		}
	}

	// Upper-bound sanity: chunk count must not exceed ceil(total/(Size-Overlap))+2.
	advance := opts.Size - opts.Overlap
	maxChunks := int(math.Ceil(float64(total)/float64(advance))) + 2
	if len(chunks) > maxChunks {
		t.Errorf("too many chunks: got %d, expected ≤ %d (text=%d runes, size=%d, overlap=%d)",
			len(chunks), maxChunks, total, opts.Size, opts.Overlap)
	}
}

// T2 — Exact symptom repro: text long enough that pre-fix would produce ≥10 junk chunks.
// With Size=100, Overlap=64 (default-ish): after the last real chunk the old code
// would emit 64 junk chunks (one per overlap rune). Post-fix: none.
func TestFixedSizeChunker_ExactRepro_TailJunk(t *testing.T) {
	c := rag.FixedSizeChunker{}
	opts := rag.ChunkOptions{Size: 100, Overlap: 64}

	// 500 runes → multiple real chunks, then the buggy loop would produce 64 junk chunks.
	text := strings.Repeat("x", 500)
	chunks := c.Chunk(text, opts)

	for i := 1; i < len(chunks); i++ {
		prev := []rune(chunks[i-1].Content)
		curr := []rune(chunks[i].Content)
		// Junk pattern: exactly 1 rune shorter and content == prev[1:]
		if len(curr) == len(prev)-1 && string(curr) == string(prev[1:]) {
			t.Errorf("chunk[%d] matches junk pattern (1-rune-drop suffix of chunk[%d])", i, i-1)
		}
	}

	// With Size=100, Overlap=64, advance=36 → ceil(500/36)+2 = 16 at most.
	maxChunks := int(math.Ceil(float64(500)/float64(opts.Size-opts.Overlap))) + 2
	if len(chunks) > maxChunks {
		t.Errorf("chunk count %d exceeds expected max %d — likely tail junk", len(chunks), maxChunks)
	}
}

// T3 — No triple-overlap: exact repro from Bug 2.
// text="0123456789abcdefghij" (20 runes), Size=5, Overlap=2.
// Pre-fix: runes at positions 2-3 appear in chunks 0, 1, AND 2.
// Post-fix: every rune index appears in at most 2 consecutive chunks.
func TestFixedSizeChunker_NoTripleOverlap(t *testing.T) {
	c := rag.FixedSizeChunker{}
	opts := rag.ChunkOptions{Size: 5, Overlap: 2}

	text := "0123456789abcdefghij" // exactly 20 runes
	chunks := c.Chunk(text, opts)

	runes := []rune(text)
	total := len(runes)

	// Reconstruct start position for each chunk by searching its content in text.
	// Since text has unique sequential characters, content uniquely identifies position.
	starts := make([]int, len(chunks))
	for i, ch := range chunks {
		cr := []rune(ch.Content)
		if len(cr) == 0 {
			continue
		}
		// Find where this chunk starts in the original text.
		found := -1
		for pos := 0; pos <= total-len(cr); pos++ {
			if string(runes[pos:pos+len(cr)]) == ch.Content {
				found = pos
				break
			}
		}
		if found < 0 {
			t.Fatalf("chunk[%d] content %q not found in text", i, ch.Content)
		}
		starts[i] = found
	}

	// For each rune index, count how many chunks contain it.
	runeChunkCount := make([]int, total)
	for i, ch := range chunks {
		cr := []rune(ch.Content)
		s := starts[i]
		for j := range cr {
			if s+j < total {
				runeChunkCount[s+j]++
			}
		}
	}

	for pos, count := range runeChunkCount {
		if count > 2 {
			t.Errorf("rune at position %d (%q) appears in %d chunks — triple overlap detected",
				pos, string(runes[pos]), count)
		}
	}
}

// T4 — Advance clamp preserves bounded overlap: the shared suffix/prefix
// between consecutive chunks must not exceed opts.Overlap runes.
// Uses unique monotone text so content-based overlap measurement is unambiguous.
func TestFixedSizeChunker_AdvanceClampPreservesBoundedOverlap(t *testing.T) {
	c := rag.FixedSizeChunker{}
	opts := rag.ChunkOptions{Size: 10, Overlap: 3}

	// Build a 60-rune text of unique printable runes (no spaces/punctuation so
	// snapBoundary always falls back to the hard-cut position, keeping results
	// deterministic). Using sequential Unicode codepoints starting at 0x4E00
	// (CJK) avoids any ASCII boundary matches.
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteRune(rune(0x4E00 + i))
	}
	text := sb.String()

	chunks := c.Chunk(text, opts)

	if len(chunks) < 2 {
		t.Skip("need at least 2 chunks")
	}

	runes := []rune(text)
	total := len(runes)

	// Recover each chunk's start position. The text is unique per rune so the
	// search always returns the correct position.
	starts := make([]int, len(chunks))
	for i, ch := range chunks {
		cr := []rune(ch.Content)
		found := -1
		for pos := 0; pos <= total-len(cr); pos++ {
			if string(runes[pos:pos+len(cr)]) == ch.Content {
				found = pos
				break
			}
		}
		if found < 0 {
			t.Fatalf("chunk[%d] content not found in text", i)
		}
		starts[i] = found
	}

	for i := 1; i < len(chunks); i++ {
		prevEnd := starts[i-1] + len([]rune(chunks[i-1].Content))
		currStart := starts[i]
		overlap := prevEnd - currStart
		if overlap < 0 {
			overlap = 0
		}
		// Overlap between consecutive chunks must not exceed opts.Overlap.
		if overlap > opts.Overlap {
			t.Errorf("chunk pair (%d,%d): overlap=%d exceeds opts.Overlap=%d", i-1, i, overlap, opts.Overlap)
		}
	}
}
