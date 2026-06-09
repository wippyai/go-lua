package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	canonicalcall "github.com/wippyai/go-lua/compiler/check/canonical/call"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/callboundary"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// callBoundaryFrame is the canonical semantic frame for one call boundary. It is
// built once from the call expression, solved point context, and selected
// summary outcome; callers ask the frame for outward projections instead of
// reconstructing call facts through driver helpers.
type callBoundaryFrame struct {
	typer   callTyper
	ctx     transfer.ProductCallContext
	site    callSiteFrame
	outcome canonicalcall.CallOutcome
}

func (ct callTyper) callBoundaryFrame(call *ast.FuncCallExpr, ctx transfer.ProductCallContext, opts productCallOutcomeOptions) (callBoundaryFrame, bool) {
	if ct.d == nil || call == nil || ct.d.activeProgram == nil {
		return callBoundaryFrame{}, false
	}
	site, ok := ct.productCallSiteFrame(call, ctx)
	if !ok {
		return callBoundaryFrame{}, false
	}
	outcome := ct.productCallOutcomeProjection(site, ctx, opts, nil).outcome().
		WithTypeFallbackOutcome(site.typeFallbackOutcome(ctx.ExprValue != nil))
	return callBoundaryFrame{
		typer:   ct,
		ctx:     ctx,
		site:    site,
		outcome: outcome,
	}, true
}

func (p callBoundaryFrame) callReturnValues() ([]product.AbstractValue, bool) {
	return p.outcome.ReturnValues(canonicalcall.ReturnValueInput{
		Call:                 p.site.call,
		TypePolicyAvailable:  p.typer.d.cfg.Types != nil,
		PendingInput:         p.ctx.PendingInput,
		BlockDynamicFallback: p.outcome.HasTargets() && !p.outcome.HasInformativeReturnValues(),
		ExprValue:            p.ctx.ExprValue,
	})
}

func (p callBoundaryFrame) result(evidence canonicalcall.BoundaryEvidence, effects callboundary.Effects) transfer.ProductCallResult {
	values, ok := p.callReturnValues()
	return transfer.ProductCallResult{
		ReturnValues:        values,
		HasReturnValues:     ok,
		ReturnStaticMembers: p.outcome.ReturnStaticMembers(),
		ReturnRefs:          evidence.ReturnRefs,
		ReturnRelations:     evidence.ReturnRelations,
		Effects:             effects,
		ArgDemands:          evidence.ArgDemands,
		NeverReturns:        evidence.NeverReturns,
		Postconditions:      evidence.Postconditions,
	}
}

func (p callBoundaryFrame) callArgDemands() []callobligation.Obligation {
	return (canonicalcall.CallArgDemandProjection{
		Call: p.site.call,
		SummaryDemands: func(*ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			return p.argDemands()
		},
		FunctionShape: func(*ast.FuncCallExpr) *typ.Function {
			return p.outcome.FunctionShape()
		},
		SelfType: func(*ast.FuncCallExpr) typ.Type {
			return p.ctx.SelfType
		},
	}).Demands()
}

func (p callBoundaryFrame) returnPostconditions() paramevidence.ReturnPostconditions {
	d := p.typer.d
	if d == nil || d.activeProgram == nil || p.site.call == nil {
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
	return p.outcome.Postconditions()
}

func (p callBoundaryFrame) argDemands() ([]callobligation.Obligation, bool) {
	targets := p.demandTargets()
	if len(targets) == 0 {
		return nil, false
	}
	return paramevidence.CallArgDemandProjection{
		Call:    p.site.call,
		Targets: targets,
	}.Obligations(), true
}

func (p callBoundaryFrame) demandTargets() []paramevidence.CallArgDemandTarget {
	d := p.typer.d
	if d == nil || d.activeProgram == nil || p.site.call == nil || len(p.site.call.Args) == 0 || !p.outcome.HasTargets() {
		return nil
	}
	prog := d.activeProgram
	projector, ok := p.typer.callEntryProjector()
	if !ok {
		return nil
	}
	currentRef, hasCurrentRef := p.typer.currentRef()
	targets := p.outcome.Targets()
	out := make([]paramevidence.CallArgDemandTarget, 0, len(targets))
	for _, target := range targets {
		ref := target.Ref
		if hasCurrentRef && ref == currentRef {
			continue
		}
		fn := prog.funcExpr(ref)
		out = append(out, paramevidence.CallArgDemandTarget{
			Graph:     prog.Graph(ref),
			Function:  fn,
			Contracts: p.callerVisibleContracts(prog, ref, target.Summary.Params),
			DeclaredSlotType: func(slot int) typ.Type {
				return prog.paramSlotDeclaredType(ref, slot)
			},
			EntrySlotType: func(slot int) typ.Type {
				return projector.slotType(ref, p.site.call, p.ctx.RuntimeArgValues, target.EntryValues, slot)
			},
			SourceParamAnnotated: func(sourceParam int) bool {
				return paramevidence.SourceParamAnnotated(fn, sourceParam)
			},
		})
	}
	return out
}

func (p callBoundaryFrame) callerVisibleContracts(prog *program, ref summary.FuncRef, exact paramevidence.Contracts) paramevidence.Contracts {
	if prog == nil || p.typer.d == nil {
		return exact
	}
	exact = prog.publicPredicateContracts(ref, exact)
	reader := p.typer.d.summaryReader()
	aggregate := prog.publicPredicateContracts(ref, reader.Summarize(ref).Params)
	if len(aggregate) == 0 {
		return exact
	}
	if len(exact) == 0 {
		return aggregate
	}
	// Exact entry summaries carry return/effect precision for this call context,
	// but they must not erase body obligations that the canonical aggregate
	// summary proved for callers of the callee.
	return paramevidence.ContractDomain.Join(aggregate, exact)
}

func (p callBoundaryFrame) expectedArgEvidence(info *cfg.CallInfo, forceMethodReceiver bool) (api.CallExpectedArgEvidence, bool) {
	if info == nil || info.Call == nil || len(info.Call.Args) == 0 {
		return api.CallExpectedArgEvidence{}, false
	}
	expectedArgs := p.site.expectedArgProjection()
	expectedArgs.ShallowFuncLiterals = true
	if !callsite.IsMethodCallInfo(info) {
		expectedArgs.Callee = p.site.expectedCalleeType(info.Callee)
	}
	expectedArgs.IsMethod = callsite.IsMethodCallInfo(info)
	expectedArgs.MethodName = info.Method
	expectedArgs.ForceMethodReceiver = forceMethodReceiver
	expectedTypes := expectedArgs.ExpectedTypes()
	args := make([]typ.Type, len(info.Call.Args))
	any := false
	for argIdx := range info.Call.Args {
		if argIdx >= len(expectedTypes) {
			break
		}
		expected := expectedTypes[argIdx]
		if expected == nil || typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
			continue
		}
		args[argIdx] = expected
		any = true
	}
	if !any {
		return api.CallExpectedArgEvidence{}, false
	}
	return api.NewCallExpectedArgEvidence(args), true
}

func (p callBoundaryFrame) expectedArgType(info *cfg.CallInfo, forceMethodReceiver bool, argIdx int) typ.Type {
	if argIdx < 0 {
		return nil
	}
	evidence, ok := p.expectedArgEvidence(info, forceMethodReceiver)
	if !ok || argIdx >= len(evidence.Args) {
		return nil
	}
	return evidence.Args[argIdx]
}

func (p callBoundaryFrame) contractEvidence() (api.CallContractEvidence, bool) {
	demands := p.callArgDemands()
	if len(demands) == 0 {
		return api.CallContractEvidence{}, false
	}
	return api.NewCallContractEvidence(demands), true
}

func (p callBoundaryFrame) boundaryEvidenceAndEffects() (canonicalcall.BoundaryEvidence, callboundary.Effects) {
	cellEffects, ok := p.cellEffectAggregation()
	if !ok {
		return p.boundaryEvidence(summary.CellEffectAggregation{}), callboundary.EmptyEffects()
	}
	evidence := p.boundaryEvidence(cellEffects)
	return evidence, callboundary.EffectsOf(
		evidence.CellEffects,
		evidence.ReceiverEffects,
		evidence.BoundaryFacts,
		p.containerElementUnions(),
	)
}

func (p callBoundaryFrame) boundaryEvidence(cellEffects summary.CellEffectAggregation) canonicalcall.BoundaryEvidence {
	evidence := p.outcome.BoundaryEvidence(canonicalcall.BoundaryEvidenceInput{
		CellEffects:    cellEffects,
		ArgDemands:     p.callArgDemands(),
		Postconditions: p.returnPostconditions(),
		HasNoReturn: func(ref summary.FuncRef) bool {
			return p.typer.d.activeProgram.facts.HasNoReturn(ref)
		},
	})
	evidence.ReturnRelations = p.callerVisibleReturnRelations(evidence.ReturnRelations)
	return evidence
}

func (p callBoundaryFrame) callerVisibleReturnRelations(exact flow.ReturnRelations) flow.ReturnRelations {
	if !p.outcome.HasTargets() || p.typer.d == nil {
		return exact
	}
	reader := p.typer.d.summaryReader()
	aggregate := flow.ReturnRelationsDomain.Bottom()
	for _, target := range p.outcome.Targets() {
		rels := reader.Summarize(target.Ref).Relations
		if !rels.HasProof() {
			rels = flow.ReturnRelationsDomain.Top()
		}
		aggregate = flow.ReturnRelationsDomain.Join(aggregate, rels)
	}
	if !aggregate.HasProof() {
		return exact
	}
	// Exact entry summaries specialize a call context, but caller-visible
	// return relations proved by the aggregate summary remain valid body facts.
	return flow.MergeReturnRelationProofs(aggregate, exact)
}

func (p callBoundaryFrame) cellEffectAggregation() (summary.CellEffectAggregation, bool) {
	callEntry, ok := p.typer.callEntryProjector()
	if !ok {
		return summary.CellEffectAggregation{}, false
	}
	callbackRefs := callEntry.productArgProjection(p.ctx).callbackRefsForCall(p.site.call)
	return summary.CellEffectAggregation{
		CallbackSpec: p.callbackSpec(),
		CallbackArgs: p.site.call.Args,
		MethodCall:   p.site.call.Method != "",
		ResolveCallback: func(arg ast.Expr) ([]summary.FuncRef, bool) {
			refs, ok := callbackRefs[arg]
			return refs, ok
		},
		EffectOf: func(ref summary.FuncRef, entryValues summary.EntryValues) flow.CaptureEffects {
			evidence := callEntry.productEvidence(ref, p.site.call, p.ctx)
			return p.effectsForRef(ref, entryValues, evidence.Facts)
		},
	}, true
}

func (p callBoundaryFrame) callbackSpec() *contract.Spec {
	return p.outcome.CallbackSpec()
}

func (p callBoundaryFrame) containerElementUnions() []effect.ContainerElementUnion {
	return p.outcome.ContainerElementUnions()
}

func (p callBoundaryFrame) effectsForRef(
	ref summary.FuncRef,
	entryValues summary.EntryValues,
	entryFacts flow.BoundaryFacts,
) flow.CaptureEffects {
	reader := p.typer.d.summaryReader()
	entry := canonicalcall.NewEntryContext(
		ref,
		flow.ReferenceContextOf(flow.CaptureCellsDomain.Bottom(), flow.FunctionRefsDomain.Bottom(), flow.ClosureRefsDomain.Bottom()),
		entryValues,
		entryFacts,
	)
	if reader.Live() {
		entry = p.typer.d.activeProgram.CallEntryContextWithFacts(ref, p.site.references, entryValues, entryFacts)
	}
	return reader.SummarizeWithKey(entry.Key()).CellEffects
}
