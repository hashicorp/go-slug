// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

import (
	"io/fs"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/hashicorp/go-slug/sourceaddrs"
)

// A DependencyFinder analyzes a file or directory inside a source package
// and reports any dependencies described in that location.
//
// The same location could potentially be analyzed by multiple different
// DependencyFinder implementations if e.g. it's a directory containing
// a mixture of different kinds of artifact where each artifact has a
// disjoint set of relevant files.
//
// All DependencyFinder implementations must be comparable in the sense of
// supporting the == operator without panicking, and should typically be
// singletons, because [Builder] will use values of this type as part of
// the unique key for tracking whether a particular dependency has already
// been analyzed. A typical DependencyFinder implementation is an empty
// struct type with the FindDependency method implemented on it.
type DependencyFinder interface {
	// FindDependencies should analyze the file or directory at the given
	// sub-path of the given filesystem and then call the given callback
	// once for each detected dependency, providing both its source
	// address and the appropriate [DependencyFinder] for whatever kind
	// of source artifact is expected at that source address.
	//
	// The same source address can potentially contain artifacts of multiple
	// different types. The calling [Builder] will visit each distinct
	// (source, finder) pair only once for analysis, and will also aim to
	// avoid redundantly re-fetching the same source package more than once.
	//
	// If an implementer sends a local source address to the callback function,
	// the calling [Builder] will automatically resolve that relative to
	// the source address being analyzed. Implementers should typically first
	// validate that the local address does not traverse up (with "..") more
	// levels than are included in subPath, because implementers can return
	// higher-quality error diagnostics (with source location information)
	// than the calling Builder can.
	//
	// If the implementer emits diagnostics with source location information
	// then the filenames in the source ranges must be strings that would
	// pass [fs.ValidPath] describing a path from the root of the given fs
	// to the file containing the error. The builder will then translate those
	// paths into remote source address strings within the containing package.
	FindDependencies(fsys fs.FS, subPath string, deps *Dependencies) Diagnostics
}

// Dependencies is part of the callback API for [DependencyFinder]. Dependency
// finders use the methods of this type to report the dependencies they find
// in the source artifact being analyzed.
type Dependencies struct {
	baseAddr sourceaddrs.RemoteSource

	remoteCb          func(source sourceaddrs.RemoteSource, depFinder DependencyFinder)
	registryCb        func(source sourceaddrs.RegistrySource, allowedVersions versions.Set, depFinder DependencyFinder)
	localResolveErrCb func(err error)
}

func (d *Dependencies) AddRemoteSource(source sourceaddrs.RemoteSource, depFinder DependencyFinder) {
	d.remoteCb(source, depFinder)
}

func (d *Dependencies) AddRegistrySource(source sourceaddrs.RegistrySource, allowedVersions versions.Set, depFinder DependencyFinder) {
	d.registryCb(source, allowedVersions, depFinder)
}

func (d *Dependencies) AddLocalSource(source sourceaddrs.LocalSource, depFinder DependencyFinder) {
	// A local source always becomes a remote source in the same package as
	// the current base address.
	realSource, err := sourceaddrs.ResolveRelativeSource(d.baseAddr, source)
	if err != nil {
		d.localResolveErrCb(err)
		return
	}
	// realSource is guaranteed to be a RemoteSource because source is
	// a LocalSource and so the ResolveRelativeSource address is guaranteed
	// to have the same source type as d.baseAddr.
	d.remoteCb(realSource.(sourceaddrs.RemoteSource), depFinder)
}

// disable ensures that a [DependencyFinder] implementation can't incorrectly
// hold on to its given Dependencies object and continue calling it after it
// returns.
func (d *Dependencies) disable() {
	d.remoteCb = nil
	d.registryCb = nil
	d.localResolveErrCb = nil
}
