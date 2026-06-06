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
		targets:  ct.resolveCallTargets(call, d.activeProgram, flow.ReferenceContextOf(cells, refs, flow.ClosureRefsDomain.Bottom())),
		argTypes: argTypesFromCall(call, exprType),
		exprType: exprType,
		references: flow.ReferenceContextOf(
			cells,
			refs,
			flow.ClosureRefsDomain.Bottom(),
		),
		entryContext: func(target canonicalcall.SelectedTarget) canonicalcall.EntryContext {
			ref := target.Ref()
			entryValues := ct.callEntryValuesForRef(ref, call, exprType)
			if d.summaryReader().Live() {
				return d.activeProgram.CallEntryContext(
					ref,
					flow.ReferenceContextOf(cells, refs, flow.ClosureRefsDomain.Bottom()),
					entryValues,
				)
			}
			return canonicalcall.NewEntryContext(
				ref,
				flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
				entryValues,
				flow.BoundaryFactsDomain.Top(),
			)
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
		targets:                  ct.resolveCallTargets(call, d.activeProgram, ctx.References),
		argTypes:                 ctx.ArgTypes(),
		exprType:                 ctx.ExprType,
		references:               ctx.References,
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
	references               flow.ReferenceContext
	methodReceiverType       typ.Type
	entryContext             canonicalcall.SelectedEntryContext
	summaryLookup            canonicalcall.SummaryLookup
	skipSignatureReturns     bool
	skipSignatureRelations   bool
	omitClosureRelationProof bool
}

func (p callOutcomeProjection) outcome() canonicalcall.CallOutcome {
	return p.targets.Outcome(
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
				return p.signatureReturns(target)
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

func (p callOutcomeProjection) signatureReturns(target canonicalcall.SelectedTarget) []typ.Type {
	d := p.typer.d
	if d == nil || p.program == nil || p.call == nil {
		return nil
	}
	sig := d.signatureForRef(p.program, target.Ref())
	if sig == nil || typ.IsAbsentOrUnknown(sig) {
		return nil
	}
	argTypes := callArgTypesWithExprFallback(p.call, p.argTypes, p.exprType)
	forcedExprType := func(expr ast.Expr) typ.Type {
		if expr == p.call.Func {
			return sig
		}
		if p.exprType == nil {
			return nil
		}
		return p.exprType(expr)
	}
	proj, ok := p.typer.callReturnProjection(
		p.call,
		argTypes,
		forcedExprType,
		p.references.CaptureCells(),
		p.references.FunctionRefs(),
		p.references.ClosureRefs(),
		p.methodReceiverType,
	)
	if !ok {
		return nil
	}
	in := proj.projection()
	in.SummaryReturns = nil
	if returns, ok := in.Types(); ok && len(returns) > 0 {
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
