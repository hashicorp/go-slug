// Copyright IBM Corp. 2018, 2025
// SPDX-License-Identifier: MPL-2.0

package version

import "github.com/hashicorp/go-slug/version"

var (
	Version           = "1.8.1"
	VersionPrerelease = "dev"
	VersionMetadata   = ""
	PluginVersion     = version.NewPluginVersion(Version, VersionPrerelease, VersionMetadata)
)
