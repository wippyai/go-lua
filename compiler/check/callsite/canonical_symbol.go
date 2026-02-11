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
