// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"regexp"

	"github.com/apparentlymart/go-versions/versions"
	regaddr "github.com/hashicorp/terraform-registry-address"
)

// RegistrySourceFinal annotates a [RegistrySource] with a specific version
// selection, thereby making it sufficient for selecting a single real source
// package.
//
// Registry sources are weird in comparison to others in that they must be
// combined with a version constraint to select from possibly many available
// versions. After completing the version selection process, the result can
// be represented as a RegistrySourceFinal that carries the selected version
// number along with the originally-specified source address.
type RegistrySourceFinal struct {
	src     RegistrySource
	version versions.Version
}

// NOTE: RegistrySourceFinal is intentionally not a Source, because it isn't
// possible to represent a final registry source as a single source address
// string.
var _ FinalSource = RegistrySourceFinal{}

func looksLikeFinalRegistrySource(given string) bool {
	var addr string
	if matches := finalRegistrySourcePattern.FindStringSubmatch(given); len(matches) != 0 {
		addr = matches[1]
		if len(matches) == 5 {
			addr = fmt.Sprintf("%s//%s", addr, matches[4])
		}
	}
	return looksLikeRegistrySource(addr)
}

// finalSourceSigil implements FinalSource
func (s RegistrySourceFinal) finalSourceSigil() {}

// ParseFinalRegistrySource parses the given string as a final registry source
// address, or returns an error if it does not use the correct syntax for
// interpretation as a final registry source address.
func ParseFinalRegistrySource(given string) (RegistrySourceFinal, error) {
	var addr, ver string
	if matches := finalRegistrySourcePattern.FindStringSubmatch(given); len(matches) != 0 {
		addr = matches[1]
		ver = matches[2]
		if len(matches) == 5 {
			addr = fmt.Sprintf("%s//%s", addr, matches[4])
		}
	}
	version, err := versions.ParseVersion(ver)
	if err != nil {
		return RegistrySourceFinal{}, fmt.Errorf("invalid version: %w", err)
	}
	regSrc, err := ParseRegistrySource(addr)
	if err != nil {
		return RegistrySourceFinal{}, fmt.Errorf("invalid registry source: %w", err)
	}
	return regSrc.Versioned(version), nil
}

// Unversioned returns the address of the registry package that this final
// address is a version of.
func (s RegistrySourceFinal) Unversioned() RegistrySource {
	return s.src
}

func (s RegistrySourceFinal) Package() regaddr.ModulePackage {
	return s.src.Package()
}

func (s RegistrySourceFinal) SubPath() string {
	return s.src.SubPath()
}

func (s RegistrySourceFinal) SelectedVersion() versions.Version {
	return s.version
}

func (s RegistrySourceFinal) String() string {
	pkgAddr := s.src.Package()
	subPath := s.src.SubPath()
	if subPath != "" {
		return pkgAddr.String() + "@" + s.version.String() + "//" + subPath
	}
	return pkgAddr.String() + "@" + s.version.String()
}

// FinalSourceAddr takes the result of looking up the package portion of the
// receiver in a module registry and appends the reciever's sub-path to the
// returned sub-path to produce the final fully-qualified remote source address.
func (s RegistrySourceFinal) FinalSourceAddr(realSource RemoteSource) RemoteSource {
	// The version number doesn't have any impact on how we combine the
	// paths together, so we can just delegate to our unversioned equivalent.
	return s.Unversioned().FinalSourceAddr(realSource)
}

// finalRegistrySourcePattern is a non-exhaustive regexp which looks only for
// the expected three components of a RegistrySourceFinal string encoding: the
// package address, version, and subpath. The subpath is optional.
var finalRegistrySourcePattern = regexp.MustCompile(`^(.+)@([^/]+)(//(.+))?$`)
