// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/hashicorp/go-slug"
	"github.com/hashicorp/go-slug/sourceaddrs"
	regaddr "github.com/hashicorp/terraform-registry-address"
)

const manifestFilename = "terraform-sources.json"

type Bundle struct {
	rootDir string

	manifestChecksum string

	remotePackageDirs map[sourceaddrs.RemotePackage]string
	remotePackageMeta map[sourceaddrs.RemotePackage]*PackageMeta

	registryPackageSources map[regaddr.ModulePackage]map[versions.Version]sourceaddrs.RemoteSource
}

// OpenDir opens a bundle rooted at the given base directory.
//
// If OpenDir succeeds then nothing else (inside or outside the calling program)
// may modify anything under the given base directory for the lifetime of
// the returned [Bundle] object. If the bundle directory is modified while the
// object is still alive then behavior is undefined.
func OpenDir(baseDir string) (*Bundle, error) {
	// We'll take the absolute form of the directory to be resilient in case
	// something else in this program rudely changes the current working
	// directory while the bundle is still alive.
	rootDir, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve base directory: %w", err)
	}

	ret := &Bundle{
		rootDir:                rootDir,
		remotePackageDirs:      make(map[sourceaddrs.RemotePackage]string),
		remotePackageMeta:      make(map[sourceaddrs.RemotePackage]*PackageMeta),
		registryPackageSources: make(map[regaddr.ModulePackage]map[versions.Version]sourceaddrs.RemoteSource),
	}

	manifestSrc, err := os.ReadFile(filepath.Join(rootDir, manifestFilename))
	if err != nil {
		return nil, fmt.Errorf("cannot read manifest: %w", err)
	}

	hash := sha256.New()
	ret.manifestChecksum = hex.EncodeToString(hash.Sum(manifestSrc))

	var manifest manifestRoot
	err = json.Unmarshal(manifestSrc, &manifest)
	if err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	if manifest.FormatVersion != 1 {
		return nil, fmt.Errorf("invalid manifest: unsupported format version %d", manifest.FormatVersion)
	}

	for _, rpm := range manifest.Packages {
		// We'll be quite fussy about the local directory name to avoid a
		// crafted manifest sending us to other random places in the filesystem.
		// It must be just a single directory name, without any path separators
		// or any traversals.
		localDir := filepath.ToSlash(rpm.LocalDir)
		if !fs.ValidPath(localDir) || localDir == "." || strings.IndexByte(localDir, '/') >= 0 {
			return nil, fmt.Errorf("invalid package directory name %q", rpm.LocalDir)
		}

		pkgAddr, err := sourceaddrs.ParseRemotePackage(rpm.SourceAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid remote package address %q: %w", rpm.SourceAddr, err)
		}
		ret.remotePackageDirs[pkgAddr] = localDir

		if rpm.Meta.GitCommitID != "" {
			ret.remotePackageMeta[pkgAddr] = PackageMetaWithGitCommit(rpm.Meta.GitCommitID)
		}
	}

	for _, rpm := range manifest.RegistryMeta {
		pkgAddr, err := sourceaddrs.ParseRegistryPackage(rpm.SourceAddr)
		if err != nil {
			return nil, fmt.Errorf("invalid registry package address %q: %w", rpm.SourceAddr, err)
		}
		vs := ret.registryPackageSources[pkgAddr]
		if vs == nil {
			vs = make(map[versions.Version]sourceaddrs.RemoteSource)
			ret.registryPackageSources[pkgAddr] = vs
		}
		for versionStr, mv := range rpm.Versions {
			version, err := versions.ParseVersion(versionStr)
			if err != nil {
				return nil, fmt.Errorf("invalid registry package version %q: %w", versionStr, err)
			}
			sourceAddr, err := sourceaddrs.ParseRemoteSource(mv.SourceAddr)
			if err != nil {
				return nil, fmt.Errorf("invalid registry package source address %q: %w", mv.SourceAddr, err)
			}
			vs[version] = sourceAddr
		}
	}

	return ret, nil
}

// LocalPathForSource takes either a remote or registry final source address
// and returns the local path within the bundle that corresponds with it.
//
// It doesn't make sense to pass a [sourceaddrs.LocalSource] to this function
// because a source bundle cannot contain anything other than remote packages,
// but as a concession to convenience this function will return a
// filepath-shaped relative path in that case, assuming that the source was
// intended to be a local filesystem path relative to the current working
// directory. The result will therefore not necessarily be a subdirectory of
// the recieving bundle in that case.
func (b *Bundle) LocalPathForSource(addr sourceaddrs.FinalSource) (string, error) {
	switch addr := addr.(type) {
	case sourceaddrs.RemoteSource:
		return b.LocalPathForRemoteSource(addr)
	case sourceaddrs.RegistrySourceFinal:
		return b.LocalPathForRegistrySource(addr.Unversioned(), addr.SelectedVersion())
	case sourceaddrs.LocalSource:
		return filepath.FromSlash(addr.RelativePath()), nil
	default:
		// If we get here then it's probably a bug: the above cases should be
		// exhaustive for all sourceaddrs.FinalSource implementations.
		return "", fmt.Errorf("cannot produce local path for source address of type %T", addr)
	}
}

// LocalPathForRemoteSource returns the local path within the bundle that
// corresponds with the given source address, or an error if the source address
// is within a source package not included in the bundle.
func (b *Bundle) LocalPathForRemoteSource(addr sourceaddrs.RemoteSource) (string, error) {
	pkgAddr := addr.Package()
	localName, ok := b.remotePackageDirs[pkgAddr]
	if !ok {
		return "", fmt.Errorf("source bundle does not include %s", pkgAddr)
	}
	subPath := filepath.FromSlash(addr.SubPath())
	return filepath.Join(b.rootDir, localName, subPath), nil
}

// LocalPathForRegistrySource returns the local path within the bundle that
// corresponds with the given registry address and version, or an error if the
// source address is within a source package not included in the bundle.
func (b *Bundle) LocalPathForRegistrySource(addr sourceaddrs.RegistrySource, version versions.Version) (string, error) {
	pkgAddr := addr.Package()
	vs, ok := b.registryPackageSources[pkgAddr]
	if !ok {
		return "", fmt.Errorf("source bundle does not include %s", pkgAddr)
	}
	baseSourceAddr, ok := vs[version]
	if !ok {
		return "", fmt.Errorf("source bundle does not include %s v%s", pkgAddr, version)
	}

	// The address we were given might have its own source address, so we need
	// to incorporate that into our result.
	finalSourceAddr := addr.FinalSourceAddr(baseSourceAddr)
	return b.LocalPathForRemoteSource(finalSourceAddr)
}

// LocalPathForFinalRegistrySource is a variant of
// [Bundle.LocalPathForRegistrySource] which passes the source address and
// selected version together as a single address value.
func (b *Bundle) LocalPathForFinalRegistrySource(addr sourceaddrs.RegistrySourceFinal) (string, error) {
	return b.LocalPathForRegistrySource(addr.Unversioned(), addr.SelectedVersion())
}

// SourceForLocalPath is the inverse of [Bundle.LocalPathForSource],
// translating a local path beneath the bundle's base directory back into
// a source address that it's a snapshot of.
//
// Returns an error if the given directory is not within the bundle's base
// directory, or is not within one of the subdirectories of the bundle
// that represents a source package. A caller using this to present more
// user-friendly file paths in error messages etc could reasonably choose
// to just retain the source string if this function returns an error, and
// not show the error to the user.
//
// The [Bundle] implementation is optimized for forward lookups from source
// address to local path rather than the other way around, so this function
// may be considerably more expensive than the forward lookup and is intended
// primarily for reporting friendly source locations in diagnostic messages
// instead of exposing the opaque internal directory names from the source
// bundle. This function should not typically be used in performance-sensitive
// portions of the happy path.
func (b *Bundle) SourceForLocalPath(p string) (sourceaddrs.FinalSource, error) {
	// This implementation is a best effort sort of thing, and might not
	// always succeed in awkward cases.

	// We'll start by making our path absolute because that'll make it
	// more comparable with b.rootDir, which is also absolute.
	absPath, err := filepath.Abs(p)
	if err != nil {
		return nil, fmt.Errorf("can't determine absolute path for %q: %w", p, err)
	}

	// Now we'll reinterpret the path as relative to our base directory,
	// so we can see what local directory name it starts with.
	relPath, err := filepath.Rel(b.rootDir, absPath)
	if err != nil {
		// If the path can't be made relative then that suggests it's on a
		// different volume, such as a different drive letter on Windows.
		return nil, fmt.Errorf("path %q does not belong to the source bundle", absPath)
	}

	// We'll do all of our remaining work in the abstract "forward-slash-path"
	// mode, matching how we represent "sub-paths" for our source addresses.
	subPath := path.Clean(filepath.ToSlash(relPath))
	if !fs.ValidPath(subPath) || subPath == "." {
		// If the path isn't "valid" by now then that suggests it's a
		// path outside of our source bundle which would appear as a
		// path with a ".." segment on the front, or to the root of
		// our source bundle which would appear as "." and isn't part
		// of any particular package.
		return nil, fmt.Errorf("path %q does not belong to the source bundle", absPath)
	}

	// If all of the above passed then we should now have one or more
	// slash-separated path segments. The first one should be one of the
	// local directories we know from our manifest, and then the rest is
	// the sub-path in the associated package.
	localDir, subPath, _ := strings.Cut(subPath, "/")

	// There can be potentially several packages all referring to the same
	// directory, so to make the result deterministic we'll just take the
	// one whose stringified source address is shortest.
	var pkgAddr sourceaddrs.RemotePackage
	found := false
	for candidateAddr, candidateDir := range b.remotePackageDirs {
		if candidateDir != localDir {
			continue
		}
		if found {
			// We've found multiple possible source addresses, so we
			// need to decide which one to keep.
			if len(candidateAddr.String()) > len(pkgAddr.String()) {
				continue
			}
		}
		pkgAddr = candidateAddr
		found = true
	}

	if !found {
		return nil, fmt.Errorf("path %q does not belong to the source bundle", absPath)
	}

	return pkgAddr.SourceAddr(subPath), nil
}

// ChecksumV1 returns a checksum of the contents of the source bundle that
// can be used to determine if another source bundle is equivalent to this one.
//
// "Equivalent" means that it contains all of the same source packages with
// identical content each.
//
// A successful result is a string with the prefix "h1:" to indicate that
// it was built with checksum algorithm version 1. Future versions may
// introduce other checksum formats.
func (b *Bundle) ChecksumV1() (string, error) {
	// Our first checksum format assumes that the checksum of the manifest
	// is sufficient to cover the entire archive, which in turn assumes that
	// the builder either directly or indirectly encodes the checksum of
	// each package into the manifest. For the initial implementation of
	// Builder we achieve that by using the checksum as the directory name
	// for each package, which avoids the need to redundantly store the
	// checksum again. If a future Builder implementation moves away from
	// using checksums as directory names then the builder will need to
	// introduce explicit checksums as a separate property into the manifest
	// in order to preserve our assumptions here.
	return "h1:" + b.manifestChecksum, nil
}

// RemotePackages returns a slice of all of the remote source packages that
// contributed to this source bundle.
//
// The result is sorted into a consistent but unspecified order.
func (b *Bundle) RemotePackages() []sourceaddrs.RemotePackage {
	ret := make([]sourceaddrs.RemotePackage, 0, len(b.remotePackageDirs))
	for pkgAddr := range b.remotePackageDirs {
		ret = append(ret, pkgAddr)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].String() < ret[j].String()
	})
	return ret
}

// RemotePackageMeta returns the package metadata for the given package address,
// or nil if there is no metadata for that package tracked in the bundle.
func (b *Bundle) RemotePackageMeta(pkgAddr sourceaddrs.RemotePackage) *PackageMeta {
	return b.remotePackageMeta[pkgAddr]
}

// RegistryPackages returns a list of all of the distinct registry packages
// that contributed to this bundle.
//
// The result is in a consistent but unspecified sorted order.
func (b *Bundle) RegistryPackages() []regaddr.ModulePackage {
	ret := make([]regaddr.ModulePackage, 0, len(b.remotePackageDirs))
	for pkgAddr := range b.registryPackageSources {
		ret = append(ret, pkgAddr)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].String() < ret[j].String()
	})
	return ret
}

// RegistryPackageVersions returns a list of all of the versions of the given
// module registry package that this bundle has package content for.
//
// This result can be used as a substitute for asking the remote registry which
// versions are available in any situation where a caller is interested only
// in what's bundled, and will not consider installing anything new from
// the origin registry.
//
// The result is guaranteed to be sorted with lower-precedence version numbers
// placed earlier in the list.
func (b *Bundle) RegistryPackageVersions(pkgAddr regaddr.ModulePackage) versions.List {
	vs := b.registryPackageSources[pkgAddr]
	if len(vs) == 0 {
		return nil
	}
	ret := make(versions.List, 0, len(vs))
	for v := range vs {
		ret = append(ret, v)
	}
	ret.Sort()
	return ret
}

// RegistryPackageSourceAddr returns the remote source address corresponding
// to the given version of the given module package, or sets its second return
// value to false if no such version is included in the bundle.
func (b *Bundle) RegistryPackageSourceAddr(pkgAddr regaddr.ModulePackage, version versions.Version) (sourceaddrs.RemoteSource, bool) {
	sourceAddr, ok := b.registryPackageSources[pkgAddr][version]
	return sourceAddr, ok
}

// WriteArchive writes a source bundle archive containing the same contents
// as the bundle to the given writer.
//
// A source bundle archive is a gzip-compressed tar stream that can then
// be extracted in some other location to produce an equivalent source
// bundle directory.
func (b *Bundle) WriteArchive(w io.Writer) error {
	// For this part we just delegate to the main slug packer, since a
	// source bundle archive is effectively just a slug with multiple packages
	// (and a manifest) inside it.
	packer, err := slug.NewPacker(slug.DereferenceSymlinks())
	if err != nil {
		return fmt.Errorf("can't instantiate archive packer: %w", err)
	}
	_, err = packer.Pack(b.rootDir, w)
	return err
}

// ExtractArchive reads a source bundle archive from the given reader and
// extracts it into the given target directory, which must already exist and
// must be empty.
//
// If successful, it returns a [Bundle] value representing the created bundle,
// as if the given target directory were passed to [OpenDir].
func ExtractArchive(r io.Reader, targetDir string) (*Bundle, error) {
	// A bundle archive is just a slug archive created over a bundle
	// directory, so we can use the normal unpack function to deal with it.
	err := slug.Unpack(r, targetDir)
	if err != nil {
		return nil, err
	}
	return OpenDir(targetDir)
}
