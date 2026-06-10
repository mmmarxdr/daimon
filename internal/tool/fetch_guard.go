package tool

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
)

// lookupIP is the package-level resolver used by validateFetchURL. Tests
// override this variable to inject deterministic stub resolvers without
// touching real DNS.
var lookupIP = net.LookupIP

// validateFetchURL checks that rawURL is safe to fetch:
//  1. Scheme must be http or https.
//  2. The resolved IP(s) must not fall in any private, loopback, link-local,
//     unspecified, or ULA range (covers IPv4 and IPv6, including cloud IMDS at
//     169.254.169.254 and DNS-rebinding attacks).
//
// resolver is the IP lookup function; pass nil to use the package-level
// lookupIP (which defaults to net.LookupIP).
func validateFetchURL(rawURL string, resolver func(string) ([]net.IP, error)) error {
	if resolver == nil {
		resolver = lookupIP
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q: only http/https are allowed", parsed.Scheme)
	}

	host := parsed.Hostname() // strips port, brackets around IPv6 literals
	if host == "" {
		return fmt.Errorf("unsupported URL scheme %q: only http/https are allowed", parsed.Scheme)
	}

	ips, err := resolver(host)
	if err != nil {
		return fmt.Errorf("resolving host %q: %w", host, err)
	}

	for _, ip := range ips {
		if err := checkIPAllowed(ip); err != nil {
			return err
		}
	}

	return nil
}

// cgnatBlock is the carrier-grade NAT range defined by RFC 6598 (100.64.0.0/10).
// addr.IsPrivate() does not cover this range, so it is checked explicitly.
var cgnatBlock = netip.MustParsePrefix("100.64.0.0/10")

// checkIPAllowed returns an error if ip falls in any range that must not be
// reachable from an agent-initiated fetch request.
func checkIPAllowed(ip net.IP) error {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return fmt.Errorf("blocked: could not parse resolved IP %v", ip)
	}
	// Unmap converts IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) to plain
	// IPv4 so that IsPrivate, IsLoopback, etc. behave correctly.
	addr = addr.Unmap()

	switch {
	case addr.IsLoopback():
		return fmt.Errorf("blocked: resolved IP %s is a loopback address", addr)
	case addr.IsLinkLocalUnicast():
		// Covers 169.254.0.0/16 (IPv4 link-local / cloud IMDS) and fe80::/10.
		return fmt.Errorf("blocked: resolved IP %s is a link-local address", addr)
	case addr.IsLinkLocalMulticast():
		return fmt.Errorf("blocked: resolved IP %s is a link-local multicast address", addr)
	case addr.IsPrivate():
		// Covers 10/8, 172.16/12, 192.168/16, fc00::/7 (IPv6 ULA).
		// NOTE: IsPrivate does NOT include loopback or link-local — those are
		// checked above.
		return fmt.Errorf("blocked: resolved IP %s is a private address", addr)
	case cgnatBlock.Contains(addr):
		// RFC 6598: 100.64.0.0/10 — carrier-grade NAT, not covered by IsPrivate.
		return fmt.Errorf("blocked: resolved IP %s is in CGNAT range (RFC 6598)", addr)
	case addr.IsUnspecified():
		// 0.0.0.0 or ::.
		return fmt.Errorf("blocked: resolved IP %s is an unspecified address", addr)
	}

	return nil
}
