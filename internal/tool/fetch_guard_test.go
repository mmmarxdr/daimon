package tool

import (
	"net"
	"strings"
	"testing"
)

// stubLookupIP returns a resolver that maps exact hostnames to fixed IP slices.
// Unrecognised hostnames return a "no such host" error.
func stubLookupIP(mapping map[string][]net.IP) func(string) ([]net.IP, error) {
	return func(host string) ([]net.IP, error) {
		ips, ok := mapping[host]
		if !ok {
			return nil, &net.DNSError{Err: "no such host", Name: host}
		}
		return ips, nil
	}
}

// ---------------------------------------------------------------------------
// T-GUARD-1 — scheme allowlist: only http/https pass
// ---------------------------------------------------------------------------

func TestValidateFetchURL_SchemeAllowlist(t *testing.T) {
	pass := stubLookupIP(map[string][]net.IP{
		"example.com": {net.ParseIP("93.184.216.34")},
	})

	cases := []struct {
		name    string
		rawURL  string
		wantErr bool
		errSub  string
	}{
		{
			name:    "http scheme accepted",
			rawURL:  "http://example.com/path",
			wantErr: false,
		},
		{
			name:    "https scheme accepted",
			rawURL:  "https://example.com/path",
			wantErr: false,
		},
		{
			name:    "file scheme rejected",
			rawURL:  "file:///etc/passwd",
			wantErr: true,
			errSub:  "scheme",
		},
		{
			name:    "gopher scheme rejected",
			rawURL:  "gopher://example.com/",
			wantErr: true,
			errSub:  "scheme",
		},
		{
			name:    "ftp scheme rejected",
			rawURL:  "ftp://example.com/file",
			wantErr: true,
			errSub:  "scheme",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFetchURL(tc.rawURL, pass)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.rawURL)
				}
				if tc.errSub != "" && !strings.Contains(err.Error(), tc.errSub) {
					t.Errorf("expected error to contain %q, got: %v", tc.errSub, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tc.rawURL, err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-GUARD-2 — private / loopback / link-local IP ranges rejected
// ---------------------------------------------------------------------------

func TestValidateFetchURL_PrivateIPRanges(t *testing.T) {
	cases := []struct {
		name   string
		rawURL string
		ip     string
	}{
		{
			name:   "cloud IMDS 169.254.169.254 rejected",
			rawURL: "http://169.254.169.254/latest/meta-data/",
			ip:     "169.254.169.254",
		},
		{
			name:   "RFC1918 10.x rejected",
			rawURL: "http://10.0.0.1/internal",
			ip:     "10.0.0.1",
		},
		{
			name:   "RFC1918 172.16.x rejected",
			rawURL: "http://172.16.0.1/internal",
			ip:     "172.16.0.1",
		},
		{
			name:   "RFC1918 192.168.x rejected",
			rawURL: "http://192.168.1.1/internal",
			ip:     "192.168.1.1",
		},
		{
			name:   "loopback 127.0.0.1 rejected",
			rawURL: "http://127.0.0.1/local",
			ip:     "127.0.0.1",
		},
		{
			name:   "loopback 127.x.x.x rejected",
			rawURL: "http://127.0.0.2/local",
			ip:     "127.0.0.2",
		},
		{
			name:   "IPv6 loopback ::1 rejected",
			rawURL: "http://[::1]/local",
			ip:     "::1",
		},
		{
			name:   "IPv6 ULA fc00:: rejected",
			rawURL: "http://[fc00::1]/internal",
			ip:     "fc00::1",
		},
		{
			name:   "IPv6 link-local fe80:: rejected",
			rawURL: "http://[fe80::1]/internal",
			ip:     "fe80::1",
		},
		{
			name:   "unspecified 0.0.0.0 rejected",
			rawURL: "http://0.0.0.0/",
			ip:     "0.0.0.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The URL already contains a literal IP; the resolver is invoked on
			// the host part. Provide an identity resolver for the few cases where
			// the host is NOT a bare IP (there are none here, but for safety).
			resolver := stubLookupIP(map[string][]net.IP{
				tc.ip: {net.ParseIP(tc.ip)},
			})
			err := validateFetchURL(tc.rawURL, resolver)
			if err == nil {
				t.Fatalf("expected error for private/loopback IP %q in URL %q, got nil", tc.ip, tc.rawURL)
			}
			if !strings.Contains(err.Error(), "blocked") {
				t.Errorf("expected 'blocked' in error message, got: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-GUARD-3 — DNS rebinding: hostname that resolves to a private IP is rejected
// ---------------------------------------------------------------------------

func TestValidateFetchURL_DNSRebinding(t *testing.T) {
	// Simulate 169.254.169.254.attacker.com resolving to the IMDS IP.
	rebindResolver := stubLookupIP(map[string][]net.IP{
		"169.254.169.254.attacker.com": {net.ParseIP("169.254.169.254")},
		"internal.corp.local":          {net.ParseIP("10.10.10.10")},
	})

	cases := []struct {
		name   string
		rawURL string
	}{
		{
			name:   "rebind hostname resolving to IMDS IP rejected",
			rawURL: "http://169.254.169.254.attacker.com/",
		},
		{
			name:   "internal corp hostname resolving to private IP rejected",
			rawURL: "http://internal.corp.local/api",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFetchURL(tc.rawURL, rebindResolver)
			if err == nil {
				t.Fatalf("expected DNS-rebinding to be caught for %q, got nil", tc.rawURL)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-GUARD-4 — a normal public URL passes the guard
// ---------------------------------------------------------------------------

func TestValidateFetchURL_PublicURLPasses(t *testing.T) {
	publicResolver := stubLookupIP(map[string][]net.IP{
		"example.com":    {net.ParseIP("93.184.216.34")},
		"8.8.8.8":        {net.ParseIP("8.8.8.8")},
		"www.google.com": {net.ParseIP("142.250.0.1")},
	})

	cases := []struct {
		name   string
		rawURL string
	}{
		{
			name:   "http public hostname passes",
			rawURL: "http://example.com/path",
		},
		{
			name:   "https public hostname passes",
			rawURL: "https://example.com/path",
		},
		{
			name:   "public literal IP passes",
			rawURL: "http://8.8.8.8/dns-query",
		},
		{
			name:   "https www hostname passes",
			rawURL: "https://www.google.com/search",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFetchURL(tc.rawURL, publicResolver)
			if err != nil {
				t.Fatalf("unexpected error for public URL %q: %v", tc.rawURL, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-GUARD-5 — invalid / empty URL handled gracefully
// ---------------------------------------------------------------------------

func TestValidateFetchURL_InvalidURL(t *testing.T) {
	noopResolver := stubLookupIP(nil)

	cases := []struct {
		name   string
		rawURL string
	}{
		{name: "empty string", rawURL: ""},
		{name: "no scheme", rawURL: "example.com/path"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFetchURL(tc.rawURL, noopResolver)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tc.rawURL)
			}
		})
	}
}
