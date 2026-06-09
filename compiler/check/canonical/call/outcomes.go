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
	typeShape  TypeShapeOutcome
}

// HasTargets reports whether the outcome has selected concrete callee summaries.
func (o CallOutcome) HasTargets() bool {
	return o.Selection.HasTargets()
}

// WithTypeShapeOutcome attaches the explicitly computed non-summary outcome.
// Type-shape facts are intentionally restricted to type-contract evidence;
// summary effects, return refs, static members, and no-return facts remain
// summary-only.
func (o CallOutcome) WithTypeShapeOutcome(typeShape TypeShapeOutcome) CallOutcome {
	o.typeShape = typeShape
	return o
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

// ReturnValues projects the caller-visible product return tuple from the single
// selected outcome. Summary results remain authoritative, while builtin/declared
// type shape is already materialized in the non-summary outcome instead of being
// recovered through late resolver callbacks.
func (o CallOutcome) ReturnValues(in ReturnValueInput) ([]product.AbstractValue, bool) {
	in.SummaryReturnValues = o.InferredReturnValues()
	in.PrimaryReturnTypes = o.typeShape.PrimaryReturnTypes()
	in.HasPrimaryReturnTypes = len(in.PrimaryReturnTypes) > 0 || o.typeShape.hasPrimaryReturnTypes
	in.TypeShapeReturnTypes = o.typeShape.TypeShapeReturnTypes()
	in.HasTypeShapeReturnTypes = len(in.TypeShapeReturnTypes) > 0 || o.typeShape.hasTypeShapeReturnTypes
	return in.Values()
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

// ReturnRelations projects return-slot relations from the selected outcome.
func (o CallOutcome) ReturnRelations() flow.ReturnRelations {
	if len(o.Projection.Targets) > 0 {
		return o.Projection.ReturnRelations()
	}
	if o.Selection.BlocksTypeShape() {
		return flow.ReturnRelationsDomain.Top()
	}
	return o.typeShape.ReturnRelations()
}

// CellEffects projects caller-visible capture effects through the canonical
// direct-summary plus syntactic callback projection policy.
func (o CallOutcome) CellEffects(aggregation summary.CellEffectAggregation) flow.CaptureEffects {
	if !o.Selection.AllowsSyntacticCallbackProjection() {
		return o.Projection.CellEffects()
	}
	aggregation.DirectEffects = o.Projection.CellEffects()
	return summary.AggregateCellEffects(aggregation)
}

// ReceiverEffects projects caller-visible receiver/container effects.
func (o CallOutcome) ReceiverEffects() flow.ReceiverEffects {
	return o.Projection.ReceiverEffects()
}

// BoundaryFacts projects caller-visible parameter/return-relative facts from the
// selected outcome.
func (o CallOutcome) BoundaryFacts() flow.BoundaryFacts {
	if len(o.Projection.Targets) > 0 {
		return o.Projection.BoundaryFacts()
	}
	if o.Selection.BlocksTypeShape() {
		return flow.BoundaryFactsDomain.Top()
	}
	return o.typeShape.BoundaryFacts()
}

// Postconditions projects portable normal-return proofs from the selected
// outcome. Summary targets own body-derived postconditions; type-shape outcome
// owns only static signature/imported contract postconditions.
func (o CallOutcome) Postconditions() paramevidence.ReturnPostconditions {
	if o.HasTargets() {
		return o.Projection.Postconditions()
	}
	if o.Selection.BlocksTypeShape() {
		return paramevidence.ReturnPostconditionsDomain.Bottom()
	}
	return o.typeShape.Postconditions()
}

// CallbackSpec returns the type-contract callback specification attached to the
// explicit type-shape source. The spec is used only to route canonical callback
// summaries; it is not itself a capture-effect proof.
func (o CallOutcome) CallbackSpec() *contract.Spec {
	return o.typeShape.CallbackSpec()
}

// ContainerElementUnions returns type-contract container mutation labels from
// the explicit type-shape source. Transfer owns lowering these labels onto runtime
// argument places.
func (o CallOutcome) ContainerElementUnions() []effect.ContainerElementUnion {
	return o.typeShape.ContainerElementUnions()
}

// FunctionShape returns the declared/imported callable shape used only for
// signature-owned argument obligations when no selected summary demands apply.
func (o CallOutcome) FunctionShape() *typ.Function {
	return o.typeShape.FunctionShape()
}

// NeverReturns reports whether every selected target is proven no-return.
func (o CallOutcome) NeverReturns(hasNoReturn func(summary.FuncRef) bool) bool {
	return selectionNeverReturns(o.Selection, hasNoReturn)
}

type specInput struct {
	Call *ast.FuncCallExpr

	SummarySignature     typ.Type
	Resolver             TypeResolver
	UseResolvedSignature bool
}

func specForCall(in specInput) *contract.Spec {
	if in.Call == nil {
		return nil
	}
	callee := typ.Type(nil)
	callee = in.SummarySignature
	if callee == nil || typ.IsAbsentOrUnknown(callee) {
		if in.Call.Method != "" {
			receiver := in.Resolver.ResolveReceiver(in.Call.Receiver)
			if receiver != nil && !typ.IsAbsentOrUnknown(receiver) {
				if member, ok := core.Field(receiver, in.Call.Method); ok {
					callee = member
				}
			}
		} else if in.UseResolvedSignature {
			callee = in.Resolver.ResolveCallee(in.Call.Func)
		} else {
			callee = in.Resolver.ResolveStaticCallee(in.Call.Func)
		}
	}
	return contract.ExtractSpec(callee)
}

func containerElementUnionsFromSpec(spec *contract.Spec) []effect.ContainerElementUnion {
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
