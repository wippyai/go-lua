package canonical

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// expectedCallArgProjection builds the call-site expected argument vector used
// for contextual function literal typing. It owns method receiver/callee
// normalization before delegating to the pure call-boundary matcher.
type expectedCallArgProjection struct {
	program *program
	graph   *cfg.Graph
	typer   callTyper
	point   cfg.Point
	info    *cfg.CallInfo
	site    callSiteFrame
}

func (p *program) expectedArgProjection(g *cfg.Graph, tr *transfer.Transfer, point cfg.Point, info *cfg.CallInfo, in *flow.PointState) (expectedCallArgProjection, bool) {
	if p == nil || p.driver == nil || g == nil || tr == nil || info == nil || info.Call == nil || in == nil {
		return expectedCallArgProjection{}, false
	}
	typer := callTyper{d: p.driver, g: g}
	site, ok := typer.productCallSiteFrame(info.Call, tr.ProductCallContext(in, info.Call))
	if !ok {
		return expectedCallArgProjection{}, false
	}
	return expectedCallArgProjection{
		program: p,
		graph:   g,
		typer:   typer,
		point:   point,
		info:    info,
		site:    site,
	}, true
}

func (p *program) expectedCallArgType(g *cfg.Graph, tr *transfer.Transfer, point cfg.Point, info *cfg.CallInfo, in *flow.PointState, argIdx int) typ.Type {
	projection, ok := p.expectedArgProjection(g, tr, point, info, in)
	if !ok {
		return nil
	}
	return projection.argType(argIdx)
}

func (p expectedCallArgProjection) argType(argIdx int) typ.Type {
	if argIdx < 0 {
		return nil
	}
	expectedArgs := p.site.expectedArgProjection()
	expectedArgs.ShallowFuncLiterals = true
	if !callsite.IsMethodCallInfo(p.info) {
		expectedArgs.Callee = p.site.expectedCalleeType(p.info.Callee)
	}
	expectedArgs.IsMethod = callsite.IsMethodCallInfo(p.info)
	expectedArgs.MethodName = p.info.Method
	expectedArgs.ForceMethodReceiver = p.forceMethodReceiver()
	expectedTypes := expectedArgs.ExpectedTypes()
	if argIdx >= len(expectedTypes) {
		return nil
	}
	expected := expectedTypes[argIdx]
	if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
		return nil
	}
	return expected
}

func (p expectedCallArgProjection) forceMethodReceiver() bool {
	ref, ok := p.program.refByGraph(p.graph)
	if !ok {
		return false
	}
	return callsite.ForceMethodReceiverAtPoint(p.graph.Bindings(), p.graph, p.program.inputs[ref].Evidence, p.point, p.info.Call)
}
