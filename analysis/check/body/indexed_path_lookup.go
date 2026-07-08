package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// IndexedPathLookup is an indexed read whose container and key both have
// static path evidence at the same point.
type IndexedPathLookup struct {
	Point     cfg.Point
	Span      SourceSpan
	Container pathdom.Path
	Key       pathdom.Path
}

// IndexedPathLookupsAt returns indexed path lookups in the expression positions
// whose value is produced at point. This deliberately mirrors the table-dispatch
// diagnostic's stable scan policy: assignment sources, plus call callee/receiver
// positions, not call arguments.
func (r *Result) IndexedPathLookupsAt(point cfg.Point) []IndexedPathLookup {
	if r == nil {
		return nil
	}
	var out []IndexedPathLookup
	for _, use := range r.expressionUsesAt(point) {
		switch use.Role {
		case ExpressionUseLocalAssignmentSource, ExpressionUseOrdinaryAssignmentSource:
			out = append(out, r.indexedPathLookupsInExpr(point, use.Expr, false)...)
		}
	}
	if call, ok := r.Call(point); ok && call.Call != nil {
		out = append(out, r.indexedPathLookupsInExpr(point, call.Func, true)...)
		if call.Receiver != nil {
			out = append(out, r.indexedPathLookupsInExpr(point, call.Receiver, true)...)
		}
	}
	return out
}

func (r *Result) indexedPathLookupsInExpr(point cfg.Point, expr ast.Expr, scanCallFunc bool) []IndexedPathLookup {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.AttrGetExpr:
		var out []IndexedPathLookup
		if lookup, ok := r.indexedPathLookupFromAttr(point, e); ok {
			out = append(out, lookup)
		}
		out = append(out, r.indexedPathLookupsInExpr(point, e.Object, scanCallFunc)...)
		out = append(out, r.indexedPathLookupsInExpr(point, e.Key, scanCallFunc)...)
		return out
	case *ast.FuncCallExpr:
		if !scanCallFunc {
			return nil
		}
		var out []IndexedPathLookup
		out = append(out, r.indexedPathLookupsInExpr(point, e.Func, scanCallFunc)...)
		out = append(out, r.indexedPathLookupsInExpr(point, e.Receiver, scanCallFunc)...)
		return out
	case *ast.CastExpr:
		return r.indexedPathLookupsInExpr(point, e.Expr, scanCallFunc)
	case *ast.NonNilAssertExpr:
		return r.indexedPathLookupsInExpr(point, e.Expr, scanCallFunc)
	case *ast.LogicalOpExpr:
		return nil
	default:
		return nil
	}
}

func (r *Result) indexedPathLookupFromAttr(point cfg.Point, attr *ast.AttrGetExpr) (IndexedPathLookup, bool) {
	if attr == nil || attr.KeySyntax != ast.AttrKeyIndex {
		return IndexedPathLookup{}, false
	}
	container, ok := r.ExpressionPath(attr.Object)
	if !ok || container.Symbol == 0 {
		return IndexedPathLookup{}, false
	}
	key, ok := r.ExpressionPath(attr.Key)
	if !ok || key.Symbol == 0 || len(key.Segments) == 0 {
		return IndexedPathLookup{}, false
	}
	return IndexedPathLookup{
		Point:     point,
		Span:      sourceSpanFromAST(ast.SpanOf(attr)),
		Container: container,
		Key:       key,
	}, true
}

// StaticStringAssignmentKeyForContainer returns the static string key for an
// indexed assignment into table when the assignment target is equivalent to
// table[key] at point.
func (r *Result) StaticStringAssignmentKeyForContainer(point cfg.Point, fact OrdinaryAssignmentFact, table pathdom.Path) (string, bool) {
	if r == nil || fact.Target == nil || table.IsEmpty() {
		return "", false
	}
	attr, ok := fact.Target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return "", false
	}
	container, ok := r.ExpressionPath(attr.Object)
	if !ok || container.IsEmpty() {
		return "", false
	}
	if !container.Equal(table) && !r.PathsEquivalentAtBoundary(point, container, table) {
		return "", false
	}
	return r.StaticStringExprValueAtBoundary(point, attr.Key)
}
