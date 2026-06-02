package predicate

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/flow"
)

// LookupPredicateLink finds the predicate link for a variable.
// When multiple links exist for the same symbol (different definition points),
// returns the one with the highest def point for deterministic results.
func LookupPredicateLink(sym cfg.SymbolID, inputs *flow.Inputs) *flow.PredicateLink {
	if inputs == nil || inputs.PredicateLinks == nil || sym == 0 {
		return nil
	}
	var bestDef cfg.Point
	found := false
	var bestLink flow.PredicateLink
	for key, link := range inputs.PredicateLinks {
		if key.Symbol != sym {
			continue
		}
		if !found || key.DefPoint > bestDef {
			found = true
			bestDef = key.DefPoint
			bestLink = link
		}
	}
	if !found {
		return nil
	}
	return &bestLink
}

// LinkKey returns the typed key for a predicate-link assignment.
func LinkKey(sym cfg.SymbolID, defPoint cfg.Point) flow.PredicateLinkKey {
	if sym == 0 {
		return flow.PredicateLinkKey{}
	}
	return flow.PredicateLinkKey{Symbol: sym, DefPoint: defPoint}
}

// BuildConstResolver creates a const resolver function for a given CFG point.
// Uses the graph's SymbolAt to resolve name -> SymbolID for const lookup.
func BuildConstResolver(inputs *flow.Inputs, p cfg.Point) func(string) *flow.ConstValue {
	if inputs == nil || inputs.ConstValues == nil || inputs.Graph == nil {
		return nil
	}
	return func(name string) *flow.ConstValue {
		sym, ok := inputs.Graph.SymbolAt(p, name)
		if !ok {
			return nil
		}
		if at := inputs.ConstValues[sym]; at != nil {
			return at[p]
		}
		return nil
	}
}
