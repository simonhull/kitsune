package config

import (
	"errors"
	"fmt"
	"net/netip"
)

// errNetwork reports text that is neither an IP address nor a CIDR network.
var errNetwork = errors.New(`want an IP address or CIDR network, such as "172.18.0.0/16"`)

// Network is an IP network. In TOML it is a string: CIDR notation, or a
// bare address for just that address.
type Network struct{ netip.Prefix }

// UnmarshalText implements [encoding.TextUnmarshaler]. It accepts CIDR
// notation, stored masked, or a bare address, stored as a single-address
// network. It rejects zoned and IPv4-mapped IPv6 addresses: the server
// compares unmapped addresses, which a mapped network never contains.
func (n *Network) UnmarshalText(text []byte) error {
	s := string(text)
	p, err := netip.ParsePrefix(s)
	if err != nil {
		a, addrErr := netip.ParseAddr(s)
		if addrErr != nil || a.Zone() != "" {
			return fmt.Errorf("%w: got %q", errNetwork, text)
		}
		p = netip.PrefixFrom(a, a.BitLen())
	}
	if p.Addr().Is4In6() {
		return fmt.Errorf("%w: got %q; use the IPv4 form, because an IPv4-mapped network never matches", errNetwork, text)
	}
	*n = Network{Prefix: p.Masked()}
	return nil
}
