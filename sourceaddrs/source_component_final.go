// Copyright IBM Corp. 2018, 2025
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"regexp"

	"github.com/apparentlymart/go-versions/versions"
	regaddr "github.com/hashicorp/terraform-registry-address"
)

type ComponentSourceFinal struct {
	src     ComponentSource
	version versions.Version
}

// NOTE: ComponentSourceFinal is intentionally not a Source, because it isn't
// possible to represent a final registry source as a single source address
// string.
var _ FinalSource = ComponentSourceFinal{}

// looksLikeFinalComponentSource returns true if the given string matches the pattern for a final component source address.
func looksLikeFinalComponentSource(given string) bool {
	var addr string
	if matches := finalComponentSourcePattern.FindStringSubmatch(given); len(matches) != 0 {
		addr = matches[1]
		if len(matches) == 5 {
			addr = fmt.Sprintf("%s//%s", addr, matches[4])
		}
	}
	return looksLikeComponentSource(addr)
}

// finalSourceSigil is a marker method to satisfy the FinalSource interface; it does not perform any action.
func (s ComponentSourceFinal) finalSourceSigil() {}

// ParseFinalComponentSource parses a string into a ComponentSourceFinal, extracting the address and version.
func ParseFinalComponentSource(given string) (ComponentSourceFinal, error) {
	var addr, ver string
	// NOTE: Components follow the same patterns and restriction as
	if matches := finalComponentSourcePattern.FindStringSubmatch(given); len(matches) != 0 {
		addr = matches[1]
		ver = matches[2]
		if len(matches) == 5 {
			addr = fmt.Sprintf("%s//%s", addr, matches[4])
		}
	}

	version, err := versions.ParseVersion(ver)
	if err != nil {
		return ComponentSourceFinal{}, fmt.Errorf("invalid version: %w", err)
	}
	compSrc, err := ParseComponentSource(addr)
	if err != nil {
		return ComponentSourceFinal{}, fmt.Errorf("invalid component source: %w", err)
	}

	return compSrc.Versioned(version), nil
}

// Unversioned returns the address of the registry package that this final
// address is a version of.
func (s ComponentSourceFinal) Unversioned() ComponentSource {
	return s.src
}

// Package returns the ComponentPackage part of the ComponentSourceFinal.
func (s ComponentSourceFinal) Package() regaddr.ComponentPackage {
	return s.src.Package()
}

// SubPath returns the sub-path portion of the ComponentSourceFinal.
func (s ComponentSourceFinal) SubPath() string {
	return s.src.SubPath()
}

// SelectedVersion returns the version associated with the ComponentSourceFinal.
func (s ComponentSourceFinal) SelectedVersion() versions.Version {
	return s.version
}

// String returns the string representation of the ComponentSourceFinal, including version and sub-path if present.
func (s ComponentSourceFinal) String() string {
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
func (s ComponentSourceFinal) FinalSourceAddr(realSource RemoteSource) RemoteSource {
	// The version number doesn't have any impact on how we combine the
	// paths together, so we can just delegate to our unversioned equivalent.
	return s.Unversioned().FinalSourceAddr(realSource)
}

// Matches: [hostname/]<namespace>/<component-name>[@version][//subpath]
var finalComponentSourcePattern = regexp.MustCompile(
   `^` +
	   // Optional hostname with domain suffix or no domain
	   `((?:[a-zA-Z0-9_.-]+(?:\.[a-zA-Z]{2,})?/)?[a-zA-Z0-9_-]+/[a-zA-Z0-9_-]+)` +
	   // Version requirement (after @)
	   `@([^/]+)` +
	   // Optional subpath (after //)
	   `(//(.+))?` +
   `$`)
