package slug

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPack(t *testing.T) {
	slug := bytes.NewBuffer(nil)
	meta, err := Pack("testdata/archive-dir", slug, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	assertArchiveFixture(t, slug, meta)
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
	verifyFile(t, filepath.Join(dst, "sub", "bar.txt"), os.ModeSymlink, "../bar.txt")
	verifyFile(t, filepath.Join(dst, "sub", "zip.txt"), 0, "zip\n")

	// Check that we can set permissions properly
	verifyPerms(t, filepath.Join(dst, "bar.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "sub", "zip.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "sub", "bar.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "exe"), 0755)
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
				&tar.Header{
					Name:     "h",
					Typeflag: tar.TypeXHeader,
				},
			},
		},
		{
			desc: "global pax header",
			headers: []*tar.Header{
				&tar.Header{
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

	// This crashes unless the bug is fixed
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
		symFound            bool
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
			symFound = true
		}

		if hdr.Name == "example.tf" {
			if hdr.Typeflag != tar.TypeSymlink {
				t.Fatalf("expected symlink file 'example.tf'")
			}
			externalTargetFound = true
		}
	}

	// Make sure we saw and handled a symlink
	if !symFound {
		t.Fatal("expected to find symlink")
	}

	// Make sure we saw and handled a nested symlink
	if !externalTargetFound {
		t.Fatal("expected to find nested symlink")
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

func verifyFile(t *testing.T, path string, mode os.FileMode, expect string) {
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		if mode == os.ModeSymlink {
			if target, _ := os.Readlink(path); target != expect {
				t.Fatalf("expect link target %q, got %q", expect, target)
			}
			return
		} else {
			t.Fatalf("found symlink, expected %v", mode)
		}
	}

	if !((mode == 0 && info.Mode().IsRegular()) || info.Mode()&mode == 0) {
		t.Fatalf("wrong file mode for %q", path)
	}

	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()

	raw := make([]byte, info.Size())
	if _, err := fh.Read(raw); err != nil {
		t.Fatal(err)
	}
	if result := string(raw); result != expect {
		t.Fatalf("bad content in file %q\n\nexpect:\n%#v\n\nactual:\n%#v",
			path, expect, result)
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
