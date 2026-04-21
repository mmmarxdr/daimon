package rag

import "time"

// Document represents a full document stored in the RAG knowledge base.
type Document struct {
	ID           string    // unique document identifier
	Namespace    string    // scoping (e.g., "global", channel-specific)
	Title        string    // human-readable title
	SourceSHA256 string    // optional: reference to MediaStore blob
	MIME         string    // MIME type of the source content
	ChunkCount   int       // number of chunks this document was split into
	CreatedAt    time.Time // when the document was first ingested
	UpdatedAt    time.Time // when the document was last updated

	// Fields added in schema v12. All are nullable at the DB layer; zero
	// values are valid defaults for pre-v12 rows that have not yet been
	// touched by the injection counter or the summary worker.
	AccessCount    int        // how many times chunks from this doc were pulled into agent context
	LastAccessedAt *time.Time // when context injection last happened (nil = never)
	Summary        string     // 1-shot LLM summary of the document (empty until Phase B runs)
	PageCount      *int       // page count for paginated formats (PDF/DOCX); nil otherwise

	// IngestedAt (schema v13) is the timestamp the ingestion worker stamped
	// after processJob completed. It is independent of CreatedAt/UpdatedAt
	// because both of those get rewritten on every INSERT OR REPLACE; this
	// one is the authoritative "worker has finished" signal. nil means the
	// worker has not yet processed this row — the API treats that as
	// "indexing".
	IngestedAt *time.Time
}

// DocumentChunk is a single chunk from a Document, including its embedding vector.
type DocumentChunk struct {
	ID         string    // unique chunk identifier
	DocID      string    // parent document ID
	Index      int       // zero-based position within the document
	Content    string    // text content of the chunk
	Embedding  []float32 // 256-dim embedding vector (nil when not yet computed)
	TokenCount int       // approximate token count of Content
}

// SearchResult pairs a matching DocumentChunk with its parent document title and score.
type SearchResult struct {
	Chunk    DocumentChunk // the matching chunk
	DocTitle string        // title of the parent document
	Score    float64       // relevance score (higher is better)
}

// ExtractedDoc holds the output of an Extractor — plain text, optional title,
// and optional page count. PageCount is non-nil only for paginated formats
// (PDF, DOCX) and propagates to Document.PageCount during ingestion.
type ExtractedDoc struct {
	Title     string // extracted or inferred title
	Text      string // full extracted text
	PageCount *int   // nil for non-paginated formats (plain text, markdown, html)
}

// ChunkOptions controls how a Chunker splits text.
type ChunkOptions struct {
	Size    int // characters per chunk; default 512
	Overlap int // overlap characters between consecutive chunks; default 64
}
