package config

import "testing"

func TestClockTimeText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    ClockTime
		wantErr bool
	}{
		{in: "03:30", want: ClockTime{Hour: 3, Minute: 30}},
		{in: "00:00", want: ClockTime{}},
		{in: "23:59", want: ClockTime{Hour: 23, Minute: 59}},
		{in: "24:00", wantErr: true},
		{in: "12:60", wantErr: true},
		{in: "03:30:00", wantErr: true},
		{in: "noon", wantErr: true},
		{in: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			var got ClockTime
			err := got.UnmarshalText([]byte(tt.in))
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnmarshalText(%q) error = %v, wantErr %t", tt.in, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("UnmarshalText(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
			if b, _ := got.MarshalText(); string(b) != tt.in {
				t.Errorf("MarshalText() = %q, want %q", b, tt.in)
			}
		})
	}
}
