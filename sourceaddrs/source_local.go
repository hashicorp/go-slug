// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"path"
	"strings"
)

// LocalSource represents a relative traversal to another path within the same
// source package as whatever source artifact included this path.
//
// LocalSource sources will typically need to be resolved into either
// [RemoteSource] or [RegistrySource] addresses by reference to the address
// of whatever artifact declared them, because otherwise they cannot be
// mapped onto any real source location.
type LocalSource struct {
	// relPath is a slash-separate path in the style of the Go standard
	// library package "path", which should always be stored in its "Clean"
	// form, aside from the mandatory "./" or "../" prefixes.
	relPath string
}

var _ Source = LocalSource{}
var _ FinalSource = LocalSource{}

// sourceSigil implements Source
func (s LocalSource) sourceSigil() {}

// finalSourceSigil implements FinalSource
func (s LocalSource) finalSourceSigil() {}

func looksLikeLocalSource(given string) bool {
	return strings.HasPrefix(given, "./") || strings.HasPrefix(given, "../")
}

// ParseLocalSource interprets the given path as a local source address, or
// returns an error if it cannot be interpreted as such.
func ParseLocalSource(given string) (LocalSource, error) {
	// First we'll catch some situations that seem likely to suggest that
	// the caller was trying to use a real filesystem path instead of
	// just a virtual relative path within a source package.
	if strings.ContainsAny(given, ":\\") {
		return LocalSource{}, fmt.Errorf("must be a relative path using forward-slash separators between segments, like in a relative URL")
	}

	// We distinguish local source addresses from other address types by them
	// starting with some kind of relative path prefix.
	if !looksLikeLocalSource(given) && given != "." && given != ".." {
		return LocalSource{}, fmt.Errorf("must start with either ./ or ../ to indicate a local path")
	}

	clean := path.Clean(given)

	// We use the "path" package's definition of "clean" aside from two
	// exceptions:
	// - we need to retain the leading "./", if it was originally present, to
	//   disambiguate from module registry addresses.
	// - If the cleaned path is just "." or ".." then we need a slash on the end
	//   because that's part of how we recognize an address as a relative path.
	if clean == ".." {
		clean = "../"
	} else if clean == "." {
		clean = "./"
	}
	if !looksLikeLocalSource(clean) {
		clean = "./" + clean
	}

	if clean != given {
		return LocalSource{}, fmt.Errorf("relative path must be written in canonical form %q", clean)
	}

	return LocalSource{relPath: clean}, nil
}

// String implements Source
func (s LocalSource) String() string {
	return s.relPath
}

// SupportsVersionConstraints implements Source
func (s LocalSource) SupportsVersionConstraints() bool {
	return false
}

// RelativePath returns the effective relative path for this source address,
// in our platform-agnostic slash-separated canonical syntax.
func (s LocalSource) RelativePath() string {
	return s.relPath
}
