// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

import (
	"context"
	"net/url"
)

// A PackageFetcher knows how to fetch remote source packages into a local
// filesystem directory.
type PackageFetcher interface {
	// FetchSourcePackage retrieves the a source package from the given
	// location and extracts it into the given local filesystem directory.
	//
	// A package fetcher is responsible for ensuring that nothing gets written
	// outside of the given target directory. However, a fetcher can assume that
	// nothing should be modifying or moving targetDir and or any of its contents
	// concurrently with the fetcher running.
	//
	// If the function returns with a nil error then the target directory must be
	// a complete copy of the designated remote package, ready for further analysis.
	//
	// Package fetchers should respond to cancellation of the given
	// [context.Context] to a reasonable extent, so that the source bundle build
	// process can be interrupted relatively promptly. Return a non-nil error when
	// cancelled to allow the caller to detect that the target directory might not
	// be in a consistent state.
	//
	// PackageFetchers should not have any persistent mutable state: each call
	// should be independent of all past, concurrent, and future calls. In
	// particular, a fetcher should not attempt to implement any caching behavior,
	// because it's [Builder]'s responsibility to handle caching and request
	// coalescing during bundle construction to ensure that it will happen
	// consistently across different fetcher implementations.
	FetchSourcePackage(ctx context.Context, sourceType string, url *url.URL, targetDir string) (FetchSourcePackageResponse, error)
}

// FetchSourcePackageResponse is a structure which represents metadata about
// the fetch operation. This type may grow to add more data over time in later
// minor releases.
type FetchSourcePackageResponse struct {
	PackageMeta *PackageMeta
}
