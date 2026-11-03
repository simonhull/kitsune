package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// Token shape. The prefix "ksk_" is reserved for API keys, which a later plan
// adds; Authenticate will dispatch on the prefix.
const (
	sessionPrefix = "kss_"
	tokenBytes    = 32
	// tokenLen is the length of a whole session token: the prefix plus 43 base64url characters.
	tokenLen = len(sessionPrefix) + 43
)

// mintedToken is a fresh session token with its hash.
type mintedToken struct {
	token string // Shown to the client once.
	hash  []byte // SHA-256 of token; the only form that is stored.
}

// newSessionToken returns a fresh session token and its hash.
func newSessionToken() mintedToken {
	raw := make([]byte, tokenBytes)
	// Since Go 1.24 rand.Read never returns an error; it crashes the program instead.
	_, _ = rand.Read(raw)
	token := sessionPrefix + base64.RawURLEncoding.EncodeToString(raw)
	return mintedToken{token: token, hash: hashToken(token)}
}

// hashToken returns the SHA-256 of the whole token string.
func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// looksLikeSessionToken reports whether token has a session token's prefix,
// length and alphabet, without touching the database.
func looksLikeSessionToken(token string) bool {
	body, ok := strings.CutPrefix(token, sessionPrefix)
	if !ok || len(token) != tokenLen {
		return false
	}
	return strings.IndexFunc(body, notBase64URL) < 0
}

// notBase64URL reports whether r is outside the base64url alphabet.
func notBase64URL(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		return false
	}
	return true
}
