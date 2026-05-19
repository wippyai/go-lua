package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
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

func NormalizeFunctionFact(ff api.FunctionFact) api.FunctionFact {
	return api.FunctionFact{
		Summary: canonicalReturnVector(ff.Summary),
		Narrow:  canonicalReturnVector(ff.Narrow),
		Type:    normalizeInterprocValueType(ff.Type),
	}
}

func functionFactEmpty(ff api.FunctionFact) bool {
	return len(ff.Summary) == 0 && len(ff.Narrow) == 0 && ff.Type == nil
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
	canonical := NormalizeFunctionFact(ff)
	if !functionFactEmpty(canonical) {
		return canonical
	}
	return api.FunctionFact{}
}

func normalizeFunctionFactMap(facts api.FunctionFacts) api.FunctionFacts {
	if len(facts) == 0 {
		return nil
	}
	out := make(api.FunctionFacts, len(facts))
	for _, sym := range cfg.SortedSymbolIDs(facts) {
		canonical := NormalizeFunctionFact(facts[sym])
		if functionFactEmpty(canonical) {
			continue
		}
		out[sym] = canonical
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func writeFunctionFactToFacts(facts *api.Facts, sym cfg.SymbolID, ff api.FunctionFact) {
	writeNormalizedFunctionFactToFacts(facts, sym, NormalizeFunctionFact(ff))
}

func writeNormalizedFunctionFactToFacts(facts *api.Facts, sym cfg.SymbolID, ff api.FunctionFact) {
	if facts == nil || sym == 0 {
		return
	}

	if functionFactEmpty(ff) {
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

// NormalizeFunctionFacts canonicalizes the stored FunctionFacts map.
func NormalizeFunctionFacts(facts *api.Facts) {
	if facts == nil {
		return
	}
	facts.FunctionFacts = normalizeFunctionFactMap(facts.FunctionFacts)
}
