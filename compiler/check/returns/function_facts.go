package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func functionFactSymbols(facts api.Facts) []cfg.SymbolID {
	return collectCanonicalFunctionFactSymbols(facts.FunctionFacts)
}

func collectFunctionFactChannelSymbols(
	summaries api.ReturnSummaries,
	narrows api.NarrowReturnSummaries,
	funcs api.FuncTypes,
	facts api.FunctionFacts,
) []cfg.SymbolID {
	symbols := make(map[cfg.SymbolID]bool, len(summaries)+len(narrows)+len(funcs)+len(facts))
	markFunctionFactSymbols(symbols, summaries)
	markFunctionFactSymbols(symbols, narrows)
	markFunctionFactSymbols(symbols, funcs)
	markFunctionFactSymbols(symbols, facts)
	return cfg.SortedSymbolIDs(symbols)
}

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

func setOrDeleteReturnSummary(m *api.ReturnSummaries, sym cfg.SymbolID, summary []typ.Type) {
	if len(summary) > 0 {
		if *m == nil {
			*m = make(api.ReturnSummaries)
		}
		(*m)[sym] = summary
		return
	}
	if *m == nil {
		return
	}
	delete(*m, sym)
	if len(*m) == 0 {
		*m = nil
	}
}

func setOrDeleteNarrowSummary(m *api.NarrowReturnSummaries, sym cfg.SymbolID, narrow []typ.Type) {
	if len(narrow) > 0 {
		if *m == nil {
			*m = make(api.NarrowReturnSummaries)
		}
		(*m)[sym] = narrow
		return
	}
	if *m == nil {
		return
	}
	delete(*m, sym)
	if len(*m) == 0 {
		*m = nil
	}
}

func setOrDeleteFuncType(m *api.FuncTypes, sym cfg.SymbolID, fn typ.Type) {
	if fn != nil {
		if *m == nil {
			*m = make(api.FuncTypes)
		}
		(*m)[sym] = fn
		return
	}
	if *m == nil {
		return
	}
	delete(*m, sym)
	if len(*m) == 0 {
		*m = nil
	}
}

func functionFactFromChannels(summary, narrow []typ.Type, fn typ.Type) api.FunctionFact {
	return api.FunctionFact{
		Summary: NormalizeReturnVector(summary),
		Narrow:  NormalizeReturnVector(narrow),
		Func:    fn,
	}
}

func readFunctionFactFromFacts(facts *api.Facts, sym cfg.SymbolID) api.FunctionFact {
	if facts == nil || sym == 0 {
		return api.FunctionFact{}
	}
	if facts.FunctionFacts != nil {
		ff, ok := facts.FunctionFacts[sym]
		if ok {
			canonical := functionFactFromChannels(ff.Summary, ff.Narrow, ff.Func)
			if len(canonical.Summary) > 0 || len(canonical.Narrow) > 0 || canonical.Func != nil {
				return canonical
			}
		}
	}
	return functionFactFromChannels(facts.ReturnSummaries[sym], facts.NarrowReturns[sym], facts.FuncTypes[sym])
}

func writeFunctionFactToFacts(facts *api.Facts, sym cfg.SymbolID, ff api.FunctionFact) {
	if facts == nil || sym == 0 {
		return
	}

	ff = functionFactFromChannels(ff.Summary, ff.Narrow, ff.Func)
	if len(ff.Summary) == 0 && len(ff.Narrow) == 0 && ff.Func == nil {
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
	setOrDeleteReturnSummary(&facts.ReturnSummaries, sym, ff.Summary)
	setOrDeleteNarrowSummary(&facts.NarrowReturns, sym, ff.Narrow)
	setOrDeleteFuncType(&facts.FuncTypes, sym, ff.Func)
}

// SummaryViewFromFacts returns the canonical summary channel view derived from
// FunctionFacts.
func SummaryViewFromFacts(facts api.Facts) api.ReturnSummaries {
	symbols := functionFactSymbols(facts)
	if len(symbols) == 0 {
		return nil
	}
	out := make(api.ReturnSummaries, len(symbols))
	for _, sym := range symbols {
		ff := facts.FunctionFacts[sym]
		if len(ff.Summary) > 0 {
			out[sym] = ff.Summary
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NarrowViewFromFacts returns the canonical narrow-summary channel view derived
// from FunctionFacts.
func NarrowViewFromFacts(facts api.Facts) api.NarrowReturnSummaries {
	symbols := functionFactSymbols(facts)
	if len(symbols) == 0 {
		return nil
	}
	out := make(api.NarrowReturnSummaries, len(symbols))
	for _, sym := range symbols {
		ff := facts.FunctionFacts[sym]
		if len(ff.Narrow) > 0 {
			out[sym] = ff.Narrow
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FuncTypeViewFromFacts returns the canonical function-type channel view
// derived from FunctionFacts.
func FuncTypeViewFromFacts(facts api.Facts) api.FuncTypes {
	symbols := functionFactSymbols(facts)
	if len(symbols) == 0 {
		return nil
	}
	out := make(api.FuncTypes, len(symbols))
	for _, sym := range symbols {
		ff := facts.FunctionFacts[sym]
		if ff.Func != nil {
			out[sym] = ff.Func
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NormalizeFunctionFactChannels reconciles legacy function channels into
// canonical FunctionFacts, then rewrites mirrors from canonical values.
func NormalizeFunctionFactChannels(facts *api.Facts) {
	if facts == nil {
		return
	}
	symbols := collectFunctionFactChannelSymbols(
		facts.ReturnSummaries,
		facts.NarrowReturns,
		facts.FuncTypes,
		facts.FunctionFacts,
	)
	if len(symbols) == 0 {
		return
	}
	for _, sym := range symbols {
		ff := readFunctionFactFromFacts(facts, sym)
		writeFunctionFactToFacts(facts, sym, ff)
	}
}

func canonicalFunctionFacts(facts api.Facts) api.FunctionFacts {
	symbols := functionFactSymbols(facts)
	if len(symbols) == 0 {
		return nil
	}

	out := make(api.FunctionFacts, len(symbols))
	factsCopy := facts
	for _, sym := range symbols {
		ff := readFunctionFactFromFacts(&factsCopy, sym)
		if len(ff.Summary) == 0 && len(ff.Narrow) == 0 && ff.Func == nil {
			continue
		}
		out[sym] = ff
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
