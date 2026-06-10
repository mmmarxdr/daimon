package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daimon/internal/config"
)

// boolPtr returns a pointer to the given bool value.
func boolPtr(v bool) *bool { return &v }

// articleHTML is a minimal but realistic HTML article for readability testing.
const articleHTML = `<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>Test Article Title</title></head>
<body>
  <header><nav>Nav stuff</nav></header>
  <article>
    <h1>Test Article Title</h1>
    <p>This is the first paragraph of the article. It contains meaningful content that readability should extract and return as the main body of the document.</p>
    <p>This is a second paragraph with more content. Readability works best when there is enough text to identify the main body of the page. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.</p>
    <p>A third paragraph ensures we have plenty of content for the readability algorithm to work with. The more text available, the better the extraction quality tends to be overall.</p>
  </article>
  <footer><p>Footer content — not part of the article.</p></footer>
</body>
</html>`

// thinHTML is a page with very little content — triggers the Jina fallback.
const thinHTML = `<!DOCTYPE html>
<html><head><title>JS App</title></head>
<body><div id="root"></div></body>
</html>`

// jinaResponseBody is what the mock Jina server returns.
const jinaResponseBody = `# Jina Extracted Content

This is rich content returned by the Jina Reader API. It contains enough text to be considered a complete article for the purposes of these tests.`

// ---------------------------------------------------------------------------
// T4.1 — NewWebFetchTool applies defaults correctly
// ---------------------------------------------------------------------------

func TestNewWebFetchTool_Defaults(t *testing.T) {
	cfg := config.WebFetchConfig{
		Enabled: boolPtr(true),
	}
	tool := NewWebFetchTool(cfg)
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}
	if tool.client.Timeout != 20*time.Second {
		t.Errorf("expected 20s default timeout, got %v", tool.client.Timeout)
	}
}

// ---------------------------------------------------------------------------
// T4.2 — Name, Description, Schema
// ---------------------------------------------------------------------------

func TestWebFetchTool_Metadata(t *testing.T) {
	tool := NewWebFetchTool(config.WebFetchConfig{Enabled: boolPtr(true)})

	if tool.Name() != "web_fetch" {
		t.Errorf("expected name 'web_fetch', got %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("description must not be empty")
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(tool.Schema(), &schema); err != nil {
		t.Fatalf("Schema() is not valid JSON: %v", err)
	}
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("schema missing 'properties'")
	}
	if _, ok := props["url"]; !ok {
		t.Error("schema missing 'url' property")
	}
	if _, ok := props["extract_content"]; !ok {
		t.Error("schema missing 'extract_content' property")
	}
}

// ---------------------------------------------------------------------------
// T4.3 — Tier 1: successful readability extraction
// ---------------------------------------------------------------------------

func TestWebFetchTool_Tier1_Extraction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, articleHTML)
	}))
	defer srv.Close()

	tool := withWebFetchResolver(NewWebFetchTool(config.WebFetchConfig{
		Enabled: boolPtr(true),
		Timeout: 10 * time.Second,
	}), localhostPassResolver)

	params, _ := json.Marshal(map[string]interface{}{
		"url":             srv.URL,
		"extract_content": true,
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "paragraph") {
		t.Errorf("expected article content in result, got:\n%s", result.Content)
	}
	if result.Meta["tier"] != "1" {
		t.Errorf("expected tier '1', got %q", result.Meta["tier"])
	}
	if result.Meta["extracted"] != "true" {
		t.Errorf("expected extracted='true', got %q", result.Meta["extracted"])
	}
}

// ---------------------------------------------------------------------------
// T4.4 — extract_content=false returns raw body
// ---------------------------------------------------------------------------

func TestWebFetchTool_RawMode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, articleHTML)
	}))
	defer srv.Close()

	tool := withWebFetchResolver(NewWebFetchTool(config.WebFetchConfig{
		Enabled: boolPtr(true),
		Timeout: 10 * time.Second,
	}), localhostPassResolver)

	params, _ := json.Marshal(map[string]interface{}{
		"url":             srv.URL,
		"extract_content": false,
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	// Raw mode should include the HTML markup.
	if !strings.Contains(result.Content, "<html") {
		t.Errorf("expected raw HTML in result, got:\n%s", result.Content)
	}
	if result.Meta["tier"] != "raw" {
		t.Errorf("expected tier 'raw', got %q", result.Meta["tier"])
	}
}

// ---------------------------------------------------------------------------
// T4.5 — Blocked domain returns error result (not hard error)
// ---------------------------------------------------------------------------

func TestWebFetchTool_BlockedDomain(t *testing.T) {
	tool := NewWebFetchTool(config.WebFetchConfig{
		Enabled:        boolPtr(true),
		Timeout:        5 * time.Second,
		BlockedDomains: []string{"evil.example.com"},
	})

	params, _ := json.Marshal(map[string]interface{}{
		"url": "http://evil.example.com/page",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for blocked domain")
	}
	if !strings.Contains(result.Content, "blocked") {
		t.Errorf("expected 'blocked' in error message, got: %s", result.Content)
	}
}

// ---------------------------------------------------------------------------
// T4.6 — Invalid URL returns error result
// ---------------------------------------------------------------------------

func TestWebFetchTool_InvalidURL(t *testing.T) {
	tool := NewWebFetchTool(config.WebFetchConfig{
		Enabled: boolPtr(true),
		Timeout: 5 * time.Second,
	})

	params, _ := json.Marshal(map[string]interface{}{
		"url": "not-a-valid-url",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid URL scheme")
	}
}

// ---------------------------------------------------------------------------
// T4.7 — Tier 2: Jina fallback when content is thin
// ---------------------------------------------------------------------------

func TestWebFetchTool_JinaFallback(t *testing.T) {
	// Mock origin — returns thin HTML.
	originSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, thinHTML)
	}))
	defer originSrv.Close()

	// Mock Jina endpoint — we need to intercept the r.jina.ai call.
	// Since the tool hardcodes r.jina.ai, we inject a round-tripper that
	// redirects Jina URLs to our mock server.
	jinaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		fmt.Fprint(w, jinaResponseBody)
	}))
	defer jinaSrv.Close()

	cfg := config.WebFetchConfig{
		Enabled:     boolPtr(true),
		Timeout:     10 * time.Second,
		JinaEnabled: true,
	}
	tool := withWebFetchResolver(NewWebFetchTool(cfg), localhostPassResolver)

	// Override the HTTP client with a custom transport that rewrites Jina URLs
	// to our local mock server.
	tool.client = &http.Client{
		Timeout: 10 * time.Second,
		Transport: &jinaRedirectTransport{
			jinaBase: jinaSrv.URL,
		},
	}

	params, _ := json.Marshal(map[string]interface{}{
		"url":             originSrv.URL,
		"extract_content": true,
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Jina") {
		t.Errorf("expected Jina content in result, got:\n%s", result.Content)
	}
	if result.Meta["tier"] != "2" {
		t.Errorf("expected tier '2' (Jina), got %q", result.Meta["tier"])
	}
}

// ---------------------------------------------------------------------------
// T4.8 — Max response size truncation
// ---------------------------------------------------------------------------

func TestWebFetchTool_Truncation(t *testing.T) {
	bigContent := strings.Repeat("a", 2000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, bigContent)
	}))
	defer srv.Close()

	tool := withWebFetchResolver(NewWebFetchTool(config.WebFetchConfig{
		Enabled:         boolPtr(true),
		Timeout:         5 * time.Second,
		MaxResponseSize: "1KB", // 1024 bytes — body is 2000 bytes
	}), localhostPassResolver)

	params, _ := json.Marshal(map[string]interface{}{
		"url":             srv.URL,
		"extract_content": false,
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "truncated") {
		t.Errorf("expected truncation notice in result, got:\n%s", result.Content)
	}
}

// ---------------------------------------------------------------------------
// T4.9 — Redirect to private/IMDS IP is blocked (FIX 1 — redirect bypass)
// ---------------------------------------------------------------------------

// TestWebFetchTool_RedirectToPrivateIPBlocked verifies that a 302 redirect
// from an attacker-controlled server pointing at a private/IMDS IP is caught
// by the SSRF guard and does not succeed.
func TestWebFetchTool_RedirectToPrivateIPBlocked(t *testing.T) {
	// Simulate a server that redirects to an internal IP.
	attackerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer attackerSrv.Close()

	// Stub resolver: attacker host passes, IMDS host resolves to the link-local IP.
	imdsResolver := func(host string) ([]net.IP, error) {
		switch host {
		case "127.0.0.1", "::1", "localhost":
			return nil, nil // allow loopback for httptest server
		case "169.254.169.254":
			return []net.IP{net.ParseIP("169.254.169.254")}, nil
		default:
			return nil, nil // allow attacker server host
		}
	}

	tool := withWebFetchResolver(NewWebFetchTool(config.WebFetchConfig{
		Enabled: boolPtr(true),
		Timeout: 5 * time.Second,
	}), imdsResolver)

	params, _ := json.Marshal(map[string]interface{}{
		"url":             attackerSrv.URL,
		"extract_content": false,
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected redirect to private IP to be blocked, but got success: %s", result.Content)
	}
	if !strings.Contains(result.Content, "blocked") && !strings.Contains(result.Content, "link-local") {
		t.Errorf("expected 'blocked' or 'link-local' in error, got: %s", result.Content)
	}
}

// ---------------------------------------------------------------------------
// jinaRedirectTransport rewrites requests to r.jina.ai/* to a local mock.
// ---------------------------------------------------------------------------

type jinaRedirectTransport struct {
	jinaBase string
}

func (tr *jinaRedirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "r.jina.ai" {
		// rewrite host to local mock
		newURL := *req.URL
		newURL.Host = strings.TrimPrefix(tr.jinaBase, "http://")
		newURL.Scheme = "http"
		newReq := req.Clone(req.Context())
		newReq.URL = &newURL
		newReq.Host = newURL.Host
		return http.DefaultTransport.RoundTrip(newReq)
	}
	return http.DefaultTransport.RoundTrip(req)
}
