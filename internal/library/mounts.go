package library

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// mountInfoPath is where Linux lists this process's mounts.
const mountInfoPath = "/proc/self/mountinfo"

// networkFS reports whether a filesystem of type fstype, as mountinfo names
// it, may change without the OS here hearing of it: a network share, a FUSE
// filesystem, or a host's folder shared into a VM or container. Changes made by
// another machine never reach inotify on those, so a root on one is polled. This
// is the one list of them.
func networkFS(fstype string) bool {
	switch fstype {
	case "nfs", "nfs4", "cifs", "smb3", "smbfs", "9p", "afs", "ceph", "glusterfs",
		"davfs", "ncpfs", "virtiofs", "vboxsf", "fuse", "fuse.sshfs", "fuse.rclone":
		return true
	}
	return strings.HasPrefix(fstype, "fuse.")
}

// mount is one line of mountinfo: where a filesystem is mounted, and its type.
type mount struct {
	point, fstype string
}

// readMounts reads mountinfo. It returns nothing when there is none, as off Linux.
func readMounts() []mount {
	b, err := os.ReadFile(mountInfoPath)
	if err != nil {
		return nil
	}
	return parseMounts(b)
}

// parseMounts reads the mount point (field 5) and the filesystem type (the
// first field after the " - " separator) of each mountinfo line. A line it
// can't read is left out.
func parseMounts(b []byte) []mount {
	var out []mount
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		before, after, ok := strings.Cut(sc.Text(), " - ")
		fields, rest := strings.Fields(before), strings.Fields(after)
		if !ok || len(fields) < 5 || len(rest) < 1 {
			continue
		}
		out = append(out, mount{point: unescapeMount(fields[4]), fstype: rest[0]})
	}
	return out
}

// unescapeMount undoes mountinfo's octal escapes, such as \040 for a space.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// mountOf returns the mount that holds path, an absolute path with its
// symlinks resolved: the one with the longest mount point at or above it. The
// last such line wins, as a later mount over the same point hides an earlier.
func mountOf(mounts []mount, path string) (mount, bool) {
	var best mount
	found := false
	for _, m := range mounts {
		if !within(path, m.point) {
			continue
		}
		if !found || len(m.point) >= len(best.point) {
			best, found = m, true
		}
	}
	return best, found
}

// within reports whether path is dir or lies below it.
func within(path, dir string) bool {
	if dir == "/" {
		return strings.HasPrefix(path, "/")
	}
	dir = filepath.Clean(dir)
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}
