package slug

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Meta provides detailed information about a slug.
type Meta struct {
	// The list of files contained in the slug.
	Files []string

	// Total size of the slug in bytes.
	Size int64
}

// IllegalSlugError indicates the provided slug (io.Writer for Pack, io.Reader
// for Unpack) violates a rule about its contents. For example, an absolute or
// external symlink. It implements the error interface.
type IllegalSlugError struct {
	Err error
}

func (e *IllegalSlugError) Error() string {
	return fmt.Sprintf("illegal slug error: %v", e.Err)
}

// Unwrap returns the underlying issue with the provided Slug into the error
// chain.
func (e *IllegalSlugError) Unwrap() error { return e.Err }

// PackerOption is a functional option that can configure non-default Packers.
type PackerOption func(*Packer) error

// ApplyTerraformIgnore is a PackerOption that will apply the .terraformignore
// rules and skip packing files it specifies.
func ApplyTerraformIgnore() PackerOption {
	return func(p *Packer) error {
		p.applyTerraformIgnore = true
		return nil
	}
}

// resolvedSymlink is a simple abstraction for a resolved symlink
type resolvedSymlink struct {
	absTarget string
	target    string
	info      os.FileInfo
}

// DereferenceSymlinks is a PackerOption that will allow symlinks that
// reference a target outside of the source directory by copying the link
// target, turning it into a normal file within the archive.
func DereferenceSymlinks() PackerOption {
	return func(p *Packer) error {
		p.dereference = true
		return nil
	}
}

// AllowSymlinkTarget relaxes safety checks on symlinks with targets matching
// path. Specifically, absolute symlink targets (e.g. "/foo/bar") and relative
// targets (e.g. "../foo/bar") which resolve to a path outside of the
// source/destination directories for pack/unpack operations respectively, may
// be expressly permitted, whereas they are forbidden by default. Exercise
// caution when using this option. A symlink matches path if its target
// resolves to path exactly, or if path is a parent directory of target.
func AllowSymlinkTarget(path string) PackerOption {
	return func(p *Packer) error {
		p.allowSymlinkTargets = append(p.allowSymlinkTargets, path)
		return nil
	}
}

// Packer holds options for the Pack function.
type Packer struct {
	dereference          bool
	applyTerraformIgnore bool
	allowSymlinkTargets  []string
}

// NewPacker is a constructor for Packer.
func NewPacker(options ...PackerOption) (*Packer, error) {
	p := &Packer{
		dereference:          false,
		applyTerraformIgnore: false,
	}

	for _, opt := range options {
		if err := opt(p); err != nil {
			return nil, fmt.Errorf("option failed: %w", err)
		}
	}

	return p, nil
}

// Pack at the package level is used to maintain compatibility with existing
// code that relies on this function signature. New options related to packing
// slugs should be added to the Packer struct instead.
func Pack(src string, w io.Writer, dereference bool) (*Meta, error) {
	p := Packer{
		dereference: dereference,

		// This defaults to false in NewPacker, but is true here. This matches
		// the old behavior of Pack, which always used .terraformignore.
		applyTerraformIgnore: true,
	}
	return p.Pack(src, w)
}

// Pack creates a slug from a src directory, and writes the new slug
// to w. Returns metadata about the slug and any errors.
//
// When dereference is set to true, symlinks with a target outside of
// the src directory will be dereferenced. When dereference is set to
// false symlinks with a target outside the src directory are omitted
// from the slug.
func (p *Packer) Pack(src string, w io.Writer) (*Meta, error) {
	// Gzip compress all the output data.
	gzipW, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		// This error is only raised when an incorrect gzip level is
		// specified.
		return nil, err
	}

	// Tar the file contents.
	tarW := tar.NewWriter(gzipW)

	// Load the ignore rule configuration, which will use
	// defaults if no .terraformignore is configured
	var ignoreRules []rule
	if p.applyTerraformIgnore {
		ignoreRules = parseIgnoreFile(src)
	}

	// Track the metadata details as we go.
	meta := &Meta{}

	// Walk the tree of files.
	err = filepath.Walk(src, p.packWalkFn(src, src, src, tarW, meta, ignoreRules))
	if err != nil {
		return nil, err
	}

	// Flush the tar writer.
	if err := tarW.Close(); err != nil {
		return nil, fmt.Errorf("failed to close the tar archive: %w", err)
	}

	// Flush the gzip writer.
	if err := gzipW.Close(); err != nil {
		return nil, fmt.Errorf("failed to close the gzip writer: %w", err)
	}

	return meta, nil
}

func (p *Packer) packWalkFn(root, src, dst string, tarW *tar.Writer, meta *Meta, ignoreRules []rule) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get the relative path from the current src directory.
		subpath, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for file %q: %w", path, err)
		}
		if subpath == "." {
			return nil
		}

		if m := matchIgnoreRule(subpath, ignoreRules); m {
			return nil
		}

		// Catch directories so we don't end up with empty directories,
		// the files are ignored correctly
		if info.IsDir() {
			if m := matchIgnoreRule(subpath+string(os.PathSeparator), ignoreRules); m {
				return nil
			}
		}

		// Get the relative path from the initial root directory.
		subpath, err = filepath.Rel(root, strings.Replace(path, src, dst, 1))
		if err != nil {
			return fmt.Errorf("failed to get relative path for file %q: %w", path, err)
		}
		if subpath == "." {
			return nil
		}

		// Check the file type and if we need to write the body.
		keepFile, writeBody := checkFileMode(info.Mode())
		if !keepFile {
			return nil
		}

		fm := info.Mode()
		header := &tar.Header{
			Name:    filepath.ToSlash(subpath),
			ModTime: info.ModTime(),
			Mode:    int64(fm.Perm()),
		}

		switch {
		case info.IsDir():
			header.Typeflag = tar.TypeDir
			header.Name += "/"

		case fm.IsRegular():
			header.Typeflag = tar.TypeReg
			header.Size = info.Size()

		case fm&os.ModeSymlink != 0:
			resolved, err := p.resolveLink(root, path, header)
			if err != nil {
				return err
			}

			// Ensure the target is acceptable per the Packer's configuration.
			if err := p.checkSymlink(root, path, resolved.target); err != nil {
				// Check if dereferencing is enabled. If so, we're going to
				// try copying the symlink's data. If not, this is an error.
				if !p.dereference {
					return err
				}
			} else {
				header.Typeflag = tar.TypeSymlink
				header.Linkname = filepath.ToSlash(resolved.absTarget)
				break
			}

			// If the target is a directory we can recurse into the target
			// directory by calling the packWalkFn with updated arguments.
			if resolved.info.IsDir() {
				return filepath.Walk(resolved.absTarget, p.packWalkFn(root, resolved.target, path, tarW, meta, ignoreRules))
			}

			// Dereference this symlink by updating the header with the target file
			// details and set writeBody to true so the body will be written.
			header.Typeflag = tar.TypeReg
			header.ModTime = resolved.info.ModTime()
			header.Mode = int64(resolved.info.Mode().Perm())
			header.Size = resolved.info.Size()
			writeBody = true

		default:
			return fmt.Errorf("unexpected file mode %v", fm)
		}

		// Write the header first to the archive.
		if err := tarW.WriteHeader(header); err != nil {
			return fmt.Errorf("failed writing archive header for file %q: %w", path, err)
		}

		// Account for the file in the list.
		meta.Files = append(meta.Files, header.Name)

		// Skip writing file data for certain file types (above).
		if !writeBody {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed opening file %q for archiving: %w", path, err)
		}
		defer f.Close()

		size, err := io.Copy(tarW, f)
		if err != nil {
			return fmt.Errorf("failed copying file %q to archive: %w", path, err)
		}

		// Add the size we copied to the body.
		meta.Size += size

		return nil
	}
}

func (p *Packer) resolveLink(root string, path string, header *tar.Header) (*resolvedSymlink, error) {
	if header == nil {
		return nil, errors.New("no tar header was specified for this link")
	}

	// Read the symlink file to find the destination.
	target, err := os.Readlink(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read symlink %q: %w", path, err)
	}

	// Get the absolute path of the symlink target.
	absTarget := target
	if !filepath.IsAbs(absTarget) {
		absTarget = filepath.Join(filepath.Dir(path), target)
	}
	if !filepath.IsAbs(absTarget) {
		absTarget = filepath.Join(root, absTarget)
	}

	// Get the file info for the target.
	info, err := os.Lstat(absTarget)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info from file %q: %w", target, err)
	}

	// Recurse if the symlink resolves to another symlink
	if info.Mode()&os.ModeSymlink != 0 {
		return p.resolveLink(root, absTarget, header)
	}

	resolved := &resolvedSymlink{
		absTarget: absTarget,
		target:    target,
		info:      info,
	}

	return resolved, err
}

// Unpack is used to read and extract the contents of a slug to the dst
// directory. Symlinks within the slug are supported, provided their targets
// are relative and point to paths within the destination directory.
func Unpack(r io.Reader, dst string) error {
	p := &Packer{}
	return p.Unpack(r, dst)
}

// Unpack unpacks the archive data in r into directory dst.
func (p *Packer) Unpack(r io.Reader, dst string) error {
	// Decompress as we read.
	uncompressed, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to uncompress slug: %w", err)
	}

	// Untar as we read.
	untar := tar.NewReader(uncompressed)

	// Unpackage all the contents into the directory.
	for {
		header, err := untar.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to untar slug: %w", err)
		}

		// Get rid of absolute paths.
		path := header.Name
		if path[0] == '/' {
			path = path[1:]
		}
		path = filepath.Join(dst, path)

		// Check for paths outside our directory, they are forbidden
		target := filepath.Clean(path)
		if !strings.HasPrefix(target, dst) {
			return &IllegalSlugError{
				Err: fmt.Errorf("invalid filename, traversal with \"..\" outside of current directory"),
			}
		}

		// Ensure the destination is not through any symlinks. This prevents
		// any files from being deployed through symlinks defined in the slug.
		// There are malicious cases where this could be used to escape the
		// slug's boundaries (zipslip), and any legitimate use is questionable
		// and likely indicates a hand-crafted tar file, which we are not in
		// the business of supporting here.
		//
		// The strategy is to Lstat each path  component from dst up to the
		// immediate parent directory of the file name in the tarball, checking
		// the mode on each to ensure we wouldn't be passing through any
		// symlinks.
		currentPath := dst // Start at the root of the unpacked tarball.
		components := strings.Split(header.Name, "/")

		for i := 0; i < len(components)-1; i++ {
			currentPath = filepath.Join(currentPath, components[i])
			fi, err := os.Lstat(currentPath)
			if os.IsNotExist(err) {
				// Parent directory structure is incomplete. Technically this
				// means from here upward cannot be a symlink, so we cancel the
				// remaining path tests.
				break
			}
			if err != nil {
				return fmt.Errorf("failed to evaluate path %q: %w", header.Name, err)
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				return &IllegalSlugError{
					Err: fmt.Errorf("cannot extract %q through symlink", header.Name),
				}
			}
		}

		// Make the directories to the path.
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", dir, err)
		}

		// Handle symlinks.
		if header.Typeflag == tar.TypeSymlink {
			err := p.checkSymlink(dst, header.Name, header.Linkname)
			if err != nil {
				return err
			}

			// Create the symlink.
			if err := os.Symlink(header.Linkname, path); err != nil {
				return fmt.Errorf("failed creating symlink (%q -> %q): %w",
					header.Name, header.Linkname, err)
			}

			continue
		}

		// Only unpack regular files from this point on.
		if header.Typeflag == tar.TypeDir || header.Typeflag == tar.TypeXGlobalHeader || header.Typeflag == tar.TypeXHeader {
			continue
		} else if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("failed creating %q: unsupported type %c", path,
				header.Typeflag)
		}

		// Open a handle to the destination.
		fh, err := os.Create(path)
		if err != nil {
			// This mimics tar's behavior wrt the tar file containing duplicate files
			// and it allowing later ones to clobber earlier ones even if the file
			// has perms that don't allow overwriting.
			if os.IsPermission(err) {
				os.Chmod(path, 0600)
				fh, err = os.Create(path)
			}

			if err != nil {
				return fmt.Errorf("failed creating file %q: %w", path, err)
			}
		}

		// Copy the contents.
		_, err = io.Copy(fh, untar)
		fh.Close()
		if err != nil {
			return fmt.Errorf("failed to copy slug file %q: %w", path, err)
		}

		// Restore the file mode. We have to do this after writing the file,
		// since it is possible we have a read-only mode.
		mode := header.FileInfo().Mode()
		if err := os.Chmod(path, mode); err != nil {
			return fmt.Errorf("failed setting permissions on %q: %w", path, err)
		}
	}
	return nil
}

// Given a "root" directory, the path to a symlink within said root, and the
// target of said symlink, checkSymlink checks that the target either falls
// into root somewhere, or is explicitly allowed per the Packer's config.
func (p *Packer) checkSymlink(root, path, target string) error {
	// Get the absolute path to root.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("failed making path %q absolute: %w", root, err)
	}

	// Get the absolute path to the file path.
	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(absRoot, path)
	}

	// Get the absolute path of the symlink target.
	var absTarget string
	if filepath.IsAbs(target) {
		absTarget = filepath.Clean(target)
	} else {
		absTarget = filepath.Join(filepath.Dir(absPath), target)
	}

	// Target falls within root.
	if strings.HasPrefix(absTarget, absRoot) {
		return nil
	}

	// The link target is outside of root. Check if it is allowed.
	for _, prefix := range p.allowSymlinkTargets {
		// Ensure prefix is absolute.
		if !filepath.IsAbs(prefix) {
			prefix = filepath.Join(absRoot, prefix)
		}

		// Exact match is allowed.
		if absTarget == prefix {
			return nil
		}

		// Prefix match of a directory is allowed.
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		if strings.HasPrefix(absTarget, prefix) {
			return nil
		}
	}

	return &IllegalSlugError{
		Err: fmt.Errorf(
			"invalid symlink (%q -> %q) has external target",
			path, target,
		),
	}
}

// checkFileMode is used to examine an os.FileMode and determine if it should
// be included in the archive, and if it has a data body which needs writing.
func checkFileMode(m os.FileMode) (keep, body bool) {
	switch {
	case m.IsDir():
		return true, false

	case m.IsRegular():
		return true, true

	case m&os.ModeSymlink != 0:
		return true, false
	}

	return false, false
}
