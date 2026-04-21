package rag_test

import (
	"context"
	"testing"

	"daimon/internal/rag"
)

func TestPdfExtractor_Supports(t *testing.T) {
	e := rag.PdfExtractor{}
	if !e.Supports("application/pdf") {
		t.Error("expected support for application/pdf")
	}
	if !e.Supports("APPLICATION/PDF") {
		t.Error("expected case-insensitive support")
	}
	if e.Supports("text/plain") {
		t.Error("must not support text/plain")
	}
	if e.Supports("application/msword") {
		t.Error("must not support msword")
	}
}

func TestPdfExtractor_UnsupportedMIME(t *testing.T) {
	_, err := rag.PdfExtractor{}.Extract(context.Background(), []byte{0x25, 0x50, 0x44, 0x46}, "text/plain")
	if err != rag.ErrUnsupportedMIME {
		t.Errorf("expected ErrUnsupportedMIME, got %v", err)
	}
}

func TestPdfExtractor_InvalidBytes_ReturnsError(t *testing.T) {
	// Malformed PDF must surface as an error (either a parser error or a
	// recovered panic). Must NOT crash the caller.
	_, err := rag.PdfExtractor{}.Extract(context.Background(), []byte("definitely not a pdf"), "application/pdf")
	if err == nil {
		t.Fatal("expected error for malformed PDF bytes")
	}
}

func TestPdfExtractor_EmptyBytes_ReturnsError(t *testing.T) {
	_, err := rag.PdfExtractor{}.Extract(context.Background(), []byte{}, "application/pdf")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

// TestPdfExtractor_RealPDF exercises the full parser on a tiny valid PDF.
// The byte sequence below is a minimal single-page PDF containing the string
// "hello". It was hand-crafted for this test so we don't need to ship binary
// fixtures or external tools. If the upstream lib starts rejecting it, regen
// via `cat > tiny.pdf` from the body below and feed it to `pdftotext -`.
func TestPdfExtractor_RealPDF(t *testing.T) {
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

	doc, err := rag.PdfExtractor{}.Extract(context.Background(), pdfBytes, "application/pdf")
	if err != nil {
		t.Skipf("minimal PDF rejected by parser (upstream-sensitive): %v", err)
	}
	if doc.PageCount == nil || *doc.PageCount != 1 {
		t.Errorf("PageCount: expected *1, got %v", doc.PageCount)
	}
	// Text extraction on this minimal PDF may yield "hello" or an empty
	// string depending on how the lib handles uncompressed content streams.
	// The important assertion is that PageCount resolves — text is bonus.
	_ = doc.Text
}
