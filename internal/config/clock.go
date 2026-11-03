package config

import (
	"errors"
	"fmt"
	"time"
)

// errClockTime reports a time of day that is not HH:MM.
var errClockTime = errors.New("want a time of day as HH:MM, 00:00 to 23:59")

// ClockTime is a time of day in the server's local time zone, to the minute.
// In TOML it is a string, "HH:MM".
type ClockTime struct {
	Hour   int // 0 to 23.
	Minute int // 0 to 59.
}

// String formats t as HH:MM.
func (t ClockTime) String() string {
	return fmt.Sprintf("%02d:%02d", t.Hour, t.Minute)
}

// MarshalText implements [encoding.TextMarshaler].
func (t ClockTime) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

// UnmarshalText implements [encoding.TextUnmarshaler]. It accepts HH:MM from
// 00:00 to 23:59.
func (t *ClockTime) UnmarshalText(text []byte) error {
	p, err := time.Parse("15:04", string(text))
	if err != nil {
		return fmt.Errorf("%w: got %q", errClockTime, text)
	}
	*t = ClockTime{Hour: p.Hour(), Minute: p.Minute()}
	return nil
}
