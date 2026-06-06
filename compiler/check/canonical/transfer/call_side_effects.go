package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	canonicalplace "github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func (t *Transfer) applyCallSideEffects(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	demand func(int, paramevidence.ParamContract),
) {
	boundaryFacts, boundaryAppendPlans := t.callBoundaryFactsAndAppendPlans(out, call, ctx)
	t.applyCallCellEffects(out, call, ctx, ctx.ExprType)
	t.applyCallReceiverEffects(out, call, ctx, demand)
	t.applyCallMutatorEffects(out, call, ctx, demand)
	if boundaryFacts.HasProof() {
		t.applyBoundaryFactsWithAppendPlans(out, call, boundaryFacts, nil, boundaryAppendPlans)
	}
}

func (t *Transfer) applyCallCellEffects(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	exprType func(ast.Expr) typ.Type,
) {
	if out == nil || call == nil {
		return
	}
	var effects flow.CaptureEffects
	if provider, ok := t.callTyper.(productCellEffectProvider); ok {
		effects = provider.CellEffectsFromValues(call, ctx)
	} else if provider, ok := t.callTyper.(cellEffectProvider); ok {
		effects = provider.CellEffects(call, exprType, out.Cells, out.FunctionRefs)
	} else {
		return
	}
	closurePath, _ := t.callClosurePath(call)
	t.applyCellEffect(out, CellEffect{Effects: effects, ClosurePath: closurePath})
}

func (t *Transfer) applyCallReceiverEffects(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	demand func(int, paramevidence.ParamContract),
) bool {
	if out == nil || call == nil || t.callTyper == nil {
		return false
	}
	provider, ok := t.callTyper.(productCallPostEffectProvider)
	if !ok || provider == nil {
		return false
	}
	effects := provider.CallPostEffectsFromValues(call, ctx).ReceiverEffects
	if flow.ReceiverEffectsDomain.Equal(effects, flow.ReceiverEffectsDomain.Bottom()) ||
		flow.ReceiverEffectsDomain.Equal(effects, flow.ReceiverEffectsIdentity()) {
		return false
	}
	changed := false
	if effects.IsTop() {
		for slot := range ctx.RuntimeArgValues {
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
	prefix, ok := target.StaticPrefixPath()
	if !ok || prefix.Symbol == 0 {
		return nil
	}
	if len(mutations) == 0 {
		return []flow.ReceiverMutation{{
			Segments: append([]constraint.Segment(nil), prefix.Segments...),
		}}
	}
	out := make([]flow.ReceiverMutation, 0, len(mutations))
	for _, mutation := range mutations {
		segs := append([]constraint.Segment(nil), prefix.Segments...)
		segs = append(segs, mutation.Segments...)
		out = append(out, flow.ReceiverMutation{
			Segments:            segs,
			PresentElementWrite: mutation.PresentElementWrite,
		})
	}
	return out
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
		updated, _, ok = t.assignPlaceValue(out, place, value, nil)
		if !ok {
			return false
		}
	}
	t.writeRootContainer(out, place.Root, updated)
	changed := true
	changed = t.recordPrototypeSelfWrite(out, place.Root, updated, true, mutations...) || changed
	return changed
}

func (t *Transfer) applyCallMutatorEffects(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	demand func(int, paramevidence.ParamContract),
) {
	if out == nil || call == nil || t.callTyper == nil {
		return
	}
	provider, ok := t.callTyper.(containerElementUnionProvider)
	if !ok || provider == nil {
		return
	}
	for _, effect := range provider.ContainerElementUnionsFromValues(call, ctx) {
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
