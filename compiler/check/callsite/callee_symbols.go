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
//  3. symbol resolved from secondary bindings using call expression
//  4. method symbol resolved from primary bindings (receiver + method)
//  5. method symbol resolved from secondary bindings (receiver + method)
//  6. binding symbols with matching callee name (primary, then secondary)
func CalleeSymbolCandidates(info *cfg.CallInfo, primary, secondary *bind.BindingTable) []cfg.SymbolID {
	if info == nil {
		return nil
	}
	set := newSymbolSet(4)
	addExprSymbolCandidates(set, info.Callee, info.CalleeSymbol, primary, secondary)
	if methodSym, ok := methodCalleeSymbolFromCall(primary, nil, info); ok {
		set.Add(methodSym)
	}
	if secondary != nil && secondary != primary {
		if methodSym, ok := methodCalleeSymbolFromCall(secondary, nil, info); ok {
			set.Add(methodSym)
		}
	}
	if info.CalleeName != "" {
		if primary != nil {
			for _, sym := range primary.SymbolsByNameReadOnly(info.CalleeName) {
				set.Add(sym)
			}
		}
		if secondary != nil && secondary != primary {
			for _, sym := range secondary.SymbolsByNameReadOnly(info.CalleeName) {
				set.Add(sym)
			}
		}
	}
	return set.Slice()
}

// CallableCalleeSymbolCandidates returns deterministic callable symbols for a
// callsite, expanded through direct-alias chains.
//
// For method calls, the resolved method symbol is the callable authority and is
// emitted before receiver-path symbols. Resolver paths that need the receiver
// identity should use ResolverCalleeSymbolCandidates instead.
//
// Candidate order is preserved and symbols are deduplicated.
func CallableCalleeSymbolCandidates(
	info *cfg.CallInfo,
	graph *cfg.Graph,
	primary, secondary *bind.BindingTable,
) []cfg.SymbolID {
	base := CalleeSymbolCandidates(info, primary, secondary)
	set := newSymbolSet(len(base)*2 + 2)
	if IsMethodCallInfo(info) {
		addCallableMethodSymbolCandidates(set, info, graph, primary, secondary)
	}
	if graph == nil {
		if len(set.order) == 0 {
			return base
		}
		for _, sym := range base {
			set.Add(sym)
		}
		return set.Slice()
	}
	for _, sym := range base {
		addAliasExpansion(set, graph, sym)
	}

	// Method calls may resolve method symbol only through an alias receiver base
	// (for example, Alias:run() where Alias = T and T.run is defined).
	if !IsMethodCallInfo(info) {
		addCallableMethodSymbolCandidates(set, info, graph, primary, secondary)
	}

	candidates := set.Slice()
	if len(candidates) == 0 {
		return base
	}
	return candidates
}

func addCallableMethodSymbolCandidates(
	set *symbolSet,
	info *cfg.CallInfo,
	graph *cfg.Graph,
	primary, secondary *bind.BindingTable,
) {
	if methodSym, ok := methodCalleeSymbolFromCall(primary, graph, info); ok {
		addCallableSymbolCandidate(set, graph, methodSym)
	}
	if secondary != nil && secondary != primary {
		if methodSym, ok := methodCalleeSymbolFromCall(secondary, graph, info); ok {
			addCallableSymbolCandidate(set, graph, methodSym)
		}
	}
}

func addCallableSymbolCandidate(set *symbolSet, graph *cfg.Graph, sym cfg.SymbolID) {
	if graph == nil {
		set.Add(sym)
		return
	}
	addAliasExpansion(set, graph, sym)
}

// ResolverCalleeSymbolCandidates returns canonical callee candidates for
// resolver contexts.
//
// Selection order:
//  1. callee path symbol from call extraction (when present)
//  2. canonical callsite candidates with alias expansion
//
// Order is deterministic and candidates are deduplicated.
//
// NOTE: this intentionally includes the base callee-path symbol (often receiver
// identity for method calls). Use this for resolver-style symbol lookups that
// can tolerate non-callable intermediate symbols; for strict callable lookup
// paths, prefer CallableCalleeSymbolCandidates.
func ResolverCalleeSymbolCandidates(
	info *cfg.CallInfo,
	graph *cfg.Graph,
	primary, secondary *bind.BindingTable,
) []cfg.SymbolID {
	if info == nil {
		return nil
	}
	set := newSymbolSet(4)
	set.Add(info.CalleePath.Symbol)
	for _, sym := range CallableCalleeSymbolCandidates(info, graph, primary, secondary) {
		set.Add(sym)
	}
	return set.Slice()
}
