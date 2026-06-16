package refinement

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// CanBeFalse reports whether value's present type evidence may be boolean false.
// Missing or unknown evidence is treated as admitting false so branch narrowing
// remains sound.
func CanBeFalse(reg *axis.Registry, value product.Value) bool {
	witness := product.Get(reg, value, typewitness.Key)
	t, ok := witness.Type()
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

// MeetConstraint refines value with constraint and recovers compatible
// type-witness or variant-origin evidence when sparse-axis identity would make a
// sound subtype refinement collapse to bottom.
func MeetConstraint(reg *axis.Registry, value, constraint product.Value) product.Value {
	refined := product.Meet(reg, value, constraint)
	if !product.Equal(reg, refined, product.Bottom(reg)) {
		return refined
	}
	if recovered, ok := recoverCompatibleVariantOriginMeet(reg, value, constraint); ok {
		return recovered
	}
	if recovered, ok := recoverCompatibleWitnessMeet(reg, value, constraint); ok {
		return recovered
	}
	return refined
}

func recoverCompatibleWitnessMeet(reg *axis.Registry, value, constraint product.Value) (product.Value, bool) {
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
		return product.Value{}, false
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
	valueOrigin := product.Get(reg, value, variantorigin.Key)
	constraintOrigin := product.Get(reg, constraint, variantorigin.Key)
	if valueOrigin.IsTop() || valueOrigin.IsBottom() ||
		constraintOrigin.IsTop() || constraintOrigin.IsBottom() ||
		valueOrigin.Family() == constraintOrigin.Family() {
		return product.Value{}, false
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
		return product.Value{}, false
	}
	valueWithoutOrigin := product.Set(reg, value, variantorigin.Key, variantorigin.Top())
	constraintWithoutOrigin := product.Set(reg, constraint, variantorigin.Key, variantorigin.Top())
	refined := product.Meet(reg, valueWithoutOrigin, constraintWithoutOrigin)
	if product.Equal(reg, refined, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return product.Set(reg, refined, variantorigin.Key, constraintOrigin), true
}
