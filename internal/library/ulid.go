package library

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	ulidAlphabet   = "0123456789ABCDEFGHJKMNPQRSTVWXYZ" // Crockford base32.
	ulidLen        = 26
	ulidCharBits   = 5
	ulidCharMask   = 1<<ulidCharBits - 1
	ulidTimeBytes  = 6
	ulidEntropyLen = 10
	ulidWordBits   = 64
	ulidMaxMillis  = 1<<48 - 1
)

var errULIDTime = errors.New("time outside the ULID range")

// NewID returns a new ULID for now, for a row the library doesn't make itself,
// such as a person's shuffle run.
//
// # Errors
//
// It fails only when the system's randomness does, or now is outside the ULID range.
func NewID(now time.Time) (string, error) { return newULID(now, rand.Reader) }

// newULID returns a ULID for t, reading its 80 random bits from entropy.
// It returns an error if t is outside the 48-bit millisecond range or entropy
// fails. ULIDs made within one millisecond are not ordered.
func newULID(t time.Time, entropy io.Reader) (string, error) {
	ms := t.UnixMilli()
	if ms < 0 || ms > ulidMaxMillis {
		return "", fmt.Errorf("%v: %w", t, errULIDTime)
	}
	// Layout: 48 bits of time, then 80 bits of entropy, big-endian: 128 bits.
	var b [ulidTimeBytes + ulidEntropyLen]byte
	var stamp [8]byte
	binary.BigEndian.PutUint64(stamp[:], uint64(ms))
	copy(b[:ulidTimeBytes], stamp[8-ulidTimeBytes:])
	if _, err := io.ReadFull(entropy, b[ulidTimeBytes:]); err != nil {
		return "", fmt.Errorf("read ULID entropy: %w", err)
	}
	hi, lo := binary.BigEndian.Uint64(b[:8]), binary.BigEndian.Uint64(b[8:])
	// 26 characters hold 130 bits: the first one carries the top 3 bits.
	var out [ulidLen]byte
	for j := range ulidLen { // j counts characters from the end.
		s := uint(j * ulidCharBits)
		var v uint64
		switch {
		case s >= ulidWordBits:
			v = hi >> (s - ulidWordBits)
		case s+ulidCharBits > ulidWordBits:
			v = lo>>s | hi<<(ulidWordBits-s)
		default:
			v = lo >> s
		}
		out[ulidLen-1-j] = ulidAlphabet[v&ulidCharMask]
	}
	return string(out[:]), nil
}
