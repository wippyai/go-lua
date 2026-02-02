package phase

import (
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
)

// RunSolve executes the flow solve phase.
func RunSolve(input FlowSolveInput) FlowSolveOutput {
	var solution *flow.Solution
	if input.Extract.Inputs != nil {
		solution = flow.Solve(input.Extract.Inputs, input.Resolver)
	}
	return FlowSolveOutput{
		Solution: solution,
	}
}

// RunSolveWithResolver is a convenience function that takes resolver directly.
func RunSolveWithResolver(inputs *flow.Inputs, resolver narrow.Resolver) FlowSolveOutput {
	var solution *flow.Solution
	if inputs != nil {
		solution = flow.Solve(inputs, resolver)
	}
	return FlowSolveOutput{
		Solution: solution,
	}
}
