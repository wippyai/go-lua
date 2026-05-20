package phase

import (
	"github.com/wippyai/go-lua/compiler/check/abstract"
)

// RunSolve executes the flow solve phase.
func RunSolve(input FlowSolveInput) FlowSolveOutput {
	return FlowSolveOutput{
		Solution: abstract.Solve(input.Extract.Inputs, input.Resolver),
	}
}
