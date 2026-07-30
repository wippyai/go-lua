package body

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func testPrepareCallOutcome(t *testing.T, program callpayload.CallOutcomeProgram, ctx transfer.NodeContext, site factflow.CallSiteView) callpayload.CallOutcomeSiteProgram {
	t.Helper()
	prepared, err := program.PrepareSite(ctx, site)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func testEvaluateCallOutcome(t *testing.T, program callpayload.CallOutcomeProgram, ctx transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput) callpayload.CallOutcome {
	t.Helper()
	prepared := testPrepareCallOutcome(t, program, ctx, site)
	outcome, err := prepared.Evaluate(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func sealedBodyCallInput(t *testing.T, program callpayload.CallOutcomeProgram, ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, operands callpayload.CallOutcomeValueOperands) callpayload.CallOutcomeInput {
	t.Helper()
	domain := state.RegisteredProductDomain(ctx.Registry)
	point := ctx.Point
	if point == 0 {
		point = 1
	}
	capability := testPrepareCallOutcome(t, program, ctx, site).Capability()
	access, err := factapply.SealExternalCallTransferAccess(domain, []state.TransferInputAccess{{}}, []cfg.Point{point}, 0, capability, nil)
	if err != nil {
		t.Fatal(err)
	}
	inputProgram, err := callpayload.PrepareExternalCallInputProgram(domain, access, []cfg.Point{point}, 0, func(root statekey.Value) (statekey.Value, bool) { return root, true })
	if err != nil {
		t.Fatal(err)
	}
	frame, err := callpayload.BindConcreteExternalCallInputFrame(&inputProgram, []state.State{in}, []callpayload.DiagnosticOutput{{}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := frame.BindCallOutcomeInput(operands)
	if err != nil {
		t.Fatal(err)
	}
	return input
}
