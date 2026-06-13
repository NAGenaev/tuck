//go:build linux

package csi

import "golang.org/x/sys/unix"

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
