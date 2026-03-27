package channel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

// TestCLIChannel_ImplementsStreamSender verifies CLIChannel satisfies StreamSender.
func TestCLIChannel_ImplementsStreamSender(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()

	ch := newTestCLIChannel(pr, &bytes.Buffer{})
	var _ StreamSender = ch // compile-time assertion
}

// TestCLIChannel_BeginStream_ReturnsValidWriter verifies BeginStream returns a non-nil StreamWriter.
func TestCLIChannel_BeginStream_ReturnsValidWriter(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()

	buf := &bytes.Buffer{}
	ch := newTestCLIChannel(pr, buf)

	sw, err := ch.BeginStream(context.Background(), "cli")
	if err != nil {
		t.Fatalf("BeginStream() returned error: %v", err)
	}
	if sw == nil {
		t.Fatal("BeginStream() returned nil StreamWriter")
	}
}

// TestCLIStreamWriter_WriteChunk verifies chunks are written to the underlying writer.
func TestCLIStreamWriter_WriteChunk(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()

	buf := &bytes.Buffer{}
	ch := newTestCLIChannel(pr, buf)

	sw, err := ch.BeginStream(context.Background(), "cli")
	if err != nil {
		t.Fatalf("BeginStream() returned error: %v", err)
	}

	if err := sw.WriteChunk("hello"); err != nil {
		t.Fatalf("WriteChunk() returned error: %v", err)
	}

	got := buf.String()
	if got != "hello" {
		t.Errorf("after WriteChunk(\"hello\"), output = %q, want %q", got, "hello")
	}
}

// TestCLIStreamWriter_Finalize verifies Finalize appends a newline.
func TestCLIStreamWriter_Finalize(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()

	buf := &bytes.Buffer{}
	ch := newTestCLIChannel(pr, buf)

	sw, err := ch.BeginStream(context.Background(), "cli")
	if err != nil {
		t.Fatalf("BeginStream() returned error: %v", err)
	}

	if err := sw.WriteChunk("data"); err != nil {
		t.Fatalf("WriteChunk() returned error: %v", err)
	}
	if err := sw.Finalize(); err != nil {
		t.Fatalf("Finalize() returned error: %v", err)
	}

	got := buf.String()
	want := "data\n"
	if got != want {
		t.Errorf("after WriteChunk + Finalize, output = %q, want %q", got, want)
	}
}

// TestCLIStreamWriter_Abort verifies Abort writes a newline.
func TestCLIStreamWriter_Abort(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()

	buf := &bytes.Buffer{}
	ch := newTestCLIChannel(pr, buf)

	sw, err := ch.BeginStream(context.Background(), "cli")
	if err != nil {
		t.Fatalf("BeginStream() returned error: %v", err)
	}

	if err := sw.WriteChunk("partial"); err != nil {
		t.Fatalf("WriteChunk() returned error: %v", err)
	}
	if err := sw.Abort(errors.New("stream error")); err != nil {
		t.Fatalf("Abort() returned error: %v", err)
	}

	got := buf.String()
	want := "partial\n"
	if got != want {
		t.Errorf("after WriteChunk + Abort, output = %q, want %q", got, want)
	}
}

// TestCLIStreamWriter_MultipleChunks verifies multiple WriteChunk calls accumulate correctly.
func TestCLIStreamWriter_MultipleChunks(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	defer pr.Close()

	buf := &bytes.Buffer{}
	ch := newTestCLIChannel(pr, buf)

	sw, err := ch.BeginStream(context.Background(), "cli")
	if err != nil {
		t.Fatalf("BeginStream() returned error: %v", err)
	}

	chunks := []string{"The ", "quick ", "brown ", "fox"}
	for _, chunk := range chunks {
		if err := sw.WriteChunk(chunk); err != nil {
			t.Fatalf("WriteChunk(%q) returned error: %v", chunk, err)
		}
	}

	if err := sw.Finalize(); err != nil {
		t.Fatalf("Finalize() returned error: %v", err)
	}

	got := buf.String()
	want := "The quick brown fox\n"
	if got != want {
		t.Errorf("after multiple WriteChunk + Finalize, output = %q, want %q", got, want)
	}
}
