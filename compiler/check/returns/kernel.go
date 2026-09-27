package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
	typjoin "github.com/wippyai/go-lua/types/typ/join"
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

	// Solved flow supersedes an incomparable pre-flow estimate. A join of the
	// two would introduce return shapes that no flow path actually produced.
	if len(out.Narrow) > 0 {
		if len(out.Summary) == 0 {
			out.Summary = NormalizeReturnVector(out.Narrow)
		} else if !returnVectorsComparable(out.Summary, out.Narrow) {
			out.Summary = NormalizeReturnVector(out.Narrow)
		} else {
			out.Summary = MergeReturnSummary(out.Summary, out.Narrow)
		}
	}

	if fn := typ.GeneralMember(out.Func); fn != nil {
		alignedSummary := out.Summary
		if len(out.Narrow) > 0 {
			// Canonical tie-breaker: function facts track post-flow behavior.
			// Narrow summaries are produced from solved flow and are authoritative
			// for call-site typing in the current iteration.
			alignedSummary = out.Narrow
		}
		if len(alignedSummary) > 0 {
			var aligned *typ.Function
			var changed bool
			if len(out.Narrow) > 0 && !returnVectorsComparable(fn.Returns, alignedSummary) {
				if _, direct := out.Func.(*typ.Function); direct {
					aligned = typjoin.WithReturns(fn, alignedSummary)
					changed = aligned != nil && !ReturnTypesEqual(fn.Returns, alignedSummary)
				}
			}
			if aligned == nil {
				aligned, changed = AlignFunctionTypeWithSummary(fn, alignedSummary)
			}
			if changed {
				if inter, ok := out.Func.(*typ.Intersection); ok {
					out.Func = withOverloadGeneral(inter, aligned)
				} else {
					out.Func = aligned
				}
				fn = aligned
			}
		}
		if len(out.Summary) == 0 && fn != nil && len(fn.Returns) > 0 {
			out.Summary = NormalizeReturnVector(fn.Returns)
		}
	}

	return out
}

func mergeCallable(existing, candidate api.FunctionFact) api.FunctionFact {
	reconciled := ReconcileFunctionFact(ReconcileFunctionFactInput{
		ExistingSummary:  existing.Summary,
		ExistingNarrow:   existing.Narrow,
		ExistingFunc:     existing.Func,
		CandidateSummary: candidate.Summary,
		CandidateNarrow:  candidate.Narrow,
		CandidateFunc:    candidate.Func,
	})
	sig := existing.Sig
	if candidate.Sig != nil {
		sig = mergeLiteralSig(sig, candidate.Sig)
	}
	return api.FunctionFact{
		Summary: reconciled.Summary,
		Narrow:  reconciled.Narrow,
		Func:    reconciled.Func,
		Sig:     sig,
	}
}

// MergeCallable reconciles one literal's facts, including its contextual signature.
func MergeCallable(facts *api.Facts, fn *ast.FunctionExpr, candidate api.FunctionFact) {
	if facts == nil || fn == nil {
		return
	}
	if facts.Callables == nil {
		facts.Callables = make(api.Callables)
	}
	facts.Callables[fn] = mergeCallable(facts.Callables[fn], candidate)
}
