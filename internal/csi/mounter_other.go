//go:build !linux

package csi

import "errors"

// StubMounter is used on non-Linux platforms for compilation only.
type StubMounter struct{}

// NewMounter returns a stub Mounter (non-Linux platforms cannot mount).
func NewMounter() Mounter { return &StubMounter{} }

func (m *StubMounter) MountTmpfs(_ string) error {
	return errors.New("tmpfs mount not supported on this OS")
}

func (m *StubMounter) Unmount(_ string) error {
	return errors.New("unmount not supported on this OS")
}
