package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type callEntryProjector struct {
	program  *program
	graph    *cfg.Graph
	typer    callTyper
	transfer *transfer.Transfer
}

func (c callEntryProjector) argPath(_ int, arg ast.Expr) (constraint.Path, bool) {
	return c.typer.exprPath(arg)
}

func (c callEntryProjector) pointReferenceArgSources() summary.EntryReferenceArgSources {
	return summary.EntryReferenceArgSources{
		FunctionRefs: func(_ int, arg ast.Expr, in *flow.PointState) (flow.FunctionRefSet, bool) {
			return c.pointArgProjection(in).functionArgRefs(arg)
		},
		RefTrees: func(_ int, arg ast.Expr, in *flow.PointState) (flow.ReferenceContext, bool) {
			return c.pointArgProjection(in).argRefTrees(arg)
		},
		ClosureRefs: func(_ int, arg ast.Expr, in *flow.PointState) (flow.ClosureRefSet, bool) {
			return c.pointArgProjection(in).closureArgRefs(arg)
		},
	}
}

func (c callEntryProjector) productReferenceArgSources(ctx transfer.ProductCallContext) summary.EntryReferenceArgSources {
	projection := c.productArgProjection(ctx)
	return summary.EntryReferenceArgSources{
		FunctionRefs: func(_ int, arg ast.Expr, _ *flow.PointState) (flow.FunctionRefSet, bool) {
			return projection.functionArgRefs(arg)
		},
		RefTrees: func(_ int, arg ast.Expr, _ *flow.PointState) (flow.ReferenceContext, bool) {
			return projection.argRefTrees(arg)
		},
		ClosureRefs: func(_ int, arg ast.Expr, _ *flow.PointState) (flow.ClosureRefSet, bool) {
			return projection.closureArgRefs(arg)
		},
	}
}

func (c callEntryProjector) entryEvidenceProjection() summary.CallEntryContextProjection {
	return summary.CallEntryContextProjection{
		ParamSlot: c.paramSlot,
		ParamSlotCount: func(callee summary.FuncRef, _ *ast.FuncCallExpr) int {
			return c.program.paramSlotCount(callee)
		},
		ParamPath: func(callee summary.FuncRef, slot int) (constraint.Path, bool) {
			return c.program.paramPath(callee, slot)
		},
		ArgPath: c.argPath,
		ReferencePaths: func(callee summary.FuncRef) flow.ReferencePathProjection {
			return c.program.referenceProjection(callee)
		},
	}
}

func (c callEntryProjector) productEvidence(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) summary.DirectEntryEvidence {
	return c.entryEvidenceProjection().DirectEvidence(summary.DirectEntryEvidenceInput{
		Callee:        ref,
		Call:          call,
		RuntimeValues: ctx.RuntimeArgValues,
		References:    ctx.References,
		ArgSources:    c.productReferenceArgSources(ctx),
		KeyPresence:   ctx.KeyPresence,
		StaticMembers: ctx.StaticMembers,
		Num:           ctx.Num,
		IndexWrites:   ctx.IndexWrites,
	})
}

func (c callEntryProjector) productCallbackRefs(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) map[ast.Expr][]summary.FuncRef {
	projection := c.productArgProjection(ctx)
	return callbackRefsForCall(call, func(arg ast.Expr) ([]summary.FuncRef, bool) {
		return projection.callbackRefs(arg, 0)
	})
}

func (c callEntryProjector) referenceCallbackRefs(call *ast.FuncCallExpr, references flow.ReferenceContext) map[ast.Expr][]summary.FuncRef {
	projection := c.referenceArgProjection(references)
	return callbackRefsForCall(call, func(arg ast.Expr) ([]summary.FuncRef, bool) {
		return projection.callbackRefs(arg, 0)
	})
}

func callbackRefsForCall(call *ast.FuncCallExpr, resolve func(ast.Expr) ([]summary.FuncRef, bool)) map[ast.Expr][]summary.FuncRef {
	if call == nil || len(call.Args) == 0 || resolve == nil {
		return nil
	}
	out := make(map[ast.Expr][]summary.FuncRef)
	for _, arg := range call.Args {
		refs, ok := resolve(arg)
		if !ok || len(refs) == 0 {
			continue
		}
		out[arg] = refs
	}
	return out
}

func (c callEntryProjector) pointFacts(ref summary.FuncRef, call *ast.FuncCallExpr, in *flow.PointState) flow.BoundaryFacts {
	if in == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	return c.entryEvidenceProjection().DirectEvidence(summary.DirectEntryEvidenceInput{
		Callee:        ref,
		Call:          call,
		KeyPresence:   in.KeyPresence,
		StaticMembers: in.StaticMembers,
		Num:           in.Num,
		IndexWrites:   in.IndexWrites,
	}).Facts
}

// callEntryProjector is the program-owned capability bundle for summary
// call-entry projection. Summary owns the pure projection algebra; this type owns
// the driver/program lookups needed to instantiate that algebra for one caller.
func (p *program) callEntryProjector(ref summary.FuncRef) (callEntryProjector, bool) {
	if p == nil || p.driver == nil {
		return callEntryProjector{}, false
	}
	g := p.Graph(ref)
	tr, ok := p.transfers[ref].(*transfer.Transfer)
	if g == nil || !ok || tr == nil {
		return callEntryProjector{}, false
	}
	return callEntryProjector{
		program:  p,
		graph:    g,
		typer:    callTyper{d: p.driver, g: g},
		transfer: tr,
	}, true
}

func (p *program) ProjectCallEntryPublication(ref summary.FuncRef, fs state.FunctionState) summary.CallEntryPublications {
	projector, ok := p.callEntryProjector(ref)
	if !ok {
		return nil
	}
	return projector.publicationProjection(fs).Project()
}

func (p *program) ProjectCallEntryContextKeys(ref summary.FuncRef, fs state.FunctionState) []summary.Key {
	projector, ok := p.callEntryProjector(ref)
	if !ok {
		return nil
	}
	return projector.contextProjection(fs).ProjectKeys()
}

func (c callEntryProjector) publicationProjection(fs state.FunctionState) summary.CallEntryPublicationProjection {
	return summary.CallEntryPublicationProjection{
		Graph: c.graph,
		State: fs,
		ResolveTargets: func(call *ast.FuncCallExpr, in *flow.PointState) []summary.CallEntryTarget {
			return c.resolveTargets(call, in)
		},
		ResolveCallback: func(arg ast.Expr, rawSym cfg.SymbolID, in *flow.PointState) ([]summary.FuncRef, bool) {
			return c.resolveCallback(arg, rawSym, in)
		},
		ExpectedArgType: func(point cfg.Point, info *cfg.CallInfo, in *flow.PointState, argIdx int) typ.Type {
			return c.expectedArgType(point, info, in, argIdx)
		},
		ParamSlot: c.paramSlot,
		ParamAnnotated: func(callee summary.FuncRef, sourceParam int) bool {
			_, slot, ok := paramevidence.ParamSlotForSourceParam(c.program.Graph(callee), c.program.funcExpr(callee), sourceParam)
			if !ok {
				return false
			}
			return c.program.paramSlotFixed(callee, slot)
		},
		ParamSlotCount: func(callee summary.FuncRef, _ *ast.FuncCallExpr) int {
			return c.program.paramSlotCount(callee)
		},
		ParamPath: func(callee summary.FuncRef, slot int) (constraint.Path, bool) {
			return c.program.paramPath(callee, slot)
		},
		ArgPath: c.argPath,
		ReferencePaths: func(callee summary.FuncRef) flow.ReferencePathProjection {
			return c.program.referenceProjection(callee)
		},
		EvalArg: c.transfer.EvalExprValue,
	}
}

func (c callEntryProjector) contextProjection(fs state.FunctionState) summary.CallEntryContextProjection {
	return summary.CallEntryContextProjection{
		Graph: c.graph,
		State: fs,
		ResolveTargets: func(call *ast.FuncCallExpr, in *flow.PointState) []summary.CallEntryTarget {
			return c.resolveTargets(call, in)
		},
		ResolveCallback: func(arg ast.Expr, rawSym cfg.SymbolID, in *flow.PointState) ([]summary.FuncRef, bool) {
			return c.resolveCallback(arg, rawSym, in)
		},
		ExpectedArgType: func(point cfg.Point, info *cfg.CallInfo, in *flow.PointState, argIdx int) typ.Type {
			return c.expectedArgType(point, info, in, argIdx)
		},
		ParamSlot: c.paramSlot,
		ParamSlotCount: func(callee summary.FuncRef, _ *ast.FuncCallExpr) int {
			return c.program.paramSlotCount(callee)
		},
		ParamPath: func(callee summary.FuncRef, slot int) (constraint.Path, bool) {
			return c.program.paramPath(callee, slot)
		},
		ArgPath:             c.argPath,
		ReferenceArgSources: c.pointReferenceArgSources(),
		EvalArg:             c.transfer.EvalExprValue,
		NormalizeValues: func(callee summary.FuncRef, call *ast.FuncCallExpr, values summary.EntryValues) summary.EntryValues {
			return c.program.withPrototypeMethodSurfacesForMethodCall(callee, call, values)
		},
		ReferencePaths: func(callee summary.FuncRef) flow.ReferencePathProjection {
			return c.program.referenceProjection(callee)
		},
	}
}

func (c callEntryProjector) resolveTargets(call *ast.FuncCallExpr, in *flow.PointState) []summary.CallEntryTarget {
	if c.program == nil || c.program.driver == nil || c.graph == nil || in == nil {
		return nil
	}
	targets := c.typer.resolveCallTargets(call, c.program, flow.ReferenceContextFromPoint(in))
	selected := targets.Select().Targets()
	out := make([]summary.CallEntryTarget, 0, len(selected))
	references := flow.ReferenceContextFromPoint(in)
	for _, target := range selected {
		ref := target.Ref()
		entryFacts := c.pointFacts(ref, call, in)
		ctx := c.program.CallEntryContextWithFacts(ref, references, nil, entryFacts)
		if closure, ok := target.Closure(); ok {
			ctx = canonicalcall.EntryContextFromClosureWithLiveContext(closure, ctx)
		}
		out = append(out, summary.CallEntryTarget{
			Ref:             ctx.Ref(),
			EntryReferences: ctx.References(),
			EntryFacts:      ctx.EntryFacts(),
		})
	}
	return out
}

func (c callEntryProjector) resolveCallback(arg ast.Expr, rawSym cfg.SymbolID, in *flow.PointState) ([]summary.FuncRef, bool) {
	return c.pointArgProjection(in).callbackRefs(arg, rawSym)
}

func (c callEntryProjector) paramSlot(callee summary.FuncRef, call *ast.FuncCallExpr, argIdx int) (int, int, bool) {
	return paramevidence.ParamSlotForRuntimeArg(c.program.Graph(callee), c.program.funcExpr(callee), argIdx)
}

func (ct callTyper) callEntryProjector() (callEntryProjector, bool) {
	if ct.d == nil || ct.d.activeProgram == nil {
		return callEntryProjector{}, false
	}
	ref, ok := ct.currentRef()
	if !ok {
		return callEntryProjector{
			program: ct.d.activeProgram,
			graph:   ct.g,
			typer:   ct,
		}, true
	}
	if projector, ok := ct.d.activeProgram.callEntryProjector(ref); ok {
		return projector, true
	}
	return callEntryProjector{
		program: ct.d.activeProgram,
		graph:   ct.g,
		typer:   ct,
	}, true
}

func (ct callTyper) callEntryValuesForRef(ref summary.FuncRef, call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) summary.EntryValues {
	projector, ok := ct.callEntryProjector()
	if !ok {
		return nil
	}
	return projector.valuesForRef(ref, call, exprType)
}

func (c callEntryProjector) slotType(ref summary.FuncRef, call *ast.FuncCallExpr, runtimeValues []product.AbstractValue, entryValues summary.EntryValues, slot int) typ.Type {
	if slot < 0 {
		return nil
	}
	for runtimeIdx, av := range runtimeValues {
		if av.IsZero() || product.Domain.Equal(av, product.Domain.Top()) {
			continue
		}
		_, mappedSlot, ok := c.paramSlot(ref, call, runtimeIdx)
		if !ok || mappedSlot != slot {
			continue
		}
		return product.ProjectValueOrUnknown(av)
	}
	if av, ok := entryValues[slot]; ok && !av.IsZero() {
		return product.ProjectValueOrUnknown(av)
	}
	return nil
}

func (c callEntryProjector) valuesForRef(ref summary.FuncRef, call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) summary.EntryValues {
	if call == nil || exprType == nil {
		return nil
	}
	runtimeValues := make([]product.AbstractValue, callsite.RuntimeArgExprCount(call))
	for i := range runtimeValues {
		arg := callsite.RuntimeArgExprAt(call, i)
		if arg == nil {
			continue
		}
		t := exprType(arg)
		if t == nil || typ.IsAbsentOrUnknown(t) {
			continue
		}
		runtimeValues[i] = product.FromType(t)
	}
	return c.entryEvidenceProjection().DirectEvidence(summary.DirectEntryEvidenceInput{
		Callee:        ref,
		Call:          call,
		RuntimeValues: runtimeValues,
	}).Values
}

func (c callEntryProjector) productEntryContext(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) canonicalcall.EntryContext {
	evidence := c.productEvidence(ref, call, ctx)
	values := c.program.withPrototypeMethodSurfacesForMethodCall(ref, call, evidence.Values)
	references := ctx.References.Join(evidence.References)
	return c.program.CallEntryContextWithFacts(
		ref,
		references,
		values,
		evidence.Facts,
	)
}
