// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package sourceaddrs

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type RemoteSource struct {
	pkg     RemotePackage
	subPath string
}

var _ Source = RemoteSource{}
var _ FinalSource = RemoteSource{}

// sourceSigil implements Source
func (RemoteSource) sourceSigil() {}

// finalSourceSigil implements FinalSource
func (RemoteSource) finalSourceSigil() {}

// ParseRemoteSource parses the given string as a remote source address,
// or returns an error if it does not use the correct syntax for interpretation
// as a remote source address.
func ParseRemoteSource(given string) (RemoteSource, error) {
	expandedGiven := given
	for _, shorthand := range remoteSourceShorthands {
		replacement, ok, err := shorthand(given)
		if err != nil {
			return RemoteSource{}, err
		}
		if ok {
			expandedGiven = replacement
		}
	}

	pkgRaw, subPathRaw := splitSubPath(expandedGiven)
	subPath, err := normalizeSubpath(subPathRaw)
	if err != nil {
		return RemoteSource{}, fmt.Errorf("invalid sub-path: %w", err)
	}

	// Once we've dealt with all the "shorthand" business, our address
	// should be in the form sourcetype::url, where "sourcetype::" is
	// optional and defaults to matching the URL scheme if not present.
	var sourceType string
	if matches := remoteSourceTypePattern.FindStringSubmatch(pkgRaw); len(matches) != 0 {
		sourceType = matches[1]
		pkgRaw = matches[2]
	}

	u, err := url.Parse(pkgRaw)
	if err != nil {
		return RemoteSource{}, fmt.Errorf("invalid URL syntax in %q: %w", pkgRaw, err)
	}
	if u.Scheme == "" {
		return RemoteSource{}, fmt.Errorf("must contain an absolute URL with a scheme")
	}
	if u.User != nil {
		return RemoteSource{}, fmt.Errorf("must not use username or password in URL portion")
	}

	u.Scheme = strings.ToLower(u.Scheme)
	sourceType = strings.ToLower(sourceType)

	if sourceType == "" {
		// sourceType defaults to the URL scheme if not explicitly set.
		sourceType = u.Scheme
	} else if sourceType == u.Scheme {
		// This catches weirdo constructions like: https::https://example.com/
		return RemoteSource{}, fmt.Errorf("don't specify redundant %q source type for %q URL", sourceType, u.Scheme)
	}

	_, err = url.ParseQuery(u.RawQuery)
	if err != nil {
		return RemoteSource{}, fmt.Errorf("invalid URL query string syntax in %q: %w", pkgRaw, err)
	}

	return makeRemoteSource(sourceType, u, subPath)
}

// MakeRemoteSource constructs a [RemoteSource] from its component parts.
//
// This is useful for deriving one remote source from another, by disassembling
// the original address into its component parts, modifying those parts, and
// then combining the modified parts back together with this function.
func MakeRemoteSource(sourceType string, u *url.URL, subPath string) (RemoteSource, error) {
	var err error
	subPath, err = normalizeSubpath(subPath)
	if err != nil {
		return RemoteSource{}, fmt.Errorf("invalid sub-path: %w", err)
	}

	copyU := *u // shallow copy so we can safely modify

	return makeRemoteSource(sourceType, &copyU, subPath)
}

func makeRemoteSource(sourceType string, u *url.URL, subPath string) (RemoteSource, error) {
	typeImpl, ok := remoteSourceTypes[sourceType]
	if !ok {
		if sourceType == u.Scheme {
			// In this case the user didn't actually specify a source type,
			// so we won't confuse them by mentioning it.
			return RemoteSource{}, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
		} else {
			return RemoteSource{}, fmt.Errorf("unsupported package source type %q", sourceType)
		}
	}

	err := typeImpl.PrepareURL(u)
	if err != nil {
		return RemoteSource{}, err
	}

	return RemoteSource{
		pkg: RemotePackage{
			sourceType: sourceType,
			url:        *u,
		},
		subPath: subPath,
	}, nil
}

// String implements Source
func (s RemoteSource) String() string {
	return s.pkg.subPathString(s.subPath)
}

func (s RemoteSource) SupportsVersionConstraints() bool {
	return false
}

func (s RemoteSource) Package() RemotePackage {
	return s.pkg
}

func (s RemoteSource) SubPath() string {
	return s.subPath
}

type remoteSourceShorthand func(given string) (normed string, ok bool, err error)

var remoteSourceShorthands = []remoteSourceShorthand{
	func(given string) (string, bool, error) {
		// Allows a github.com repository to be presented in a scheme-less
		// format like github.com/organization/repository/path, which we'll
		// turn into a git:: source string selecting the repository's main
		// branch.
		//
		// This is intentionally compatible with what's accepted by the
		// "GitHub detector" in the go-getter library, so that module authors
		// can specify GitHub repositories in the same way both for the
		// old Terraform module installer and the newer source bundle builder.

		if !strings.HasPrefix(given, "github.com/") {
			return "", false, nil
		}

		parts := strings.Split(given, "/")
		if len(parts) < 3 {
			return "", false, fmt.Errorf("GitHub.com shorthand addresses must start with github.com/organization/repository")
		}

		urlStr := "https://" + strings.Join(parts[:3], "/")
		if !strings.HasSuffix(urlStr, "git") {
			urlStr += ".git"
		}

		if len(parts) > 3 {
			// The remaining parts will become the sub-path portion, since
			// the repository as a whole is the source package.
			urlStr += "//" + strings.Join(parts[3:], "/")
		}

		return "git::" + urlStr, true, nil
	},
	func(given string) (string, bool, error) {
		// Allows a gitlab.com repository to be presented in a scheme-less
		// format like gitlab.com/organization/repository/path, which we'll
		// turn into a git:: source string selecting the repository's main
		// branch.
		//
		// This is intentionally compatible with what's accepted by the
		// "GitLab detector" in the go-getter library, so that module authors
		// can specify GitHub repositories in the same way both for the
		// old Terraform module installer and the newer source bundle builder.

		if !strings.HasPrefix(given, "gitlab.com/") {
			return "", false, nil
		}

		parts := strings.Split(given, "/")
		if len(parts) < 3 {
			return "", false, fmt.Errorf("GitLab.com shorthand addresses must start with gitlab.com/organization/repository")
		}

		urlStr := "https://" + strings.Join(parts[:3], "/")
		if !strings.HasSuffix(urlStr, "git") {
			urlStr += ".git"
		}

		if len(parts) > 3 {
			// The remaining parts will become the sub-path portion, since
			// the repository as a whole is the source package.
			urlStr += "//" + strings.Join(parts[3:], "/")
			// NOTE: We can't actually get here if there are exactly four
			// parts, because gitlab.com is also a Terraform module registry
			// and so gitlab.com/a/b/c must be interpreted as a registry
			// module address instead of a GitLab repository address. Users
			// must write an explicit git source address if they intend to
			// refer to a Git repository.
		}

		return "git::" + urlStr, true, nil
	},
}

var remoteSourceTypePattern = regexp.MustCompile(`^([A-Za-z0-9]+)::(.+)$`)
