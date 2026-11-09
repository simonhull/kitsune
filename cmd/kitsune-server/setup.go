package main

import (
	"net"
	"net/netip"
	"strconv"
)

// setupURL is where the setup log line sends people: base_url when it is set,
// else the address the server listens on. An unspecified host (0.0.0.0 or ::)
// becomes localhost, which a browser on the server's machine can open.
func setupURL(baseURL string, listen net.Addr) string {
	if baseURL != "" {
		return baseURL
	}
	addr := listen.String()
	if ap, err := netip.ParseAddrPort(addr); err == nil && ap.Addr().IsUnspecified() {
		addr = net.JoinHostPort("localhost", strconv.Itoa(int(ap.Port())))
	}
	return "http://" + addr
}
