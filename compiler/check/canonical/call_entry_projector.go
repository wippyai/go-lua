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
		ArgPath: func(_ int, arg ast.Expr) (constraint.Path, bool) {
			return c.typer.exprPath(arg)
		},
		FunctionArgRefs: func(_ int, arg ast.Expr, in *flow.PointState) (flow.FunctionRefSet, bool) {
			return c.program.callEntryFunctionArgRefs(c.graph, arg, in)
		},
		FunctionArgRefTree: func(_ int, arg ast.Expr, in *flow.PointState) (flow.FunctionRefs, bool) {
			return c.program.callEntryFunctionArgTreeRefs(c.graph, c.transfer, arg, in)
		},
		ClosureArgRefs: func(_ int, arg ast.Expr, in *flow.PointState) (flow.ClosureRefSet, bool) {
			return c.program.callEntryClosureArgRefs(c.graph, arg, in)
		},
		ClosureArgRefTree: func(_ int, arg ast.Expr, in *flow.PointState) (flow.ClosureRefs, bool) {
			return c.program.callEntryClosureArgTreeRefs(c.graph, c.transfer, arg, in)
		},
		EvalArg: c.transfer.EvalExprValue,
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
	targets := c.typer.resolveCallTargets(call, c.program, in.FunctionRefs, in.ClosureRefs)
	selected := canonicalcall.SelectTargets(targets).Targets()
	out := make([]summary.CallEntryTarget, 0, len(selected))
	for _, target := range selected {
		ref := target.Ref()
		cells := c.program.CallEntryCells(ref, in.Cells)
		refs := c.program.CallEntryFunctionRefs(ref, in.FunctionRefs)
		closures := c.program.CallEntryClosureRefs(ref, in.ClosureRefs)
		entryFacts := summary.DirectCallEntryFacts(summary.DirectCallEntryFactInput{
			Call:      call,
			Callee:    ref,
			ParamSlot: c.paramSlot,
			ArgPath: func(_ int, arg ast.Expr) (constraint.Path, bool) {
				return c.typer.exprPath(arg)
			},
			KeyPresence: in.KeyPresence,
			Num:         in.Num,
			IndexWrites: in.IndexWrites,
		})
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
	return c.program.callbackArgRefs(c.graph, arg, rawSym, in)
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

func (ct callTyper) callEntrySlotType(ref summary.FuncRef, call *ast.FuncCallExpr, runtimeValues []product.AbstractValue, entryValues summary.EntryValues, slot int) typ.Type {
	projector, ok := ct.callEntryProjector()
	if !ok {
		return nil
	}
	return projector.slotType(ref, call, runtimeValues, entryValues, slot)
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

func (c callEntryProjector) productContext(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (canonicalcall.EntryContext, bool) {
	entryValues := c.productValuesForRef(ref, call, ctx.RuntimeArgValues)
	entryFacts := c.factsForRef(ref, call, ctx)
	entryRefs := flow.FunctionRefsDomain.Join(
		c.program.CallEntryFunctionRefs(ref, ctx.FunctionRefs),
		c.functionRefsForRef(ref, call, ctx),
	)
	entryClosures := flow.ClosureRefsDomain.Join(
		c.program.CallEntryClosureRefs(ref, ctx.ClosureRefs),
		c.closureRefsForRef(ref, call, ctx),
	)
	return canonicalcall.NewEntryContextWithFacts(
		ref,
		c.program.CallEntryCells(ref, ctx.Cells),
		entryRefs,
		entryClosures,
		entryValues,
		entryFacts,
	), true
}

func (c callEntryProjector) productClosureContext(ref summary.FuncRef, closure flow.ClosureRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (canonicalcall.EntryContext, bool) {
	entryValues := c.productValuesForRef(ref, call, ctx.RuntimeArgValues)
	entryFacts := c.factsForRef(ref, call, ctx)
	entryRefs := flow.FunctionRefsDomain.Join(
		c.program.CallEntryFunctionRefs(ref, ctx.FunctionRefs),
		c.functionRefsForRef(ref, call, ctx),
	)
	entryClosures := flow.ClosureRefsDomain.Join(
		c.program.CallEntryClosureRefs(ref, ctx.ClosureRefs),
		c.closureRefsForRef(ref, call, ctx),
	)
	return canonicalcall.EntryContextFromClosureWithLiveAxesAndFacts(
		ref,
		closure,
		c.program.CallEntryCells(ref, ctx.Cells),
		entryRefs,
		entryClosures,
		entryValues,
		entryFacts,
	), true
}

func (c callEntryProjector) factsForRef(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.BoundaryFacts {
	return summary.DirectCallEntryFacts(summary.DirectCallEntryFactInput{
		Call:        call,
		Callee:      ref,
		ParamSlot:   c.paramSlot,
		ArgPath:     func(_ int, arg ast.Expr) (constraint.Path, bool) { return c.typer.exprPath(arg) },
		KeyPresence: ctx.KeyPresence,
		Num:         ctx.Num,
		IndexWrites: ctx.IndexWrites,
	})
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
	return summary.DirectCallEntryFunctionRefs(summary.DirectCallEntryReferenceInput{
		Call:                call,
		Callee:              ref,
		FunctionRefs:        ctx.FunctionRefs,
		ReferenceProjection: c.program.referenceProjection(ref),
		LimitReferencePaths: true,
		ParamSlot:           c.paramSlot,
		ParamPath: func(callee summary.FuncRef, slot int) (constraint.Path, bool) {
			return c.program.paramPath(callee, slot)
		},
		ArgPath: func(_ int, arg ast.Expr) (constraint.Path, bool) {
			return c.typer.exprPath(arg)
		},
		ResolveFunctionArg: func(_ int, arg ast.Expr, _ *flow.PointState) (flow.FunctionRefSet, bool) {
			return c.typer.callEntryFunctionArgRefs(arg, ctx.FunctionRefs)
		},
		ResolveFunctionArgRefs: func(_ int, arg ast.Expr, _ *flow.PointState) (flow.FunctionRefs, bool) {
			return c.typer.callEntryFunctionArgTreeRefs(arg, ctx)
		},
	})
}

func (c callEntryProjector) closureRefsForRef(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.ClosureRefs {
	if call == nil {
		return flow.ClosureRefsDomain.Bottom()
	}
	return summary.DirectCallEntryClosureRefs(summary.DirectCallEntryReferenceInput{
		Call:                call,
		Callee:              ref,
		ClosureRefs:         ctx.ClosureRefs,
		ReferenceProjection: c.program.referenceProjection(ref),
		LimitReferencePaths: true,
		ParamSlot:           c.paramSlot,
		ParamPath: func(callee summary.FuncRef, slot int) (constraint.Path, bool) {
			return c.program.paramPath(callee, slot)
		},
		ArgPath: func(_ int, arg ast.Expr) (constraint.Path, bool) {
			return c.typer.exprPath(arg)
		},
		ResolveClosureArg: func(_ int, arg ast.Expr, _ *flow.PointState) (flow.ClosureRefSet, bool) {
			return c.typer.callEntryClosureArgRefs(arg, ctx)
		},
		ResolveClosureArgRefs: func(_ int, arg ast.Expr, _ *flow.PointState) (flow.ClosureRefs, bool) {
			return c.typer.callEntryClosureArgTreeRefs(arg, ctx)
		},
	})
}
