package tool

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"daimon/internal/config"
)

// ---------------------------------------------------------------------------
// T-HTTP-1 — redirect to private/IMDS IP is blocked (FIX 1 — redirect bypass)
// ---------------------------------------------------------------------------

// TestHTTPFetchTool_RedirectToPrivateIPBlocked verifies that a 302 redirect
// from an attacker-controlled server pointing at a private/IMDS IP is caught
// by the SSRF guard and does not succeed.
func TestHTTPFetchTool_RedirectToPrivateIPBlocked(t *testing.T) {
	// Simulate an attacker-controlled server that redirects to IMDS.
	attackerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer attackerSrv.Close()

	// Stub resolver: attacker host passes (returns no IPs → only scheme is
	// checked), IMDS hostname resolves to the link-local IP.
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

	tool := withHTTPFetchResolver(NewHTTPFetchTool(config.HTTPToolConfig{
		Timeout: 5 * time.Second,
	}), imdsResolver)

	params, _ := json.Marshal(map[string]interface{}{
		"url":    attackerSrv.URL,
		"method": "GET",
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
// T-HTTP-2 — normal GET request succeeds (baseline sanity)
// ---------------------------------------------------------------------------

func TestHTTPFetchTool_BasicGET(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello from test server"))
	}))
	defer srv.Close()

	tool := withHTTPFetchResolver(NewHTTPFetchTool(config.HTTPToolConfig{
		Timeout: 5 * time.Second,
	}), localhostPassResolver)

	params, _ := json.Marshal(map[string]interface{}{
		"url":    srv.URL,
		"method": "GET",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected hard error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "hello from test server") {
		t.Errorf("expected response body in result, got: %s", result.Content)
	}
}
