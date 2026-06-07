package canonical

import (
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

func (p solvedCallEvidenceProjection) expectedArgs() []api.CallExpectedArgEvidence {
	var out []api.CallExpectedArgEvidence
	for i, ev := range p.evidence.Calls {
		info := ev.Info
		if info == nil || info.Call == nil || len(info.Call.Args) == 0 {
			continue
		}
		ps, ok := callEventPointState(p.state, ev.Point)
		if !ok {
			continue
		}
		args := make([]typ.Type, len(info.Call.Args))
		any := false
		for argIdx := range info.Call.Args {
			expected := p.program.expectedCallArgType(p.graph, p.transfer, ev.Point, info, &ps, argIdx)
			if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
				continue
			}
			args[argIdx] = expected
			any = true
		}
		if !any {
			continue
		}
		if out == nil {
			out = make([]api.CallExpectedArgEvidence, len(p.evidence.Calls))
		}
		out[i] = api.NewCallExpectedArgEvidence(args)
	}
	return out
}

func (p solvedCallEvidenceProjection) contracts() []api.CallContractEvidence {
	ct := callTyper{d: p.driver, g: p.graph, ref: p.ref}
	var contracts []api.CallContractEvidence
	for i, ev := range p.evidence.Calls {
		if ev.Info == nil || ev.Info.Call == nil || len(ev.Info.Call.Args) == 0 {
			continue
		}
		ps, ok := p.state.Points[ev.Point]
		if !ok {
			continue
		}
		ctx := p.transfer.ProductCallContext(&ps, ev.Info.Call)
		frame, ok := ct.productCallFrame(ev.Info.Call, ctx, productCallOutcomeOptions{})
		if !ok {
			continue
		}
		demands := frame.callArgDemands()
		if len(demands) == 0 {
			continue
		}
		if contracts == nil {
			contracts = make([]api.CallContractEvidence, len(p.evidence.Calls))
		}
		contracts[i] = api.NewCallContractEvidence(demands)
	}
	return contracts
}

func callEventPointState(fs state.FunctionState, point cfg.Point) (flow.PointState, bool) {
	ps, ok := fs.Points[point]
	if ok {
		return ps, true
	}
	ps, ok = fs.InPoints[point]
	return ps, ok
}
