package provenance

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
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

// CurrentFreshTableLiteral returns the dominating fresh table literal for source
// when the source is an identifier whose current value is still the literal and
// has not escaped through another alias, call, return, or structured mutation.
func CurrentFreshTableLiteral(source ast.Expr, at cfg.Point, graph *cfg.Graph) (FreshTableLiteral, bool) {
	if source == nil || graph == nil {
		return FreshTableLiteral{}, false
	}
	ident, ok := source.(*ast.IdentExpr)
	if !ok {
		return FreshTableLiteral{}, false
	}
	bindings := graph.Bindings()
	if bindings == nil {
		return FreshTableLiteral{}, false
	}
	sym, ok := bindings.SymbolOf(ident)
	if !ok || sym == 0 {
		return FreshTableLiteral{}, false
	}
	version := graph.VisibleVersion(at, sym)
	if version.Symbol == 0 || version.ID == 0 {
		return FreshTableLiteral{}, false
	}

	current := at
	seen := make(map[cfg.Point]struct{}, 4)
	for {
		preds := graph.PredecessorsReadOnly(current)
		if len(preds) != 1 {
			return FreshTableLiteral{}, false
		}
		pred := preds[0]
		if _, ok := seen[pred]; ok {
			return FreshTableLiteral{}, false
		}
		seen[pred] = struct{}{}

		switch info := graph.Info(pred).(type) {
		case *cfg.AssignInfo:
			if fresh, found, ok := freshTableAssignment(info, pred, sym, version, graph); found {
				return fresh, ok
			}
			if assignmentInvalidatesFreshness(info, sym, bindings) {
				return FreshTableLiteral{}, false
			}
		case *cfg.CallInfo:
			return FreshTableLiteral{}, false
		case *cfg.ReturnInfo:
			if returnInvalidatesFreshness(info, sym, bindings) {
				return FreshTableLiteral{}, false
			}
		case *cfg.FuncDefInfo:
			return FreshTableLiteral{}, false
		}

		current = pred
	}
}

func freshTableAssignment(info *cfg.AssignInfo, p cfg.Point, sym cfg.SymbolID, version cfg.Version, graph *cfg.Graph) (FreshTableLiteral, bool, bool) {
	if info == nil {
		return FreshTableLiteral{}, false, false
	}
	var fresh FreshTableLiteral
	found := false
	ok := false
	info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
		if found || target.Kind != cfg.TargetIdent || target.Symbol != sym {
			return
		}
		found = true
		if graph == nil {
			return
		}
		assignedVersion := graph.VisibleVersion(p, sym)
		if assignedVersion.Symbol != version.Symbol || assignedVersion.ID != version.ID {
			return
		}
		table, isTable := src.(*ast.TableExpr)
		if !isTable || table == nil {
			return
		}
		fresh = FreshTableLiteral{Table: table, Point: p}
		ok = true
	})
	return fresh, found, ok
}

func assignmentInvalidatesFreshness(info *cfg.AssignInfo, sym cfg.SymbolID, bindings IdentBindingLookup) bool {
	if info == nil {
		return false
	}
	for _, call := range info.SourceCalls {
		if call != nil {
			return true
		}
	}
	for i, target := range info.Targets {
		if target.Kind != cfg.TargetIdent {
			if target.BaseSymbol == sym || ExprReferencesSymbol(target.Expr, sym, bindings) {
				return true
			}
		}
		if ExprMayExposeSymbolValue(info.SourceAt(i), sym, bindings) {
			return true
		}
	}
	return false
}

func returnInvalidatesFreshness(info *cfg.ReturnInfo, sym cfg.SymbolID, bindings IdentBindingLookup) bool {
	if info == nil {
		return false
	}
	for _, call := range info.SourceCalls {
		if call != nil {
			return true
		}
	}
	for _, expr := range info.Exprs {
		if ExprMayExposeSymbolValue(expr, sym, bindings) {
			return true
		}
	}
	return false
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
