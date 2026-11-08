package library

import (
	"testing"
)

// Filesystem types the tests name.
const (
	fsBtrfs = "btrfs"
	fsCIFS  = "cifs"
	fsExt4  = "ext4"
)

// mountinfoFixture is /proc/self/mountinfo from a NAS-like host: a local root,
// a Samba share with a space in its mount point, NFS, rclone over FUSE, a
// local NTFS disk through fuseblk, and a local disk mounted inside the share.
const mountinfoFixture = `22 1 259:2 / / rw,relatime shared:1 - btrfs /dev/nvme0n1p2 rw,ssd
25 22 0:22 / /proc rw,nosuid,nodev,noexec,relatime shared:12 - proc procfs rw
40 22 0:40 / /mnt/music\040share rw,relatime shared:20 - cifs //nas/music rw,vers=3.1.1
41 22 0:41 / /mnt/nfs rw,relatime shared:21 - nfs4 nas:/export rw
42 22 0:42 / /mnt/cloud rw,nosuid,nodev,relatime shared:22 - fuse.rclone remote: rw
43 22 8:17 / /mnt/usb rw,relatime shared:23 - fuseblk /dev/sdb1 rw
44 40 0:44 / /mnt/music\040share/local rw,relatime shared:24 - ext4 /dev/sdc1 rw
not a mountinfo line
`

func TestMountOfFindsTheFilesystemOfAPath(t *testing.T) {
	t.Parallel()
	mounts := parseMounts([]byte(mountinfoFixture))
	for _, tc := range []struct {
		path, fstype string
		network      bool
	}{
		{"/home/simon/Music", fsBtrfs, false},
		{"/mnt/music share", fsCIFS, true},
		{"/mnt/music share/Artist/Album", fsCIFS, true},
		{"/mnt/music share/local/Album", fsExt4, false},
		{"/mnt/music shared", fsBtrfs, false},
		{"/mnt/nfs/a", "nfs4", true},
		{"/mnt/cloud/a", "fuse.rclone", true},
		{"/mnt/usb/a", "fuseblk", false},
	} {
		m, ok := mountOf(mounts, tc.path)
		if !ok || m.fstype != tc.fstype || networkFS(m.fstype) != tc.network {
			t.Errorf("mountOf(%q) = %+v, %v; want %s, network %v", tc.path, m, ok, tc.fstype, tc.network)
		}
	}
}

func TestNetworkFS(t *testing.T) {
	t.Parallel()
	for fstype, want := range map[string]bool{
		"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "9p": true, "fuse.sshfs": true, "fuse.mergerfs": true,
		"virtiofs": true, "ext4": false, "btrfs": false, "xfs": false, "zfs": false, "fuseblk": false, "overlay": false, "": false,
	} {
		if got := networkFS(fstype); got != want {
			t.Errorf("networkFS(%q) = %v, want %v", fstype, got, want)
		}
	}
}

func TestUnescapeMount(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`/a\040b`: "/a b", `/tab\011x`: "/tab\tx", `/plain`: "/plain", `/end\04`: `/end\04`, `/bad\09x`: `/bad\09x`,
	} {
		if got := unescapeMount(in); got != want {
			t.Errorf("unescapeMount(%q) = %q, want %q", in, got, want)
		}
	}
}
