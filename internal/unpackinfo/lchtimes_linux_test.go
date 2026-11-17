// Copyright IBM Corp. 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux && !android && (386 || arm || amd64 || arm64)

package unpackinfo

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRestoreInfoWithRootSetsSymlinkTimestamp(t *testing.T) {
	dst := t.TempDir()
	root, err := os.OpenRoot(dst)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})

	if err := root.Mkdir("subdir", 0755); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("target", nil, 0600); err != nil {
		t.Fatal(err)
	}
	targetTime := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	if err := root.Chtimes("target", targetTime, targetTime); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join("subdir", "link")
	if err := root.Symlink("../target", linkPath); err != nil {
		t.Fatal(err)
	}
	linkTime := time.Date(2023, time.April, 1, 11, 22, 33, 123456000, time.UTC)
	info, err := NewUnpackInfo(dst, &tar.Header{
		Name:     linkPath,
		Typeflag: tar.TypeSymlink,
		ModTime:  linkTime,
		// USTAR has no access time; the zero value must work on 32-bit Linux too.
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := info.RestoreInfoWithRoot(root, dst); err != nil {
		t.Fatal(err)
	}
	link, err := root.Lstat(linkPath)
	if err != nil {
		t.Fatal(err)
	}
	if !link.ModTime().Equal(linkTime) {
		t.Errorf("symlink mtime = %v, want %v", link.ModTime(), linkTime)
	}
	target, err := root.Stat("target")
	if err != nil {
		t.Fatal(err)
	}
	if !target.ModTime().Equal(targetTime) {
		t.Errorf("target mtime = %v, want unchanged %v", target.ModTime(), targetTime)
	}
}
