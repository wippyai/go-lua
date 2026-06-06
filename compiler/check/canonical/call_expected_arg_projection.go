package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/domain/value/product"
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
	ctx     transfer.ProductCallContext
}

func (p *program) expectedArgProjection(g *cfg.Graph, tr *transfer.Transfer, point cfg.Point, info *cfg.CallInfo, in *flow.PointState) (expectedCallArgProjection, bool) {
	if p == nil || p.driver == nil || g == nil || tr == nil || info == nil || info.Call == nil || in == nil {
		return expectedCallArgProjection{}, false
	}
	return expectedCallArgProjection{
		program: p,
		graph:   g,
		typer:   callTyper{d: p.driver, g: g},
		point:   point,
		info:    info,
		ctx:     tr.ProductCallContext(in, info.Call),
	}, true
}

func (p expectedCallArgProjection) argType(argIdx int) typ.Type {
	if argIdx < 0 {
		return nil
	}
	call := p.info.Call
	expectedArgsInput := p.typer.expectedArgsInput(
		call,
		canonicalcall.ShallowArgTypes(call.Args, p.ctx.ArgTypes(), p.ctx.ExprType),
		p.ctx.ExprType,
		p.methodReceiverType(),
	)
	expectedArgsInput.Callee = p.calleeType()
	expectedArgsInput.IsMethod = callsite.IsMethodCallInfo(p.info)
	expectedArgsInput.MethodName = p.info.Method
	expectedArgsInput.ForceMethodReceiver = p.forceMethodReceiver()
	expectedArgs := canonicalcall.ExpectedArgTypesForCall(expectedArgsInput)
	if argIdx >= len(expectedArgs) {
		return nil
	}
	expected := expectedArgs[argIdx]
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

func (p expectedCallArgProjection) methodReceiverType() typ.Type {
	methodReceiver := p.ctx.SelfType
	if methodReceiver != nil && !typ.IsAbsentOrUnknown(methodReceiver) {
		return methodReceiver
	}
	return p.ctx.ExprType(p.info.Receiver)
}

func (p expectedCallArgProjection) calleeType() typ.Type {
	if callsite.IsMethodCallInfo(p.info) {
		return nil
	}
	return p.expectedCalleeType(p.info.Callee)
}

func (p expectedCallArgProjection) expectedCalleeType(expr ast.Expr) typ.Type {
	call := p.info.Call
	if p.typer.d != nil && p.typer.d.activeProgram != nil {
		if ref, ok := p.typer.targetResolver(p.typer.d.activeProgram).ResolveStaticCall(call); ok {
			if sig := p.typer.d.signatureForRef(p.typer.d.activeProgram, ref); sig != nil {
				return sig
			}
		}
	}
	if nested, ok := expr.(*ast.FuncCallExpr); ok && nested != nil {
		returns, ok := p.typer.CallReturnValues(nested, p.ctx.ForCall(nested))
		if ok && len(returns) > 0 {
			if t := product.ProjectValueOrUnknown(returns[0]); t != nil && !typ.IsAbsentOrUnknown(t) {
				return t
			}
		}
	}
	if demand, ok := p.typer.callDemandProjection(call, p.ctx.ExprType); ok {
		if fn := demand.functionShape(); fn != nil {
			return fn
		}
	}
	if expr != nil {
		if t := p.ctx.ExprType(expr); t != nil && !typ.IsAbsentOrUnknown(t) {
			return t
		}
	}
	return nil
}
