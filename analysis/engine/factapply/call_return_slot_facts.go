package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyCallOutcomeReturnSlotFactsAfterRootAssignment(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	outcomeProvider callpayload.CallOutcomeProvider,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	widen CovariantWiden,
	typeValues *typevalue.Cache,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if outcomeProvider == nil || targetPath.IsEmpty() {
		return out
	}
	callPoint, site, slots, ok := callResultSiteForAssignment(facts, ctx.Point, targetPath, source)
	if !ok {
		return out
	}
	callCtx := callContextAt(ctx, callPoint, read)
	outcome := outcomeProvider(callCtx, site, in, read)
	normalFacts := normalReturnFactsForReturnSlots(outcome.NormalReturnFacts, slots)
	normalFacts = dropReturnSlotDescendantsBelowMaybeAbsentResults(normalFacts, slots, outcome.Results)
	if normalFacts.Empty() {
		return out
	}
	return applyCallOutcomeFacts(ctx, facts, resolver, projectPath, widen, typeValues, out, site, callpayload.CallOutcome{
		NormalReturnFacts: normalFacts,
	})
}

func callResultSiteForAssignment(
	facts factflow.Facts,
	point cfg.Point,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) (cfg.Point, factflow.CallSiteView, map[int]struct{}, bool) {
	if site, ok := facts.CallSiteView(point); ok {
		if slots := callResultSlotsForTarget(site, targetPath); len(slots) != 0 {
			return point, site, slots, true
		}
	}
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint == 0 {
		return 0, factflow.CallSiteView{}, nil, false
	}
	site, ok := facts.CallSiteView(source.CallPoint)
	if !ok {
		return 0, factflow.CallSiteView{}, nil, false
	}
	slots := callResultSlotsForTarget(site, targetPath)
	if len(slots) == 0 {
		return 0, factflow.CallSiteView{}, nil, false
	}
	if source.ResultIndex >= 0 {
		if _, ok := slots[source.ResultIndex]; !ok {
			return 0, factflow.CallSiteView{}, nil, false
		}
		slots = map[int]struct{}{source.ResultIndex: {}}
	}
	return source.CallPoint, site, slots, true
}

func callResultSlotsForTarget(site factflow.CallSiteView, targetPath pathdom.Path) map[int]struct{} {
	var slots map[int]struct{}
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if target.ResultIndex() < 0 || !target.TargetPathEqual(targetPath) {
			return true
		}
		if slots == nil {
			slots = make(map[int]struct{}, 1)
		}
		slots[target.ResultIndex()] = struct{}{}
		return true
	})
	return slots
}

func normalReturnFactsForReturnSlots(facts callboundary.NormalReturnFacts, slots map[int]struct{}) callboundary.NormalReturnFacts {
	if facts.Empty() || len(slots) == 0 {
		return callboundary.NormalReturnFacts{}
	}
	return facts.FilterPaths(func(p pathdom.Path) bool {
		return callboundary.PathRootedInReturnSlots(p, slots)
	})
}

func dropReturnSlotDescendantsBelowMaybeAbsentResults(facts callboundary.NormalReturnFacts, slots map[int]struct{}, results []callpayload.CallResult) callboundary.NormalReturnFacts {
	if facts.Empty() || len(slots) == 0 || len(results) == 0 {
		return facts
	}
	maybeAbsent := make(map[int]struct{}, len(slots))
	for _, result := range results {
		if _, used := slots[result.Index]; !used {
			continue
		}
		if !product.DefinitelyPresent(result.Value) {
			maybeAbsent[result.Index] = struct{}{}
		}
	}
	if len(maybeAbsent) == 0 {
		return facts
	}
	return facts.DropFactsTouchingPaths(func(p pathdom.Path) bool {
		if len(p.Segments) == 0 {
			return false
		}
		index, indexed := callboundary.ReturnSlotIndex(p)
		if !indexed {
			return false
		}
		_, ok := maybeAbsent[index]
		return ok
	})
}
