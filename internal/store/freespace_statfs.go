//go:build linux || darwin

package store

import (
	"fmt"
	"syscall"
)

// freeSpace returns the bytes an unprivileged process can still write in dir.
func freeSpace(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", dir, err)
	}
	return st.Bavail * uint64(st.Bsize), nil //nolint:gosec // G115: a block size is never negative.
}
