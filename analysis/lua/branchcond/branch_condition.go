// Package branchcond recognizes normalized Lua branch-condition checks.
package branchcond

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

type CheckKind uint8

const (
	CheckNone CheckKind = iota
	CheckTruthy
	CheckFalsy
	CheckNil
	CheckNotNil
	CheckTypeEqual
	CheckTypeNot
	CheckLiteralEqual
	CheckLiteralNot
	CheckPathEqual
	CheckPathNot
)

type Check struct {
	Kind          CheckKind
	Path          path.Path
	OtherPath     path.Path
	TypeName      string
	LiteralString string
}

// PredicateCall returns the direct call whose boolean result selects a branch.
// The negated flag is true when the branch condition is `not call(...)`.
func PredicateCall(expr ast.Expr) (*ast.FuncCallExpr, bool, bool) {
	switch expr := sourceprovenance.AssertionInner(expr).(type) {
	case *ast.FuncCallExpr:
		return expr, false, true
	case *ast.UnaryNotOpExpr:
		call, ok := sourceprovenance.Call(expr.Expr)
		return call, true, ok
	default:
		return nil, false, false
	}
}

// TypeCall reports whether expr is a direct one-argument `type(...)` call
// shape after stripping assertion wrappers. Semantic resolution of the callee
// stays with the caller.
func TypeCall(expr ast.Expr) (*ast.FuncCallExpr, bool) {
	return typeCallShape(expr, "", false)
}

// TypeIsCall reports whether expr is a direct one-argument `:is(...)` call
// shape after stripping assertion wrappers. Semantic resolution of the
// receiver stays with the caller.
func TypeIsCall(expr ast.Expr) (*ast.FuncCallExpr, bool) {
	return typeCallShape(expr, "is", true)
}

func Normalize(expr ast.Expr, bindings *bind.Result) Check {
	if p, ok := pathexpr.Resolve(expr, bindings); ok {
		return Check{Kind: CheckTruthy, Path: p}
	}

	switch expr := expr.(type) {
	case *ast.UnaryNotOpExpr:
		if p, ok := pathexpr.Resolve(expr.Expr, bindings); ok {
			return Check{Kind: CheckFalsy, Path: p}
		}
	case *ast.RelationalOpExpr:
		if !isSupportedRelop(expr.Operator) {
			return Check{}
		}
		if check, ok := normalizeTypeComparison(expr, bindings); ok {
			return check
		}
		if check, ok := normalizeStringLiteralComparison(expr, bindings); ok {
			return check
		}
		if p, ok := nilComparisonPath(expr.Lhs, expr.Rhs, bindings); ok {
			kind := CheckNil
			if expr.Operator == "~=" {
				kind = CheckNotNil
			}
			return Check{Kind: kind, Path: p}
		}
		if check, ok := normalizePathComparison(expr, bindings); ok {
			return check
		}
	}

	return Check{}
}

// TruthyChecks returns checks that must all hold when expr is truthy. For
// conjunctions, Lua's true result proves both sides; for disjunctions it does
// not prove either side individually.
func TruthyChecks(expr ast.Expr, bindings *bind.Result) []Check {
	check := Normalize(expr, bindings)
	if check.Kind != CheckNone {
		return []Check{check}
	}
	if unary, ok := expr.(*ast.UnaryNotOpExpr); ok {
		return FalsyChecks(unary.Expr, bindings)
	}
	logical, ok := expr.(*ast.LogicalOpExpr)
	if !ok || logical.Operator != "and" {
		return nil
	}
	left := TruthyChecks(logical.Lhs, bindings)
	right := TruthyChecks(logical.Rhs, bindings)
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	out := make([]Check, 0, len(left)+len(right))
	out = append(out, left...)
	out = append(out, right...)
	return out
}

// FalsyChecks returns checks that must all hold when expr is falsy. For
// disjunctions, Lua's false result proves both sides false; for conjunctions it
// does not prove either side individually.
func FalsyChecks(expr ast.Expr, bindings *bind.Result) []Check {
	check := Normalize(expr, bindings)
	if check.Kind != CheckNone {
		return []Check{check}
	}
	if unary, ok := expr.(*ast.UnaryNotOpExpr); ok {
		return TruthyChecks(unary.Expr, bindings)
	}
	logical, ok := expr.(*ast.LogicalOpExpr)
	if !ok || logical.Operator != "or" {
		return nil
	}
	left := FalsyChecks(logical.Lhs, bindings)
	right := FalsyChecks(logical.Rhs, bindings)
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	out := make([]Check, 0, len(left)+len(right))
	out = append(out, left...)
	out = append(out, right...)
	return out
}

func normalizePathComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	lhs, ok := pathexpr.Resolve(expr.Lhs, bindings)
	if !ok || lhs.IsEmpty() {
		return Check{}, false
	}
	rhs, ok := pathexpr.Resolve(expr.Rhs, bindings)
	if !ok || rhs.IsEmpty() {
		return Check{}, false
	}
	if rhs.Less(lhs) {
		lhs, rhs = rhs, lhs
	}
	kind := CheckPathEqual
	if expr.Operator == "~=" {
		kind = CheckPathNot
	}
	return Check{Kind: kind, Path: lhs, OtherPath: rhs}, true
}

func normalizeStringLiteralComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	p, value, ok := stringLiteralComparisonOperands(expr.Lhs, expr.Rhs, bindings)
	if !ok {
		p, value, ok = stringLiteralComparisonOperands(expr.Rhs, expr.Lhs, bindings)
	}
	if !ok {
		return Check{}, false
	}
	kind := CheckLiteralEqual
	if expr.Operator == "~=" {
		kind = CheckLiteralNot
	}
	return Check{Kind: kind, Path: p, LiteralString: value}, true
}

func stringLiteralComparisonOperands(pathExpr, literalExpr ast.Expr, bindings *bind.Result) (path.Path, string, bool) {
	lit, ok := literalExpr.(*ast.StringExpr)
	if !ok {
		return path.Path{}, "", false
	}
	p, ok := pathexpr.Resolve(pathExpr, bindings)
	if !ok || p.IsEmpty() {
		return path.Path{}, "", false
	}
	return p, lit.Value, true
}

func SupportsTypeComparison(expr ast.Expr, bindings *bind.Result) bool {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok || !isSupportedRelop(rel.Operator) {
		return false
	}
	_, ok = normalizeTypeComparison(rel, bindings)
	return ok
}

func normalizeTypeComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	p, typeName, ok := typeComparisonOperands(expr.Lhs, expr.Rhs, bindings)
	if !ok {
		p, typeName, ok = typeComparisonOperands(expr.Rhs, expr.Lhs, bindings)
	}
	if !ok {
		return Check{}, false
	}
	kind := CheckTypeEqual
	if expr.Operator == "~=" {
		kind = CheckTypeNot
	}
	return Check{Kind: kind, Path: p, TypeName: typeName}, true
}

func typeComparisonOperands(callExpr, literalExpr ast.Expr, bindings *bind.Result) (path.Path, string, bool) {
	lit, ok := literalExpr.(*ast.StringExpr)
	if !ok {
		return path.Path{}, "", false
	}
	call, ok := TypeCall(callExpr)
	if !ok {
		return path.Path{}, "", false
	}
	p, ok := typeCallSubjectPath(call, bindings)
	if !ok {
		return path.Path{}, "", false
	}
	return p, lit.Value, true
}

func typeCallSubjectPath(call *ast.FuncCallExpr, bindings *bind.Result) (path.Path, bool) {
	fn, ok := call.Func.(*ast.IdentExpr)
	if !ok || !bindings.ResolvesToGlobal(fn, "type") {
		return path.Path{}, false
	}
	return pathexpr.Resolve(call.Args[0], bindings)
}

func typeCallShape(expr ast.Expr, method string, hasReceiver bool) (*ast.FuncCallExpr, bool) {
	call, ok := sourceprovenance.AssertionInner(expr).(*ast.FuncCallExpr)
	if !ok || call == nil || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return nil, false
	}
	if hasReceiver {
		if call.Receiver == nil || call.Method != method {
			return nil, false
		}
		return call, true
	}
	if call.Receiver != nil || call.Method != "" {
		return nil, false
	}
	return call, true
}

func nilComparisonPath(lhs, rhs ast.Expr, bindings *bind.Result) (path.Path, bool) {
	if _, ok := lhs.(*ast.NilExpr); ok {
		return pathexpr.Resolve(rhs, bindings)
	}
	if _, ok := rhs.(*ast.NilExpr); ok {
		return pathexpr.Resolve(lhs, bindings)
	}
	return path.Path{}, false
}

func isSupportedRelop(op string) bool {
	return op == "==" || op == "~="
}
