// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"net/url"
)

type RemotePackage struct {
	sourceType string

	// NOTE: A remote package URL may never have a "userinfo" portion, and
	// all relevant fields are comparable, so it's safe to compare
	// RemotePackage using the == operator.
	url url.URL
}

// ParseRemotePackage parses a standalone remote package address, which is a
// remote source address without any sub-path portion.
func ParseRemotePackage(given string) (RemotePackage, error) {
	srcAddr, err := ParseRemoteSource(given)
	if err != nil {
		return RemotePackage{}, err
	}
	if srcAddr.subPath != "" {
		return RemotePackage{}, fmt.Errorf("remote package address may not have a sub-path")
	}
	return srcAddr.pkg, nil
}

func (p RemotePackage) String() string {
	// Our address normalization rules are a bit odd since we inherited the
	// fundamentals of this addressing scheme from go-getter.
	if p.url.Scheme == p.sourceType {
		// When scheme and source type match we don't actually mention the
		// source type in the stringification, because it looks redundant
		// and confusing.
		return p.url.String()
	}
	return p.sourceType + "::" + p.url.String()
}

// SourceAddr returns a remote source address referring to the given sub-path
// inside the recieving package.
//
// subPath must be a valid sub-path (as defined by [ValidSubPath]) or this
// function will panic. An empty string is a valid sub-path representing the
// root directory of the package.
func (p RemotePackage) SourceAddr(subPath string) RemoteSource {
	finalPath, err := normalizeSubpath(subPath)
	if err != nil {
		panic(fmt.Sprintf("invalid subPath: %s", subPath))
	}
	return RemoteSource{
		pkg:     p,
		subPath: finalPath,
	}
}

func (p RemotePackage) subPathString(subPath string) string {
	if subPath == "" {
		// Easy case... the package address is also the source address
		return p.String()
	}

	// The weird syntax we've inherited from go-getter expects the URL's
	// query string to appear after the subpath portion, so we need to
	// now tweak the package URL to be a sub-path URL instead.
	subURL := p.url // shallow copy
	subURL.Path += "//" + subPath
	if subURL.Scheme == p.sourceType {
		return subURL.String()
	}
	return p.sourceType + "::" + subURL.String()
}

// SourceType returns the source type component of the package address.
func (p RemotePackage) SourceType() string {
	return p.sourceType
}

// URL returns the URL component of the package address.
//
// Callers MUST NOT mutate anything accessible through the returned pointer,
// even though the Go type system cannot enforce that.
func (p RemotePackage) URL() *url.URL {
	return &p.url
}
