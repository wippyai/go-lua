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
// TODO(canonical-call-entry): fold the ProductCallContext entry builders in
// driver.go onto this same boundary so callTyper no longer hand-wires
// ParamSlot/ArgPath/ReferenceProjection for product calls.
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
