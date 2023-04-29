package sourceaddrs

import (
	"fmt"
)

// Source acts as a tagged union over the three possible source address types,
// for situations where all three are acceptable.
//
// Only address types within this package can implement Source.
type Source interface {
	sourceSigil()

	String() string
	SupportsVersionConstraints() bool
}

// Source attempts to parse the given string as any one of the three supported
// source address types, recognizing which type it belongs to based on the
// syntax differences between the address forms.
func ParseSource(given string) (Source, error) {
	switch {
	case looksLikeLocalSource(given):
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
	// TODO: implement
	panic("unimplemented")
}
