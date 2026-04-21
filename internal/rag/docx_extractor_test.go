package rag_test

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"daimon/internal/rag"
)

// buildDocx constructs a minimal valid docx in-memory from the given
// document.xml and (optional) app.xml bodies. Callers provide the raw XML
// strings wrapped in the standard W3C namespaces so the extractor's token
// walker sees the expected local names.
func buildDocx(t *testing.T, documentXML, appXML string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	if err := writeZipEntry(zw, "word/document.xml", documentXML); err != nil {
		t.Fatalf("writing document.xml: %v", err)
	}
	if appXML != "" {
		if err := writeZipEntry(zw, "docProps/app.xml", appXML); err != nil {
			t.Fatalf("writing app.xml: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip: %v", err)
	}
	return buf.Bytes()
}

func writeZipEntry(zw *zip.Writer, name, body string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(body))
	return err
}

const docMime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

const minimalDocXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Hello, </w:t><w:t>world.</w:t></w:r></w:p>
    <w:p><w:r><w:t>Second paragraph here.</w:t></w:r></w:p>
  </w:body>
</w:document>`

func TestDocxExtractor_Supports(t *testing.T) {
	e := rag.DocxExtractor{}
	if !e.Supports(docMime) {
		t.Errorf("expected support for %q", docMime)
	}
	if e.Supports("application/msword") {
		t.Error("must NOT support legacy .doc MIME")
	}
	if e.Supports("text/plain") {
		t.Error("must NOT support text/plain")
	}
}

func TestDocxExtractor_Extract_Text(t *testing.T) {
	data := buildDocx(t, minimalDocXML, "")
	doc, err := rag.DocxExtractor{}.Extract(context.Background(), data, docMime)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(doc.Text, "Hello, world.") {
		t.Errorf("expected 'Hello, world.' in text, got %q", doc.Text)
	}
	if !strings.Contains(doc.Text, "Second paragraph here.") {
		t.Errorf("expected 'Second paragraph here.' in text, got %q", doc.Text)
	}
	// Paragraphs separated by newlines from the <w:p> close boundary.
	if !strings.Contains(doc.Text, "\n") {
		t.Error("expected paragraph boundary as newline")
	}
}

func TestDocxExtractor_PageCount_FromAppXML(t *testing.T) {
	appXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties">
  <Pages>7</Pages>
</Properties>`
	data := buildDocx(t, minimalDocXML, appXML)
	doc, err := rag.DocxExtractor{}.Extract(context.Background(), data, docMime)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if doc.PageCount == nil || *doc.PageCount != 7 {
		t.Errorf("PageCount: expected *7 from app.xml, got %v", doc.PageCount)
	}
}

func TestDocxExtractor_PageCount_FromPageBreaks(t *testing.T) {
	// Two <w:br w:type="page"/> markers → 3 pages (breaks + 1).
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>First page.</w:t></w:r></w:p>
    <w:p><w:r><w:br w:type="page"/><w:t>Second page.</w:t></w:r></w:p>
    <w:p><w:r><w:br w:type="page"/><w:t>Third page.</w:t></w:r></w:p>
  </w:body>
</w:document>`
	data := buildDocx(t, docXML, "")
	doc, err := rag.DocxExtractor{}.Extract(context.Background(), data, docMime)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if doc.PageCount == nil || *doc.PageCount != 3 {
		t.Errorf("PageCount: expected *3 from page breaks, got %v", doc.PageCount)
	}
}

func TestDocxExtractor_PageCount_NilWhenNoSignal(t *testing.T) {
	data := buildDocx(t, minimalDocXML, "")
	doc, err := rag.DocxExtractor{}.Extract(context.Background(), data, docMime)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if doc.PageCount != nil {
		t.Errorf("expected nil PageCount when no signal, got %v", *doc.PageCount)
	}
}

func TestDocxExtractor_UnsupportedMIME(t *testing.T) {
	data := buildDocx(t, minimalDocXML, "")
	_, err := rag.DocxExtractor{}.Extract(context.Background(), data, "text/plain")
	if err != rag.ErrUnsupportedMIME {
		t.Errorf("expected ErrUnsupportedMIME, got %v", err)
	}
}

func TestDocxExtractor_InvalidZip(t *testing.T) {
	_, err := rag.DocxExtractor{}.Extract(context.Background(), []byte("not a zip"), docMime)
	if err == nil {
		t.Fatal("expected error for invalid zip bytes")
	}
}

func TestDocxExtractor_MissingDocumentXML(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_ = writeZipEntry(zw, "word/styles.xml", "<styles/>")
	_ = zw.Close()
	_, err := rag.DocxExtractor{}.Extract(context.Background(), buf.Bytes(), docMime)
	if err == nil {
		t.Fatal("expected error when word/document.xml is missing")
	}
}
