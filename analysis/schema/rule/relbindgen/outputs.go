package relbindgen

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// Outputs is the publication side of one emitted row. Like Inputs it names
// only what the sealed signature declared: the ordered output columns and the
// destination row already resolved for this emission. It carries no way to
// choose a relation, a scope, or a denominator.
type Outputs struct {
	declared []signature.Output
	buffer   *binding.ProposalBuffer
	issuer   binding.Issuer
	row      model.RowID
	presence model.Presence
}

// Len returns the number of declared output columns.
func (outputs Outputs) Len() int { return len(outputs.declared) }

// Presence returns the logical status this emission publishes. A generated
// encoder publishes a value only when the status carries one.
func (outputs Outputs) Presence() model.Presence { return outputs.presence }

// CarriesValue reports whether this emission publishes an encoded value.
func (outputs Outputs) CarriesValue() bool {
	return outputs.presence.Is(model.Present) || outputs.presence.Is(model.AuthenticatedOpaque)
}

// PutColumn publishes one declared output column of the emitted row.
func PutColumn[T any](outputs Outputs, index int, column *Column[T], value T) bool {
	declared, destination, ok := outputs.destination(index, column.Type())
	if !ok {
		return false
	}
	if !outputs.CarriesValue() || !declared.Presence.Allows(outputs.presence) {
		return false
	}
	token, ok := column.Encode(outputs.issuer, value)
	if !ok {
		return false
	}
	proposal, ok := binding.NewProposal(destination, token, outputs.presence)
	if !ok {
		return false
	}
	return outputs.buffer.Append(proposal)
}

// PutAbsentColumn publishes proven absence for one declared output column.
func PutAbsentColumn(outputs Outputs, index int) bool {
	declared, destination, ok := outputs.destination(index, model.TypeID{})
	if !ok || outputs.CarriesValue() || !declared.Presence.Allows(outputs.presence) {
		return false
	}
	proposal, ok := binding.NewProposal(destination, binding.ValueToken{}, outputs.presence)
	if !ok {
		return false
	}
	return outputs.buffer.Append(proposal)
}

func (outputs Outputs) destination(index int, typeID model.TypeID) (signature.Output, binding.CellToken, bool) {
	if index < 0 || index >= len(outputs.declared) || outputs.buffer == nil || !outputs.issuer.Available() {
		return signature.Output{}, binding.CellToken{}, false
	}
	declared := outputs.declared[index]
	if !declared.Available() {
		return signature.Output{}, binding.CellToken{}, false
	}
	if typeID.Available() && declared.Type != typeID {
		return signature.Output{}, binding.CellToken{}, false
	}
	if !outputs.row.Available() {
		return signature.Output{}, binding.CellToken{}, false
	}
	// ScalarSource and SpanSource identify an output coordinate through an
	// authenticated input row. RowID is nominally fenced by its relation, so
	// the source row itself can never be used as an output row when the
	// publication targets another relation. Preserve the owner-issued content
	// coordinate and issue the corresponding row under the signature-declared
	// output relation. OwnerNamed rows take the same path idempotently.
	destinationRow, ok := model.IssueRowID(declared.Relation, outputs.row.Content())
	if !ok {
		return signature.Output{}, binding.CellToken{}, false
	}
	witness, ok := outputs.buffer.DestinationWitness(declared.Denominator)
	if !ok {
		return signature.Output{}, binding.CellToken{}, false
	}
	destination, ok := outputs.issuer.IssueCell(witness, outputs.buffer.Scope(), declared.Column, destinationRow)
	if !ok {
		return signature.Output{}, binding.CellToken{}, false
	}
	return declared, destination, true
}
