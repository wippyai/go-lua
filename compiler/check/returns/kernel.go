package returns

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ReconcileFunctionFactInput captures all channels that can influence a single
// local-function fact slot during one update step.
type ReconcileFunctionFactInput struct {
	ExistingSummary []typ.Type
	ExistingNarrow  []typ.Type
	ExistingFunc    typ.Type

	CandidateSummary []typ.Type
	CandidateNarrow  []typ.Type
	CandidateFunc    typ.Type
}

// ReconcileFunctionFactOutput is the canonical reconciled state for one symbol.
type ReconcileFunctionFactOutput struct {
	Summary []typ.Type
	Narrow  []typ.Type
	Func    typ.Type
}

// FunctionFactCandidate captures incoming candidate data for one symbol's
// function-related fact channels.
type FunctionFactCandidate struct {
	Summary []typ.Type
	Narrow  []typ.Type
	Func    typ.Type
}

// ReconcileFunctionFact centralizes reconciliation of return summary, narrow
// return summary, and function type for one symbol.
//
// This is the only policy entrypoint for function-fact channel convergence.
func ReconcileFunctionFact(in ReconcileFunctionFactInput) ReconcileFunctionFactOutput {
	out := ReconcileFunctionFactOutput{
		Summary: NormalizeReturnVector(in.ExistingSummary),
		Narrow:  NormalizeReturnVector(in.ExistingNarrow),
		Func:    in.ExistingFunc,
	}

	if len(in.CandidateSummary) > 0 {
		out.Summary = MergeReturnSummary(out.Summary, in.CandidateSummary)
	}
	if len(in.CandidateNarrow) > 0 {
		out.Narrow = MergeReturnSummary(out.Narrow, in.CandidateNarrow)
	}
	if in.CandidateFunc != nil {
		out.Func = MergeFunctionFactType(out.Func, in.CandidateFunc)
	}

	// Keep summary and narrow channels mutually refining when post-flow narrow
	// provides strictly better first-order information.
	if len(out.Narrow) > 0 {
		if len(out.Summary) == 0 {
			out.Summary = NormalizeReturnVector(out.Narrow)
		} else if ReturnTypesRefine(out.Narrow, out.Summary) ||
			ReturnTypesElideOptional(out.Narrow, out.Summary) ||
			ReturnTypesExtendRecord(out.Narrow, out.Summary) {
			out.Summary = MergeReturnSummary(out.Summary, out.Narrow)
		}
	}

	if fn := unwrap.Function(out.Func); fn != nil {
		alignedSummary := out.Summary
		if len(alignedSummary) == 0 && len(out.Narrow) > 0 {
			alignedSummary = out.Narrow
		}
		if len(alignedSummary) > 0 {
			if aligned, changed := AlignFunctionTypeWithSummary(fn, alignedSummary); changed {
				out.Func = aligned
				fn = aligned
			}
		}
		if len(out.Summary) == 0 && fn != nil && len(fn.Returns) > 0 {
			out.Summary = NormalizeReturnVector(fn.Returns)
		}
	}

	return out
}

// MergeFunctionFactIntoFacts reconciles and writes function-related facts for
// one symbol into a facts bundle using canonical kernel policy.
func MergeFunctionFactIntoFacts(facts *api.Facts, sym cfg.SymbolID, candidate FunctionFactCandidate) {
	if facts == nil || sym == 0 {
		return
	}

	reconciled := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary:  facts.ReturnSummaries[sym],
		ExistingNarrow:   facts.NarrowReturns[sym],
		ExistingFunc:     facts.FuncTypes[sym],
		CandidateSummary: candidate.Summary,
		CandidateNarrow:  candidate.Narrow,
		CandidateFunc:    candidate.Func,
	})

	if len(reconciled.Summary) > 0 {
		if facts.ReturnSummaries == nil {
			facts.ReturnSummaries = make(api.ReturnSummaries, 1)
		}
		facts.ReturnSummaries[sym] = reconciled.Summary
	}

	if len(reconciled.Narrow) > 0 {
		if facts.NarrowReturns == nil {
			facts.NarrowReturns = make(api.NarrowReturnSummaries, 1)
		}
		facts.NarrowReturns[sym] = reconciled.Narrow
	}

	if reconciled.Func != nil {
		if facts.FuncTypes == nil {
			facts.FuncTypes = make(api.FuncTypes, 1)
		}
		facts.FuncTypes[sym] = reconciled.Func
	}
}

// MergeFunctionFactsIntoFacts merges full function-fact channel maps into facts
// via the canonical single-symbol reconciliation path.
func MergeFunctionFactsIntoFacts(
	facts *api.Facts,
	summaries api.ReturnSummaries,
	narrows api.NarrowReturnSummaries,
	funcs api.FuncTypes,
) {
	if facts == nil {
		return
	}
	for _, sym := range collectFunctionFactSymbols(summaries, narrows, funcs) {
		MergeFunctionFactIntoFacts(facts, sym, FunctionFactCandidate{
			Summary: summaries[sym],
			Narrow:  narrows[sym],
			Func:    funcs[sym],
		})
	}
}

func collectFunctionFactSymbols(
	summaries api.ReturnSummaries,
	narrows api.NarrowReturnSummaries,
	funcs api.FuncTypes,
) []cfg.SymbolID {
	symbols := make(map[cfg.SymbolID]bool, len(summaries)+len(narrows)+len(funcs))
	markFunctionFactSymbols(symbols, summaries)
	markFunctionFactSymbols(symbols, narrows)
	markFunctionFactSymbols(symbols, funcs)
	return cfg.SortedSymbolIDs(symbols)
}

func collectFunctionFactSymbolsFromPairs(
	prevSummary api.ReturnSummaries,
	nextSummary api.ReturnSummaries,
	prevNarrow api.NarrowReturnSummaries,
	nextNarrow api.NarrowReturnSummaries,
	prevFuncs api.FuncTypes,
	nextFuncs api.FuncTypes,
) []cfg.SymbolID {
	symbols := make(map[cfg.SymbolID]bool, len(prevSummary)+len(nextSummary)+len(prevNarrow)+len(nextNarrow)+len(prevFuncs)+len(nextFuncs))
	markFunctionFactSymbols(symbols, prevSummary)
	markFunctionFactSymbols(symbols, nextSummary)
	markFunctionFactSymbols(symbols, prevNarrow)
	markFunctionFactSymbols(symbols, nextNarrow)
	markFunctionFactSymbols(symbols, prevFuncs)
	markFunctionFactSymbols(symbols, nextFuncs)
	return cfg.SortedSymbolIDs(symbols)
}

func markFunctionFactSymbols[T any](dst map[cfg.SymbolID]bool, src map[cfg.SymbolID]T) {
	for _, sym := range cfg.SortedSymbolIDs(src) {
		dst[sym] = true
	}
}
