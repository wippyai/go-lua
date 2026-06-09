package canonical

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/flow"
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

func (d *Driver) solvedCallEvidenceProjection(prog *program, artifact canonicalSolveArtifact, ref summary.FuncRef, evidence api.FlowEvidence) (solvedCallEvidenceProjection, bool) {
	if d == nil || prog == nil || len(evidence.Calls) == 0 {
		return solvedCallEvidenceProjection{}, false
	}
	g := prog.Graph(ref)
	tr, _ := prog.transfers[ref].(*transfer.Transfer)
	fs, ok := artifact.States[ref]
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
			expectation, ok := p.expectationForCall(ct, ev.Point, info, &ps)
			if !ok {
				continue
			}
			if expected, hasExpected := apiExpectedArgsFromExpectation(expectation); hasExpected {
				if out.ExpectedArgs == nil {
					out.ExpectedArgs = make([]api.CallExpectedArgEvidence, len(p.evidence.Calls))
				}
				out.ExpectedArgs[i] = expected
			}
			_, hasPostState := p.state.Points[ev.Point]
			if contracts, hasContracts := apiContractsFromExpectation(expectation); hasPostState && hasContracts {
				if out.Contracts == nil {
					out.Contracts = make([]api.CallContractEvidence, len(p.evidence.Calls))
				}
				out.Contracts[i] = contracts
			}
		}
	}
	return out
}

func (p solvedCallEvidenceProjection) expectationForCall(
	ct callTyper,
	point cfg.Point,
	info *cfg.CallInfo,
	ps *flow.PointState,
) (canonicalcall.CallExpectation, bool) {
	if info == nil || info.Call == nil || ps == nil {
		return canonicalcall.CallExpectation{}, false
	}
	ctx := p.transfer.ProductCallContext(ps, info.Call)
	frame, ok := ct.callBoundaryFrame(info.Call, ctx, productCallOutcomeOptions{})
	if !ok {
		return canonicalcall.CallExpectation{}, false
	}
	return frame.expectation(info, p.forceMethodReceiver(point, info)), true
}

func apiExpectedArgsFromExpectation(expectation canonicalcall.CallExpectation) (api.CallExpectedArgEvidence, bool) {
	if !expectation.HasExpectedArgs() {
		return api.CallExpectedArgEvidence{}, false
	}
	return api.NewCallExpectedArgEvidence(expectation.CloneExpectedArgs()), true
}

func apiContractsFromExpectation(expectation canonicalcall.CallExpectation) (api.CallContractEvidence, bool) {
	if !expectation.HasArgDemands() {
		return api.CallContractEvidence{}, false
	}
	return api.NewCallContractEvidence(expectation.CloneArgDemands()), true
}

func (p solvedCallEvidenceProjection) forceMethodReceiver(point cfg.Point, info *cfg.CallInfo) bool {
	if p.program == nil || p.graph == nil || info == nil || info.Call == nil {
		return false
	}
	return callsite.ForceMethodReceiverAtPoint(p.graph.Bindings(), p.graph, p.program.inputs[p.ref].Evidence, point, info.Call)
}

func callEventPointState(fs state.FunctionState, point cfg.Point) (flow.PointState, bool) {
	ps, ok := fs.Points[point]
	if ok {
		return ps, true
	}
	ps, ok = fs.InPoints[point]
	return ps, ok
}
