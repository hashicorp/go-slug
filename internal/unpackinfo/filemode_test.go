package unpackinfo

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func assertFileMode(t *testing.T, expected FileMode, got FileMode) {
	if expected != got {
		t.Fatalf("expected filemode: %s, got: %s", expected, got)
	}
}

func assertOSFileMode(t *testing.T, expected os.FileMode, got os.FileMode) {
	if expected != got {
		t.Fatalf("expected OS filemode: %s, got: %s", expected, got)
	}
}

func TestFileMode_New(t *testing.T) {
	for _, c := range []struct {
		mode     os.FileMode
		expected FileMode
	}{
		{os.FileMode(0755) | os.ModeDir, Dir},
		{os.FileMode(0700) | os.ModeDir, Dir},
		{os.FileMode(0500) | os.ModeDir, Dir},
		// dirs with a sticky bit are just dirs
		{os.FileMode(0755) | os.ModeDir | os.ModeSticky, Dir},
		{os.FileMode(0644), Regular},
		// append only files are regular
		{os.FileMode(0644) | os.ModeAppend, Regular},
		// exclusive only files are regular
		{os.FileMode(0644) | os.ModeExclusive, Regular},
		// depending on owner perms, setguid can be regular
		{os.FileMode(0644) | os.ModeSetgid, Regular},
		{os.FileMode(0660), Regular},
		{os.FileMode(0640), Regular},
		{os.FileMode(0600), Regular},
		{os.FileMode(0400), Regular},
		{os.FileMode(0000), Regular},
		{os.FileMode(0755), Executable},
		// setuid and setguid are executables
		{os.FileMode(0755) | os.ModeSetuid, Executable},
		{os.FileMode(0755) | os.ModeSetgid, Executable},
		{os.FileMode(0700), Executable},
		{os.FileMode(0500), Executable},
		{os.FileMode(0744), Executable},
		{os.FileMode(0540), Executable},
		{os.FileMode(0550), Executable},
		{os.FileMode(0777) | os.ModeSymlink, Symlink},
	} {
		m, err := New(c.mode)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertFileMode(t, c.expected, m)
	}
}

func TestFileMode_NewWithErrors(t *testing.T) {
	for _, c := range []struct {
		mode        os.FileMode
		expected    FileMode
		expectedErr string
	}{
		// temporary files are ignored
		{os.FileMode(0644) | os.ModeTemporary, Empty, "invalid file mode"},
		// device files are ignored
		{os.FileMode(0644) | os.ModeCharDevice, Empty, "invalid file mode"},
		// named pipes are ignored
		{os.FileMode(0644) | os.ModeNamedPipe, Empty, "invalid file mode"},
		// sockets are ignored
		{os.FileMode(0644) | os.ModeSocket, Empty, "invalid file mode"},
	} {
		m, err := New(c.mode)
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
		expected os.FileMode
	}{
		{Regular, os.FileMode(0644)},
		{Dir, os.ModePerm | os.ModeDir},
		{Symlink, os.ModePerm | os.ModeSymlink},
		{Executable, os.FileMode(0755)},
	} {
		m, err := c.mode.ToOSFileMode()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertOSFileMode(t, c.expected, m)
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
		m, err := mode.ToOSFileMode()
		if err == nil {
			t.Fatalf("expected an error, got nil")
		}

		expectedErr := fmt.Sprintf("malformed file mode: %s", mode)
		gotErr := err.Error()
		if gotErr != expectedErr {
			t.Fatalf("expected error: %s, got: %s", expectedErr, gotErr)
		}

		if m != os.FileMode(0) {
			t.Fatalf("expected file mode 0, got: %s", m)
		}
	}
}
