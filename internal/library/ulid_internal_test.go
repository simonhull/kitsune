package library

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var errBoom = errors.New("boom")

type failReader struct{}

func (failReader) Read([]byte) (int, error) { return 0, errBoom }

func TestNewULIDKnownValue(t *testing.T) {
	t.Parallel()
	got, err := newULID(time.UnixMilli(1469918176385), bytes.NewReader(make([]byte, 10)))
	if err != nil {
		t.Fatal(err)
	}
	if want := "01ARYZ6S410000000000000000"; got != want {
		t.Errorf("newULID = %q, want %q", got, want)
	}
}

func TestNewULIDShape(t *testing.T) {
	t.Parallel()
	got, err := newULID(time.Now(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 26 {
		t.Fatalf("len(%q) = %d, want 26", got, len(got))
	}
	for _, c := range got {
		if !strings.ContainsRune(crockford, c) {
			t.Errorf("%q has %q outside the Crockford alphabet", got, c)
		}
	}
}

func TestNewULIDOrdersByTime(t *testing.T) {
	t.Parallel()
	now := time.Now()
	a, err := newULID(now, rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newULID(now.Add(time.Millisecond), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if a >= b {
		t.Errorf("%q should sort before %q", a, b)
	}
}

func TestNewULIDEntropyError(t *testing.T) {
	t.Parallel()
	if _, err := newULID(time.Now(), failReader{}); !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want it to wrap errBoom", err)
	}
}

func TestNewULIDMaxEntropy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		at      time.Time
		entropy byte
		want    string
	}{
		{"max entropy", time.UnixMilli(0), 0xFF, "0000000000ZZZZZZZZZZZZZZZZ"},
		{"max time", time.UnixMilli(1<<48 - 1), 0, "7ZZZZZZZZZ0000000000000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := newULID(tt.at, bytes.NewReader(bytes.Repeat([]byte{tt.entropy}, 10)))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("newULID = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewULIDTimeRange(t *testing.T) {
	t.Parallel()
	for _, at := range []time.Time{time.UnixMilli(-1), time.UnixMilli(1 << 48)} {
		if _, err := newULID(at, bytes.NewReader(make([]byte, 10))); !errors.Is(err, errULIDTime) {
			t.Errorf("newULID(%v) err = %v, want errULIDTime", at, err)
		}
	}
}
