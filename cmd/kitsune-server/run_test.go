package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simonhull/kitsune/internal/config"
)

// Strings the tests look for in cli's output.
const (
	versionCmd  = "version"
	usageHeader = "Usage: kitsune-server"
)

// noEnv is an empty environment.
func noEnv(string) (string, bool) { return "", false }

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
			if code := cli(tt.args, noEnv, strings.NewReader(""), &stdout, &stderr); code != tt.wantCode {
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
	if code := cli([]string{versionCmd}, noEnv, strings.NewReader(""), failingWriter{}, &stderr); code != exitFailure {
		t.Errorf("cli = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "write version: write failed") {
		t.Errorf("stderr = %q, want the wrapped write error", stderr.String())
	}
}

// writeFile writes body to name in a new temp dir and returns its path.
func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIConfig(t *testing.T) {
	t.Parallel()
	const configCmd, configFlag = "config", "-config"
	debug := writeFile(t, "debug.toml", "[log]\nlevel = \"debug\"\n")
	bogus := writeFile(t, "bogus.toml", "bogus = 1\n")
	missing := filepath.Join(t.TempDir(), "missing.toml")

	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout []string // Substrings of stdout.
		wantStderr string   // Substring of stderr; "" means it must stay empty.
	}{
		{
			name:       "defaults",
			args:       []string{configCmd},
			wantStdout: []string{`listen = ":4533"`, `roots = []`, `trusted_proxies = []`, `quick_scan = "03:30"`, `tag_writes = false`, `data_dir = "HOME/.local/share/kitsune-server"`},
		},
		{name: "single-dash flag", args: []string{configCmd, configFlag, debug}, wantStdout: []string{`level = "DEBUG"`}},
		{name: "double-dash flag", args: []string{configCmd, "--config", debug}, wantStdout: []string{`level = "DEBUG"`}},
		{name: "missing file", args: []string{configCmd, configFlag, missing}, wantCode: exitFailure, wantStderr: "no such file"},
		{name: "unknown key", args: []string{configCmd, configFlag, bogus}, wantCode: exitFailure, wantStderr: "not a config key"},
		{name: "unknown flag", args: []string{configCmd, "-nope"}, wantCode: exitUsage, wantStderr: usageHeader},
		{name: "extra arguments", args: []string{configCmd, "extra"}, wantCode: exitUsage, wantStderr: "config takes no arguments"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			env := config.Env(func(key string) (string, bool) { return home, key == "HOME" })
			var stdout, stderr bytes.Buffer
			if code := cli(tt.args, env, strings.NewReader(""), &stdout, &stderr); code != tt.wantCode {
				t.Errorf("cli(%q) = %d, want %d; stderr: %s", tt.args, code, tt.wantCode, &stderr)
			}
			out := stdout.String()
			for _, want := range tt.wantStdout {
				if want = strings.ReplaceAll(want, "HOME", home); !strings.Contains(out, want) {
					t.Errorf("cli(%q) stdout = %q, want it to contain %q", tt.args, out, want)
				}
			}
			errOut := stderr.String()
			if !matches(errOut, tt.wantStderr, strings.Contains) {
				t.Errorf("cli(%q) stderr = %q, want it to contain %q", tt.args, errOut, tt.wantStderr)
			}
			if tt.wantCode == exitUsage && !strings.Contains(errOut, usageHeader) {
				t.Errorf("cli(%q) stderr = %q, want the usage text", tt.args, errOut)
			}
			if tt.wantCode != exitOK && out != "" {
				t.Errorf("cli(%q) stdout = %q, want nothing on failure", tt.args, out)
			}
		})
	}
}
