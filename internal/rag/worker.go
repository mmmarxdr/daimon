package rag

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// MediaStoreReader is a minimal interface to read blobs from a media store.
// It avoids importing the full store package and breaks import cycles.
type MediaStoreReader interface {
	GetMedia(ctx context.Context, sha256 string) ([]byte, string, error)
}

// IngestionJob represents a single document queued for ingestion.
type IngestionJob struct {
	DocID     string // unique document identifier
	Namespace string // scoping namespace (e.g. "global")
	Title     string // human-readable document title
	Content   string // inline text content (used when SHA256 is empty)
	SHA256    string // MediaStore blob reference (used when Content is empty)
	MIME      string // MIME type of the content
}

// DocIngestionWorkerConfig holds all dependencies for DocIngestionWorker.
type DocIngestionWorkerConfig struct {
	Store      DocumentStore
	Extractor  Extractor
	Chunker    Chunker
	EmbedFn    func(ctx context.Context, text string) ([]float32, error) // nil = no embedding
	SummaryFn  func(ctx context.Context, text string) (string, error)    // nil = no summary
	MediaStore MediaStoreReader
	ChunkOpts  ChunkOptions

	// EmbedBatchFn, when non-nil, lets the worker embed many chunks per HTTP
	// call instead of one. Cuts ingestion of a 1500-chunk book from minutes
	// (single-call + free-tier throttle) to seconds. The worker prefers
	// EmbedBatchFn over EmbedFn when both are set; EmbedFn is only used as
	// a last-resort fallback when batching fails for the entire document.
	EmbedBatchFn func(ctx context.Context, texts []string) ([][]float32, error)

	// EmbedThrottle paces calls to EmbedFn — minimum delay between consecutive
	// embed requests. Zero = no throttling (use only for paid tiers or local
	// providers like Ollama). Gemini's free tier caps at 100 req/min/model, so
	// rag_wiring.go sets this to ~700 ms when a Gemini embed provider is wired.
	// Batched calls bypass this throttle (one HTTP call regardless of size).
	EmbedThrottle time.Duration
}

// DocIngestionWorker asynchronously ingests documents into the RAG store.
// It extracts text, chunks it, optionally embeds each chunk, then persists.
//
// Use NewDocIngestionWorker to construct; call Start(ctx) before Enqueue.
type DocIngestionWorker struct {
	ch         chan IngestionJob
	wg         sync.WaitGroup
	stopOnce      sync.Once
	cancel        context.CancelFunc
	store         DocumentStore
	extractor     Extractor
	chunker       Chunker
	embedFn       func(ctx context.Context, text string) ([]float32, error)
	embedBatchFn  func(ctx context.Context, texts []string) ([][]float32, error)
	summaryFn     func(ctx context.Context, text string) (string, error)
	mediaStore    MediaStoreReader
	chunkOpts     ChunkOptions
	embedThrottle time.Duration
}

// NewDocIngestionWorker constructs a DocIngestionWorker from the given config.
// The worker is not started; call Start(ctx) to begin processing.
func NewDocIngestionWorker(cfg DocIngestionWorkerConfig) *DocIngestionWorker {
	return &DocIngestionWorker{
		ch:            make(chan IngestionJob, 5),
		store:         cfg.Store,
		extractor:     cfg.Extractor,
		chunker:       cfg.Chunker,
		embedFn:       cfg.EmbedFn,
		embedBatchFn:  cfg.EmbedBatchFn,
		summaryFn:     cfg.SummaryFn,
		mediaStore:    cfg.MediaStore,
		chunkOpts:     cfg.ChunkOpts,
		embedThrottle: cfg.EmbedThrottle,
	}
}

// embedBatchSize bounds how many chunks go in a single batched embed call.
// Conservative — Gemini batchEmbedContents documents up to ~100 per call,
// OpenAI accepts 2048 but tokens-per-request matters too. 100 is a safe
// common denominator and a 1500-chunk book fits in ~15 batched HTTP calls.
const embedBatchSize = 100

// embedRateLimitBackoff is the extra wait the worker takes after seeing a
// 429/RESOURCE_EXHAUSTED from the embedding provider. Gemini's free tier
// 429s suggest 22-23s; we round up to 30s to give the bucket headroom and
// avoid a second wave of failures.
const embedRateLimitBackoff = 30 * time.Second

// isRateLimitError sniffs whether an embed error came from the provider's
// quota. Substring match is intentional — different providers wrap differently
// (Gemini: "rate limit: gemini api error 429", OpenAI: "rate_limit_exceeded").
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "rate limit") ||
		strings.Contains(s, "RESOURCE_EXHAUSTED") ||
		strings.Contains(s, "429") ||
		strings.Contains(s, "rate_limit_exceeded")
}

// summaryMaxInputChars caps the text sent to the summarizer. Keeps token cost
// bounded regardless of document size — the Curator's prompt is ~100 tokens,
// 4000 input chars ≈ 1000 tokens of content, well under Haiku limits.
const summaryMaxInputChars = 4000

// summaryTimeout bounds one summary LLM call. Summaries are nice-to-have: a
// slow or hung provider must not block the ingestion pipeline.
const summaryTimeout = 15 * time.Second

// maybeSummarize invokes the optional SummaryFn with a bounded slice of the
// extracted text. Returns "" on any failure — the caller persists the empty
// summary and the UI shows nothing instead of a broken state.
func (w *DocIngestionWorker) maybeSummarize(ctx context.Context, text string) string {
	if w.summaryFn == nil {
		return ""
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	input := trimmed
	if len(input) > summaryMaxInputChars {
		input = input[:summaryMaxInputChars]
	}
	summaryCtx, cancel := context.WithTimeout(ctx, summaryTimeout)
	defer cancel()
	out, err := w.summaryFn(summaryCtx, input)
	if err != nil {
		slog.Debug("rag: summary fn failed (best-effort)", "error", err)
		return ""
	}
	return strings.TrimSpace(out)
}

// Start begins the background worker goroutine. When ctx is cancelled,
// the worker drains remaining queued jobs and exits cleanly.
func (w *DocIngestionWorker) Start(ctx context.Context) {
	workerCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.wg.Add(1)
	go w.run(workerCtx)
}

// Enqueue submits a job for ingestion. Non-blocking: if the internal channel
// is full, the job is silently dropped and a DEBUG log is emitted.
func (w *DocIngestionWorker) Enqueue(job IngestionJob) {
	select {
	case w.ch <- job:
	default:
		slog.Debug("rag: ingestion worker channel full, dropping job", "doc_id", job.DocID)
	}
}

// Stop signals the worker to stop and waits for it to finish. Idempotent.
func (w *DocIngestionWorker) Stop() {
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
	})
	w.wg.Wait()
}

// run is the main worker loop. Processes jobs until ctx is cancelled, then drains.
func (w *DocIngestionWorker) run(ctx context.Context) {
	defer w.wg.Done()
	for {
		select {
		case <-ctx.Done():
			// Drain remaining jobs with a short deadline.
			drainCtx, drainCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer drainCancel()
		drain:
			for {
				select {
				case job := <-w.ch:
					w.processJob(drainCtx, job)
				default:
					break drain
				}
			}
			return
		case job := <-w.ch:
			w.processJob(ctx, job)
		}
	}
}

// processJob executes the full ingestion pipeline for a single job.
func (w *DocIngestionWorker) processJob(ctx context.Context, job IngestionJob) {
	extracted, err := w.resolveDoc(ctx, job)
	if err != nil {
		// Extract failed (corrupted file, unsupported format, MediaStore miss).
		// Persist a placeholder marked as ingested so the API surfaces "empty"
		// instead of leaving the upload in an indefinite "indexing" state.
		slog.Warn("rag: failed to resolve text for job", "doc_id", job.DocID, "error", err)
		now := time.Now().UTC()
		if addErr := w.store.AddDocument(ctx, Document{
			ID:           job.DocID,
			Namespace:    job.Namespace,
			Title:        job.Title,
			SourceSHA256: job.SHA256,
			MIME:         job.MIME,
			ChunkCount:   0,
			Summary:      "",
			IngestedAt:   &now,
		}); addErr != nil {
			slog.Warn("rag: failed to persist extract-fail placeholder", "doc_id", job.DocID, "error", addErr)
		}
		return
	}

	// Chunk the text.
	rawChunks := w.chunker.Chunk(extracted.Text, w.chunkOpts)

	// Build chunks first; embed them in a separate pass.
	chunks := make([]DocumentChunk, 0, len(rawChunks))
	for _, ch := range rawChunks {
		ch.DocID = job.DocID
		ch.ID = fmt.Sprintf("%s/%s", job.DocID, ch.ID)
		chunks = append(chunks, ch)
	}

	// Prefer batched embedding when the provider supports it — single HTTP
	// round-trip per `embedBatchSize` chunks instead of one per chunk. For a
	// 1500-chunk book on Gemini paid this is ~15 calls vs. 1500.
	if w.embedBatchFn != nil {
		w.embedChunksBatched(ctx, chunks)
	} else if w.embedFn != nil {
		w.embedChunksSequential(ctx, chunks)
	}

	// Generate a summary BEFORE persisting so the INSERT carries everything in
	// one write. Best-effort: failures log at debug and leave Summary empty.
	summary := w.maybeSummarize(ctx, extracted.Text)

	// Persist document. IngestedAt is the authoritative "worker has finished"
	// signal — set unconditionally so the API can distinguish "still indexing"
	// from "ran but extracted no text".
	now := time.Now().UTC()
	doc := Document{
		ID:           job.DocID,
		Namespace:    job.Namespace,
		Title:        job.Title,
		SourceSHA256: job.SHA256,
		MIME:         job.MIME,
		ChunkCount:   len(chunks),
		PageCount:    extracted.PageCount,
		Summary:      summary,
		IngestedAt:   &now,
	}
	if err := w.store.AddDocument(ctx, doc); err != nil {
		slog.Warn("rag: failed to add document", "doc_id", job.DocID, "error", err)
		return
	}

	if len(chunks) > 0 {
		if err := w.store.AddChunks(ctx, job.DocID, chunks); err != nil {
			slog.Warn("rag: failed to add chunks", "doc_id", job.DocID, "error", err)
		}
	}
}

// embedChunksBatched embeds chunks in batches of embedBatchSize via the
// provider's batch endpoint. On rate-limit error backs off once and retries
// the same batch. On any other error the batch is skipped (chunks keep
// nil embeddings — RAG falls back to FTS5 keyword search for those).
//
// Mutates chunks in place — sets ch.Embedding when the call succeeds.
func (w *DocIngestionWorker) embedChunksBatched(ctx context.Context, chunks []DocumentChunk) {
	for start := 0; start < len(chunks); start += embedBatchSize {
		end := start + embedBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[start:end]
		texts := make([]string, len(batch))
		for i, ch := range batch {
			texts[i] = ch.Content
		}

		batchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		vecs, err := w.embedBatchFn(batchCtx, texts)
		cancel()

		if err != nil && isRateLimitError(err) {
			slog.Warn("rag: batch embed rate-limited, backing off",
				"start", start, "size", len(batch), "backoff", embedRateLimitBackoff)
			select {
			case <-time.After(embedRateLimitBackoff):
			case <-ctx.Done():
				return
			}
			retryCtx, retryCancel := context.WithTimeout(ctx, 30*time.Second)
			vecs, err = w.embedBatchFn(retryCtx, texts)
			retryCancel()
		}
		if err != nil {
			slog.Warn("rag: batch embed failed; chunks will be persisted without vectors",
				"start", start, "size", len(batch), "error", err)
			continue
		}
		if len(vecs) != len(batch) {
			slog.Warn("rag: batch embed returned wrong count; skipping batch",
				"expected", len(batch), "got", len(vecs))
			continue
		}
		for i, vec := range vecs {
			chunks[start+i].Embedding = NormalizeEmbedding(vec, 256)
		}
	}
}

// embedChunksSequential is the legacy single-call path used when the provider
// does not implement BatchEmbeddingProvider (e.g. older provider versions, or
// future providers we add without batch support yet). Throttled to avoid
// hammering rate-limited free tiers.
func (w *DocIngestionWorker) embedChunksSequential(ctx context.Context, chunks []DocumentChunk) {
	for i := range chunks {
		ch := &chunks[i]
		if i > 0 && w.embedThrottle > 0 {
			select {
			case <-time.After(w.embedThrottle):
			case <-ctx.Done():
				return
			}
		}
		embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		vec, embedErr := w.embedFn(embedCtx, ch.Content)
		cancel()
		if embedErr != nil && isRateLimitError(embedErr) {
			slog.Warn("rag: embed rate-limited, backing off", "chunk_id", ch.ID, "backoff", embedRateLimitBackoff)
			select {
			case <-time.After(embedRateLimitBackoff):
			case <-ctx.Done():
				return
			}
			retryCtx, retryCancel := context.WithTimeout(ctx, 10*time.Second)
			vec, embedErr = w.embedFn(retryCtx, ch.Content)
			retryCancel()
		}
		if embedErr != nil {
			slog.Warn("rag: embed failed for chunk", "chunk_id", ch.ID, "error", embedErr)
			continue
		}
		ch.Embedding = NormalizeEmbedding(vec, 256)
	}
}

// resolveDoc returns the extracted content for the job. Either fetches the
// blob from MediaStore and runs the extractor, or uses the inline Content as
// a plain-text fallback. The returned ExtractedDoc carries PageCount when the
// source format supports it (PDF/DOCX) and nil otherwise.
func (w *DocIngestionWorker) resolveDoc(ctx context.Context, job IngestionJob) (ExtractedDoc, error) {
	if job.SHA256 != "" {
		if w.mediaStore == nil {
			return ExtractedDoc{}, fmt.Errorf("rag: SHA256 job requires a MediaStoreReader")
		}
		data, mime, err := w.mediaStore.GetMedia(ctx, job.SHA256)
		if err != nil {
			return ExtractedDoc{}, fmt.Errorf("rag: GetMedia(%s): %w", job.SHA256, err)
		}
		if mime == "" {
			mime = job.MIME
		}
		if w.extractor != nil && w.extractor.Supports(mime) {
			doc, err := w.extractor.Extract(ctx, data, mime)
			if err != nil {
				return ExtractedDoc{}, fmt.Errorf("rag: extract from SHA256 %s: %w", job.SHA256, err)
			}
			return doc, nil
		}
		// Fall back to raw bytes as text.
		return ExtractedDoc{Text: string(data)}, nil
	}

	if w.extractor != nil && job.MIME != "" && w.extractor.Supports(job.MIME) {
		doc, err := w.extractor.Extract(ctx, []byte(job.Content), job.MIME)
		if err != nil {
			return ExtractedDoc{}, fmt.Errorf("rag: extract inline content: %w", err)
		}
		return doc, nil
	}

	return ExtractedDoc{Text: job.Content}, nil
}
