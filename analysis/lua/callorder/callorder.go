// Package callorder enumerates Lua call expressions in evaluation order.
package callorder

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/channelruntime"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NoExprIndex marks call occurrences that are not rooted in a value-list slot.
const NoExprIndex = -1

// Occurrence describes one call expression discovered under an expression.
type Occurrence struct {
	ExprIndex int
	Call      *ast.FuncCallExpr
}

// Options configures traversal for syntax that is owned by higher-level
// semantic recognizers.
type Options struct {
	// OpaqueCall reports calls whose children should not be traversed. The call
	// itself is still returned as an occurrence.
	OpaqueCall func(*ast.FuncCallExpr) bool

	// ExpressionCoveredCall reports pure predicate calls that are fully modeled
	// by the enclosing expression and therefore do not need a runtime call node.
	ExpressionCoveredCall func(*ast.FuncCallExpr, ast.Expr) bool
}

// LuaOptions returns the default call-order policy for Lua analysis.
func LuaOptions(bindings *bind.Result) Options {
	return Options{
		OpaqueCall: func(call *ast.FuncCallExpr) bool {
			return channelSelectCallCovered(call, bindings)
		},
		ExpressionCoveredCall: func(call *ast.FuncCallExpr, owner ast.Expr) bool {
			return typePredicateCallCovered(call, owner, bindings)
		},
	}
}

// ValueList returns all supported calls below exprs in Lua evaluation order.
func ValueList(exprs []ast.Expr, options Options) ([]Occurrence, bool) {
	var calls []Occurrence
	for i, expr := range exprs {
		if !collectExpr(expr, i, options, &calls) {
			return nil, false
		}
	}
	return calls, true
}

// Expr returns all supported calls below expr in Lua evaluation order.
func Expr(expr ast.Expr, options Options) ([]Occurrence, bool) {
	var calls []Occurrence
	if !collectExpr(expr, NoExprIndex, options, &calls) {
		return nil, false
	}
	return calls, true
}

// Call returns all supported calls involved in evaluating call, including call.
func Call(call *ast.FuncCallExpr, exprIndex int, options Options) ([]Occurrence, bool) {
	var calls []Occurrence
	if !collectCall(call, exprIndex, options, &calls) {
		return nil, false
	}
	return calls, true
}

func collectExpr(expr ast.Expr, exprIndex int, options Options, calls *[]Occurrence) bool {
	inner := sourceprovenance.AssertionInner(expr)
	if inner != expr {
		return collectExpr(inner, exprIndex, options, calls)
	}

	switch expr := inner.(type) {
	case nil:
		return true
	case *ast.TrueExpr, *ast.FalseExpr, *ast.NilExpr, *ast.NumberExpr, *ast.StringExpr, *ast.Comma3Expr:
		return true
	case *ast.IdentExpr:
		return true
	case *ast.AttrGetExpr:
		return collectExpr(expr.Object, exprIndex, options, calls) &&
			collectExpr(expr.Key, exprIndex, options, calls)
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field == nil {
				continue
			}
			if !collectExpr(field.Key, exprIndex, options, calls) ||
				!collectExpr(field.Value, exprIndex, options, calls) {
				return false
			}
		}
		return true
	case *ast.FuncCallExpr:
		if expressionCoveredCall(expr, expr, options) {
			return true
		}
		return collectCall(expr, exprIndex, options, calls)
	case *ast.FunctionExpr:
		return true
	case *ast.LogicalOpExpr:
		if containsCall(expr.Lhs, options) || containsCall(expr.Rhs, options) {
			return false
		}
		return collectExpr(expr.Lhs, exprIndex, options, calls) &&
			collectExpr(expr.Rhs, exprIndex, options, calls)
	case *ast.RelationalOpExpr:
		if expressionCoveredRelationalCall(expr, options) {
			return true
		}
		return collectExpr(expr.Lhs, exprIndex, options, calls) &&
			collectExpr(expr.Rhs, exprIndex, options, calls)
	case *ast.StringConcatOpExpr:
		return collectExpr(expr.Lhs, exprIndex, options, calls) &&
			collectExpr(expr.Rhs, exprIndex, options, calls)
	case *ast.ArithmeticOpExpr:
		return collectExpr(expr.Lhs, exprIndex, options, calls) &&
			collectExpr(expr.Rhs, exprIndex, options, calls)
	case *ast.UnaryMinusOpExpr:
		return collectExpr(expr.Expr, exprIndex, options, calls)
	case *ast.UnaryNotOpExpr:
		return collectExpr(expr.Expr, exprIndex, options, calls)
	case *ast.UnaryLenOpExpr:
		return collectExpr(expr.Expr, exprIndex, options, calls)
	case *ast.UnaryBNotOpExpr:
		return collectExpr(expr.Expr, exprIndex, options, calls)
	default:
		return false
	}
}

func collectCall(call *ast.FuncCallExpr, exprIndex int, options Options, calls *[]Occurrence) bool {
	if call == nil {
		return true
	}
	if expressionCoveredCall(call, call, options) {
		return true
	}
	if options.OpaqueCall != nil && options.OpaqueCall(call) {
		*calls = append(*calls, Occurrence{ExprIndex: exprIndex, Call: call})
		return true
	}
	if call.Receiver != nil {
		if !collectExpr(call.Receiver, exprIndex, options, calls) {
			return false
		}
	} else if !collectExpr(call.Func, exprIndex, options, calls) {
		return false
	}
	for _, arg := range call.Args {
		if !collectExpr(arg, exprIndex, options, calls) {
			return false
		}
	}
	*calls = append(*calls, Occurrence{ExprIndex: exprIndex, Call: call})
	return true
}

func containsCall(expr ast.Expr, options Options) bool {
	inner := sourceprovenance.AssertionInner(expr)
	if inner != expr {
		return containsCall(inner, options)
	}
	switch expr := inner.(type) {
	case nil:
		return false
	case *ast.FuncCallExpr:
		return !expressionCoveredCall(expr, expr, options)
	case *ast.AttrGetExpr:
		return containsCall(expr.Object, options) || containsCall(expr.Key, options)
	case *ast.TableExpr:
		for _, field := range expr.Fields {
			if field != nil && (containsCall(field.Key, options) || containsCall(field.Value, options)) {
				return true
			}
		}
		return false
	case *ast.LogicalOpExpr:
		return containsCall(expr.Lhs, options) || containsCall(expr.Rhs, options)
	case *ast.RelationalOpExpr:
		if expressionCoveredRelationalCall(expr, options) {
			return false
		}
		return containsCall(expr.Lhs, options) || containsCall(expr.Rhs, options)
	case *ast.StringConcatOpExpr:
		return containsCall(expr.Lhs, options) || containsCall(expr.Rhs, options)
	case *ast.ArithmeticOpExpr:
		return containsCall(expr.Lhs, options) || containsCall(expr.Rhs, options)
	case *ast.UnaryMinusOpExpr:
		return containsCall(expr.Expr, options)
	case *ast.UnaryNotOpExpr:
		return containsCall(expr.Expr, options)
	case *ast.UnaryLenOpExpr:
		return containsCall(expr.Expr, options)
	case *ast.UnaryBNotOpExpr:
		return containsCall(expr.Expr, options)
	default:
		return false
	}
}

func expressionCoveredRelationalCall(expr *ast.RelationalOpExpr, options Options) bool {
	if expr == nil {
		return false
	}
	if call, ok := sourceprovenance.Call(expr.Lhs); ok && expressionCoveredCall(call, expr, options) {
		return true
	}
	if call, ok := sourceprovenance.Call(expr.Rhs); ok && expressionCoveredCall(call, expr, options) {
		return true
	}
	return false
}

func expressionCoveredCall(call *ast.FuncCallExpr, owner ast.Expr, options Options) bool {
	return call != nil && options.ExpressionCoveredCall != nil && options.ExpressionCoveredCall(call, owner)
}

func typePredicateCallCovered(call *ast.FuncCallExpr, owner ast.Expr, bindings *bind.Result) bool {
	rel, ok := owner.(*ast.RelationalOpExpr)
	if !ok || !relationalOperandCall(rel, call) {
		return false
	}
	return branchcond.SupportsTypeComparison(rel, bindings)
}

func relationalOperandCall(rel *ast.RelationalOpExpr, call *ast.FuncCallExpr) bool {
	if rel == nil || call == nil {
		return false
	}
	if operand, ok := sourceprovenance.Call(rel.Lhs); ok && operand == call {
		return true
	}
	if operand, ok := sourceprovenance.Call(rel.Rhs); ok && operand == call {
		return true
	}
	return false
}

func channelSelectCallCovered(call *ast.FuncCallExpr, bindings *bind.Result) bool {
	if !channelruntime.IsSelectCall(call, bindings) {
		return false
	}
	table, ok := call.Args[0].(*ast.TableExpr)
	if !ok {
		return false
	}
	return channelSelectTableCovered(table, bindings)
}

func channelSelectTableCovered(table *ast.TableExpr, bindings *bind.Result) bool {
	if table == nil {
		return false
	}
	for _, field := range table.Fields {
		if field == nil || channelSelectDefaultField(field) {
			continue
		}
		if !channelSelectCaseCallCovered(field.Value, bindings) {
			return false
		}
	}
	return true
}

func channelSelectDefaultField(field *ast.Field) bool {
	if field == nil {
		return false
	}
	return ast.KeyName(field.Key) == "default"
}

func channelSelectCaseCallCovered(expr ast.Expr, bindings *bind.Result) bool {
	call, ok := sourceprovenance.Call(expr)
	if !ok || !channelruntime.IsReceiveCaseCall(call, bindings) {
		return false
	}
	_, ok = Expr(call.Receiver, Options{})
	return ok
}
