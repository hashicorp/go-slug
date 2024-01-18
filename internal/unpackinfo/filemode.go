package unpackinfo

import (
	"fmt"
	"io/fs"
	"os"
)

type FileMode uint32

const (
	Empty FileMode = 0
	// Dir represent a Directory.
	Dir FileMode = 0040000
	// Regular represent non-executable files.  Please note this is not
	// the same as golang regular files, which include executable files.
	Regular FileMode = 0100644
	// Executable represents executable files.
	Executable FileMode = 0100755
	// Symlink represents symbolic links to files.
	Symlink FileMode = 0120000
)

func NewFileMode(mode fs.FileMode) (FileMode, error) {
	if mode.IsRegular() {
		// disallow pipes, I/O, temporary files etc
		if isCharDevice(mode) || isTemporary(mode) {
			return Empty, fmt.Errorf("invalid file mode: %s", mode)
		}

		if isExecutable(mode) {
			return Executable, nil
		}

		return Regular, nil
	}

	if mode.IsDir() {
		return Dir, nil
	}

	if isSymlink(mode) {
		return Symlink, nil
	}

	return Empty, fmt.Errorf("invalid file mode: %s", mode)
}

// Maps a FileMode integer to an fs.FileMode type, normalizing
// permissions for regular and executable files.
func (m FileMode) ToFsFileMode() (fs.FileMode, error) {
	switch m {
	case Regular:
		return fs.FileMode(0644), nil
	case Dir:
		return fs.ModePerm | fs.ModeDir, nil
	case Executable:
		return fs.FileMode(0755), nil
	case Symlink:
		return fs.ModePerm | fs.ModeSymlink, nil
	}

	return fs.FileMode(0), fmt.Errorf("malformed file mode: %s", m)
}

func (m FileMode) IsRegular() bool {
	return m == Regular
}

func (m FileMode) IsFile() bool {
	return m == Regular ||
		m == Executable ||
		m == Symlink
}

func (m FileMode) String() string {
	return fmt.Sprintf("%07o", uint32(m))
}

func isCharDevice(m fs.FileMode) bool {
	return m&os.ModeCharDevice != 0
}

func isExecutable(m fs.FileMode) bool {
	return m&0100 != 0
}

func isSymlink(m fs.FileMode) bool {
	return m&fs.ModeSymlink != 0
}

func isTemporary(m fs.FileMode) bool {
	return m&fs.ModeTemporary != 0
}
