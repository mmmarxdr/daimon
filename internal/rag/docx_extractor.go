package rag

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// docxMIME is the canonical MIME for modern Word docx files.
const docxMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

// DocxExtractor reads text from Office Open XML (.docx) documents using only
// the standard library (archive/zip + encoding/xml). It does NOT support
// legacy .doc binaries.
//
// Text strategy: iterate tokens of `word/document.xml`, collecting <w:t>
// runs. Paragraph elements (<w:p>) emit a newline boundary so the chunker sees
// natural breaks.
//
// Page count strategy: try `docProps/app.xml` (Word writes <Pages> here when
// saving); fall back to counting <w:br w:type="page"/> + 1 in document.xml.
// Both signals are approximate — a document with no explicit page breaks and
// no app.xml yields a nil PageCount, and the UI shows "—".
type DocxExtractor struct{}

// Supports returns true only for the docx MIME. Legacy .doc is NOT supported
// because its binary format requires a heavy parser.
func (e DocxExtractor) Supports(mime string) bool {
	return strings.ToLower(strings.TrimSpace(mime)) == docxMIME
}

func (e DocxExtractor) Extract(_ context.Context, data []byte, mime string) (ExtractedDoc, error) {
	if !e.Supports(mime) {
		return ExtractedDoc{}, ErrUnsupportedMIME
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ExtractedDoc{}, fmt.Errorf("rag: docx: open zip: %w", err)
	}

	var docXML, appXML []byte
	for _, f := range zr.File {
		switch f.Name {
		case "word/document.xml":
			docXML, err = readZipFile(f)
		case "docProps/app.xml":
			appXML, _ = readZipFile(f)
		}
		if err != nil {
			return ExtractedDoc{}, fmt.Errorf("rag: docx: read %s: %w", f.Name, err)
		}
	}
	if docXML == nil {
		return ExtractedDoc{}, fmt.Errorf("rag: docx: missing word/document.xml")
	}

	text, breakPages := extractDocxText(docXML)

	var pageCount *int
	if n := pagesFromAppXML(appXML); n > 0 {
		pageCount = &n
	} else if breakPages > 0 {
		n := breakPages
		pageCount = &n
	}

	return ExtractedDoc{
		Text:      strings.TrimSpace(text),
		PageCount: pageCount,
	}, nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// extractDocxText walks the XML stream emitting:
//   - text from <w:t> runs
//   - a newline after each <w:p> (paragraph)
//   - a page-break count derived from <w:br w:type="page"/>
//
// Page count returned is (break count + 1) when breaks were seen, or 0 when
// there were none (the caller then falls back to app.xml).
func extractDocxText(docXML []byte) (string, int) {
	dec := xml.NewDecoder(bytes.NewReader(docXML))
	var buf strings.Builder
	var inText bool
	breakCount := 0

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Malformed XML — return whatever we've accumulated so far.
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "br":
				// <w:br w:type="page"/> → page break signal.
				for _, a := range t.Attr {
					if a.Name.Local == "type" && a.Value == "page" {
						breakCount++
					}
				}
			case "tab":
				buf.WriteString("\t")
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				buf.WriteString("\n")
			}
		case xml.CharData:
			if inText {
				buf.Write(t)
			}
		}
	}

	pages := 0
	if breakCount > 0 {
		pages = breakCount + 1
	}
	return buf.String(), pages
}

// pagesFromAppXML reads the <Pages>N</Pages> element from docProps/app.xml.
// Returns 0 when the field is missing or not a positive integer.
func pagesFromAppXML(appXML []byte) int {
	if len(appXML) == 0 {
		return 0
	}
	dec := xml.NewDecoder(bytes.NewReader(appXML))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return 0
		}
		if err != nil {
			return 0
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "Pages" {
			continue
		}
		var body string
		if err := dec.DecodeElement(&body, &start); err != nil {
			return 0
		}
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(body), "%d", &n); err != nil || n <= 0 {
			return 0
		}
		return n
	}
}
