// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourcebundle

// PackageMeta is a collection of metadata about how the content of a
// particular remote package was derived.
//
// A nil value of this type represents no metadata. A non-nil value will
// typically omit some or all of the fields if they are not relevant.
type PackageMeta struct {
	// NOTE: Everything in here is unexported for now because it's not clear
	// how this is going to evolve in future and whether it's a good idea
	// to just have a separate field for each piece of metadata. This will
	// give some freedom to switch to other storage strategies in future if
	// this struct ends up getting too big and is only sparsely used by most
	// fetchers.

	gitCommitID string
}

// PackageMetaWithGitCommit returns a [PackageMeta] object with a Git Commit
// ID tracked. The given commit ID must be a fully-qualified ID, and never an
// abbreviated commit ID, the name of a ref, or anything other proxy-for-commit
// identifier.
func PackageMetaWithGitCommit(commitID string) *PackageMeta {
	return &PackageMeta{
		gitCommitID: commitID,
	}
}

// If the content of this package was derived from a particular commit
// from a Git repository, GitCommitID returns the fully-qualified ID of
// that commit. This is never an abbreviated commit ID, the name of a ref,
// or anything else that could serve as a proxy for a commit ID.
//
// If there is no relevant commit ID for this package, returns an empty string.
func (m *PackageMeta) GitCommitID() string {
	return m.gitCommitID
}
