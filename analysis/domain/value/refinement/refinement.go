package refinement

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/internal/typegraph"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	typerefine "github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// PartitionTruthiness intersects value with one side of Lua's truthiness
// partition. exact reports whether the selected side is represented exactly
// by the product vocabulary. It never truncates recursive types or drops an
// unrelated axis: unrepresentable complements retain value and report false.
//
// Lua's falsy side is always representable (nil | false). The truthy side is a
// relative complement and is exact only when truthyTypePart can express it in
// the stable type vocabulary.
func PartitionTruthiness(reg *axis.Registry, value product.Value, wantTruthy bool) (product.Value, bool) {
	if product.Equal(reg, value, product.Bottom(reg)) {
		return value, true
	}
	if !wantTruthy {
		falsyType := typeexpr.Union(typ.Nil, typ.False)
		if valueType, ok := typevalue.TypeOf(reg, value); ok && valueType != nil && !typ.IsTopLike(valueType) {
			var exact bool
			falsyType, exact = falsyTypePart(valueType, &typegraph.Path{})
			if !exact {
				return value, false
			}
			if falsyType == nil || typ.IsNever(falsyType) {
				return product.Bottom(reg), true
			}
		}
		// Optional values encode nil in the dedicated presence lane; their
		// remaining axes describe the present alternative.  Meeting an optional
		// string with a fully materialized nil value therefore contradicts the
		// string runtime-kind axis.  When the exact falsy projection is nil-only,
		// refine the owning presence coordinate directly and preserve every
		// independent present-alternative axis.
		if subtype.IsSubtype(falsyType, typ.Nil) {
			return product.WithPresence(reg, value, presence.Absent()), true
		}
		constraint := typevalue.WithWitness(reg, typevalue.FromType(reg, falsyType), falsyType)
		return MeetConstraint(reg, value, constraint), true
	}
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return value, false
	}
	narrowed, exact := truthyTypePart(t, &typegraph.Path{})
	if !exact {
		return value, false
	}
	if narrowed == nil || typ.IsNever(narrowed) {
		return product.Bottom(reg), true
	}
	// FromType alone materializes the runtime axes, but broad-to-literal
	// narrowing (notably boolean -> true) also requires the stable witness.
	// Keeping both is what makes the partition exact across the full product.
	constraint := typevalue.WithWitness(reg, typevalue.FromType(reg, narrowed), narrowed)
	return MeetConstraint(reg, value, constraint), true
}

func falsyTypePart(t typ.Type, active *typegraph.Path) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	t = unwrap.Annotated(t)
	if typ.IsTopLike(t) {
		return typeexpr.Union(typ.Nil, typ.False), true
	}
	if !active.Enter(t, 2) {
		return t, false
	}
	defer active.Leave(t, 2)
	switch value := t.(type) {
	case *typ.Optional:
		inner, exact := falsyTypePart(value.Inner, active)
		if !exact {
			return t, false
		}
		// Lua optionality is a disjoint nil alternative.  When the present
		// member has no falsy inhabitant, the exact falsy projection is nil,
		// not a synthetic nil|Never wrapper.  Keeping that wrapper loses the
		// presence-axis certificate during type materialization and makes a
		// direct `if optionalString` false edge remain Maybe.
		if inner == nil || typ.IsNever(inner) {
			return typ.Nil, true
		}
		return typeexpr.Union(typ.Nil, inner), true
	case *typ.Union:
		members := make([]typ.Type, 0, len(value.Members))
		for _, member := range value.Members {
			part, exact := falsyTypePart(member, active)
			if !exact {
				return t, false
			}
			if part != nil && !typ.IsNever(part) {
				members = append(members, part)
			}
		}
		return typeexpr.Union(members...), true
	case *typ.Literal:
		if boolean, ok := value.Value.(bool); ok && !boolean {
			return t, true
		}
		return typ.Never, true
	case *typ.Alias:
		return falsyTypePart(value.UnaliasedTarget(), active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(value)
		if expanded == nil || expanded == t {
			return t, false
		}
		return falsyTypePart(expanded, active)
	case *typ.Recursive:
		if value.Body == nil || value.Body == t {
			return t, false
		}
		return falsyTypePart(value.Body, active)
	default:
		switch {
		case subtype.IsSubtype(t, typ.Nil), subtype.IsSubtype(t, typ.False):
			return t, true
		case t.Kind() == kind.Boolean:
			return typ.False, true
		case !typeAdmitsFalsy(t, nil):
			return typ.Never, true
		default:
			return t, false
		}
	}
}

func truthyTypePart(t typ.Type, active *typegraph.Path) (typ.Type, bool) {
	if t == nil || typ.IsTopLike(t) {
		return t, false
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, 1) {
		return t, false
	}
	defer active.Leave(t, 1)
	switch value := t.(type) {
	case *typ.Optional:
		return truthyTypePart(value.Inner, active)
	case *typ.Union:
		members := make([]typ.Type, 0, len(value.Members))
		for _, member := range value.Members {
			part, exact := truthyTypePart(member, active)
			if !exact {
				return t, false
			}
			if part != nil && !typ.IsNever(part) {
				members = append(members, part)
			}
		}
		return typeexpr.Union(members...), true
	case *typ.Literal:
		if boolean, ok := value.Value.(bool); ok && !boolean {
			return typ.Never, true
		}
		return t, true
	case *typ.Alias:
		return truthyTypePart(value.UnaliasedTarget(), active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(value)
		if expanded == nil || expanded == t {
			return t, false
		}
		return truthyTypePart(expanded, active)
	case *typ.Recursive:
		if value.Body == nil || value.Body == t {
			return t, false
		}
		return truthyTypePart(value.Body, active)
	default:
		if subtype.IsSubtype(t, typ.Nil) || subtype.IsSubtype(t, typ.False) {
			return typ.Never, true
		}
		if t.Kind() == kind.Boolean {
			return typ.True, true
		}
		if !typeAdmitsFalsy(t, nil) {
			return t, true
		}
		return t, false
	}
}

// CanBeFalse reports whether value's present type evidence may be boolean false.
// Missing or unknown evidence is treated as admitting false so branch narrowing
// remains sound.
func CanBeFalse(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return true
	}
	return typeAdmitsFalse(t, nil)
}

func typeAdmitsFalse(t typ.Type, typeCache *typevalue.Cache) bool {
	return typeAdmitsFalseSeen(t, &typegraph.Path{}, typeCache)
}

func typeAdmitsFalseSeen(t typ.Type, active *typegraph.Path, typeCache *typevalue.Cache) bool {
	if t == nil {
		return true
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	switch v := t.(type) {
	case *typ.Optional:
		return typeAdmitsFalseSeen(v.Inner, active, typeCache)
	case *typ.Union:
		for _, member := range v.Members {
			if typeAdmitsFalseSeen(member, active, typeCache) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return typeAdmitsFalseSeen(v.UnaliasedTarget(), active, typeCache)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && typeAdmitsFalseSeen(expanded, active, typeCache)
	case *typ.Recursive:
		return v.Body != nil && v.Body != t && typeAdmitsFalseSeen(v.Body, active, typeCache)
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Function:
		return false
	default:
		if typ.IsNever(t) {
			return false
		}
		return isSubtypeCached(typeCache, typ.False, t)
	}
}

// CanBeTruthy reports whether value's stable type evidence may evaluate truthy
// under Lua truthiness. Missing or unknown evidence is treated as admitting
// truth so branch narrowing remains sound.
func CanBeTruthy(reg *axis.Registry, value product.Value) bool {
	return CanBeTruthyCached(reg, nil, value)
}

// CanBeTruthyCached is CanBeTruthy for a caller that already owns this check
// run's *typevalue.Cache. A non-nil cache reuses the run's memoized
// variant-origin projection instead of rebuilding it for this query.
func CanBeTruthyCached(reg *axis.Registry, typeCache *typevalue.Cache, value product.Value) bool {
	t, ok := cachedTypeOf(reg, typeCache, value)
	if !ok || t == nil {
		return true
	}
	return typeAdmitsTruthy(t, typeCache)
}

// CanBeFalsy reports whether value may evaluate falsy under Lua truthiness.
// Missing or unknown evidence is treated as admitting falsy so branch pruning
// remains sound.
func CanBeFalsy(reg *axis.Registry, value product.Value) bool {
	return CanBeFalsyCached(reg, nil, value)
}

// CanBeFalsyCached is CanBeFalsy for a caller that already owns this check
// run's *typevalue.Cache. A non-nil cache reuses the run's memoized
// variant-origin projection instead of rebuilding it for this query.
func CanBeFalsyCached(reg *axis.Registry, typeCache *typevalue.Cache, value product.Value) bool {
	t, ok := cachedTypeOf(reg, typeCache, value)
	if !ok || t == nil {
		return true
	}
	return typeAdmitsFalsy(t, typeCache)
}

func cachedTypeOf(reg *axis.Registry, typeCache *typevalue.Cache, value product.Value) (typ.Type, bool) {
	if typeCache != nil {
		return typeCache.TypeOf(reg, value)
	}
	return typevalue.TypeOf(reg, value)
}

func isSubtypeCached(typeCache *typevalue.Cache, sub, super typ.Type) bool {
	if typeCache != nil {
		return typeCache.IsSubtype(sub, super)
	}
	return subtype.IsSubtype(sub, super)
}

// TypeAdmitsTruthy reports whether a value of t may evaluate truthy under Lua
// truthiness. It is the type-domain entry point to the same lattice CanBeTruthy
// reaches through a product value, for a caller that already holds the resolved
// type. A missing type admits truth.
func TypeAdmitsTruthy(t typ.Type) bool {
	return typeAdmitsTruthy(t, nil)
}

// TypeAdmitsFalsy reports whether a value of t may evaluate falsy under Lua
// truthiness: only nil and false are falsy, so a string, a number and a table
// admit no falsy value.
func TypeAdmitsFalsy(t typ.Type) bool {
	return typeAdmitsFalsy(t, nil)
}

func typeAdmitsTruthy(t typ.Type, typeCache *typevalue.Cache) bool {
	return typeAdmitsTruthySeen(t, &typegraph.Path{}, typeCache)
}

func typeAdmitsTruthySeen(t typ.Type, active *typegraph.Path, typeCache *typevalue.Cache) bool {
	if t == nil {
		return true
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	switch v := t.(type) {
	case *typ.Optional:
		return typeAdmitsTruthySeen(v.Inner, active, typeCache)
	case *typ.Union:
		for _, member := range v.Members {
			if typeAdmitsTruthySeen(member, active, typeCache) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return typeAdmitsTruthySeen(v.UnaliasedTarget(), active, typeCache)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && typeAdmitsTruthySeen(expanded, active, typeCache)
	case *typ.Recursive:
		return v.Body != nil && v.Body != t && typeAdmitsTruthySeen(v.Body, active, typeCache)
	default:
		if typ.IsNever(t) {
			return false
		}
		return !isSubtypeCached(typeCache, t, typ.Nil) && !isSubtypeCached(typeCache, t, typ.False)
	}
}

func typeAdmitsFalsy(t typ.Type, typeCache *typevalue.Cache) bool {
	return typeAdmitsFalsySeen(t, &typegraph.Path{}, typeCache)
}

func typeAdmitsFalsySeen(t typ.Type, active *typegraph.Path, typeCache *typevalue.Cache) bool {
	if t == nil {
		return true
	}
	t = unwrap.Annotated(t)
	if !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	switch v := t.(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, member := range v.Members {
			if typeAdmitsFalsySeen(member, active, typeCache) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return typeAdmitsFalsySeen(v.UnaliasedTarget(), active, typeCache)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && typeAdmitsFalsySeen(expanded, active, typeCache)
	case *typ.Recursive:
		return v.Body != nil && v.Body != t && typeAdmitsFalsySeen(v.Body, active, typeCache)
	default:
		if typ.IsNever(t) {
			return false
		}
		return isSubtypeCached(typeCache, typ.Nil, t) || isSubtypeCached(typeCache, typ.False, t)
	}
}

// LiteralType returns the literal type carried by a product constraint.
func LiteralType(reg *axis.Registry, constraint product.Value) (typ.Type, bool) {
	t, ok := typevalue.TypeOf(reg, constraint)
	if !ok {
		return nil, false
	}
	if _, ok := t.(*typ.Literal); !ok {
		return nil, false
	}
	return t, true
}

// NegatedLiteralContradictsValue reports whether a value is already proven to
// be the exact literal excluded by a negated literal constraint. The constraint
// must carry an explicit literal witness; derived broad type evidence is not
// enough to make a control-flow edge unreachable.
func NegatedLiteralContradictsValue(reg *axis.Registry, typeCache *typevalue.Cache, value, constraint product.Value) bool {
	lit, ok := LiteralWitnessType(reg, constraint)
	if !ok || lit == nil {
		return false
	}
	var valueType typ.Type
	if typeCache != nil {
		valueType, _ = typeCache.TypeOf(reg, value)
	}
	if valueType == nil {
		valueType, _ = typevalue.TypeOf(reg, value)
	}
	if valueType == nil {
		return false
	}
	if typeCache != nil {
		return typeCache.IsSubtype(valueType, lit)
	}
	return subtype.IsSubtype(valueType, lit)
}

// LiteralWitnessType returns the literal type carried by a value's explicit
// type witness. It is stricter than LiteralType, which may derive a literal
// from other axes.
func LiteralWitnessType(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	if _, ok := reg.LookupErased(typewitness.Key.ID()); !ok {
		return nil, false
	}
	t, ok := typevalue.WitnessOf(reg, value)
	if !ok {
		return nil, false
	}
	if _, ok := t.(*typ.Literal); !ok {
		return nil, false
	}
	return t, true
}

// MeetConstraint refines value with constraint and recovers compatible
// type-witness or variant-origin evidence when sparse-axis identity would make a
// sound subtype refinement collapse to bottom.
func MeetConstraint(reg *axis.Registry, value, constraint product.Value) product.Value {
	refined := product.Meet(reg, value, constraint)
	if !product.Equal(reg, refined, product.Bottom(reg)) {
		if recovered, ok := recoverCompatibleVariantOriginMeet(reg, value, constraint); ok {
			return recovered
		}
		if recovered, ok := recoverCompatibleWitnessMeet(reg, value, constraint); ok {
			return recovered
		}
		if recovered, ok := typevalue.RecoverRuntimeKindWitnessMeet(reg, value, constraint); ok {
			return recovered
		}
		return refined
	}
	if recovered, ok := recoverCompatibleVariantOriginMeet(reg, value, constraint); ok {
		return recovered
	}
	if recovered, ok := recoverCompatibleWitnessMeet(reg, value, constraint); ok {
		return recovered
	}
	if recovered, ok := typevalue.RecoverRuntimeKindWitnessMeet(reg, value, constraint); ok {
		return recovered
	}
	return refined
}

func recoverCompatibleWitnessMeet(reg *axis.Registry, value, constraint product.Value) (product.Value, bool) {
	if !registryHasAxis(reg, typewitness.Key.ID()) {
		return product.Value{}, false
	}
	valueWitness := product.Get(reg, value, typewitness.Key)
	constraintWitness := product.Get(reg, constraint, typewitness.Key)
	if valueWitness.IsTop() || valueWitness.IsBottom() ||
		constraintWitness.IsTop() || constraintWitness.IsBottom() {
		return product.Value{}, false
	}
	valueType, ok := valueWitness.Type()
	if !ok {
		return product.Value{}, false
	}
	constraintType, ok := constraintWitness.Type()
	if !ok {
		return product.Value{}, false
	}
	narrower := constraintWitness
	switch {
	case subtype.IsSubtype(constraintType, valueType):
		narrower = constraintWitness
	case subtype.IsSubtype(valueType, constraintType):
		narrower = valueWitness
	default:
		narrowed, ok := compatibleValueWitnessType(valueType, constraintType)
		if !ok {
			return product.Value{}, false
		}
		narrower = typewitness.Of(narrowed)
	}
	valueWithoutWitness := product.Set(reg, value, typewitness.Key, typewitness.Top())
	constraintWithoutWitness := product.Set(reg, constraint, typewitness.Key, typewitness.Top())
	refined := product.Meet(reg, valueWithoutWitness, constraintWithoutWitness)
	if product.Equal(reg, refined, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return product.Set(reg, refined, typewitness.Key, narrower), true
}

func recoverCompatibleVariantOriginMeet(reg *axis.Registry, value, constraint product.Value) (product.Value, bool) {
	if !typevalue.HasLuaTypeEvidenceAxes(reg) {
		return product.Value{}, false
	}
	valueOrigin := product.Get(reg, value, variantorigin.Key)
	constraintOrigin := product.Get(reg, constraint, variantorigin.Key)
	if constraintOrigin.IsTop() || constraintOrigin.IsBottom() {
		return product.Value{}, false
	}
	if valueOrigin.IsTop() || valueOrigin.IsBottom() {
		return recoverCompatibleConcreteVariantOriginMeet(reg, value, constraint, constraintOrigin)
	}
	if valueOrigin.Family() == constraintOrigin.Family() {
		return recoverCompatibleConcreteVariantOriginMeet(reg, value, constraint, constraintOrigin)
	}
	valueType, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return product.Value{}, false
	}
	constraintType, ok := typevalue.TypeOf(reg, constraint)
	if !ok {
		return product.Value{}, false
	}
	if !subtype.IsSubtype(valueType, constraintType) && !subtype.IsSubtype(constraintType, valueType) {
		if openVariantConstraintAdmitsValue(valueType, constraintType) {
			if selected, ok := currentVariantCasesAdmittedByConstraint(valueOrigin, constraintType); ok {
				return recoverCompatibleConcreteVariantOriginMeet(reg, value, constraint, variantorigin.Of(valueOrigin.Family(), selected))
			}
			return recoverCompatibleConcreteVariantOriginMeet(reg, value, constraint, constraintOrigin)
		}
		return product.Value{}, false
	}
	if selected, ok := currentVariantCasesAdmittedByConstraint(valueOrigin, constraintType); ok {
		return recoverCompatibleConcreteVariantOriginMeet(reg, value, constraint, variantorigin.Of(valueOrigin.Family(), selected))
	}
	valueWithoutOrigin := product.Set(reg, value, variantorigin.Key, variantorigin.Top())
	constraintWithoutOrigin := product.Set(reg, constraint, variantorigin.Key, variantorigin.Top())
	refined := product.Meet(reg, valueWithoutOrigin, constraintWithoutOrigin)
	if product.Equal(reg, refined, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return product.Set(reg, refined, variantorigin.Key, constraintOrigin), true
}

func registryHasAxis(reg *axis.Registry, id string) bool {
	if reg == nil {
		return false
	}
	_, ok := reg.LookupErased(id)
	return ok
}

func currentVariantCasesAdmittedByConstraint(origin variantorigin.Value, constraintType typ.Type) ([]int, bool) {
	if origin.IsTop() || origin.IsBottom() || constraintType == nil {
		return nil, false
	}
	var selected []int
	for i := 0; i < origin.CasesLen(); i++ {
		c := origin.CaseAt(i)
		caseType, ok := variant.TypeFromOrigin(origin.Family(), []int{c})
		if !ok {
			continue
		}
		if openVariantConstraintAdmitsValue(caseType, constraintType) {
			selected = append(selected, c)
		}
	}
	return selected, len(selected) != 0
}

func recoverCompatibleConcreteVariantOriginMeet(reg *axis.Registry, value, constraint product.Value, origin variantorigin.Value) (product.Value, bool) {
	valueType, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return product.Value{}, false
	}
	constraintType, ok := typevalue.TypeOf(reg, constraint)
	if !ok || !openVariantConstraintAdmitsValue(valueType, constraintType) {
		return product.Value{}, false
	}
	constraintWithoutOrigin := product.Set(reg, constraint, variantorigin.Key, variantorigin.Top())
	refined := product.Meet(reg, value, constraintWithoutOrigin)
	if product.Equal(reg, refined, product.Bottom(reg)) {
		refined = value
	}
	if narrowed, ok := compatibleValueWitnessType(valueType, constraintType); ok {
		refined = product.Set(reg, refined, typewitness.Key, typewitness.Top())
		refined = typevalue.WithWitness(reg, refined, narrowed)
	}
	return product.Set(reg, refined, variantorigin.Key, origin), true
}

func compatibleValueWitnessType(valueType, constraintType typ.Type) (typ.Type, bool) {
	return compatibleValueWitnessTypeSeen(valueType, constraintType, &typegraph.PairPath{})
}

func compatibleValueWitnessTypeSeen(valueType, constraintType typ.Type, active *typegraph.PairPath) (typ.Type, bool) {
	if valueType == nil || constraintType == nil {
		return nil, false
	}
	if !active.Enter(valueType, constraintType) {
		return nil, false
	}
	defer active.Leave(valueType, constraintType)
	valueType = subst.ExpandInstantiated(unwrap.Alias(valueType))
	constraintType = subst.ExpandInstantiated(unwrap.Alias(constraintType))
	switch v := unwrap.Alias(valueType).(type) {
	case *typ.Optional:
		if subtype.IsSubtype(typ.Nil, constraintType) {
			return typ.Nil, true
		}
		return compatibleValueWitnessTypeSeen(v.Inner, constraintType, active)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if narrowed, ok := compatibleValueWitnessTypeSeen(member, constraintType, active); ok {
				out = append(out, narrowed)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return typenormalize.UnionForEvidence(out...), true
	case *typ.Intersection:
		for _, member := range v.Members {
			if narrowed, ok := compatibleValueWitnessTypeSeen(member, constraintType, active); ok {
				return narrowed, true
			}
		}
		return nil, false
	default:
		if openVariantConstraintAdmitsValue(valueType, constraintType) {
			return valueType, true
		}
		return nil, false
	}
}

func openVariantConstraintAdmitsValue(valueType, constraintType typ.Type) bool {
	return openVariantConstraintAdmitsValueSeen(valueType, constraintType, &typegraph.PairPath{})
}

func openVariantConstraintAdmitsValueSeen(valueType, constraintType typ.Type, active *typegraph.PairPath) bool {
	if valueType == nil || constraintType == nil {
		return false
	}
	if !active.Enter(valueType, constraintType) {
		return true
	}
	defer active.Leave(valueType, constraintType)
	if typerefine.ContainsFreeTypeParam(constraintType) {
		switch unwrap.Alias(constraintType).(type) {
		case *typ.TypeParam, *typ.Ref:
			return true
		}
	}
	valueType = subst.ExpandInstantiated(unwrap.Alias(valueType))
	constraintType = subst.ExpandInstantiated(unwrap.Alias(constraintType))
	switch v := unwrap.Alias(valueType).(type) {
	case *typ.Optional:
		if subtype.IsSubtype(typ.Nil, constraintType) {
			return true
		}
		return openVariantConstraintAdmitsValueSeen(v.Inner, constraintType, active)
	case *typ.Union:
		for _, member := range v.Members {
			if openVariantConstraintAdmitsValueSeen(member, constraintType, active) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range v.Members {
			if openVariantConstraintAdmitsValueSeen(member, constraintType, active) {
				return true
			}
		}
		return false
	}
	switch c := constraintType.(type) {
	case *typ.TypeParam, *typ.Ref:
		return true
	case *typ.Annotated:
		return openVariantConstraintAdmitsValueSeen(valueType, c.Inner, active)
	case *typ.Optional:
		if subtype.IsSubtype(typ.Nil, valueType) {
			return true
		}
		return openVariantConstraintAdmitsValueSeen(valueType, c.Inner, active)
	case *typ.Union:
		for _, member := range c.Members {
			if openVariantConstraintAdmitsValueSeen(valueType, member, active) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range c.Members {
			if !openVariantConstraintAdmitsValueSeen(valueType, member, active) {
				return false
			}
		}
		return true
	case *typ.Record:
		valueRecord, ok := unwrap.Alias(valueType).(*typ.Record)
		if !ok {
			return false
		}
		for _, wantField := range c.Fields {
			gotField := valueRecord.GetField(wantField.Name)
			if gotField == nil {
				if wantField.Optional {
					continue
				}
				return false
			}
			if !wantField.Optional && gotField.Optional {
				return false
			}
			if !openVariantConstraintAdmitsValueSeen(gotField.Type, wantField.Type, active) {
				return false
			}
		}
		return true
	default:
		return subtype.IsSubtype(valueType, constraintType)
	}
}
