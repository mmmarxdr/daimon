package rag

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// pdftotextBinary holds the resolved path to the `pdftotext` CLI (from
// poppler-utils), cached at package-init time. When the user later installs
// poppler-utils, daimon needs a restart to pick this up — that's the same
// behaviour as the rest of the config.
//
// pdftotext's text extraction handles PDFs that ledongthuc/pdf can't —
// LaTeX-generated, CID-encoded fonts, complex multi-column layouts. It's the
// gold standard for PDF→text and a single `apt install poppler-utils` away.
//
// Shell-out is opt-in: when the binary is absent, PdftotextExtractor.Supports
// returns false and SelectExtractor cleanly falls through to PdfExtractor
// (the pure-Go ledongthuc-based path) without breaking the zero-CGO,
// single-binary deploy contract.
var pdftotextBinary = func() string {
	p, err := exec.LookPath("pdftotext")
	if err != nil {
		return ""
	}
	return p
}()

// PdftotextExtractor wraps the `pdftotext` CLI from poppler-utils. Stateless;
// availability is determined at package init. Construct with the zero value.
type PdftotextExtractor struct{}

// Supports returns true only when both (a) the MIME is application/pdf and
// (b) `pdftotext` was found in PATH at startup. Any other case yields false
// so the SelectExtractor cascade reaches the next extractor.
func (e PdftotextExtractor) Supports(mime string) bool {
	if pdftotextBinary == "" {
		return false
	}
	return strings.ToLower(strings.TrimSpace(mime)) == pdfMIME
}

// Extract pipes the PDF bytes through `pdftotext - -` (stdin → stdout).
// Page count comes from counting form-feed bytes (\f), which pdftotext emits
// between pages by default — no second `pdfinfo` subprocess required.
func (e PdftotextExtractor) Extract(ctx context.Context, data []byte, mime string) (ExtractedDoc, error) {
	if !e.Supports(mime) {
		return ExtractedDoc{}, ErrUnsupportedMIME
	}

	// `-q` suppresses warnings on stderr (we only care about stdout).
	// `-` for input means stdin; `-` for output means stdout.
	cmd := exec.CommandContext(ctx, pdftotextBinary, "-q", "-", "-")
	cmd.Stdin = bytes.NewReader(data)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return ExtractedDoc{}, fmt.Errorf("rag: pdftotext failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	text := stdout.String()
	// pdftotext default behaviour: emits a form-feed (\f, 0x0c) between pages.
	// Count gives a reliable page count even when the underlying PDF has no
	// /Count metadata. Add 1 because the marker is a *separator*, not a
	// terminator. Empty output (scanned PDF with no text layer) → 0 pages.
	trimmed := strings.TrimSpace(text)
	var pages *int
	if trimmed != "" {
		n := strings.Count(text, "\f") + 1
		// pdftotext sometimes emits a trailing \f — don't count an empty page.
		if strings.HasSuffix(strings.TrimRight(text, "\n"), "\f") {
			n--
		}
		if n > 0 {
			pages = &n
		}
	}

	return ExtractedDoc{
		Text:      trimmed,
		PageCount: pages,
	}, nil
}
