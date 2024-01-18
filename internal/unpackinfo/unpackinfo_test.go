// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package unpackinfo

import (
	"archive/tar"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"
	"time"
)

func TestNewUnpackInfo(t *testing.T) {
	t.Parallel()

	t.Run("disallow parent traversal", func(t *testing.T) {
		_, err := NewUnpackInfo("test", &tar.Header{
			Name:     "../off-limits",
			Typeflag: tar.TypeSymlink,
		})

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		expected := "invalid filename, traversal with \"..\""
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected error to contain %q, got %q", expected, err)
		}
	})

	t.Run("disallow zipslip", func(t *testing.T) {
		dst := t.TempDir()

		err := os.Symlink("..", path.Join(dst, "subdir"))
		if err != nil {
			t.Fatalf("failed to create temp symlink: %s", err)
		}

		_, err = NewUnpackInfo(dst, &tar.Header{
			Name:     "subdir/escapes",
			Typeflag: tar.TypeReg,
		})

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		expected := "through symlink"
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected error to contain %q, got %q", expected, err)
		}
	})

	t.Run("disallow strange types", func(t *testing.T) {
		_, err := NewUnpackInfo("test", &tar.Header{
			Name:     "subdir/escapes",
			Typeflag: tar.TypeFifo,
		})

		if err == nil {
			t.Fatal("expected error, got nil")
		}

		expected := "unsupported file type"
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected error to contain %q, got %q", expected, err)
		}
	})
}

func TestUnpackInfo_RestoreInfo(t *testing.T) {
	exampleModTime := time.Date(2023, time.May, 29, 11, 22, 33, 0, time.UTC)
	cases := testUnpackInfoCases(t)
	for _, info := range cases {
		err := info.RestoreInfo()
		if err != nil {
			t.Errorf("failed to restore %q: %s", info.Path, err)
		}
		stat, err := os.Lstat(info.Path)
		if err != nil {
			t.Errorf("failed to lstat %q: %s", info.Path, err)
		}

		if !info.IsSymlink() {
			if stat.Mode() != info.OriginalMode {
				t.Errorf("%q mode %q did not match expected header mode %q", info.Path, stat.Mode(), info.OriginalMode)
			}
		} else if CanMaintainSymlinkTimestamps() {
			if !stat.ModTime().Equal(exampleModTime) {
				t.Errorf("%q modtime %q did not match example", info.Path, stat.ModTime())
			}
		}
	}
}

func TestUnpackInfo_normalizedFileModes(t *testing.T) {
	cases := testUnpackInfoCases(t)
	for _, info := range cases {
		err := info.SetNormalizedMode()
		if err != nil {
			t.Fatalf("failed normalizing permissions for %s: %s", info.Path, err)
		}
		err = info.RestoreInfo()
		if err != nil {
			t.Fatalf("failed to restore %q: %s", info.Path, err)
		}
		stat, err := os.Lstat(info.Path)
		if err != nil {
			t.Fatalf("failed to lstat %q: %s", info.Path, err)
		}

		var expectedMode fs.FileMode
		switch info.NormalizedMode {
		case Regular:
			expectedMode = fs.FileMode(0644)
		case Dir:
			expectedMode = os.ModePerm | os.ModeDir
		case Executable:
			expectedMode = fs.FileMode(0755)
		case Symlink:
			// ignore symlinks
			continue
		}

		if stat.Mode() != expectedMode {
			t.Errorf("%q mode %q did not match normalized mode %q", info.Path, stat.Mode(), expectedMode)
		}
	}
}

func testUnpackInfoCases(t *testing.T) []*UnpackInfo {
	root := t.TempDir()

	err := os.Mkdir(path.Join(root, "subdir"), 0700)
	if err != nil {
		t.Fatalf("failed to create temp subdir: %s", err)
	}

	err = os.WriteFile(path.Join(root, "bar.txt"), []byte("Hello, World!"), 0700)
	if err != nil {
		t.Fatalf("failed to create temp file: %s", err)
	}

	err = os.WriteFile(path.Join(root, "exefoobar"), []byte("echo Hello!"), 0755)
	if err != nil {
		t.Fatalf("failed to create executable file: %s", err)
	}

	err = os.Symlink(path.Join(root, "bar.txt"), path.Join(root, "foo.txt"))
	if err != nil {
		t.Fatalf("failed to create temp symlink: %s", err)
	}

	exampleAccessTime := time.Date(2023, time.April, 1, 11, 22, 33, 0, time.UTC)
	exampleModTime := time.Date(2023, time.May, 29, 11, 22, 33, 0, time.UTC)

	dirinfo, err := NewUnpackInfo(root, &tar.Header{
		Name:       "subdir",
		Typeflag:   tar.TypeDir,
		AccessTime: exampleAccessTime,
		ModTime:    exampleModTime,
		Mode:       0666,
	})
	if err != nil {
		t.Fatalf("failed to define dirinfo: %s", err)
	}

	finfo, err := NewUnpackInfo(root, &tar.Header{
		Name:       "bar.txt",
		Typeflag:   tar.TypeReg,
		AccessTime: exampleAccessTime,
		ModTime:    exampleModTime,
		Mode:       0666,
	})
	if err != nil {
		t.Fatalf("failed to define finfo: %s", err)
	}

	linfo, err := NewUnpackInfo(root, &tar.Header{
		Name:       "foo.txt",
		Typeflag:   tar.TypeSymlink,
		AccessTime: exampleAccessTime,
		ModTime:    exampleModTime,
		Mode:       0666,
	})
	if err != nil {
		t.Fatalf("failed to define linfo: %s", err)
	}

	exeinfo, err := NewUnpackInfo(root, &tar.Header{
		Name:       "exefoobar",
		Typeflag:   tar.TypeReg,
		AccessTime: exampleAccessTime,
		ModTime:    exampleModTime,
		Mode:       0755,
	})
	if err != nil {
		t.Fatalf("failed to define exeinfo: %s", err)
	}

	return []*UnpackInfo{dirinfo, finfo, linfo, exeinfo}
}
