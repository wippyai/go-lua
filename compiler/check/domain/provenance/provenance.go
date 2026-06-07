package provenance

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// IdentBindingLookup resolves identifier expressions to graph symbols.
type IdentBindingLookup interface {
	SymbolOf(ident *ast.IdentExpr) (cfg.SymbolID, bool)
}

// FreshTableLiteral is a table constructor that still owns the value currently
// read through a local symbol. Point is where the literal assignment occurred.
type FreshTableLiteral struct {
	Table *ast.TableExpr
	Point cfg.Point
}

// SegmentTypeProjector projects a structural type through a field/index suffix.
type SegmentTypeProjector func(base typ.Type, segments []constraint.Segment) typ.Type

// RouteLocalType projects a source type forward through one provenance route to
// the local routed value type. Route discovery is flow-owned; this package owns
// route interpretation that is independent of checker observation state.
func RouteLocalType(route flow.ProvenanceRoute, source typ.Type, project SegmentTypeProjector) typ.Type {
	if source == nil {
		return nil
	}
	var local typ.Type
	switch route.Kind {
	case flow.ProvenanceRouteIdentityAlias:
		local = source
	case flow.ProvenanceRouteIndexedIterator:
		if route.VarIndex != 1 {
			return nil
		}
		local = querycore.ElementType(source)
	case flow.ProvenanceRouteKeyedIterator:
		switch route.VarIndex {
		case 0:
			local = querycore.EntryKeyType(source)
		case 1:
			local = querycore.EntryValueType(source)
		default:
			return nil
		}
	default:
		return nil
	}
	if local == nil || len(route.Remainder) == 0 {
		return local
	}
	if project == nil {
		return nil
	}
	return project(local, route.Remainder)
}

// CurrentFreshTableLiteral returns the transfer-proven fresh table literal for
// source at a point. The freshness proof is produced by trace.GraphEvidence;
// this reducer only matches the identifier use to canonical evidence.
func CurrentFreshTableLiteral(
	source ast.Expr,
	at cfg.Point,
	bindings IdentBindingLookup,
	freshTables []api.FreshTableLiteralEvidence,
) (FreshTableLiteral, bool) {
	if source == nil || bindings == nil || len(freshTables) == 0 {
		return FreshTableLiteral{}, false
	}
	ident, ok := source.(*ast.IdentExpr)
	if !ok {
		return FreshTableLiteral{}, false
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return FreshTableLiteral{}, false
	}
	for _, ev := range freshTables {
		if ev.Point != at || ev.Symbol != sym || ev.Table == nil {
			continue
		}
		return FreshTableLiteral{Table: ev.Table, Point: ev.AssignmentPoint}, true
	}
	return FreshTableLiteral{}, false
}

// ExprMayExposeSymbolValue reports whether evaluating expr may publish the
// symbol's current value as an alias or to a call. Field reads do not expose the
// base object; calls and table constructors do.
func ExprMayExposeSymbolValue(expr ast.Expr, sym cfg.SymbolID, bindings IdentBindingLookup) bool {
	if expr == nil || sym == 0 || bindings == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		bound, ok := bindings.SymbolOf(e)
		return ok && bound == sym
	case *ast.CastExpr:
		return ExprMayExposeSymbolValue(e.Expr, sym, bindings)
	case *ast.NonNilAssertExpr:
		return ExprMayExposeSymbolValue(e.Expr, sym, bindings)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if ExprMayExposeSymbolValue(field.Key, sym, bindings) || ExprMayExposeSymbolValue(field.Value, sym, bindings) {
				return true
			}
		}
		return false
	case *ast.FuncCallExpr:
		return ExprReferencesSymbol(e.Func, sym, bindings) ||
			ExprReferencesSymbol(e.Receiver, sym, bindings) ||
			exprsReferenceSymbol(e.Args, sym, bindings)
	case *ast.LogicalOpExpr:
		return ExprMayExposeSymbolValue(e.Lhs, sym, bindings) || ExprMayExposeSymbolValue(e.Rhs, sym, bindings)
	default:
		return false
	}
}

// ExprReferencesSymbol reports whether expr reads the given symbol anywhere.
func ExprReferencesSymbol(expr ast.Expr, sym cfg.SymbolID, bindings IdentBindingLookup) bool {
	if expr == nil || sym == 0 || bindings == nil {
		return false
	}

	switch e := expr.(type) {
	case *ast.IdentExpr:
		if bound, ok := bindings.SymbolOf(e); ok && bound == sym {
			return true
		}
		return false
	case *ast.AttrGetExpr:
		return ExprReferencesSymbol(e.Object, sym, bindings) || ExprReferencesSymbol(e.Key, sym, bindings)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			if ExprReferencesSymbol(field.Key, sym, bindings) || ExprReferencesSymbol(field.Value, sym, bindings) {
				return true
			}
		}
		return false
	case *ast.FuncCallExpr:
		return ExprReferencesSymbol(e.Func, sym, bindings) ||
			ExprReferencesSymbol(e.Receiver, sym, bindings) ||
			exprsReferenceSymbol(e.Args, sym, bindings)
	case *ast.LogicalOpExpr:
		return ExprReferencesSymbol(e.Lhs, sym, bindings) || ExprReferencesSymbol(e.Rhs, sym, bindings)
	case *ast.RelationalOpExpr:
		return ExprReferencesSymbol(e.Lhs, sym, bindings) || ExprReferencesSymbol(e.Rhs, sym, bindings)
	case *ast.StringConcatOpExpr:
		return ExprReferencesSymbol(e.Lhs, sym, bindings) || ExprReferencesSymbol(e.Rhs, sym, bindings)
	case *ast.ArithmeticOpExpr:
		return ExprReferencesSymbol(e.Lhs, sym, bindings) || ExprReferencesSymbol(e.Rhs, sym, bindings)
	case *ast.UnaryMinusOpExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.UnaryNotOpExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.UnaryLenOpExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.UnaryBNotOpExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.CastExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	case *ast.NonNilAssertExpr:
		return ExprReferencesSymbol(e.Expr, sym, bindings)
	default:
		return false
	}
}

func exprsReferenceSymbol(exprs []ast.Expr, sym cfg.SymbolID, bindings IdentBindingLookup) bool {
	for _, expr := range exprs {
		if ExprReferencesSymbol(expr, sym, bindings) {
			return true
		}
	}
	return false
}
