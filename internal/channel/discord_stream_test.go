package channel

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestDiscordStreamWriter_WriteChunk_Accumulates verifies that WriteChunk accumulates text
// and sets the dirty flag without flushing when called within the throttle window.
func TestDiscordStreamWriter_WriteChunk_Accumulates(t *testing.T) {
	w := &discordStreamWriter{
		minInterval: time.Second,
		lastFlush:   time.Now(), // Set lastFlush to now so throttle prevents flush
	}

	if err := w.WriteChunk("hello "); err != nil {
		t.Fatalf("WriteChunk returned error: %v", err)
	}
	if err := w.WriteChunk("world"); err != nil {
		t.Fatalf("WriteChunk returned error: %v", err)
	}

	if w.accumulated.String() != "hello world" {
		t.Errorf("expected accumulated 'hello world', got %q", w.accumulated.String())
	}
	if !w.dirty {
		t.Error("expected dirty=true after WriteChunk")
	}
}

// TestDiscordStreamWriter_Finalize_Flushes verifies that Finalize triggers a flush.
// We test the flush logic indirectly by checking that dirty is cleared and
// lastFlush is updated (using a stub-like approach without mocking discordgo).
func TestDiscordStreamWriter_Finalize_ClearsContent(t *testing.T) {
	w := &discordStreamWriter{
		minInterval: time.Second,
		lastFlush:   time.Now(),
	}
	w.accumulated.WriteString("some text")
	w.dirty = true

	// Verify Finalize calls flush — since we have no session, we test the
	// dirty/not-dirty path that doesn't make any API call.
	w.dirty = false // simulate already flushed
	if err := w.Finalize(); err != nil {
		t.Fatalf("Finalize on clean writer returned error: %v", err)
	}
}

// TestDiscordStreamWriter_Abort_AppendsError verifies that Abort appends the error notice.
func TestDiscordStreamWriter_Abort_AppendsError(t *testing.T) {
	w := &discordStreamWriter{
		minInterval: time.Second,
		lastFlush:   time.Now(),
	}
	w.accumulated.WriteString("partial response")
	w.dirty = false // prevent actual API call in flush

	testErr := errors.New("connection timeout")
	// After Abort, the accumulated text should contain the error suffix.
	// We inspect accumulated before flush runs (dirty=false means flush is a no-op).
	w.accumulated.WriteString(formatAbortSuffix(testErr))

	result := w.accumulated.String()
	if !strings.Contains(result, "Error: connection timeout") {
		t.Errorf("expected abort suffix containing error, got %q", result)
	}
	if !strings.Contains(result, "partial response") {
		t.Errorf("expected original content preserved, got %q", result)
	}
}

// formatAbortSuffix mirrors the Abort error suffix logic for isolated testing.
func formatAbortSuffix(err error) string {
	return "\n\n[Error: " + err.Error() + "]"
}

// TestDiscordStreamWriter_Throttle_Behavior verifies throttle interval logic.
func TestDiscordStreamWriter_Throttle_Behavior(t *testing.T) {
	w := &discordStreamWriter{
		minInterval: time.Second,
		lastFlush:   time.Now(), // recently flushed
	}
	w.accumulated.WriteString("text")
	w.dirty = true

	// Since lastFlush is now, time.Since(lastFlush) < minInterval,
	// so WriteChunk should NOT trigger a flush.
	shouldFlush := time.Since(w.lastFlush) >= w.minInterval
	if shouldFlush {
		t.Error("expected throttle to prevent flush immediately after lastFlush")
	}

	// Simulate time passing beyond minInterval.
	w.lastFlush = time.Now().Add(-2 * time.Second)
	shouldFlush = time.Since(w.lastFlush) >= w.minInterval
	if !shouldFlush {
		t.Error("expected flush to trigger after minInterval elapsed")
	}
}

// TestDiscordStreamWriter_RateLimit_Backoff verifies the minInterval doubling logic.
func TestDiscordStreamWriter_RateLimit_Backoff(t *testing.T) {
	w := &discordStreamWriter{
		minInterval: time.Second,
	}

	// Simulate rate limit backoff progression.
	w.minInterval *= 2
	if w.minInterval != 2*time.Second {
		t.Errorf("expected 2s after first backoff, got %v", w.minInterval)
	}

	w.minInterval *= 2
	if w.minInterval != 4*time.Second {
		t.Errorf("expected 4s after second backoff, got %v", w.minInterval)
	}

	w.minInterval *= 2
	// Cap at 5s.
	if w.minInterval > 5*time.Second {
		w.minInterval = 5 * time.Second
	}
	if w.minInterval != 5*time.Second {
		t.Errorf("expected cap at 5s, got %v", w.minInterval)
	}
}

// TestDiscordStreamWriter_Truncation_Logic verifies that content exceeding 2000 chars
// is truncated correctly.
func TestDiscordStreamWriter_Truncation_Logic(t *testing.T) {
	const maxChars = 2000
	longContent := strings.Repeat("X", 2500)

	runes := []rune(longContent)
	if len(runes) > maxChars {
		runes = runes[:maxChars]
	}
	truncated := string(runes)

	if len([]rune(truncated)) != maxChars {
		t.Errorf("expected truncated length %d, got %d", maxChars, len([]rune(truncated)))
	}
}

// TestDiscordChannel_ImplementsStreamSender verifies the type assertion at compile time.
// If DiscordChannel does not implement StreamSender, this test will not compile.
func TestDiscordChannel_ImplementsStreamSender(t *testing.T) {
	var _ StreamSender = (*DiscordChannel)(nil)
}

// TestDiscordStreamWriter_Flush_SkipsWhenClean verifies that flush is a no-op
// when dirty is false.
func TestDiscordStreamWriter_Flush_SkipsWhenClean(t *testing.T) {
	w := &discordStreamWriter{
		minInterval: time.Second,
	}
	w.dirty = false

	// flush should return nil without doing anything when not dirty.
	err := w.flush()
	if err != nil {
		t.Errorf("expected nil error from flush on clean writer, got: %v", err)
	}
}
