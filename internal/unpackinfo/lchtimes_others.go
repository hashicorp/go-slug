//go:build !(linux || darwin)
// +build !linux,!darwin

package unpackinfo

import (
	"errors"
)

// Lchtimes modifies the access and modified timestamps on a target path
// This capability is only available on Linux and Darwin as of now.
func (i UnpackInfo) Lchtimes() error {
	return errors.New("Lchtimes is not supported on this platform")
}
