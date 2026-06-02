package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	canonref "github.com/wippyai/go-lua/compiler/check/canonical/ref"
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

// InferredReturnTypes projects selected summary returns to the type surface.
func (o CallOutcome) InferredReturnTypes() []typ.Type {
	if !o.HasTargets() {
		return nil
	}
	return o.Projection.InferredReturnTypes()
}

// ReturnFunctionRefs projects returned function identities from selected
// summaries.
func (o CallOutcome) ReturnFunctionRefs() []flow.FunctionRefs {
	return o.Projection.ReturnFunctionRefs()
}

// ReturnClosureRefs projects returned closure identities from selected summaries.
func (o CallOutcome) ReturnClosureRefs() []flow.ClosureRefs {
	return o.Projection.ReturnClosureRefs()
}

// ReturnRelations projects return-slot relations through the canonical fallback
// policy owned by this package.
func (o CallOutcome) ReturnRelations(call *ast.FuncCallExpr, resolver TypeResolver, useResolvedSignature bool) flow.ReturnRelations {
	return InferReturnRelations(ReturnRelationsInput{
		Projection:           o.Projection,
		Selection:            o.Selection,
		Call:                 call,
		Resolver:             resolver,
		UseResolvedSignature: useResolvedSignature,
	})
}

// CellEffects projects caller-visible capture effects through the canonical
// direct-summary plus callback-fallback policy.
func (o CallOutcome) CellEffects(aggregation summary.CellEffectAggregation) flow.CaptureEffects {
	return InferCellEffects(CellEffectsInput{
		Projection:  o.Projection,
		Selection:   o.Selection,
		Aggregation: aggregation,
	})
}

// ReceiverEffects projects caller-visible receiver/container effects.
func (o CallOutcome) ReceiverEffects() flow.ReceiverEffects {
	return o.Projection.ReceiverEffects()
}

// NeverReturns reports whether every selected target is proven no-return.
func (o CallOutcome) NeverReturns(hasNoReturn func(summary.FuncRef) bool) bool {
	return SelectionNeverReturns(o.Selection, hasNoReturn)
}

// ReturnRelationsInput is the canonical call-site policy for caller-visible
// return-slot relations. Summary projection wins; closure-authoritative misses
// block type fallback; type facts are used only as fallbacks.
type ReturnRelationsInput struct {
	Projection summary.CallSummaryProjection
	Selection  TargetSelection

	Call                 *ast.FuncCallExpr
	Resolver             TypeResolver
	UseResolvedSignature bool
}

// InferReturnRelations resolves return-slot relations for a call without reading
// driver or program state. The driver supplies normalized summary projection and
// signature providers; this package owns the fallback order.
func InferReturnRelations(in ReturnRelationsInput) flow.ReturnRelations {
	if len(in.Projection.Targets) > 0 {
		return in.Projection.ReturnRelations()
	}
	if in.Selection.BlocksTypeFallback() {
		return flow.ReturnRelationsDomain.Top()
	}
	if in.UseResolvedSignature && in.Call != nil {
		sig := in.Resolver.ResolveCallee(in.Call.Func)
		if rels := flow.ReturnRelationsFromFunctionType(sig); len(rels.ErrorReturns()) > 0 {
			return rels
		}
	}
	if in.Call == nil {
		return flow.ReturnRelationsFromFunctionType(nil)
	}
	return flow.ReturnRelationsFromFunctionType(in.Resolver.ResolveStaticCallee(in.Call.Func))
}

// CellEffectsInput is the canonical call-site policy for caller-visible capture
// effects. Summary projection supplies direct callee effects. Callback fallback
// is composed only when target selection says it is legal.
type CellEffectsInput struct {
	Projection  summary.CallSummaryProjection
	Selection   TargetSelection
	Aggregation summary.CellEffectAggregation
}

// InferCellEffects combines direct summary effects with legal callback fallback.
func InferCellEffects(in CellEffectsInput) flow.CaptureEffects {
	if !in.Selection.AllowsCallbackFallback() {
		return in.Projection.CellEffects()
	}
	in.Aggregation.DirectEffects = in.Projection.CellEffects()
	return summary.AggregateCellEffects(in.Aggregation)
}

// ParamNarrowsInput is the canonical call-site policy for argument refinements
// proven by a callee. Module summary facts win; imported signature refinements
// are a fallback for external callees.
type ParamNarrowsInput struct {
	Call *ast.FuncCallExpr

	SummaryNarrows func(*ast.FuncCallExpr) ([]paramevidence.ParamNarrow, bool)
	Resolver       TypeResolver
}

// ParamNarrowsForCall resolves caller-visible parameter refinements.
func ParamNarrowsForCall(in ParamNarrowsInput) []paramevidence.ParamNarrow {
	if in.Call == nil {
		return nil
	}
	if in.SummaryNarrows != nil {
		if narrows, ok := in.SummaryNarrows(in.Call); ok {
			return paramevidence.SortParamNarrows(narrows)
		}
	}
	return paramevidence.ParamNarrowsFromFunctionType(in.Resolver.ResolveImportedFieldCallee(in.Call.Func))
}

// CallbackSpecInput is the canonical policy for finding a call's callback
// contract. Summary-known module signatures win; unresolved calls fall back to
// caller-visible callee/receiver type resolution.
type CallbackSpecInput struct {
	Call *ast.FuncCallExpr

	SummarySignature func(*ast.FuncCallExpr) typ.Type
	Resolver         TypeResolver
}

// CallbackSpecForCall extracts the callback contract used by call-site cell
// effect projection.
func CallbackSpecForCall(in CallbackSpecInput) *contract.Spec {
	return SpecForCall(SpecInput{
		Call:             in.Call,
		SummarySignature: in.SummarySignature,
		Resolver:         in.Resolver,
	})
}

// SpecInput is the canonical policy for resolving the callable spec attached to
// one call expression. Summary-known module signatures win; unresolved calls
// fall back to caller-visible callee/receiver type resolution.
type SpecInput struct {
	Call *ast.FuncCallExpr

	SummarySignature func(*ast.FuncCallExpr) typ.Type
	Resolver         TypeResolver
}

// SpecForCall resolves the callable contract spec without reading driver state.
func SpecForCall(in SpecInput) *contract.Spec {
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

// ContainerElementUnionInput is the canonical call-site policy for mutator specs
// that widen a container's element slot.
type ContainerElementUnionInput struct {
	Call *ast.FuncCallExpr

	SummarySignature func(*ast.FuncCallExpr) typ.Type
	Resolver         TypeResolver
}

// ContainerElementUnionsForCall extracts ContainerElementUnion labels from a
// call's resolved spec. It returns labels only; transfer owns lowering parameter
// refs to runtime argument places/values and applying the product mutation.
func ContainerElementUnionsForCall(in ContainerElementUnionInput) []effect.ContainerElementUnion {
	spec := SpecForCall(SpecInput{
		Call:             in.Call,
		SummarySignature: in.SummarySignature,
		Resolver:         in.Resolver,
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

// CallbackArgInput is the canonical policy for resolving one callback argument
// expression to a module function identity.
type CallbackArgInput struct {
	Arg ast.Expr

	FunctionLiteral func(*ast.FunctionExpr) (summary.FuncRef, bool)
	FunctionRefs    func(ast.Expr) ([]summary.FuncRef, bool)
	StaticExpr      func(ast.Expr) (summary.FuncRef, bool)
}

// ResolveCallbackArgRefs resolves callback arguments in deterministic precision
// order: direct function literals first, live product FunctionRefs next,
// immutable static expression facts last. A present-but-unknown product axis
// returns (nil, true), deliberately blocking static fallback.
func ResolveCallbackArgRefs(in CallbackArgInput) ([]summary.FuncRef, bool) {
	if in.Arg == nil {
		return nil, false
	}
	if fn, ok := in.Arg.(*ast.FunctionExpr); ok && fn != nil && in.FunctionLiteral != nil {
		if ref, ok := in.FunctionLiteral(fn); ok {
			return []summary.FuncRef{ref}, true
		}
	}
	if in.FunctionRefs != nil {
		if refs, ok := in.FunctionRefs(in.Arg); ok {
			return canonref.UniqueSortedFuncRefs(refs), true
		}
	}
	if in.StaticExpr != nil {
		if ref, ok := in.StaticExpr(in.Arg); ok {
			return []summary.FuncRef{ref}, true
		}
	}
	return nil, false
}

// ResolveCallbackArg is the singleton compatibility form for callers that
// cannot represent multiple callback targets. It never falls back to static
// facts after an authoritative product-axis miss; that decision is made by
// ResolveCallbackArgRefs.
func ResolveCallbackArg(in CallbackArgInput) (summary.FuncRef, bool) {
	refs, ok := ResolveCallbackArgRefs(in)
	if !ok || len(refs) != 1 {
		return summary.FuncRef{}, false
	}
	return refs[0], true
}

// StaticCallbackOverlayInput is the canonical pre-solve policy for callback
// environment overlays on callees that are not handled as module-local refs.
type StaticCallbackOverlayInput struct {
	Call *ast.FuncCallExpr

	Resolver TypeResolver
}

// StaticCallbackOverlaysForCall resolves only external/static callback overlay
// contracts for fact extraction. Module-local callee overlays are supplied by
// summary/facts through a separate ref-based path, so this function deliberately
// does not read inferred module signatures.
func StaticCallbackOverlaysForCall(in StaticCallbackOverlayInput) callbackenv.Overlays {
	if in.Call == nil {
		return nil
	}
	if ident, ok := in.Call.Func.(*ast.IdentExpr); ok && ident != nil {
		if ov := callbackenv.OverlaysFromFunction(in.Resolver.ResolveGlobalIdentType(ident)); len(ov) > 0 {
			return ov
		}
	}
	return callbackenv.OverlaysFromFunction(in.Resolver.ResolveStaticFieldCallee(in.Call.Func))
}
