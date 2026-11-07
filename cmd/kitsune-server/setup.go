package main

import (
	"crypto/rand"
	"encoding/base32"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

const (
	// setupCodeBytes is the setup code's randomness: 80 bits.
	setupCodeBytes = 10
	// setupCodeGroup is how many characters the code shows between dashes.
	setupCodeGroup = 4
)

// crockford is Crockford's base32 in lower case: no i, l, o or u, so the code
// reads aloud and copies by hand without confusion.
var crockford = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding) //nolint:gochecknoglobals // A fixed encoding, never written.

// newSetupCode returns a fresh setup code: 80 random bits as four groups of
// four lower-case Crockford base32 characters, such as k7m2-q9xd-4tbn-r8ve.
func newSetupCode() string {
	b := make([]byte, setupCodeBytes)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error; it crashes the program instead.
	groups := make([]string, 0, setupCodeGroup)
	for g := range slices.Chunk([]byte(crockford.EncodeToString(b)), setupCodeGroup) {
		groups = append(groups, string(g))
	}
	return strings.Join(groups, "-")
}

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
