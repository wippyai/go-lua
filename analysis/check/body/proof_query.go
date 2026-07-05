package body

import (
	"github.com/wippyai/go-lua/analysis/check/internal/projection"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func (r *Result) proofReader() proof.Reader {
	if r == nil {
		return proof.New(nil, nil)
	}
	return proof.New(r.Registry(), r.typeValues)
}

func (r *Result) ValueType(value product.Value) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	return r.proofReader().ValueType(value)
}

// RuntimeKindReducedType narrows declared by value's runtime-kind axis: the
// alternatives whose runtime kind the axis excludes are dropped. This reports
// the type a value actually holds on a path that a type() guard has narrowed.
func (r *Result) RuntimeKindReducedType(value product.Value, declared typ.Type) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	return r.proofReader().RuntimeKindReducedType(value, declared)
}

func (r *Result) ValueHasUntrustedTopOrigin(value product.Value) bool {
	return r != nil && r.proofReader().ValueHasUntrustedTopOrigin(value)
}

func (r *Result) ValueHasExplicitTopOrigin(value product.Value) bool {
	return r != nil && r.proofReader().ValueHasExplicitTopOrigin(value)
}

func (r *Result) ValueTypeWithPresence(value product.Value) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	return projection.ValueTypeWithPresence(r.registry, r.typeValues, value)
}

func (r *Result) ValueHasExactIdentity(value product.Value) bool {
	return r != nil && r.proofReader().ValueHasExactIdentity(value)
}

// ValueHasStackLocalExactIdentity reports whether value is a concrete table
// identity that remains confined to the current activation at point.
func (r *Result) ValueHasStackLocalExactIdentity(point cfg.Point, value product.Value) bool {
	if r == nil || r.Registry() == nil {
		return false
	}
	st, ok := r.StateAtBoundary(point)
	return ok && st.ValueHasStackLocalExactIdentity(r.Registry(), value)
}

// ValueHasLocalExclusiveExactIdentity reports whether value is a concrete table
// identity whose placement proves no external writer can materialize a missing
// slot at point.
func (r *Result) ValueHasLocalExclusiveExactIdentity(point cfg.Point, value product.Value) bool {
	if r == nil || r.Registry() == nil {
		return false
	}
	st, ok := r.StateAtBoundary(point)
	return ok && st.ValueHasLocalExclusiveExactIdentity(r.Registry(), value)
}

func (r *Result) VariantOriginType(value product.Value) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	return r.proofReader().VariantOriginType(value)
}

// FullVariantOriginType returns the complete structural union of the value's
// variant-origin family, independent of any narrowing recorded on this value.
func (r *Result) FullVariantOriginType(value product.Value) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	return r.proofReader().FullVariantOriginType(value)
}

func (r *Result) RefineDeclaredType(declared typ.Type, value product.Value) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	return r.proofReader().RefineDeclaredType(declared, value)
}

// NarrowDeclaredByOrigin narrows declared by the value's variant-origin facts
// and presence only, without substituting any structural type witness.
func (r *Result) NarrowDeclaredByOrigin(declared typ.Type, value product.Value) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	return r.proofReader().NarrowDeclaredByOrigin(declared, value)
}

func (r *Result) ValueAdmissible(value product.Value, want typ.Type) bool {
	return r != nil && r.proofReader().ValueAdmissible(value, want)
}

// ValueWitnessProvenMismatch reports whether a value carries a concrete
// (non-top) type witness whose presence-adjusted form is provably not a subtype
// of want.
func (r *Result) ValueWitnessProvenMismatch(value product.Value, want typ.Type) bool {
	return r != nil && r.proofReader().ValueWitnessProvenMismatch(value, want)
}

func (r *Result) ValueProofAdmissible(value product.Value, want typ.Type) bool {
	return r != nil && r.proofReader().ValueProofAdmissible(value, want)
}

func (r *Result) IsSubtype(sub, super typ.Type) bool {
	if r == nil {
		return proof.New(nil, nil).IsSubtype(sub, super)
	}
	return proof.New(nil, r.typeValues).IsSubtype(sub, super)
}

func ProjectionWithoutNil(t typ.Type) typ.Type {
	return proof.ProjectionWithoutNil(t)
}
