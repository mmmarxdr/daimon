package rag

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

const pdfMIME = "application/pdf"

// PdfExtractor reads text from PDF documents via github.com/ledongthuc/pdf
// (pure Go, no CGO). Preserves the daimon zero-CGO philosophy at the cost of
// ~1 MB of binary bloat and a modest "handles 80% of real-world PDFs" ceiling.
//
// Known limitations of the upstream lib:
//   - Scanned PDFs without embedded text layers yield empty output (no OCR).
//   - Some complex multi-column or heavily-styled layouts lose ordering.
//   - Encrypted PDFs require a password (NewReaderEncrypted) which we don't wire.
//   - The lib has been observed to panic on malformed streams; we recover().
type PdfExtractor struct{}

func (e PdfExtractor) Supports(mime string) bool {
	return strings.ToLower(strings.TrimSpace(mime)) == pdfMIME
}

func (e PdfExtractor) Extract(_ context.Context, data []byte, mime string) (ExtractedDoc, error) {
	if !e.Supports(mime) {
		return ExtractedDoc{}, ErrUnsupportedMIME
	}

	doc, err := safeExtractPDF(data)
	if err != nil {
		return ExtractedDoc{}, err
	}
	return doc, nil
}

// safeExtractPDF wraps the ledongthuc/pdf calls with a recover() because the
// upstream lib occasionally panics on malformed PDF streams rather than
// returning an error. A recovered panic surfaces as a normal error to callers.
func safeExtractPDF(data []byte) (doc ExtractedDoc, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("rag: pdf: parser panic: %v", r)
			doc = ExtractedDoc{}
		}
	}()

	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ExtractedDoc{}, fmt.Errorf("rag: pdf: open: %w", err)
	}

	pages := reader.NumPage()

	textReader, err := reader.GetPlainText()
	if err != nil {
		return ExtractedDoc{}, fmt.Errorf("rag: pdf: extract text: %w", err)
	}
	text, err := io.ReadAll(textReader)
	if err != nil {
		return ExtractedDoc{}, fmt.Errorf("rag: pdf: read text: %w", err)
	}

	result := ExtractedDoc{Text: strings.TrimSpace(string(text))}
	if pages > 0 {
		n := pages
		result.PageCount = &n
	}
	return result, nil
}
