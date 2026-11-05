//go:build !linux && !darwin

package store

import "math"

// freeSpace can't tell on this platform, so it lets the backup try: a full disk
// then fails the backup itself.
func freeSpace(string) (uint64, error) { return math.MaxUint64, nil }
