package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// RedundantClaimOccurrence is a runtime claim/cast whose operand type is
// independently known to satisfy the target type before the claim runs.
type RedundantClaimOccurrence struct {
	Point        cfg.Point
	OperandLabel string
	ClaimLabel   string
	OperandType  typ.Type
	ClaimedType  typ.Type
	OperandSpan  SourceSpan
	ClaimSpan    SourceSpan
}

// ForEachRedundantClaimOccurrence visits runtime validation sites whose
// operand already satisfies the target type before the validation boundary.
func (r *Result) ForEachRedundantClaimOccurrence(visit func(RedundantClaimOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	seenCasts := make(map[adviceCastSeenKey]struct{})
	seenCalls := make(map[adviceCallSeenKey]struct{})
	r.ForEachReachableExpressionUse(func(use ExpressionUse) bool {
		return r.walkAdviceClaims(use.Point, use.Expr, seenCasts, seenCalls, visit, &visited)
	})
	return visited
}

type adviceCastSeenKey struct {
	point cfg.Point
	expr  *ast.CastExpr
}

type adviceCallSeenKey struct {
	point cfg.Point
	expr  *ast.FuncCallExpr
}

func (r *Result) walkAdviceClaims(
	point cfg.Point,
	expr ast.Expr,
	seenCasts map[adviceCastSeenKey]struct{},
	seenCalls map[adviceCallSeenKey]struct{},
	visit func(RedundantClaimOccurrence) bool,
	visited *bool,
) bool {
	type frame struct {
		expr ast.Expr
		exit bool
	}
	stack := []frame{{expr: expr}}
	for len(stack) != 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.expr == nil {
			continue
		}
		if !current.exit {
			stack = append(stack, frame{expr: current.expr, exit: true})
			children := adviceClaimChildren(current.expr)
			for i := len(children) - 1; i >= 0; i-- {
				stack = append(stack, frame{expr: children[i]})
			}
			continue
		}
		switch e := current.expr.(type) {
		case *ast.CastExpr:
			if markSeen(seenCasts, adviceCastSeenKey{point: point, expr: e}) {
				continue
			}
			if occ, ok := r.redundantCastOccurrence(point, e); ok {
				*visited = true
				if !visit(occ) {
					return false
				}
			}
		case *ast.FuncCallExpr:
			if markSeen(seenCalls, adviceCallSeenKey{point: point, expr: e}) {
				continue
			}
			if occ, ok := r.redundantTypeCastCallOccurrence(point, e); ok {
				*visited = true
				if !visit(occ) {
					return false
				}
			}
		}
	}
	return true
}

func adviceClaimChildren(expr ast.Expr) []ast.Expr {
	switch e := expr.(type) {
	case *ast.CastExpr:
		return []ast.Expr{e.Expr}
	case *ast.FuncCallExpr:
		out := make([]ast.Expr, 0, 2+len(e.Args))
		out = append(out, e.Func, e.Receiver)
		return append(out, e.Args...)
	case *ast.AttrGetExpr:
		if e.KeySyntax == ast.AttrKeyIndex {
			return []ast.Expr{e.Object, e.Key}
		}
		return []ast.Expr{e.Object}
	case *ast.TableExpr:
		out := make([]ast.Expr, 0, 2*len(e.Fields))
		for _, field := range e.Fields {
			if field != nil {
				if field.KeySyntax == ast.AttrKeyIndex {
					out = append(out, field.Key)
				}
				out = append(out, field.Value)
			}
		}
		return out
	case *ast.LogicalOpExpr:
		return []ast.Expr{e.Lhs, e.Rhs}
	case *ast.RelationalOpExpr:
		return []ast.Expr{e.Lhs, e.Rhs}
	case *ast.StringConcatOpExpr:
		return []ast.Expr{e.Lhs, e.Rhs}
	case *ast.ArithmeticOpExpr:
		return []ast.Expr{e.Lhs, e.Rhs}
	case *ast.UnaryMinusOpExpr:
		return []ast.Expr{e.Expr}
	case *ast.UnaryNotOpExpr:
		return []ast.Expr{e.Expr}
	case *ast.UnaryLenOpExpr:
		return []ast.Expr{e.Expr}
	case *ast.UnaryBNotOpExpr:
		return []ast.Expr{e.Expr}
	case *ast.NonNilAssertExpr:
		return []ast.Expr{e.Expr}
	default:
		return nil
	}
}

func (r *Result) redundantCastOccurrence(point cfg.Point, expr *ast.CastExpr) (RedundantClaimOccurrence, bool) {
	if expr == nil || expr.Expr == nil || expr.Type == nil || r.TypeResolver() == nil {
		return RedundantClaimOccurrence{}, false
	}
	target, ok := r.TypeResolver().Type(expr.Type)
	if !ok {
		return RedundantClaimOccurrence{}, false
	}
	return r.redundantClaimOccurrence(point, expr.Expr, target, sourceSpanFromAST(ast.SpanOf(expr)), "type claim")
}

func (r *Result) redundantTypeCastCallOccurrence(point cfg.Point, call *ast.FuncCallExpr) (RedundantClaimOccurrence, bool) {
	if call == nil || call.Method != "" || len(call.Args) != 1 {
		return RedundantClaimOccurrence{}, false
	}
	if _, ok := r.CallSiteView(point); !ok {
		return RedundantClaimOccurrence{}, false
	}
	target, ok := r.typeCastCallTarget(call)
	if !ok {
		return RedundantClaimOccurrence{}, false
	}
	return r.redundantClaimOccurrence(point, call.Args[0], target, sourceSpanFromAST(ast.SpanOf(call)), "type cast call")
}

func (r *Result) redundantClaimOccurrence(point cfg.Point, operand ast.Expr, target typ.Type, claimSpan SourceSpan, claimLabel string) (RedundantClaimOccurrence, bool) {
	if operand == nil || target == nil || typ.IsTopLike(target) || typ.IsNever(target) {
		return RedundantClaimOccurrence{}, false
	}
	operandType, ok := r.ExpressionTypeBeforeBoundary(point, operand)
	if !ok || operandType == nil || typ.IsTopLike(operandType) || typ.IsNever(operandType) {
		return RedundantClaimOccurrence{}, false
	}
	if !r.IsSubtype(operandType, target) {
		return RedundantClaimOccurrence{}, false
	}
	operandLabel := ExpressionLabel(operand)
	if operandLabel == "" {
		operandLabel = "value"
	}
	return RedundantClaimOccurrence{
		Point:        point,
		OperandLabel: operandLabel,
		ClaimLabel:   claimLabel,
		OperandType:  operandType,
		ClaimedType:  target,
		OperandSpan:  sourceSpanFromAST(ast.SpanOf(operand)),
		ClaimSpan:    claimSpan,
	}, true
}

func (r *Result) typeCastCallTarget(call *ast.FuncCallExpr) (typ.Type, bool) {
	if call == nil || call.Func == nil {
		return nil, false
	}
	if ident, ok := call.Func.(*ast.IdentExpr); ok {
		if t, ok := primitiveRuntimeCastType(ident.Value); ok && r.IdentResolvesToGlobal(ident, ident.Value) {
			return t, true
		}
		if decl, ok := r.TypeValueRef(ident); ok && r.TypeResolver() != nil {
			return r.TypeResolver().Decl(decl)
		}
	}
	if r.TypeResolver() == nil {
		return nil, false
	}
	parts, ok := valueexpr.TypeValueRefParts(call.Func)
	if !ok {
		return nil, false
	}
	return r.TypeResolver().ResolveTypeRef(parts)
}

func primitiveRuntimeCastType(name string) (typ.Type, bool) {
	switch name {
	case "boolean":
		return typ.Boolean, true
	case "number":
		return typ.Number, true
	case "integer":
		return typ.Integer, true
	case "string":
		return typ.String, true
	default:
		return nil, false
	}
}

func markSeen[K comparable](seen map[K]struct{}, key K) (seenBefore bool) {
	_, seenBefore = seen[key]
	seen[key] = struct{}{}
	return seenBefore
}
