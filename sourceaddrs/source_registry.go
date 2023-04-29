package sourceaddrs

import (
	"fmt"

	regaddr "github.com/hashicorp/terraform-registry-address"
)

// RegistrySource represents a source address referring to a set of versions
// published in a Module Registry.
//
// A RegistrySource is an extra indirection over a set of [RemoteSource]
// addresses, which Terraform chooses from based on version constraints given
// alongside the registry source address.
type RegistrySource struct {
	pkg regaddr.ModulePackage

	// subPath is an optional subdirectory or sub-file path beneath the
	// prefix of the selected underlying source address.
	//
	// Sub-paths are always slash-separated paths interpreted relative to
	// the root of the package, and may not include ".." or "." segments.
	// The sub-path is empty to indicate the root directory of the package.
	subPath string
}

// sourceSigil implements Source
func (s RegistrySource) sourceSigil() {}

var _ Source = RegistrySource{}

func looksLikeRegistrySource(given string) bool {
	_, err := regaddr.ParseModuleSource(given)
	return err == nil
}

// ParseRegistrySource parses the given string as a registry source address,
// or returns an error if it does not use the correct syntax for interpretation
// as a registry source address.
func ParseRegistrySource(given string) (RegistrySource, error) {
	// We delegate the first level of parsing to the shared library
	// terraform-registry-address, but then we'll impose some additional
	// validation and normalization over that since we're intentionally
	// being a little stricter than Terraform has historically been,
	// prioritizing "one obvious way to do it" over many esoteric variations.

	startingAddr, err := regaddr.ParseModuleSource(given)
	if err != nil {
		return RegistrySource{}, err
	}

	subPath, err := normalizeSubpath(startingAddr.Subdir)
	if err != nil {
		return RegistrySource{}, fmt.Errorf("invalid sub-path: %w", err)
	}

	return RegistrySource{
		pkg:     startingAddr.Package,
		subPath: subPath,
	}, nil
}

func (s RegistrySource) String() string {
	if s.subPath != "" {
		return s.pkg.String() + "//" + s.subPath
	}
	return s.pkg.String()
}

func (s RegistrySource) SupportsVersionConstraints() bool {
	return true
}
