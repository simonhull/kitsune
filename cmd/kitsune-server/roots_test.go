package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// rootsChildEnv marks the child process TestVerifiesWithoutSystemRoots runs.
const rootsChildEnv = "KITSUNE_TEST_ROOTS_CHILD"

// TestVerifiesWithoutSystemRoots runs itself again with no system roots, as in
// the scratch image, and checks Last.fm's chain still verifies.
func TestVerifiesWithoutSystemRoots(t *testing.T) {
	t.Parallel()
	empty := t.TempDir()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestVerifiesWithoutSystemRootsChild$") //nolint:gosec // The test binary running itself.
	cmd.Env = append(os.Environ(),
		rootsChildEnv+"=1",
		"SSL_CERT_FILE="+filepath.Join(empty, "none.pem"),
		"SSL_CERT_DIR="+empty,
	)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		t.Fatalf("child: %v\n%s", runErr, out)
	}
}

// TestVerifiesWithoutSystemRootsChild does the check, when run as the child.
func TestVerifiesWithoutSystemRootsChild(t *testing.T) {
	t.Parallel()
	if os.Getenv(rootsChildEnv) == "" {
		t.Skip("run by TestVerifiesWithoutSystemRoots")
	}
	data, err := os.ReadFile(filepath.Join("testdata", "lastfm-chain.pem"))
	if err != nil {
		t.Fatal(err)
	}
	var certs []*x509.Certificate
	for block, rest := pem.Decode(data); block != nil; block, rest = pem.Decode(rest) {
		c, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		certs = append(certs, c)
	}
	if len(certs) < 2 {
		t.Fatalf("chain has %d certificates, want the leaf and its intermediates", len(certs))
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	// Roots nil means the system's, which here are none, so only the fallback
	// can verify. The time is fixed inside the leaf's validity.
	_, err = certs[0].Verify(x509.VerifyOptions{
		DNSName:       "ws.audioscrobbler.com",
		Intermediates: inter,
		CurrentTime:   time.Date(2026, time.November, 8, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("verify without system roots: %v", err)
	}
}
