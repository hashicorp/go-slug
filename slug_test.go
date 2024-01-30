// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package slug

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-slug/internal/unpackinfo"
)

func TestPack(t *testing.T) {
	slug := bytes.NewBuffer(nil)
	meta, err := Pack("testdata/archive-dir", slug, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	assertArchiveFixture(t, slug, meta)
}

func TestPack_defaultRulesOnly(t *testing.T) {
	slug := bytes.NewBuffer(nil)
	meta, err := Pack("testdata/archive-dir-defaults-only", slug, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Make sure .terraform/modules/** are included.
	subModuleDir := false

	// Make sure .terraform/plugins are excluded.
	pluginsDir := false

	for _, file := range meta.Files {
		if strings.HasPrefix(file, filepath.Clean(".terraform/modules/subdir/README")) {
			subModuleDir = true
			continue
		}

		if strings.HasPrefix(file, filepath.Clean(".terraform/plugins/")) {
			pluginsDir = true
			continue
		}
	}
	if !subModuleDir {
		t.Fatal("expected to include .terraform/modules/subdir/README")
	}

	if pluginsDir {
		t.Fatal("expected to exclude .terraform/plugins")
	}
}

func TestPack_rootIsSymlink(t *testing.T) {
	for _, path := range []string{
		"testdata/archive-dir",
		"./testdata/archive-dir",
	} {
		t.Run(fmt.Sprintf("target is path: %s", path), func(t *testing.T) {
			symlinkPath := path + "-symlink"
			err := os.Symlink(path, symlinkPath)
			if err != nil {
				t.Fatalf("Failed creating dir %s symlink: %v", path, err)

			}
			t.Cleanup(func() {
				err = os.Remove(symlinkPath)
				if err != nil {
					t.Fatalf("failed removing %s: %v", symlinkPath, err)
				}
			})

			slug := bytes.NewBuffer(nil)
			meta, err := Pack(symlinkPath, slug, true)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			assertArchiveFixture(t, slug, meta)
		})
	}
}

func TestPack_absoluteSrcRelativeSymlinks(t *testing.T) {
	var path string
	var err error

	// In instances we run within CI, we want to fetch
	// the absolute path where our test is located
	if workDir, ok := os.LookupEnv("GITHUB_WORKSPACE"); ok {
		path = workDir
	} else {
		path, err = os.Getwd()
		if err != nil {
			t.Fatalf("could not determine the working dir: %v", err)
		}
	}

	// One last check, if this variable is empty we'll error
	// since we need the absolute path for the source
	if path == "" {
		t.Fatal("Home directory could not be determined")
	}

	path = filepath.Join(path, "testdata/archive-dir-absolute/dev")
	slug := bytes.NewBuffer(nil)
	_, err = Pack(path, slug, true)
	if err != nil {
		// We simply want to ensure paths can be resolved while
		// traversing the source directory
		t.Fatalf("err: %v", err)
	}

	// Cannot pack without dereferencing
	_, err = Pack(path, slug, false)
	if !strings.HasPrefix(err.Error(), "illegal slug error:") {
		t.Fatalf("expected illegal slug error, got %q", err)
	}
}

func TestPackWithoutIgnoring(t *testing.T) {
	slug := bytes.NewBuffer(nil)

	// By default NewPacker() creates a Packer that does not use
	// .terraformignore or dereference symlinks.
	p, err := NewPacker()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	meta, err := p.Pack("testdata/archive-dir-no-external", slug)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	gzipR, err := gzip.NewReader(slug)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	tarR := tar.NewReader(gzipR)
	var (
		fileList []string
		slugSize int64
	)

	for {
		hdr, err := tarR.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		fileList = append(fileList, hdr.Name)
		if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
			slugSize += hdr.Size
		}
	}

	// baz.txt would normally be ignored, but should not be
	var bazFound bool
	for _, file := range fileList {
		if file == "baz.txt" {
			bazFound = true
		}
	}
	if !bazFound {
		t.Fatal("expected file baz.txt to be present, but not found")
	}

	// .terraform/file.txt would normally be ignored, but should not be
	var dotTerraformFileFound bool
	for _, file := range fileList {
		if file == ".terraform/file.txt" {
			dotTerraformFileFound = true
		}
	}
	if !dotTerraformFileFound {
		t.Fatal("expected file .terraform/file.txt to be present, but not found")
	}

	// Check the metadata
	expect := &Meta{
		Files: fileList,
		Size:  slugSize,
	}
	if !reflect.DeepEqual(meta, expect) {
		t.Fatalf("\nexpect:\n%#v\n\nactual:\n%#v", expect, meta)
	}
}

func TestPack_symlinks(t *testing.T) {
	type tcase struct {
		absolute     bool
		external     bool
		targetExists bool
		dereference  bool
	}

	var tcases []tcase
	for _, absolute := range []bool{true, false} {
		for _, external := range []bool{true, false} {
			for _, targetExists := range []bool{true, false} {
				for _, dereference := range []bool{true, false} {
					tcases = append(tcases, tcase{
						absolute:     absolute,
						external:     external,
						targetExists: targetExists,
						dereference:  dereference,
					})
				}
			}
		}
	}

	for _, tc := range tcases {
		desc := fmt.Sprintf(
			"absolute:%v external:%v targetExists:%v dereference:%v",
			tc.absolute, tc.external, tc.targetExists, tc.dereference)

		t.Run(desc, func(t *testing.T) {
			td, err := ioutil.TempDir("", "go-slug")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(td)

			internal := filepath.Join(td, "internal")
			if err := os.MkdirAll(internal, 0700); err != nil {
				t.Fatal(err)
			}

			external := filepath.Join(td, "external")
			if err := os.MkdirAll(external, 0700); err != nil {
				t.Fatal(err)
			}

			// Make the symlink in a subdirectory. This will help ensure that
			// a proper relative link gets created, even when the relative
			// path requires upward directory traversal.
			symPath := filepath.Join(internal, "sub", "sym")
			if err := os.MkdirAll(filepath.Join(internal, "sub"), 0700); err != nil {
				t.Fatal(err)
			}

			// Get an absolute path within the temp dir and an absolute target.
			// We place the target into a subdir to ensure the link is created
			// properly within a nested structure.
			var targetPath string
			if tc.external {
				targetPath = filepath.Join(external, "foo", "bar")
			} else {
				targetPath = filepath.Join(internal, "foo", "bar")
			}

			if tc.targetExists {
				if err := os.MkdirAll(filepath.Dir(targetPath), 0700); err != nil {
					t.Fatal(err)
				}
				if err := ioutil.WriteFile(targetPath, []byte("foo"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			if !tc.absolute {
				targetPath, err = filepath.Rel(filepath.Dir(symPath), targetPath)
				if err != nil {
					t.Fatal(err)
				}
			}

			// Make the symlink.
			if err := os.Symlink(targetPath, symPath); err != nil {
				t.Fatal(err)
			}

			var expectErr string
			if tc.external && !tc.dereference {
				expectErr = "has external target"
			}
			if tc.external && tc.dereference && !tc.targetExists {
				expectErr = "no such file or directory"
			}

			var expectTypeflag byte
			if tc.external && tc.dereference {
				expectTypeflag = tar.TypeReg
			} else {
				expectTypeflag = tar.TypeSymlink
			}

			// Pack up the temp dir.
			slug := bytes.NewBuffer(nil)
			_, err = Pack(internal, slug, tc.dereference)
			if expectErr != "" {
				if err != nil {
					if strings.Contains(err.Error(), expectErr) {
						return
					}
					t.Fatalf("expected error %q, got %v", expectErr, err)
				}
				t.Fatal("expected error, got nil")
			} else if err != nil {
				t.Fatal(err)
			}

			// Inspect the result.
			gzipR, err := gzip.NewReader(slug)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			tarR := tar.NewReader(gzipR)

			symFound := false
			for {
				hdr, err := tarR.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("err: %v", err)
				}
				if hdr.Name == "sub/sym" {
					symFound = true
					if hdr.Typeflag != expectTypeflag {
						t.Fatalf("unexpected file type in slug: %q", hdr.Typeflag)
					}
					if expectTypeflag == tar.TypeSymlink && hdr.Linkname != targetPath {
						t.Fatalf("unexpected link target in slug: %q", hdr.Linkname)
					}
				}
			}

			if !symFound {
				t.Fatal("did not find symlink in archive")
			}
		})
	}
}

func TestAllowSymlinkTarget(t *testing.T) {
	tcases := []struct {
		desc   string
		allow  string
		target string
		err    string
	}{
		{
			desc:   "absolute symlink, exact match",
			allow:  "/foo/bar/baz",
			target: "/foo/bar/baz",
		},
		{
			desc:   "relative symlink, exact match",
			allow:  "../foo/bar",
			target: "../foo/bar",
		},
		{
			desc:   "absolute symlink, prefix match",
			allow:  "/foo/",
			target: "/foo/bar/baz",
		},
		{
			desc:   "relative symlink, prefix match",
			allow:  "../foo/",
			target: "../foo/bar/baz",
		},
		{
			desc:   "absolute symlink, non-match",
			allow:  "/zip",
			target: "/foo/bar/baz",
			err:    "has external target",
		},
		{
			desc:   "relative symlink, non-match",
			allow:  "../zip/",
			target: "../foo/bar/baz",
			err:    "has external target",
		},
		{
			desc:   "absolute symlink, embedded traversal, non-match",
			allow:  "/foo/",
			target: "/foo/../../zip",
			err:    "has external target",
		},
		{
			desc:   "relative symlink, embedded traversal, non-match",
			allow:  "../foo/",
			target: "../foo/../../zip",
			err:    "has external target",
		},
		{
			desc:   "absolute symlink, embedded traversal, match",
			allow:  "/foo/",
			target: "/foo/bar/../baz",
		},
		{
			desc:   "relative symlink, embedded traversal, match",
			allow:  "../foo/",
			target: "../foo/bar/../baz",
		},
		{
			desc:   "external target with embedded upward path traversal",
			allow:  "foo/bar/",
			target: "foo/bar/../../../lol",
			err:    "has external target",
		},
		{
			desc:   "similar file path, non-match",
			allow:  "/foo",
			target: "/foobar",
			err:    "has external target",
		},
	}

	for _, tc := range tcases {
		t.Run("Pack: "+tc.desc, func(t *testing.T) {
			td, err := ioutil.TempDir("", "go-slug")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(td)

			// Make the symlink.
			if err := os.Symlink(tc.target, filepath.Join(td, "sym")); err != nil {
				t.Fatal(err)
			}

			// Pack up the temp dir.
			slug := bytes.NewBuffer(nil)
			p, err := NewPacker(AllowSymlinkTarget(tc.allow))
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Pack(td, slug)
			if tc.err != "" {
				if err != nil {
					if strings.Contains(err.Error(), tc.err) {
						return
					}
					t.Fatalf("expected error %q, got %v", tc.err, err)
				}
				t.Fatal("expected error, got nil")
			} else if err != nil {
				t.Fatal(err)
			}

			// Inspect the result.
			gzipR, err := gzip.NewReader(slug)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			tarR := tar.NewReader(gzipR)

			symFound := false
			for {
				hdr, err := tarR.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("err: %v", err)
				}
				if hdr.Name == "sym" {
					symFound = true
					if hdr.Typeflag != tar.TypeSymlink {
						t.Fatalf("unexpected file type in slug: %q", hdr.Typeflag)
					}
					if hdr.Linkname != tc.target {
						t.Fatalf("unexpected link target in slug: %q", hdr.Linkname)
					}
				}
			}

			if !symFound {
				t.Fatal("did not find symlink in archive")
			}
		})

		t.Run("Unpack: "+tc.desc, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "slug")
			if err != nil {
				t.Fatalf("err:%v", err)
			}
			defer os.RemoveAll(dir)
			in := filepath.Join(dir, "slug.tar.gz")

			// Create the output file
			wfh, err := os.Create(in)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			// Gzip compress all the output data
			gzipW := gzip.NewWriter(wfh)

			// Tar the file contents
			tarW := tar.NewWriter(gzipW)

			// Write the header.
			tarW.WriteHeader(&tar.Header{
				Name:     "l",
				Linkname: tc.target,
				Typeflag: tar.TypeSymlink,
			})

			tarW.Close()
			gzipW.Close()
			wfh.Close()

			// Open the slug file for reading.
			fh, err := os.Open(in)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			// Create a dir to unpack into.
			dst, err := ioutil.TempDir(dir, "")
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			defer os.RemoveAll(dst)

			// Unpack.
			p, err := NewPacker(AllowSymlinkTarget(tc.allow))
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Unpack(fh, dst); err != nil {
				if tc.err != "" {
					if !strings.Contains(err.Error(), tc.err) {
						t.Fatalf("expected error %q, got %v", tc.err, err)
					}
				} else {
					t.Fatal(err)
				}
			} else if tc.err != "" {
				t.Fatalf("expected error %q, got nil", tc.err)
			}
		})
	}
}

func TestUnpack(t *testing.T) {
	// First create the slug file so we can try to unpack it.
	slug := bytes.NewBuffer(nil)

	if _, err := Pack("testdata/archive-dir-no-external", slug, true); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Create a dir to unpack into.
	dst, err := ioutil.TempDir("", "slug")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer os.RemoveAll(dst)

	// Now try unpacking it.
	if err := Unpack(slug, dst); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Verify all the files
	verifyFile(t, filepath.Join(dst, "bar.txt"), 0, "bar\n")
	verifyFile(t, filepath.Join(dst, "sub/bar.txt"), os.ModeSymlink, "../bar.txt")
	verifyFile(t, filepath.Join(dst, "sub/zip.txt"), 0, "zip\n")

	// Verify timestamps for files
	verifyTimestamps(t, "testdata/archive-dir-no-external/bar.txt", filepath.Join(dst, "bar.txt"))
	verifyTimestamps(t, "testdata/archive-dir-no-external/sub/zip.txt", filepath.Join(dst, "sub/zip.txt"))
	verifyTimestamps(t, "testdata/archive-dir-no-external/sub2/zip.txt", filepath.Join(dst, "sub2/zip.txt"))

	// Verify timestamps for symlinks
	if unpackinfo.CanMaintainSymlinkTimestamps() {
		verifyTimestamps(t, "testdata/archive-dir-no-external/sub/bar.txt", filepath.Join(dst, "sub/bar.txt"))
	}

	// Verify timestamps for directories
	verifyTimestamps(t, "testdata/archive-dir-no-external/foo.terraform", filepath.Join(dst, "foo.terraform"))
	verifyTimestamps(t, "testdata/archive-dir-no-external/sub", filepath.Join(dst, "sub"))
	verifyTimestamps(t, "testdata/archive-dir-no-external/sub2", filepath.Join(dst, "sub2"))

	// Check that we can set permissions properly
	verifyPerms(t, filepath.Join(dst, "bar.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "sub/zip.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "sub/bar.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "exe"), 0755)
}

func TestUnpack_HeaderOrdering(t *testing.T) {
	// Tests that when a tar file has subdirectories ordered before parent directories, the
	// timestamps get restored correctly in the plaform where the tests are run.

	// This file is created by the go program found in `testdata/subdir-ordering`
	f, err := os.Open("testdata/subdir-appears-first.tar.gz")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()

	packer, err := NewPacker()
	if err != nil {
		t.Fatalf("expected no error, got %s", err)
	}

	err = packer.Unpack(f, dir)
	if err != nil {
		t.Fatalf("expected no error, got %s", err)
	}

	// These times were recorded when the archive was created
	testCases := []struct {
		Path string
		TS   time.Time
	}{
		{TS: time.Unix(0, 1698787142347461403).Round(time.Second), Path: path.Join(dir, "super/duper")},
		{TS: time.Unix(0, 1698780461367973574).Round(time.Second), Path: path.Join(dir, "super")},
		{TS: time.Unix(0, 1698787142347461286).Round(time.Second), Path: path.Join(dir, "super/duper/trooper")},
		{TS: time.Unix(0, 1698780470254368545).Round(time.Second), Path: path.Join(dir, "super/duper/trooper/foo.txt")},
	}

	for _, tc := range testCases {
		info, err := os.Stat(tc.Path)
		if err != nil {
			t.Fatalf("error when stat %q: %s", tc.Path, err)
		}
		if info.ModTime() != tc.TS {
			t.Errorf("timestamp of file %q (%d) did not match expected value %d", tc.Path, info.ModTime().UnixNano(), tc.TS.UnixNano())
		}
	}
}

func TestUnpackDuplicateNoWritePerm(t *testing.T) {
	dir, err := ioutil.TempDir("", "slug")
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "slug.tar.gz")

	// Create the output file
	wfh, err := os.Create(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Gzip compress all the output data
	gzipW := gzip.NewWriter(wfh)

	// Tar the file contents
	tarW := tar.NewWriter(gzipW)

	var hdr tar.Header

	data := "this is a\n"

	hdr.Name = "a"
	hdr.Mode = 0100000 | 0400
	hdr.Size = int64(len(data))

	tarW.WriteHeader(&hdr)
	tarW.Write([]byte(data))

	// write it twice
	tarW.WriteHeader(&hdr)
	tarW.Write([]byte(data))

	tarW.Close()
	gzipW.Close()
	wfh.Close()

	// Open the slug file for reading.
	fh, err := os.Open(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Create a dir to unpack into.
	dst, err := ioutil.TempDir(dir, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer os.RemoveAll(dst)

	// Now try unpacking it.
	if err := Unpack(fh, dst); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Verify all the files
	verifyFile(t, filepath.Join(dst, "a"), 0, "this is a\n")

	// Check that we can set permissions properly
	verifyPerms(t, filepath.Join(dst, "a"), 0400)
}

func TestUnpackPaxHeaders(t *testing.T) {
	tcases := []struct {
		desc    string
		headers []*tar.Header
	}{
		{
			desc: "extended pax header",
			headers: []*tar.Header{
				{
					Name:     "h",
					Typeflag: tar.TypeXHeader,
				},
			},
		},
		{
			desc: "global pax header",
			headers: []*tar.Header{
				{
					Name:     "h",
					Typeflag: tar.TypeXGlobalHeader,
				},
			},
		},
	}

	for _, tc := range tcases {
		t.Run(tc.desc, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "slug")
			if err != nil {
				t.Fatalf("err:%v", err)
			}
			defer os.RemoveAll(dir)
			in := filepath.Join(dir, "slug.tar.gz")

			// Create the output file
			wfh, err := os.Create(in)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			// Gzip compress all the output data
			gzipW := gzip.NewWriter(wfh)

			// Tar the file contents
			tarW := tar.NewWriter(gzipW)

			for _, hdr := range tc.headers {
				tarW.WriteHeader(hdr)
			}

			tarW.Close()
			gzipW.Close()
			wfh.Close()

			// Open the slug file for reading.
			fh, err := os.Open(in)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			// Create a dir to unpack into.
			dst, err := ioutil.TempDir(dir, "")
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			defer os.RemoveAll(dst)

			// Now try unpacking it.
			if err := Unpack(fh, dst); err != nil {
				t.Fatalf("err: %v", err)
			}

			// Verify no file was created.
			path := filepath.Join(dst, "h")
			fh, err = os.Open(path)
			if err == nil {
				t.Fatalf("expected file not to exist: %q", path)
			}
			defer fh.Close()
		})
	}
}

// ensure Unpack returns an error when an unsupported file type is encountered
// in an archive, rather than silently discarding the data.
func TestUnpackErrorOnUnhandledType(t *testing.T) {
	dir, err := ioutil.TempDir("", "slug")
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "slug.tar.gz")

	// Create the output file
	wfh, err := os.Create(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Gzip compress all the output data
	gzipW := gzip.NewWriter(wfh)

	// Tar the file contents
	tarW := tar.NewWriter(gzipW)

	var hdr tar.Header

	hdr.Typeflag = tar.TypeFifo // we're unlikely to support FIFOs :-)
	hdr.Name = "l"
	hdr.Size = int64(0)

	tarW.WriteHeader(&hdr)

	tarW.Close()
	gzipW.Close()
	wfh.Close()

	// Open the slug file for reading.
	fh, err := os.Open(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Create a dir to unpack into.
	dst, err := ioutil.TempDir(dir, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer os.RemoveAll(dst)

	// Now try unpacking it, which should fail
	if err := Unpack(fh, dst); err == nil {
		t.Fatalf("should have gotten error unpacking slug with fifo, got none")
	}
}

func TestUnpackMaliciousSymlinks(t *testing.T) {
	tcases := []struct {
		desc    string
		headers []*tar.Header
		err     string
	}{
		{
			desc: "symlink with absolute path",
			headers: []*tar.Header{
				&tar.Header{
					Name:     "l",
					Linkname: "/etc/shadow",
					Typeflag: tar.TypeSymlink,
				},
			},
			err: "has external target",
		},
		{
			desc: "symlink with external target",
			headers: []*tar.Header{
				&tar.Header{
					Name:     "l",
					Linkname: "../../../../../etc/shadow",
					Typeflag: tar.TypeSymlink,
				},
			},
			err: "has external target",
		},
		{
			desc: "symlink with nested external target",
			headers: []*tar.Header{
				&tar.Header{
					Name:     "l",
					Linkname: "foo/bar/baz/../../../../../../../../etc/shadow",
					Typeflag: tar.TypeSymlink,
				},
			},
			err: "has external target",
		},
		{
			desc: "zipslip vulnerability",
			headers: []*tar.Header{
				&tar.Header{
					Name:     "subdir/parent",
					Linkname: "..",
					Typeflag: tar.TypeSymlink,
				},
				&tar.Header{
					Name:     "subdir/parent/escapes",
					Linkname: "..",
					Typeflag: tar.TypeSymlink,
				},
			},
			err: `cannot extract "subdir/parent/escapes" through symlink`,
		},
		{
			desc: "nested symlinks within symlinked dir",
			headers: []*tar.Header{
				&tar.Header{
					Name:     "subdir/parent",
					Linkname: "..",
					Typeflag: tar.TypeSymlink,
				},
				&tar.Header{
					Name:     "subdir/parent/otherdir/escapes",
					Linkname: "../..",
					Typeflag: tar.TypeSymlink,
				},
			},
			err: `cannot extract "subdir/parent/otherdir/escapes" through symlink`,
		},
		{
			desc: "regular file through symlink",
			headers: []*tar.Header{
				&tar.Header{
					Name:     "subdir/parent",
					Linkname: "..",
					Typeflag: tar.TypeSymlink,
				},
				&tar.Header{
					Name:     "subdir/parent/file",
					Typeflag: tar.TypeReg,
				},
			},
			err: `cannot extract "subdir/parent/file" through symlink`,
		},
		{
			desc: "directory through symlink",
			headers: []*tar.Header{
				&tar.Header{
					Name:     "subdir/parent",
					Linkname: "..",
					Typeflag: tar.TypeSymlink,
				},
				&tar.Header{
					Name:     "subdir/parent/dir",
					Typeflag: tar.TypeDir,
				},
			},
			err: `cannot extract "subdir/parent/dir" through symlink`,
		},
	}

	for _, tc := range tcases {
		t.Run(tc.desc, func(t *testing.T) {

			dir, err := ioutil.TempDir("", "slug")
			if err != nil {
				t.Fatalf("err:%v", err)
			}
			defer os.RemoveAll(dir)
			in := filepath.Join(dir, "slug.tar.gz")

			// Create the output file
			wfh, err := os.Create(in)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			// Gzip compress all the output data
			gzipW := gzip.NewWriter(wfh)

			// Tar the file contents
			tarW := tar.NewWriter(gzipW)

			for _, hdr := range tc.headers {
				tarW.WriteHeader(hdr)
			}

			tarW.Close()
			gzipW.Close()
			wfh.Close()

			// Open the slug file for reading.
			fh, err := os.Open(in)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			// Create a dir to unpack into.
			dst, err := ioutil.TempDir(dir, "")
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			defer os.RemoveAll(dst)

			// Now try unpacking it, which should fail
			var e *IllegalSlugError
			err = Unpack(fh, dst)
			if err == nil || !errors.As(err, &e) || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("expected *IllegalSlugError %v, got %T %v", tc.err, err, err)
			}
		})
	}
}

func TestUnpackMaliciousFiles(t *testing.T) {
	tcases := []struct {
		desc string
		name string
		err  string
	}{
		{
			desc: "filename containing path traversal",
			name: "../../../../../../../../tmp/test",
			err:  "invalid filename, traversal with \"..\" outside of current directory",
		},
		{
			desc: "should fail before attempting to create directories",
			name: "../../../../../../../../Users/root",
			err:  "invalid filename, traversal with \"..\" outside of current directory",
		},
	}

	for _, tc := range tcases {
		t.Run(tc.desc, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "slug")
			if err != nil {
				t.Fatalf("err:%v", err)
			}
			defer os.RemoveAll(dir)
			in := filepath.Join(dir, "slug.tar.gz")

			// Create the output file
			wfh, err := os.Create(in)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			// Gzip compress all the output data
			gzipW := gzip.NewWriter(wfh)

			// Tar the file contents
			tarW := tar.NewWriter(gzipW)

			hdr := &tar.Header{
				Name: tc.name,
				Mode: 0600,
				Size: int64(0),
			}
			if err := tarW.WriteHeader(hdr); err != nil {
				t.Fatalf("err: %v", err)
			}
			if _, err := tarW.Write([]byte{}); err != nil {
				t.Fatalf("err: %v", err)
			}

			tarW.Close()
			gzipW.Close()
			wfh.Close()

			// Open the slug file for reading.
			fh, err := os.Open(in)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			// Create a dir to unpack into.
			dst, err := ioutil.TempDir(dir, "")
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			defer os.RemoveAll(dst)

			// Now try unpacking it, which should fail
			var e *IllegalSlugError
			err = Unpack(fh, dst)
			if err == nil || !errors.As(err, &e) || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("expected *IllegalSlugError %v, got %T %v", tc.err, err, err)
			}
		})
	}
}

func TestCheckFileMode(t *testing.T) {
	for _, tc := range []struct {
		desc string
		mode os.FileMode
		keep bool
		body bool
	}{
		{"includes regular files", 0, true, true},
		{"includes directories", os.ModeDir, true, false},
		{"includes symlinks", os.ModeSymlink, true, false},
		{"excludes unrecognized modes", os.ModeDevice, false, false},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			keep, body := checkFileMode(tc.mode)
			if keep != tc.keep || body != tc.body {
				t.Fatalf("expect (%v, %v), got (%v, %v)",
					tc.keep, tc.body, keep, body)
			}
		})
	}
}

func TestNewPacker(t *testing.T) {
	for _, tc := range []struct {
		desc    string
		options []PackerOption
		expect  *Packer
	}{
		{
			desc: "defaults",
			expect: &Packer{
				dereference:          false,
				applyTerraformIgnore: false,
			},
		},
		{
			desc:    "enable dereferencing",
			options: []PackerOption{DereferenceSymlinks()},
			expect: &Packer{
				dereference: true,
			},
		},
		{
			desc:    "apply .terraformignore",
			options: []PackerOption{ApplyTerraformIgnore()},
			expect: &Packer{
				applyTerraformIgnore: true,
			},
		},
		{
			desc:    "multiple options",
			options: []PackerOption{ApplyTerraformIgnore(), DereferenceSymlinks()},
			expect: &Packer{
				dereference:          true,
				applyTerraformIgnore: true,
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			p, err := NewPacker(tc.options...)
			if err != nil {
				t.Fatalf("err: %v", err)
			}

			if !reflect.DeepEqual(p, tc.expect) {
				t.Fatalf("\nexpect:\n%#v\n\nactual:\n%#v", p, tc.expect)
			}
		})
	}
}

func TestUnpackEmptyName(t *testing.T) {
	var buf bytes.Buffer

	gw := gzip.NewWriter(&buf)

	tw := tar.NewWriter(gw)

	tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
	})

	tw.Close()
	gw.Close()

	if buf.Len() == 0 {
		t.Fatal("unable to create tar properly")
	}

	dir, err := ioutil.TempDir("", "slug")
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	defer os.RemoveAll(dir)

	err = Unpack(&buf, dir)
	if err != nil {
		t.Fatalf("err:%v", err)
	}
}

// This is a reusable assertion for when packing testdata/archive-dir
func assertArchiveFixture(t *testing.T, slug *bytes.Buffer, got *Meta) {
	gzipR, err := gzip.NewReader(slug)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	tarR := tar.NewReader(gzipR)
	var (
		sym1Found           bool
		sym2Found           bool
		externalTargetFound bool
		fileList            []string
		slugSize            int64
	)

	for {
		hdr, err := tarR.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("err: %v", err)
		}

		fileList = append(fileList, hdr.Name)
		if hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeRegA {
			slugSize += hdr.Size
		}

		if hdr.Name == "sub/bar.txt" {
			if hdr.Typeflag != tar.TypeSymlink {
				t.Fatalf("expect symlink for file 'sub/bar.txt'")
			}
			if hdr.Linkname != "../bar.txt" {
				t.Fatalf("expect target of '../bar.txt', got %q", hdr.Linkname)
			}
			sym1Found = true
		}

		if hdr.Name == "sub2/bar.txt" {
			if hdr.Typeflag != tar.TypeSymlink {
				t.Fatalf("expect symlink for file 'sub2/bar.txt'")
			}
			if hdr.Linkname != "../sub/bar.txt" {
				t.Fatalf("expect target of '../sub/bar.txt', got %q", hdr.Linkname)
			}
			sym2Found = true
		}

		if hdr.Name == "example.tf" {
			if hdr.Typeflag != tar.TypeReg {
				t.Fatalf("expected symlink to be dereferenced 'example.tf'")
			}
			externalTargetFound = true
		}
	}

	// Make sure we saw and handled a symlink
	if !sym1Found || !sym2Found {
		t.Fatal("expected to find two symlinks")
	}

	// Make sure we saw and handled a dereferenced symlink
	if !externalTargetFound {
		t.Fatal("expected to find dereferenced symlink")
	}

	// Make sure the .git directory is ignored
	for _, file := range fileList {
		if strings.Contains(file, ".git") {
			t.Fatalf("unexpected .git content: %s", file)
		}
	}

	// Make sure the .terraform directory is ignored,
	// except for the .terraform/modules subdirectory.
	for _, file := range fileList {
		if strings.HasPrefix(file, ".terraform"+string(filepath.Separator)) &&
			!strings.HasPrefix(file, filepath.Clean(".terraform/modules")) {
			t.Fatalf("unexpected .terraform content: %s", file)
		}
	}

	// Make sure .terraform/modules is included.
	moduleDir := false
	for _, file := range fileList {
		if strings.HasPrefix(file, filepath.Clean(".terraform/modules")) {
			moduleDir = true
			break
		}
	}
	if !moduleDir {
		t.Fatal("expected to include .terraform/modules")
	}

	// Make sure .terraformrc is included.
	terraformrc := false
	for _, file := range fileList {
		if file == ".terraformrc" {
			terraformrc = true
			break
		}
	}
	if !terraformrc {
		t.Fatal("expected to include .terraformrc")
	}

	// Make sure foo.terraform/bar.txt is included.
	fooTerraformDir := false
	for _, file := range fileList {
		if file == filepath.Clean("foo.terraform/bar.txt") {
			fooTerraformDir = true
			break
		}
	}
	if !fooTerraformDir {
		t.Fatal("expected to include foo.terraform/bar.txt")
	}

	// Make sure baz.txt is excluded.
	bazTxt := false
	for _, file := range fileList {
		if file == filepath.Clean("baz.txt") {
			bazTxt = true
			break
		}
	}
	if bazTxt {
		t.Fatal("should not include baz.txt")
	}

	// Check the metadata
	expect := &Meta{
		Files: fileList,
		Size:  slugSize,
	}
	if !reflect.DeepEqual(got, expect) {
		t.Fatalf("\nexpect:\n%#v\n\nactual:\n%#v", expect, got)
	}
}

func verifyTimestamps(t *testing.T, src, dst string) {
	sourceInfo, err := os.Lstat(src)
	if err != nil {
		t.Fatalf("source file %q not found", src)
	}

	dstInfo, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("dst file %q not found", dst)
	}

	// archive/tar purports to round timestamps to the nearest second so that behavior
	// is duplicated here to test the restored timestamps.
	sourceModTime := sourceInfo.ModTime().Round(time.Second)
	destModTime := dstInfo.ModTime()

	if !sourceModTime.Equal(destModTime) {
		t.Fatalf("source %q and dst %q do not have the same mtime (%q and %q, respectively)", src, dst, sourceModTime, destModTime)
	}
}

func verifyFile(t *testing.T, dst string, expectedMode fs.FileMode, expectedTarget string) {
	info, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("dst file %q not found", dst)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		if expectedMode == os.ModeSymlink {
			if target, _ := os.Readlink(dst); target != expectedTarget {
				t.Fatalf("expect link target %q, got %q", expectedTarget, target)
			}
			return
		} else {
			t.Fatalf("found symlink, expected %v", expectedMode)
		}
	}

	if !((expectedMode == 0 && info.Mode().IsRegular()) || info.Mode()&expectedMode == 0) {
		t.Fatalf("wrong file mode for %q", dst)
	}

	fh, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()

	raw := make([]byte, info.Size())
	if _, err := fh.Read(raw); err != nil {
		t.Fatal(err)
	}
	if result := string(raw); result != expectedTarget {
		t.Fatalf("bad content in file %q\n\nexpect:\n%#v\n\nactual:\n%#v",
			dst, expectedTarget, result)
	}
}

func verifyPerms(t *testing.T, path string, expect os.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != expect {
		t.Fatalf("expect perms %o for path %s, got %o", expect, path, perm)
	}
}
