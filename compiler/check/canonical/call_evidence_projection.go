package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// solvedCallEvidenceProjection is the diagnostic bridge from abstract
// interpreter call events to solved call-edge evidence. It does not infer new
// facts; it reprojects already-solved transfer state into the API carriers used
// by later checks.
type solvedCallEvidenceProjection struct {
	driver   *Driver
	program  *program
	ref      summary.FuncRef
	graph    *cfg.Graph
	transfer *transfer.Transfer
	state    state.FunctionState
	evidence api.FlowEvidence
}

type solvedCallEdgeEvidence struct {
	ExpectedArgs []api.CallExpectedArgEvidence
	Contracts    []api.CallContractEvidence
}

func (d *Driver) solvedCallEvidenceProjection(prog *program, ref summary.FuncRef, evidence api.FlowEvidence) (solvedCallEvidenceProjection, bool) {
	if d == nil || prog == nil || len(evidence.Calls) == 0 {
		return solvedCallEvidenceProjection{}, false
	}
	g := prog.Graph(ref)
	tr, _ := prog.transfers[ref].(*transfer.Transfer)
	fs, ok := d.states[ref]
	if g == nil || tr == nil || !ok {
		return solvedCallEvidenceProjection{}, false
	}
	return solvedCallEvidenceProjection{
		driver:   d,
		program:  prog,
		ref:      ref,
		graph:    g,
		transfer: tr,
		state:    fs,
		evidence: evidence,
	}, true
}

func (p solvedCallEvidenceProjection) project() solvedCallEdgeEvidence {
	var out solvedCallEdgeEvidence
	ct := callTyper{d: p.driver, g: p.graph, ref: p.ref}
	for i, ev := range p.evidence.Calls {
		info := ev.Info
		if info == nil || info.Call == nil || len(info.Call.Args) == 0 {
			continue
		}
		if ps, ok := callEventPointState(p.state, ev.Point); ok {
			if args, ok := p.expectedArgsForCall(ev.Point, info, &ps); ok {
				if out.ExpectedArgs == nil {
					out.ExpectedArgs = make([]api.CallExpectedArgEvidence, len(p.evidence.Calls))
				}
				out.ExpectedArgs[i] = args
			}
		}
		if ps, ok := p.state.Points[ev.Point]; ok {
			contracts, ok := p.contractsForCall(ct, info.Call, &ps)
			if !ok {
				continue
			}
			if out.Contracts == nil {
				out.Contracts = make([]api.CallContractEvidence, len(p.evidence.Calls))
			}
			out.Contracts[i] = contracts
		}
	}
	return out
}

func (p solvedCallEvidenceProjection) expectedArgsForCall(point cfg.Point, info *cfg.CallInfo, ps *flow.PointState) (api.CallExpectedArgEvidence, bool) {
	args := make([]typ.Type, len(info.Call.Args))
	any := false
	for argIdx := range info.Call.Args {
		expected := p.program.expectedCallArgType(p.graph, p.transfer, point, info, ps, argIdx)
		if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
			continue
		}
		args[argIdx] = expected
		any = true
	}
	if !any {
		return api.CallExpectedArgEvidence{}, false
	}
	return api.NewCallExpectedArgEvidence(args), true
}

func (p solvedCallEvidenceProjection) contractsForCall(ct callTyper, call *ast.FuncCallExpr, ps *flow.PointState) (api.CallContractEvidence, bool) {
	ctx := p.transfer.ProductCallContext(ps, call)
	frame, ok := ct.productCallFrame(call, ctx, productCallOutcomeOptions{})
	if !ok {
		return api.CallContractEvidence{}, false
	}
	demands := frame.callArgDemands()
	if len(demands) == 0 {
		return api.CallContractEvidence{}, false
	}
	return api.NewCallContractEvidence(demands), true
}

func callEventPointState(fs state.FunctionState, point cfg.Point) (flow.PointState, bool) {
	ps, ok := fs.Points[point]
	if ok {
		return ps, true
	}
	ps, ok = fs.InPoints[point]
	return ps, ok
}
