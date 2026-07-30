package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	variantoriginpkg "github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func ObjectLiteralType(reg *axis.Registry, lit factflow.ObjectLiteral, resolver factflow.ValueSourceResolver) (typ.Type, bool) {
	return ObjectLiteralTypeCached(reg, nil, lit, resolver)
}

func ObjectLiteralTypeCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteral, resolver factflow.ValueSourceResolver) (typ.Type, bool) {
	return ObjectLiteralTypeViewCached(reg, typeValues, lit.View(), resolver)
}

func ObjectLiteralTypeViewCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteralView, resolver factflow.ValueSourceResolver) (typ.Type, bool) {
	plan, ok := CompileObjectLiteralPlanCached(reg, typeValues, lit)
	if !ok {
		return nil, false
	}
	observations, ok := resolveObjectLiteralPlanObservations(reg, typeValues, plan, resolver)
	if !ok {
		return nil, false
	}
	return composeObjectLiteralPlanTypeFromObservationsCached(reg, typeValues, plan, observations)
}

// ObjectLiteralValueFromViewCached evaluates a lowered object literal to the
// same product value used by the checker: a type witness for the constructed
// record, optional literal identity, and fresh escape evidence.
func ObjectLiteralValueFromViewCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteralView, resolver factflow.ValueSourceResolver) (product.Value, bool) {
	plan, ok := CompileObjectLiteralPlanCached(reg, typeValues, lit)
	if !ok {
		return product.Value{}, false
	}
	observations, ok := resolveObjectLiteralPlanObservations(reg, typeValues, plan, resolver)
	if !ok {
		return product.Value{}, false
	}
	return ComposeObjectLiteralPlanFromObservationsCached(reg, typeValues, plan, observations)
}

func constructorPathFromEntry(entry factflow.ObjectEntryView) ([]typetable.ConstructorKey, bool) {
	return luatypeprojection.ConstructorPathFromSegmentReader(entry.SuffixSegmentCount(), entry.SuffixSegmentAt)
}

func untrustedObjectLiteralEntryType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) typ.Type {
	if t, ok := typeValues.TypeOf(reg, value); ok && t != nil && (typ.IsAny(t) || typ.IsUnknown(t)) {
		return t
	}
	return typ.Unknown
}

func ObjectLiteralEntryType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (typ.Type, bool) {
	if t, ok := proof.New(reg, typeValues).ValueType(value); ok {
		if ObjectLiteralEntryHasUntrustedTopOrigin(reg, value) && (typ.IsAny(t) || typ.IsUnknown(t)) {
			return nil, false
		}
		origin := product.Get(reg, value, variantoriginpkg.Key)
		if !origin.IsBottom() && !origin.IsTop() {
			if narrowed, ok := typeValues.NarrowVariantByOriginView(t, origin.Family(), origin.CasesView()); ok {
				return narrowed, true
			}
		}
		return t, true
	}
	if t, ok := runtimeKindValueType(reg, value); ok {
		return t, true
	}
	return nil, false
}

func ObjectLiteralEntryHasUntrustedTopOrigin(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsGradualTop() || ev.IsExplicitTop()
}

func ExpectedEntryAdmissibleCached(reg *axis.Registry, typeValues *typevalue.Cache, value, expected product.Value) bool {
	expectedType, hasExpected := typeValues.TypeOf(reg, expected)
	if !hasExpected || expectedType == nil {
		return true
	}
	actualType, hasActual := typeValues.TypeOf(reg, value)
	if !hasActual || actualType == nil {
		return true
	}
	return typeValues.IsFreshAssignable(actualType, expectedType)
}
