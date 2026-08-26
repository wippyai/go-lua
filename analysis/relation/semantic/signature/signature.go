package signature

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
)

// Signature is an immutable sealed operation contract.
type Signature struct {
	identity    Identity
	fence       Fence
	inputs      []Input
	outputs     []Output
	cardinality model.Cardinality
	outcomes    outcome.Set
	digest      identity.ContentID
}

// Seal freezes and copies the ABI vocabulary. Full validity, including
// malformed references and cross-contract compatibility, belongs to the
// independent checker.
func Seal(spec Spec) (Signature, bool) {
	outputs := append([]Output(nil), spec.Outputs...)
	for _, output := range outputs {
		if !output.Available() {
			return Signature{}, false
		}
	}
	sealed := Signature{
		identity: spec.Identity, fence: spec.Fence,
		inputs: append([]Input(nil), spec.Inputs...), outputs: outputs,
		cardinality: spec.Cardinality,
		outcomes:    copySet(spec.Outcomes),
	}
	sealed.digest = digest(sealed)
	return sealed, sealed.digest.Available()
}

func copySet(set outcome.Set) outcome.Set {
	copyOf, _ := outcome.NewSet(set.Codes()...)
	return copyOf
}

func (signatureValue Signature) Available() bool { return signatureValue.digest.Available() }

func (signatureValue Signature) Digest() identity.ContentID { return signatureValue.digest }

func (signatureValue Signature) Identity() Identity { return signatureValue.identity }

func (signatureValue Signature) Fence() Fence { return signatureValue.fence }

func (signatureValue Signature) Inputs() []Input {
	return append([]Input(nil), signatureValue.inputs...)
}

func (signatureValue Signature) InputLen() int { return len(signatureValue.inputs) }

func (signatureValue Signature) Outputs() []Output {
	return append([]Output(nil), signatureValue.outputs...)
}

func (signatureValue Signature) OutputLen() int { return len(signatureValue.outputs) }

func (signatureValue Signature) InputAt(index int) (Input, bool) {
	if !signatureValue.Available() || index < 0 || index >= len(signatureValue.inputs) {
		return Input{}, false
	}
	return signatureValue.inputs[index], true
}

func (signatureValue Signature) OutputFor(relation model.RelationID, column model.ColumnID) (Output, bool) {
	for _, output := range signatureValue.outputs {
		if output.Relation == relation && output.Column == column {
			return output, true
		}
	}
	return Output{}, false
}

// OutputDestination returns the exact denominator owned by one declared
// output.
func (signatureValue Signature) OutputDestination(relation model.RelationID, column model.ColumnID) (model.DenominatorRef, bool) {
	output, ok := signatureValue.OutputFor(relation, column)
	if !ok || !output.Denominator.Available() {
		return model.DenominatorRef{}, false
	}
	return output.Denominator, true
}

func (signatureValue Signature) Cardinality() model.Cardinality { return signatureValue.cardinality }

func (signatureValue Signature) Outcomes() outcome.Set { return copySet(signatureValue.outcomes) }

func (signatureValue Signature) Allows(code outcome.Code) bool {
	return signatureValue.outcomes.Contains(code)
}

func (signatureValue Signature) AllowsDestination(relation model.RelationID, column model.ColumnID) bool {
	_, ok := signatureValue.OutputFor(relation, column)
	return ok
}
