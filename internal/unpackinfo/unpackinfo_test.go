// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package unpackinfo

import (
	"archive/tar"
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
	root := t.TempDir()

	err := os.Mkdir(path.Join(root, "subdir"), 0700)
	if err != nil {
		t.Fatalf("failed to create temp subdir: %s", err)
	}

	err = os.WriteFile(path.Join(root, "bar.txt"), []byte("Hello, World!"), 0700)
	if err != nil {
		t.Fatalf("failed to create temp file: %s", err)
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

	infoCollection := []UnpackInfo{dirinfo, finfo, linfo}

	for _, info := range infoCollection {
		err = info.RestoreInfo()
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
