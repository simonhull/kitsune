package server

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

const (
	proxyPeer   = "172.18.0.2:5000"
	outsider    = "203.0.113.9:5000"
	outsiderIP  = "203.0.113.9"
	clientAddr  = "198.51.100.7"
	spoofedAddr = "6.6.6.6"
	dockerNet   = "172.18.0.0/16"
	tenNet      = "10.0.0.0/8"
	linkLocal   = "fe80::/10"
	longGarbage = 200
)

func prefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

func forwardedRequest(t *testing.T, remote string, xff ...string) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
	r.RemoteAddr = remote
	for _, v := range xff {
		r.Header.Add("X-Forwarded-For", v)
	}
	return r
}

func TestForwardedClientIP(t *testing.T) {
	t.Parallel()
	garbage := strings.Repeat("x", longGarbage)
	tests := []struct {
		name    string
		trusted []string
		remote  string
		xff     []string
		want    string
	}{
		{name: "no trusted networks", remote: outsider, xff: []string{"1.2.3.4"}, want: outsiderIP},
		{name: "untrusted peer", trusted: []string{dockerNet}, remote: outsider, xff: []string{"1.2.3.4"}, want: outsiderIP},
		{name: "one proxy", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{clientAddr}, want: clientAddr},
		{name: "spoofed left part", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{spoofedAddr + ", " + clientAddr}, want: clientAddr},
		{name: "two trusted hops", trusted: []string{dockerNet, tenNet}, remote: proxyPeer, xff: []string{clientAddr + ", 10.0.0.5"}, want: clientAddr},
		{name: "several header lines", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{spoofedAddr, clientAddr}, want: clientAddr},
		{name: "all trusted", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{"172.18.0.9"}, want: "172.18.0.9"},
		{name: "no header", trusted: []string{dockerNet}, remote: proxyPeer, want: "172.18.0.2"},
		{name: "hop with port", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{clientAddr + ":443"}, want: clientAddr},
		{name: "IPv6 hop with port", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{"[2001:db8::7]:443"}, want: "2001:db8::7"},
		{name: "IPv4-mapped peer", trusted: []string{dockerNet}, remote: "[::ffff:172.18.0.2]:5000", xff: []string{clientAddr}, want: clientAddr},
		{name: "IPv4-mapped hop", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{"::ffff:" + clientAddr}, want: clientAddr},
		{name: "zoned hop", trusted: []string{dockerNet, linkLocal}, remote: proxyPeer, xff: []string{spoofedAddr + ", fe80::1%eth0"}, want: spoofedAddr},
		{name: "garbage hop", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{spoofedAddr + ", not-an-ip"}, want: "not-an-ip"},
		{name: "long garbage hop", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{garbage}, want: garbage[:maxHopLen]},
		{name: "empty entries", trusted: []string{dockerNet}, remote: proxyPeer, xff: []string{clientAddr + ", , "}, want: clientAddr},
		{name: "unparsable peer", trusted: []string{dockerNet}, remote: "pipe", xff: []string{clientAddr}, want: "pipe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fn := ForwardedClientIP(prefixes(t, tt.trusted...))
			if got := fn(forwardedRequest(t, tt.remote, tt.xff...)); got != tt.want {
				t.Errorf("ForwardedClientIP(%q)(%q, %q) = %q, want %q", tt.trusted, tt.remote, tt.xff, got, tt.want)
			}
		})
	}
}

func TestForwardedClientIPCopiesTrusted(t *testing.T) {
	t.Parallel()
	trusted := prefixes(t, dockerNet)
	fn := ForwardedClientIP(trusted)
	trusted[0] = netip.MustParsePrefix("0.0.0.0/0")
	if got := fn(forwardedRequest(t, outsider, clientAddr)); got != outsiderIP {
		t.Errorf("result = %q after the input changed, want %q", got, outsiderIP)
	}
}
