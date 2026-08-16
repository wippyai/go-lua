package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// PreferredLocalAssignmentSourceValue selects the value that should represent a
// local assignment source when both the lowered boundary source and the generic
// expression read are available. This is proof selection, not presentation:
// body owns the subtype/equivalence decision.
func (r *Result) PreferredLocalAssignmentSourceValue(lowered, generic product.Value) product.Value {
	if r.LocalAssignmentGenericSourceValueMorePrecise(lowered, generic) {
		return generic
	}
	return lowered
}

// LocalAssignmentGenericSourceValueMorePrecise reports whether the generic
// expression read carries the same non-nil type as a nilable lowered source and
// should therefore preserve the tighter proof. A lowered value that is Any/
// Unknown because it is an explicit declared-any contract is not a proof gap:
// it is preferred over the generic read exactly like a proof-bearing lowered
// type is, so an explicit `local x: any` boundary never gets laundered away by
// a sharper-looking but untagged generic read.
func (r *Result) LocalAssignmentGenericSourceValueMorePrecise(lowered, generic product.Value) bool {
	loweredType, loweredOK := r.ValueTypeWithPresence(lowered)
	genericType, genericOK := r.ValueTypeWithPresence(generic)
	if !genericOK || genericType == nil || typ.IsNever(genericType) {
		return false
	}
	if !loweredOK || loweredType == nil {
		return true
	}
	if typ.IsAny(loweredType) || typ.IsUnknown(loweredType) {
		return !r.ValueHasExplicitTopOrigin(lowered)
	}
	withoutNil := ProjectionWithoutNil(loweredType)
	if withoutNil == nil || typ.IsNever(withoutNil) || typ.TypeEquals(withoutNil, loweredType) {
		return false
	}
	return r.IsSubtype(genericType, withoutNil) && r.IsSubtype(withoutNil, genericType)
}

// ExplanationValueShouldReplaceAssignmentSource reports whether explanatory
// recovery found a clearer concrete witness than the primary assignment source.
func (r *Result) ExplanationValueShouldReplaceAssignmentSource(base, explanation product.Value) bool {
	if r == nil || r.Registry() == nil {
		return false
	}
	if !r.ValueHasUntrustedTopOrigin(explanation) && !r.ValueHasExplicitTopOrigin(explanation) {
		return false
	}
	if !r.ValueHasReadableConcreteWitness(explanation) {
		return false
	}
	return r.ValueHasUntrustedTopOrigin(base) ||
		r.ValueHasExplicitTopOrigin(base) ||
		!r.valueHasReadableType(base)
}

func (r *Result) valueHasReadableType(value product.Value) bool {
	t, ok := r.ValueType(value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}
