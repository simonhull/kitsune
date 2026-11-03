package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

var (
	// errMalformedHash means a stored hash is not a valid Argon2id PHC string.
	errMalformedHash = errors.New("malformed password hash")
	// errBadParams means Argon2id parameters are outside the accepted bounds.
	errBadParams = errors.New("invalid Argon2id parameters")
)

// Argon2id defaults (RFC 9106, second recommended option) and encoding sizes.
const (
	defaultTime      = 3
	defaultMemoryKiB = 64 * 1024
	defaultThreads   = 4

	saltBytes = 16
	keyBytes  = 32

	minSaltBytes = 8
	maxSaltBytes = 64
	minKeyBytes  = 16
	maxKeyBytes  = 64

	maxTime      = 16
	maxThreads   = 16
	maxMemoryKiB = 1 << 20 // 1 GiB.

	minMemoryPerThread = 8 // KiB; Argon2 needs at least 8 blocks per lane.
	paramCount         = 3 // m, t and p.

	phcParts    = 6 // "", algorithm, version, parameters, salt, key.
	phcVersion  = "v=19"
	phcAlg      = "argon2id"
	decimalBase = 10
	bits32      = 32
	bits8       = 8
)

// Name and password limits.
const (
	maxNameRunes     = 64
	minPasswordRunes = 8
	maxPasswordBytes = 1024
)

// Params are Argon2id cost parameters.
type Params struct {
	Time      uint32 // Passes over memory.
	MemoryKiB uint32 // Memory in KiB.
	Threads   uint8  // Lanes.
}

// DefaultParams returns RFC 9106's second recommended option: t=3, 64 MiB, p=4.
func DefaultParams() Params {
	return Params{Time: defaultTime, MemoryKiB: defaultMemoryKiB, Threads: defaultThreads}
}

// parsedHash is a decoded PHC string.
type parsedHash struct {
	params    Params
	salt, key []byte
}

// checkParams reports whether p is within the bounds the parser accepts.
func checkParams(p Params) error {
	switch {
	case p.Time < 1 || p.Time > maxTime:
		return fmt.Errorf("%w: time %d outside 1-%d", errBadParams, p.Time, maxTime)
	case p.Threads < 1 || p.Threads > maxThreads:
		return fmt.Errorf("%w: threads %d outside 1-%d", errBadParams, p.Threads, maxThreads)
	case p.MemoryKiB < minMemoryPerThread*uint32(p.Threads) || p.MemoryKiB > maxMemoryKiB:
		return fmt.Errorf("%w: memory %d KiB outside %d-%d", errBadParams, p.MemoryKiB, minMemoryPerThread*uint32(p.Threads), maxMemoryKiB)
	}
	return nil
}

// hashPassword returns the PHC encoding of an Argon2id hash of password. p
// must already have passed checkParams.
func hashPassword(p Params, password string) string {
	salt := make([]byte, saltBytes)
	// Since Go 1.24 rand.Read never returns an error; it crashes the program instead.
	_, _ = rand.Read(salt)
	key := argon2.IDKey([]byte(password), salt, p.Time, p.MemoryKiB, p.Threads, keyBytes)
	return fmt.Sprintf("$%s$%s$m=%d,t=%d,p=%d$%s$%s", phcAlg, phcVersion, p.MemoryKiB, p.Time, p.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

// verifyPassword reports whether password matches the encoded hash. It also
// returns the parsed hash so callers can decide on a rehash.
func verifyPassword(encoded, password string) (bool, parsedHash, error) {
	ph, err := parseHash(encoded)
	if err != nil {
		return false, parsedHash{}, err
	}
	p := ph.params
	got := argon2.IDKey([]byte(password), ph.salt, p.Time, p.MemoryKiB, p.Threads, uint32(len(ph.key))) //nolint:gosec // G115: parseHash caps the key at maxKeyBytes.
	return subtle.ConstantTimeCompare(got, ph.key) == 1, ph, nil
}

// parseHash strictly decodes a PHC string.
func parseHash(encoded string) (parsedHash, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != phcParts || parts[0] != "" {
		return parsedHash{}, fmt.Errorf("%w: want %d fields", errMalformedHash, phcParts-1)
	}
	if parts[1] != phcAlg || parts[2] != phcVersion {
		return parsedHash{}, fmt.Errorf("%w: unsupported algorithm or version", errMalformedHash)
	}
	params, err := parseParams(parts[3])
	if err != nil {
		return parsedHash{}, err
	}
	salt, err := decodeB64(parts[4], minSaltBytes, maxSaltBytes, "salt")
	if err != nil {
		return parsedHash{}, err
	}
	key, err := decodeB64(parts[5], minKeyBytes, maxKeyBytes, "key")
	if err != nil {
		return parsedHash{}, err
	}
	return parsedHash{params: params, salt: salt, key: key}, nil
}

// parseParams decodes "m=..,t=..,p=.." and checks the bounds.
func parseParams(s string) (Params, error) {
	fields := strings.Split(s, ",")
	if len(fields) != paramCount {
		return Params{}, fmt.Errorf("%w: want %d parameters", errMalformedHash, paramCount)
	}
	var vals [paramCount]uint64
	for i, key := range [...]string{"m", "t", "p"} {
		v, ok := strings.CutPrefix(fields[i], key+"=")
		if !ok {
			return Params{}, fmt.Errorf("%w: want %s=", errMalformedHash, key)
		}
		bitSize := bits32
		if key == "p" {
			bitSize = bits8
		}
		n, err := strconv.ParseUint(v, decimalBase, bitSize)
		if err != nil {
			return Params{}, fmt.Errorf("%w: bad %s", errMalformedHash, key)
		}
		vals[i] = n
	}
	p := Params{MemoryKiB: uint32(vals[0]), Time: uint32(vals[1]), Threads: uint8(vals[2])} //nolint:gosec // G115: ParseUint with bitSize already bounded each value to its field's type.
	if err := checkParams(p); err != nil {
		return Params{}, fmt.Errorf("%w: %w", errMalformedHash, err)
	}
	return p, nil
}

// decodeB64 decodes unpadded standard base64 and checks the length bounds.
func decodeB64(s string, minLen, maxLen int, what string) ([]byte, error) {
	b, err := base64.RawStdEncoding.Strict().DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: bad %s encoding", errMalformedHash, what)
	}
	if len(b) < minLen || len(b) > maxLen {
		return nil, fmt.Errorf("%w: %s length %d outside %d-%d", errMalformedHash, what, len(b), minLen, maxLen)
	}
	return b, nil
}

// ValidateName reports whether name may be a user name. It returns an error
// wrapping [ErrInvalidName] if not.
func ValidateName(name string) error {
	n := utf8.RuneCountInString(name)
	switch {
	case !utf8.ValidString(name):
		return fmt.Errorf("%w: not valid UTF-8", ErrInvalidName)
	case n < 1 || n > maxNameRunes:
		return fmt.Errorf("%w: use 1-%d characters", ErrInvalidName, maxNameRunes)
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return fmt.Errorf("%w: control characters are not allowed", ErrInvalidName)
	case strings.TrimSpace(name) != name:
		return fmt.Errorf("%w: no leading or trailing spaces", ErrInvalidName)
	}
	return nil
}

// ValidatePassword reports whether password is acceptable. It returns an error
// wrapping [ErrPasswordTooShort] or [ErrPasswordTooLong] if not.
func ValidatePassword(password string) error {
	switch {
	case len(password) > maxPasswordBytes:
		return ErrPasswordTooLong
	case utf8.RuneCountInString(password) < minPasswordRunes:
		return ErrPasswordTooShort
	}
	return nil
}
