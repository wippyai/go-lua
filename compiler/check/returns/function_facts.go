package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

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
	if facts.FunctionFacts == nil {
		return api.FunctionFact{}
	}
	ff := facts.FunctionFacts[sym]
	return functionFactFromChannels(ff.Summary, ff.Narrow, ff.Func)
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

	if len(ff.Summary) > 0 {
		if facts.ReturnSummaries == nil {
			facts.ReturnSummaries = make(api.ReturnSummaries)
		}
		facts.ReturnSummaries[sym] = ff.Summary
	} else if facts.ReturnSummaries != nil {
		delete(facts.ReturnSummaries, sym)
		if len(facts.ReturnSummaries) == 0 {
			facts.ReturnSummaries = nil
		}
	}

	if len(ff.Narrow) > 0 {
		if facts.NarrowReturns == nil {
			facts.NarrowReturns = make(api.NarrowReturnSummaries)
		}
		facts.NarrowReturns[sym] = ff.Narrow
	} else if facts.NarrowReturns != nil {
		delete(facts.NarrowReturns, sym)
		if len(facts.NarrowReturns) == 0 {
			facts.NarrowReturns = nil
		}
	}

	if ff.Func != nil {
		if facts.FuncTypes == nil {
			facts.FuncTypes = make(api.FuncTypes)
		}
		facts.FuncTypes[sym] = ff.Func
	} else if facts.FuncTypes != nil {
		delete(facts.FuncTypes, sym)
		if len(facts.FuncTypes) == 0 {
			facts.FuncTypes = nil
		}
	}
}

func canonicalFunctionFacts(facts api.Facts) api.FunctionFacts {
	if len(facts.FunctionFacts) == 0 {
		return nil
	}
	symbols := cfg.SortedSymbolIDs(facts.FunctionFacts)
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
