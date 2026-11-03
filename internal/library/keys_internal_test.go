package library

import "testing"

func TestMatchKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, in, want string
	}{
		{"trim and collapse", "  Daft   Punk ", "daft punk"},
		{"decomposed accent", "Beyoncé", "beyoncé"},
		{"composed accent", "Beyoncé", "beyoncé"},
		{"upper sharp s", "STRASSE", "strasse"},
		{"sharp s", "Straße", "strasse"},
		{"slash kept", "AC/DC", "ac/dc"},
		{"accent kept", "Sigur Rós", "sigur rós"},
		{"only white space", "\t\n", ""},
		{"inner tab and newline", "a\t\nb", "a b"},
		{"feat kept", featName, "a feat. b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MatchKey(tt.in)
			if got != tt.want {
				t.Errorf("MatchKey(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if again := MatchKey(got); again != got {
				t.Errorf("MatchKey is not idempotent: %q then %q", got, again)
			}
		})
	}
}
