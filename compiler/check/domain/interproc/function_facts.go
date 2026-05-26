package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
)

func collectCanonicalFunctionFactSymbols(factSets ...api.FunctionFacts) []cfg.SymbolID {
	total := 0
	for _, facts := range factSets {
		total += len(facts)
	}
	symbols := make(map[cfg.SymbolID]bool, total)
	for _, facts := range factSets {
		markFunctionFactSymbols(symbols, facts)
	}
	return cfg.SortedSymbolIDs(symbols)
}

func markFunctionFactSymbols[T any](dst map[cfg.SymbolID]bool, src map[cfg.SymbolID]T) {
	for sym := range src {
		dst[sym] = true
	}
}

func readFunctionFactFromFacts(facts *api.Facts, sym cfg.SymbolID) api.FunctionFact {
	if facts == nil || sym == 0 {
		return api.FunctionFact{}
	}
	if facts.FunctionFacts == nil {
		return api.FunctionFact{}
	}
	ff, ok := facts.FunctionFacts[sym]
	if !ok {
		return api.FunctionFact{}
	}
	if !functionfact.Empty(ff) {
		return ff
	}
	return api.FunctionFact{}
}

func writeNormalizedFunctionFactToFacts(facts *api.Facts, sym cfg.SymbolID, ff api.FunctionFact) {
	if facts == nil || sym == 0 {
		return
	}
	ff = functionfact.Normalize(ff)

	if functionfact.Empty(ff) {
		if facts.FunctionFacts != nil {
			delete(facts.FunctionFacts, sym)
			if len(facts.FunctionFacts) == 0 {
				facts.FunctionFacts = nil
			}
		}
	} else {
		if facts.FunctionFacts == nil {
			facts.FunctionFacts = make(api.FunctionFacts)
		}
		facts.FunctionFacts[sym] = ff
	}
}
