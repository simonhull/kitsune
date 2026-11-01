package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// Strings the tests look for in cli's output.
const (
	versionCmd  = "version"
	usageHeader = "Usage: kitsune-server"
)

func TestCLI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string // prefix; "" means stdout must stay empty
		wantStderr string // substring; "" means stderr must stay empty
	}{
		{name: versionCmd, args: []string{versionCmd}, wantCode: 0, wantStdout: "kitsune-server "},
		{name: "help", args: []string{"help"}, wantCode: 0, wantStdout: usageHeader},
		{name: "short help flag", args: []string{"-h"}, wantCode: 0, wantStdout: usageHeader},
		{name: "long help flag", args: []string{"--help"}, wantCode: 0, wantStdout: usageHeader},
		{name: "no command", args: nil, wantCode: exitUsage, wantStderr: "no command given"},
		{name: "unknown command", args: []string{"frobnicate"}, wantCode: exitUsage, wantStderr: `unknown command "frobnicate"`},
		{name: "extra arguments", args: []string{versionCmd, "--json"}, wantCode: exitUsage, wantStderr: "version takes no arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if code := cli(tt.args, &stdout, &stderr); code != tt.wantCode {
				t.Errorf("cli(%q) = %d, want %d", tt.args, code, tt.wantCode)
			}
			if out := stdout.String(); !matches(out, tt.wantStdout, strings.HasPrefix) {
				t.Errorf("cli(%q) stdout = %q, want prefix %q", tt.args, out, tt.wantStdout)
			}
			errOut := stderr.String()
			if !matches(errOut, tt.wantStderr, strings.Contains) {
				t.Errorf("cli(%q) stderr = %q, want it to contain %q", tt.args, errOut, tt.wantStderr)
			}
			if tt.wantCode == exitUsage && !strings.Contains(errOut, usageHeader) {
				t.Errorf("cli(%q) stderr = %q, want the usage text", tt.args, errOut)
			}
		})
	}
}

// matches reports whether got satisfies want under match, where an empty
// want means got must be empty too.
func matches(got, want string, match func(s, substr string) bool) bool {
	if want == "" {
		return got == ""
	}
	return match(got, want)
}

// errWriteFailed is returned by failingWriter.
var errWriteFailed = errors.New("write failed")

// failingWriter fails every write, like a full disk or a closed pipe.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

func TestCLI_WriteFailure(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	if code := cli([]string{versionCmd}, failingWriter{}, &stderr); code != exitFailure {
		t.Errorf("cli = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "write version: write failed") {
		t.Errorf("stderr = %q, want the wrapped write error", stderr.String())
	}
}
