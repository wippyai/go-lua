package callsite

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// CalleeSymbolCandidates returns deterministic candidate symbols for a callsite.
//
// Candidate order:
//  1. raw call callee symbol
//  2. symbol resolved from primary bindings using call expression
//  3. symbol resolved from fallback bindings using call expression
//  4. binding symbols with matching callee name (primary, then fallback)
func CalleeSymbolCandidates(info *cfg.CallInfo, primary, fallback *bind.BindingTable) []cfg.SymbolID {
	if info == nil {
		return nil
	}
	candidates := make([]cfg.SymbolID, 0, 4)
	seen := make(map[cfg.SymbolID]struct{}, 4)
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

	push(info.CalleeSymbol)
	push(SymbolFromExpr(info.Callee, primary))
	if fallback != primary {
		push(SymbolFromExpr(info.Callee, fallback))
	}
	if info.CalleeName != "" {
		if primary != nil {
			for _, sym := range primary.SymbolsByName(info.CalleeName) {
				push(sym)
			}
		}
		if fallback != nil && fallback != primary {
			for _, sym := range fallback.SymbolsByName(info.CalleeName) {
				push(sym)
			}
		}
	}
	return candidates
}
