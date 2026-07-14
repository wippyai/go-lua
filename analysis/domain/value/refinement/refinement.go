package refinement

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	typerefine "github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// CanBeFalse reports whether value's present type evidence may be boolean false.
// Missing or unknown evidence is treated as admitting false so branch narrowing
// remains sound.
func CanBeFalse(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return true
	}
	return typeAdmitsFalse(t, 0)
}

func typeAdmitsFalse(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return typeAdmitsFalse(v.Inner, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if typeAdmitsFalse(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return typeAdmitsFalse(v.UnaliasedTarget(), depth+1)
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple, *typ.Interface, *typ.Function:
		return false
	default:
		if typ.IsNever(t) {
			return false
		}
		return subtype.IsSubtype(typ.False, t)
	}
}

// CanBeTruthy reports whether value's stable type evidence may evaluate truthy
// under Lua truthiness. Missing or unknown evidence is treated as admitting
// truth so branch narrowing remains sound.
func CanBeTruthy(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return true
	}
	return typeAdmitsTruthy(t, 0)
}

// CanBeFalsy reports whether value may evaluate falsy under Lua truthiness.
// Missing or unknown evidence is treated as admitting falsy so branch pruning
// remains sound.
func CanBeFalsy(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return true
	}
	return typeAdmitsFalsy(t, 0)
}

func typeAdmitsTruthy(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return typeAdmitsTruthy(v.Inner, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if typeAdmitsTruthy(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return typeAdmitsTruthy(v.UnaliasedTarget(), depth+1)
	default:
		if typ.IsNever(t) {
			return false
		}
		return !subtype.IsSubtype(t, typ.Nil) && !subtype.IsSubtype(t, typ.False)
	}
}

func typeAdmitsFalsy(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return true
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, member := range v.Members {
			if typeAdmitsFalsy(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return typeAdmitsFalsy(v.UnaliasedTarget(), depth+1)
	default:
		if typ.IsNever(t) {
			return false
		}
		return subtype.IsSubtype(typ.Nil, t) || subtype.IsSubtype(typ.False, t)
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
		narrowed, ok := compatibleValueWitnessType(valueType, constraintType, 0)
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
		if openVariantConstraintAdmitsValue(valueType, constraintType, 0) {
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
		if openVariantConstraintAdmitsValue(caseType, constraintType, 0) {
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
	if !ok || !openVariantConstraintAdmitsValue(valueType, constraintType, 0) {
		return product.Value{}, false
	}
	constraintWithoutOrigin := product.Set(reg, constraint, variantorigin.Key, variantorigin.Top())
	refined := product.Meet(reg, value, constraintWithoutOrigin)
	if product.Equal(reg, refined, product.Bottom(reg)) {
		refined = value
	}
	if narrowed, ok := compatibleValueWitnessType(valueType, constraintType, 0); ok {
		refined = product.Set(reg, refined, typewitness.Key, typewitness.Top())
		refined = typevalue.WithWitness(reg, refined, narrowed)
	}
	return product.Set(reg, refined, variantorigin.Key, origin), true
}

func compatibleValueWitnessType(valueType, constraintType typ.Type, depth int) (typ.Type, bool) {
	if valueType == nil || constraintType == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	valueType = subst.ExpandInstantiated(unwrap.Alias(valueType))
	constraintType = subst.ExpandInstantiated(unwrap.Alias(constraintType))
	switch v := unwrap.Alias(valueType).(type) {
	case *typ.Optional:
		if subtype.IsSubtype(typ.Nil, constraintType) {
			return typ.Nil, true
		}
		return compatibleValueWitnessType(v.Inner, constraintType, depth+1)
	case *typ.Union:
		out := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if narrowed, ok := compatibleValueWitnessType(member, constraintType, depth+1); ok {
				out = append(out, narrowed)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return typenormalize.UnionForEvidence(out...), true
	case *typ.Intersection:
		for _, member := range v.Members {
			if narrowed, ok := compatibleValueWitnessType(member, constraintType, depth+1); ok {
				return narrowed, true
			}
		}
		return nil, false
	default:
		if openVariantConstraintAdmitsValue(valueType, constraintType, depth+1) {
			return valueType, true
		}
		return nil, false
	}
}

func openVariantConstraintAdmitsValue(valueType, constraintType typ.Type, depth int) bool {
	if valueType == nil || constraintType == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
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
		return openVariantConstraintAdmitsValue(v.Inner, constraintType, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if openVariantConstraintAdmitsValue(member, constraintType, depth+1) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range v.Members {
			if openVariantConstraintAdmitsValue(member, constraintType, depth+1) {
				return true
			}
		}
		return false
	}
	switch c := constraintType.(type) {
	case *typ.TypeParam, *typ.Ref:
		return true
	case *typ.Annotated:
		return openVariantConstraintAdmitsValue(valueType, c.Inner, depth+1)
	case *typ.Optional:
		if subtype.IsSubtype(typ.Nil, valueType) {
			return true
		}
		return openVariantConstraintAdmitsValue(valueType, c.Inner, depth+1)
	case *typ.Union:
		for _, member := range c.Members {
			if openVariantConstraintAdmitsValue(valueType, member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Intersection:
		for _, member := range c.Members {
			if !openVariantConstraintAdmitsValue(valueType, member, depth+1) {
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
			if !openVariantConstraintAdmitsValue(gotField.Type, wantField.Type, depth+1) {
				return false
			}
		}
		return true
	default:
		return subtype.IsSubtype(valueType, constraintType)
	}
}
