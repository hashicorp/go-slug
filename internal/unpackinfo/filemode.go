package unpackinfo

import (
	"fmt"
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

func NewFileMode(mode os.FileMode) (FileMode, error) {
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

// Maps a FileMode integer to an os.FileMode type, normalizing
// permissions for regular and executable files.
func (m FileMode) ToOSFileMode() (os.FileMode, error) {
	switch m {
	case Regular:
		return os.FileMode(0644), nil
	case Dir:
		return os.ModePerm | os.ModeDir, nil
	case Executable:
		return os.FileMode(0755), nil
	case Symlink:
		return os.ModePerm | os.ModeSymlink, nil
	}

	return os.FileMode(0), fmt.Errorf("malformed file mode: %s", m)
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

func isCharDevice(m os.FileMode) bool {
	return m&os.ModeCharDevice != 0
}

func isExecutable(m os.FileMode) bool {
	return m&0100 != 0
}

func isSymlink(m os.FileMode) bool {
	return m&os.ModeSymlink != 0
}

func isTemporary(m os.FileMode) bool {
	return m&os.ModeTemporary != 0
}
