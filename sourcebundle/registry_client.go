// Copyright IBM Corp. 2018, 2025
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

import (
	"context"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/hashicorp/go-slug/sourceaddrs"
	regaddr "github.com/hashicorp/terraform-registry-address"
)

// RegistryClient provides a minimal client for the Terraform registry
// protocol, sufficient to find the available versions for a particular
// registry entry (module or component) and then to find the real remote
// package for a particular version.
//
// An implementation should not itself attempt to cache the direct results of
// the client methods, but it can (and probably should) cache prerequisite
// information such as the results of performing service discovery against
// the hostname in a package address.
type RegistryClient interface {
	// ModulePackageVersions fetches all of the known exact versions
	// available for the given package in its module registry.
	ModulePackageVersions(ctx context.Context, pkgAddr regaddr.ModulePackage) (ModulePackageVersionsResponse, error)

	// ModulePackageSourceAddr fetches the real remote source address for the
	// given version of the given module registry package.
	ModulePackageSourceAddr(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version) (ModulePackageSourceAddrResponse, error)

	// ComponentPackageVersions fetches all of the known exact versions
	// available for the given package in its component registry.
	ComponentPackageVersions(ctx context.Context, pkgAddr regaddr.ComponentPackage) (ComponentPackageVersionsResponse, error)

	// ComponentPackageSourceAddr fetches the real remote source address for the
	// given version of the given component registry package.
	ComponentPackageSourceAddr(ctx context.Context, pkgAddr regaddr.ComponentPackage, version versions.Version) (ComponentPackageSourceAddrResponse, error)
}

// ModulePackageVersionsResponse is an opaque type which represents the result
// of the package versions client operation. This type may grow to add more
// functionality over time in later minor releases.
type ModulePackageVersionsResponse struct {
	Versions []ModulePackageInfo `json:"versions"`
}

type ModulePackageInfo struct {
	Version     versions.Version
	Deprecation *ModulePackageVersionDeprecation `json:"deprecation"`
}

type ModulePackageVersionDeprecation struct {
	Reason string `json:"reason"`
	Link   string `json:"link"`
}

// ModulePackageSourceAddrResponse is an opaque type which represents the
// result of the source address client operation. This type may grow to add
// more functionality over time in later minor releases.
type ModulePackageSourceAddrResponse struct {
	SourceAddr sourceaddrs.RemoteSource
}

// ComponentPackageVersionsResponse is an opaque type which represents the result
// of the component package versions client operation. This type may grow to add more
// functionality over time in later minor releases.
type ComponentPackageVersionsResponse struct {
	Versions []ComponentPackageInfo `json:"versions"`
}

type ComponentPackageInfo struct {
	Version     versions.Version
	Deprecation *ComponentPackageVersionDeprecation `json:"deprecation"`
}

type ComponentPackageVersionDeprecation struct {
	Reason string `json:"reason"`
	Link   string `json:"link"`
}

// ComponentPackageSourceAddrResponse is an opaque type which represents the
// result of the component source address client operation. This type may grow to add
// more functionality over time in later minor releases.
type ComponentPackageSourceAddrResponse struct {
	SourceAddr sourceaddrs.RemoteSource
}
