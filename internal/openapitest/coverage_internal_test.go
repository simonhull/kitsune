package openapitest

import (
	"strings"
	"testing"
)

func TestCheckExercised(t *testing.T) {
	t.Parallel()
	c := Load(t) // Nothing exercised: GET /healthz 204 is missing.
	tests := []struct {
		name     string
		code     int
		partial  bool
		wantCode int
		wantOut  string
	}{
		{"full run, passed", 0, false, exitUnexercised, "GET /healthz 204"},
		{"full run, failed", 3, false, 3, ""},
		{"partial run", 0, true, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out strings.Builder
			if got := c.checkExercised(&out, tt.code, tt.partial); got != tt.wantCode {
				t.Errorf("code = %d, want %d", got, tt.wantCode)
			}
			if got := out.String(); (tt.wantOut == "") != (got == "") || !strings.Contains(got, tt.wantOut) {
				t.Errorf("output = %q, want %q", got, tt.wantOut)
			}
		})
	}
}
