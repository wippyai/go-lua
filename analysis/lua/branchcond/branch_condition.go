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
	CheckFrozenTable
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
	// Negated is true when the bound holds on the FALSE edge of the comparison
	// rather than the true edge: e.g. `i > #xs` proves the in-range bound i <= #xs
	// on its false edge, the standard `if oob then error end` guard form. Only the
	// bound checks (CheckIndexInRange, CheckNumGe, CheckLenGe) use it.
	Negated bool
}

// ImpliedCheck is a normalized leaf check proven by taking a particular branch
// edge of a possibly-compound condition. Edge is the outer branch edge carrying
// the proof; Polarity is the truth value of the leaf check on that edge.
//
// They differ for shapes like `not (i >= 1 and i <= #xs)`: on the outer false
// edge, each inner comparison is true.
type ImpliedCheck struct {
	Check    Check
	Edge     bool
	Polarity bool
}

// ImpliedRelationalOp is a raw relational comparison proven by taking a
// particular branch edge. It is the source-preserving sibling of ImpliedCheck:
// callers that need the original expression shape, such as difference-logic
// extraction, use Expr while sharing the same boolean implication traversal.
type ImpliedRelationalOp struct {
	Expr     *ast.RelationalOpExpr
	Edge     bool
	Polarity bool
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

// TypeIsCallReceiver reports a direct one-argument `receiver:is(path)` or
// `receiver.is(path)` call and returns the receiver expression whose type value
// is being applied to the argument path.
func TypeIsCallReceiver(expr ast.Expr) (*ast.FuncCallExpr, ast.Expr, bool) {
	call, ok := TypeIsCall(expr)
	if !ok {
		return nil, nil, false
	}
	if call.Receiver != nil && call.Method == "is" {
		return call, call.Receiver, true
	}
	if call.Receiver == nil && call.Method == "" {
		attr, ok := call.Func.(*ast.AttrGetExpr)
		if ok && attr.KeySyntax == ast.AttrKeyDot && ast.KeyName(attr.Key) == "is" {
			return call, attr.Object, true
		}
	}
	return nil, nil, false
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
	case *ast.FuncCallExpr:
		if p, ok := normalizeFrozenTableCall(expr, bindings); ok {
			return Check{Kind: CheckFrozenTable, Path: p}
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

func normalizeFrozenTableCall(call *ast.FuncCallExpr, bindings *bind.Result) (path.Path, bool) {
	if call == nil || bindings == nil || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return path.Path{}, false
	}
	switch {
	case call.Receiver != nil:
		recv, ok := call.Receiver.(*ast.IdentExpr)
		if !ok || !bindings.ResolvesToGlobal(recv, "table") || call.Method != "isfrozen" {
			return path.Path{}, false
		}
	case call.Func != nil:
		attr, ok := call.Func.(*ast.AttrGetExpr)
		if !ok || attr == nil || ast.KeyName(attr.Key) != "isfrozen" || attr.KeySyntax != ast.AttrKeyDot {
			return path.Path{}, false
		}
		recv, ok := attr.Object.(*ast.IdentExpr)
		if !ok || !bindings.ResolvesToGlobal(recv, "table") {
			return path.Path{}, false
		}
	default:
		return path.Path{}, false
	}
	argPath, ok := pathexpr.Resolve(call.Args[0], bindings)
	if !ok || argPath.IsEmpty() {
		return path.Path{}, false
	}
	return argPath, true
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

// ImpliedChecksOnEdge returns normalized leaf checks whose value is known on the
// requested outer branch edge. It preserves both the outer edge and the leaf
// polarity so downstream evidence can select the right leaf consequence while
// publishing it on the right CFG edge.
func ImpliedChecksOnEdge(expr ast.Expr, bindings *bind.Result, edge bool) []ImpliedCheck {
	return impliedChecks(expr, bindings, edge, edge)
}

// ImpliedChecksOnBothEdges returns all leaf checks implied by either branch
// edge, in true-edge then false-edge order. It is for fact lanes that publish a
// single collection carrying edge information on each element.
func ImpliedChecksOnBothEdges(expr ast.Expr, bindings *bind.Result) []ImpliedCheck {
	trueEdge := ImpliedChecksOnEdge(expr, bindings, true)
	falseEdge := ImpliedChecksOnEdge(expr, bindings, false)
	if len(trueEdge) == 0 {
		return falseEdge
	}
	if len(falseEdge) == 0 {
		return trueEdge
	}
	out := make([]ImpliedCheck, 0, len(trueEdge)+len(falseEdge))
	out = append(out, trueEdge...)
	out = append(out, falseEdge...)
	return out
}

// ImpliedRelationalOpsOnEdge returns relational leaf expressions whose value is
// known on the requested outer branch edge. Unlike ImpliedChecksOnEdge it does
// not normalize the comparison; the caller owns interpreting the raw relop.
func ImpliedRelationalOpsOnEdge(expr ast.Expr, edge bool) []ImpliedRelationalOp {
	return impliedRelationalOps(expr, edge, edge)
}

// SufficientChecksOnEdge returns leaf checks that, by themselves, force the
// requested outer branch edge. For example, any true disjunct is sufficient for
// an OR true edge, while a true OR edge does not imply that any particular
// disjunct held.
func SufficientChecksOnEdge(expr ast.Expr, bindings *bind.Result, edge bool) []ImpliedCheck {
	return sufficientChecks(expr, bindings, edge, edge)
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

func sufficientChecks(expr ast.Expr, bindings *bind.Result, polarity bool, edge bool) []ImpliedCheck {
	check := Normalize(expr, bindings)
	if check.Kind != CheckNone {
		return []ImpliedCheck{{Check: check, Edge: edge, Polarity: polarity}}
	}
	if unary, ok := expr.(*ast.UnaryNotOpExpr); ok {
		return sufficientChecks(unary.Expr, bindings, !polarity, edge)
	}
	logical, ok := expr.(*ast.LogicalOpExpr)
	if !ok {
		return nil
	}
	splitOp := "or"
	if !polarity {
		splitOp = "and"
	}
	if logical.Operator != splitOp {
		return nil
	}
	left := sufficientChecks(logical.Lhs, bindings, polarity, edge)
	right := sufficientChecks(logical.Rhs, bindings, polarity, edge)
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	out := make([]ImpliedCheck, 0, len(left)+len(right))
	out = append(out, left...)
	out = append(out, right...)
	return out
}

func impliedChecks(expr ast.Expr, bindings *bind.Result, polarity bool, edge bool) []ImpliedCheck {
	check := Normalize(expr, bindings)
	if check.Kind != CheckNone {
		return []ImpliedCheck{{Check: check, Edge: edge, Polarity: polarity}}
	}
	if unary, ok := expr.(*ast.UnaryNotOpExpr); ok {
		return impliedChecks(unary.Expr, bindings, !polarity, edge)
	}
	splitOp := "and"
	if !polarity {
		splitOp = "or"
	}
	logical, ok := expr.(*ast.LogicalOpExpr)
	if !ok || logical.Operator != splitOp {
		return nil
	}
	left := impliedChecks(logical.Lhs, bindings, polarity, edge)
	right := impliedChecks(logical.Rhs, bindings, polarity, edge)
	if len(left) == 0 {
		return right
	}
	if len(right) == 0 {
		return left
	}
	out := make([]ImpliedCheck, 0, len(left)+len(right))
	out = append(out, left...)
	out = append(out, right...)
	return out
}

func impliedRelationalOps(expr ast.Expr, polarity bool, edge bool) []ImpliedRelationalOp {
	switch e := sourceprovenance.AssertionInner(expr).(type) {
	case *ast.RelationalOpExpr:
		return []ImpliedRelationalOp{{Expr: e, Edge: edge, Polarity: polarity}}
	case *ast.UnaryNotOpExpr:
		return impliedRelationalOps(e.Expr, !polarity, edge)
	case *ast.LogicalOpExpr:
		splitOp := "and"
		if !polarity {
			splitOp = "or"
		}
		if e.Operator != splitOp {
			return nil
		}
		left := impliedRelationalOps(e.Lhs, polarity, edge)
		right := impliedRelationalOps(e.Rhs, polarity, edge)
		if len(left) == 0 {
			return right
		}
		if len(right) == 0 {
			return left
		}
		out := make([]ImpliedRelationalOp, 0, len(left)+len(right))
		out = append(out, left...)
		out = append(out, right...)
		return out
	}
	return nil
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
		if call.Receiver != nil && call.Method == method {
			return call, true
		}
		if call.Receiver == nil && call.Method == "" {
			attr, ok := call.Func.(*ast.AttrGetExpr)
			if ok && attr.KeySyntax == ast.AttrKeyDot && ast.KeyName(attr.Key) == method {
				return call, true
			}
		}
		return nil, false
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
		func(arrayPath path.Path, floor int64, negated bool) Check {
			return Check{Kind: CheckLenGe, Path: arrayPath, LenFloor: floor, Negated: negated}
		})
}

// normalizeFlippedComparison tries operands in both operand orders (flipping the
// relational operator for the second), building the check from the first match.
func normalizeFlippedComparison[A, B any](
	expr *ast.RelationalOpExpr,
	bindings *bind.Result,
	operands func(ast.Expr, string, ast.Expr, *bind.Result) (A, B, bool, bool),
	build func(A, B, bool) Check,
) (Check, bool) {
	if a, b, negated, ok := operands(expr.Lhs, expr.Operator, expr.Rhs, bindings); ok {
		return build(a, b, negated), true
	}
	if a, b, negated, ok := operands(expr.Rhs, flipRelop(expr.Operator), expr.Lhs, bindings); ok {
		return build(a, b, negated), true
	}
	return Check{}, false
}

// normalizeIndexInRangeComparison recognizes an index upper-bound guard such as
// `i <= #xs` or `#xs >= i`. On the true edge it proves reads `xs[i]` are
// in-range when paired with a positive index fact.
func normalizeIndexInRangeComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	return normalizeFlippedComparison(expr, bindings, indexLenBoundOperands,
		func(indexPath, arrayPath path.Path, negated bool) Check {
			return Check{Kind: CheckIndexInRange, Path: indexPath, OtherPath: arrayPath, Negated: negated}
		})
}

// normalizeNumericFloorComparison recognizes a numeric lower-bound guard such as
// `i >= 1`, `i > 0`, or the flipped `1 <= i`, producing CheckNumGe{Path: i,
// NumFloor: k}. It supplies the positive-index proof that pairs with an
// index-in-range guard to remove the optional nil from an array read.
func normalizeNumericFloorComparison(expr *ast.RelationalOpExpr, bindings *bind.Result) (Check, bool) {
	return normalizeFlippedComparison(expr, bindings, numericFloorOperands,
		func(numPath path.Path, floor int64, negated bool) Check {
			return Check{Kind: CheckNumGe, Path: numPath, NumFloor: floor, Negated: negated}
		})
}

// numericFloorOperands matches `path <op> const` and returns the numeric path and
// the proven lower bound on its value when op establishes a non-negative floor. A
// floor of 0 (`j >= 0`) feeds relational sum proofs such as i + j <= #xs.
func numericFloorOperands(numExpr ast.Expr, op string, constExpr ast.Expr, bindings *bind.Result) (path.Path, int64, bool, bool) {
	numPath, ok := pathexpr.Resolve(numExpr, bindings)
	if !ok || numPath.IsEmpty() {
		return path.Path{}, 0, false, false
	}
	c, ok := constExpr.(*ast.NumberExpr)
	if !ok {
		return path.Path{}, 0, false, false
	}
	value, ok := numparse.ParseIntegerLiteral(c.Value)
	if !ok {
		return path.Path{}, 0, false, false
	}
	floor, negated, ok := numericFloorForRelop(op, value)
	if !ok || floor < 0 {
		return path.Path{}, 0, false, false
	}
	return numPath, floor, negated, true
}

// numericFloorForRelop computes the proven value lower bound from `path <op> c`
// and the edge it holds on. `>`/`>=` prove the floor on the true edge; `<`/`<=`
// prove it on the false edge (not(i < c) is i >= c; not(i <= c) is i >= c+1) for
// the `if i < lo then error end` guard form.
func numericFloorForRelop(op string, c int64) (int64, bool, bool) {
	switch op {
	case ">":
		return c + 1, false, true
	case ">=":
		return c, false, true
	case "<":
		return c, true, true
	case "<=":
		return c + 1, true, true
	default:
		return 0, false, false
	}
}

func indexLenBoundOperands(indexExpr ast.Expr, op string, lenExpr ast.Expr, bindings *bind.Result) (path.Path, path.Path, bool, bool) {
	// `i <= #xs` / `i < #xs` prove the in-range bound i <= len on the TRUE edge.
	// `i > #xs` / `i >= #xs` prove it on the FALSE edge (the `if oob then error`
	// guard form): not(i > #xs) is i <= #xs, and not(i >= #xs) is i < #xs <= len.
	var negated bool
	switch op {
	case "<=", "<":
		negated = false
	case ">", ">=":
		negated = true
	default:
		return path.Path{}, path.Path{}, false, false
	}
	indexPath, ok := pathexpr.Resolve(indexExpr, bindings)
	if !ok || indexPath.IsEmpty() {
		return path.Path{}, path.Path{}, false, false
	}
	arrayPath, ok := pathexpr.ResolveLengthOperand(lenExpr, bindings)
	if !ok || arrayPath.IsEmpty() {
		return path.Path{}, path.Path{}, false, false
	}
	return indexPath, arrayPath, negated, true
}

// lengthFloorOperands matches `#array <op> const` and returns the array path and
// the proven floor on its length when op establishes a positive lower bound.
func lengthFloorOperands(lenExpr ast.Expr, op string, constExpr ast.Expr, bindings *bind.Result) (path.Path, int64, bool, bool) {
	arrayPath, ok := pathexpr.ResolveLengthOperand(lenExpr, bindings)
	if !ok || arrayPath.IsEmpty() {
		return path.Path{}, 0, false, false
	}
	c, ok := constExpr.(*ast.NumberExpr)
	if !ok {
		return path.Path{}, 0, false, false
	}
	value, ok := numparse.ParseIntegerLiteral(c.Value)
	if !ok {
		return path.Path{}, 0, false, false
	}
	floor, negated, ok := lengthFloorForRelop(op, value)
	if !ok || floor <= 0 {
		return path.Path{}, 0, false, false
	}
	return arrayPath, floor, negated, true
}

// lengthFloorForRelop computes the proven length lower bound from `len <op> c`
// and the edge it holds on. `>`/`>=`/`==positive`/`~=0` prove the floor on the
// true edge; `<`/`<=`, `==0`, and `~=positive` prove it on the false edge for
// guard-return forms such as `if #xs == 0 then return end`.
func lengthFloorForRelop(op string, c int64) (int64, bool, bool) {
	switch op {
	case ">":
		return c + 1, false, true
	case ">=":
		return c, false, true
	case "==":
		if c == 0 {
			return 1, true, true
		}
		return c, false, true
	case "~=":
		// len ~= 0 proves len >= 1 since length is non-negative.
		if c == 0 {
			return 1, false, true
		}
		return c, true, true
	case "<":
		return c, true, true
	case "<=":
		return c + 1, true, true
	default:
		return 0, false, false
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
