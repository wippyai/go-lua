package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

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
	selector := preferredSymbolSelector{prefer: prefer}
	visitExprSymbolCandidates(expr, raw, primary, fallback, func(sym cfg.SymbolID) bool {
		return visitAliasExpansion(graph, sym, selector.Add)
	})
	return selector.selected
}
