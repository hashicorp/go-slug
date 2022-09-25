package slugoverlay

import (
	"bytes"
	"errors"
	"hash/crc32"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/hashicorp/go-slug"
)

func TestOverlayPackerChecksums(t *testing.T) {
	t.Parallel()

	p, err := NewOverlayPacker("testdata/archive-dir")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	t.Run("rebase checksums files", func(t *testing.T) {
		t.Parallel()

		testFiles := []string{"sub/zip.txt", "exe", ".terraform/modules/README"}

		for _, f := range testFiles {
			expectedChecksum := simpleChecksum(t, "testdata/archive-dir/"+f)
			entry, ok := p.checksums[f]
			if !ok {
				t.Fatalf("expected %s in checksum set, but was not found", f)
			}

			if entry.crc32 != expectedChecksum {
				t.Fatalf("expected %s in checksum %d to match calculated checksum %d", f, entry.crc32, expectedChecksum)
			}
		}
	})

	t.Run("rebase identifies directories", func(t *testing.T) {
		t.Parallel()

		testDirs := []string{"sub", ".terraform"}

		for _, d := range testDirs {
			entry, ok := p.checksums[d]
			if !ok {
				t.Fatalf("expected %s in checksum set, but was not found", d)
			}
			if !entry.isDir {
				t.Fatalf("expected %s in checksum set to be a directory", d)
			}
		}
	})
}

func TestOverlayPackerBaseNotExists(t *testing.T) {
	t.Parallel()

	_, err := NewOverlayPacker("nonexisting")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected err to be os.ErrNotExist but got %v", err)
	}
}

func TestOverlayPackerBaseSlashSuffix(t *testing.T) {
	t.Parallel()

	p, err := NewOverlayPacker("testdata/archive-dir///")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(p.checksums) <= 0 {
		t.Fatalf("expected a snapshot, but snapshot contained %d files", len(p.checksums))
	}
}

func TestOverlayPackerDiffCreatedFiles(t *testing.T) {
	t.Parallel()

	p, dir := duplicateArchiveDir(t)

	// create a file in the duplicate dir
	err := os.WriteFile(dir+"/new_file.txt", []byte("Hello, World"), 0644)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	slug := bytes.NewBuffer(nil)

	meta, err := p.PackOverlay(slug)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	checkForSingleFile(t, meta, "new_file.txt")
}

func TestOverlayPackerDiffModifiedFileIsNotDirectory(t *testing.T) {
	t.Parallel()

	// Delete the directory called sub
	p, dir := duplicateArchiveDir(t)
	err := os.RemoveAll(dir + "/sub")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Create a file called sub
	err = os.WriteFile(dir+"/sub", []byte("replaced contents"), 0644)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	slug := bytes.NewBuffer(nil)
	meta, err := p.PackOverlay(slug)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	checkForSingleFile(t, meta, "sub")
}

func TestOverlayPackerDiffModifiedDirectoryIsNotFile(t *testing.T) {
	t.Parallel()

	// Delete the file called sub/bar.txt, which is a symbol
	p, dir := duplicateArchiveDir(t)
	err := os.RemoveAll(dir + "/sub/bar.txt")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Create a directory called sub/bar.txt
	err = os.Mkdir(dir+"/sub/bar.txt", 0700)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Create a file called bar.txt/new-file
	err = os.WriteFile(dir+"/sub/bar.txt/new-file", []byte("replaced contents"), 0644)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	slug := bytes.NewBuffer(nil)
	meta, err := p.PackOverlay(slug)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	checkForSingleFile(t, meta, "sub/bar.txt/new-file")
}

func TestOverlayPackerDiffModifiedFiles(t *testing.T) {
	t.Parallel()

	p, dir := duplicateArchiveDir(t)

	err := os.WriteFile(dir+"/sub/zip.txt", []byte("replaced contents"), 0644)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	slug := bytes.NewBuffer(nil)
	meta, err := p.PackOverlay(slug)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	checkForSingleFile(t, meta, "sub/zip.txt")
}

func TestOverlayPackerDiffDeletedFiles(t *testing.T) {
	t.Parallel()

	p, dir := duplicateArchiveDir(t)

	fileToDelete := ".terraform/modules/README"
	err := os.Remove(path.Join(dir, fileToDelete))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	zipDir, err := os.MkdirTemp("", "slug-overlay")
	if err != nil {
		t.Fatalf("err:%v", err)
	}
	defer os.RemoveAll(zipDir)
	in := path.Join(zipDir, "slug.tar.gz")

	// Create the output file
	wfh, err := os.Create(in)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	_, err = p.PackOverlay(wfh)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	wfh.Close()

	_, baseDir := duplicateArchiveDir(t)
	_, err = os.Stat(path.Join(baseDir, fileToDelete))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Open the slug file for reading.
	fh, err := os.Open(in)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Unzip the pack into a directory known to have the file deleted
	err = p.UnpackOverlay(fh, baseDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// It should not exist
	_, err = os.Stat(path.Join(baseDir, fileToDelete))
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}

	// The tombstone file should not exist, either
	_, err = os.Stat(path.Join(baseDir, fileToDelete+".tombstone"))
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %s", err)
	}
}

func simpleChecksum(t *testing.T, path string) uint32 {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	buf, err := ioutil.ReadAll(file)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	return crc32.ChecksumIEEE(buf)
}

func duplicateArchiveDir(t *testing.T) (*OverlayPacker, string) {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "overlay-packer-archive-dir")
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	cmd := exec.Command("cp", "-R", "testdata/archive-dir", tempDir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// cp copies the root directory if tempDir already exists
	tempDir = path.Join(tempDir, "archive-dir")

	p, err := NewOverlayPacker(tempDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	return p, tempDir
}

func checkForSingleFile(t *testing.T, meta *slug.Meta, expected string) {
	t.Helper()

	if len(meta.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(meta.Files))
	}

	if meta.Files[0] != expected {
		t.Fatalf("expected %s to be present in metadata, but got %s", expected, meta.Files[0])
	}
}
