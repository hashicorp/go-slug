// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"path"
	"strings"
)

// FinalSource is a variant of [Source] that always refers to a single
// specific package.
//
// Specifically this models the annoying oddity that while [LocalSource] and
// [RemoteSource] fully specify what they refer to, [RegistrySource] only
// gives partial information and must be qualified with a selected version
// number to determine exactly what it refers to.
type FinalSource interface {
	finalSourceSigil()

	String() string
}

// ParseFinalSource attempts to parse the given string as any one of the three
// supported final source address types, recognizing which type it belongs to
// based on the syntax differences between the address forms.
func ParseFinalSource(given string) (FinalSource, error) {
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
	case looksLikeFinalRegistrySource(given):
		ret, err := ParseFinalRegistrySource(given)
		if err != nil {
			return nil, fmt.Errorf("invalid module registry source address %q: %w", given, err)
		}
		return ret, nil
	default:
		// If it's neither a local source nor a final module registry source
		// then we'll assume it's intended to be a remote source.
		// (This parser will return a suitable error if the given string
		// is not of any of the supported address types.)
		ret, err := ParseRemoteSource(given)
		if err != nil {
			return nil, fmt.Errorf("invalid remote source address %q: %w", given, err)
		}
		return ret, nil
	}
}

// FinalSourceFilename returns the base name (in the same sense as [path.Base])
// of the sub-path or local path portion of the given final source address.
//
// This only really makes sense for a source address that refers to an
// individual file, and is intended for needs such as using the suffix of
// the filename to decide how to parse a particular file. Passing a source
// address that refers to a directory will not fail but its result is
// unlikely to be useful.
func FinalSourceFilename(addr FinalSource) string {
	switch addr := addr.(type) {
	case LocalSource:
		return path.Base(addr.RelativePath())
	case RemoteSource:
		return path.Base(addr.SubPath())
	case RegistrySourceFinal:
		return path.Base(addr.SubPath())
	default:
		// above should be exhaustive for all final source types
		panic(fmt.Sprintf("cannot FinalSourceFilename for %T", addr))
	}
}

// ResolveRelativeFinalSource is like [ResolveRelativeSource] but for
// [FinalSource] addresses instead of [Source] addresses.
//
// Aside from the address type difference its meaning and behavior rules
// are the same.
func ResolveRelativeFinalSource(a, b FinalSource) (FinalSource, error) {
	if finalSourceIsAbs(b) {
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
	case RegistrySourceFinal:
		aSub := a.src.subPath
		newSub, err := joinSubPath(aSub, bRaw)
		if err != nil {
			return nil, fmt.Errorf("invalid traversal from %s: %w", a.String(), err)
		}
		return RegistrySource{
			pkg:     a.Package(),
			subPath: newSub,
		}.Versioned(a.version), nil
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

func finalSourceIsAbs(source FinalSource) bool {
	_, isLocal := source.(LocalSource)
	return !isLocal
}
