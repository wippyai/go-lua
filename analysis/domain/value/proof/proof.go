// Package proof projects solved abstract values into type/proof queries without
// depending on syntax, engine state, or diagnostic rendering.
package proof

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekindof"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type Reader struct {
	reg       *axis.Registry
	typeCache *typevalue.Cache
}

func New(reg *axis.Registry, typeCache *typevalue.Cache) Reader {
	return Reader{reg: reg, typeCache: typeCache}
}

func (r Reader) ValueType(value product.Value) (typ.Type, bool) {
	return ConcreteBoundaryType(r.reg, r.typeCache, value)
}

// ValueStructuralType reconstructs the presence-aware structural type witnessed
// by value. This is the canonical proof-domain projection for callers that need
// table shape evidence instead of a declared contract.
func (r Reader) ValueStructuralType(value product.Value) (typ.Type, bool) {
	if r.reg == nil {
		return nil, false
	}
	return typevalue.StructuralTypeOf(r.reg, r.typeCache, value, typevalue.StructuralTypeOptions{ApplyPresence: true})
}

// ValueHasReadableConcreteWitness reports whether value carries an explicit
// witness type worth showing as concrete evidence. Top-like witnesses and a
// bare nil witness are not enough; callers should treat those as unknown or
// nilability evidence instead of a precise source type.
func (r Reader) ValueHasReadableConcreteWitness(value product.Value) bool {
	if r.reg == nil {
		return false
	}
	t, ok := typevalue.TypeOf(r.reg, value)
	return ok && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.TypeEquals(t, typ.Nil)
}

// RuntimeKindReducedType narrows declared by value's runtime-kind axis: the
// alternatives whose runtime kind the axis excludes are dropped. This reports
// the type a value actually holds on a path that a type() guard has narrowed
// (e.g. the else edge of type(v) == "number" makes a number | string value a
// string), which the declared witness alone does not reflect.
func (r Reader) RuntimeKindReducedType(value product.Value, declared typ.Type) (typ.Type, bool) {
	if r.reg == nil || declared == nil {
		return nil, false
	}
	kinds := product.Get(r.reg, value, runtimekind.Key)
	if kinds.IsTop() || kinds.IsBottom() {
		return nil, false
	}
	narrowed, changed := runtimekindof.RestrictTypeToRuntimeKind(declared, kinds)
	if !changed || narrowed == typ.Never {
		return nil, false
	}
	return narrowed, true
}

func (r Reader) VariantOriginType(value product.Value) (typ.Type, bool) {
	if r.reg == nil || r.typeCache == nil {
		return nil, false
	}
	if product.Equal(r.reg, value, product.Bottom(r.reg)) {
		return nil, false
	}
	origin := product.Get(r.reg, value, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return nil, false
	}
	return r.typeCache.TypeFromVariantOriginView(origin.Family(), origin.CasesView())
}

// FullVariantOriginType returns the complete structural union of the value's
// variant-origin family, independent of any narrowing recorded on this value.
// It yields the broad declared shape the discriminated value originated from.
func (r Reader) FullVariantOriginType(value product.Value) (typ.Type, bool) {
	if r.reg == nil {
		return nil, false
	}
	if product.Equal(r.reg, value, product.Bottom(r.reg)) {
		return nil, false
	}
	origin := product.Get(r.reg, value, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return nil, false
	}
	return variant.FullFamilyType(origin.Family())
}

func (r Reader) ValueHasUntrustedTopOrigin(value product.Value) bool {
	if r.reg == nil {
		return false
	}
	got := product.Get(r.reg, value, evidence.Key)
	return got.IsGradualTop() || got.IsExplicitTop()
}

// ValueHasExplicitTopOrigin reports whether value crossed an explicit
// any/unknown assertion boundary. Gradual top does not satisfy this predicate.
func (r Reader) ValueHasExplicitTopOrigin(value product.Value) bool {
	if r.reg == nil {
		return false
	}
	return product.Get(r.reg, value, evidence.Key).IsExplicitTop()
}

func (r Reader) ValueHasExactIdentity(value product.Value) bool {
	return identityvalue.HasExact(r.reg, value)
}

func (r Reader) ValueTypeWithPresence(value product.Value) (typ.Type, bool) {
	t, ok := r.ValueType(value)
	if !ok {
		if presence.Equal(product.PresenceOf(value), presence.Absent()) {
			return typ.Nil, true
		}
		return nil, false
	}
	return TypeWithBoundaryPresence(t, value), true
}

func (r Reader) RefineDeclaredType(declared typ.Type, value product.Value) (typ.Type, bool) {
	if declared == nil {
		return nil, false
	}
	out := declared
	p := product.PresenceOf(value)
	switch {
	case presence.Equal(p, presence.Present()):
		withoutNil := ProjectionWithoutNil(out)
		if withoutNil != nil && !typ.IsNever(withoutNil) {
			out = withoutNil
		}
	case presence.Equal(p, presence.Absent()):
		return typ.Nil, true
	}
	if r.reg != nil {
		origin := product.Get(r.reg, value, variantorigin.Key)
		if t, ok := typevalue.WitnessOf(r.reg, value); ok {
			out = WitnessTypeForPresence(t, p)
		}
		if r.typeCache != nil && !origin.IsBottom() && !origin.IsTop() {
			if refined, ok := r.typeCache.NarrowVariantByOriginView(out, origin.Family(), origin.CasesView()); ok {
				out = refined
			} else if refined, ok := r.typeCache.TypeFromVariantOriginView(origin.Family(), origin.CasesView()); ok {
				out = WitnessTypeForPresence(refined, p)
			}
		}
		kinds := product.Get(r.reg, value, runtimekind.Key)
		if refined, ok := RefineTypeByRuntimeKindSet(out, kinds, p); ok {
			out = refined
		} else if runtimeType, ok := RuntimeKindType(r.reg, value, p); ok {
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
		withoutNil := ProjectionWithoutNil(out)
		if withoutNil != nil && !typ.IsNever(withoutNil) {
			out = withoutNil
		}
	case presence.Equal(p, presence.Absent()):
		return typ.Nil, true
	}
	if r.reg != nil && r.typeCache != nil {
		origin := product.Get(r.reg, value, variantorigin.Key)
		if !origin.IsBottom() && !origin.IsTop() {
			if refined, ok := r.typeCache.NarrowVariantByOriginView(out, origin.Family(), origin.CasesView()); ok {
				out = refined
			} else if refined, ok := r.typeCache.TypeFromVariantOriginView(origin.Family(), origin.CasesView()); ok {
				out = WitnessTypeForPresence(refined, p)
			}
		}
	}
	return out, true
}

func (r Reader) ValueAdmissible(value product.Value, want typ.Type) bool {
	if r.reg == nil || want == nil {
		return false
	}
	if presence.Equal(product.PresenceOf(value), presence.Absent()) {
		return r.IsSubtype(typ.Nil, want)
	}
	if presence.Equal(product.PresenceOf(value), presence.Maybe()) && !r.IsSubtype(typ.Nil, want) {
		return false
	}
	if claims := product.Get(r.reg, value, assertion.Key); claims.Has(assertion.AnyClaim) && !claims.Has(assertion.RuntimeClaim) {
		return false
	}
	gotEvidence := product.Get(r.reg, value, evidence.Key)
	if gotEvidence.IsExplicitTop() || gotEvidence.IsGradualTop() {
		return r.runtimeValidationAdmissible(value, want)
	}
	if t, ok := typevalue.WitnessOf(r.reg, value); ok && r.IsSubtype(t, want) {
		return true
	}
	if projected, ok := ConcreteBoundaryType(r.reg, r.typeCache, value); ok && r.IsSubtype(projected, want) {
		return true
	}
	return false
}

func (r Reader) ValueWitnessProvenMismatch(value product.Value, want typ.Type) bool {
	if r.reg == nil || want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
		return false
	}
	if r.ValueHasUntrustedTopOrigin(value) {
		return false
	}
	valuePresence := product.PresenceOf(value)
	if presence.Equal(valuePresence, presence.Absent()) {
		return !r.IsSubtype(typ.Nil, want)
	}
	if presence.Equal(valuePresence, presence.Maybe()) && !r.IsSubtype(typ.Nil, want) {
		return true
	}
	t, ok := typevalue.WitnessOf(r.reg, value)
	if !ok {
		return false
	}
	t = WitnessTypeForPresence(t, valuePresence)
	return !r.IsSubtype(t, want)
}

func (r Reader) ValueProofAdmissible(value product.Value, want typ.Type) bool {
	if r.reg == nil || want == nil {
		return false
	}
	if presence.Equal(product.PresenceOf(value), presence.Absent()) {
		return r.IsSubtype(typ.Nil, want)
	}
	if presence.Equal(product.PresenceOf(value), presence.Maybe()) && !r.IsSubtype(typ.Nil, want) {
		return false
	}
	if product.Get(r.reg, value, evidence.Key).IsExplicitTop() {
		return r.explicitTopProofAdmissible(value, want)
	}
	if product.Get(r.reg, value, evidence.Key).IsGradualTop() {
		return r.exactLiteralWitnessAdmissible(value, want) || r.runtimeValidationAdmissible(value, want)
	}
	if claims := product.Get(r.reg, value, assertion.Key); claims.Has(assertion.AnyClaim) && !claims.Has(assertion.RuntimeClaim) {
		return false
	}
	if t, ok := typevalue.WitnessOf(r.reg, value); ok {
		t = WitnessTypeForPresence(t, product.PresenceOf(value))
		if r.IsSubtype(t, want) {
			return true
		}
		if r.freshWitnessAssignable(value, t, want) {
			return true
		}
	}
	if t, ok := RuntimeKindType(r.reg, value, product.PresenceOf(value)); ok && r.IsSubtype(t, want) {
		return true
	}
	return false
}

// TypeEvidenceUsableForInference reports whether value's type evidence is
// trustworthy enough to drive generic-call inference. Gradual or explicit any
// remains unusable; concrete runtime-validation casts are admissible because
// the runtime check establishes the witnessed type before the call executes.
func (r Reader) TypeEvidenceUsableForInference(value product.Value) bool {
	if r.reg == nil {
		return false
	}
	ev := product.Get(r.reg, value, evidence.Key)
	if ev.IsGradualTop() {
		return false
	}
	if ev.IsExplicitTop() {
		return product.Get(r.reg, value, assertion.Key).Has(assertion.RuntimeClaim)
	}
	return true
}

func (r Reader) IsSubtype(sub, super typ.Type) bool {
	if r.typeCache == nil {
		return typevalue.NewCache().IsSubtype(sub, super)
	}
	return r.typeCache.IsSubtype(sub, super)
}

func (r Reader) explicitTopProofAdmissible(value product.Value, want typ.Type) bool {
	if topLikeContract(want) {
		return true
	}
	if explicitTopRecordWithTopLikeMember(want) && product.Get(r.reg, value, assertion.Key).IsTop() {
		if t, ok := typevalue.WitnessOf(r.reg, value); ok && r.IsSubtype(t, want) {
			return true
		}
		if t, ok := ConcreteBoundaryType(r.reg, r.typeCache, value); ok && r.IsSubtype(t, want) {
			return true
		}
	}
	return r.runtimeValidationAdmissible(value, want) ||
		r.exactLiteralWitnessAdmissible(value, want) ||
		r.freshStructuralWitnessAdmissible(value, want) ||
		r.userScalarAssertionAdmissible(value, want)
}

func explicitTopRecordWithTopLikeMember(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil {
		return false
	}
	for _, field := range rec.Fields {
		if topLikeContract(field.Type) {
			return true
		}
	}
	for _, member := range rec.StaticMembers {
		if topLikeContract(member.Type) {
			return true
		}
	}
	return rec.HasMapComponent() && topLikeContract(rec.MapValue)
}

func topLikeContract(t typ.Type) bool {
	return topLikeContractSeen(t, &typegraph.Path{})
}

func topLikeContractSeen(t typ.Type, active *typegraph.Path) bool {
	if t == nil {
		return false
	}
	t = unwrap.Alias(t)
	if t == nil || !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	switch tt := t.(type) {
	case *typ.Optional:
		return tt != nil && topLikeContractSeen(tt.Inner, active)
	case *typ.Array:
		return tt != nil && topLikeContractSeen(tt.Element, active)
	case *typ.Map:
		return tt != nil && topLikeContractSeen(tt.Value, active)
	case *typ.ReadonlyMap:
		return tt != nil && topLikeContractSeen(tt.Value, active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		return expanded != nil && expanded != t && topLikeContractSeen(expanded, active)
	case *typ.Recursive:
		return tt.Body != nil && tt.Body != t && topLikeContractSeen(tt.Body, active)
	}
	return false
}

func (r Reader) freshStructuralWitnessAdmissible(value product.Value, want typ.Type) bool {
	if claims := product.Get(r.reg, value, assertion.Key); claims.Has(assertion.AnyClaim) && !claims.Has(assertion.RuntimeClaim) {
		return false
	}
	got, ok := typevalue.WitnessOf(r.reg, value)
	if !ok {
		return false
	}
	return r.freshWitnessAssignable(value, got, want)
}

func (r Reader) exactLiteralWitnessAdmissible(value product.Value, want typ.Type) bool {
	t, ok := typevalue.WitnessOf(r.reg, value)
	if !ok {
		return false
	}
	lit, ok := unwrap.Alias(t).(*typ.Literal)
	if !ok {
		return false
	}
	switch lit.Base {
	case kind.Boolean, kind.Number, kind.Integer, kind.String:
		return r.IsSubtype(lit, want)
	default:
		return false
	}
}

func (r Reader) freshWitnessAssignable(value product.Value, got, want typ.Type) bool {
	if got == nil || want == nil || !escape.Equal(product.Get(r.reg, value, escape.Key), escape.Fresh()) {
		return false
	}
	if r.typeCache == nil {
		return typevalue.NewCache().IsFreshAssignable(got, want)
	}
	return r.typeCache.IsFreshAssignable(got, want)
}

func (r Reader) userScalarAssertionAdmissible(value product.Value, want typ.Type) bool {
	if !product.Get(r.reg, value, assertion.Key).Has(assertion.TypeClaim) {
		return false
	}
	t, ok := typevalue.WitnessOf(r.reg, value)
	if !ok || !r.IsSubtype(t, want) {
		return false
	}
	return scalarUserAssertionTarget(t) && scalarUserAssertionTarget(want)
}

func (r Reader) runtimeValidationAdmissible(value product.Value, want typ.Type) bool {
	if !product.Get(r.reg, value, assertion.Key).Has(assertion.RuntimeClaim) {
		return false
	}
	if t, ok := typevalue.WitnessOf(r.reg, value); ok && r.IsSubtype(t, want) {
		return true
	}
	if t, ok := RuntimeKindType(r.reg, value, product.PresenceOf(value)); ok && r.IsSubtype(t, want) {
		return true
	}
	return false
}

func scalarUserAssertionTarget(t typ.Type) bool {
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Boolean, kind.Number, kind.Integer, kind.String:
		return true
	default:
		return false
	}
}

func ConcreteBoundaryType(reg *axis.Registry, typeCache *typevalue.Cache, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	valuePresence := product.PresenceOf(value)
	if presence.Equal(valuePresence, presence.Absent()) {
		return typ.Nil, true
	}
	origin := product.Get(reg, value, variantorigin.Key)
	if t, ok := typevalue.WitnessOf(reg, value); ok {
		t = WitnessTypeForPresence(t, valuePresence)
		if !origin.IsBottom() && !origin.IsTop() && typeCache != nil {
			if narrowed, ok := typeCache.NarrowVariantByOriginView(t, origin.Family(), origin.CasesView()); ok {
				return narrowed, true
			}
			if narrowed, ok := typeCache.TypeFromVariantOriginView(origin.Family(), origin.CasesView()); ok {
				return WitnessTypeForPresence(narrowed, valuePresence), true
			}
		}
		return t, true
	}
	gotEvidence := product.Get(reg, value, evidence.Key)
	if gotEvidence.IsGradualTop() || gotEvidence.IsExplicitTop() {
		return typ.Any, true
	}
	if !origin.IsBottom() && !origin.IsTop() && typeCache != nil {
		if t, ok := typeCache.TypeFromVariantOriginView(origin.Family(), origin.CasesView()); ok {
			return t, true
		}
	}
	return ScalarRuntimeKindType(reg, value)
}

func WitnessTypeForPresence(t typ.Type, p presence.Value) typ.Type {
	if presence.Equal(p, presence.Absent()) {
		return typ.Nil
	}
	if presence.Equal(p, presence.Present()) {
		if withoutNil := ProjectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	return t
}

func TypeWithBoundaryPresence(t typ.Type, value product.Value) typ.Type {
	switch p := product.PresenceOf(value); {
	case presence.Equal(p, presence.Absent()):
		return typ.Nil
	case presence.Equal(p, presence.Maybe()):
		if !typevalue.ProjectionHasNil(t) {
			return normalize.Optional(t)
		}
	case presence.Equal(p, presence.Present()):
		if withoutNil := ProjectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	return t
}

func ScalarRuntimeKindType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
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

func RuntimeKindType(reg *axis.Registry, value product.Value, p presence.Value) (typ.Type, bool) {
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
	if presence.Equal(p, presence.Maybe()) && !typevalue.TypeIncludesNil(t) {
		t = normalize.Optional(t)
	}
	return t, true
}

func RefineTypeByRuntimeKindSet(t typ.Type, kinds runtimekind.Value, p presence.Value) (typ.Type, bool) {
	if kinds.IsBottom() || kinds.IsTop() {
		return nil, false
	}
	keepNil := presence.Equal(p, presence.Maybe()) && typevalue.ProjectionHasNil(t)
	return refineTypeByRuntimeKindSetSeen(t, kinds, keepNil, &typegraph.Path{})
}

func refineTypeByRuntimeKindSetSeen(t typ.Type, kinds runtimekind.Value, keepNil bool, active *typegraph.Path) (typ.Type, bool) {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) {
		return nil, false
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, 0) {
		return t, true
	}
	defer active.Leave(t, 0)
	switch v := t.(type) {
	case *typ.Alias:
		return refineTypeByRuntimeKindSetSeen(v.UnaliasedTarget(), kinds, keepNil, active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return refineTypeByRuntimeKindSetSeen(expanded, kinds, keepNil, active)
	case *typ.Optional:
		innerKinds := kinds.Without(runtimekind.Nil)
		inner, ok := refineTypeByRuntimeKindSetSeen(v.Inner, innerKinds, false, active)
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
			refined, ok := refineTypeByRuntimeKindSetSeen(member, kinds, keepNil, active)
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

func ProjectionWithoutNil(t typ.Type) typ.Type {
	return typetable.PresentReadonlyEntryValue(t)
}

// OptionalTypeHasConcreteValue reports whether t is a concrete optional-like
// projection with both a nil arm and a non-never value arm. Gradual and unknown
// projections are intentionally inconclusive.
func OptionalTypeHasConcreteValue(t typ.Type) bool {
	if t == nil || typ.IsTopLike(t) || typ.IsNever(t) || !typevalue.ProjectionHasNil(t) {
		return false
	}
	value := ProjectionWithoutNil(t)
	return value != nil && !typ.IsNever(value)
}

// OptionalTruthinessPartitionsNilValue reports whether truthiness checks can
// split an optional-like type into nil and value cases. If the value arm may be
// false, truthiness cannot prove the nil arm was handled.
func OptionalTruthinessPartitionsNilValue(t typ.Type) bool {
	if !OptionalTypeHasConcreteValue(t) {
		return false
	}
	value := ProjectionWithoutNil(t)
	return value != nil && !typ.IsNever(value) && !typ.AdmitsFalse(value)
}
