package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// CallOutcome is the selected-target call result carrier. It is a pure value:
// target selection and summary projection are computed once, then every
// caller-visible call fact is projected from the same carrier.
type CallOutcome struct {
	Projection summary.CallSummaryProjection
	Selection  TargetSelection
}

// HasTargets reports whether the outcome has selected concrete callee summaries.
func (o CallOutcome) HasTargets() bool {
	return o.Selection.HasTargets()
}

// Targets returns the selected summary targets as a copy.
func (o CallOutcome) Targets() []summary.CallSummaryTarget {
	return append([]summary.CallSummaryTarget(nil), o.Projection.Targets...)
}

// InferredReturnValues returns the selected summary return tuple when summaries
// are authoritative for all selected targets.
func (o CallOutcome) InferredReturnValues() []product.AbstractValue {
	if !o.HasTargets() {
		return nil
	}
	return o.Projection.InferredReturnValues()
}

// HasInformativeReturnValues reports whether selected target summaries currently
// provide concrete caller-visible return evidence.
func (o CallOutcome) HasInformativeReturnValues() bool {
	return informativeReturnValues(o.InferredReturnValues())
}

// InferredReturnTypes projects selected summary returns to the type surface.
func (o CallOutcome) InferredReturnTypes() []typ.Type {
	if !o.HasTargets() {
		return nil
	}
	return o.Projection.InferredReturnTypes()
}

// ReturnRefs projects returned callable identities from selected summaries.
func (o CallOutcome) ReturnRefs() flow.ReturnRefs {
	return o.Projection.ReturnRefs()
}

// ReturnStaticMembers projects returned child path facts from selected summaries.
func (o CallOutcome) ReturnStaticMembers() []flow.StaticMemberFacts {
	if !o.HasTargets() {
		return nil
	}
	return o.Projection.ReturnStaticMembers()
}

// ReturnRelations projects return-slot relations through the canonical fallback
// policy owned by this package.
func (o CallOutcome) ReturnRelations(call *ast.FuncCallExpr, resolver TypeResolver, useResolvedSignature bool) flow.ReturnRelations {
	if len(o.Projection.Targets) > 0 {
		return o.Projection.ReturnRelations()
	}
	if o.Selection.BlocksTypeFallback() {
		return flow.ReturnRelationsDomain.Top()
	}
	if useResolvedSignature && call != nil {
		sig := resolver.ResolveCallee(call.Func)
		if rels := flow.ReturnRelationsFromFunctionType(sig); rels.HasProof() {
			return rels
		}
	}
	if call == nil {
		return flow.ReturnRelationsFromFunctionType(nil)
	}
	return flow.ReturnRelationsFromFunctionType(resolver.ResolveStaticCallee(call.Func))
}

// CellEffects projects caller-visible capture effects through the canonical
// direct-summary plus callback-fallback policy.
func (o CallOutcome) CellEffects(aggregation summary.CellEffectAggregation) flow.CaptureEffects {
	if !o.Selection.AllowsCallbackFallback() {
		return o.Projection.CellEffects()
	}
	aggregation.DirectEffects = o.Projection.CellEffects()
	return summary.AggregateCellEffects(aggregation)
}

// ReceiverEffects projects caller-visible receiver/container effects.
func (o CallOutcome) ReceiverEffects() flow.ReceiverEffects {
	return o.Projection.ReceiverEffects()
}

// BoundaryFacts projects caller-visible parameter/return-relative facts through
// the canonical summary-first, static-contract-fallback policy.
func (o CallOutcome) BoundaryFacts(call *ast.FuncCallExpr, resolver TypeResolver, useResolvedSignature bool) flow.BoundaryFacts {
	if len(o.Projection.Targets) > 0 {
		return o.Projection.BoundaryFacts()
	}
	if o.Selection.BlocksTypeFallback() {
		return flow.BoundaryFactsDomain.Top()
	}
	if useResolvedSignature && call != nil {
		facts := flow.BoundaryFactsFromFunctionType(resolver.ResolveCallee(call.Func))
		if facts.HasProof() {
			return facts
		}
	}
	if call == nil {
		return flow.BoundaryFactsFromFunctionType(nil)
	}
	return flow.BoundaryFactsFromFunctionType(resolver.ResolveStaticCallee(call.Func))
}

// Postconditions projects portable normal-return proofs from selected summaries.
func (o CallOutcome) Postconditions() paramevidence.ReturnPostconditions {
	return o.Projection.Postconditions()
}

// NeverReturns reports whether every selected target is proven no-return.
func (o CallOutcome) NeverReturns(hasNoReturn func(summary.FuncRef) bool) bool {
	return selectionNeverReturns(o.Selection, hasNoReturn)
}

// PostconditionProjection resolves portable normal-return postconditions without
// collapsing imported FunctionRefinement conditions into a local extraction view.
type PostconditionProjection struct {
	Call *ast.FuncCallExpr

	SummaryPostconditions func(*ast.FuncCallExpr) (paramevidence.ReturnPostconditions, bool)
	Resolver              TypeResolver
}

func (p PostconditionProjection) Postconditions() paramevidence.ReturnPostconditions {
	if p.Call == nil {
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
	if p.SummaryPostconditions != nil {
		if post, ok := p.SummaryPostconditions(p.Call); ok {
			return paramevidence.CloneReturnPostconditions(post)
		}
	}
	return paramevidence.ReturnPostconditionsFromFunctionType(p.Resolver.ResolveStaticCallee(p.Call.Func))
}

// CallbackSpecProjection is the canonical policy for finding a call's callback
// contract. Summary-known module signatures win; unresolved calls fall back to
// caller-visible callee/receiver type resolution.
type CallbackSpecProjection struct {
	Call *ast.FuncCallExpr

	SummarySignature func(*ast.FuncCallExpr) typ.Type
	Resolver         TypeResolver
}

// Spec extracts the callback contract used by call-site cell effect projection.
func (p CallbackSpecProjection) Spec() *contract.Spec {
	return specForCall(specInput{
		Call:             p.Call,
		SummarySignature: p.SummarySignature,
		Resolver:         p.Resolver,
	})
}

type specInput struct {
	Call *ast.FuncCallExpr

	SummarySignature func(*ast.FuncCallExpr) typ.Type
	Resolver         TypeResolver
}

func specForCall(in specInput) *contract.Spec {
	if in.Call == nil {
		return nil
	}
	callee := typ.Type(nil)
	if in.SummarySignature != nil {
		callee = in.SummarySignature(in.Call)
	}
	if callee == nil || typ.IsAbsentOrUnknown(callee) {
		if in.Call.Method != "" {
			receiver := in.Resolver.ResolveReceiver(in.Call.Receiver)
			if receiver != nil && !typ.IsAbsentOrUnknown(receiver) {
				if member, ok := core.Field(receiver, in.Call.Method); ok {
					callee = member
				}
			}
		} else {
			callee = in.Resolver.ResolveCallee(in.Call.Func)
		}
	}
	return contract.ExtractSpec(callee)
}

// ContainerElementUnionProjection is the canonical call-site policy for mutator
// specs that widen a container's element slot.
type ContainerElementUnionProjection struct {
	Call *ast.FuncCallExpr

	SummarySignature func(*ast.FuncCallExpr) typ.Type
	Resolver         TypeResolver
}

// Effects extracts ContainerElementUnion labels from a call's resolved spec. It
// returns labels only; transfer owns lowering parameter refs to runtime argument
// places/values and applying the product mutation.
func (p ContainerElementUnionProjection) Effects() []effect.ContainerElementUnion {
	spec := specForCall(specInput{
		Call:             p.Call,
		SummarySignature: p.SummarySignature,
		Resolver:         p.Resolver,
	})
	if spec == nil || len(spec.Effects.Labels) == 0 {
		return nil
	}
	var out []effect.ContainerElementUnion
	for _, label := range spec.Effects.Labels {
		mut, ok := label.(effect.Mutate)
		if !ok {
			continue
		}
		ceu, ok := mut.Transform.(effect.ContainerElementUnion)
		if !ok {
			continue
		}
		out = append(out, ceu)
	}
	return out
}

// StaticCallbackOverlayProjection is the canonical pre-solve policy for callback
// environment overlays on callees that are not handled as module-local refs.
type StaticCallbackOverlayProjection struct {
	Call *ast.FuncCallExpr

	Resolver TypeResolver
}

// Overlays resolves only external/static callback overlay
// contracts for fact extraction. Module-local callee overlays are supplied by
// summary/facts through a separate ref-based path, so this projection
// deliberately does not read inferred module signatures.
func (p StaticCallbackOverlayProjection) Overlays() callbackenv.Overlays {
	if p.Call == nil {
		return nil
	}
	if ident, ok := p.Call.Func.(*ast.IdentExpr); ok && ident != nil {
		if ov := callbackenv.OverlaysFromFunction(p.Resolver.ResolveGlobalIdentType(ident)); len(ov) > 0 {
			return ov
		}
	}
	return callbackenv.OverlaysFromFunction(p.Resolver.ResolveStaticFieldCallee(p.Call.Func))
}
