package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func emitValuesReachability(*boundaryReachabilityProgramBuilder, valueLane) {}

func projectValuesBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.values, _ = projectValuesBoundaryFactor(ctx, source.values)
	return true
}
func projectValuesBoundaryFactor(ctx *boundaryProjectContext, source valueLane) (valueLane, bool) {
	if source.top {
		return source, true
	}
	out := valueLane{}
	for slot := range ctx.closure.slots {
		if value, ok := source.get(slot); ok {
			out, _ = out.write(ctx.reg, slot, product.ProjectBoundary(ctx.reg, value))
		}
	}
	return out, true
}
func rebaseValuesBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.values, ok = rebaseValuesBoundaryFactor(ctx, source.values)
	return ok
}
func rebaseValuesBoundaryFactor(ctx *boundaryRebaseContext, source valueLane) (valueLane, bool) {
	if source.top {
		return source, true
	}
	out := valueLane{}
	for slot, value := range source.cloneValues() {
		nextSlots, ok := boundaryRebaseSlots(ctx, slot)
		if !ok {
			return valueLane{}, false
		}
		nextValue, ok := rebaseBoundaryProduct(ctx, value)
		if !ok {
			return valueLane{}, false
		}
		for _, nextSlot := range nextSlots {
			candidate := nextValue
			if existing, exists := out.get(nextSlot); exists {
				candidate = product.Join(ctx.reg, existing, candidate)
			}
			out, _ = out.write(ctx.reg, nextSlot, candidate)
		}
	}
	return out, true
}
func applyValuesBoundaryLane(ctx *boundaryApplyContext, destination, fragment valueLane) (valueLane, bool) {
	if destination.top || fragment.top {
		return valueLane{top: true}, true
	}
	next := destination
	for slot := range ctx.closure.slots {
		next, _ = next.write(ctx.reg, slot, product.Bottom(ctx.reg))
	}
	for slot, value := range fragment.cloneValues() {
		next, _ = next.write(ctx.reg, slot, value)
	}
	return next, true
}

func applyValuesBoundaryRoots(ctx *boundaryApplyContext, lane valueLane, roots boundaryRootPlan) (valueLane, bool) {
	if lane.top {
		return lane, true
	}
	for _, root := range roots.slots {
		lane, _ = lane.write(ctx.reg, root.Slot, root.Value)
	}
	return lane, true
}
func equalValuesBoundary(reg *axis.Registry, a, b State) bool {
	return valueLaneDomain(reg).Equal(a.values, b.values)
}
