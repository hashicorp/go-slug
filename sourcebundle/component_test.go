package sourcebundle

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/go-slug/sourceaddrs"
	regaddr "github.com/hashicorp/terraform-registry-address"

	"github.com/apparentlymart/go-versions/versions"
)

// TestBuilderComponentSimple tests the basic component resolution pattern:
// component registry address -> remote source address -> download
func TestBuilderComponentSimple(t *testing.T) {
	tracer := testBuildTracer{}
	ctx := tracer.OnContext(context.Background())

	targetDir := t.TempDir()
	builder := testingBuilderWithComponents(
		t, targetDir,
		map[string]string{
			"https://example.com/comp.tgz": "testdata/pkgs/hello",
		},
		nil, // no module registry packages
		map[string]map[string]string{
			"example.com/hashicorp/mycomponent": {
				"2.0.0": "https://example.com/comp.tgz",
			},
		},
		nil, // no module deprecations
	)

	realSource := sourceaddrs.MustParseSource("https://example.com/comp.tgz").(sourceaddrs.RemoteSource)
	compSource := sourceaddrs.MustParseSource("example.com/hashicorp/mycomponent").(sourceaddrs.ComponentSource)
	diags := builder.AddComponentSource(ctx, compSource, versions.All, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics")
	}

	// Note: BuildTracer doesn't yet have component-specific callbacks,
	// so we only see the remote package download events
	wantLog := []string{
		"start downloading https://example.com/comp.tgz",
		"downloaded https://example.com/comp.tgz",
	}
	gotLog := tracer.log
	if diff := cmp.Diff(wantLog, gotLog); diff != "" {
		t.Errorf("wrong trace events\n%s", diff)
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	// Test LocalPathForRemoteSource
	localPkgDir, err := bundle.LocalPathForRemoteSource(realSource)
	if err != nil {
		t.Fatalf("builder does not know a local directory for %s: %s", realSource.Package(), err)
	}

	if info, err := os.Lstat(filepath.Join(localPkgDir, "hello")); err != nil {
		t.Errorf("problem with output file: %s", err)
	} else if !info.Mode().IsRegular() {
		t.Errorf("output file is not a regular file")
	}

	// Test LocalPathForComponentSource
	componentPkgDir, err := bundle.LocalPathForComponentSource(compSource, versions.MustParseVersion("2.0.0"))
	if err != nil {
		t.Fatalf("builder does not know a local directory for %s: %s", compSource.Package(), err)
	}
	if componentPkgDir != localPkgDir {
		t.Errorf("local dir for %s doesn't match local dir for %s", compSource, realSource)
	}

	// Test ComponentPackages
	gotPackages := bundle.ComponentPackages()
	if len(gotPackages) != 1 {
		t.Fatalf("expected 1 component package, got %d", len(gotPackages))
	}
	wantPkgAddr, _ := sourceaddrs.ParseComponentPackage("example.com/hashicorp/mycomponent")
	if gotPackages[0] != wantPkgAddr {
		t.Errorf("wrong package address: got %s, want %s", gotPackages[0], wantPkgAddr)
	}

	// Test ComponentPackageVersions
	gotVersions := bundle.ComponentPackageVersions(wantPkgAddr)
	wantVersions := versions.List{versions.MustParseVersion("2.0.0")}
	if diff := cmp.Diff(wantVersions, gotVersions); diff != "" {
		t.Errorf("wrong versions\n%s", diff)
	}

	// Test ComponentPackageSourceAddr
	gotSourceAddr, ok := bundle.ComponentPackageSourceAddr(wantPkgAddr, versions.MustParseVersion("2.0.0"))
	if !ok {
		t.Fatal("ComponentPackageSourceAddr returned false")
	}
	if gotSourceAddr.String() != "https://example.com/comp.tgz" {
		t.Errorf("wrong source address: got %s, want https://example.com/comp.tgz", gotSourceAddr)
	}
}

// TestBuilderComponentMultipleVersions tests adding multiple component sources with different versions
func TestBuilderComponentMultipleVersions(t *testing.T) {
	ctx := context.Background()

	targetDir := t.TempDir()
	builder := testingBuilderWithComponents(
		t, targetDir,
		map[string]string{
			"https://example.com/comp-1.0.tgz": "testdata/pkgs/hello",
			"https://example.com/comp-2.0.tgz": "testdata/pkgs/hello",
			"https://example.com/comp-3.0.tgz": "testdata/pkgs/hello",
		},
		nil,
		map[string]map[string]string{
			"example.com/hashicorp/mycomponent": {
				"1.0.0": "https://example.com/comp-1.0.tgz",
				"2.0.0": "https://example.com/comp-2.0.tgz",
				"3.0.0": "https://example.com/comp-3.0.tgz",
			},
		},
		nil,
	)

	compSource := sourceaddrs.MustParseSource("example.com/hashicorp/mycomponent").(sourceaddrs.ComponentSource)

	// Add component three times with different constraints to get all versions in the bundle
	v1Constraint, _ := versions.MeetingConstraintsStringRuby("~> 1.0")
	v2Constraint, _ := versions.MeetingConstraintsStringRuby("~> 2.0")
	v3Constraint, _ := versions.MeetingConstraintsStringRuby("~> 3.0")

	diags := builder.AddComponentSource(ctx, compSource, v1Constraint, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics for v1")
	}

	diags = builder.AddComponentSource(ctx, compSource, v2Constraint, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics for v2")
	}

	diags = builder.AddComponentSource(ctx, compSource, v3Constraint, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics for v3")
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	pkgAddr, _ := sourceaddrs.ParseComponentPackage("example.com/hashicorp/mycomponent")

	// Should have all three versions in the bundle's component package sources
	gotVersions := bundle.ComponentPackageVersions(pkgAddr)
	wantVersions := versions.List{
		versions.MustParseVersion("1.0.0"),
		versions.MustParseVersion("2.0.0"),
		versions.MustParseVersion("3.0.0"),
	}
	if diff := cmp.Diff(wantVersions, gotVersions); diff != "" {
		t.Errorf("wrong versions\n%s", diff)
	}

	// Verify each version points to correct source
	// Note: the test data uses short version format in URLs (1.0, 2.0, 3.0)
	expectedURLs := map[string]string{
		"1.0.0": "https://example.com/comp-1.0.tgz",
		"2.0.0": "https://example.com/comp-2.0.tgz",
		"3.0.0": "https://example.com/comp-3.0.tgz",
	}
	for ver, expectedURL := range expectedURLs {
		version := versions.MustParseVersion(ver)
		sourceAddr, ok := bundle.ComponentPackageSourceAddr(pkgAddr, version)
		if !ok {
			t.Errorf("no source address for version %s", ver)
			continue
		}
		if sourceAddr.String() != expectedURL {
			t.Errorf("wrong source for version %s: got %s, want %s", ver, sourceAddr, expectedURL)
		}
	}
}

// TestBuilderComponentFinalSource tests ComponentSourceFinal resolution
func TestBuilderComponentFinalSource(t *testing.T) {
	ctx := context.Background()

	targetDir := t.TempDir()
	builder := testingBuilderWithComponents(
		t, targetDir,
		map[string]string{
			"https://example.com/comp.tgz": "testdata/pkgs/hello",
		},
		nil,
		map[string]map[string]string{
			"example.com/hashicorp/mycomponent": {
				"1.2.3": "https://example.com/comp.tgz",
			},
		},
		nil,
	)

	compSource := sourceaddrs.MustParseSource("example.com/hashicorp/mycomponent").(sourceaddrs.ComponentSource)
	version := versions.MustParseVersion("1.2.3")

	// Create a FinalComponentSource and use AddFinalComponentSource
	finalSource := compSource.Versioned(version)
	diags := builder.AddFinalComponentSource(ctx, finalSource, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics")
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	// finalSource already created above for testing LocalPathForSource switch case

	// Test LocalPathForFinalComponentSource
	localPath, err := bundle.LocalPathForFinalComponentSource(finalSource)
	if err != nil {
		t.Fatalf("LocalPathForFinalComponentSource failed: %s", err)
	}

	// Test the critical LocalPathForSource switch case
	localPathFromSwitch, err := bundle.LocalPathForSource(finalSource)
	if err != nil {
		t.Fatalf("LocalPathForSource failed for ComponentSourceFinal: %s", err)
	}

	if localPath != localPathFromSwitch {
		t.Errorf("LocalPathForFinalComponentSource and LocalPathForSource returned different paths: %s vs %s", localPath, localPathFromSwitch)
	}

	// Verify the file actually exists
	if info, err := os.Lstat(filepath.Join(localPath, "hello")); err != nil {
		t.Errorf("problem with output file: %s", err)
	} else if !info.Mode().IsRegular() {
		t.Errorf("output file is not a regular file")
	}
}

// TestBuilderComponentWithSubpath tests component source with subpath
func TestBuilderComponentWithSubpath(t *testing.T) {
	ctx := context.Background()

	targetDir := t.TempDir()
	builder := testingBuilderWithComponents(
		t, targetDir,
		map[string]string{
			"https://example.com/comp.tgz": "testdata/pkgs", // Contains subdirs
		},
		nil,
		map[string]map[string]string{
			"example.com/hashicorp/mycomponent": {
				"1.0.0": "https://example.com/comp.tgz",
			},
		},
		nil,
	)

	// Parse component source with subpath
	compSourceWithSubpath := sourceaddrs.MustParseSource("example.com/hashicorp/mycomponent//hello").(sourceaddrs.ComponentSource)
	diags := builder.AddComponentSource(ctx, compSourceWithSubpath, versions.All, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics")
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	// LocalPathForComponentSource should include the subpath
	localPath, err := bundle.LocalPathForComponentSource(compSourceWithSubpath, versions.MustParseVersion("1.0.0"))
	if err != nil {
		t.Fatalf("LocalPathForComponentSource failed: %s", err)
	}

	// The path should point to the 'hello' subdirectory
	if info, err := os.Lstat(filepath.Join(localPath, "hello")); err != nil {
		t.Errorf("problem with output file in subpath: %s", err)
	} else if !info.Mode().IsRegular() {
		t.Errorf("output file is not a regular file")
	}
}

// TestComponentPackagesSorting tests that ComponentPackages returns consistently sorted results
func TestComponentPackagesSorting(t *testing.T) {
	ctx := context.Background()

	targetDir := t.TempDir()
	builder := testingBuilderWithComponents(
		t, targetDir,
		map[string]string{
			"https://example.com/alpha.tgz": "testdata/pkgs/hello",
			"https://example.com/beta.tgz":  "testdata/pkgs/hello",
			"https://example.com/gamma.tgz": "testdata/pkgs/hello",
		},
		nil,
		map[string]map[string]string{
			"example.com/zzz/component": {
				"1.0.0": "https://example.com/alpha.tgz",
			},
			"example.com/aaa/component": {
				"1.0.0": "https://example.com/beta.tgz",
			},
			"example.com/mmm/component": {
				"1.0.0": "https://example.com/gamma.tgz",
			},
		},
		nil,
	)

	// Add components in random order
	for _, addr := range []string{
		"example.com/zzz/component",
		"example.com/aaa/component",
		"example.com/mmm/component",
	} {
		compSource := sourceaddrs.MustParseSource(addr).(sourceaddrs.ComponentSource)
		diags := builder.AddComponentSource(ctx, compSource, versions.All, noDependencyFinder)
		if len(diags) > 0 {
			t.Fatalf("unexpected diagnostics for %s", addr)
		}
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	// ComponentPackages should return sorted results
	gotPackages := bundle.ComponentPackages()
	if len(gotPackages) != 3 {
		t.Fatalf("expected 3 packages, got %d", len(gotPackages))
	}

	// Check that they're sorted lexicographically by string representation
	wantOrder := []string{
		"example.com/aaa/component",
		"example.com/mmm/component",
		"example.com/zzz/component",
	}
	for i, pkg := range gotPackages {
		if pkg.String() != wantOrder[i] {
			t.Errorf("package at position %d: got %s, want %s", i, pkg, wantOrder[i])
		}
	}
}

// TestComponentMixedWithModules tests that components and modules can coexist
func TestComponentMixedWithModules(t *testing.T) {
	ctx := context.Background()

	targetDir := t.TempDir()
	builder := testingBuilderWithComponents(
		t, targetDir,
		map[string]string{
			"https://example.com/module.tgz":    "testdata/pkgs/hello",
			"https://example.com/component.tgz": "testdata/pkgs/hello",
		},
		map[string]map[string]string{
			"example.com/hashicorp/mymodule/aws": {
				"1.0.0": "https://example.com/module.tgz",
			},
		},
		map[string]map[string]string{
			"example.com/hashicorp/mycomponent": {
				"2.0.0": "https://example.com/component.tgz",
			},
		},
		nil,
	)

	// Add both module and component
	modSource := sourceaddrs.MustParseSource("example.com/hashicorp/mymodule/aws").(sourceaddrs.RegistrySource)
	compSource := sourceaddrs.MustParseSource("example.com/hashicorp/mycomponent").(sourceaddrs.ComponentSource)

	diags := builder.AddRegistrySource(ctx, modSource, versions.All, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics for module")
	}

	diags = builder.AddComponentSource(ctx, compSource, versions.All, noDependencyFinder)
	if len(diags) > 0 {
		t.Fatal("unexpected diagnostics for component")
	}

	bundle, err := builder.Close()
	if err != nil {
		t.Fatalf("failed to close bundle: %s", err)
	}

	// Check both are present
	gotModules := bundle.RegistryPackages()
	if len(gotModules) != 1 {
		t.Errorf("expected 1 module package, got %d", len(gotModules))
	}

	gotComponents := bundle.ComponentPackages()
	if len(gotComponents) != 1 {
		t.Errorf("expected 1 component package, got %d", len(gotComponents))
	}
}

// testingBuilderWithComponents extends testingBuilder to support component packages
func testingBuilderWithComponents(
	t *testing.T,
	targetDir string,
	remotePackages map[string]string,
	registryPackages map[string]map[string]string,
	componentPackages map[string]map[string]string,
	registryVersionDeprecations map[string]map[string]*ModulePackageVersionDeprecation,
) *Builder {
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
	type fakeComponentPackage struct {
		pkgAddr  regaddr.ComponentPackage
		versions map[versions.Version]sourceaddrs.RemoteSource
	}

	remotePkgs := make([]fakeRemotePackage, 0, len(remotePackages))
	registryPkgs := make([]fakeRegistryPackage, 0, len(registryPackages))
	componentPkgs := make([]fakeComponentPackage, 0, len(componentPackages))
	registryDeprecations := make(map[string]map[versions.Version]*ModulePackageVersionDeprecation)

	// Parse remote packages
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

	// Parse module registry packages
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

	// Parse component packages
	for pkgAddrRaw, versionsRaw := range componentPackages {
		pkgAddr, err := sourceaddrs.ParseComponentPackage(pkgAddrRaw)
		if err != nil {
			t.Fatalf("invalid component package address %q: %s", pkgAddrRaw, err)
		}
		pkg := fakeComponentPackage{
			pkgAddr:  pkgAddr,
			versions: make(map[versions.Version]sourceaddrs.RemoteSource),
		}
		for versionRaw, sourceAddrRaw := range versionsRaw {
			version, err := versions.ParseVersion(versionRaw)
			if err != nil {
				t.Fatalf("invalid component package version %q for %s: %s", versionRaw, pkgAddr, err)
			}
			sourceAddr, err := sourceaddrs.ParseRemoteSource(sourceAddrRaw)
			if err != nil {
				t.Fatalf("invalid component package source address %q for %s %s: %s", sourceAddrRaw, pkgAddr, version, err)
			}
			pkg.versions[version] = sourceAddr
		}
		componentPkgs = append(componentPkgs, pkg)
	}

	for pkgAddrRaw, deprecations := range registryVersionDeprecations {
		pkgAddr, err := sourceaddrs.ParseRegistryPackage(pkgAddrRaw)
		if err != nil {
			t.Fatalf("invalid registry package address %q: %s", pkgAddrRaw, err)
		}
		if registryDeprecations[pkgAddr.Namespace] == nil {
			registryDeprecations[pkgAddr.Namespace] = make(map[versions.Version]*ModulePackageVersionDeprecation)
		}
		for versionRaw, versionDeprecation := range deprecations {
			version, err := versions.ParseVersion(versionRaw)
			if err != nil {
				t.Fatalf("invalid registry package version %q for %s: %s", versionRaw, pkgAddr, err)
			}
			registryDeprecations[pkgAddr.Namespace][version] = versionDeprecation
		}
	}

	fetcher := packageFetcherFunc(func(ctx context.Context, sourceType string, url *url.URL, targetDir string) (FetchSourcePackageResponse, error) {
		var ret FetchSourcePackageResponse
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
				ret.Versions = make([]ModulePackageInfo, 0, len(pkg.versions))
				for version := range pkg.versions {
					ret.Versions = append(ret.Versions, ModulePackageInfo{
						Version:     version,
						Deprecation: registryDeprecations[pkg.pkgAddr.Namespace][version],
					})
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
		componentPackageVersions: func(ctx context.Context, pkgAddr regaddr.ComponentPackage) (ComponentPackageVersionsResponse, error) {
			var ret ComponentPackageVersionsResponse
			for _, pkg := range componentPkgs {
				if pkg.pkgAddr != pkgAddr {
					continue
				}
				ret.Versions = make([]ComponentPackageInfo, 0, len(pkg.versions))
				for version := range pkg.versions {
					ret.Versions = append(ret.Versions, ComponentPackageInfo{
						Version: version,
					})
				}
				return ret, nil
			}
			return ret, fmt.Errorf("no fake component registry package matches %s", pkgAddr)
		},
		componentPackageSourceAddr: func(ctx context.Context, pkgAddr regaddr.ComponentPackage, version versions.Version) (ComponentPackageSourceAddrResponse, error) {
			var ret ComponentPackageSourceAddrResponse
			for _, pkg := range componentPkgs {
				if pkg.pkgAddr != pkgAddr {
					continue
				}
				sourceAddr, ok := pkg.versions[version]
				if !ok {
					return ret, fmt.Errorf("no fake component registry package matches %s %s", pkgAddr, version)
				}
				ret.SourceAddr = sourceAddr
				return ret, nil
			}
			return ret, fmt.Errorf("no fake component registry package matches %s", pkgAddr)
		},
	}

	builder, err := NewBuilder(targetDir, fetcher, registryClient)
	if err != nil {
		t.Fatalf("failed to create builder: %s", err)
	}
	return builder
}
