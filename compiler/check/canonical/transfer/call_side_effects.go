package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	canonicalplace "github.com/wippyai/go-lua/compiler/check/canonical/place"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
)

type assignCallApplications struct {
	byCall map[*ast.FuncCallExpr]assignCallApplication
	calls  []*ast.FuncCallExpr
}

type assignCallApplication struct {
	Context ProductCallContext
	Result  ProductCallResult
}

// ProductCallBoundaryApplication selects which continuation-only call-boundary
// facts are admissible at this transfer site. Expression calls apply obligations
// and effects but do not narrow their argument continuations; statement calls can
// additionally prune no-return callees and instantiate callee-proven parameter
// postconditions.
type ProductCallBoundaryApplication struct {
	Point               cfg.Point
	PruneNoReturn       bool
	ApplyPostconditions bool
}

func (t *Transfer) applyProductCallBoundary(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	result ProductCallResult,
	demand func(int, paramevidence.ParamContract),
	app ProductCallBoundaryApplication,
) (dead bool) {
	boundary := result.Boundary
	if app.PruneNoReturn && boundary.NeverReturns {
		return true
	}
	t.applyCallArgDemands(out, call, boundary.ArgDemands, demand)
	t.applyCallBoundaryOutcome(out, app.Point, call, ctx, boundary, demand)
	if app.ApplyPostconditions && boundary.Postconditions.HasConstraints() {
		return t.ApplyReturnPostconditionsAtPoint(out, app.Point, call, boundary.Postconditions)
	}
	return false
}

// applyCallArgDemands replays caller-visible argument obligations from the
// selected BoundaryOutcome into the backward demand sink for this call site.
func (t *Transfer) applyCallArgDemands(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	expected []callobligation.Obligation,
	demand func(int, paramevidence.ParamContract),
) {
	if out == nil || call == nil || demand == nil || len(expected) == 0 {
		return
	}
	for i, arg := range call.Args {
		if i >= len(expected) {
			break
		}
		if !expected[i].Informative() {
			continue
		}
		if contract, ok := paramevidence.ObligationContract(expected[i]); ok {
			t.demandExprContractCtx(out, arg, contract, demand)
			continue
		}
		t.demandExprCtx(out, arg, expected[i].Type, demand)
	}
}

func (t *Transfer) prepareAssignCallApplications(
	out *flow.PointState,
	info *cfg.AssignInfo,
	demand func(int, paramevidence.ParamContract),
) assignCallApplications {
	apps := assignCallApplications{}
	if out == nil || info == nil || t.callTyper == nil {
		return apps
	}
	addCall := func(call *ast.FuncCallExpr) {
		if call == nil || t.transferOwnsCallValue(call) {
			return
		}
		if apps.byCall != nil {
			if _, exists := apps.byCall[call]; exists {
				return
			}
		}
		ctx, result, ok := t.selectProductCall(out, call, demand)
		if !ok {
			return
		}
		if apps.byCall == nil {
			apps.byCall = make(map[*ast.FuncCallExpr]assignCallApplication)
		}
		apps.calls = append(apps.calls, call)
		apps.byCall[call] = assignCallApplication{
			Context: ctx,
			Result:  result,
		}
	}
	info.EachSourceCall(func(_ int, callInfo *cfg.CallInfo) {
		if callInfo == nil {
			return
		}
		addCall(callInfo.Call)
	})
	info.EachSource(func(_ int, src ast.Expr) {
		if call, ok := src.(*ast.FuncCallExpr); ok {
			addCall(call)
		}
	})
	return apps
}

func (t *Transfer) applyAssignCallApplications(
	out *flow.PointState,
	apps assignCallApplications,
	demand func(int, paramevidence.ParamContract),
) {
	for _, call := range apps.calls {
		app, ok := apps.byCall[call]
		if !ok {
			continue
		}
		t.applyProductCallBoundary(out, call, app.Context, app.Result, demand, ProductCallBoundaryApplication{})
	}
}

func (apps assignCallApplications) applicationForTarget(
	info *cfg.AssignInfo,
	targetIndex int,
	src ast.Expr,
) (assignCallApplication, int, bool, bool) {
	if len(apps.byCall) == 0 {
		return assignCallApplication{}, 0, false, false
	}
	if info != nil {
		if call, retIndex := info.CallForTarget(targetIndex); call != nil && call.Call != nil {
			app, ok := apps.byCall[call.Call]
			expanding, first := info.ExpandingSourceCall()
			return app, retIndex, expanding != nil && targetIndex >= first, ok
		}
		if targetIndex >= 0 && targetIndex < len(info.Sources) {
			if call, ok := info.Sources[targetIndex].(*ast.FuncCallExpr); ok && call != nil {
				app, ok := apps.byCall[call]
				return app, 0, false, ok
			}
		}
		lastSource := len(info.Sources) - 1
		if lastSource >= 0 && targetIndex >= lastSource && len(info.Targets)-lastSource >= 2 {
			if call, ok := info.Sources[lastSource].(*ast.FuncCallExpr); ok && call != nil {
				app, ok := apps.byCall[call]
				return app, targetIndex - lastSource, true, ok
			}
		}
	}
	call, ok := src.(*ast.FuncCallExpr)
	if !ok || call == nil {
		return assignCallApplication{}, 0, false, false
	}
	app, ok := apps.byCall[call]
	return app, 0, false, ok
}

func (t *Transfer) productCallApplication(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
	app ProductCallBoundaryApplication,
) (ProductCallContext, ProductCallResult, bool, bool) {
	ctx, result, ok := t.selectProductCall(out, call, demand)
	if !ok {
		return ProductCallContext{}, EmptyProductCallResult(), false, false
	}
	dead := t.applyProductCallBoundary(out, call, ctx, result, demand, app)
	return ctx, result, dead, true
}

func (t *Transfer) selectProductCall(
	out *flow.PointState,
	call *ast.FuncCallExpr,
	demand func(int, paramevidence.ParamContract),
) (ProductCallContext, ProductCallResult, bool) {
	if call == nil || t.callTyper == nil {
		return ProductCallContext{}, EmptyProductCallResult(), false
	}
	ctx := t.productCallContext(out, call, demand)
	result := t.productCallResult(call, ctx)
	return ctx, result, true
}

func (t *Transfer) transferOwnsCallValue(call *ast.FuncCallExpr) bool {
	if call == nil {
		return false
	}
	if t.in.Graph != nil && metatable.IsSetMetatableCall(call, t.in.Graph.Bindings()) {
		return true
	}
	return t.isTableCreateCall(call)
}

func (t *Transfer) applyCallBoundaryOutcome(
	out *flow.PointState,
	p cfg.Point,
	call *ast.FuncCallExpr,
	ctx ProductCallContext,
	boundary BoundaryOutcome,
	demand func(int, paramevidence.ParamContract),
) {
	if out == nil {
		return
	}
	boundaryFacts := boundary.BoundaryFacts
	boundaryRoots := t.callBoundaryLocalRootsAt(out, p, call, nil)
	boundaryPlan := flow.PrepareBoundaryFactReplay(*out, boundaryFacts, boundaryRoots)
	t.applyCallCellEffects(out, call, boundary.CellEffects)
	t.applyCallReceiverEffects(out, call, boundary.ReceiverEffects, len(ctx.RuntimeArgValues), demand)
	t.applyCallMutatorEffects(out, call, ctx, boundary.ElementUnions, demand)
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
