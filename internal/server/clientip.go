package server

import (
	"net/http"
	"net/netip"
	"slices"
	"strings"
)

// maxHopLen caps an X-Forwarded-For entry that is not an address, so a
// broken proxy can't make unbounded rate-limit keys.
const maxHopLen = 64

// ForwardedClientIP returns a Config.ClientIP that believes X-Forwarded-For
// only from trusted peers. With no trusted networks it returns the peer's
// address, as a nil ClientIP does.
//
// For a trusted peer it walks the header from the right and returns the first
// hop that is not trusted: everything left of the first trusted proxy's entry
// is whatever the client sent. The returned function is safe for concurrent
// use.
func ForwardedClientIP(trusted []netip.Prefix) func(*http.Request) string {
	if len(trusted) == 0 {
		return remoteHost
	}
	trusted = slices.Clone(trusted)
	isTrusted := func(a netip.Addr) bool {
		return slices.ContainsFunc(trusted, func(p netip.Prefix) bool { return p.Contains(a) })
	}
	return func(r *http.Request) string {
		peer := remoteHost(r)
		addr, err := netip.ParseAddr(peer)
		if err != nil || !isTrusted(normalize(addr)) {
			return peer
		}
		hops := forwardedHops(r.Header)
		for i, raw := range slices.Backward(hops) {
			hop, ok := parseHop(raw)
			if !ok {
				return raw[:min(len(raw), maxHopLen)]
			}
			if !isTrusted(hop) || i == 0 {
				return hop.String()
			}
		}
		return peer
	}
}

// normalize unmaps an IPv4-mapped address and drops the zone, because
// [netip.Prefix.Contains] is false for zoned addresses.
func normalize(a netip.Addr) netip.Addr { return a.Unmap().WithZone("") }

// forwardedHops returns the entries of every X-Forwarded-For header, in
// order, trimmed and without the empty ones.
func forwardedHops(h http.Header) []string {
	var hops []string
	for _, v := range h.Values("X-Forwarded-For") {
		for part := range strings.SplitSeq(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				hops = append(hops, part)
			}
		}
	}
	return hops
}

// parseHop parses an X-Forwarded-For entry: an address, or an address with a
// port, which some proxies append. The result is normalized.
func parseHop(s string) (netip.Addr, bool) {
	if a, err := netip.ParseAddr(s); err == nil {
		return normalize(a), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return normalize(ap.Addr()), true
	}
	return netip.Addr{}, false
}
