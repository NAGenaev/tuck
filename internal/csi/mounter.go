package csi

// Mounter abstracts OS-level mount operations. The real implementation is in
// mounter_linux.go; other platforms provide a stub for test compilation.
type Mounter interface {
	// MountTmpfs mounts a tmpfs at the given target path.
	// The path is created by the caller before this is invoked.
	MountTmpfs(target string) error
	// Unmount unmounts the filesystem at target.
	Unmount(target string) error
}
