package store

import "testing"

func TestFoldSearch(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"Zayde Wølf", "Zayde Wolf"},
		{"Przybyłowicz", "Przybylowicz"},
		{"Hørizøns", "Horizons"},
		{"Ægir", "AEgir"},
		{"Œuvre", "OEuvre"},
		{"Straße", "Strasse"},
		{"Þórr", "THórr"}, // The accent is the tokenizer's job.
		{"plain ASCII", "plain ASCII"},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := foldSearch(tc.in); got != tc.want {
				t.Errorf("foldSearch(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
