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
//  4. method symbol resolved from primary bindings (receiver + method)
//  5. method symbol resolved from fallback bindings (receiver + method)
//  6. binding symbols with matching callee name (primary, then fallback)
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
	if methodSym, ok := MethodCalleeSymbolWithAliases(primary, nil, info); ok {
		push(methodSym)
	}
	if fallback != nil && fallback != primary {
		if methodSym, ok := MethodCalleeSymbolWithAliases(fallback, nil, info); ok {
			push(methodSym)
		}
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

// PreferredCalleeSymbol selects a single symbol from callsite candidates.
//
// Selection rule:
//  1. start with the first candidate (if any)
//  2. if prefer is provided, pick the first candidate where prefer(sym) is true
func PreferredCalleeSymbol(
	info *cfg.CallInfo,
	primary, fallback *bind.BindingTable,
	prefer func(cfg.SymbolID) bool,
) cfg.SymbolID {
	selected := cfg.SymbolID(0)
	for _, sym := range CalleeSymbolCandidates(info, primary, fallback) {
		if selected == 0 {
			selected = sym
		}
		if prefer != nil && prefer(sym) {
			return sym
		}
	}
	return selected
}

// PreferredCalleeSymbolWithAliases selects a single symbol from alias-expanded candidates.
//
// Selection rule:
//  1. start with the first candidate (if any)
//  2. if prefer is provided, pick the first candidate where prefer(sym) is true
func PreferredCalleeSymbolWithAliases(
	info *cfg.CallInfo,
	graph *cfg.Graph,
	primary, fallback *bind.BindingTable,
	prefer func(cfg.SymbolID) bool,
) cfg.SymbolID {
	selected := cfg.SymbolID(0)
	for _, sym := range CalleeSymbolCandidatesWithAliases(info, graph, primary, fallback) {
		if selected == 0 {
			selected = sym
		}
		if prefer != nil && prefer(sym) {
			return sym
		}
	}
	return selected
}

// CalleeSymbolCandidatesWithAliases expands callee candidates through direct-alias
// chains and includes method symbols resolvable through alias receiver bases.
//
// Candidate order is preserved and symbols are deduplicated.
func CalleeSymbolCandidatesWithAliases(
	info *cfg.CallInfo,
	graph *cfg.Graph,
	primary, fallback *bind.BindingTable,
) []cfg.SymbolID {
	base := CalleeSymbolCandidates(info, primary, fallback)
	candidates := make([]cfg.SymbolID, 0, len(base)*2+2)
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
		graph.EachAliasSymbol(sym, func(candidate cfg.SymbolID) bool {
			push(candidate)
			return false
		})
	}

	// Method calls may resolve method symbol only through an alias receiver base
	// (for example, Alias:run() where Alias = T and T.run is defined).
	if methodSym, ok := MethodCalleeSymbolWithAliases(primary, graph, info); ok {
		graph.EachAliasSymbol(methodSym, func(candidate cfg.SymbolID) bool {
			push(candidate)
			return false
		})
	}
	if fallback != nil && fallback != primary {
		if methodSym, ok := MethodCalleeSymbolWithAliases(fallback, graph, info); ok {
			graph.EachAliasSymbol(methodSym, func(candidate cfg.SymbolID) bool {
				push(candidate)
				return false
			})
		}
	}

	if len(candidates) == 0 {
		return base
	}
	return candidates
}
