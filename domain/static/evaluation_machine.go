package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/authority"
)

// evaluate consumes the type owner's sealed projection and records the exact
// namespace-scoped result. It owns no evaluator state machine or placeholder
// context dimensions.
func (a *Authority) evaluate(ref typeauthority.StaticTypeRef, namespace identity.ContentID) (Value, error) {
	if a == nil || a.types == nil || !ref.Valid() || !namespace.Available() {
		return Value{}, errors.New("static: foreign evaluation coordinate")
	}
	projection, ok := a.types.Projection(ref)
	if !ok {
		return Value{}, errors.New("static: artifact reference projection unavailable")
	}
	if projection.Open() {
		site := Symbolic{reference: ref, sourceOwner: ref.Owner(), source: ref.NodeID(), namespace: namespace, law: a.lawID, dependency: ref.Owner(), reason: ReasonOpenFormal}
		return a.addSymbolic(site)
	}
	input, inputOK := projection.ClosedInput()
	if !inputOK {
		return Value{}, errors.New("static: closed reference input unavailable")
	}
	return a.addClosedInput(input)
}
