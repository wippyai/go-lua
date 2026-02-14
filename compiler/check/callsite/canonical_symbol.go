package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// CanonicalSymbolFromExpr chooses a stable symbol identity for an expression.
//
// Candidate order is:
//  1. raw symbol (if non-zero)
//  2. symbol resolved from primary bindings
//  3. symbol resolved from fallback bindings
//
// When prefer is provided, the first candidate satisfying prefer(sym) is
// returned. Otherwise, the first non-zero candidate is returned.
func CanonicalSymbolFromExpr(
	expr ast.Expr,
	raw cfg.SymbolID,
	primary *bind.BindingTable,
	fallback *bind.BindingTable,
	prefer func(cfg.SymbolID) bool,
) cfg.SymbolID {
	return selectPreferredSymbol(exprSymbolCandidates(expr, raw, primary, fallback), prefer)
}

// CanonicalSymbolFromExprWithAliases chooses a stable symbol identity for an
// expression and expands candidates through direct-alias chains when graph is
// provided.
func CanonicalSymbolFromExprWithAliases(
	expr ast.Expr,
	raw cfg.SymbolID,
	graph *cfg.Graph,
	primary *bind.BindingTable,
	fallback *bind.BindingTable,
	prefer func(cfg.SymbolID) bool,
) cfg.SymbolID {
	base := exprSymbolCandidates(expr, raw, primary, fallback)
	if len(base) == 0 {
		return 0
	}
	if graph == nil {
		return selectPreferredSymbol(base, prefer)
	}
	candidates := expandAliasCandidates(base, graph)
	return selectPreferredSymbol(candidates, prefer)
}
