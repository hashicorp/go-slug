// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/apparentlymart/go-versions/versions"
	regaddr "github.com/hashicorp/terraform-registry-address"
	"golang.org/x/mod/sumdb/dirhash"

	"github.com/hashicorp/go-slug/internal/ignorefiles"
	"github.com/hashicorp/go-slug/sourceaddrs"
)

// Builder deals with the process of gathering source code
type Builder struct {
	// targetDir is the base directory of the source bundle we're writing
	// into.
	targetDir string

	// fetcher is the package fetching callback we'll use to fetch remote
	// packages into subdirectories of the bundle directory.
	fetcher PackageFetcher

	// registryClient is the module registry client we'll use to resolve
	// any module registry sources into their underlying remote package
	// addresses which we can then fetch using "fetcher".
	registryClient RegistryClient

	// pendingRemote is an unordered set of remote artifacts that we've
	// discovered we need to analyze but have not yet done so.
	pendingRemote []remoteArtifact

	// analyzed is a set of remote artifacts that we've already analyzed and
	// thus already found the dependencies of.
	analyzed map[remoteArtifact]struct{}

	// remotePackageDirs tracks the local directory name for each remote
	// package we've already fetched. The keys of this map also serve as our
	// memory of which packages we've already fetched and therefore don't need
	// to fetch again if we find more source addresses in those packages.
	//
	// In our current implementation thse directory names are always checksums
	// of the content of the package, and we rely on that when building a
	// manifest file so if a future update changes the directory naming scheme
	// then we'll need a different solution for tracking the checksums for
	// use in the manifest file. For external callers the local directory
	// naming scheme is always an implementation detail that they may not
	// rely on.
	remotePackageDirs map[sourceaddrs.RemotePackage]string

	// remotePackageMeta tracks the package metadata of each remote package
	// we've fetched so far. This does not include any packages for which
	// the fetcher returned no metadata.
	remotePackageMeta map[sourceaddrs.RemotePackage]*PackageMeta

	// pendingRegistry is an unordered set of registry artifacts that need to
	// be translated into remote artifacts before further processing.
	pendingRegistry []registryArtifact

	// resolvedRegistry tracks the underlying remote source address for each
	// selected version of each module registry package.
	resolvedRegistry map[registryPackageVersion]sourceaddrs.RemoteSource

	// registryPackageVersions caches responses from module registry calls to
	// look up the available versions for a particular module package. Although
	// these could potentially change while we're running, we assume that the
	// lifetime of a particular Builder is short enough for that not to
	// matter.
	registryPackageVersions map[regaddr.ModulePackage]versions.List

	mu sync.Mutex
}

// NewBuilder creates a new builder that will construct a source bundle in the
// given target directory, which must already exist and be empty before any
// work begins.
//
// During the lifetime of a builder the target directory must not be modified
// or moved by anything other than the builder, including other concurrent
// processes running on the system. The target directory is not a valid source
// bundle until a call to [Builder.Close] returns successfully; the directory
// may be apepar in an inconsistent state while the builder is working.
func NewBuilder(targetDir string, fetcher PackageFetcher, registryClient RegistryClient) (*Builder, error) {
	// We'll lock in our absolute path here just in case someone changes the
	// process working directory out from under us for some reason.
	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("invalid target directory: %w", err)
	}
	return &Builder{
		targetDir:               absDir,
		fetcher:                 fetcher,
		registryClient:          registryClient,
		analyzed:                make(map[remoteArtifact]struct{}),
		remotePackageDirs:       make(map[sourceaddrs.RemotePackage]string),
		remotePackageMeta:       make(map[sourceaddrs.RemotePackage]*PackageMeta),
		resolvedRegistry:        make(map[registryPackageVersion]sourceaddrs.RemoteSource),
		registryPackageVersions: make(map[regaddr.ModulePackage]versions.List),
	}, nil
}

// AddRemoteSource incorporates the package containing the given remote source
// into the bundle, and then analyzes the source artifact for dependencies
// using the given dependency finder.
//
// If the returned diagnostics contains errors then the bundle is left in an
// inconsistent state and must not be used for any other calls.
func (b *Builder) AddRemoteSource(ctx context.Context, addr sourceaddrs.RemoteSource, depFinder DependencyFinder) Diagnostics {
	if b.targetDir == "" {
		// The builder has been closed, so cannot be modified further.
		// This is always a bug in the caller, which should discard a builder
		// as soon as it's been closed.
		panic("AddRemoteSource on closed sourcebundle.Builder")
	}

	af := remoteArtifact{addr, depFinder}
	b.mu.Lock()
	if _, exists := b.analyzed[af]; exists {
		// Nothing further to do with this one, then.
		// NOTE: This early check is just an optimization; b.resolvePending
		// will re-check whether each queued item has already been analyzed
		// anyway, so this just avoids growing b.pendingRemote if possible,
		// since once something has become analyzed it never becomes
		// "un-analyzed" again.
		b.mu.Unlock()
		return nil
	}
	b.pendingRemote = append(b.pendingRemote, af)
	b.mu.Unlock()

	return b.resolvePending(ctx)
}

// AddRegistrySource incorporates the registry metadata for the given address
// and the package associated with the latest version in allowedVersions
// into the bundle, and then analyzes the new artifact for dependencies
// using the given dependency finder.
//
// If you have already selected a specific version to install, consider using
// [Builder.AddFinalRegistrySource] instead.
//
// If the returned diagnostics contains errors then the bundle is left in an
// inconsistent state and must not be used for any other calls.
func (b *Builder) AddRegistrySource(ctx context.Context, addr sourceaddrs.RegistrySource, allowedVersions versions.Set, depFinder DependencyFinder) Diagnostics {
	if b.targetDir == "" {
		// The builder has been closed, so cannot be modified further.
		// This is always a bug in the caller, which should discard a builder
		// as soon as it's been closed.
		panic("AddRegistrySource on closed sourcebundle.Builder")
	}

	b.mu.Lock()
	b.pendingRegistry = append(b.pendingRegistry, registryArtifact{addr, allowedVersions, depFinder})
	b.mu.Unlock()

	return b.resolvePending(ctx)
}

// AddFinalRegistrySource is a variant of [Builder.AddRegistrySource] which
// takes an already-selected version of a registry source, instead of taking
// a version constraint and then selecting the latest available version
// matching that constraint.
//
// This function still asks the registry for its set of available versions for
// the unversioned package first, to ensure that the results from installing
// from a final source will always be consistent with those from installing
// from a not-yet-resolved registry source.
func (b *Builder) AddFinalRegistrySource(ctx context.Context, addr sourceaddrs.RegistrySourceFinal, depFinder DependencyFinder) Diagnostics {
	// We handle this just by turning the version selection into an exact
	// version set and then installing from that as normal.
	allowedVersions := versions.Only(addr.SelectedVersion())
	return b.AddRegistrySource(ctx, addr.Unversioned(), allowedVersions, depFinder)
}

// Close ensures that the target directory is in a valid and consistent state
// to be used as a source bundle and then returns an object providing the
// read-only API for that bundle.
//
// After calling Close the receiving builder becomes invalid and must not be
// used any further.
func (b *Builder) Close() (*Bundle, error) {
	b.mu.Lock()
	if b.targetDir == "" {
		b.mu.Unlock()
		panic("Close on already-closed sourcebundle.Builder")
	}
	baseDir := b.targetDir
	b.targetDir = "" // makes the Add... methods panic when called, to avoid mutating the finalized bundle
	b.mu.Unlock()

	// We need to freeze all of the metadata we've been tracking into the
	// manifest file so that OpenDir can discover equivalent metadata itself
	// when opening the finalized bundle.
	err := b.writeManifest(filepath.Join(baseDir, manifestFilename))
	if err != nil {
		return nil, fmt.Errorf("failed to generate source bundle manifest: %w", err)
	}

	ret, err := OpenDir(baseDir)
	if err != nil {
		// If we get here then it suggests that we've left the bundle directory
		// in an inconsistent state which therefore made OpenDir fail its
		// early checks.
		return nil, fmt.Errorf("failed to open bundle after Close: %w", err)
	}
	return ret, nil
}

// resolvePending depletes the queues of pending source artifacts, making sure
// that everything required is present in the bundle directory, both directly
// and indirectly.
func (b *Builder) resolvePending(ctx context.Context) (diags Diagnostics) {
	b.mu.Lock()
	defer func() {
		// If anything we do here generates any errors then the bundle
		// directory is in an inconsistent state and must not be used
		// any further. This will make all subsequent calls panic.
		if diags.HasErrors() {
			b.targetDir = ""
		}

		b.mu.Unlock()
	}()

	trace := buildTraceFromContext(ctx)

	// We'll just keep iterating until we've depleted our queues.
	// Note that the order of operations isn't actually important here and
	// so we're consuming the "queues" in LIFO order instead of FIFO order,
	// since that is easier to model using a Go slice.
	for len(b.pendingRemote) > 0 || len(b.pendingRegistry) > 0 {
		// We'll consume items from the "registry" queue first because resolving
		// this will contribute additional items to the "remote" queue.
		for len(b.pendingRegistry) > 0 {
			next, remain := b.pendingRegistry[len(b.pendingRegistry)-1], b.pendingRegistry[:len(b.pendingRegistry)-1]
			b.pendingRegistry = remain

			realSource, err := b.findRegistryPackageSource(ctx, next.sourceAddr, next.versions)
			if err != nil {
				diags = diags.Append(&internalDiagnostic{
					severity: DiagError,
					summary:  "Cannot resolve module registry package",
					detail:   fmt.Sprintf("Error resolving module registry source %s: %s.", next.sourceAddr, err),
				})
				continue
			}

			b.pendingRemote = append(b.pendingRemote, remoteArtifact{
				sourceAddr: realSource,
				depFinder:  next.depFinder,
			})
		}

		// Now we'll consume items from the "remote" queue, which might have
		// grown as a result of resolving some registry queue items.
		for len(b.pendingRemote) > 0 {
			next, remain := b.pendingRemote[len(b.pendingRemote)-1], b.pendingRemote[:len(b.pendingRemote)-1]
			b.pendingRemote = remain

			pkgAddr := next.sourceAddr.Package()
			pkgLocalDir, err := b.ensureRemotePackage(ctx, pkgAddr)
			if err != nil {
				diags = diags.Append(&internalDiagnostic{
					severity: DiagError,
					summary:  "Cannot install source package",
					detail:   fmt.Sprintf("Error installing %s: %s.", next.sourceAddr.Package(), err),
				})
				continue
			}

			// localDirPath now refers to the local equivalent of whatever
			// sub-path or sub-file the source address referred to, so we
			// can ask the dependency finder to analyze it and possibly
			// contribute more items to our queues.
			artifact := remoteArtifact{
				sourceAddr: next.sourceAddr,
				depFinder:  next.depFinder,
			}
			if _, exists := b.analyzed[artifact]; !exists {
				fsys := os.DirFS(filepath.Join(b.targetDir, pkgLocalDir))
				subPath := next.sourceAddr.SubPath()
				depFinder := next.depFinder

				deps := Dependencies{
					baseAddr: next.sourceAddr,

					remoteCb: func(source sourceaddrs.RemoteSource, depFinder DependencyFinder) {
						b.pendingRemote = append(b.pendingRemote, remoteArtifact{
							sourceAddr: source,
							depFinder:  depFinder,
						})
					},
					registryCb: func(source sourceaddrs.RegistrySource, allowedVersions versions.Set, depFinder DependencyFinder) {
						b.pendingRegistry = append(b.pendingRegistry, registryArtifact{
							sourceAddr: source,
							versions:   allowedVersions,
							depFinder:  depFinder,
						})
					},
					localResolveErrCb: func(err error) {
						diags = diags.Append(&internalDiagnostic{
							severity: DiagError,
							summary:  "Invalid relative source address",
							detail:   fmt.Sprintf("Invalid relative path from %s: %s.", next.sourceAddr, err),
						})
					},
				}
				moreDiags := depFinder.FindDependencies(fsys, subPath, &deps)
				deps.disable()
				b.analyzed[artifact] = struct{}{}
				if len(moreDiags) != 0 {
					moreDiags = moreDiags.inRemoteSourcePackage(pkgAddr)
					if cb := trace.Diagnostics; cb != nil {
						cb(ctx, moreDiags)
					}
				}
				diags = diags.Append(moreDiags)
				if diags.HasErrors() {
					continue
				}
			}
		}
	}

	return diags
}

func (b *Builder) findRegistryPackageSource(ctx context.Context, sourceAddr sourceaddrs.RegistrySource, allowedVersions versions.Set) (sourceaddrs.RemoteSource, error) {
	// NOTE: This expects to be called while b.mu is already locked.

	trace := buildTraceFromContext(ctx)

	pkgAddr := sourceAddr.Package()
	availableVersions, ok := b.registryPackageVersions[pkgAddr]
	if !ok {
		var reqCtx context.Context
		if cb := trace.RegistryPackageVersionsStart; cb != nil {
			reqCtx = cb(ctx, pkgAddr)
		}
		if reqCtx == nil {
			reqCtx = ctx
		}

		resp, err := b.registryClient.ModulePackageVersions(reqCtx, pkgAddr)
		if err != nil {
			if cb := trace.RegistryPackageVersionsFailure; cb != nil {
				cb(reqCtx, pkgAddr, err)
			}
			return sourceaddrs.RemoteSource{}, fmt.Errorf("failed to query available versions for %s: %w", pkgAddr, err)
		}
		vs := resp.Versions
		vs.Sort()
		availableVersions = vs
		b.registryPackageVersions[pkgAddr] = availableVersions
		if cb := trace.RegistryPackageVersionsSuccess; cb != nil {
			cb(reqCtx, pkgAddr, availableVersions)
		}
	} else {
		if cb := trace.RegistryPackageVersionsAlready; cb != nil {
			cb(ctx, pkgAddr, availableVersions)
		}
	}

	selectedVersion := availableVersions.NewestInSet(allowedVersions)
	if selectedVersion == versions.Unspecified {
		return sourceaddrs.RemoteSource{}, fmt.Errorf("no available version of %s matches the specified version constraint", pkgAddr)
	}

	pkgVer := registryPackageVersion{
		pkg:     pkgAddr,
		version: selectedVersion,
	}
	realSourceAddr, ok := b.resolvedRegistry[pkgVer]
	if !ok {
		var reqCtx context.Context
		if cb := trace.RegistryPackageSourceStart; cb != nil {
			reqCtx = cb(ctx, pkgAddr, selectedVersion)
		}
		if reqCtx == nil {
			reqCtx = ctx
		}

		resp, err := b.registryClient.ModulePackageSourceAddr(reqCtx, pkgAddr, selectedVersion)
		if err != nil {
			if cb := trace.RegistryPackageSourceFailure; cb != nil {
				cb(reqCtx, pkgAddr, selectedVersion, err)
			}
			return sourceaddrs.RemoteSource{}, fmt.Errorf("failed to find real source address for %s %s: %w", pkgAddr, selectedVersion, err)
		}
		realSourceAddr = resp.SourceAddr
		b.resolvedRegistry[pkgVer] = realSourceAddr
		if cb := trace.RegistryPackageSourceSuccess; cb != nil {
			cb(reqCtx, pkgAddr, selectedVersion, realSourceAddr)
		}
	} else {
		if cb := trace.RegistryPackageSourceAlready; cb != nil {
			cb(ctx, pkgAddr, selectedVersion, realSourceAddr)
		}
	}

	// If our original source address had its own sub-path component then we
	// need to combine that with the one in realSourceAddr to get the correct
	// final path: the sourceAddr subpath is relative to the realSourceAddr
	// subpath.
	realSourceAddr = sourceAddr.FinalSourceAddr(realSourceAddr)

	return realSourceAddr, nil
}

func (b *Builder) ensureRemotePackage(ctx context.Context, pkgAddr sourceaddrs.RemotePackage) (localDir string, err error) {
	// NOTE: This expects to be called while b.mu is already locked.

	trace := buildTraceFromContext(ctx)

	existingDir, ok := b.remotePackageDirs[pkgAddr]
	if ok {
		// We already have this package, so there's nothing more to do.
		if cb := trace.RemotePackageDownloadAlready; cb != nil {
			cb(ctx, pkgAddr)
		}
		return existingDir, nil
	}

	var reqCtx context.Context
	if cb := trace.RemotePackageDownloadStart; cb != nil {
		reqCtx = cb(ctx, pkgAddr)
	}
	if reqCtx == nil {
		reqCtx = ctx
	}
	defer func() {
		if err == nil {
			if cb := trace.RemotePackageDownloadSuccess; cb != nil {
				cb(reqCtx, pkgAddr)
			}
		} else {
			if cb := trace.RemotePackageDownloadFailure; cb != nil {
				cb(reqCtx, pkgAddr, err)
			}
		}
	}()

	// We'll eventually name our local directory after a checksum of its
	// content, but we don't know its content yet so we'll use a temporary
	// name while we work on getting it populated.
	workDir, err := ioutil.TempDir(b.targetDir, ".tmp-")
	if err != nil {
		return "", fmt.Errorf("failed to create new package directory: %w", err)
	}

	response, err := b.fetcher.FetchSourcePackage(reqCtx, pkgAddr.SourceType(), pkgAddr.URL(), workDir)
	if err != nil {
		return "", fmt.Errorf("failed to fetch package: %w", err)
	}
	if response.PackageMeta != nil {
		// We'll remember the meta so we can use it when building a manifest later.
		b.remotePackageMeta[pkgAddr] = response.PackageMeta
	}

	// If the package has a .terraformignore file then we now need to remove
	// everything that we've been instructed to ignore.
	ignoreRules, err := ignorefiles.LoadPackageIgnoreRules(workDir)
	if err != nil {
		return "", fmt.Errorf("invalid .terraformignore file: %w", err)
	}

	// NOTE: The checks in packagePrepareWalkFn are safe only if we are sure
	// that no other process is concurrently modifying our temporary directory.
	// Source bundle building should only occur on hosts that are trusted by
	// whoever will ultimately be using the generated bundle.
	err = filepath.Walk(workDir, packagePrepareWalkFn(workDir, ignoreRules))
	if err != nil {
		return "", fmt.Errorf("failed to prepare package directory: %#w", err)
	}

	// If we got here then our tmpDir contains the final source code of a valid
	// module package. We'll compute a hash of its contents so we can notice
	// if it is identical to some other package we already installed, and then
	// if not rename it into its final directory name.
	// For this purpose we reuse the same directory tree hashing scheme that
	// Go uses for its own modules, although that's an implementation detail
	// subject to change in future versions: callers should always resolve
	// paths through the source bundle's manifest rather than assuming a path.
	//
	// FIXME: We should implement our own thing similar to Go's dirhash but
	// which can preserve file metadata at least to the level of detail that
	// Git can, so that we can e.g. avoid coalescing two packages that differ
	// only in whether a particular file is executable, or similar.
	//
	// We do currently _internally_ rely on the temporary directory being a
	// hash when we build the final manifest for the bundle, so if you change
	// this naming scheme you'll need to devise a new way for the manifest
	// to learn about the checksum. External callers are forbidden from relying
	// on it though, so you only have to worry about making the internals of
	// this package self-consistent in how they deal with naming and hashes.
	hash, err := dirhash.HashDir(workDir, "", dirhash.Hash1)
	if err != nil {
		return "", fmt.Errorf("failed to calculate package checksum: %w", err)
	}
	dirName := strings.TrimPrefix(hash, "h1:")

	// dirhash produces standard base64 encoding, but we need URL-friendly
	// base64 encoding since we're using these as filenames.
	rawChecksum, err := base64.StdEncoding.DecodeString(dirName)
	if err != nil {
		// Should not get here
		return "", fmt.Errorf("package has invalid checksum: %w", err)
	}
	dirName = base64.RawURLEncoding.EncodeToString(rawChecksum)

	b.remotePackageDirs[pkgAddr] = dirName

	// We might already have a directory with the same hash if we have two
	// different package addresses that happen to return the same source code.
	// For example, this could happen if one Git source leaves ref unspecified
	// and another explicitly specifies the main branch, therefore causing us
	// to fetch the same source code in two different ways. If a directory
	// already exists then we'll assume that it's suitable for this package
	// and discard the temporary directory we've been working on here, thereby
	// making the final bundle smaller.
	finalDir := filepath.Join(b.targetDir, dirName)
	if info, err := os.Lstat(finalDir); err == nil && info.IsDir() {
		err := os.RemoveAll(workDir)
		if err != nil {
			return "", fmt.Errorf("failed to clean temporary directory: %w", err)
		}
		return dirName, nil
	}

	// If a directory isn't already present then we'll now rename our
	// temporary directory to its final name.
	err = os.Rename(workDir, finalDir)
	if err != nil {
		return "", fmt.Errorf("failed to place final package directory: %w", err)
	}

	return dirName, nil
}

func (b *Builder) writeManifest(filename string) error {
	var root manifestRoot
	root.FormatVersion = 1

	for pkgAddr, localDirName := range b.remotePackageDirs {
		pkgMeta := b.remotePackageMeta[pkgAddr]

		manifestPkg := manifestRemotePackage{
			SourceAddr: pkgAddr.String(),
			LocalDir:   localDirName,
		}
		if pkgMeta != nil && pkgMeta.gitCommitID != "" {
			manifestPkg.Meta.GitCommitID = pkgMeta.gitCommitID
		}

		root.Packages = append(root.Packages, manifestPkg)
	}
	sort.Slice(root.Packages, func(i, j int) bool {
		return root.Packages[i].SourceAddr < root.Packages[j].SourceAddr
	})

	registryObjs := make(map[regaddr.ModulePackage]*manifestRegistryMeta)
	for rpv, sourceAddr := range b.resolvedRegistry {
		manifestMeta, ok := registryObjs[rpv.pkg]
		if !ok {
			root.RegistryMeta = append(root.RegistryMeta, manifestRegistryMeta{
				SourceAddr: rpv.pkg.String(),
				Versions:   make(map[string]manifestRegistryVersion),
			})
			manifestMeta = &root.RegistryMeta[len(root.RegistryMeta)-1]
			registryObjs[rpv.pkg] = manifestMeta
		}
		manifestMeta.Versions[rpv.version.String()] = manifestRegistryVersion{
			SourceAddr: sourceAddr.String(),
		}
	}
	sort.Slice(root.RegistryMeta, func(i, j int) bool {
		return root.Packages[i].SourceAddr < root.Packages[j].SourceAddr
	})

	buf, err := json.MarshalIndent(&root, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize to JSON: %#w", err)
	}
	err = os.WriteFile(filename, buf, 0664)
	if err != nil {
		return fmt.Errorf("failed to write file: %#w", err)
	}

	return nil
}

type remoteArtifact struct {
	sourceAddr sourceaddrs.RemoteSource
	depFinder  DependencyFinder
}

type registryArtifact struct {
	sourceAddr sourceaddrs.RegistrySource
	versions   versions.Set
	depFinder  DependencyFinder
}

type registryPackageVersion struct {
	pkg     regaddr.ModulePackage
	version versions.Version
}

func packagePrepareWalkFn(root string, ignoreRules *ignorefiles.Ruleset) filepath.WalkFunc {
	return func(absPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Get the relative path from the current src directory.
		relPath, err := filepath.Rel(root, absPath)
		if err != nil {
			return fmt.Errorf("failed to get relative path for file %q: %w", absPath, err)
		}
		if relPath == "." {
			return nil
		}

		ignored, err := ignoreRules.Excludes(relPath)
		if err != nil {
			return fmt.Errorf("invalid .terraformignore rules: %#w", err)
		}
		if ignored.Excluded {
			err := os.RemoveAll(absPath)
			if err != nil {
				return fmt.Errorf("failed to remove ignored file %s: %s", relPath, err)
			}
			return nil
		}

		// For directories we also need to check with a path separator on the
		// end, which ignores entire subtrees.
		//
		// TODO: What about exclusion rules that follow a matching directory?
		// Example:
		//   /logs
		//   !/logs/production/*
		if info.IsDir() {
			ignored, err := ignoreRules.Excludes(relPath + string(os.PathSeparator))
			if err != nil {
				return fmt.Errorf("invalid .terraformignore rules: %#w", err)
			}
			if ignored.Excluded {
				err := os.RemoveAll(absPath)
				if err != nil {
					return fmt.Errorf("failed to remove ignored file %s: %s", relPath, err)
				}
				return filepath.SkipDir
			}
		}

		// If we get here then we have a file or directory that isn't
		// covered by the ignore rules, but we still need to make sure it's
		// valid for inclusion in a source bundle.
		// We only allow regular files, directories, and symlinks to either
		// of those as long as they are under the root directory prefix.
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for root directory %q: %w", root, err)
		}
		absRoot, err = filepath.EvalSymlinks(absRoot)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for root directory %q: %w", root, err)
		}
		reAbsPath := filepath.Join(absRoot, relPath)
		realPath, err := filepath.EvalSymlinks(reAbsPath)
		if err != nil {
			return fmt.Errorf("failed to get real path for sub-path %q: %w", relPath, err)
		}
		realPathRel, err := filepath.Rel(absRoot, realPath)
		if err != nil {
			return fmt.Errorf("failed to get real relative path for sub-path %q: %w", relPath, err)
		}

		// After all of the above we can finally safely test whether the
		// transformed path is "local", meaning that it only descends down
		// from the real root.
		if !filepath.IsLocal(realPathRel) {
			return fmt.Errorf("module package path %q is symlink traversing out of the package root", relPath)
		}

		// The real referent must also be either a regular file or a directory.
		// (Not, for example, a Unix device node or socket or other such oddities.)
		lInfo, err := os.Lstat(realPath)
		if err != nil {
			return fmt.Errorf("failed to stat %q: %w", realPath, err)
		}
		if !(lInfo.Mode().IsRegular() || lInfo.Mode().IsDir()) {
			return fmt.Errorf("module package path %q is not a regular file or directory", relPath)
		}

		return nil
	}
}
