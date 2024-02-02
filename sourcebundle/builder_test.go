// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/apparentlymart/go-versions/versions/constraints"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/go-slug/sourceaddrs"
	regaddr "github.com/hashicorp/terraform-registry-address"
)

func TestBuilderSimple(t *testing.T) {
	// This tests the common pattern of specifying a module registry address
	// to start, having that translated into a real remote source address,
	// and then downloading from that real source address. There are no
	// oddities or edge-cases here.

	tracer := testBuildTracer{}
	ctx := tracer.OnContext(context.Background())

	targetDir := t.TempDir()
	builder := testingBuilder(
		t, targetDir,
		map[string]string{
			"https://example.com/foo.tgz": "testdata/pkgs/hello",
		},
		map[string]map[string]string{
			"example.com/foo/bar/baz": map[string]string{
				"1.0.0": "https://example.com/foo.tgz",
			},
		},
	)

	realSource := sourceaddrs.MustParseSource("https://example.com/foo.tgz").(sourceaddrs.RemoteSource)
	regSource := sourceaddrs.MustParseSource("example.com/foo/bar/baz").(sourceaddrs.RegistrySource)
	diags := builder.AddRegistrySource(ctx, regSource, versions.All, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics")
	}

	wantLog := []string{
		"start requesting versions for example.com/foo/bar/baz",
		"success requesting versions for example.com/foo/bar/baz",
		"start requesting source address for example.com/foo/bar/baz 1.0.0",
		"source address for example.com/foo/bar/baz 1.0.0 is https://example.com/foo.tgz",
		"start downloading https://example.com/foo.tgz",
		"downloaded https://example.com/foo.tgz",
	}
	gotLog := tracer.log
	if diff := cmp.Diff(wantLog, gotLog); diff != "" {
		t.Errorf("wrong trace events\n%s", diff)
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	localPkgDir, err := bundle.LocalPathForRemoteSource(realSource)
	if err != nil {
		for pkgAddr, localDir := range builder.remotePackageDirs {
			t.Logf("contents of %s are in %s", pkgAddr, localDir)
		}
		t.Fatalf("builder does not know a local directory for %s: %s", realSource.Package(), err)
	}

	if info, err := os.Lstat(filepath.Join(localPkgDir, "hello")); err != nil {
		t.Errorf("problem with output file: %s", err)
	} else if !info.Mode().IsRegular() {
		t.Errorf("output file is not a regular file")
	}

	// Looking up the original registry address at the selected version
	// should return the same directory, because the registry address is just
	// an indirection over the same source address.
	registryPkgDir, err := bundle.LocalPathForRegistrySource(regSource, versions.MustParseVersion("1.0.0"))
	if err != nil {
		t.Fatalf("builder does not know a local directory for %s: %s", regSource.Package(), err)
	}
	if registryPkgDir != localPkgDir {
		t.Errorf("local dir for %s doesn't match local dir for %s", regSource, realSource)
	}
}

func TestBuilderSubdirs(t *testing.T) {
	tracer := testBuildTracer{}
	ctx := tracer.OnContext(context.Background())

	targetDir := t.TempDir()
	builder := testingBuilder(
		t, targetDir,
		map[string]string{
			"https://example.com/subdirs.tgz": "testdata/pkgs/subdirs",
		},
		map[string]map[string]string{
			"example.com/foo/bar/baz": map[string]string{
				// NOTE: The registry response points to a sub-directory of
				// this package, not to the root of the package.
				"1.0.0": "https://example.com/subdirs.tgz//a",
			},
		},
	)

	// NOTE: We're asking for subdir "b" of the registry address. That combines
	// with the registry's own "b" subdir to produce "a/b" as the final
	// subdirectory path.
	regSource := sourceaddrs.MustParseSource("example.com/foo/bar/baz//b").(sourceaddrs.RegistrySource)
	realSource := sourceaddrs.MustParseSource("https://example.com/subdirs.tgz//a/b").(sourceaddrs.RemoteSource)
	diags := builder.AddRegistrySource(ctx, regSource, versions.All, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics")
	}

	wantLog := []string{
		"start requesting versions for example.com/foo/bar/baz",
		"success requesting versions for example.com/foo/bar/baz",
		"start requesting source address for example.com/foo/bar/baz 1.0.0",
		"source address for example.com/foo/bar/baz 1.0.0 is https://example.com/subdirs.tgz//a",
		"start downloading https://example.com/subdirs.tgz",
		"downloaded https://example.com/subdirs.tgz",
	}
	gotLog := tracer.log
	if diff := cmp.Diff(wantLog, gotLog); diff != "" {
		t.Errorf("wrong trace events\n%s", diff)
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	localPkgDir, err := bundle.LocalPathForRemoteSource(realSource)
	if err != nil {
		for pkgAddr, localDir := range builder.remotePackageDirs {
			t.Logf("contents of %s are in %s", pkgAddr, localDir)
		}
		t.Fatalf("builder does not know a local directory for %s: %s", realSource.Package(), err)
	}

	if info, err := os.Lstat(filepath.Join(localPkgDir, "beepbeep")); err != nil {
		t.Errorf("problem with output file: %s", err)
	} else if !info.Mode().IsRegular() {
		t.Errorf("output file is not a regular file")
	}

	// Looking up the original registry address at the selected version
	// should return the same directory, because the registry address is just
	// an indirection over the same source address.
	registryPkgDir, err := bundle.LocalPathForRegistrySource(regSource, versions.MustParseVersion("1.0.0"))
	if err != nil {
		t.Fatalf("builder does not know a local directory for %s: %s", regSource.Package(), err)
	}
	if registryPkgDir != localPkgDir {
		t.Errorf("local dir for %s doesn't match local dir for %s", regSource, realSource)
	}
}

func TestBuilderRemoteDeps(t *testing.T) {
	tracer := testBuildTracer{}
	ctx := tracer.OnContext(context.Background())

	targetDir := t.TempDir()
	builder := testingBuilder(
		t, targetDir,
		map[string]string{
			"https://example.com/with-deps.tgz":   "testdata/pkgs/with-remote-deps",
			"https://example.com/dependency1.tgz": "testdata/pkgs/hello",
			"https://example.com/dependency2.tgz": "testdata/pkgs/terraformignore",
		},
		nil,
	)

	startSource := sourceaddrs.MustParseSource("https://example.com/with-deps.tgz").(sourceaddrs.RemoteSource)
	dep1Source := sourceaddrs.MustParseSource("https://example.com/dependency1.tgz").(sourceaddrs.RemoteSource)
	dep2Source := sourceaddrs.MustParseSource("https://example.com/dependency2.tgz").(sourceaddrs.RemoteSource)
	diags := builder.AddRemoteSource(ctx, startSource, stubDependencyFinder{filename: "dependencies"})
	if len(diags) > 0 {
		for _, diag := range diags {
			t.Errorf("unexpected diagnostic\nSummary: %s\nDetail:  %s", diag.Description().Summary, diag.Description().Detail)
		}
		t.Fatal("unexpected diagnostics")
	}

	wantLog := []string{
		"start downloading https://example.com/with-deps.tgz",
		"downloaded https://example.com/with-deps.tgz",

		// NOTE: The exact ordering of these two pairs is an implementation
		// detail of Builder: it consumes its "queues" in LIFO order. If you've
		// changed that implementation to a different order then it's expected
		// for this to mismatch and you can just reorder these as long as
		// all of the same events appear in any sensible order. Callers are
		// not allowed to depend on the relative ordering of events relating
		// to different packages.
		"start downloading https://example.com/dependency2.tgz",
		"downloaded https://example.com/dependency2.tgz",
		"start downloading https://example.com/dependency1.tgz",
		"downloaded https://example.com/dependency1.tgz",
	}
	gotLog := tracer.log
	if diff := cmp.Diff(wantLog, gotLog); diff != "" {
		t.Errorf("wrong trace events\n%s", diff)
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	t.Run("starting package", func(t *testing.T) {
		localPkgDir, err := bundle.LocalPathForRemoteSource(startSource)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s: %s", startSource.Package(), err)
		}

		if info, err := os.Lstat(filepath.Join(localPkgDir, "dependencies")); err != nil {
			t.Errorf("problem with output file: %s", err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("output file is not a regular file")
		}
	})
	t.Run("dependency 1", func(t *testing.T) {
		localPkgDir, err := bundle.LocalPathForRemoteSource(dep1Source)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s: %s", dep1Source.Package(), err)
		}

		if info, err := os.Lstat(filepath.Join(localPkgDir, "hello")); err != nil {
			t.Errorf("problem with output file: %s", err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("output file is not a regular file")
		}
	})
	t.Run("dependency 2", func(t *testing.T) {
		localPkgDir, err := bundle.LocalPathForRemoteSource(dep2Source)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s: %s", dep2Source.Package(), err)
		}

		if info, err := os.Lstat(filepath.Join(localPkgDir, "included")); err != nil {
			t.Errorf("problem with output file: %s", err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("output file is not a regular file")
		}
	})
}

func TestBuilderRemoteDepsDifferingTypes(t *testing.T) {
	tracer := testBuildTracer{}
	ctx := tracer.OnContext(context.Background())

	targetDir := t.TempDir()
	builder := testingBuilder(
		t, targetDir,
		map[string]string{
			"https://example.com/self_dependency.tgz": "testdata/pkgs/with-remote-deps",
			"https://example.com/dependency1.tgz":     "testdata/pkgs/hello",
			"https://example.com/dependency2.tgz":     "testdata/pkgs/terraformignore",
		},
		nil,
	)

	startSource := sourceaddrs.MustParseSource("https://example.com/self_dependency.tgz").(sourceaddrs.RemoteSource)
	dep1Source := sourceaddrs.MustParseSource("https://example.com/dependency1.tgz").(sourceaddrs.RemoteSource)
	dep2Source := sourceaddrs.MustParseSource("https://example.com/dependency2.tgz").(sourceaddrs.RemoteSource)
	diags := builder.AddRemoteSource(ctx, startSource, stubDependencyFinder{
		filename:     "self_dependency",
		nextFilename: "dependencies",
	})
	if len(diags) > 0 {
		for _, diag := range diags {
			t.Errorf("unexpected diagnostic\nSummary: %s\nDetail:  %s", diag.Description().Summary, diag.Description().Detail)
		}
		t.Fatal("unexpected diagnostics")
	}

	wantLog := []string{
		"start downloading https://example.com/self_dependency.tgz",
		"downloaded https://example.com/self_dependency.tgz",
		"reusing existing local copy of https://example.com/self_dependency.tgz",

		// NOTE: The exact ordering of these two pairs is an implementation
		// detail of Builder: it consumes its "queues" in LIFO order. If you've
		// changed that implementation to a different order then it's expected
		// for this to mismatch and you can just reorder these as long as
		// all of the same events appear in any sensible order. Callers are
		// not allowed to depend on the relative ordering of events relating
		// to different packages.
		"start downloading https://example.com/dependency2.tgz",
		"downloaded https://example.com/dependency2.tgz",
		"start downloading https://example.com/dependency1.tgz",
		"downloaded https://example.com/dependency1.tgz",
	}
	gotLog := tracer.log
	if diff := cmp.Diff(wantLog, gotLog); diff != "" {
		t.Errorf("wrong trace events\n%s", diff)
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	t.Run("starting package", func(t *testing.T) {
		localPkgDir, err := bundle.LocalPathForRemoteSource(startSource)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s: %s", startSource.Package(), err)
		}

		if info, err := os.Lstat(filepath.Join(localPkgDir, "dependencies")); err != nil {
			t.Errorf("problem with output file: %s", err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("output file is not a regular file")
		}
	})
	t.Run("dependency 1", func(t *testing.T) {
		localPkgDir, err := bundle.LocalPathForRemoteSource(dep1Source)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s: %s", dep1Source.Package(), err)
		}

		if info, err := os.Lstat(filepath.Join(localPkgDir, "hello")); err != nil {
			t.Errorf("problem with output file: %s", err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("output file is not a regular file")
		}
	})
	t.Run("dependency 2", func(t *testing.T) {
		localPkgDir, err := bundle.LocalPathForRemoteSource(dep2Source)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s: %s", dep2Source.Package(), err)
		}

		if info, err := os.Lstat(filepath.Join(localPkgDir, "included")); err != nil {
			t.Errorf("problem with output file: %s", err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("output file is not a regular file")
		}
	})
}

func TestBuilderTerraformIgnore(t *testing.T) {
	tracer := testBuildTracer{}
	ctx := tracer.OnContext(context.Background())

	targetDir := t.TempDir()
	builder := testingBuilder(
		t, targetDir,
		map[string]string{
			"https://example.com/ignore.tgz": "testdata/pkgs/terraformignore",
		},
		nil,
	)

	startSource := sourceaddrs.MustParseSource("https://example.com/ignore.tgz").(sourceaddrs.RemoteSource)
	diags := builder.AddRemoteSource(ctx, startSource, noDependencyFinder)
	if len(diags) > 0 {
		for _, diag := range diags {
			t.Errorf("unexpected diagnostic\nSummary: %s\nDetail:  %s", diag.Description().Summary, diag.Description().Detail)
		}
		t.Fatal("unexpected diagnostics")
	}

	wantLog := []string{
		"start downloading https://example.com/ignore.tgz",
		"downloaded https://example.com/ignore.tgz",
	}
	gotLog := tracer.log
	if diff := cmp.Diff(wantLog, gotLog); diff != "" {
		t.Errorf("wrong trace events\n%s", diff)
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	localPkgDir, err := bundle.LocalPathForRemoteSource(startSource)
	if err != nil {
		for pkgAddr, localDir := range builder.remotePackageDirs {
			t.Logf("contents of %s are in %s", pkgAddr, localDir)
		}
		t.Fatalf("builder does not know a local directory for %s: %s", startSource.Package(), err)
	}

	if info, err := os.Lstat(filepath.Join(localPkgDir, "included")); err != nil {
		t.Errorf("problem with output file: %s", err)
	} else if !info.Mode().IsRegular() {
		t.Errorf("output file is not a regular file")
	}

	if _, err := os.Lstat(filepath.Join(localPkgDir, "excluded")); err == nil {
		t.Errorf("excluded file exists; should have been removed")
	} else if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("excluded file exists but is not readable; should have been removed altogether")
	}
}

func TestBuilderCoalescePackages(t *testing.T) {
	tracer := testBuildTracer{}
	ctx := tracer.OnContext(context.Background())

	targetDir := t.TempDir()
	builder := testingBuilder(
		t, targetDir,
		map[string]string{
			"https://example.com/with-deps.tgz":   "testdata/pkgs/with-remote-deps",
			"https://example.com/dependency1.tgz": "testdata/pkgs/hello",
			"https://example.com/dependency2.tgz": "testdata/pkgs/hello",
		},
		nil,
	)

	startSource := sourceaddrs.MustParseSource("https://example.com/with-deps.tgz").(sourceaddrs.RemoteSource)
	dep1Source := sourceaddrs.MustParseSource("https://example.com/dependency1.tgz").(sourceaddrs.RemoteSource)
	dep2Source := sourceaddrs.MustParseSource("https://example.com/dependency2.tgz").(sourceaddrs.RemoteSource)
	diags := builder.AddRemoteSource(ctx, startSource, stubDependencyFinder{filename: "dependencies"})
	if len(diags) > 0 {
		for _, diag := range diags {
			t.Errorf("unexpected diagnostic\nSummary: %s\nDetail:  %s", diag.Description().Summary, diag.Description().Detail)
		}
		t.Fatal("unexpected diagnostics")
	}

	wantLog := []string{
		"start downloading https://example.com/with-deps.tgz",
		"downloaded https://example.com/with-deps.tgz",

		// NOTE: The exact ordering of these two pairs is an implementation
		// detail of Builder: it consumes its "queues" in LIFO order. If you've
		// changed that implementation to a different order then it's expected
		// for this to mismatch and you can just reorder these as long as
		// all of the same events appear in any sensible order. Callers are
		// not allowed to depend on the relative ordering of events relating
		// to different packages.
		"start downloading https://example.com/dependency2.tgz",
		"downloaded https://example.com/dependency2.tgz",
		"start downloading https://example.com/dependency1.tgz",
		"downloaded https://example.com/dependency1.tgz",
	}
	gotLog := tracer.log
	if diff := cmp.Diff(wantLog, gotLog); diff != "" {
		t.Errorf("wrong trace events\n%s", diff)
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	t.Run("starting package", func(t *testing.T) {
		localPkgDir, err := bundle.LocalPathForRemoteSource(startSource)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s: %s", startSource.Package(), err)
		}

		if info, err := os.Lstat(filepath.Join(localPkgDir, "dependencies")); err != nil {
			t.Errorf("problem with output file: %s", err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("output file is not a regular file")
		}
	})
	t.Run("dependency 1", func(t *testing.T) {
		localPkgDir, err := bundle.LocalPathForSource(dep1Source)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s: %s", dep1Source.Package(), err)
		}

		if info, err := os.Lstat(filepath.Join(localPkgDir, "hello")); err != nil {
			t.Errorf("problem with output file: %s", err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("output file is not a regular file")
		}
	})
	t.Run("dependency 2", func(t *testing.T) {
		localPkgDir, err := bundle.LocalPathForRemoteSource(dep2Source)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s: %s", dep2Source.Package(), err)
		}

		if info, err := os.Lstat(filepath.Join(localPkgDir, "hello")); err != nil {
			t.Errorf("problem with output file: %s", err)
		} else if !info.Mode().IsRegular() {
			t.Errorf("output file is not a regular file")
		}

		// The package directory for dependency 2 should be the same as for
		// dependency 1 because they both have identical content, despite
		// having different source addresses.
		otherLocalPkgDir, err := bundle.LocalPathForRemoteSource(dep1Source)
		if err != nil {
			for pkgAddr, localDir := range builder.remotePackageDirs {
				t.Logf("contents of %s are in %s", pkgAddr, localDir)
			}
			t.Fatalf("builder does not know a local directory for %s", dep1Source.Package())
		}
		if otherLocalPkgDir != localPkgDir {
			t.Errorf("'hello' packages were not coalesced\ndep1 path: %s\ndep2 path: %s", otherLocalPkgDir, localPkgDir)
		}
	})
}

func testingBuilder(t *testing.T, targetDir string, remotePackages map[string]string, registryPackages map[string]map[string]string) *Builder {
	t.Helper()

	type fakeRemotePackage struct {
		sourceType string
		url        *url.URL
		localDir   string
	}
	type fakeRegistryPackage struct {
		pkgAddr  regaddr.ModulePackage
		versions map[versions.Version]sourceaddrs.RemoteSource
	}

	remotePkgs := make([]fakeRemotePackage, 0, len(remotePackages))
	registryPkgs := make([]fakeRegistryPackage, 0, len(registryPackages))

	for pkgAddrRaw, localDir := range remotePackages {
		pkgAddr, err := sourceaddrs.ParseRemotePackage(pkgAddrRaw)
		if err != nil {
			t.Fatalf("invalid remote package address %q: %s", pkgAddrRaw, err)
		}
		remotePkgs = append(remotePkgs, fakeRemotePackage{
			sourceType: pkgAddr.SourceType(),
			url:        pkgAddr.URL(),
			localDir:   localDir,
		})
	}

	for pkgAddrRaw, versionsRaw := range registryPackages {
		pkgAddr, err := sourceaddrs.ParseRegistryPackage(pkgAddrRaw)
		if err != nil {
			t.Fatalf("invalid registry package address %q: %s", pkgAddrRaw, err)
		}
		pkg := fakeRegistryPackage{
			pkgAddr:  pkgAddr,
			versions: make(map[versions.Version]sourceaddrs.RemoteSource),
		}
		for versionRaw, sourceAddrRaw := range versionsRaw {
			version, err := versions.ParseVersion(versionRaw)
			if err != nil {
				t.Fatalf("invalid registry package version %q for %s: %s", versionRaw, pkgAddr, err)
			}
			sourceAddr, err := sourceaddrs.ParseRemoteSource(sourceAddrRaw)
			if err != nil {
				t.Fatalf("invalid registry package source address %q for %s %s: %s", sourceAddrRaw, pkgAddr, version, err)
			}
			pkg.versions[version] = sourceAddr
		}
		registryPkgs = append(registryPkgs, pkg)
	}

	fetcher := packageFetcherFunc(func(ctx context.Context, sourceType string, url *url.URL, targetDir string) (FetchSourcePackageResponse, error) {
		var ret FetchSourcePackageResponse
		// Our fake implementation of "fetching" is to just copy one local
		// directory into another.
		for _, pkg := range remotePkgs {
			if pkg.sourceType != sourceType {
				continue
			}
			if pkg.url.String() != url.String() {
				continue
			}
			localDir := pkg.localDir
			err := copyDir(targetDir, localDir)
			if err != nil {
				return ret, fmt.Errorf("copying %s to %s: %w", localDir, targetDir, err)
			}
			return ret, nil
		}
		return ret, fmt.Errorf("no fake remote package matches %s %s", sourceType, url)
	})

	registryClient := registryClientFuncs{
		modulePackageVersions: func(ctx context.Context, pkgAddr regaddr.ModulePackage) (ModulePackageVersionsResponse, error) {
			var ret ModulePackageVersionsResponse
			for _, pkg := range registryPkgs {
				if pkg.pkgAddr != pkgAddr {
					continue
				}
				ret.Versions = make(versions.List, len(pkg.versions))
				for version := range pkg.versions {
					ret.Versions = append(ret.Versions, version)
				}
				return ret, nil
			}
			return ret, fmt.Errorf("no fake registry package matches %s", pkgAddr)
		},
		modulePackageSourceAddr: func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version) (ModulePackageSourceAddrResponse, error) {
			var ret ModulePackageSourceAddrResponse
			for _, pkg := range registryPkgs {
				if pkg.pkgAddr != pkgAddr {
					continue
				}
				sourceAddr, ok := pkg.versions[version]
				if !ok {
					return ret, fmt.Errorf("no fake registry package matches %s %s", pkgAddr, version)
				}
				ret.SourceAddr = sourceAddr
				return ret, nil
			}
			return ret, fmt.Errorf("no fake registry package matches %s", pkgAddr)
		},
	}

	builder, err := NewBuilder(targetDir, fetcher, registryClient)
	if err != nil {
		t.Fatalf("failed to create builder: %s", err)
	}
	return builder
}

// testBuildTracer is a BuildTracer that just remembers calls in memory
// as strings, for relatively-easy comparison in tests.
type testBuildTracer struct {
	log []string
}

func (t *testBuildTracer) OnContext(ctx context.Context) context.Context {
	trace := BuildTracer{
		RegistryPackageVersionsStart: func(ctx context.Context, pkgAddr regaddr.ModulePackage) context.Context {
			t.appendLogf("start requesting versions for %s", pkgAddr)
			return ctx
		},
		RegistryPackageVersionsSuccess: func(ctx context.Context, pkgAddr regaddr.ModulePackage, versions versions.List) {
			t.appendLogf("success requesting versions for %s", pkgAddr)
		},
		RegistryPackageVersionsFailure: func(ctx context.Context, pkgAddr regaddr.ModulePackage, err error) {
			t.appendLogf("error requesting versions for %s: %s", pkgAddr, err)
		},
		RegistryPackageVersionsAlready: func(ctx context.Context, pkgAddr regaddr.ModulePackage, versions versions.List) {
			t.appendLogf("reusing existing versions for %s", pkgAddr)
		},

		RegistryPackageSourceStart: func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version) context.Context {
			t.appendLogf("start requesting source address for %s %s", pkgAddr, version)
			return ctx
		},
		RegistryPackageSourceSuccess: func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version, sourceAddr sourceaddrs.RemoteSource) {
			t.appendLogf("source address for %s %s is %s", pkgAddr, version, sourceAddr)
		},
		RegistryPackageSourceFailure: func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version, err error) {
			t.appendLogf("error requesting source address for %s %s: %s", pkgAddr, version, err)
		},
		RegistryPackageSourceAlready: func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version, sourceAddr sourceaddrs.RemoteSource) {
			t.appendLogf("reusing existing source address for %s %s: %s", pkgAddr, version, sourceAddr)
		},

		RemotePackageDownloadStart: func(ctx context.Context, pkgAddr sourceaddrs.RemotePackage) context.Context {
			t.appendLogf("start downloading %s", pkgAddr)
			return ctx
		},
		RemotePackageDownloadSuccess: func(ctx context.Context, pkgAddr sourceaddrs.RemotePackage) {
			t.appendLogf("downloaded %s", pkgAddr)
		},
		RemotePackageDownloadFailure: func(ctx context.Context, pkgAddr sourceaddrs.RemotePackage, err error) {
			t.appendLogf("failed to download %s: %s", pkgAddr, err)
		},
		RemotePackageDownloadAlready: func(ctx context.Context, pkgAddr sourceaddrs.RemotePackage) {
			t.appendLogf("reusing existing local copy of %s", pkgAddr)
		},

		Diagnostics: func(ctx context.Context, diags Diagnostics) {
			for _, diag := range diags {
				switch diag.Severity() {
				case DiagError:
					t.appendLogf("Error: %s", diag.Description().Summary)
				case DiagWarning:
					t.appendLogf("Warning: %s", diag.Description().Summary)
				default:
					t.appendLogf("Diagnostic with invalid severity: %s", diag.Description().Summary)
				}
			}
		},
	}
	return trace.OnContext(ctx)
}

func (t *testBuildTracer) appendLogf(f string, v ...interface{}) {
	t.log = append(t.log, fmt.Sprintf(f, v...))
}

type packageFetcherFunc func(ctx context.Context, sourceType string, url *url.URL, targetDir string) (FetchSourcePackageResponse, error)

func (f packageFetcherFunc) FetchSourcePackage(ctx context.Context, sourceType string, url *url.URL, targetDir string) (FetchSourcePackageResponse, error) {
	return f(ctx, sourceType, url, targetDir)
}

type registryClientFuncs struct {
	modulePackageVersions   func(ctx context.Context, pkgAddr regaddr.ModulePackage) (ModulePackageVersionsResponse, error)
	modulePackageSourceAddr func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version) (ModulePackageSourceAddrResponse, error)
}

func (f registryClientFuncs) ModulePackageVersions(ctx context.Context, pkgAddr regaddr.ModulePackage) (ModulePackageVersionsResponse, error) {
	return f.modulePackageVersions(ctx, pkgAddr)
}

func (f registryClientFuncs) ModulePackageSourceAddr(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version) (ModulePackageSourceAddrResponse, error) {
	return f.modulePackageSourceAddr(ctx, pkgAddr, version)
}

type noopDependencyFinder struct{}

func (f noopDependencyFinder) FindDependencies(fsys fs.FS, subPath string, deps *Dependencies) Diagnostics {
	return nil
}

var noDependencyFinder = noopDependencyFinder{}

// stubDependencyFinder is a test-only [DependencyFinder] which just reads
// lines of text from a given filename and tries to treat each one as a source
// address, which it then reports as a dependency.
type stubDependencyFinder struct {
	filename     string
	nextFilename string
}

func (f stubDependencyFinder) FindDependencies(fsys fs.FS, subPath string, deps *Dependencies) Diagnostics {
	var diags Diagnostics
	filePath := path.Join(subPath, f.filename)
	file, err := fsys.Open(filePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			diags = diags.Append(&internalDiagnostic{
				severity: DiagError,
				summary:  "Missing stub dependency file",
				detail:   fmt.Sprintf("There is no file %q in the package.", filePath),
			})
		} else {
			diags = diags.Append(&internalDiagnostic{
				severity: DiagError,
				summary:  "Invalid stub dependency file",
				detail:   fmt.Sprintf("Cannot open %q in the package: %s.", filePath, err),
			})
		}
		return diags
	}

	sc := bufio.NewScanner(file) // defaults to scanning for lines
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		sourceAddrRaw, versionsRaw, hasVersions := strings.Cut(line, " ")
		sourceAddr, err := sourceaddrs.ParseSource(sourceAddrRaw)
		if err != nil {
			diags = diags.Append(&internalDiagnostic{
				severity: DiagError,
				summary:  "Invalid source address in stub dependency file",
				detail:   fmt.Sprintf("Cannot use %q as a source address: %s.", sourceAddrRaw, err),
			})
			continue
		}
		if hasVersions && !sourceAddr.SupportsVersionConstraints() {
			diags = diags.Append(&internalDiagnostic{
				severity: DiagError,
				summary:  "Invalid source address in stub dependency file",
				detail:   fmt.Sprintf("Cannot specify a version constraint string for %s.", sourceAddr),
			})
			continue
		}
		var allowedVersions versions.Set
		if hasVersions {
			cnsts, err := constraints.ParseRubyStyleMulti(versionsRaw)
			if err != nil {
				diags = diags.Append(&internalDiagnostic{
					severity: DiagError,
					summary:  "Invalid version constraints in stub dependency file",
					detail:   fmt.Sprintf("Cannot use %q as version constraints for %s: %s.", versionsRaw, sourceAddrRaw, err),
				})
				continue
			}
			allowedVersions = versions.MeetingConstraints(cnsts)
		} else {
			allowedVersions = versions.All
		}

		depFinder := DependencyFinder(noDependencyFinder)
		if f.nextFilename != "" {
			// If a next filename is specified then we're chaining to another
			// dependency file for all of the discovered dependencies.
			depFinder = stubDependencyFinder{filename: f.nextFilename}
		}

		switch sourceAddr := sourceAddr.(type) {
		case sourceaddrs.RemoteSource:
			deps.AddRemoteSource(sourceAddr, depFinder)
		case sourceaddrs.RegistrySource:
			deps.AddRegistrySource(sourceAddr, allowedVersions, depFinder)
		case sourceaddrs.LocalSource:
			deps.AddLocalSource(sourceAddr, depFinder)
		default:
			diags = diags.Append(&internalDiagnostic{
				severity: DiagError,
				summary:  "Unsupported source address type",
				detail:   fmt.Sprintf("stubDependencyFinder doesn't support %T addresses", sourceAddr),
			})
			continue
		}
	}
	if err := sc.Err(); err != nil {
		diags = diags.Append(&internalDiagnostic{
			severity: DiagError,
			summary:  "Invalid stub dependency file",
			detail:   fmt.Sprintf("Failed to read %s in the package: %s.", filePath, err),
		})
		return diags
	}

	return diags
}

func copyDir(dst, src string) error {
	src, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}

	walkFn := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path == src {
			return nil
		}

		// The "path" has the src prefixed to it. We need to join our
		// destination with the path without the src on it.
		dstPath := filepath.Join(dst, path[len(src):])

		// we don't want to try and copy the same file over itself.
		if eq, err := sameFile(path, dstPath); eq {
			return nil
		} else if err != nil {
			return err
		}

		// If we have a directory, make that subdirectory, then continue
		// the walk.
		if info.IsDir() {
			if path == filepath.Join(src, dst) {
				// dst is in src; don't walk it.
				return nil
			}

			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}

			return nil
		}

		// If the current path is a symlink, recreate the symlink relative to
		// the dst directory
		if info.Mode()&os.ModeSymlink == os.ModeSymlink {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}

			return os.Symlink(target, dstPath)
		}

		// If we have a file, copy the contents.
		srcF, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcF.Close()

		dstF, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer dstF.Close()

		if _, err := io.Copy(dstF, srcF); err != nil {
			return err
		}

		// Chmod it
		return os.Chmod(dstPath, info.Mode())
	}

	return filepath.Walk(src, walkFn)
}

func sameFile(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}

	aInfo, err := os.Lstat(a)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	bInfo, err := os.Lstat(b)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return os.SameFile(aInfo, bInfo), nil
}
