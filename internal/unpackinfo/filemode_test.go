package unpackinfo

import (
	"fmt"
	"io/fs"
	"strings"
	"testing"
)

func assertFileMode(t *testing.T, expected FileMode, got FileMode) {
	if expected != got {
		t.Fatalf("expected filemode: %s, got: %s", expected, got)
	}
}

func assertFsFileMode(t *testing.T, expected fs.FileMode, got fs.FileMode) {
	if expected != got {
		t.Fatalf("expected OS filemode: %s, got: %s", expected, got)
	}
}

func TestFileMode_New(t *testing.T) {
	for _, c := range []struct {
		mode     fs.FileMode
		expected FileMode
	}{
		{fs.FileMode(0755) | fs.ModeDir, Dir},
		{fs.FileMode(0700) | fs.ModeDir, Dir},
		{fs.FileMode(0500) | fs.ModeDir, Dir},
		// dirs with a sticky bit are just dirs
		{fs.FileMode(0755) | fs.ModeDir | fs.ModeSticky, Dir},
		{fs.FileMode(0644), Regular},
		// append only files are regular
		{fs.FileMode(0644) | fs.ModeAppend, Regular},
		// exclusive only files are regular
		{fs.FileMode(0644) | fs.ModeExclusive, Regular},
		// depending on owner perms, setguid can be regular
		{fs.FileMode(0644) | fs.ModeSetgid, Regular},
		{fs.FileMode(0660), Regular},
		{fs.FileMode(0640), Regular},
		{fs.FileMode(0600), Regular},
		{fs.FileMode(0400), Regular},
		{fs.FileMode(0000), Regular},
		{fs.FileMode(0755), Executable},
		// setuid and setguid are executables
		{fs.FileMode(0755) | fs.ModeSetuid, Executable},
		{fs.FileMode(0755) | fs.ModeSetgid, Executable},
		{fs.FileMode(0700), Executable},
		{fs.FileMode(0500), Executable},
		{fs.FileMode(0744), Executable},
		{fs.FileMode(0540), Executable},
		{fs.FileMode(0550), Executable},
		{fs.FileMode(0777) | fs.ModeSymlink, Symlink},
	} {
		m, err := NewFileMode(c.mode)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertFileMode(t, c.expected, m)
	}
}

func TestFileMode_NewWithErrors(t *testing.T) {
	for _, c := range []struct {
		mode        fs.FileMode
		expected    FileMode
		expectedErr string
	}{
		// temporary files are ignored
		{fs.FileMode(0644) | fs.ModeTemporary, Empty, "invalid file mode"},
		// device files are ignored
		{fs.FileMode(0644) | fs.ModeCharDevice, Empty, "invalid file mode"},
		// named pipes are ignored
		{fs.FileMode(0644) | fs.ModeNamedPipe, Empty, "invalid file mode"},
		// sockets are ignored
		{fs.FileMode(0644) | fs.ModeSocket, Empty, "invalid file mode"},
	} {
		m, err := NewFileMode(c.mode)
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}
		if !strings.Contains(err.Error(), c.expectedErr) {
			t.Fatalf("unexpected error got: %v", err)
		}

		if m != c.expected {
			t.Fatalf("expected empty file mode, got: %s", m)
		}
	}
}

func TestFileMode_ToOSFileMode(t *testing.T) {
	for _, c := range []struct {
		mode     FileMode
		expected fs.FileMode
	}{
		{Regular, fs.FileMode(0644)},
		{Dir, fs.ModePerm | fs.ModeDir},
		{Symlink, fs.ModePerm | fs.ModeSymlink},
		{Executable, fs.FileMode(0755)},
	} {
		m, err := c.mode.ToFsFileMode()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertFsFileMode(t, c.expected, m)
	}
}

func TestFileMode_ToOSFileModeWithErrors(t *testing.T) {
	for _, mode := range []FileMode{
		Empty,
		FileMode(01),
		FileMode(010),
		FileMode(0100),
		FileMode(01000),
		FileMode(010000),
		FileMode(0100000),
	} {
		m, err := mode.ToFsFileMode()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedErr := fmt.Sprintf("malformed file mode: %s", mode)
		gotErr := err.Error()
		if gotErr != expectedErr {
			t.Fatalf("expected error: %s, got: %s", expectedErr, gotErr)
		}

		if m != fs.FileMode(0) {
			t.Fatalf("expected file mode 0, got: %s", m)
		}
	}
}
