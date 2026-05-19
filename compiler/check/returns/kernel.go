package returns

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// JoinFunctionFact precisely merges two observations for one local function
// during a single analysis iteration.
func JoinFunctionFact(existing, candidate api.FunctionFact) api.FunctionFact {
	existing = NormalizeFunctionFact(existing)
	candidate = NormalizeFunctionFact(candidate)
	out := existing

	if len(candidate.Params) > 0 {
		out.Params = paramevidence.JoinVectors(out.Params, candidate.Params)
	}
	if len(candidate.Summary) > 0 {
		out.Summary = MergeReturnSummary(out.Summary, candidate.Summary)
	}
	if len(candidate.Narrow) > 0 {
		out.Narrow = MergeReturnSummary(out.Narrow, candidate.Narrow)
	}
	if candidate.Type != nil {
		out.Type = MergeFunctionFactType(out.Type, candidate.Type)
	}

	// Keep summary and post-flow narrow results mutually refining when narrow
	// provides first-order information. MergeReturnSummary is the canonical
	// policy and already encodes directional refinement preference.
	if len(out.Narrow) > 0 {
		if len(out.Summary) == 0 {
			out.Summary = canonicalReturnVector(out.Narrow)
		} else {
			out.Summary = MergeReturnSummary(out.Summary, out.Narrow)
		}
	}

	if fn := unwrap.Function(out.Type); fn != nil {
		alignedSummary := out.Summary
		if len(alignedSummary) > 0 {
			if aligned, changed := AlignFunctionTypeWithSummary(fn, alignedSummary); changed {
				out.Type = aligned
				fn = aligned
			}
		}
		if len(out.Summary) == 0 && fn != nil && len(fn.Returns) > 0 {
			out.Summary = canonicalReturnVector(fn.Returns)
		}
	}

	return out
}
