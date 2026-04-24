package agent

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/store"
)

// titlePrompt is the exact prompt sent to the provider. Kept non-configurable
// in v1 per the spec — a later change can parametrize it when we have data on
// which phrasings produce the cleanest titles. {serialized_turns} is appended
// by the caller at call time (not interpolated by template to avoid surprises
// if a user message contains `{` / `}`).
const titlePrompt = "Generate a 3-8 word title summarising this conversation. " +
	"Respond with ONLY the title — no quotes, no preamble, no explanation.\n\n"

// maxTitleRunes clamps the normalized title length. Spec caps at 100 runes.
const maxTitleRunes = 100

// titleStoreAPI is the slice of store.Store the titler needs. Keeping the
// interface tiny makes the worker easy to test with a fake.
type titleStoreAPI interface {
	LoadConversation(ctx context.Context, id string) (*store.Conversation, error)
	SaveConversation(ctx context.Context, conv store.Conversation) error
}

// titleProviderAPI is the slice of provider.Provider the titler needs.
type titleProviderAPI interface {
	Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error)
}

// TitleGenerator runs async LLM title-generation jobs for conversations that
// have accumulated enough real content to summarise. Jobs are enqueued by
// the agent loop's post-save hook; the generator runs them on a small worker
// pool with a queue. Failures are silent (slog.Warn at most) — title
// generation is a nice-to-have that must never block or break the turn path.
type TitleGenerator struct {
	jobs     chan string
	store    titleStoreAPI
	provider titleProviderAPI
	cfg      config.TitleGenYAMLConfig

	doneCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	stopOnce sync.Once
	closeJobsOnce sync.Once
}

// NewTitleGenerator constructs and starts a TitleGenerator. cfg is expected
// to be already default-applied + clamped by config.ApplyDefaults.
func NewTitleGenerator(st titleStoreAPI, prov titleProviderAPI, cfg config.TitleGenYAMLConfig) *TitleGenerator {
	ctx, cancel := context.WithCancel(context.Background())
	tg := &TitleGenerator{
		jobs:     make(chan string, cfg.QueueSize),
		store:    st,
		provider: prov,
		cfg:      cfg,
		doneCtx:  ctx,
		cancel:   cancel,
	}
	for i := 0; i < cfg.WorkerCount; i++ {
		tg.wg.Add(1)
		go tg.worker()
	}
	return tg
}

// Enqueue requests a title job for convID. Non-blocking: if the queue is
// full OR the generator is shutting down, the job is dropped with a warn.
// Satisfies the agent.Titler interface.
func (tg *TitleGenerator) Enqueue(_ context.Context, convID string) {
	select {
	case <-tg.doneCtx.Done():
		slog.Debug("title_generator: shutting down, job dropped", "conv_id", convID)
		return
	case tg.jobs <- convID:
		return
	default:
		slog.Warn("title_generator: queue full, job dropped", "conv_id", convID)
	}
}

// Stop signals workers to exit after their current job completes. Waits up
// to ctx's deadline for in-flight work to finish; discards pending jobs on
// timeout. Safe to call multiple times.
func (tg *TitleGenerator) Stop(ctx context.Context) error {
	tg.stopOnce.Do(func() {
		tg.cancel()
		tg.closeJobsOnce.Do(func() { close(tg.jobs) })
	})
	done := make(chan struct{})
	go func() { tg.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (tg *TitleGenerator) worker() {
	defer tg.wg.Done()
	for convID := range tg.jobs {
		tg.run(convID)
	}
}

// run processes one title job end-to-end. Keep this function close to the
// spec scenarios — every branch maps to a spec bullet.
func (tg *TitleGenerator) run(convID string) {
	// Per-job timeout — independent of the doneCtx so Stop() can cancel an
	// in-flight call cleanly via Done propagation.
	timeout := time.Duration(tg.cfg.CallTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(tg.doneCtx, timeout)
	defer cancel()

	conv, err := tg.store.LoadConversation(ctx, convID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("title_generator: load failed", "conv_id", convID, "error", err)
		}
		return
	}
	if !shouldGenerateTitle(conv) {
		// Race: conv was renamed or shrank (deletion+restore) between
		// enqueue and run. Respect the current state and skip.
		return
	}

	prompt := titlePrompt + serializeFirstTurns(conv.Messages)
	resp, err := tg.provider.Chat(ctx, provider.ChatRequest{
		Model: tg.cfg.Model, // empty string → provider's default
		Messages: []provider.ChatMessage{{
			Role:    "user",
			Content: content.Blocks{{Type: content.BlockText, Text: prompt}},
		}},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			slog.Warn("title_generator: provider_timeout", "conv_id", convID)
			return
		}
		slog.Warn("title_generator: chat failed", "conv_id", convID, "error", err)
		return
	}
	title := normalizeTitle(resp.Content)
	if title == "" {
		slog.Warn("title_generator: empty_response", "conv_id", convID)
		return
	}

	// Re-load + re-check to avoid overwriting a manual rename that
	// happened during the provider call.
	conv, err = tg.store.LoadConversation(ctx, convID)
	if err != nil || !shouldGenerateTitle(conv) {
		return
	}
	if conv.Metadata == nil {
		conv.Metadata = map[string]string{}
	}
	conv.Metadata["title"] = title
	if err := tg.store.SaveConversation(ctx, *conv); err != nil {
		slog.Warn("title_generator: save failed", "conv_id", convID, "error", err)
	}
}

// serializeFirstTurns formats the first 6 messages as role-tagged text.
// Media and tool-use blocks are intentionally omitted — titles summarise
// intent, which lives in the text content.
func serializeFirstTurns(msgs []provider.ChatMessage) string {
	var b strings.Builder
	count := 0
	for _, m := range msgs {
		if count >= 6 {
			break
		}
		text := strings.TrimSpace(m.Content.TextOnly())
		if text == "" {
			continue
		}
		switch m.Role {
		case "user":
			b.WriteString("User: ")
		case "assistant":
			b.WriteString("Assistant: ")
		default:
			// System / tool_result / unknown — skip (tool results are
			// especially noisy for a title).
			continue
		}
		b.WriteString(text)
		b.WriteString("\n")
		count++
	}
	return b.String()
}

// normalizeTitle trims whitespace, strips markdown delimiters and quotes
// from the ends, replaces any embedded newlines with spaces, and clamps the
// result to maxTitleRunes. Returns the normalized string — empty means the
// caller should skip the save.
func normalizeTitle(s string) string {
	s = strings.TrimSpace(s)
	// Strip quote / markdown wrappers from both ends, repeatedly (LLMs often
	// produce `"title"` or `*title*` even when told not to).
	for i := 0; i < 4; i++ {
		before := s
		for _, ch := range []string{`"`, `'`, "`", "*", "_"} {
			s = strings.TrimPrefix(s, ch)
			s = strings.TrimSuffix(s, ch)
		}
		s = strings.TrimSpace(s)
		if s == before {
			break
		}
	}
	// Replace newlines with spaces.
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	// Collapse repeated whitespace.
	s = strings.Join(strings.Fields(s), " ")
	// Clamp to maxTitleRunes.
	if utf8.RuneCountInString(s) > maxTitleRunes {
		// Find the byte index of the maxTitleRunes-th rune and cut there.
		i, count := 0, 0
		for ; i < len(s); {
			_, size := utf8.DecodeRuneInString(s[i:])
			count++
			if count > maxTitleRunes {
				break
			}
			i += size
		}
		s = s[:i]
	}
	return s
}
