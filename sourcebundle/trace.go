// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

import (
	"context"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/hashicorp/go-slug/sourceaddrs"
	regaddr "github.com/hashicorp/terraform-registry-address"
)

// BuildTracer contains a set of callbacks that a caller can optionally provide
// to [Builder] methods via their [context.Context] arguments to be notified
// when various long-running events are starting and stopping, to allow both
// for debugging and for UI feedback about progress.
//
// Any or all of the callbacks may be left as nil, in which case no event
// will be delivered for the corresponding event.
//
// The [context.Context] passed to each trace function is guaranteed to be a
// child of the one passed to whatever [Builder] method caused the event
// to occur, and so it can carry cross-cutting information such as distributed
// tracing clients.
//
// The "Start"-suffixed methods all allow returning a new context which will
// then be passed to the corresponding "Success"-suffixed or "Failure"-suffixed
// function, and also used for outgoing requests within the scope of that
// operation. This allows carrying values such as tracing spans between the
// start and end, so they can properly bracket the operation in question. If
// your tracer doesn't need this then just return the given context.
type BuildTracer struct {
	// The RegistryPackageVersions... callbacks frame any requests to
	// fetch the list of available versions for a module registry package.
	RegistryPackageVersionsStart   func(ctx context.Context, pkgAddr regaddr.ModulePackage) context.Context
	RegistryPackageVersionsSuccess func(ctx context.Context, pkgAddr regaddr.ModulePackage, versions versions.List)
	RegistryPackageVersionsFailure func(ctx context.Context, pkgAddr regaddr.ModulePackage, err error)
	RegistryPackageVersionsAlready func(ctx context.Context, pkgAddr regaddr.ModulePackage, versions versions.List)

	// The RegistryPackageSource... callbacks frame any requests to fetch
	// the real underlying source address for a selected registry package
	// version.
	RegistryPackageSourceStart   func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version) context.Context
	RegistryPackageSourceSuccess func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version, sourceAddr sourceaddrs.RemoteSource)
	RegistryPackageSourceFailure func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version, err error)
	RegistryPackageSourceAlready func(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version, sourceAddr sourceaddrs.RemoteSource)

	// The RemotePackageDownload... callbacks frame any requests to download
	// remote source packages.
	RemotePackageDownloadStart   func(ctx context.Context, pkgAddr sourceaddrs.RemotePackage) context.Context
	RemotePackageDownloadSuccess func(ctx context.Context, pkgAddr sourceaddrs.RemotePackage)
	RemotePackageDownloadFailure func(ctx context.Context, pkgAddr sourceaddrs.RemotePackage, err error)
	RemotePackageDownloadAlready func(ctx context.Context, pkgAddr sourceaddrs.RemotePackage)

	// Diagnostics will be called for any diagnostics that describe problems
	// that aren't also reported by calling one of the "Failure" callbacks
	// above. A recipient that is going to report the errors itself using
	// the Failure callbacks anyway should consume diagnostics from this
	// event, rather than from the return values of the [Builder] methods,
	// to avoid redundantly reporting the same errors twice.
	//
	// Diagnostics might be called multiple times during an operation. Callers
	// should consider each new call to represent additional diagnostics,
	// not replacing any previously returned.
	Diagnostics func(ctx context.Context, diags Diagnostics)
}

// OnContext takes a context and returns a derived context which has everything
// the given context already had plus also the receiving BuildTrace object,
// so that passing the resulting context to methods of [Builder] will cause
// the trace object's callbacks to be called.
//
// Each context can have only one tracer, so if the given context already has
// a tracer then it will be overridden by the new one.
func (bt *BuildTracer) OnContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, buildTraceKey, bt)
}

func buildTraceFromContext(ctx context.Context) *BuildTracer {
	ret, ok := ctx.Value(buildTraceKey).(*BuildTracer)
	if !ok {
		// We'll always return a non-nil pointer just because that reduces
		// the amount of boilerplate required in the caller when announcing
		// events.
		ret = &noopBuildTrace
	}
	return ret
}

type buildTraceKeyType int

const buildTraceKey buildTraceKeyType = 0

// noopBuildTrace is an all-nil [BuildTracer] we return a pointer to if we're
// asked for a BuildTrace from a context that doesn't have one.
var noopBuildTrace BuildTracer
