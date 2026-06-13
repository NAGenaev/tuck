//go:build !linux

package main

import "log"

func lockMemory() {
	log.Printf("tuck: mlockall not available on this OS — root key may be swapped to disk")
}
