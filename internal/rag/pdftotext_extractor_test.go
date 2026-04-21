package rag_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"daimon/internal/rag"
)

// requirePdftotext skips when the binary isn't installed. Tests stay green on
// CI runners that don't have poppler-utils, and run for-real on dev boxes
// that do.
func requirePdftotext(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not in PATH — install poppler-utils to run this test")
	}
}

func TestPdftotextExtractor_Supports(t *testing.T) {
	requirePdftotext(t)
	e := rag.PdftotextExtractor{}
	if !e.Supports("application/pdf") {
		t.Error("expected support for application/pdf when pdftotext is installed")
	}
	if !e.Supports("APPLICATION/PDF") {
		t.Error("expected case-insensitive support")
	}
	if e.Supports("text/plain") {
		t.Error("must not support text/plain")
	}
}

func TestPdftotextExtractor_UnsupportedMIME(t *testing.T) {
	requirePdftotext(t)
	_, err := rag.PdftotextExtractor{}.Extract(context.Background(), []byte("%PDF-1.1"), "text/plain")
	if err != rag.ErrUnsupportedMIME {
		t.Errorf("expected ErrUnsupportedMIME, got %v", err)
	}
}

func TestPdftotextExtractor_RealPDF(t *testing.T) {
	requirePdftotext(t)
	// Reuses the hand-crafted minimal PDF from pdf_extractor_test.go.
	pdfBytes := []byte("%PDF-1.1\n" +
		"1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n" +
		"2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n" +
		"3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 144 144]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj\n" +
		"4 0 obj<</Length 44>>stream\n" +
		"BT /F1 24 Tf 10 60 Td (hello) Tj ET\n" +
		"endstream\nendobj\n" +
		"5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj\n" +
		"xref\n" +
		"0 6\n" +
		"0000000000 65535 f \n" +
		"0000000009 00000 n \n" +
		"0000000052 00000 n \n" +
		"0000000101 00000 n \n" +
		"0000000199 00000 n \n" +
		"0000000275 00000 n \n" +
		"trailer<</Size 6/Root 1 0 R>>\n" +
		"startxref\n330\n%%EOF\n")

	doc, err := rag.PdftotextExtractor{}.Extract(context.Background(), pdfBytes, "application/pdf")
	if err != nil {
		t.Skipf("pdftotext rejected the minimal fixture (older poppler): %v", err)
	}
	if !strings.Contains(doc.Text, "hello") {
		t.Errorf("expected 'hello' in extracted text, got %q", doc.Text)
	}
	if doc.PageCount == nil || *doc.PageCount != 1 {
		t.Errorf("expected PageCount=1 from form-feed count, got %v", doc.PageCount)
	}
}

func TestPdftotextExtractor_InvalidBytes_ReturnsError(t *testing.T) {
	requirePdftotext(t)
	_, err := rag.PdftotextExtractor{}.Extract(context.Background(), []byte("definitely not a pdf"), "application/pdf")
	if err == nil {
		t.Fatal("expected error for malformed PDF input")
	}
}
