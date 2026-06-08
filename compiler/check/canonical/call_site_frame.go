package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// callSiteFrame is the normalized caller-side evidence for one call boundary.
// It owns the type-only projections that still need to exist at the boundary;
// selected summary/product facts are layered on top by callBoundaryFrame.
type callSiteFrame struct {
	typer              callTyper
	call               *ast.FuncCallExpr
	argTypes           []typ.Type
	exprType           func(ast.Expr) typ.Type
	references         flow.ReferenceContext
	methodReceiverType typ.Type
	nestedCall         func(*ast.FuncCallExpr) transfer.ProductCallContext
}

func (ct callTyper) productCallSiteFrame(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) (callSiteFrame, bool) {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return callSiteFrame{}, false
	}
	return callSiteFrame{
		typer:              ct,
		call:               call,
		argTypes:           ctx.ArgTypes(),
		exprType:           ctx.ExprType,
		references:         ctx.References,
		methodReceiverType: ctx.SelfType,
		nestedCall:         ctx.NestedCall,
	}, true
}

func (ct callTyper) typedCallSiteFrame(
	call *ast.FuncCallExpr,
	argTypes []typ.Type,
	exprType func(ast.Expr) typ.Type,
	references flow.ReferenceContext,
	methodReceiverType typ.Type,
) (callSiteFrame, bool) {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return callSiteFrame{}, false
	}
	return callSiteFrame{
		typer:              ct,
		call:               call,
		argTypes:           argTypes,
		exprType:           exprType,
		references:         references,
		methodReceiverType: methodReceiverType,
	}, true
}

func (f callSiteFrame) expectedArgProjection() canonicalcall.ExpectedArgProjection {
	d := f.typer.d
	in := canonicalcall.ExpectedArgProjection{
		Call:               f.call,
		ArgTypes:           f.argTypes,
		ExprType:           f.exprType,
		Resolver:           f.typer.callTypeResolver(f.exprType),
		MethodReceiverType: f.methodReceiverType,
	}
	if d == nil {
		return in
	}
	in.Ctx = d.activeCtx
	in.Query = d.cfg.Types
	in.ResolveTypeArg = func(expr ast.TypeExpr) typ.Type {
		return d.resolveType(expr, d.baseScope())
	}
	return in
}

func (f callSiteFrame) functionShape() *typ.Function {
	return (canonicalcall.DemandFunctionProjection{
		Call:            f.call,
		SummaryFunction: f.summaryFunction,
		Resolver:        f.typer.callTypeResolver(f.exprType),
	}).Function()
}

func (f callSiteFrame) summaryFunction(call *ast.FuncCallExpr) *typ.Function {
	d := f.typer.d
	if d == nil || d.activeProgram == nil || d.activeQueries == nil || d.activeCtx == nil {
		return nil
	}
	ref, ok := f.typer.resolveCalleeRef(call, d.activeProgram)
	if !ok {
		return nil
	}
	return d.signatureForRef(d.activeProgram, ref)
}

func (f callSiteFrame) returnInput(summaryReturns func(*ast.FuncCallExpr, func(ast.Expr) typ.Type) []typ.Type) canonicalcall.ReturnInput {
	d := f.typer.d
	return canonicalcall.ReturnInput{
		Call:               f.call,
		ArgTypes:           f.refinedArgTypes(),
		Env:                f.typer.callInterceptEnv(f.exprType),
		Ctx:                d.activeCtx,
		Query:              d.cfg.Types,
		MethodReceiverType: f.methodReceiverType,
		SummaryReturns:     summaryReturns,
		Resolver:           f.typer.callTypeResolver(f.exprType),
		ResolveTypeArg: func(expr ast.TypeExpr) typ.Type {
			return d.resolveType(expr, d.baseScope())
		},
	}
}

func (f callSiteFrame) returnTypes(summaryReturns func(*ast.FuncCallExpr, func(ast.Expr) typ.Type) []typ.Type) ([]typ.Type, bool) {
	return f.returnInput(summaryReturns).Types()
}

func (f callSiteFrame) refinedArgTypes() []typ.Type {
	if f.call == nil || len(f.call.Args) == 0 {
		return f.argTypes
	}
	callbackRefs := f.callbackRefs()
	if len(callbackRefs) == 0 {
		return f.argTypes
	}
	projector := newCallableProjector(f.typer.d, f.typer.d.activeProgram, f.typer.d.activeQueries, f.typer.d.activeCtx)
	expectedInput := f.expectedArgProjection()
	expectedInput.CallbackArg = func(arg ast.Expr) bool {
		_, ok := callbackRefs[arg]
		return ok
	}
	expectedArgs := expectedInput.ExpectedTypes()
	return (canonicalcall.CallbackArgRefinementProjection{
		Call:         f.call,
		ArgTypes:     f.argTypes,
		ExpectedArgs: expectedArgs,
		CallbackRefs: func(arg ast.Expr) ([]summary.FuncRef, bool) {
			argRefs, ok := callbackRefs[arg]
			return argRefs, ok
		},
		FunctionType: func(ref summary.FuncRef) typ.Type {
			return projector.FunctionTypeByRef(canonref.ToFlow(ref), f.references)
		},
		ContextualFunction: func(ref summary.FuncRef, values summary.EntryValues) typ.Type {
			return f.contextualFunction(projector, ref, values)
		},
	}).RefinedTypes()
}

func (f callSiteFrame) callbackRefs() map[ast.Expr][]summary.FuncRef {
	entryProjector, ok := f.typer.callEntryProjector()
	if !ok {
		return nil
	}
	return entryProjector.referenceArgProjection(f.references).callbackRefsForCall(f.call)
}

func (f callSiteFrame) contextualFunction(projector callableProjector, ref summary.FuncRef, values summary.EntryValues) typ.Type {
	d := f.typer.d
	if d == nil || d.activeProgram == nil || len(values) == 0 {
		return nil
	}
	sig := d.signatureForRef(d.activeProgram, ref)
	if sig == nil {
		return nil
	}
	entry := d.activeProgram.CallEntryContext(ref, f.references, values)
	sum := projector.reader.SummarizeWithKey(entry.Key())
	return summary.FunctionSignatureWithEntryParamsAndProjectedReturns(sig, d.refHasDeclaredReturns(d.activeProgram, ref), sum, values)
}

func (f callSiteFrame) expectedCalleeType(expr ast.Expr) typ.Type {
	if f.typer.d != nil && f.typer.d.activeProgram != nil {
		if ref, ok := f.typer.targetResolver(f.typer.d.activeProgram).ResolveStaticCall(f.call); ok {
			if sig := f.typer.d.signatureForRef(f.typer.d.activeProgram, ref); sig != nil {
				return sig
			}
		}
	}
	if nested, ok := expr.(*ast.FuncCallExpr); ok && nested != nil && f.nestedCall != nil {
		result := f.typer.ProductCallFromValues(nested, f.nestedCall(nested))
		if result.HasReturnValues && len(result.ReturnValues) > 0 {
			if t := product.ProjectValueOrUnknown(result.ReturnValues[0]); t != nil && !typ.IsAbsentOrUnknown(t) {
				return t
			}
		}
	}
	if fn := f.functionShape(); fn != nil {
		return fn
	}
	if expr != nil && f.exprType != nil {
		if t := f.exprType(expr); t != nil && !typ.IsAbsentOrUnknown(t) {
			return t
		}
	}
	return nil
}
