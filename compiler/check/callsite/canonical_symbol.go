package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func canonicalExprSymbolCandidates(
	expr ast.Expr,
	raw cfg.SymbolID,
	primary *bind.BindingTable,
	fallback *bind.BindingTable,
) []cfg.SymbolID {
	candidates := make([]cfg.SymbolID, 0, 3)
	seen := make(map[cfg.SymbolID]struct{}, 3)
	push := func(sym cfg.SymbolID) {
		if sym == 0 {
			return
		}
		if _, ok := seen[sym]; ok {
			return
		}
		seen[sym] = struct{}{}
		candidates = append(candidates, sym)
	}

	push(raw)
	push(SymbolFromExpr(expr, primary))
	if fallback != primary {
		push(SymbolFromExpr(expr, fallback))
	}

	return candidates
}

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
	candidates := canonicalExprSymbolCandidates(expr, raw, primary, fallback)
	best := cfg.SymbolID(0)
	for _, sym := range candidates {
		if best == 0 {
			best = sym
		}
		if prefer != nil && prefer(sym) {
			return sym
		}
	}
	return best
}

// CanonicalSymbolFromExprWithAliases chooses a stable symbol identity for an
// expression and expands candidates with direct aliases when graph is provided.
func CanonicalSymbolFromExprWithAliases(
	expr ast.Expr,
	raw cfg.SymbolID,
	graph *cfg.Graph,
	primary *bind.BindingTable,
	fallback *bind.BindingTable,
	prefer func(cfg.SymbolID) bool,
) cfg.SymbolID {
	base := canonicalExprSymbolCandidates(expr, raw, primary, fallback)
	if len(base) == 0 {
		return 0
	}

	candidates := base
	if graph != nil {
		candidates = make([]cfg.SymbolID, 0, len(base)*2)
		seen := make(map[cfg.SymbolID]struct{}, len(base)*2)
		push := func(sym cfg.SymbolID) {
			if sym == 0 {
				return
			}
			if _, ok := seen[sym]; ok {
				return
			}
			seen[sym] = struct{}{}
			candidates = append(candidates, sym)
		}

		for _, sym := range base {
			push(sym)
			if aliasSym := graph.DirectAliasSymbol(sym); aliasSym != 0 {
				push(aliasSym)
			}
		}
	}

	best := cfg.SymbolID(0)
	for _, sym := range candidates {
		if best == 0 {
			best = sym
		}
		if prefer != nil && prefer(sym) {
			return sym
		}
	}
	return best
}
