// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

import (
	"context"

	"github.com/apparentlymart/go-versions/versions"
	"github.com/hashicorp/go-slug/sourceaddrs"
	regaddr "github.com/hashicorp/terraform-registry-address"
)

// RegistryClient provides a minimal client for the Terraform module registry
// protocol, sufficient to find the available versions for a particular
// registry entry and then to find the real remote package for a particular
// version.
//
// An implementation should not itself attempt to cache the direct results of
// the client methods, but it can (and probably should) cache prerequisite
// information such as the results of performing service discovery against
// the hostname in a module package address.
type RegistryClient interface {
	// ModulePackageVersions fetches all of the known exact versions
	// available for the given package in its module registry.
	ModulePackageVersions(ctx context.Context, pkgAddr regaddr.ModulePackage) (ModulePackageVersionsResponse, error)

	// ModulePackageSourceAddr fetches the real remote source address for the
	// given version of the given module registry package.
	ModulePackageSourceAddr(ctx context.Context, pkgAddr regaddr.ModulePackage, version versions.Version) (ModulePackageSourceAddrResponse, error)
}

// ModulePackageVersionsResponse is an opaque type which represents the result
// of the package versions client operation. This type may grow to add more
// functionality over time in later minor releases.
type ModulePackageVersionsResponse struct {
	Versions versions.List
}

// ModulePackageSourceAddrResponse is an opaque type which represents the
// result of the source address client operation. This type may grow to add
// more functionality over time in later minor releases.
type ModulePackageSourceAddrResponse struct {
	SourceAddr sourceaddrs.RemoteSource
}
