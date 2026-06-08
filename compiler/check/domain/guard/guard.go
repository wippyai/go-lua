package guard

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TypeProbe describes a builtin type(expr) equality check.
type TypeProbe struct {
	Expr ast.Expr
	Key  narrow.TypeKey
}

// TypeProbeComparison describes a builtin type(expr) comparison. Equal is
// true for `==` and false for `~=`.
type TypeProbeComparison struct {
	Probe TypeProbe
	Equal bool
}

// ExtractTypeProbeComparison extracts the runtime type predicate from a
// `type(expr) == "kind"` or `type(expr) ~= "kind"` comparison.
func ExtractTypeProbeComparison(expr ast.Expr) (TypeProbeComparison, bool) {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok || rel == nil {
		return TypeProbeComparison{}, false
	}
	equal := false
	switch rel.Operator {
	case "==":
		equal = true
	case "~=":
		equal = false
	default:
		return TypeProbeComparison{}, false
	}
	if probe, ok := typeProbeSide(rel.Lhs, rel.Rhs); ok {
		return TypeProbeComparison{Probe: probe, Equal: equal}, true
	}
	if probe, ok := typeProbeSide(rel.Rhs, rel.Lhs); ok {
		return TypeProbeComparison{Probe: probe, Equal: equal}, true
	}
	return TypeProbeComparison{}, false
}

// ExtractTypeEqualityProbe extracts the runtime type predicate from a
// `type(expr) == "kind"` comparison. It is intentionally expression-only so
// synthesis, field validation, and flow guard collection share one parser.
func ExtractTypeEqualityProbe(expr ast.Expr) (TypeProbe, bool) {
	cmp, ok := ExtractTypeProbeComparison(expr)
	if !ok || !cmp.Equal {
		return TypeProbe{}, false
	}
	return cmp.Probe, true
}

// IsTypeCall reports whether call has builtin type(expr) shape.
func IsTypeCall(call *ast.FuncCallExpr) bool {
	if call == nil || callsite.IsMethodLikeExpr(call) || len(call.Args) != 1 {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	return ok && ident != nil && ident.Value == "type"
}

// TypeForTypeKey returns the broad runtime type represented by a builtin
// type() result key.
func TypeForTypeKey(key narrow.TypeKey) typ.Type {
	if kind, ok := key.BuiltinKind(); ok {
		return narrow.TypeForKind(kind)
	}
	return typ.Unknown
}

// EvaluateTypeProbeComparison returns the boolean singleton for a builtin
// type() comparison when the current abstract value proves it. It returns
// typ.Boolean when gradual or union uncertainty remains, and typ.Never when
// the observed value is unreachable.
func EvaluateTypeProbeComparison(observed typ.Type, cmp TypeProbeComparison) typ.Type {
	truth, known := TypeProbeEqualityTruth(observed, cmp.Probe.Key)
	if !known {
		return typ.Boolean
	}
	if typ.IsNever(truth) {
		return typ.Never
	}
	if !cmp.Equal {
		if truth == typ.True {
			return typ.False
		}
		return typ.True
	}
	return truth
}

// TypeProbeEqualityTruth evaluates `type(observed) == key` as a singleton
// boolean when the abstract type is precise enough to prove or refute it.
func TypeProbeEqualityTruth(observed typ.Type, key narrow.TypeKey) (typ.Type, bool) {
	if observed == nil {
		return typ.Boolean, false
	}
	observed = typ.UnwrapAnnotated(unwrap.Alias(observed))
	if typ.IsNever(observed) {
		return typ.Never, true
	}
	if observed.Kind().IsPlaceholder() {
		return typ.Boolean, false
	}
	target := TypeForTypeKey(key)
	target = typ.UnwrapAnnotated(unwrap.Alias(target))
	if target == nil || typ.IsAny(target) || typ.IsUnknown(target) {
		return typ.Boolean, false
	}
	if subtype.IsSubtype(observed, target) {
		return typ.True, true
	}
	if !narrow.TypesOverlap(observed, target) {
		return typ.False, true
	}
	return typ.Boolean, false
}

func typeProbeSide(typeExpr, keyExpr ast.Expr) (TypeProbe, bool) {
	call, ok := typeExpr.(*ast.FuncCallExpr)
	if !ok || !IsTypeCall(call) {
		return TypeProbe{}, false
	}
	typeName, ok := typeStringLiteral(keyExpr)
	if !ok {
		return TypeProbe{}, false
	}
	typeKey, ok := narrow.KnownBuiltinTypeKey(typeName)
	if !ok {
		return TypeProbe{}, false
	}
	return TypeProbe{Expr: call.Args[0], Key: typeKey}, true
}

func typeStringLiteral(expr ast.Expr) (string, bool) {
	if v, ok := expr.(*ast.StringExpr); ok {
		return v.Value, true
	}
	return "", false
}
