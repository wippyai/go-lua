package product

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// DifferenceAxes names every product component whose lattice values differ.
// It is intended for semantic differential gates and diagnostics, not solver
// hot paths; the returned slice is newly allocated.
func DifferenceAxes(reg *axis.Registry, left, right Value) []string {
	rt := mustRuntime(reg)
	rt.validateValue(left)
	rt.validateValue(right)
	var out []string
	if ShapeOf(left) != ShapeOf(right) {
		out = append(out, "shape")
	}
	if !presence.Equal(PresenceOf(left), PresenceOf(right)) {
		out = append(out, presence.Key.ID())
	}
	for _, info := range rt.axes {
		leftValue := info.topAny
		if value, exists := lookupSlot(left, info.ordinal); exists {
			leftValue = value
		}
		rightValue := info.topAny
		if value, exists := lookupSlot(right, info.ordinal); exists {
			rightValue = value
		}
		if !info.spec.EqualAny(leftValue, rightValue) {
			out = append(out, info.id)
		}
	}
	return out
}
