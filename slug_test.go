package slug

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

	meta, err := Pack("test-fixtures/archive-dir", slug, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	gzipR, err := gzip.NewReader(slug)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	tarR := tar.NewReader(gzipR)
	var (
		symFound bool
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

		if hdr.Name == "sub/bar.txt" {
			if hdr.Typeflag != tar.TypeSymlink {
				t.Fatalf("expect symlink for file 'sub/bar.txt'")
			}
			if hdr.Linkname != "../bar.txt" {
				t.Fatalf("expect target of '../bar.txt', got %q", hdr.Linkname)
			}
			symFound = true
		}
	}

	// Make sure we saw and handled a symlink
	if !symFound {
		t.Fatal("expected to find symlink")
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

	// Check the metadata
	expect := &Meta{
		Files: fileList,
		Size:  slugSize,
	}
	if !reflect.DeepEqual(meta, expect) {
		t.Fatalf("\nexpect:\n%#v\n\nactual:\n%#v", expect, meta)
	}
}

func TestPackWithDereferencing(t *testing.T) {
	slug := bytes.NewBuffer(nil)

	meta, err := Pack("test-fixtures/archive-dir", slug, true)
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

		if hdr.Name == "foo.txt" {
			if hdr.Typeflag != tar.TypeReg {
				t.Fatalf("expect symlink 'foo.txt' to be dereferenced")
			}
		}
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

func TestPackWithoutDereferencing(t *testing.T) {
	slug := bytes.NewBuffer(nil)

	meta, err := Pack("test-fixtures/archive-dir", slug, false)
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

	// Make sure the the foo.txt symlink is not dereferenced
	// but is indeed ignored and not added to the archive.
	for _, file := range fileList {
		if file == "foo.txt" {
			t.Fatalf("unexpected dereferenced symlink: %s", file)
		}
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

func TestUnarchive(t *testing.T) {
	// First create the slug file so we can try to unpack it.
	slug := bytes.NewBuffer(nil)

	if _, err := Pack("test-fixtures/archive-dir", slug, true); err != nil {
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
	verifyFile(t, filepath.Join(dst, "foo.txt"), 0, "foo\n")
	verifyFile(t, filepath.Join(dst, "bar.txt"), 0, "bar\n")
	verifyFile(t, filepath.Join(dst, "sub", "bar.txt"), os.ModeSymlink, "../bar.txt")
	verifyFile(t, filepath.Join(dst, "sub", "zip.txt"), 0, "zip\n")

	// Check that we can set permissions properly
	verifyPerms(t, filepath.Join(dst, "foo.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "bar.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "sub", "zip.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "sub", "bar.txt"), 0644)
	verifyPerms(t, filepath.Join(dst, "exe"), 0755)
}

func TestUnarchiveDuplicateNoWritePerm(t *testing.T) {
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

// ensure Unpack returns an error when an unsupported file type is encountered
// in an archive, rather than silently discarding the data.
func TestUnarchiveErrorOnUnhandledType(t *testing.T) {
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

func verifyFile(t *testing.T, path string, mode os.FileMode, expect string) {
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()

	info, err := fh.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !((mode == 0 && info.Mode().IsRegular()) || info.Mode()&mode == 0) {
		t.Fatalf("wrong file mode for %q", path)
	}

	if mode == os.ModeSymlink {
		if target, _ := os.Readlink(path); target != expect {
			t.Fatalf("expect link target %q, got %q", expect, target)
		}
		return
	}

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
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != expect {
		t.Fatalf("expect perms %o, got %o", expect, perm)
	}
}
