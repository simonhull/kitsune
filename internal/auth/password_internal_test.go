package auth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

const (
	testPassword = "correct horse"
	testSalt     = "c2FsdHNhbHQ"
	testKey      = "a2V5a2V5a2V5a2V5a2V5aw"
)

func cheap() Params { return Params{Time: 1, MemoryKiB: 64, Threads: 1} }

func TestHashPasswordFormat(t *testing.T) {
	t.Parallel()
	re := regexp.MustCompile(`^\$argon2id\$v=19\$m=64,t=1,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)
	if got := hashPassword(cheap(), testPassword); !re.MatchString(got) {
		t.Errorf("hash %q does not match PHC format", got)
	}
}

func TestHashPasswordSalts(t *testing.T) {
	t.Parallel()
	first, second := hashPassword(cheap(), testPassword), hashPassword(cheap(), testPassword)
	if first == second {
		t.Error("two hashes of the same password are identical")
	}
}

func TestVerifyPassword(t *testing.T) {
	t.Parallel()
	h := hashPassword(cheap(), testPassword)
	if _, ph, err := verifyPassword(h, testPassword); err != nil || ph.params != cheap() {
		t.Errorf("verifyPassword parsed %+v, %v; want %+v", ph.params, err, cheap())
	}
	for _, tt := range []struct {
		password string
		want     bool
	}{{testPassword, true}, {"wrong horse", false}, {"", false}} {
		got, _, err := verifyPassword(h, tt.password)
		if err != nil || got != tt.want {
			t.Errorf("verifyPassword(%q) = %v, %v; want %v, nil", tt.password, got, err, tt.want)
		}
	}
}

func b64(n int) string { return base64.RawStdEncoding.EncodeToString(make([]byte, n)) }

func TestVerifyPasswordMalformed(t *testing.T) {
	t.Parallel()
	phc := func(params, salt, key string) string {
		return fmt.Sprintf("$argon2id$v=19$%s$%s$%s", params, salt, key)
	}
	const ok = "m=64,t=1,p=1"
	tests := map[string]string{
		"wrong algorithm": "$argon2i$v=19$m=64,t=1,p=1$" + testSalt + "$" + testKey,
		"wrong version":   "$argon2id$v=18$m=64,t=1,p=1$" + testSalt + "$" + testKey,
		"missing key":     "$argon2id$v=19$m=64,t=1,p=1$" + testSalt,
		"extra field":     phc(ok, testSalt, testKey) + "$extra",
		"bad salt":        phc(ok, "!!!!", testKey),
		"t=0":             phc("m=64,t=0,p=1", testSalt, testKey),
		"p=0":             phc("m=64,t=1,p=0", testSalt, testKey),
		"m under 8p":      phc("m=4,t=1,p=1", testSalt, testKey),
		"m over 1 GiB":    phc("m=2097152,t=1,p=1", testSalt, testKey),
		"t=17":            phc("m=64,t=17,p=1", testSalt, testKey),
		"short salt":      phc(ok, b64(4), testKey),
		"short key":       phc(ok, testSalt, b64(8)),
		"long salt":       phc(ok, b64(65), testKey),
		"long key":        phc(ok, testSalt, b64(65)),
		"trailing junk":   phc("m=64,t=1,p=1x", testSalt, testKey),
		"empty":           "",
	}
	for name, h := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, _, err := verifyPassword(h, testPassword)
			if got || !errors.Is(err, errMalformedHash) {
				t.Errorf("verifyPassword = %v, %v; want false, errMalformedHash", got, err)
			}
		})
	}
}

func TestParamsOf(t *testing.T) {
	t.Parallel()
	p, err := parseHash(hashPassword(cheap(), "another password"))
	if err != nil {
		t.Fatal(err)
	}
	if p.params != cheap() || len(p.salt) != saltBytes || len(p.key) != keyBytes {
		t.Errorf("parsed = %+v (salt %d, key %d)", p.params, len(p.salt), len(p.key))
	}
}

func TestCheckParams(t *testing.T) {
	t.Parallel()
	bad := map[string]Params{
		"time 0":      {Time: 0, MemoryKiB: 64, Threads: 1},
		"threads 0":   {Time: 1, MemoryKiB: 64, Threads: 0},
		"memory < 8p": {Time: 1, MemoryKiB: 15, Threads: 2},
		"time 17":     {Time: 17, MemoryKiB: 64, Threads: 1},
	}
	for name, p := range bad {
		if err := checkParams(p); !errors.Is(err, errBadParams) {
			t.Errorf("%s: checkParams = %v, want rejection", name, err)
		}
	}
	for _, p := range []Params{DefaultParams(), cheap()} {
		if err := checkParams(p); err != nil {
			t.Errorf("checkParams(%+v) = %v", p, err)
		}
	}
}

func TestValidateName(t *testing.T) {
	t.Parallel()
	for _, n := range []string{"simon", "Ünïcödé", "a", strings.Repeat("é", 64)} {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v", n, err)
		}
	}
	for _, n := range []string{"", " simon", "simon ", "si\x00mon", "si\u0085mon", "\xff", strings.Repeat("a", 65)} {
		if err := ValidateName(n); !errors.Is(err, ErrInvalidName) {
			t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", n, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		pw   string
		want error
	}{
		{"7 runes", "abcdefg", ErrPasswordTooShort},
		{"8 two-byte runes", strings.Repeat("é", 8), nil},
		{"1024 bytes", strings.Repeat("a", 1024), nil},
		{"1025 bytes", strings.Repeat("a", 1025), ErrPasswordTooLong},
		{"spaces", "        ", nil},
	}
	for _, tt := range tests {
		if err := ValidatePassword(tt.pw); !errors.Is(err, tt.want) {
			t.Errorf("%s: ValidatePassword = %v, want %v", tt.name, err, tt.want)
		}
	}
}
