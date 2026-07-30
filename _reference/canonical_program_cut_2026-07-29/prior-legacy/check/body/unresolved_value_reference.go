package body

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// UnresolvedValueReferenceOccurrence is a reachable identifier read that
// remains an implicit global after binding and is not known type syntax.
type UnresolvedValueReferenceOccurrence struct {
	Point cfg.Point
	Name  string
	Key   string
	Span  SourceSpan
}

// ForEachUnresolvedValueReferenceOccurrence visits unresolved value references
// in deterministic CFG order.
func (r *Result) ForEachUnresolvedValueReferenceOccurrence(visit func(UnresolvedValueReferenceOccurrence) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	seen := make(map[*ast.IdentExpr]struct{})
	emit := func(point cfg.Point, expr ast.Expr) bool {
		return r.walkUnresolvedValueExpr(point, expr, seen, func(ref UnresolvedValueReferenceOccurrence) bool {
			visited = true
			return visit(ref)
		})
	}
	r.ForEachReachableExpressionUse(func(use ExpressionUse) bool {
		if use.Role == ExpressionUseOrdinaryAssignmentTarget {
			return r.walkUnresolvedValueAssignmentTarget(use.Point, use.Expr, seen, func(ref UnresolvedValueReferenceOccurrence) bool {
				visited = true
				return visit(ref)
			})
		}
		return emit(use.Point, use.Expr)
	})
	return visited
}

func (r *Result) walkUnresolvedValueAssignmentTarget(point cfg.Point, target ast.Expr, seen map[*ast.IdentExpr]struct{}, visit func(UnresolvedValueReferenceOccurrence) bool) bool {
	switch t := target.(type) {
	case nil:
		return true
	case *ast.AttrGetExpr:
		if !r.walkUnresolvedValueExpr(point, t.Object, seen, visit) {
			return false
		}
		if t.KeySyntax == ast.AttrKeyIndex {
			return r.walkUnresolvedValueExpr(point, t.Key, seen, visit)
		}
		return true
	case *ast.CastExpr:
		return r.walkUnresolvedValueAssignmentTarget(point, t.Expr, seen, visit)
	case *ast.NonNilAssertExpr:
		return r.walkUnresolvedValueAssignmentTarget(point, t.Expr, seen, visit)
	default:
		return r.walkUnresolvedValueExpr(point, target, seen, visit)
	}
}

func (r *Result) walkUnresolvedValueExpr(point cfg.Point, expr ast.Expr, seen map[*ast.IdentExpr]struct{}, visit func(UnresolvedValueReferenceOccurrence) bool) bool {
	if expr == nil {
		return true
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return r.visitUnresolvedValueIdent(point, e, seen, visit)
	case *ast.AttrGetExpr:
		if !r.walkUnresolvedValueExpr(point, e.Object, seen, visit) {
			return false
		}
		if e.KeySyntax == ast.AttrKeyIndex {
			return r.walkUnresolvedValueExpr(point, e.Key, seen, visit)
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if field.KeySyntax == ast.AttrKeyIndex {
				if !r.walkUnresolvedValueExpr(point, field.Key, seen, visit) {
					return false
				}
			}
			if !r.walkUnresolvedValueExpr(point, field.Value, seen, visit) {
				return false
			}
		}
	case *ast.FuncCallExpr:
		if !r.typeSyntaxCallee(e) {
			if !r.walkUnresolvedValueExpr(point, e.Func, seen, visit) {
				return false
			}
		}
		if !r.typeSyntaxReceiver(e) {
			if !r.walkUnresolvedValueExpr(point, e.Receiver, seen, visit) {
				return false
			}
		}
		for _, arg := range e.Args {
			if !r.walkUnresolvedValueExpr(point, arg, seen, visit) {
				return false
			}
		}
	case *ast.LogicalOpExpr:
		if !r.walkUnresolvedValueExpr(point, e.Lhs, seen, visit) {
			return false
		}
		return r.walkUnresolvedValueExpr(point, e.Rhs, seen, visit)
	case *ast.RelationalOpExpr:
		if !r.walkUnresolvedValueExpr(point, e.Lhs, seen, visit) {
			return false
		}
		return r.walkUnresolvedValueExpr(point, e.Rhs, seen, visit)
	case *ast.StringConcatOpExpr:
		if !r.walkUnresolvedValueExpr(point, e.Lhs, seen, visit) {
			return false
		}
		return r.walkUnresolvedValueExpr(point, e.Rhs, seen, visit)
	case *ast.ArithmeticOpExpr:
		if !r.walkUnresolvedValueExpr(point, e.Lhs, seen, visit) {
			return false
		}
		return r.walkUnresolvedValueExpr(point, e.Rhs, seen, visit)
	case *ast.UnaryMinusOpExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.UnaryNotOpExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.UnaryLenOpExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.UnaryBNotOpExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.CastExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	case *ast.NonNilAssertExpr:
		return r.walkUnresolvedValueExpr(point, e.Expr, seen, visit)
	}
	return true
}

func (r *Result) visitUnresolvedValueIdent(point cfg.Point, ident *ast.IdentExpr, seen map[*ast.IdentExpr]struct{}, visit func(UnresolvedValueReferenceOccurrence) bool) bool {
	if ident == nil {
		return true
	}
	if _, ok := seen[ident]; ok {
		return true
	}
	seen[ident] = struct{}{}
	if !r.IsImplicitGlobalUse(ident) {
		return true
	}
	if sym, ok := r.SymbolOfIdent(ident); ok && r.IsFunctionDefinitionTarget(sym) {
		return true
	}
	if r.identifierResolvesTypeName(ident) {
		return true
	}
	span := sourceSpanFromAST(ast.SpanOf(ident))
	name := ident.Value
	return visit(UnresolvedValueReferenceOccurrence{
		Point: point,
		Name:  name,
		Key:   "value:" + name + ":" + strconv.Itoa(span.StartLine) + ":" + strconv.Itoa(span.StartCol),
		Span:  span,
	})
}

func (r *Result) typeSyntaxCallee(call *ast.FuncCallExpr) bool {
	if call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return false
	}
	ident, ok := call.Func.(*ast.IdentExpr)
	return ok && r.identifierResolvesTypeName(ident)
}

func (r *Result) typeSyntaxReceiver(call *ast.FuncCallExpr) bool {
	if call == nil || call.Method == "" || len(call.TypeArgs) != 0 {
		return false
	}
	ident, ok := call.Receiver.(*ast.IdentExpr)
	return ok && r.identifierResolvesTypeName(ident)
}

func (r *Result) identifierResolvesTypeName(ident *ast.IdentExpr) bool {
	if ident == nil || ident.Value == "" {
		return false
	}
	if typ.BuiltinPrimitiveName(ident.Value) {
		return true
	}
	switch ident.Value {
	case "int", "bool":
		return true
	}
	if _, ok := r.TypeValueRef(ident); ok {
		return true
	}
	if decl, ok := r.TypeRef(&ast.TypeRefExpr{Path: []string{ident.Value}}); ok && decl.ID != 0 {
		return true
	}
	return false
}
