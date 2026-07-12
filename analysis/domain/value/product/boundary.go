package product

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// ProjectBoundary applies every registered sparse axis's declared function-
// boundary policy. Shape and the core presence component are intrinsically
// portable and remain unchanged. The original value is returned when every
// constrained axis projects identically.
func ProjectBoundary(reg *axis.Registry, value Value) Value {
	rt := mustRuntime(reg)
	rt.validateValue(value)
	if value.n == nil || len(value.n.slots) == 0 {
		return value
	}

	var projected []slot
	for i, current := range value.n.slots {
		info := rt.axisOrdinal(current.ordinal)
		if info.spec.BoundaryPolicy() == axis.PortableIdentity {
			if projected != nil {
				projected = append(projected, current)
			}
			continue
		}
		next := info.spec.ProjectBoundaryAny(current.value)
		drop := info.spec.IsTopAny(next)
		changed := drop || !info.spec.EqualAny(current.value, next)
		if projected == nil && !changed {
			continue
		}
		if projected == nil {
			projected = make([]slot, 0, len(value.n.slots))
			projected = append(projected, value.n.slots[:i]...)
		}
		if !drop {
			projected = append(projected, slot{ordinal: current.ordinal, value: next})
		}
	}
	if projected == nil {
		return value
	}
	return internConstructedRuntime(rt, ShapeOf(value), PresenceOf(value), projected)
}
