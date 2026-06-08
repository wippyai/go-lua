package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	canonicalplace "github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/callboundary"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
)

// ProductCallBoundaryApplication selects which continuation-only call-boundary
// facts are admissible at this transfer site. Expression calls apply obligations
// and effects but do not narrow their argument continuations; statement calls can
// additionally prune no-return callees and instantiate callee-proven parameter
// postconditions.
type ProductCallBoundaryApplication struct {
	Point             cfg.Point
	PruneNoReturn     bool
	ApplyParamNarrows bool
}

func (t *Transfer) applyProductCallBoundary(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	result ProductCallResult,
	demand func(int, paramevidence.ParamContract),
	app ProductCallBoundaryApplication,
) (dead bool) {
	if app.PruneNoReturn && result.NeverReturns {
		return true
	}
	t.applyCallArgDemands(out, call, result.ArgDemands, demand)
	t.applyCallResultEffects(out, app.Point, call, ctx, result.Effects, demand)
	if app.ApplyParamNarrows && (result.Postconditions.HasConstraints() || len(result.ParamNarrows) > 0) {
		return t.applyParamEvidenceAtPoint(out, app.Point, call, result.Postconditions, result.ParamNarrows)
	}
	return false
}

func (t *Transfer) applyCallResultEffects(
	out *flow.PointState,
	p cfg.Point,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	effects callboundary.Effects,
	demand func(int, paramevidence.ParamContract),
) {
	if out == nil {
		return
	}
	boundaryFacts := effects.BoundaryFacts
	boundaryRoots := t.callBoundaryLocalRoots(call, nil)
	boundaryPlan := flow.PrepareBoundaryFactReplay(*out, boundaryFacts, boundaryRoots)
	t.applyCallCellEffects(out, call, effects.CellEffects)
	t.applyCallReceiverEffects(out, call, effects.ReceiverEffects, len(ctx.RuntimeArgValues), demand)
	t.applyCallMutatorEffects(out, call, ctx, effects.ElementUnions, demand)
	if boundaryFacts.HasProof() {
		t.applyBoundaryFactsWithPlan(out, p, call, boundaryFacts, nil, boundaryPlan)
	}
}

func (t *Transfer) productCallResult(call *ast.FuncCallExpr, ctx ProductCallContext) ProductCallResult {
	if call == nil || t.callTyper == nil {
		return EmptyProductCallResult()
	}
	provider, ok := t.callTyper.(ProductCallProvider)
	if !ok || provider == nil {
		return EmptyProductCallResult()
	}
	return provider.ProductCallFromValues(call, ctx)
}

func (t *Transfer) applyCallCellEffects(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	effects flow.CaptureEffects,
) {
	if out == nil || call == nil {
		return
	}
	closurePath, _ := t.callClosurePath(call)
	t.applyCellEffect(out, CellEffect{Effects: effects, ClosurePath: closurePath})
}

func (t *Transfer) applyCallReceiverEffects(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	effects flow.ReceiverEffects,
	runtimeArgCount int,
	demand func(int, paramevidence.ParamContract),
) bool {
	if out == nil || call == nil {
		return false
	}
	if flow.ReceiverEffectsDomain.Equal(effects, flow.ReceiverEffectsDomain.Bottom()) ||
		flow.ReceiverEffectsDomain.Equal(effects, flow.ReceiverEffectsIdentity()) {
		return false
	}
	changed := false
	if effects.IsTop() {
		for slot := 0; slot < runtimeArgCount; slot++ {
			target := callsite.RuntimeArgExprAt(call, slot)
			if target == nil {
				continue
			}
			place, ok := t.placeOfExpr(out, target, demand)
			if !ok || place.Root == 0 {
				continue
			}
			changed = t.applyWriteEffect(out, WriteEffect{
				Place:         place,
				Value:         product.Domain.Top(),
				KillRelations: true,
				RecordProto:   true,
				RecordStatic:  true,
			}) || changed
		}
		return changed
	}
	for _, entry := range effects.Entries() {
		target := callsite.RuntimeArgExprAt(call, entry.Slot)
		if target == nil {
			continue
		}
		place, ok := t.placeOfExpr(out, target, demand)
		if !ok || place.Root == 0 {
			continue
		}
		mutations := receiverMutationsForTargetPlace(place, entry.Mutations)
		changed = t.applyReceiverMutations(out, place.Root, place.RootName, mutations) || changed
		value := entry.Value
		if !entry.MustWrite && !value.IsZero() {
			if old, ok := t.resolveExprValue(out, target, demand); ok && !old.IsZero() {
				value = product.Domain.Join(old, value)
			}
		}
		if value.IsZero() {
			continue
		}
		changed = t.applyReceiverValueEffect(out, place, value, mutations) || changed
	}
	return changed
}

func receiverMutationsForTargetPlace(target Place, mutations []flow.ReceiverMutation) []flow.ReceiverMutation {
	footprint, ok := target.WriteFootprint(false, product.AbstractValue{})
	if !ok {
		return nil
	}
	base, ok := flow.ReceiverMutationFromAccessFootprint(footprint)
	if !ok {
		return nil
	}
	return flow.RebaseReceiverMutations(base, mutations)
}

func (t *Transfer) applyReceiverMutations(
	out *flow.PointState,
	root cfg.SymbolID,
	rootName string,
	mutations []flow.ReceiverMutation,
) bool {
	if out == nil || root == 0 {
		return false
	}
	changed := false
	for _, mutation := range mutations {
		place, ok := receiverMutationPlace(root, rootName, mutation)
		if !ok {
			continue
		}
		changed = t.applyPlaceMutationEffect(out, PlaceMutationEffect{
			Place:                  place,
			StaticMembers:          true,
			Conditions:             true,
			KeyFacts:               true,
			PresentElementKeyFacts: mutation.PresentElementWrite,
		}) || changed
	}
	return changed
}

func receiverMutationPlace(root cfg.SymbolID, rootName string, mutation flow.ReceiverMutation) (Place, bool) {
	path := constraint.NewPath(root, rootName)
	path.Segments = append([]constraint.Segment(nil), mutation.Segments...)
	place, ok := canonicalplace.FromStaticPath(path)
	if !ok {
		return Place{}, false
	}
	if mutation.PresentElementWrite {
		place.Steps = append(place.Steps, PlaceStep{Kind: PlaceStepDynamicIndex})
	}
	return place, true
}

func (t *Transfer) applyReceiverValueEffect(
	out *flow.PointState,
	place Place,
	value product.AbstractValue,
	mutations []flow.ReceiverMutation,
) bool {
	if out == nil || place.Root == 0 || value.IsZero() {
		return false
	}
	updated := value
	if len(place.Steps) > 0 {
		var ok bool
		updated, ok = t.placeWriter().Assign(out, place, value, nil)
		if !ok {
			return false
		}
	} else {
		t.writeSymbolValue(out, place.Root, updated, false, true)
	}
	changed := true
	changed = t.recordPrototypeSelfWrite(out, place.Root, updated, true, mutations...) || changed
	return changed
}

func (t *Transfer) applyCallMutatorEffects(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	effects []effect.ContainerElementUnion,
	demand func(int, paramevidence.ParamContract),
) {
	if out == nil || call == nil {
		return
	}
	for _, effect := range effects {
		t.applyContainerElementUnionEffect(out, call, ctx, effect, demand)
	}
}

func (t *Transfer) applyContainerElementUnionEffect(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	mutation effect.ContainerElementUnion,
	demand func(int, paramevidence.ParamContract),
) bool {
	targetExpr := callsite.RuntimeArgExprAt(call, mutation.Container.Index)
	valueExpr := callsite.RuntimeArgExprAt(call, mutation.Value.Index)
	if targetExpr == nil || valueExpr == nil {
		return false
	}
	target, ok := t.placeOfExpr(out, targetExpr, demand)
	if !ok || target.Root == 0 {
		return false
	}
	elem, ok := ctx.RuntimeArgValueAt(mutation.Value.Index)
	if !ok || elem.IsZero() {
		elem = product.Top()
	}
	return t.applyMutatorEffect(out, MutatorEffect{
		Place:   target,
		Kind:    MutatorContainerElementUnion,
		Element: elem,
	})
}
