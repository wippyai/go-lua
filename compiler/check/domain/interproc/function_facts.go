package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
)

func collectCanonicalFunctionFactSymbols(factSets ...api.FunctionFacts) []cfg.SymbolID {
	var symbols functionsymbols.Set
	for _, facts := range factSets {
		for sym := range facts {
			symbols.Add(sym)
		}
	}
	return symbols.Slice()
}

func readFunctionFact(facts api.FunctionFacts, sym cfg.SymbolID) api.FunctionFact {
	if sym == 0 {
		return api.FunctionFact{}
	}
	if facts == nil {
		return api.FunctionFact{}
	}
	ff, ok := facts[sym]
	if !ok {
		return api.FunctionFact{}
	}
	if !functionfact.Empty(ff) {
		return ff
	}
	return api.FunctionFact{}
}

func writeNormalizedFunctionFact(facts api.FunctionFacts, sym cfg.SymbolID, ff api.FunctionFact) {
	if facts == nil || sym == 0 {
		return
	}
	ff = functionfact.Normalize(ff)

	if functionfact.Empty(ff) {
		delete(facts, sym)
	} else {
		facts[sym] = ff
	}
}
