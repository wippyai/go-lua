package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// callReturnProjection carries the live call-boundary evidence used by
// type-level return inference. It normalizes higher-order callback argument
// signatures before the generic call pipeline sees the call.
type callReturnProjection struct {
	typer              callTyper
	call               *ast.FuncCallExpr
	argTypes           []typ.Type
	exprType           func(ast.Expr) typ.Type
	cells              flow.CaptureCells
	refs               flow.FunctionRefs
	closures           flow.ClosureRefs
	methodReceiverType typ.Type
}

func (ct callTyper) callReturnProjection(
	call *ast.FuncCallExpr,
	argTypes []typ.Type,
	exprType func(ast.Expr) typ.Type,
	cells flow.CaptureCells,
	refs flow.FunctionRefs,
	closures flow.ClosureRefs,
	methodReceiverType typ.Type,
) (callReturnProjection, bool) {
	d := ct.d
	if call == nil || d == nil || d.activeProgram == nil {
		return callReturnProjection{}, false
	}
	return callReturnProjection{
		typer:              ct,
		call:               call,
		argTypes:           argTypes,
		exprType:           exprType,
		cells:              cells,
		refs:               refs,
		closures:           closures,
		methodReceiverType: methodReceiverType,
	}, true
}

func (p callReturnProjection) projection() canonicalcall.ReturnInput {
	d := p.typer.d
	return canonicalcall.ReturnInput{
		Call:               p.call,
		ArgTypes:           p.refinedArgTypes(),
		Env:                p.typer.callInterceptEnv(p.exprType),
		Ctx:                d.activeCtx,
		Query:              d.cfg.Types,
		MethodReceiverType: p.methodReceiverType,
		SummaryReturns: func(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) []typ.Type {
			return p.typer.callOutcomeForTypedCall(call, exprType, p.cells, p.refs).InferredReturnTypes()
		},
		Resolver: p.typer.callTypeResolver(p.exprType),
		ResolveTypeArg: func(expr ast.TypeExpr) typ.Type {
			return d.resolveType(expr, d.baseScope())
		},
	}
}

func (p callReturnProjection) types() ([]typ.Type, bool) {
	return p.projection().Types()
}

func (p callReturnProjection) refinedArgTypes() []typ.Type {
	if p.call == nil || len(p.call.Args) == 0 {
		return p.argTypes
	}
	callbackRefs := p.callbackRefs()
	if len(callbackRefs) == 0 {
		return p.argTypes
	}
	projector := newCallableProjector(p.typer.d, p.typer.d.activeProgram, p.typer.d.activeQueries, p.typer.d.activeCtx)
	expectedInput := p.typer.expectedArgProjection(p.call, p.argTypes, p.exprType, p.methodReceiverType)
	expectedInput.CallbackArg = func(arg ast.Expr) bool {
		_, ok := callbackRefs[arg]
		return ok
	}
	expectedArgs := expectedInput.ExpectedTypes()
	return (canonicalcall.CallbackArgRefinementProjection{
		Call:         p.call,
		ArgTypes:     p.argTypes,
		ExpectedArgs: expectedArgs,
		CallbackRefs: func(arg ast.Expr) ([]summary.FuncRef, bool) {
			argRefs, ok := callbackRefs[arg]
			return argRefs, ok
		},
		FunctionType: func(ref summary.FuncRef) typ.Type {
			return projector.FunctionTypeByRef(canonref.ToFlow(ref), p.cells, p.refs, p.closures)
		},
		ContextualFunction: func(ref summary.FuncRef, values summary.EntryValues) typ.Type {
			return p.contextualFunction(projector, ref, values)
		},
	}).RefinedTypes()
}

func (p callReturnProjection) callbackRefs() map[ast.Expr][]summary.FuncRef {
	d := p.typer.d
	if d == nil || d.activeProgram == nil {
		return nil
	}
	resolver := p.typer.targetResolver(d.activeProgram)
	out := make(map[ast.Expr][]summary.FuncRef)
	for _, arg := range p.call.Args {
		argRefs, ok := resolver.ResolveCallbackArgRefs(arg, p.refs, d.activeProgram.refByFunc)
		if !ok || len(argRefs) == 0 {
			continue
		}
		out[arg] = argRefs
	}
	return out
}

func (p callReturnProjection) contextualFunction(projector callableProjector, ref summary.FuncRef, values summary.EntryValues) typ.Type {
	d := p.typer.d
	if d == nil || d.activeProgram == nil || len(values) == 0 {
		return nil
	}
	sig := d.signatureForRef(d.activeProgram, ref)
	if sig == nil {
		return nil
	}
	entry := d.activeProgram.CallEntryContext(ref, flow.ReferenceContextOf(p.cells, p.refs, p.closures), values)
	sum := projector.reader.SummarizeWithKey(entry.Key())
	return summary.FunctionSignatureWithEntryParamsAndProjectedReturns(sig, d.refHasDeclaredReturns(d.activeProgram, ref), sum, values)
}
