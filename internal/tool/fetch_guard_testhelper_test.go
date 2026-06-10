package tool

import (
	"net"
)

// localhostPassResolver is an injectable resolver for tests that use
// httptest.NewServer (which always binds to 127.0.0.1). For loopback
// addresses it returns an empty IP slice so validateFetchURL skips the
// IP-range check (scheme is still validated). All other hosts are
// delegated to net.LookupIP.
func localhostPassResolver(host string) ([]net.IP, error) {
	if host == "127.0.0.1" || host == "::1" || host == "localhost" {
		// Return empty — no IPs to check; scheme guard already ran.
		return nil, nil
	}
	return lookupIP(host)
}

// withHTTPFetchResolver returns a copy of the tool with the given resolver.
// Used exclusively in tests to avoid the SSRF guard blocking loopback addresses
// used by httptest servers.
func withHTTPFetchResolver(t *HTTPFetchTool, r func(string) ([]net.IP, error)) *HTTPFetchTool {
	copy := *t
	copy.resolver = r
	return &copy
}

// withWebFetchResolver returns a copy of the tool with the given resolver.
func withWebFetchResolver(t *WebFetchTool, r func(string) ([]net.IP, error)) *WebFetchTool {
	copy := *t
	copy.resolver = r
	return &copy
}
