//go:build !linux

package main

import "errors"

func lockMemory() error {
	return errors.New("memory locking is only supported on Linux")
}
