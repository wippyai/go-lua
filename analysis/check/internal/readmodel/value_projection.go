package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/internal/sourcebridge"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ValueHash returns the product-domain hash used for stable value references.
func (r Reader) ValueHash(value product.Value) uint64 {
	if r.result == nil {
		return 0
	}
	return product.Hash(r.result.Registry(), value)
}

func (r Reader) SourceValue(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	switch source.Kind {
	case sourceprovenance.SourceExpression:
		if source.Expr == nil {
			return product.Value{}, false
		}
		if value, ok := r.result.ExpressionValueAtBoundary(point, source.Expr); ok {
			if !r.valueHasReadableType(value) {
				if p, ok := r.result.ExpressionPath(source.Expr); ok && !p.IsEmpty() {
					if pathValue, pathOK := r.result.PathValueAtBoundary(point, p); pathOK {
						return pathValue, true
					}
				}
			}
			return value, true
		}
		_, proofTransparent := sourceprovenance.ProofInner(source.Expr)
		if !sourceprovenance.ProofInnerIsFunction(source.Expr) && proofTransparent {
			return product.Value{}, false
		}
		if value, ok := r.result.LocalAssignmentSourceValueForExplanationAtBoundary(point, source); ok {
			return value, true
		}
		if p, ok := r.result.ExpressionPath(source.Expr); ok && !p.IsEmpty() {
			return r.result.PathValueAtBoundary(point, p)
		}
		return product.Value{}, false
	case sourceprovenance.SourceCall:
		if source.Expr != nil && source.ResultIndex == 0 {
			if value, ok := r.result.ExpressionValueAtBoundary(point, source.Expr); ok {
				return value, true
			}
		}
		if value, ok := r.result.LocalAssignmentSourceValueForExplanationAtBoundary(point, source); ok {
			return value, true
		}
		valueSource, ok := sourcebridge.ValueSourceFromASTSource(source)
		if !ok {
			return product.Value{}, false
		}
		return r.result.SourceValueForExplanationAtBoundary(point, valueSource)
	case sourceprovenance.SourceVararg, sourceprovenance.SourceNil, sourceprovenance.SourceUnknown:
		if value, ok := r.result.LocalAssignmentSourceValueForExplanationAtBoundary(point, source); ok {
			return value, true
		}
		valueSource, ok := sourcebridge.ValueSourceFromASTSource(source)
		if !ok {
			return product.Value{}, false
		}
		return r.result.SourceValueForExplanationAtBoundary(point, valueSource)
	default:
		return product.Value{}, false
	}
}

func (r Reader) valueHasReadableType(value product.Value) bool {
	t, ok := r.ValueType(value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t)
}

func (r Reader) SourceType(point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	value, ok := r.SourceValue(point, source)
	if !ok {
		return nil, false
	}
	return r.ValueType(value)
}

func (r Reader) ValueType(value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.ValueType(value)
}

// RuntimeKindReducedType narrows declared by value's runtime-kind axis: the
// alternatives whose runtime kind the axis excludes are dropped. This reports
// the type a value actually holds on a path that a type() guard has narrowed
// (e.g. the else edge of type(v) == "number" makes a number | string value a
// string), which the declared witness alone does not reflect.
func (r Reader) RuntimeKindReducedType(value product.Value, declared typ.Type) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.RuntimeKindReducedType(value, declared)
}

func (r Reader) ValueHasUntrustedTopOrigin(value product.Value) bool {
	if r.result == nil {
		return false
	}
	return r.result.ValueHasUntrustedTopOrigin(value)
}

func (r Reader) ValueHasExplicitTopOrigin(value product.Value) bool {
	if r.result == nil {
		return false
	}
	return r.result.ValueHasExplicitTopOrigin(value)
}

func (r Reader) ValueTypeWithPresence(value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.ValueTypeWithPresence(value)
}

// ValueHasExactIdentity reports whether value carries an exact identity lane,
// which a freshly constructed table literal holds and an imported or opaque
// value does not. It distinguishes a locally-built table whose witness type is
// complete from a value whose modeled type may omit reachable members.
func (r Reader) ValueHasExactIdentity(value product.Value) bool {
	if r.result == nil {
		return false
	}
	return r.result.ValueHasExactIdentity(value)
}

// ValueHasStackLocalExactIdentity reports whether value is a concrete table
// identity that remains confined to the current activation at point. Only this
// placement makes an absent field witness complete enough to type the read as
// exactly nil; escaped or shared identities can gain members through aliases or
// callbacks and must fall back to broader evidence.
func (r Reader) ValueHasStackLocalExactIdentity(point cfg.Point, value product.Value) bool {
	if r.result == nil || r.result.Registry() == nil {
		return false
	}
	return r.result.ValueHasStackLocalExactIdentity(point, value)
}

// ValueHasLocalExclusiveExactIdentity reports whether value is a concrete table
// identity whose placement proves no external writer can materialize a missing
// slot at point. Stack values and owned-heap values are local-exclusive; shared
// or unknown placements are not.
func (r Reader) ValueHasLocalExclusiveExactIdentity(point cfg.Point, value product.Value) bool {
	if r.result == nil || r.result.Registry() == nil {
		return false
	}
	return r.result.ValueHasLocalExclusiveExactIdentity(point, value)
}

func (r Reader) VariantOriginType(value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.VariantOriginType(value)
}

// FullVariantOriginType returns the complete structural union of the value's
// variant-origin family, independent of any narrowing recorded on this value.
// It yields the broad declared shape the discriminated value originated from.
func (r Reader) FullVariantOriginType(value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.FullVariantOriginType(value)
}

func (r Reader) RefineDeclaredType(declared typ.Type, value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.RefineDeclaredType(declared, value)
}

// NarrowDeclaredByOrigin narrows declared by the value's variant-origin facts
// and presence only, without substituting any structural type witness. The
// result stays a sound supertype of the runtime value: discriminant narrowing
// removes union arms but never drops declared fields the way a partial observed
// table-literal witness can.
func (r Reader) NarrowDeclaredByOrigin(declared typ.Type, value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.NarrowDeclaredByOrigin(declared, value)
}

func (r Reader) ValueAdmissible(value product.Value, want typ.Type) bool {
	if r.result == nil {
		return false
	}
	return r.result.ValueAdmissible(value, want)
}

// ValueWitnessProvenMismatch reports whether a value carries a concrete
// (non-top) type witness whose presence-adjusted form is provably not a subtype
// of want. It signals a proven contradiction, not a gradual one: a value with no
// concrete witness (genuinely unknown or gradual) never qualifies, so callers can
// report a mismatch without over-reporting on unknown results.
func (r Reader) ValueWitnessProvenMismatch(value product.Value, want typ.Type) bool {
	if r.result == nil {
		return false
	}
	return r.result.ValueWitnessProvenMismatch(value, want)
}

func (r Reader) ValueProofAdmissible(value product.Value, want typ.Type) bool {
	if r.result == nil {
		return false
	}
	return r.result.ValueProofAdmissible(value, want)
}

func (r Reader) IsSubtype(sub, super typ.Type) bool {
	return r.result.IsSubtype(sub, super)
}
