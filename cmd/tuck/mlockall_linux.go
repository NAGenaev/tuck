//go:build linux

package main

import (
	"log"
	"syscall"
)

func lockMemory() {
	if err := syscall.Mlockall(syscall.MCL_CURRENT | syscall.MCL_FUTURE); err != nil {
		log.Printf("tuck: mlockall failed (running as non-root?): %v — root key may be swapped to disk", err)
	} else {
		log.Printf("tuck: mlockall OK — process memory locked (no swap)")
	}
}
