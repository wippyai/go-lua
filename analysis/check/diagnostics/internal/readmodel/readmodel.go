package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Reader projects solved body boundary values into typed diagnostic read data.
type Reader struct {
	result *body.Result
}

func New(result *body.Result) Reader {
	return Reader{result: result}
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
			return value, true
		}
		_, proofTransparent := sourceprovenance.ProofInner(source.Expr)
		if _, ok := source.Expr.(*ast.FunctionExpr); !ok && proofTransparent {
			return product.Value{}, false
		}
		return r.result.LocalAssignmentSourceValueAtBoundary(point, source)
	case sourceprovenance.SourceCall, sourceprovenance.SourceVararg, sourceprovenance.SourceNil, sourceprovenance.SourceUnknown:
		valueSource, ok := valueSourceFromASTSource(source)
		if !ok {
			return product.Value{}, false
		}
		return r.result.SourceValueAtBoundary(point, valueSource)
	default:
		return product.Value{}, false
	}
}

func (r Reader) SourceType(point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	value, ok := r.SourceValue(point, source)
	if !ok {
		return nil, false
	}
	return r.ValueType(value)
}

func (r Reader) ValueType(value product.Value) (typ.Type, bool) {
	if r.result == nil || r.result.Registry() == nil {
		return nil, false
	}
	return concreteBoundaryType(r.result.Registry(), value)
}

func (r Reader) ValueHasUntrustedTopOrigin(value product.Value) bool {
	if r.result == nil || r.result.Registry() == nil {
		return false
	}
	got := product.Get(r.result.Registry(), value, evidence.Key)
	return got.IsGradualTop() || got.IsExplicitTop()
}

func (r Reader) ValueTypeWithPresence(value product.Value) (typ.Type, bool) {
	t, ok := r.ValueType(value)
	if !ok {
		if presence.Equal(product.PresenceOf(value), presence.Absent()) {
			return typ.Nil, true
		}
		return nil, false
	}
	return typeWithBoundaryPresence(t, value), true
}

func (r Reader) VariantOriginType(value product.Value) (typ.Type, bool) {
	if r.result == nil || r.result.Registry() == nil {
		return nil, false
	}
	reg := r.result.Registry()
	if product.Equal(reg, value, product.Bottom(reg)) {
		return nil, false
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return nil, false
	}
	return variant.TypeFromOrigin(origin.Family(), origin.Cases())
}

// FullVariantOriginType returns the complete structural union of the value's
// variant-origin family, independent of any narrowing recorded on this value.
// It yields the broad declared shape the discriminated value originated from.
func (r Reader) FullVariantOriginType(value product.Value) (typ.Type, bool) {
	if r.result == nil || r.result.Registry() == nil {
		return nil, false
	}
	reg := r.result.Registry()
	if product.Equal(reg, value, product.Bottom(reg)) {
		return nil, false
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return nil, false
	}
	return variant.FullFamilyType(origin.Family())
}

func (r Reader) RefineDeclaredType(declared typ.Type, value product.Value) (typ.Type, bool) {
	if declared == nil {
		return nil, false
	}
	out := declared
	p := product.PresenceOf(value)
	switch {
	case presence.Equal(p, presence.Present()):
		withoutNil := projectionWithoutNil(out)
		if withoutNil != nil && !typ.IsNever(withoutNil) {
			out = withoutNil
		}
	case presence.Equal(p, presence.Absent()):
		return typ.Nil, true
	}
	if r.result != nil && r.result.Registry() != nil {
		reg := r.result.Registry()
		origin := product.Get(reg, value, variantorigin.Key)
		witness := product.Get(reg, value, typewitness.Key)
		if t, ok := witness.Type(); ok {
			out = witnessTypeForPresence(t, p)
		}
		if !origin.IsBottom() && !origin.IsTop() {
			if refined, ok := variant.NarrowByOrigin(out, origin.Family(), origin.Cases()); ok {
				out = refined
			} else if refined, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases()); ok {
				out = witnessTypeForPresence(refined, p)
			}
		}
		kinds := product.Get(reg, value, runtimekind.Key)
		if refined, ok := refineTypeByRuntimeKindSet(out, kinds, p); ok {
			out = refined
		} else if runtimeType, ok := runtimeKindType(reg, value, p); ok {
			out = runtimeType
		}
	}
	return out, true
}

// NarrowDeclaredByOrigin narrows declared by the value's variant-origin facts
// and presence only, without substituting any structural type witness. The
// result stays a sound supertype of the runtime value: discriminant narrowing
// removes union arms but never drops declared fields the way a partial observed
// table-literal witness can.
func (r Reader) NarrowDeclaredByOrigin(declared typ.Type, value product.Value) (typ.Type, bool) {
	if declared == nil {
		return nil, false
	}
	out := declared
	p := product.PresenceOf(value)
	switch {
	case presence.Equal(p, presence.Present()):
		withoutNil := projectionWithoutNil(out)
		if withoutNil != nil && !typ.IsNever(withoutNil) {
			out = withoutNil
		}
	case presence.Equal(p, presence.Absent()):
		return typ.Nil, true
	}
	if r.result != nil && r.result.Registry() != nil {
		reg := r.result.Registry()
		origin := product.Get(reg, value, variantorigin.Key)
		if !origin.IsBottom() && !origin.IsTop() {
			if refined, ok := variant.NarrowByOrigin(out, origin.Family(), origin.Cases()); ok {
				out = refined
			} else if refined, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases()); ok {
				out = witnessTypeForPresence(refined, p)
			}
		}
	}
	return out, true
}

func (r Reader) ValueAdmissible(value product.Value, want typ.Type) bool {
	if r.result == nil || r.result.Registry() == nil || want == nil {
		return false
	}
	reg := r.result.Registry()
	if presence.Equal(product.PresenceOf(value), presence.Absent()) {
		return subtype.IsSubtype(typ.Nil, want)
	}
	if presence.Equal(product.PresenceOf(value), presence.Maybe()) && !subtype.IsSubtype(typ.Nil, want) {
		return false
	}
	gotEvidence := product.Get(reg, value, evidence.Key)
	if gotEvidence.IsExplicitTop() && explicitAnyClaim(reg, value) {
		if projected, ok := scalarRuntimeKindType(reg, value); ok && subtype.IsSubtype(projected, want) {
			return true
		}
		return false
	}
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok && subtype.IsSubtype(t, want) {
			return true
		}
	}
	if gotEvidence.IsGradualTop() {
		return false
	}
	if projected, ok := concreteBoundaryType(reg, value); ok && subtype.IsSubtype(projected, want) {
		return true
	}
	return false
}

// ValueWitnessProvenMismatch reports whether a value carries a concrete
// (non-top) type witness whose presence-adjusted form is provably not a subtype
// of want. It signals a proven contradiction, not a gradual one: a value with no
// concrete witness (genuinely unknown or gradual) never qualifies, so callers can
// report a mismatch without over-reporting on unknown results.
func (r Reader) ValueWitnessProvenMismatch(value product.Value, want typ.Type) bool {
	if r.result == nil || r.result.Registry() == nil || want == nil {
		return false
	}
	if typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	reg := r.result.Registry()
	valuePresence := product.PresenceOf(value)
	if presence.Equal(valuePresence, presence.Maybe()) || presence.Equal(valuePresence, presence.Absent()) {
		return false
	}
	witness := product.Get(reg, value, typewitness.Key)
	if witness.IsTop() {
		return false
	}
	t, ok := witness.Type()
	if !ok {
		return false
	}
	t = witnessTypeForPresence(t, valuePresence)
	return !subtype.IsSubtype(t, want)
}

func (r Reader) ValueProofAdmissible(value product.Value, want typ.Type) bool {
	if r.result == nil || r.result.Registry() == nil || want == nil {
		return false
	}
	reg := r.result.Registry()
	if presence.Equal(product.PresenceOf(value), presence.Absent()) {
		return subtype.IsSubtype(typ.Nil, want)
	}
	if presence.Equal(product.PresenceOf(value), presence.Maybe()) && !subtype.IsSubtype(typ.Nil, want) {
		return false
	}
	if product.Get(reg, value, evidence.Key).IsExplicitTop() && explicitAnyClaim(reg, value) {
		if t, ok := scalarRuntimeKindType(reg, value); ok && subtype.IsSubtype(t, want) {
			return true
		}
		return false
	}
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok && subtype.IsSubtype(t, want) {
			return true
		}
	}
	if t, ok := runtimeKindType(reg, value, product.PresenceOf(value)); ok && subtype.IsSubtype(t, want) {
		return true
	}
	return false
}

func explicitAnyClaim(reg *axis.Registry, value product.Value) bool {
	claim := product.Get(reg, value, assertion.Key)
	return claim.Has(assertion.AnyClaim)
}

func valueSourceFromASTSource(source sourceprovenance.ASTSource) (factflow.ValueSource, bool) {
	shape, ok := factflow.NewValueSourceShape(source.Final, source.Expanded, source.Adjusted, source.OpenTail)
	if !ok {
		return factflow.ValueSource{}, false
	}
	switch source.Kind {
	case sourceprovenance.SourceCall:
		return factflow.NewCallValueSource(0, source.ExprIndex, source.TargetIndex, source.ResultIndex, source.CallPoint, shape)
	case sourceprovenance.SourceVararg:
		return factflow.NewVarargValueSource(0, source.ExprIndex, source.TargetIndex, source.ResultIndex, shape)
	case sourceprovenance.SourceNil:
		return factflow.NewNilValueSource(source.TargetIndex), true
	case sourceprovenance.SourceUnknown:
		return factflow.NewUnknownValueSource(source.TargetIndex), true
	default:
		return factflow.ValueSource{}, false
	}
}

func concreteBoundaryType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	valuePresence := product.PresenceOf(value)
	if presence.Equal(valuePresence, presence.Absent()) {
		return typ.Nil, true
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if witness := product.Get(reg, value, typewitness.Key); !witness.IsTop() {
		if t, ok := witness.Type(); ok {
			t = witnessTypeForPresence(t, valuePresence)
			if !origin.IsBottom() && !origin.IsTop() {
				if narrowed, ok := variant.NarrowByOrigin(t, origin.Family(), origin.Cases()); ok {
					return narrowed, true
				}
				if narrowed, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases()); ok {
					return witnessTypeForPresence(narrowed, valuePresence), true
				}
			}
			return t, true
		}
		return nil, false
	}
	if gotEvidence := product.Get(reg, value, evidence.Key); gotEvidence.IsGradualTop() || gotEvidence.IsExplicitTop() {
		return typ.Any, true
	}
	if !origin.IsBottom() && !origin.IsTop() {
		if t, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases()); ok {
			return t, true
		}
	}
	return scalarRuntimeKindType(reg, value)
}

func witnessTypeForPresence(t typ.Type, p presence.Value) typ.Type {
	if presence.Equal(p, presence.Absent()) {
		return typ.Nil
	}
	if presence.Equal(p, presence.Present()) {
		if withoutNil := projectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	return t
}

func typeWithBoundaryPresence(t typ.Type, value product.Value) typ.Type {
	switch p := product.PresenceOf(value); {
	case presence.Equal(p, presence.Absent()):
		return typ.Nil
	case presence.Equal(p, presence.Maybe()):
		if !projectionHasNil(t) {
			return normalize.Optional(t)
		}
	case presence.Equal(p, presence.Present()):
		if withoutNil := projectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	return t
}

func scalarRuntimeKindType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return nil, false
	}
	var members []typ.Type
	for _, tag := range kinds.Tags() {
		switch tag {
		case runtimekind.Nil:
			members = append(members, typ.Nil)
		case runtimekind.Boolean:
			members = append(members, typ.Boolean)
		case runtimekind.Number:
			members = append(members, typ.Number)
		case runtimekind.String:
			members = append(members, typ.String)
		default:
			return nil, false
		}
	}
	t, ok := runtimeKindEvidenceUnion(product.PresenceOf(value), members)
	if !ok {
		return nil, false
	}
	if normalized := unwrap.NormalizeNil(t); normalized != nil && normalized.Kind() == kind.Nil {
		return typ.Nil, true
	}
	return t, true
}

func runtimeKindType(reg *axis.Registry, value product.Value, p presence.Value) (typ.Type, bool) {
	kinds := product.Get(reg, value, runtimekind.Key)
	if kinds.IsBottom() || kinds.IsTop() {
		return nil, false
	}
	var members []typ.Type
	for _, tag := range kinds.Tags() {
		switch tag {
		case runtimekind.Nil:
			members = append(members, typ.Nil)
		case runtimekind.Boolean:
			members = append(members, typ.Boolean)
		case runtimekind.Number:
			members = append(members, typ.Number)
		case runtimekind.String:
			members = append(members, typ.String)
		case runtimekind.Table:
			members = append(members, typetable.NewMap(typ.Any, typ.Unknown))
		case runtimekind.Function:
			members = append(members, typ.Func().Build())
		default:
			return nil, false
		}
	}
	return runtimeKindEvidenceUnion(p, members)
}

func runtimeKindEvidenceUnion(p presence.Value, members []typ.Type) (typ.Type, bool) {
	if len(members) == 0 {
		return nil, false
	}
	t := normalize.UnionForEvidence(members...)
	if presence.Equal(p, presence.Maybe()) && !typeIncludesNil(t) {
		t = normalize.Optional(t)
	}
	return t, true
}

func refineTypeByRuntimeKindSet(t typ.Type, kinds runtimekind.Value, p presence.Value) (typ.Type, bool) {
	if kinds.IsBottom() || kinds.IsTop() {
		return nil, false
	}
	keepNil := presence.Equal(p, presence.Maybe()) && projectionHasNil(t)
	return refineTypeByRuntimeKindSetDepth(t, kinds, keepNil, 0)
}

func refineTypeByRuntimeKindSetDepth(t typ.Type, kinds runtimekind.Value, keepNil bool, depth int) (typ.Type, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth || typ.IsAny(t) || typ.IsUnknown(t) {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Alias:
		return refineTypeByRuntimeKindSetDepth(v.UnaliasedTarget(), kinds, keepNil, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return refineTypeByRuntimeKindSetDepth(expanded, kinds, keepNil, depth+1)
	case *typ.Optional:
		innerKinds := kinds.Without(runtimekind.Nil)
		inner, ok := refineTypeByRuntimeKindSetDepth(v.Inner, innerKinds, false, depth+1)
		includeNil := keepNil || kinds.Contains(runtimekind.Nil)
		if !ok {
			if includeNil {
				return typ.Nil, true
			}
			return nil, false
		}
		if typ.IsNever(inner) {
			if includeNil {
				return typ.Nil, true
			}
			return typ.Never, true
		}
		if includeNil {
			return normalize.Optional(inner), true
		}
		return inner, true
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		changed := false
		for _, member := range v.Members {
			refined, ok := refineTypeByRuntimeKindSetDepth(member, kinds, keepNil, depth+1)
			if !ok {
				out = append(out, member)
				continue
			}
			if typ.IsNever(refined) {
				changed = true
				continue
			}
			if !typ.SameNodeOrAcyclicEqual(refined, member) {
				changed = true
			}
			out = append(out, refined)
		}
		if !changed {
			return t, true
		}
		return normalize.UnionForEvidence(out...), true
	default:
		normalized := unwrap.NormalizeNil(unwrap.Annotated(t))
		if normalized == nil {
			return nil, false
		}
		if normalized.Kind() == kind.Nil {
			if keepNil || kinds.Contains(runtimekind.Nil) {
				return typ.Nil, true
			}
			return typ.Never, true
		}
		memberKinds, ok := typevalue.RuntimeKindFromType(normalized)
		if !ok || memberKinds.IsTop() || memberKinds.IsBottom() {
			return nil, false
		}
		if runtimekind.Intersect(memberKinds, kinds).IsBottom() {
			return typ.Never, true
		}
		return t, true
	}
}

func typeIncludesNil(t typ.Type) bool {
	if t == nil {
		return false
	}
	normalized := unwrap.NormalizeNil(t)
	return (normalized != nil && normalized.Kind() == kind.Nil) || projectionHasNil(t)
}

func projectionHasNil(t typ.Type) bool {
	return projectionHasNilDepth(t, 0)
}

func projectionHasNilDepth(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) {
		return false
	}
	t = unwrap.NormalizeNil(unwrap.Annotated(t))
	if t == nil {
		return false
	}
	if t.Kind() == kind.Nil {
		return true
	}
	switch v := t.(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, member := range v.Members {
			if projectionHasNilDepth(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return projectionHasNilDepth(v.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && projectionHasNilDepth(expanded, depth+1)
	default:
		return false
	}
}

func projectionWithoutNil(t typ.Type) typ.Type {
	return typetable.PresentReadonlyEntryValue(t)
}
