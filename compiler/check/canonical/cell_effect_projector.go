package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

type cellEffectProjector struct {
	typer     callTyper
	program   *program
	driver    *Driver
	callEntry callEntryProjector
}

// cellEffectProjector is the canonical/program-owned adapter for caller-visible
// capture-cell effects. The summary package owns callback ordering and effect
// algebra; this type supplies callback specs, callback target refs, and summary
// effect lookups without making driver.go wire that policy inline.
func (ct callTyper) cellEffectProjector() (cellEffectProjector, bool) {
	if ct.d == nil || ct.d.activeProgram == nil {
		return cellEffectProjector{}, false
	}
	callEntry, ok := ct.callEntryProjector()
	if !ok {
		return cellEffectProjector{}, false
	}
	return cellEffectProjector{
		typer:     ct,
		program:   ct.d.activeProgram,
		driver:    ct.d,
		callEntry: callEntry,
	}, true
}

func (p cellEffectProjector) typedCallEffects(
	outcome canonicalcall.CallOutcome,
	call *ast.FuncCallExpr,
	exprType func(ast.Expr) typ.Type,
	cells flow.CaptureCells,
	refs flow.FunctionRefs,
) flow.CaptureEffects {
	return outcome.CellEffects(summary.CellEffectAggregation{
		CallbackSpec: p.callbackSpecForCall(call, exprType),
		CallbackArgs: call.Args,
		MethodCall:   call.Method != "",
		ResolveCallback: func(arg ast.Expr) ([]summary.FuncRef, bool) {
			return p.typer.targetResolver(p.program).ResolveCallbackArgRefs(arg, refs, p.program.refByFunc)
		},
		EffectOf: func(ref summary.FuncRef, entryValues summary.EntryValues) flow.CaptureEffects {
			return p.effectsForRef(ref, cells, refs, flow.ClosureRefsDomain.Bottom(), entryValues, flow.BoundaryFactsDomain.Top())
		},
	})
}

func (p cellEffectProjector) productCallEffects(
	outcome canonicalcall.CallOutcome,
	call *ast.FuncCallExpr,
	ctx transfer.ProductCallContext,
) flow.CaptureEffects {
	return outcome.CellEffects(summary.CellEffectAggregation{
		CallbackSpec: p.callbackSpecForCall(call, ctx.ExprType),
		CallbackArgs: call.Args,
		MethodCall:   call.Method != "",
		ResolveCallback: func(arg ast.Expr) ([]summary.FuncRef, bool) {
			return p.typer.targetResolver(p.program).ResolveCallbackArgRefs(arg, ctx.FunctionRefs, p.program.refByFunc)
		},
		EffectOf: func(ref summary.FuncRef, entryValues summary.EntryValues) flow.CaptureEffects {
			entryFacts := p.callEntry.access().productFacts(ref, call, ctx)
			return p.effectsForRef(ref, ctx.Cells, ctx.FunctionRefs, ctx.ClosureRefs, entryValues, entryFacts)
		},
	})
}

func (p cellEffectProjector) effectsForRef(
	ref summary.FuncRef,
	cells flow.CaptureCells,
	refs flow.FunctionRefs,
	closures flow.ClosureRefs,
	entryValues summary.EntryValues,
	entryFacts flow.BoundaryFacts,
) flow.CaptureEffects {
	reader := p.driver.summaryReader()
	entry := canonicalcall.NewEntryContext(
		ref,
		flow.CaptureCellsDomain.Bottom(),
		flow.FunctionRefsDomain.Bottom(),
		flow.ClosureRefsDomain.Bottom(),
		entryValues,
		entryFacts,
	)
	if reader.Live() {
		entry = p.program.CallEntryContextWithFacts(ref, cells, refs, closures, entryValues, entryFacts)
	}
	return reader.SummarizeWithKey(entry.Key()).CellEffects
}

func (p cellEffectProjector) callbackSpecForCall(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) *contract.Spec {
	resolver := p.typer.callTypeResolver(exprType)
	return canonicalcall.CallbackSpecForCall(canonicalcall.CallbackSpecInput{
		Call: call,
		SummarySignature: func(call *ast.FuncCallExpr) typ.Type {
			if ref, ok := p.typer.resolveCalleeRef(call, p.program); ok {
				return p.driver.signatureForRef(p.program, ref)
			}
			return nil
		},
		Resolver: resolver,
	})
}
