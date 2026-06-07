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
	"github.com/wippyai/go-lua/types/flow/numeric"
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

func (a callEntryAccess) pointReferenceArgSources() summary.EntryReferenceArgSources {
	return summary.EntryReferenceArgSources{
		FunctionRefs: func(_ int, arg ast.Expr, in *flow.PointState) (flow.FunctionRefSet, bool) {
			return a.projector.pointArgProjection(in).functionArgRefs(arg)
		},
		RefTrees: func(_ int, arg ast.Expr, in *flow.PointState) (flow.ReferenceContext, bool) {
			return a.projector.pointArgProjection(in).argRefTrees(arg)
		},
		ClosureRefs: func(_ int, arg ast.Expr, in *flow.PointState) (flow.ClosureRefSet, bool) {
			return a.projector.pointArgProjection(in).closureArgRefs(arg)
		},
	}
}

func (a callEntryAccess) productReferenceArgSources(ctx transfer.ProductCallContext) summary.EntryReferenceArgSources {
	projection := a.projector.productArgProjection(ctx)
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

func (a callEntryAccess) pointFacts(ref summary.FuncRef, call *ast.FuncCallExpr, in *flow.PointState) flow.BoundaryFacts {
	if in == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	return a.facts(ref, call, in.KeyPresence, in.Num, in.IndexWrites)
}

func (a callEntryAccess) productFacts(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.BoundaryFacts {
	return a.facts(ref, call, ctx.KeyPresence, ctx.Num, ctx.IndexWrites)
}

func (a callEntryAccess) facts(
	ref summary.FuncRef,
	call *ast.FuncCallExpr,
	keyPresence flow.KeyPresenceFacts,
	num *numeric.State,
	indexWrites flow.IndexWriteAdmissionFacts,
) flow.BoundaryFacts {
	return summary.CallEntryContextProjection{
		ParamSlot: a.projector.paramSlot,
		ArgPath:   a.argPath,
	}.DirectFacts(ref, call, keyPresence, num, indexWrites)
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

func (p *program) ProjectCallEntryValues(ref summary.FuncRef, fs state.FunctionState) summary.CallEntryValues {
	projector, ok := p.callEntryProjector(ref)
	if !ok {
		return nil
	}
	return projector.valueProjection(fs).Project()
}

func (p *program) ProjectCallEntryContextKeys(ref summary.FuncRef, fs state.FunctionState) []summary.Key {
	projector, ok := p.callEntryProjector(ref)
	if !ok {
		return nil
	}
	return projector.contextProjection(fs).ProjectKeys()
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
		ArgPath:             access.argPath,
		ReferenceArgSources: access.pointReferenceArgSources(),
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
	access := c.access()
	targets := c.typer.resolveCallTargets(call, c.program, flow.ReferenceContextFromPoint(in))
	selected := targets.Select().Targets()
	out := make([]summary.CallEntryTarget, 0, len(selected))
	references := flow.ReferenceContextFromPoint(in)
	for _, target := range selected {
		ref := target.Ref()
		entryFacts := access.pointFacts(ref, call, in)
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
	return c.entryValueLayout().DirectProductValues(ref, call, runtimeValues)
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
	return c.productEntryContext(ref, call, ctx), true
}

func (c callEntryProjector) productClosureContext(ref summary.FuncRef, closure flow.ClosureRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (canonicalcall.EntryContext, bool) {
	return canonicalcall.EntryContextFromClosureWithLiveContext(closure, c.productEntryContext(ref, call, ctx)), true
}

func (c callEntryProjector) productEntryContext(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) canonicalcall.EntryContext {
	access := c.access()
	directReferences := c.productReferencesForRef(ref, call, ctx)
	references := ctx.References.Join(directReferences)
	return c.program.CallEntryContextWithFacts(
		ref,
		references,
		c.productValuesForRef(ref, call, ctx.RuntimeArgValues),
		access.productFacts(ref, call, ctx),
	)
}

func (c callEntryProjector) productValuesForRef(ref summary.FuncRef, call *ast.FuncCallExpr, runtimeValues []product.AbstractValue) summary.EntryValues {
	if call == nil {
		return nil
	}
	values := c.entryValueLayout().DirectProductValues(ref, call, runtimeValues)
	return c.program.withPrototypeMethodSurfacesForMethodCall(ref, call, values)
}

func (c callEntryProjector) entryValueLayout() summary.CallEntryContextProjection {
	return summary.CallEntryContextProjection{
		ParamSlot: c.paramSlot,
		ParamSlotCount: func(callee summary.FuncRef, _ *ast.FuncCallExpr) int {
			return c.program.paramSlotCount(callee)
		},
	}
}

func (c callEntryProjector) productReferencesForRef(ref summary.FuncRef, call *ast.FuncCallExpr, ctx transfer.ProductCallContext) flow.ReferenceContext {
	if call == nil {
		return flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom())
	}
	access := c.access()
	return c.referenceLayout().DirectReferences(ref, call, nil, ctx.References, access.productReferenceArgSources(ctx))
}

func (c callEntryProjector) referenceLayout() summary.CallEntryContextProjection {
	return summary.CallEntryContextProjection{
		ParamSlot: c.paramSlot,
		ParamPath: func(callee summary.FuncRef, slot int) (constraint.Path, bool) {
			return c.program.paramPath(callee, slot)
		},
		ArgPath: c.access().argPath,
		ReferencePaths: func(callee summary.FuncRef) flow.ReferencePathProjection {
			return c.program.referenceProjection(callee)
		},
	}
}
