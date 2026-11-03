package auth

import (
	"bytes"
	"crypto/sha256"
	"regexp"
	"strings"
	"testing"
)

func TestNewSessionToken(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`^kss_[A-Za-z0-9_-]{43}$`)
	seen := make(map[string]bool)
	for range 100 {
		m := newSessionToken()
		token := m.token
		if !re.MatchString(token) {
			t.Fatalf("token %q has the wrong shape", token)
		}
		if want := sha256.Sum256([]byte(token)); !bytes.Equal(m.hash, want[:]) {
			t.Fatal("hash is not SHA-256 of the token")
		}
		if seen[token] {
			t.Fatal("duplicate token")
		}
		seen[token] = true
	}
}

func TestLooksLikeSessionToken(t *testing.T) {
	t.Parallel()
	token := newSessionToken().token
	body := strings.TrimPrefix(token, "kss_")
	if !looksLikeSessionToken(token) {
		t.Error("a fresh token was rejected")
	}
	for name, s := range map[string]string{
		"empty":         "",
		"no prefix":     body,
		"api key":       "ksk_" + body,
		"short":         "kss_" + body[:42],
		"long":          "kss_" + body + "A",
		"standard char": "kss_" + body[:42] + "+",
	} {
		if looksLikeSessionToken(s) {
			t.Errorf("%s: accepted", name)
		}
	}
}
