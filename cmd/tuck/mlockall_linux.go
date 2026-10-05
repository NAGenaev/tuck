//go:build linux

package main

import (
	"fmt"
	"syscall"
)

func lockMemory() error {
	// Core dumps would write the root key and DEK to disk.
	_ = syscall.Setrlimit(syscall.RLIMIT_CORE, &syscall.Rlimit{Cur: 0, Max: 0})
	if err := syscall.Mlockall(syscall.MCL_CURRENT | syscall.MCL_FUTURE); err != nil {
		return fmt.Errorf("mlockall: %w (needs CAP_IPC_LOCK or a raised RLIMIT_MEMLOCK)", err)
	}
	return nil
}
