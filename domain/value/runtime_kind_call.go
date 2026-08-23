package value

import (
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
)

// RuntimeKindCall is Value's owner-fenced interpretation of one strict
// unary call geometry. Program owns the call occurrence and ingress exposes
// its sealed Call/CallArgument rows; Value retains only the two mounted
// coordinates needed by the runtime-kind transfer.
type RuntimeKindCall struct {
	schema     *Schema
	key        computationKey
	content    identity.ContentID
	result     Coordinate
	input      Coordinate
	comparison Coordinate
	write      Coordinate
	endpoints  uint32
	call       identity.ContentID
	op         flowkind.BinaryOp
	truth      bool
	refinement bool
}

// RuntimeKindCall resolves the Value-owned operand for one mounted call
// occurrence. The row is issued only during Schema sealing and is detached
// from the Program/ingress objects before publication.
func (schema *Schema) RuntimeKindCall(module, occurrence identity.ContentID) (RuntimeKindCall, bool) {
	if schema == nil || schema.runtimeKindCalls == nil {
		return RuntimeKindCall{}, false
	}
	row, ok := schema.runtimeKindCalls[computationKey{module: module, occurrence: occurrence}]
	return row, ok && row.valid()
}

func (row RuntimeKindCall) valid() bool {
	if row.schema == nil || !row.key.module.Available() || !row.key.occurrence.Available() || !row.content.Available() {
		return false
	}
	if row.refinement {
		return row.result.Valid() && row.input.Valid() && row.call.Available() && row.comparison.Valid() && row.write.Valid() &&
			(row.op == flowkind.BinaryEqual || row.op == flowkind.BinaryNotEqual)
	}
	return row.result.Valid() && row.input.Valid() && row.write.Valid()
}

// OwnsRuntimeKindCall reports whether row was issued by this exact Value
// Schema. Coordinates from an independently sealed Link must never mix.
func (schema *Schema) OwnsRuntimeKindCall(row RuntimeKindCall) bool {
	return schema != nil && row.schema == schema && row.valid()
}

func (row RuntimeKindCall) ID() (identity.ContentID, bool) {
	if !row.valid() {
		return identity.ContentID{}, false
	}
	return row.content, true
}

// Endpoints returns the call result coordinate and its sole actual argument
// coordinate. The identity join is exposed separately by CallOccurrence so
// coordinate consumers do not need to carry mount identity through every
// Value projection.
func (row RuntimeKindCall) Endpoints() (result, input Coordinate, ok bool) {
	if !row.valid() {
		return Coordinate{}, Coordinate{}, false
	}
	return row.result, row.input, true
}

// Refinement returns the optional guarded operation-predicate interpretation.
// The route certificate itself remains on the Artifact placement; Value owns
// only the equality polarity and branch truth needed to evaluate the sealed
// predicate against the comparison Value.
func (row RuntimeKindCall) Refinement() (comparison Coordinate, op flowkind.BinaryOp, truth bool, ok bool) {
	if !row.valid() || !row.refinement {
		return Coordinate{}, 0, false, false
	}
	return row.comparison, row.op, row.truth, true
}

// WriteTarget returns the exact Value coordinate this interpretation writes:
// the call result for the ordinary operation-result transfer, or the call's
// argument subject for a guarded predicate refinement.
func (row RuntimeKindCall) WriteTarget() (Coordinate, bool) {
	if !row.valid() {
		return Coordinate{}, false
	}
	return row.write, true
}

// CallOccurrence returns the mounted occurrence identity used to project the
// existing Call factor read. It is the exact parent-issued join, not a new
// call identity or a reconstructed Program key.
func (row RuntimeKindCall) CallOccurrence() (module, occurrence identity.ContentID, ok bool) {
	if !row.valid() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	occurrence = row.key.occurrence
	if row.refinement {
		occurrence = row.call
	}
	return row.key.module, occurrence, true
}
