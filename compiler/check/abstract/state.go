package abstract

import (
	"github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
)

// Result is the complete abstract-interpretation output for one function.
type Result struct {
	Inputs   *flow.Inputs
	Evidence api.FlowEvidence
}

// Run lowers CFG events into flow inputs and evidence streams.
func Run(ctx *core.FlowContext) Result {
	inputs := BuildInputs(ctx)
	return Result{
		Inputs:   inputs,
		Evidence: ExtractEvidence(ctx, inputs),
	}
}

// Solve computes the flow-sensitive product state for abstract flow inputs.
func Solve(inputs *flow.Inputs, resolver narrow.Resolver) *flow.Solution {
	if inputs == nil {
		return nil
	}
	return flow.Solve(inputs, resolver)
}
