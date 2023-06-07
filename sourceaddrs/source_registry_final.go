package sourceaddrs

import (
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

// finalSourceSigil implements FinalSource
func (s RegistrySourceFinal) finalSourceSigil() {}

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
