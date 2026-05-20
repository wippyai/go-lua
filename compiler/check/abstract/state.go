package abstract

import (
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer"
	transfercore "github.com/wippyai/go-lua/compiler/check/abstract/transfer/core"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/narrow"
)

// TransferResult is the complete abstract-transfer output for one function.
type TransferResult struct {
	Inputs   *flow.Inputs
	Evidence api.FlowEvidence
}

// RunTransfer lowers CFG events into flow inputs and evidence streams.
func RunTransfer(ctx *transfercore.FlowContext) TransferResult {
	inputs := transfer.Run(ctx)
	return TransferResult{
		Inputs:   inputs,
		Evidence: transfer.ExtractEvidence(ctx, inputs),
	}
}

// Solve computes the flow-sensitive product state for transfer inputs.
func Solve(inputs *flow.Inputs, resolver narrow.Resolver) *flow.Solution {
	if inputs == nil {
		return nil
	}
	return flow.Solve(inputs, resolver)
}
