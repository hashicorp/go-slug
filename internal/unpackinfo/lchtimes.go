//go:build darwin || linux
// +build darwin linux

package unpackinfo

import (
	"golang.org/x/sys/unix"
)

// Lchtimes modifies the access and modified timestamps on a target path
// This capability is only available on Linux and Darwin as of now.
func (i UnpackInfo) Lchtimes() error {
	return unix.Lutimes(i.Path, []unix.Timeval{
		{Sec: i.OriginalAccessTime.Unix(), Usec: int32(i.OriginalAccessTime.UnixMicro() % 1000)},
		{Sec: i.OriginalModTime.Unix(), Usec: int32(i.OriginalModTime.UnixMicro() % 1000)}},
	)
}
