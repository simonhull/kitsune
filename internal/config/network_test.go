package config

import (
	"errors"
	"strings"
	"testing"
)

func TestNetworkUnmarshal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    string // Prefix.String(); "" means an error.
		wantMsg string // Extra substring of the error.
	}{
		{in: "10.1.2.3/8", want: "10.0.0.0/8"},
		{in: "192.168.1.5", want: "192.168.1.5/32"},
		{in: "fd00::/8", want: "fd00::/8"},
		{in: "2001:db8::1", want: "2001:db8::1/128"},
		{in: "fe80::1%eth0"},
		{in: "::ffff:10.0.0.1", wantMsg: "IPv4 form"},
		{in: "::ffff:10.0.0.0/104", wantMsg: "IPv4 form"},
		{in: "10.0.0.0/33"},
		{in: "example.com"},
		{in: ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			var n Network
			err := n.UnmarshalText([]byte(tt.in))
			if tt.want == "" {
				if !errors.Is(err, errNetwork) {
					t.Fatalf("UnmarshalText(%q) error = %v, want errNetwork", tt.in, err)
				}
				if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("error %q does not mention %q", err, tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalText(%q): %v", tt.in, err)
			}
			if got := n.String(); got != tt.want {
				t.Errorf("UnmarshalText(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}
