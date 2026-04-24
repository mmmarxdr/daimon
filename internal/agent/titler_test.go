package agent

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/store"
)

// --- Fakes ---

type fakeTitleStore struct {
	mu     sync.Mutex
	convs  map[string]*store.Conversation
	saves  int
}

func newFakeTitleStore() *fakeTitleStore {
	return &fakeTitleStore{convs: map[string]*store.Conversation{}}
}

func (f *fakeTitleStore) put(c *store.Conversation) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *c
	f.convs[c.ID] = &cp
}

func (f *fakeTitleStore) LoadConversation(_ context.Context, id string) (*store.Conversation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.convs[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *c
	cp.Messages = append([]provider.ChatMessage(nil), c.Messages...)
	if c.Metadata != nil {
		cp.Metadata = map[string]string{}
		for k, v := range c.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp, nil
}

func (f *fakeTitleStore) SaveConversation(_ context.Context, c store.Conversation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves++
	f.convs[c.ID] = &c
	return nil
}

func (f *fakeTitleStore) saveCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saves
}

type fakeProv struct {
	resp      string
	err       error
	block     chan struct{} // if non-nil, Chat blocks on it before returning (for timeout tests)
	callCount int32
}

func (f *fakeProv) Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	atomic.AddInt32(&f.callCount, 1)
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return &provider.ChatResponse{Content: f.resp}, nil
}

// --- Helpers ---

func mkEligibleConv(id string) *store.Conversation {
	mk := func(role, text string) provider.ChatMessage {
		return provider.ChatMessage{
			Role:    role,
			Content: content.Blocks{{Type: content.BlockText, Text: text}},
		}
	}
	return &store.Conversation{
		ID:        id,
		ChannelID: "web:t1",
		Messages: []provider.ChatMessage{
			mk("user", strings.Repeat("quiero entender cómo funciona el RAG ", 2)),
			mk("assistant", "claro, veamos"),
			mk("user", "dale"),
			mk("assistant", "paso 1..."),
			mk("user", "ok"),
			mk("assistant", "paso 2..."),
		},
	}
}

func newTitler(st titleStoreAPI, prov titleProviderAPI) *TitleGenerator {
	cfg := config.TitleGenYAMLConfig{
		Enabled:       true,
		WorkerCount:   1,
		QueueSize:     4,
		CallTimeoutMS: 500,
	}
	return NewTitleGenerator(st, prov, cfg)
}

func waitForSaves(t *testing.T, st *fakeTitleStore, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st.saveCount() >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d saves, got %d", want, st.saveCount())
}

// --- D1. Enqueue non-blocking ---

func TestEnqueue_QueueFullDoesNotBlock(t *testing.T) {
	// WorkerCount=0 would make the channel fill up forever; use 1 worker
	// but block it with a blocking provider, so the queue fills.
	block := make(chan struct{})
	defer close(block)

	st := newFakeTitleStore()
	st.put(mkEligibleConv("conv_busy"))

	cfg := config.TitleGenYAMLConfig{
		Enabled: true, WorkerCount: 1, QueueSize: 2, CallTimeoutMS: 5000,
	}
	tg := NewTitleGenerator(st, &fakeProv{resp: "Title", block: block}, cfg)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tg.Stop(ctx)
	}()

	// Fill the queue: 1 worker holds a job + 2 queued = 3 sticky jobs.
	// The 4th Enqueue must return immediately without blocking.
	for i := 0; i < 3; i++ {
		tg.Enqueue(context.Background(), "conv_busy")
	}
	done := make(chan struct{})
	go func() {
		tg.Enqueue(context.Background(), "conv_busy") // should NOT block
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Enqueue blocked when the queue was full")
	}
}

// --- D3/D4. Worker execution ---

func TestTitler_SuccessfulGeneration(t *testing.T) {
	st := newFakeTitleStore()
	st.put(mkEligibleConv("conv_ok"))
	prov := &fakeProv{resp: "Entender el RAG"}

	tg := newTitler(st, prov)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tg.Stop(ctx)
	}()
	tg.Enqueue(context.Background(), "conv_ok")

	waitForSaves(t, st, 1)

	c, _ := st.LoadConversation(context.Background(), "conv_ok")
	if c.Metadata["title"] != "Entender el RAG" {
		t.Errorf("title: got %q, want %q", c.Metadata["title"], "Entender el RAG")
	}
}

func TestTitler_ProviderTimeoutIsSilent(t *testing.T) {
	st := newFakeTitleStore()
	st.put(mkEligibleConv("conv_to"))
	block := make(chan struct{})
	defer close(block)
	prov := &fakeProv{block: block} // will never return unless ctx cancels

	cfg := config.TitleGenYAMLConfig{
		Enabled: true, WorkerCount: 1, QueueSize: 4, CallTimeoutMS: 50, // 50ms
	}
	tg := NewTitleGenerator(st, prov, cfg)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tg.Stop(ctx)
	}()
	tg.Enqueue(context.Background(), "conv_to")

	// Wait for the timeout to fire.
	time.Sleep(200 * time.Millisecond)
	if st.saveCount() != 0 {
		t.Errorf("expected no save on timeout, got %d", st.saveCount())
	}
}

func TestTitler_EmptyResponseDropped(t *testing.T) {
	st := newFakeTitleStore()
	st.put(mkEligibleConv("conv_empty_resp"))
	prov := &fakeProv{resp: "   "}

	tg := newTitler(st, prov)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tg.Stop(ctx)
	}()
	tg.Enqueue(context.Background(), "conv_empty_resp")

	time.Sleep(200 * time.Millisecond)
	if st.saveCount() != 0 {
		t.Errorf("expected no save on empty response, got %d", st.saveCount())
	}
}

func TestTitler_DeletedConvIsSilentDrop(t *testing.T) {
	st := newFakeTitleStore()
	// Deliberately do NOT put the conv — LoadConversation will return ErrNotFound.
	prov := &fakeProv{resp: "Title"}

	tg := newTitler(st, prov)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tg.Stop(ctx)
	}()
	tg.Enqueue(context.Background(), "conv_gone")

	time.Sleep(150 * time.Millisecond)
	if atomic.LoadInt32(&prov.callCount) != 0 {
		t.Errorf("provider should not be called when conv is missing, got callCount=%d", prov.callCount)
	}
}

func TestTitler_AlreadyTitledIsSkipped(t *testing.T) {
	st := newFakeTitleStore()
	conv := mkEligibleConv("conv_titled")
	conv.Metadata = map[string]string{"title": "manual title"}
	st.put(conv)
	prov := &fakeProv{resp: "LLM Title"}

	tg := newTitler(st, prov)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tg.Stop(ctx)
	}()
	tg.Enqueue(context.Background(), "conv_titled")

	time.Sleep(150 * time.Millisecond)
	c, _ := st.LoadConversation(context.Background(), "conv_titled")
	if c.Metadata["title"] != "manual title" {
		t.Errorf("manual title overwritten: got %q", c.Metadata["title"])
	}
	if atomic.LoadInt32(&prov.callCount) != 0 {
		t.Errorf("provider should not be called when title already set, got callCount=%d", prov.callCount)
	}
}

func TestTitler_MediaBlocksOmittedFromPrompt(t *testing.T) {
	st := newFakeTitleStore()

	// Conv where turn 2 has a media block. The serialized prompt should
	// contain only text — the image block must not leak through.
	conv := mkEligibleConv("conv_media")
	conv.Messages[1].Content = content.Blocks{
		{Type: content.BlockText, Text: "(turn 2 text)"},
		{Type: content.BlockImage, MIME: "image/png", MediaSHA256: "binary-bytes-shouldnt-be-in-prompt", Size: 123},
	}
	st.put(conv)

	// Capture the prompt sent to Chat by using a custom provider.
	seen := make(chan string, 1)
	prov := &captureProv{respText: "Title OK", seen: seen}

	tg := newTitler(st, prov)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tg.Stop(ctx)
	}()
	tg.Enqueue(context.Background(), "conv_media")

	select {
	case prompt := <-seen:
		if strings.Contains(prompt, "binary-bytes-shouldnt-be-in-prompt") {
			t.Errorf("media block leaked into prompt: %q", prompt)
		}
		if !strings.Contains(prompt, "(turn 2 text)") {
			t.Errorf("text portion of media turn missing from prompt: %q", prompt)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("prompt never captured")
	}
}

type captureProv struct {
	respText string
	seen     chan<- string
}

func (c *captureProv) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	if len(req.Messages) > 0 {
		c.seen <- req.Messages[0].Content.TextOnly()
	}
	return &provider.ChatResponse{Content: c.respText}, nil
}

// --- D3. Shutdown ---

func TestTitler_GracefulShutdown(t *testing.T) {
	st := newFakeTitleStore()
	st.put(mkEligibleConv("conv_sd"))
	prov := &fakeProv{resp: "Title"}

	tg := newTitler(st, prov)

	tg.Enqueue(context.Background(), "conv_sd")
	time.Sleep(20 * time.Millisecond) // let the job start

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tg.Stop(ctx); err != nil {
		t.Errorf("graceful Stop should return nil, got %v", err)
	}
}

// stubbornProv ignores context cancellation (emulates an HTTP client stuck on
// a syscall that doesn't respect ctx). Needed to exercise the Stop-deadline
// path — our well-behaved fakeProv returns on ctx.Done, which is what most
// real providers do.
type stubbornProv struct {
	release chan struct{}
}

func (s *stubbornProv) Chat(_ context.Context, _ provider.ChatRequest) (*provider.ChatResponse, error) {
	<-s.release // blocks forever unless release is closed
	return &provider.ChatResponse{Content: "too late"}, nil
}

func TestTitler_ShutdownDeadlineExceeded(t *testing.T) {
	st := newFakeTitleStore()
	st.put(mkEligibleConv("conv_stuck"))
	release := make(chan struct{})
	defer close(release)
	prov := &stubbornProv{release: release}

	cfg := config.TitleGenYAMLConfig{
		Enabled: true, WorkerCount: 1, QueueSize: 4, CallTimeoutMS: 120000, // long
	}
	tg := NewTitleGenerator(st, prov, cfg)

	tg.Enqueue(context.Background(), "conv_stuck")
	time.Sleep(20 * time.Millisecond) // let the job start and be stuck

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	err := tg.Stop(ctx)
	if err == nil {
		t.Error("Stop with expired deadline should return non-nil error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

// --- normalizeTitle / serializeFirstTurns ---

func TestNormalizeTitle(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`Mi Nuevo Hilo`, `Mi Nuevo Hilo`},
		{`   trimmed   `, `trimmed`},
		{`"quoted"`, `quoted`},
		{"'single'", "single"},
		{"*bold*", "bold"},
		{"_italic_", "italic"},
		{"`code`", "code"},
		{"  \"*fancy*\"  ", "fancy"},
		{"line1\nline2", "line1 line2"},
		{"line1\r\nline2", "line1 line2"},
		{"tabs\t\tand    spaces", "tabs and spaces"},
		{"", ""},
		{strings.Repeat("x", 150), strings.Repeat("x", 100)},
	}
	for _, c := range cases {
		got := normalizeTitle(c.in)
		if got != c.want {
			t.Errorf("normalizeTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
