package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func expandValuesBoundary(expansion *boundaryClosureExpansion, source State) {
	if source.values.top {
		expansion.addValue(product.Top())
		return
	}
	for slot := range expansion.closure.slots {
		if value, ok := source.values.get(slot); ok {
			expansion.addValue(value)
		}
	}
}

func projectValuesBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	if source.values.top {
		out.values = source.values
		return true
	}
	for slot := range ctx.closure.slots {
		if value, ok := source.values.get(slot); ok {
			out.values, _ = out.values.write(ctx.reg, slot, product.ProjectBoundary(ctx.reg, value))
		}
	}
	return true
}
func rebaseValuesBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	if source.values.top {
		out.values = source.values
		return true
	}
	for slot, value := range source.values.cloneValues() {
		nextSlots, ok := ctx.slots[slot]
		if !ok || len(nextSlots) == 0 {
			return false
		}
		nextValue, ok := rebaseBoundaryProduct(ctx, value)
		if !ok {
			return false
		}
		for _, nextSlot := range nextSlots {
			candidate := nextValue
			if existing, exists := out.values.get(nextSlot); exists {
				candidate = product.Join(ctx.reg, existing, candidate)
			}
			out.values, _ = out.values.write(ctx.reg, nextSlot, candidate)
		}
	}
	return true
}
func applyValuesBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	if destination.values.top || fragment.values.top {
		out.values = valueLane{top: true}
		return true
	}
	next := destination.values
	for slot := range ctx.closure.slots {
		next, _ = next.write(ctx.reg, slot, product.Bottom(ctx.reg))
	}
	for slot, value := range fragment.values.cloneValues() {
		next, _ = next.write(ctx.reg, slot, value)
	}
	out.values = next
	return true
}
func equalValuesBoundary(reg *axis.Registry, a, b State) bool {
	return valueLaneDomain(reg).Equal(a.values, b.values)
}
