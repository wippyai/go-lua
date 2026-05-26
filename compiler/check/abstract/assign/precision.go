package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/domain/value"
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
	if !singleTarget && !typ.IsAny(assignedType) {
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
		if preferNamedEquivalentDirectType(precise, assignedType) {
			return precise
		}
		if typ.ContainsRecursive(precise) || typ.ContainsRecursive(assignedType) {
			if refines, changed := value.RefinesSoftContainer(precise, assignedType); refines && changed {
				return precise
			}
			return assignedType
		}
		if typ.MorePrecise(precise, assignedType) {
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

func mergeUnannotatedParamType(current, inferred typ.Type) typ.Type {
	if typ.IsAbsentOrUnknown(inferred) || typ.IsAny(inferred) {
		return current
	}
	if typ.IsAny(current) {
		return current
	}
	if current == nil || current.Kind().IsPlaceholder() || typ.IsUnknown(current) {
		return inferred
	}
	if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(inferred, current); ok && reconciled != nil {
		return reconciled
	}
	if inner, nilable := typ.SplitNilableFieldType(inferred); nilable {
		if typ.TypeEquals(current, inner) || subtype.IsSubtype(current, inner) {
			return current
		}
	}
	if subtype.IsSubtype(current, inferred) {
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
	if typ.MorePrecise(inferred, declared) {
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
	if typ.MorePrecise(inferred, iterType) {
		return inferred
	}
	return iterType
}
