package sourceaddrs

import (
	"fmt"
	"path"

	"github.com/apparentlymart/go-versions/versions"
	regaddr "github.com/hashicorp/terraform-registry-address"
)

type ComponentSource struct {
	pkg     regaddr.ComponentPackage
	subPath string
}

var _ Source = ComponentSource{}

// looksLikeComponentSource returns true if the given string can be parsed as a component source address.
func looksLikeComponentSource(given string) bool {
	_, err := regaddr.ParseComponentSource(given)
	return err == nil
}

// ParseComponentSource parses a string into a ComponentSource, splitting out any sub-path and normalizing it.
func ParseComponentSource(given string) (ComponentSource, error) {
	pkgRaw, subPathRaw := splitSubPath(given)
	subPath, err := normalizeSubpath(subPathRaw)
	if err != nil {
		return ComponentSource{}, fmt.Errorf("invalid sub-path: %w", err)
	}

	pkgOnlyAddr, err := regaddr.ParseComponentSource(pkgRaw)
	if err != nil {
		return ComponentSource{}, err
	}
	if pkgOnlyAddr.Subdir != "" {
		panic("post-split registry address still has subdir")
	}

	return ComponentSource{
		pkg:     pkgOnlyAddr.Package,
		subPath: subPath,
	}, nil
}

// ParseComponentPackage parses a string into a ComponentPackage, ensuring no sub-path is present.
func ParseComponentPackage(given string) (regaddr.ComponentPackage, error) {
	srcAddr, err := ParseComponentSource(given)
	if err != nil {
		return regaddr.ComponentPackage{}, err
	}
	if srcAddr.subPath != "" {
		return regaddr.ComponentPackage{}, fmt.Errorf("registry component package address may not have a sub-path")
	}
	return srcAddr.pkg, nil
}

func (s ComponentSource) sourceSigil() {}

// String returns the string representation of the ComponentSource, including sub-path if present.
func (s ComponentSource) String() string {
	if s.subPath != "" {
		return s.pkg.String() + "//" + s.subPath
	}
	return s.pkg.String()
}

// SupportsVersionConstraints indicates that ComponentSource always supports version constraints.
func (s ComponentSource) SupportsVersionConstraints() bool {
	return true
}

// Package returns the ComponentPackage part of the ComponentSource.
func (s ComponentSource) Package() regaddr.ComponentPackage {
	return s.pkg
}

// SubPath returns the sub-path portion of the ComponentSource.
func (s ComponentSource) SubPath() string {
	return s.subPath
}

// Versioned returns a ComponentSourceFinal with the selected version applied to the ComponentSource.
func (s ComponentSource) Versioned(selectedVersion versions.Version) ComponentSourceFinal {
	return ComponentSourceFinal{
		src:     s,
		version: selectedVersion,
	}
}

// FinalSourceAddr returns a RemoteSource with the sub-path from the ComponentSource, if present.
func (s ComponentSource) FinalSourceAddr(realSource RemoteSource) RemoteSource {
	if s.subPath == "" {
		return realSource
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
