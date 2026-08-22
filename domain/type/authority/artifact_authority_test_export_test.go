package typeauthority

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// ResolveForTest exposes the construction resolver only to black-box owner
// laws. Production consumers must use ReferenceProjection.
func ResolveForTest(owner any, coordinate any) (typ.Type, bool) {
	var artifact *artifactAuthority
	var id identity.ContentID
	switch authority := owner.(type) {
	case *artifactAuthority:
		artifact = authority
		id, _ = coordinate.(identity.ContentID)
	case *Authority:
		ref, ok := coordinate.(StaticTypeRef)
		if !ok {
			return nil, false
		}
		selector, ok := authority.Lookup(ref)
		if !ok {
			return nil, false
		}
		entry, ok := authority.entry(selector)
		if !ok || !entry.projection.valid() {
			return nil, false
		}
		artifact, id = authority.artifact, ref.NodeID()
	default:
		return nil, false
	}
	if artifact == nil || !id.Available() {
		return nil, false
	}
	resolver := &artifactResolver{authority: artifact, built: make(map[identity.ContentID]typ.Type)}
	value, ok := resolver.resolve(id)
	return value, ok && value != nil
}

// SealProgramsForTest exposes the detached construction boundary only to its
// black-box laws. Production constructs it solely through SealProgramRows.
func SealProgramsForTest(programs []programschema.Program) (*artifactAuthority, error) {
	return sealPrograms(programs, false)
}
