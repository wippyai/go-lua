package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// preferPreciseDirectSourceType keeps assignment inference on the canonical
// expression-synthesis path.
//
// Direct synthesis is allowed to repair a slot only when it is strictly more
// informative than the expanded assignment value. This keeps tuple expansion as
// the primary source of truth for assignment slots while still allowing
// canonical single-expression synthesis to repair top-like degradation.
func preferPreciseDirectSourceType(
	assignedType typ.Type,
	source ast.Expr,
	p cfg.Point,
	sc *scope.State,
	synth func(ast.Expr, cfg.Point) typ.Type,
	singleTarget bool,
) typ.Type {
	if source == nil || synth == nil {
		return assignedType
	}
	switch source.(type) {
	case *ast.Comma3Expr:
		return assignedType
	}

	precise := resolve.Ref(synth(source, p), sc)
	if typ.IsAbsentOrUnknown(precise) {
		return assignedType
	}
	if singleTarget {
		if typ.IsAbsentOrUnknown(assignedType) || typ.IsAny(assignedType) {
			return precise
		}
		if subtype.IsSubtype(precise, assignedType) && !subtype.IsSubtype(assignedType, precise) {
			return precise
		}
		if preferNamedEquivalentDirectType(precise, assignedType) {
			return precise
		}
		if sameExpressionHasMoreEvidence(precise, assignedType) {
			return precise
		}
		return assignedType
	}
	if typ.IsAny(assignedType) && !typ.IsAny(precise) {
		return precise
	}
	return assignedType
}

func preferNamedEquivalentDirectType(precise, assignedType typ.Type) bool {
	if !isNamedIdentityType(precise) || isNamedIdentityType(assignedType) {
		return false
	}
	return subtype.IsSubtype(precise, assignedType) && subtype.IsSubtype(assignedType, precise)
}

func isNamedIdentityType(t typ.Type) bool {
	switch t.(type) {
	case *typ.Alias, *typ.Ref:
		return true
	default:
		return false
	}
}

func sameExpressionHasMoreEvidence(precise, assigned typ.Type) bool {
	improved, ok := compareSameExpressionEvidence(precise, assigned, 0)
	return ok && improved
}

func mergeUnannotatedParamType(current, inferred typ.Type) typ.Type {
	if typ.IsAbsentOrUnknown(inferred) || typ.IsAny(inferred) {
		return current
	}
	if current == nil || current.Kind().IsPlaceholder() || typ.IsUnknown(current) {
		return inferred
	}
	if typ.IsAny(current) || subtype.IsSubtype(current, inferred) {
		return current
	}
	return inferred
}

func inferredOverridesUnannotatedDeclared(inferred, declared typ.Type) bool {
	if typ.IsAbsentOrUnknown(inferred) {
		return false
	}
	if declared == nil || typ.IsAbsentOrUnknown(declared) || declared.Kind().IsPlaceholder() || typ.IsSoft(declared, typ.SoftAnnotationPolicy) {
		return true
	}
	if typ.IsAny(declared) {
		return false
	}
	if subtype.IsSubtype(inferred, declared) && !subtype.IsSubtype(declared, inferred) {
		return true
	}
	if sameExpressionHasMoreEvidence(inferred, declared) {
		return true
	}
	return false
}

func informativeLoopVarType(t typ.Type) bool {
	return t != nil && !typ.IsAbsentOrUnknown(t) && !typ.IsAny(t) && !t.Kind().IsPlaceholder()
}

func refineLoopVarTypeFromInference(iterType, inferred typ.Type) typ.Type {
	if !informativeLoopVarType(inferred) {
		return iterType
	}
	if typ.IsAbsentOrUnknown(iterType) || typ.IsAny(iterType) || iterType.Kind().IsPlaceholder() || typ.IsSoft(iterType, typ.SoftAnnotationPolicy) {
		return inferred
	}
	if subtype.IsSubtype(inferred, iterType) && !subtype.IsSubtype(iterType, inferred) {
		return inferred
	}
	if sameExpressionHasMoreEvidence(inferred, iterType) {
		return inferred
	}
	return iterType
}

func compareSameExpressionEvidence(precise, assigned typ.Type, depth int) (bool, bool) {
	if depth > typ.DefaultRecursionDepth {
		return false, false
	}
	if typ.TypeEquals(precise, assigned) {
		return false, true
	}
	if typ.IsAbsentOrUnknown(assigned) {
		return !typ.IsAbsentOrUnknown(precise), true
	}
	if typ.IsAbsentOrUnknown(precise) {
		return false, false
	}

	switch p := precise.(type) {
	case *typ.Alias:
		return compareSameExpressionEvidence(p.UnaliasedTarget(), assigned, depth+1)
	case *typ.Ref:
		if a, ok := assigned.(*typ.Alias); ok && a.Name == p.Name && p.Module == "" {
			return false, true
		}
	}
	switch a := assigned.(type) {
	case *typ.Alias:
		return compareSameExpressionEvidence(precise, a.UnaliasedTarget(), depth+1)
	case *typ.Ref:
		if p, ok := precise.(*typ.Alias); ok && p.Name == a.Name && a.Module == "" {
			return false, true
		}
	}

	switch p := precise.(type) {
	case *typ.Record:
		a, ok := assigned.(*typ.Record)
		if !ok {
			return false, false
		}
		return compareRecordEvidence(p, a, depth+1)
	case *typ.Optional:
		a, ok := assigned.(*typ.Optional)
		if !ok {
			return false, false
		}
		return compareSameExpressionEvidence(p.Inner, a.Inner, depth+1)
	case *typ.Tuple:
		a, ok := assigned.(*typ.Tuple)
		if !ok || len(p.Elements) != len(a.Elements) {
			return false, false
		}
		improved := false
		for i := range p.Elements {
			fieldImproved, ok := compareSameExpressionEvidence(p.Elements[i], a.Elements[i], depth+1)
			if !ok {
				return false, false
			}
			improved = improved || fieldImproved
		}
		return improved, true
	case *typ.Array:
		a, ok := assigned.(*typ.Array)
		if !ok {
			return false, false
		}
		return compareSameExpressionEvidence(p.Element, a.Element, depth+1)
	case *typ.Map:
		a, ok := assigned.(*typ.Map)
		if !ok {
			return false, false
		}
		keyImproved, ok := compareSameExpressionEvidence(p.Key, a.Key, depth+1)
		if !ok {
			return false, false
		}
		valueImproved, ok := compareSameExpressionEvidence(p.Value, a.Value, depth+1)
		if !ok {
			return false, false
		}
		return keyImproved || valueImproved, true
	default:
		return false, false
	}
}

func compareRecordEvidence(precise, assigned *typ.Record, depth int) (bool, bool) {
	if precise == nil || assigned == nil {
		return false, false
	}
	if precise.Open != assigned.Open {
		return false, false
	}
	if (precise.HasMapComponent()) != (assigned.HasMapComponent()) {
		return false, false
	}
	improved := false
	for _, assignedField := range assigned.Fields {
		preciseField := precise.GetField(assignedField.Name)
		if preciseField == nil {
			return false, false
		}
		if preciseField.Optional != assignedField.Optional || preciseField.Readonly != assignedField.Readonly {
			return false, false
		}
		fieldImproved, ok := compareSameExpressionEvidence(preciseField.Type, assignedField.Type, depth+1)
		if !ok {
			return false, false
		}
		improved = improved || fieldImproved
	}
	if assigned.HasMapComponent() {
		keyImproved, ok := compareSameExpressionEvidence(precise.MapKey, assigned.MapKey, depth+1)
		if !ok {
			return false, false
		}
		valueImproved, ok := compareSameExpressionEvidence(precise.MapValue, assigned.MapValue, depth+1)
		if !ok {
			return false, false
		}
		improved = improved || keyImproved || valueImproved
	}
	return improved, true
}
