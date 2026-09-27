package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

// SummaryViewFromFacts derives symbol summaries from the canonical facts.
func SummaryViewFromFacts(facts api.Facts) api.ReturnSummaries {
	var out api.ReturnSummaries
	for _, sym := range cfg.SortedSymbolIDs(facts.FunctionFacts) {
		if summary := facts.FunctionFacts[sym].Summary; len(summary) > 0 {
			if out == nil {
				out = make(api.ReturnSummaries)
			}
			out[sym] = summary
		}
	}
	return out
}

// NarrowViewFromFacts derives solved return summaries from the canonical facts.
func NarrowViewFromFacts(facts api.Facts) api.NarrowReturnSummaries {
	var out api.NarrowReturnSummaries
	for _, sym := range cfg.SortedSymbolIDs(facts.FunctionFacts) {
		if narrow := facts.FunctionFacts[sym].Narrow; len(narrow) > 0 {
			if out == nil {
				out = make(api.NarrowReturnSummaries)
			}
			out[sym] = narrow
		}
	}
	return out
}

// FuncTypeViewFromFacts derives symbol function types from the canonical facts.
func FuncTypeViewFromFacts(facts api.Facts) api.FuncTypes {
	var out api.FuncTypes
	for _, sym := range cfg.SortedSymbolIDs(facts.FunctionFacts) {
		if fn := facts.FunctionFacts[sym].Func; fn != nil {
			if out == nil {
				out = make(map[cfg.SymbolID]typ.Type)
			}
			out[sym] = fn
		}
	}
	return out
}
