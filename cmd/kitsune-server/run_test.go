package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		args      []string
		wantOut   string // prefix of stdout
		wantUsage bool   // want a *usageError
	}{
		{name: "version", args: []string{"version"}, wantOut: "kitsune-server "},
		{name: "help", args: []string{"help"}, wantOut: "Usage: kitsune-server"},
		{name: "help flag", args: []string{"-h"}, wantOut: "Usage: kitsune-server"},
		{name: "no command", args: nil, wantUsage: true},
		{name: "unknown command", args: []string{"frobnicate"}, wantUsage: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			err := run(tt.args, &out)
			if _, isUsage := errors.AsType[*usageError](err); isUsage != tt.wantUsage {
				t.Fatalf("run(%q) error = %v, want usage error: %t", tt.args, err, tt.wantUsage)
			}
			if !tt.wantUsage && err != nil {
				t.Fatalf("run(%q) error = %v", tt.args, err)
			}
			if !strings.HasPrefix(out.String(), tt.wantOut) {
				t.Errorf("run(%q) output = %q, want prefix %q", tt.args, out.String(), tt.wantOut)
			}
		})
	}
}

func TestRun_UnknownCommandIsNamed(t *testing.T) {
	t.Parallel()
	err := run([]string{"frobnicate"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `"frobnicate"`) {
		t.Errorf("error = %v, want it to name the command", err)
	}
}

func TestVersion_NeverEmpty(t *testing.T) {
	t.Parallel()
	if version() == "" {
		t.Error("version() is empty")
	}
}
