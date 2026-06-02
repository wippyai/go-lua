package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func (ct callTyper) callOutcomeForTypedCall(
	call *ast.FuncCallExpr,
	exprType func(ast.Expr) typ.Type,
	cells flow.CaptureCells,
	refs flow.FunctionRefs,
) canonicalcall.CallOutcome {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return canonicalcall.CallOutcome{}
	}
	targets := ct.resolveCallTargets(call, d.activeProgram, refs, nil)
	return canonicalcall.CallOutcomeForTargets(
		targets,
		func(target canonicalcall.SelectedTarget) canonicalcall.EntryContext {
			ref := target.Ref()
			entryValues := ct.callEntryValuesForRef(ref, call, exprType)
			entryCells := flow.CaptureCellsDomain.Bottom()
			entryRefs := flow.FunctionRefsDomain.Bottom()
			if d.summaryReader().Live() {
				entryCells = d.activeProgram.CallEntryCells(ref, cells)
				entryRefs = d.activeProgram.CallEntryFunctionRefs(ref, refs)
			}
			return canonicalcall.NewEntryContext(ref, entryCells, entryRefs, flow.ClosureRefsDomain.Bottom(), entryValues)
		},
		func(ctx canonicalcall.EntryContext) summary.Summary {
			return ct.summaryForCallEntryContext(ctx)
		},
		canonicalcall.SummaryTargetInfo{
			DeclaredReturns: func(target canonicalcall.SelectedTarget) bool {
				return len(d.activeProgram.declaredReturns[target.Ref()]) > 0
			},
			SignatureReturns: func(target canonicalcall.SelectedTarget) []typ.Type {
				return ct.selectedTargetSignatureReturns(
					d.activeProgram,
					target,
					call,
					argTypesFromCall(call, exprType),
					exprType,
					cells,
					refs,
					nil,
				)
			},
			SignatureRelations: func(target canonicalcall.SelectedTarget) flow.ReturnRelations {
				return flow.ReturnRelationsFromFunctionType(d.signatureForRef(d.activeProgram, target.Ref()))
			},
		},
	)
}

func (ct callTyper) callOutcomeForProductCall(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) canonicalcall.CallOutcome {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return canonicalcall.CallOutcome{}
	}
	targets := ct.resolveCallTargets(call, d.activeProgram, ctx.FunctionRefs, ctx.ClosureRefs)
	return canonicalcall.CallOutcomeForTargets(
		targets,
		func(target canonicalcall.SelectedTarget) canonicalcall.EntryContext {
			ref := target.Ref()
			if closure, ok := target.Closure(); ok {
				entry, _ := ct.productClosureCallEntryContext(ref, closure, call, ctx)
				return entry
			}
			entry, _ := ct.productCallEntryContext(ref, call, ctx)
			return entry
		},
		func(ctx canonicalcall.EntryContext) summary.Summary {
			return ct.summaryForCallEntryContext(ctx)
		},
		canonicalcall.SummaryTargetInfo{
			DeclaredReturns: func(target canonicalcall.SelectedTarget) bool {
				return len(d.activeProgram.declaredReturns[target.Ref()]) > 0
			},
			SignatureReturns: func(target canonicalcall.SelectedTarget) []typ.Type {
				return ct.selectedTargetSignatureReturns(
					d.activeProgram,
					target,
					call,
					ctx.ArgTypes(),
					ctx.ExprType,
					ctx.Cells,
					ctx.FunctionRefs,
					ctx.SelfType,
				)
			},
			SignatureRelations: func(target canonicalcall.SelectedTarget) flow.ReturnRelations {
				if target.IsClosure() {
					return flow.ReturnRelations{}
				}
				return flow.ReturnRelationsFromFunctionType(d.signatureForRef(d.activeProgram, target.Ref()))
			},
		},
	)
}

func (ct callTyper) selectedTargetSignatureReturns(
	prog *program,
	target canonicalcall.SelectedTarget,
	call *ast.FuncCallExpr,
	argTypes []typ.Type,
	exprType func(ast.Expr) typ.Type,
	cells flow.CaptureCells,
	refs flow.FunctionRefs,
	methodReceiverType typ.Type,
) []typ.Type {
	d := ct.d
	if d == nil || prog == nil || call == nil {
		return nil
	}
	sig := d.signatureForRef(prog, target.Ref())
	if sig == nil || typ.IsAbsentOrUnknown(sig) {
		return nil
	}
	forcedExprType := func(expr ast.Expr) typ.Type {
		if expr == call.Func {
			return sig
		}
		if exprType == nil {
			return nil
		}
		return exprType(expr)
	}
	in := ct.callReturnInput(call, argTypes, forcedExprType, cells, refs, methodReceiverType)
	in.SummaryReturns = nil
	in.Resolver = ct.callTypeResolver(forcedExprType)
	if returns, ok := canonicalcall.InferReturnTypes(in); ok && len(returns) > 0 {
		return returns
	}
	if fn := unwrap.Function(sig); fn != nil && len(fn.Returns) > 0 {
		return append([]typ.Type(nil), fn.Returns...)
	}
	return nil
}

func argTypesFromCall(call *ast.FuncCallExpr, exprType func(ast.Expr) typ.Type) []typ.Type {
	if call == nil || exprType == nil {
		return nil
	}
	out := make([]typ.Type, len(call.Args))
	for i, arg := range call.Args {
		out[i] = exprType(arg)
	}
	return out
}
