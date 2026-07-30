package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ForEachNonNilAssertion visits runtime non-nil assertions in deterministic RPO
// order, projecting each operand through solved boundary state. The obligation
// pass owns deciding whether the operand is provably nil-only.
func (r Reader) ForEachNonNilAssertion(visit func(NonNilAssertion) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	return r.result.ForEachNonNilAssertionOccurrence(func(occ body.NonNilAssertionOccurrence) bool {
		item := NonNilAssertion{
			Point:            occ.Point,
			OperandLabel:     occ.OperandLabel,
			OperandKey:       occ.OperandKey,
			Value:            occ.Value,
			ValueHash:        assignmentValueHash(r, occ.Value, occ.HasValue),
			TypeWithPresence: occ.TypeWithPresence,
			OperandNilOnly:   readapi.NonNilAssertionOperandNilOnly(occ.TypeWithPresence),
			OperandSpan:      sourceSpanFromBody(occ.OperandSpan),
			AssertionSpan:    sourceSpanFromBody(occ.AssertionSpan),
		}
		visited = true
		return visit(item)
	}) || visited
}

// ForEachConcatOperand visits `..` operands whose solved projection still
// includes nil in deterministic RPO order.
func (r Reader) ForEachConcatOperand(visit func(ConcatOperand) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	return r.result.ForEachConcatOperandOccurrence(func(occ body.ConcatOperandOccurrence) bool {
		value, _ := r.result.ExpressionValueAtBoundary(occ.Point, occ.Operand)
		provenance := r.nilabilityProvenance(occ.Point, occ.Operand, value)
		if typ.IsAny(occ.TypeWithPresence) || typ.IsUnknown(occ.TypeWithPresence) {
			provenance.UntrustedTopOrigin = true
		}
		item := ConcatOperand{
			Point:            occ.Point,
			Side:             occ.Side,
			OperandLabel:     occ.OperandLabel,
			OperandKey:       occ.OperandKey,
			TypeWithPresence: occ.TypeWithPresence,
			OperandSpan:      sourceSpanFromBody(occ.OperandSpan),
			Nilability:       provenance,
		}
		visited = true
		return visit(item)
	}) || visited
}

// ForEachNumericForOperand visits the init, limit, and explicit step operands of
// numeric-for loops in deterministic RPO order.
func (r Reader) ForEachNumericForOperand(visit func(NumericForOperand) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	return r.result.ForEachNumericForOperandOccurrence(func(occ body.NumericForOperandOccurrence) bool {
		item := NumericForOperand{
			Point:               occ.Point,
			Role:                occ.Role,
			OperandLabel:        occ.OperandLabel,
			OperandKey:          occ.OperandKey,
			TypeWithPresence:    occ.TypeWithPresence,
			OperandSpan:         sourceSpanFromBody(occ.OperandSpan),
			ExplicitTopLikeCast: occ.ExplicitTopLikeCast,
			DefinitelyNotNumber: readapi.NumericForDefinitelyNotNumber(occ.TypeWithPresence),
		}
		visited = true
		return visit(item)
	}) || visited
}
