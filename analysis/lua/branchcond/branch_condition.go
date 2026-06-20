// Package branchcond recognizes normalized Lua branch-condition checks.
package branchcond

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
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
	CheckLenGe
	CheckIndexInRange
	CheckNumGe
)

type Check struct {
	Kind          CheckKind
	Path          path.Path
	OtherPath     path.Path
	TypeName      string
	Literal       typ.Type
	LiteralString string
	LenFloor      int64
	NumFloor      int64
}

func (c Check) LiteralValue() (typ.Type, bool) {
	if c.Literal != nil {
		return c.Literal, true
	}
	if c.Kind == CheckLiteralEqual || c.Kind == CheckLiteralNot {
		return typ.LiteralString(c.LiteralString), true
	}
	return nil, false
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
		if check, ok := normalizeLengthFloorComparison(expr, bindings); ok {
			return check
		}
		if check, ok := normalizeIndexInRangeComparison(expr, bindings); ok {
			return check
		}
		if check, ok := normalizeNumericFloorComparison(expr, bindings); ok {
			return check
		}
		if !isEqualityRelop(expr.Operator) {
			return Check{}
		}
		if check, ok := normalizeTypeComparison(expr, bindings); ok {
			return check
		}
		if check, ok := normalizeLiteralComparison(expr, bindings); ok {
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
	return polarityChecks(expr, bindings, true)
}

// FalsyChecks returns checks that must all hold when expr is falsy. For
// disjunctions, Lua's false result proves both sides false; for conjunctions it
// does not prove either side individually.
func FalsyChecks(expr ast.Expr, bindings *bind.Result) []Check {
	return polarityChecks(expr, bindings, false)
}

// polarityChecks collects the narrowing checks implied when expr holds the given
// truth polarity. A truthy `and` (or falsy `or`) proves both operands; `not`
// flips polarity; any other shape proves nothing individually.
func polarityChecks(expr ast.Expr, bindings *bind.Result, truthy bool) []Check {
	check := Normalize(expr, bindings)
	if check.Kind != CheckNone {
		return []Check{check}
	}
	if unary, ok := expr.(*ast.UnaryNotOpExpr); ok {
		return polarityChecks(unary.Expr, bindings, !truthy)
	}
	splitOp := "and"
	if !truthy {
		splitOp = "or"
	}
	logical, ok := expr.(*ast.LogicalOpExpr)
	if !ok || logical.Operator != splitOp {
		return nil
	}
	left := polarityChecks(logical.Lhs, bindings, truthy)
	right := polarityChecks(logical.Rhs, bindings, truthy)
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

func normalizeLiteralComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	p, lit, kind, ok := resolveEqualityComparison(expr, bindings, literalComparisonOperands, CheckLiteralEqual, CheckLiteralNot)
	if !ok {
		return Check{}, false
	}
	check := Check{Kind: kind, Path: p, Literal: lit}
	if literal, ok := lit.(*typ.Literal); ok {
		if value, ok := literal.Value.(string); ok {
			check.LiteralString = value
		}
	}
	return check, true
}

// resolveEqualityComparison resolves operands in either order and selects eq for
// `==` or ne for `~=`, returning the path, the operand, and the kind.
func resolveEqualityComparison[T any](
	expr *ast.RelationalOpExpr,
	bindings *bind.Result,
	operands func(ast.Expr, ast.Expr, *bind.Result) (path.Path, T, bool),
	eq, ne CheckKind,
) (path.Path, T, CheckKind, bool) {
	p, value, ok := operands(expr.Lhs, expr.Rhs, bindings)
	if !ok {
		p, value, ok = operands(expr.Rhs, expr.Lhs, bindings)
	}
	if !ok {
		var zero T
		return path.Path{}, zero, 0, false
	}
	kind := eq
	if expr.Operator == "~=" {
		kind = ne
	}
	return p, value, kind, true
}

func literalComparisonOperands(pathExpr, literalExpr ast.Expr, bindings *bind.Result) (path.Path, typ.Type, bool) {
	lit, ok := valueexpr.LiteralType(literalExpr)
	if !ok || typ.TypeEquals(lit, typ.Nil) {
		return path.Path{}, nil, false
	}
	p, ok := pathexpr.Resolve(pathExpr, bindings)
	if !ok || p.IsEmpty() {
		return path.Path{}, nil, false
	}
	return p, lit, true
}

func SupportsTypeComparison(expr ast.Expr, bindings *bind.Result) bool {
	rel, ok := expr.(*ast.RelationalOpExpr)
	if !ok || !isEqualityRelop(rel.Operator) {
		return false
	}
	_, ok = normalizeTypeComparison(rel, bindings)
	return ok
}

func normalizeTypeComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	// type(path) == "literal": the type name is known syntactically.
	if p, typeName, kind, ok := resolveEqualityComparison(expr, bindings, typeComparisonOperands, CheckTypeEqual, CheckTypeNot); ok {
		return Check{Kind: kind, Path: p, TypeName: typeName}, true
	}
	// type(path) == otherPath: the right side is a value path whose type may be a
	// single string literal (e.g. a kind: "number" field or local). The literal
	// type name is resolved where type facts are available; consumers that have
	// no type name leave this comparison un-narrowing.
	if subject, other, kind, ok := resolveEqualityComparison(expr, bindings, typeComparisonPathOperands, CheckTypeEqual, CheckTypeNot); ok {
		return Check{Kind: kind, Path: subject, OtherPath: other}, true
	}
	return Check{}, false
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

func typeComparisonPathOperands(callExpr, otherExpr ast.Expr, bindings *bind.Result) (path.Path, path.Path, bool) {
	if _, ok := otherExpr.(*ast.StringExpr); ok {
		return path.Path{}, path.Path{}, false
	}
	call, ok := TypeCall(callExpr)
	if !ok {
		return path.Path{}, path.Path{}, false
	}
	subject, ok := typeCallSubjectPath(call, bindings)
	if !ok {
		return path.Path{}, path.Path{}, false
	}
	other, ok := pathexpr.Resolve(otherExpr, bindings)
	if !ok {
		return path.Path{}, path.Path{}, false
	}
	return subject, other, true
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
	switch op {
	case "==", "~=", "<", ">", "<=", ">=":
		return true
	default:
		return false
	}
}

func isEqualityRelop(op string) bool {
	return op == "==" || op == "~="
}

// normalizeLengthFloorComparison recognizes a non-empty / lower-bound guard on
// an array length, such as #xs > 0, #xs >= 1, or #xs ~= 0, and lowers it to a
// canonical CheckLenGe{Path: xs, LenFloor: k} that holds on the true edge.
func normalizeLengthFloorComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	return normalizeFlippedComparison(expr, bindings, lengthFloorOperands,
		func(arrayPath path.Path, floor int64) Check {
			return Check{Kind: CheckLenGe, Path: arrayPath, LenFloor: floor}
		})
}

// normalizeFlippedComparison tries operands in both operand orders (flipping the
// relational operator for the second), building the check from the first match.
func normalizeFlippedComparison[A, B any](
	expr *ast.RelationalOpExpr,
	bindings *bind.Result,
	operands func(ast.Expr, string, ast.Expr, *bind.Result) (A, B, bool),
	build func(A, B) Check,
) (Check, bool) {
	if a, b, ok := operands(expr.Lhs, expr.Operator, expr.Rhs, bindings); ok {
		return build(a, b), true
	}
	if a, b, ok := operands(expr.Rhs, flipRelop(expr.Operator), expr.Lhs, bindings); ok {
		return build(a, b), true
	}
	return Check{}, false
}

// normalizeIndexInRangeComparison recognizes an index upper-bound guard such as
// `i <= #xs` or `#xs >= i`. On the true edge it proves reads `xs[i]` are
// in-range when paired with a positive index fact.
func normalizeIndexInRangeComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	return normalizeFlippedComparison(expr, bindings, indexLenBoundOperands,
		func(indexPath, arrayPath path.Path) Check {
			return Check{Kind: CheckIndexInRange, Path: indexPath, OtherPath: arrayPath}
		})
}

// normalizeNumericFloorComparison recognizes a numeric lower-bound guard such as
// `i >= 1`, `i > 0`, or the flipped `1 <= i`, producing CheckNumGe{Path: i,
// NumFloor: k}. It supplies the positive-index proof that pairs with an
// index-in-range guard to remove the optional nil from an array read.
func normalizeNumericFloorComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	return normalizeFlippedComparison(expr, bindings, numericFloorOperands,
		func(numPath path.Path, floor int64) Check {
			return Check{Kind: CheckNumGe, Path: numPath, NumFloor: floor}
		})
}

// numericFloorOperands matches `path <op> const` and returns the numeric path and
// the proven lower bound on its value when op establishes a non-negative floor. A
// floor of 0 (`j >= 0`) feeds relational sum proofs such as i + j <= #xs.
func numericFloorOperands(numExpr ast.Expr, op string, constExpr ast.Expr, bindings *bind.Result) (path.Path, int64, bool) {
	numPath, ok := pathexpr.Resolve(numExpr, bindings)
	if !ok || numPath.IsEmpty() {
		return path.Path{}, 0, false
	}
	c, ok := constExpr.(*ast.NumberExpr)
	if !ok {
		return path.Path{}, 0, false
	}
	value, ok := numparse.ParseIntegerLiteral(c.Value)
	if !ok {
		return path.Path{}, 0, false
	}
	floor, ok := numericFloorForRelop(op, value)
	if !ok || floor < 0 {
		return path.Path{}, 0, false
	}
	return numPath, floor, true
}

// numericFloorForRelop computes the proven value lower bound from `path <op> c`.
func numericFloorForRelop(op string, c int64) (int64, bool) {
	switch op {
	case ">":
		return c + 1, true
	case ">=":
		return c, true
	default:
		return 0, false
	}
}

func indexLenBoundOperands(indexExpr ast.Expr, op string, lenExpr ast.Expr, bindings *bind.Result) (path.Path, path.Path, bool) {
	// `i <= #xs` proves i <= len; the strict `i < #xs` proves i <= len-1, which is
	// also in range. Both establish the index-in-range upper bound.
	if op != "<=" && op != "<" {
		return path.Path{}, path.Path{}, false
	}
	indexPath, ok := pathexpr.Resolve(indexExpr, bindings)
	if !ok || indexPath.IsEmpty() {
		return path.Path{}, path.Path{}, false
	}
	lenOp, ok := lenExpr.(*ast.UnaryLenOpExpr)
	if !ok {
		return path.Path{}, path.Path{}, false
	}
	arrayPath, ok := pathexpr.Resolve(lenOp.Expr, bindings)
	if !ok || arrayPath.IsEmpty() {
		return path.Path{}, path.Path{}, false
	}
	return indexPath, arrayPath, true
}

// lengthFloorOperands matches `#array <op> const` and returns the array path and
// the proven floor on its length when op establishes a positive lower bound.
func lengthFloorOperands(lenExpr ast.Expr, op string, constExpr ast.Expr, bindings *bind.Result) (path.Path, int64, bool) {
	lenOp, ok := lenExpr.(*ast.UnaryLenOpExpr)
	if !ok {
		return path.Path{}, 0, false
	}
	arrayPath, ok := pathexpr.Resolve(lenOp.Expr, bindings)
	if !ok || arrayPath.IsEmpty() {
		return path.Path{}, 0, false
	}
	c, ok := constExpr.(*ast.NumberExpr)
	if !ok {
		return path.Path{}, 0, false
	}
	value, ok := numparse.ParseIntegerLiteral(c.Value)
	if !ok {
		return path.Path{}, 0, false
	}
	floor, ok := lengthFloorForRelop(op, value)
	if !ok || floor <= 0 {
		return path.Path{}, 0, false
	}
	return arrayPath, floor, true
}

// lengthFloorForRelop computes the proven length lower bound from `len <op> c`.
func lengthFloorForRelop(op string, c int64) (int64, bool) {
	switch op {
	case ">":
		return c + 1, true
	case ">=":
		return c, true
	case "~=":
		// len ~= 0 proves len >= 1 since length is non-negative.
		if c == 0 {
			return 1, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func flipRelop(op string) string {
	switch op {
	case "<":
		return ">"
	case ">":
		return "<"
	case "<=":
		return ">="
	case ">=":
		return "<="
	default:
		return op
	}
}
