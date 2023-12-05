// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

// This file contains some internal-only types used to help with marshalling
// and unmarshalling our manifest file format. The manifest format is not
// itself a public interface, so these should stay unexported and any caller
// that needs to interact with previously-generated source bundle manifests
// should do so via the Bundle type.

type manifestRoot struct {
	// FormatVersion should always be 1 for now, because there is only
	// one version of this format.
	FormatVersion uint64 `json:"terraform_source_bundle"`

	Packages     []manifestRemotePackage `json:"packages,omitempty"`
	RegistryMeta []manifestRegistryMeta  `json:"registry,omitempty"`
}

type manifestRemotePackage struct {
	// SourceAddr is the address of an entire remote package, meaning that
	// it must not have a sub-path portion.
	SourceAddr string `json:"source"`

	// LocalDir is the name of the subdirectory of the bundle containing the
	// source code for this package.
	LocalDir string `json:"local"`

	Meta manifestPackageMeta `json:"meta,omitempty"`
}

type manifestRegistryMeta struct {
	// SourceAddr is the address of an entire registry package, meaning that
	// it must not have a sub-path portion.
	SourceAddr string `json:"source"`

	// Versions is a map from string representations of [versions.Version].
	Versions map[string]manifestRegistryVersion `json:"versions,omitempty"`
}

type manifestRegistryVersion struct {
	// This SourceAddr is a full source address, so it might potentially
	// have a sub-path portion. If it does then it must be combined with
	// any sub-path included in the user's registry module source address.
	SourceAddr string `json:"source"`
}

type manifestPackageMeta struct {
	GitCommitID string `json:"git_commit_id,omitempty"`
}
