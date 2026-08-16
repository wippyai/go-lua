package boundary

import "github.com/wippyai/go-lua/analysis/identity"

// CallTargetFormalKind is Boundary's closed distinction between an admitted
// Target seed and an external loader seed. Denied bootstrap values have no
// Call formal and therefore cannot enter Call's target vocabulary.
type CallTargetFormalKind uint8

const (
	CallTargetFormalInvalid CallTargetFormalKind = iota
	CallTargetFormalSeed
	CallTargetFormalExternal
)

// CallTargetFormal is the Boundary-owned portable identity of one admitted
// external Call target. It contains no dense seed ordinal or live authority.
type CallTargetFormal struct {
	kind CallTargetFormalKind
	id   identity.ContentID
}

func (formal CallTargetFormal) Valid() bool {
	return (formal.kind == CallTargetFormalSeed || formal.kind == CallTargetFormalExternal) && formal.id.Available()
}
func (formal CallTargetFormal) Kind() CallTargetFormalKind { return formal.kind }
func (formal CallTargetFormal) ID() (identity.ContentID, bool) {
	if !formal.Valid() {
		return identity.ContentID{}, false
	}
	return formal.id, true
}

// CallTarget projects one admitted Boundary seed into its explicit formal.
// Operation and loader seeds remain distinct formal kinds; denied bootstrap
// rows are intentionally rejected.
func (s Seeds) CallTarget(seed Seed) (CallTargetFormal, bool) {
	if !s.valid(seed) {
		return CallTargetFormal{}, false
	}
	id, ok := s.ID(seed)
	if !ok {
		return CallTargetFormal{}, false
	}
	if _, loader := s.Loader(seed); loader {
		formal := CallTargetFormal{kind: CallTargetFormalExternal, id: id}
		return formal, formal.Valid()
	}
	if _, operation := s.Operation(seed); operation {
		formal := CallTargetFormal{kind: CallTargetFormalSeed, id: id}
		return formal, formal.Valid()
	}
	return CallTargetFormal{}, false
}
