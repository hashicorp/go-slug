// Copyright IBM Corp. 2018, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux && !android && (386 || arm || amd64 || arm64)

package unpackinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// Lchtimes sets the timestamps on the symlink itself, not its target.
func (i UnpackInfo) Lchtimes() error {
	return i.lchtimesAt(unix.AT_FDCWD, i.Path)
}

func (i UnpackInfo) lchtimesAt(dirfd int, name string) error {
	atime, err := symlinkTimespec(i.OriginalAccessTime)
	if err != nil {
		return err
	}
	mtime, err := symlinkTimespec(i.OriginalModTime)
	if err != nil {
		return err
	}
	return unix.UtimesNanoAt(dirfd, name, []unix.Timespec{atime, mtime}, unix.AT_SYMLINK_NOFOLLOW)
}

func symlinkTimespec(t time.Time) (unix.Timespec, error) {
	if t.IsZero() {
		// Like os.Chtimes, leave timestamps absent from a tar header untouched.
		return unix.Timespec{Nsec: unix.UTIME_OMIT}, nil
	}
	// Use the platform's native width without rounding or silently wrapping on 32-bit targets.
	return unix.TimeToTimespec(t.Truncate(time.Microsecond))
}

func (i UnpackInfo) restoreSymlinkWithRoot(root *os.Root, relPath string) error {
	// os.Root does not have Lchtimes. Open the parent through the root, then
	// change only the final path component relative to that directory handle.
	dir, err := root.OpenFile(filepath.Dir(relPath), unix.O_PATH|unix.O_DIRECTORY, 0)
	if err != nil {
		return fmt.Errorf("failed opening parent directory of symlink %q: %w", i.Path, err)
	}
	defer func() { _ = dir.Close() }()

	if err := i.lchtimesAt(int(dir.Fd()), filepath.Base(relPath)); err != nil {
		return fmt.Errorf("failed setting times on symlink %q: %w", i.Path, err)
	}
	return nil
}

// CanMaintainSymlinkTimestamps reports whether the platform can set a symlink's timestamps.
func CanMaintainSymlinkTimestamps() bool {
	return true
}
