package callsite

import (
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// CalleeSymbolCandidates returns deterministic candidate symbols for a callsite.
//
// Candidate order:
//  1. raw call callee symbol
//  2. canonical callee symbol from call expression/bindings
//  3. fallback-binding symbols with matching callee name
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
	push(CanonicalSymbolFromExpr(info.Callee, info.CalleeSymbol, primary, fallback, nil))
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
