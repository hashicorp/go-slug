// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"path"

	"github.com/apparentlymart/go-versions/versions"
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
	pkgRaw, subPathRaw := splitSubPath(given)
	subPath, err := normalizeSubpath(subPathRaw)
	if err != nil {
		return RegistrySource{}, fmt.Errorf("invalid sub-path: %w", err)
	}

	// We delegate the package address parsing to the shared library
	// terraform-registry-address, but then we'll impose some additional
	// validation and normalization over that since we're intentionally
	// being a little stricter than Terraform has historically been,
	// prioritizing "one obvious way to do it" over many esoteric variations.
	pkgOnlyAddr, err := regaddr.ParseModuleSource(pkgRaw)
	if err != nil {
		return RegistrySource{}, err
	}
	if pkgOnlyAddr.Subdir != "" {
		// Should never happen, because we split the subpath off above.
		panic("post-split registry address still has subdir")
	}

	return RegistrySource{
		pkg:     pkgOnlyAddr.Package,
		subPath: subPath,
	}, nil
}

// ParseRegistryPackage parses the given string as a registry package address,
// which is the same syntax as a registry source address with no sub-path
// portion.
func ParseRegistryPackage(given string) (regaddr.ModulePackage, error) {
	srcAddr, err := ParseRegistrySource(given)
	if err != nil {
		return regaddr.ModulePackage{}, err
	}
	if srcAddr.subPath != "" {
		return regaddr.ModulePackage{}, fmt.Errorf("remote package address may not have a sub-path")
	}
	return srcAddr.pkg, nil
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

func (s RegistrySource) Package() regaddr.ModulePackage {
	return s.pkg
}

func (s RegistrySource) SubPath() string {
	return s.subPath
}

// Versioned combines the receiver with a specific selected version number to
// produce a final source address that can be used to resolve to a single
// source package.
func (s RegistrySource) Versioned(selectedVersion versions.Version) RegistrySourceFinal {
	return RegistrySourceFinal{
		src:     s,
		version: selectedVersion,
	}
}

// FinalSourceAddr takes the result of looking up the package portion of the
// receiver in a module registry and appends the reciever's sub-path to the
// returned sub-path to produce the final fully-qualified remote source address.
func (s RegistrySource) FinalSourceAddr(realSource RemoteSource) RemoteSource {
	if s.subPath == "" {
		return realSource // Easy case
	}
	if realSource.subPath == "" {
		return RemoteSource{
			pkg:     realSource.pkg,
			subPath: s.subPath,
		}
	}
	// If we get here then both addresses have a sub-path, so we need to
	// combine them together. This assumes that the "real source" from the
	// module registry will always refer to a directory, which is a fundamental
	// assumption of the module registry protocol.
	return RemoteSource{
		pkg:     realSource.pkg,
		subPath: path.Join(realSource.subPath, s.subPath),
	}
}
