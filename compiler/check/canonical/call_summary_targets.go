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
	return callOutcomeProjection{
		typer:    ct,
		program:  d.activeProgram,
		call:     call,
		targets:  ct.resolveCallTargets(call, d.activeProgram, refs, nil),
		argTypes: argTypesFromCall(call, exprType),
		exprType: exprType,
		cells:    cells,
		refs:     refs,
		entryContext: func(target canonicalcall.SelectedTarget) canonicalcall.EntryContext {
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
	}.outcome()
}

type productCallOutcomeOptions struct {
	skipSignatureReturns   bool
	skipSignatureRelations bool
}

func (ct callTyper) callOutcomeForProductCall(call *ast.FuncCallExpr, ctx transfer.ProductCallContext) canonicalcall.CallOutcome {
	return ct.callOutcomeForProductCallWithOptions(call, ctx, productCallOutcomeOptions{})
}

func (ct callTyper) callOutcomeForProductCallWithOptions(call *ast.FuncCallExpr, ctx transfer.ProductCallContext, opts productCallOutcomeOptions) canonicalcall.CallOutcome {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return canonicalcall.CallOutcome{}
	}
	return ct.productCallOutcomeProjection(call, ctx, opts, nil).outcome()
}

func (ct callTyper) productCallOutcomeProjection(
	call *ast.FuncCallExpr,
	ctx transfer.ProductCallContext,
	opts productCallOutcomeOptions,
	lookup canonicalcall.SummaryLookup,
) callOutcomeProjection {
	d := ct.d
	if d == nil || call == nil || d.activeProgram == nil {
		return callOutcomeProjection{}
	}
	return callOutcomeProjection{
		typer:                    ct,
		program:                  d.activeProgram,
		call:                     call,
		targets:                  ct.resolveCallTargets(call, d.activeProgram, ctx.FunctionRefs, ctx.ClosureRefs),
		argTypes:                 ctx.ArgTypes(),
		exprType:                 ctx.ExprType,
		cells:                    ctx.Cells,
		refs:                     ctx.FunctionRefs,
		closures:                 ctx.ClosureRefs,
		methodReceiverType:       ctx.SelfType,
		skipSignatureReturns:     opts.skipSignatureReturns,
		skipSignatureRelations:   opts.skipSignatureRelations,
		omitClosureRelationProof: true,
		entryContext: func(target canonicalcall.SelectedTarget) canonicalcall.EntryContext {
			ref := target.Ref()
			if closure, ok := target.Closure(); ok {
				entry, _ := ct.productClosureCallEntryContext(ref, closure, call, ctx)
				return entry
			}
			entry, _ := ct.productCallEntryContext(ref, call, ctx)
			return entry
		},
		summaryLookup: lookup,
	}
}

// callOutcomeProjection centralizes the selected-target summary policy for both
// typed and product call contexts. The caller supplies only the target axes and
// the entry-context constructor that is specific to its evidence carrier.
type callOutcomeProjection struct {
	typer                    callTyper
	program                  *program
	call                     *ast.FuncCallExpr
	targets                  canonicalcall.TargetSet
	argTypes                 []typ.Type
	exprType                 func(ast.Expr) typ.Type
	cells                    flow.CaptureCells
	refs                     flow.FunctionRefs
	closures                 flow.ClosureRefs
	methodReceiverType       typ.Type
	entryContext             canonicalcall.SelectedEntryContext
	summaryLookup            canonicalcall.SummaryLookup
	skipSignatureReturns     bool
	skipSignatureRelations   bool
	omitClosureRelationProof bool
}

func (p callOutcomeProjection) outcome() canonicalcall.CallOutcome {
	return canonicalcall.CallOutcomeForTargets(
		p.targets,
		p.entryContext,
		func(ctx canonicalcall.EntryContext) summary.Summary {
			if p.summaryLookup != nil {
				return p.summaryLookup(ctx)
			}
			return p.typer.summaryForCallEntryContext(ctx)
		},
		canonicalcall.SummaryTargetInfo{
			DeclaredReturns: func(target canonicalcall.SelectedTarget) bool {
				return p.program.refHasClosedDeclaredReturns(target.Ref())
			},
			SignatureReturns: func(target canonicalcall.SelectedTarget) []typ.Type {
				return p.typer.selectedTargetSignatureReturns(
					p.program,
					target,
					p.call,
					p.argTypes,
					p.exprType,
					p.cells,
					p.refs,
					p.closures,
					p.methodReceiverType,
				)
			},
			SignatureRelations: func(target canonicalcall.SelectedTarget) flow.ReturnRelations {
				if p.omitClosureRelationProof && target.IsClosure() {
					return flow.ReturnRelations{}
				}
				return flow.ReturnRelationsFromFunctionType(p.typer.d.signatureForRef(p.program, target.Ref()))
			},
			SkipSignatureReturns:   p.skipSignatureReturns,
			SkipSignatureRelations: p.skipSignatureRelations,
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
	closures flow.ClosureRefs,
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
	argTypes = callArgTypesWithExprFallback(call, argTypes, exprType)
	forcedExprType := func(expr ast.Expr) typ.Type {
		if expr == call.Func {
			return sig
		}
		if exprType == nil {
			return nil
		}
		return exprType(expr)
	}
	proj, ok := ct.callReturnProjection(call, argTypes, forcedExprType, cells, refs, closures, methodReceiverType)
	if !ok {
		return nil
	}
	in := proj.input()
	in.SummaryReturns = nil
	if returns, ok := canonicalcall.InferReturnTypes(in); ok && len(returns) > 0 {
		return returns
	}
	if fn := unwrap.Function(sig); fn != nil && len(fn.Returns) > 0 {
		return append([]typ.Type(nil), fn.Returns...)
	}
	return nil
}

func callArgTypesWithExprFallback(call *ast.FuncCallExpr, argTypes []typ.Type, exprType func(ast.Expr) typ.Type) []typ.Type {
	if call == nil {
		return nil
	}
	out := make([]typ.Type, len(call.Args))
	for i, arg := range call.Args {
		if i < len(argTypes) && !typ.IsAbsentOrUnknown(argTypes[i]) {
			out[i] = argTypes[i]
			continue
		}
		if exprType != nil {
			if t := exprType(arg); !typ.IsAbsentOrUnknown(t) {
				out[i] = t
				continue
			}
		}
		out[i] = typ.Unknown
	}
	return out
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
