//go:build linux

package csi

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// LinuxMounter performs real OS mount/unmount calls.
type LinuxMounter struct{}

// NewMounter returns the production Mounter for Linux.
func NewMounter() Mounter { return &LinuxMounter{} }

func (m *LinuxMounter) MountTmpfs(target string) error {
	return unix.Mount("tmpfs", target, "tmpfs", unix.MS_NODEV|unix.MS_NOSUID|unix.MS_NOEXEC, "mode=0750,size=64m")
}

func (m *LinuxMounter) Unmount(target string) error {
	return unix.Unmount(target, unix.MNT_DETACH)
}

// IsMounted reports whether target is a mount point, per /proc/self/mountinfo.
func (m *LinuxMounter) IsMounted(target string) (bool, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	return mountinfoHas(f, filepath.Clean(target))
}

func mountinfoHas(f *os.File, target string) (bool, error) {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) > 4 && strings.ReplaceAll(fields[4], `\040`, " ") == target {
			return true, nil
		}
	}
	return false, sc.Err()
}
