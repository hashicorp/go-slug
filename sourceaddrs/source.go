// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"path"
	"strings"
)

// Source acts as a tagged union over the three possible source address types,
// for situations where all three are acceptable.
//
// Source is used to specify source addresses for installation. Once packages
// have been resolved and installed we use [SourceFinal] instead to represent
// those finalized selections, which allows capturing the selected version
// number for a module registry source address.
//
// Only address types within this package can implement Source.
type Source interface {
	sourceSigil()

	String() string
	SupportsVersionConstraints() bool
}

// ParseSource attempts to parse the given string as any one of the three
// supported source address types, recognizing which type it belongs to based
// on the syntax differences between the address forms.
func ParseSource(given string) (Source, error) {
	if strings.TrimSpace(given) != given {
		return nil, fmt.Errorf("source address must not have leading or trailing spaces")
	}
	if len(given) == 0 {
		return nil, fmt.Errorf("a valid source address is required")
	}
	switch {
	case looksLikeLocalSource(given) || given == "." || given == "..":
		ret, err := ParseLocalSource(given)
		if err != nil {
			return nil, fmt.Errorf("invalid local source address %q: %w", given, err)
		}
		return ret, nil
	case looksLikeRegistrySource(given):
		ret, err := ParseRegistrySource(given)
		if err != nil {
			return nil, fmt.Errorf("invalid module registry source address %q: %w", given, err)
		}
		return ret, nil
	default:
		// If it's neither a local source nor a module registry source then
		// we'll assume it's intended to be a remote source.
		// (This parser will return a suitable error if the given string
		// is not of any of the supported address types.)
		ret, err := ParseRemoteSource(given)
		if err != nil {
			return nil, fmt.Errorf("invalid remote source address %q: %w", given, err)
		}
		return ret, nil
	}
}

// MustParseSource is a thin wrapper around [ParseSource] that panics if it
// returns an error, or returns its result if not.
func MustParseSource(given string) Source {
	ret, err := ParseSource(given)
	if err != nil {
		panic(err)
	}
	return ret
}

// ResolveRelativeSource calculates a new source address from the combination
// of two other source addresses.
//
// If "b" is already an absolute source address then the result is "b" verbatim.
//
// If "b" is a relative source then the result is an address of the same type
// as "a", but with a different path component. If "a" is an absolute address
// type then the result is guaranteed to also be an absolute address type.
//
// Returns an error if "b" is a relative path that attempts to traverse out
// of the package of an absolute address given in "a".
func ResolveRelativeSource(a, b Source) (Source, error) {
	if sourceIsAbs(b) {
		return b, nil
	}
	// If we get here then b is definitely a local source, because
	// otherwise it would have been absolute.
	bRaw := b.(LocalSource).relPath

	switch a := a.(type) {
	case LocalSource:
		aRaw := a.relPath
		new := path.Join(aRaw, bRaw)
		if !looksLikeLocalSource(new) {
			new = "./" + new // preserve LocalSource's prefix invariant
		}
		return LocalSource{relPath: new}, nil
	case RegistrySource:
		aSub := a.subPath
		newSub, err := joinSubPath(aSub, bRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid traversal from %s: %w", a.String(), err)
		}
		return RegistrySource{
			pkg:     a.pkg,
			subPath: newSub,
		}, nil
	case RemoteSource:
		aSub := a.subPath
		newSub, err := joinSubPath(aSub, bRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid traversal from %s: %w", a.String(), err)
		}
		return RemoteSource{
			pkg:     a.pkg,
			subPath: newSub,
		}, nil
	default:
		// Should not get here, because the cases above are exhaustive for
		// all of our defined Source implementations.
		panic(fmt.Sprintf("unsupported Source implementation %T", a))
	}
}

// SourceFilename returns the base name (in the same sense as [path.Base])
// of the sub-path or local path portion of the given source address.
//
// This only really makes sense for a source address that refers to an
// individual file, and is intended for needs such as using the suffix of
// the filename to decide how to parse a particular file. Passing a source
// address that refers to a directory will not fail but its result is
// unlikely to be useful.
func SourceFilename(addr Source) string {
	switch addr := addr.(type) {
	case LocalSource:
		return path.Base(addr.RelativePath())
	case RemoteSource:
		return path.Base(addr.SubPath())
	case RegistrySource:
		return path.Base(addr.SubPath())
	default:
		// above should be exhaustive for all source types
		panic(fmt.Sprintf("cannot SourceFilename for %T", addr))
	}
}

func sourceIsAbs(source Source) bool {
	_, isLocal := source.(LocalSource)
	return !isLocal
}
