package returns

import (
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
