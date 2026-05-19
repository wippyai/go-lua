package returns

import (
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/returnsummary"
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
		out.Summary = returnsummary.Merge(out.Summary, candidate.Summary)
	}
	if len(candidate.Narrow) > 0 {
		out.Narrow = returnsummary.Merge(out.Narrow, candidate.Narrow)
	}
	if candidate.Type != nil {
		out.Type = MergeFunctionFactType(out.Type, candidate.Type)
	}

	// Keep summary and post-flow narrow results mutually refining when narrow
	// provides first-order information. returnsummary.Merge is the canonical
	// policy and already encodes directional refinement preference.
	if len(out.Narrow) > 0 {
		if len(out.Summary) == 0 {
			out.Summary = returnsummary.Canonical(out.Narrow)
		} else {
			out.Summary = returnsummary.Merge(out.Summary, out.Narrow)
		}
	}

	if fn := unwrap.Function(out.Type); fn != nil {
		alignedSummary := out.Summary
		if len(alignedSummary) > 0 {
			if aligned, changed := returnsummary.AlignFunction(fn, alignedSummary); changed {
				out.Type = aligned
				fn = aligned
			}
		}
		if len(out.Summary) == 0 && fn != nil && len(fn.Returns) > 0 {
			out.Summary = returnsummary.Canonical(fn.Returns)
		}
	}

	return out
}
