package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
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

// callEntryAccess is the normalized read surface for projecting one call's
// arguments into callee entry evidence. It owns expression path conversion and
// point/product argument reference reads so summary projections do not each wire
// their own path/reference closures.
type callEntryAccess struct {
	projector callEntryProjector
}

func (c callEntryProjector) access() callEntryAccess {
	return callEntryAccess{projector: c}
}

func (a callEntryAccess) argPath(_ int, arg ast.Expr) (constraint.Path, bool) {
	return a.projector.typer.exprPath(arg)
}

func (a callEntryAccess) pointFunctionArgRefs(_ int, arg ast.Expr, in *flow.PointState) (flow.FunctionRefSet, bool) {
	return a.projector.pointArgProjection(in).functionArgRefs(arg)
}

func (a callEntryAccess) pointFunctionArgTreeRefs(_ int, arg ast.Expr, in *flow.PointState) (flow.FunctionRefs, bool) {
	return a.projector.pointArgProjection(in).functionArgTreeRefs(arg)
}

func (a callEntryAccess) pointClosureArgRefs(_ int, arg ast.Expr, in *flow.PointState) (flow.ClosureRefSet, bool) {
	return a.projector.pointArgProjection(in).closureArgRefs(arg)
}

func (a callEntryAccess) pointClosureArgTreeRefs(_ int, arg ast.Expr, in *flow.PointState) (flow.ClosureRefs, bool) {
	return a.projector.pointArgProjection(in).closureArgTreeRefs(arg)
}

func (a callEntryAccess) productFunctionArgRefs(ctx transfer.ProductCallContext, arg ast.Expr) (flow.FunctionRefSet, bool) {
	return a.projector.productArgProjection(ctx).functionArgRefs(arg)
}

func (a callEntryAccess) productFunctionArgTreeRefs(ctx transfer.ProductCallContext, arg ast.Expr) (flow.FunctionRefs, bool) {
	return a.projector.productArgProjection(ctx).functionArgTreeRefs(arg)
}

func (a callEntryAccess) productClosureArgRefs(ctx transfer.ProductCallContext, arg ast.Expr) (flow.ClosureRefSet, bool) {
	return a.projector.productArgProjection(ctx).closureArgRefs(arg)
}

func (a callEntryAccess) productClosureArgTreeRefs(ctx transfer.ProductCallContext, arg ast.Expr) (flow.ClosureRefs, bool) {
	return a.projector.productArgProjection(ctx).closureArgTreeRefs(arg)
}

func (a callEntryAccess) pointFacts(ref summary.FuncRef, call *ast.FuncCallExpr, in *flow.PointState) flow.BoundaryFacts {
	if in == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	return a.facts(ref, call, summary.DirectCallEntryFactAxes{
		KeyPresence: in.KeyPresence,
		Num:         in.Num,
		IndexWrites: in.IndexWrites,
	})
}

func (a callEntryAccess) productFacts(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.BoundaryFacts {
	return a.facts(ref, call, summary.DirectCallEntryFactAxes{
		KeyPresence: ctx.KeyPresence,
		Num:         ctx.Num,
		IndexWrites: ctx.IndexWrites,
	})
}

func (a callEntryAccess) facts(ref summary.FuncRef, call *ast.FuncCallExpr, axes summary.DirectCallEntryFactAxes) flow.BoundaryFacts {
	return summary.DirectCallEntryFacts(summary.DirectCallEntryFactInput{
		Call:                    call,
		Callee:                  ref,
		ParamSlot:               a.projector.paramSlot,
		ArgPath:                 a.argPath,
		DirectCallEntryFactAxes: axes,
	})
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

func (c callEntryProjector) valueProjection(fs state.FunctionState) summary.CallEntryValueProjection {
	return summary.CallEntryValueProjection{
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
		EvalArg: c.transfer.EvalExprValue,
	}
}

func (c callEntryProjector) contextProjection(fs state.FunctionState) summary.CallEntryContextProjection {
	access := c.access()
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
		ArgPath:            access.argPath,
		FunctionArgRefs:    access.pointFunctionArgRefs,
		FunctionArgRefTree: access.pointFunctionArgTreeRefs,
		ClosureArgRefs:     access.pointClosureArgRefs,
		ClosureArgRefTree:  access.pointClosureArgTreeRefs,
		EvalArg:            c.transfer.EvalExprValue,
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
	access := c.access()
	targets := c.typer.resolveCallTargets(call, c.program, in.FunctionRefs, in.ClosureRefs)
	selected := canonicalcall.SelectTargets(targets).Targets()
	out := make([]summary.CallEntryTarget, 0, len(selected))
	for _, target := range selected {
		ref := target.Ref()
		cells := c.program.CallEntryCells(ref, in.Cells)
		refs := c.program.CallEntryFunctionRefs(ref, in.FunctionRefs)
		closures := c.program.CallEntryClosureRefs(ref, in.ClosureRefs)
		entryFacts := access.pointFacts(ref, call, in)
		entry := canonicalcall.NewEntryContextWithFacts(ref, cells, refs, closures, nil, entryFacts)
		if closure, ok := target.Closure(); ok {
			entry = canonicalcall.EntryContextFromClosureWithLiveAxesAndFacts(ref, closure, cells, refs, closures, nil, entryFacts)
		}
		out = append(out, summary.CallEntryTarget{
			Ref:               entry.Ref(),
			EntryCells:        entry.CaptureCells(),
			EntryFunctionRefs: entry.FunctionRefs(),
			EntryClosureRefs:  entry.ClosureRefs(),
			EntryFacts:        entry.EntryFacts(),
		})
	}
	return out
}

func (c callEntryProjector) resolveCallback(arg ast.Expr, rawSym cfg.SymbolID, in *flow.PointState) ([]summary.FuncRef, bool) {
	return c.pointArgProjection(in).callbackRefs(arg, rawSym)
}

func (c callEntryProjector) expectedArgType(point cfg.Point, info *cfg.CallInfo, in *flow.PointState, argIdx int) typ.Type {
	return c.program.expectedCallArgType(c.graph, c.transfer, point, info, in, argIdx)
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
	return summary.DirectCallEntryValuesWithParamCount(
		call,
		ref,
		exprType,
		c.paramSlot,
		func(callee summary.FuncRef, _ *ast.FuncCallExpr) int {
			return c.program.paramSlotCount(callee)
		},
	)
}

func (ct callTyper) productCallEntryContext(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (canonicalcall.EntryContext, bool) {
	projector, ok := ct.callEntryProjector()
	if !ok {
		return canonicalcall.EntryContext{}, false
	}
	return projector.productContext(ref, call, ctx)
}

func (ct callTyper) productClosureCallEntryContext(ref summary.FuncRef, closure flow.ClosureRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (canonicalcall.EntryContext, bool) {
	projector, ok := ct.callEntryProjector()
	if !ok {
		return canonicalcall.EntryContext{}, false
	}
	return projector.productClosureContext(ref, closure, call, ctx)
}

// productEntryAxes is the normalized callee-entry evidence shared by direct and
// closure product calls; only the final EntryContext constructor differs.
type productEntryAxes struct {
	ref      summary.FuncRef
	cells    flow.CaptureCells
	refs     flow.FunctionRefs
	closures flow.ClosureRefs
	values   summary.EntryValues
	facts    flow.BoundaryFacts
}

func (c callEntryProjector) productContext(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (canonicalcall.EntryContext, bool) {
	axes := c.productAxes(ref, call, ctx)
	return canonicalcall.NewEntryContextWithFacts(
		axes.ref,
		axes.cells,
		axes.refs,
		axes.closures,
		axes.values,
		axes.facts,
	), true
}

func (c callEntryProjector) productClosureContext(ref summary.FuncRef, closure flow.ClosureRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (canonicalcall.EntryContext, bool) {
	axes := c.productAxes(ref, call, ctx)
	return canonicalcall.EntryContextFromClosureWithLiveAxesAndFacts(
		axes.ref,
		closure,
		axes.cells,
		axes.refs,
		axes.closures,
		axes.values,
		axes.facts,
	), true
}

func (c callEntryProjector) productAxes(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) productEntryAxes {
	return productEntryAxes{
		ref:   ref,
		cells: c.program.CallEntryCells(ref, ctx.Cells),
		refs: flow.FunctionRefsDomain.Join(
			c.program.CallEntryFunctionRefs(ref, ctx.FunctionRefs),
			c.functionRefsForRef(ref, call, ctx),
		),
		closures: flow.ClosureRefsDomain.Join(
			c.program.CallEntryClosureRefs(ref, ctx.ClosureRefs),
			c.closureRefsForRef(ref, call, ctx),
		),
		values: c.productValuesForRef(ref, call, ctx.RuntimeArgValues),
		facts:  c.factsForRef(ref, call, ctx),
	}
}

func (c callEntryProjector) factsForRef(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.BoundaryFacts {
	return c.access().productFacts(ref, call, ctx)
}

func (c callEntryProjector) productValuesForRef(ref summary.FuncRef, call *ast.FuncCallExpr, runtimeValues []product.AbstractValue) summary.EntryValues {
	if call == nil {
		return nil
	}
	values := summary.DirectCallEntryProductValuesWithParamCount(
		call,
		ref,
		runtimeValues,
		c.paramSlot,
		func(callee summary.FuncRef, _ *ast.FuncCallExpr) int {
			return c.program.paramSlotCount(callee)
		},
	)
	return c.program.withPrototypeMethodSurfacesForMethodCall(ref, call, values)
}

func (c callEntryProjector) functionRefsForRef(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.FunctionRefs {
	if call == nil {
		return flow.FunctionRefsDomain.Bottom()
	}
	access := c.access()
	in := c.referenceInput(ref, call)
	in.FunctionRefs = ctx.FunctionRefs
	in.ResolveFunctionArg = func(_ int, arg ast.Expr, _ *flow.PointState) (flow.FunctionRefSet, bool) {
		return access.productFunctionArgRefs(ctx, arg)
	}
	in.ResolveFunctionArgRefs = func(_ int, arg ast.Expr, _ *flow.PointState) (flow.FunctionRefs, bool) {
		return access.productFunctionArgTreeRefs(ctx, arg)
	}
	return summary.DirectCallEntryFunctionRefs(in)
}

func (c callEntryProjector) closureRefsForRef(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.ClosureRefs {
	if call == nil {
		return flow.ClosureRefsDomain.Bottom()
	}
	access := c.access()
	in := c.referenceInput(ref, call)
	in.ClosureRefs = ctx.ClosureRefs
	in.ResolveClosureArg = func(_ int, arg ast.Expr, _ *flow.PointState) (flow.ClosureRefSet, bool) {
		return access.productClosureArgRefs(ctx, arg)
	}
	in.ResolveClosureArgRefs = func(_ int, arg ast.Expr, _ *flow.PointState) (flow.ClosureRefs, bool) {
		return access.productClosureArgTreeRefs(ctx, arg)
	}
	return summary.DirectCallEntryClosureRefs(in)
}

func (c callEntryProjector) referenceInput(ref summary.FuncRef, call *ast.FuncCallExpr) summary.DirectCallEntryReferenceInput {
	return summary.DirectCallEntryReferenceInput{
		Call:                call,
		Callee:              ref,
		ReferenceProjection: c.program.referenceProjection(ref),
		LimitReferencePaths: true,
		ParamSlot:           c.paramSlot,
		ParamPath: func(callee summary.FuncRef, slot int) (constraint.Path, bool) {
			return c.program.paramPath(callee, slot)
		},
		ArgPath: c.access().argPath,
	}
}
