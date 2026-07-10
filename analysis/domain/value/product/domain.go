package product

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

var domainCache registrycache.Cache[lattice.Lattice[Value]]

func Domain(reg *axis.Registry) lattice.Lattice[Value] {
	return domainCache.GetFor(mustRuntime(reg).reg, productDomainForRegistry)
}

func productDomainForRegistry(reg *axis.Registry) lattice.Lattice[Value] {
	rt := mustRuntime(reg)
	return lattice.Lattice[Value]{
		Bottom: rt.bottomValue,
		Top:    Top,
		Equal: func(a, b Value) bool {
			return equalRuntime(rt, a, b)
		},
		LessOrEq: func(a, b Value) bool {
			return lessOrEqRuntime(rt, a, b)
		},
		Join: func(a, b Value) Value {
			return joinRuntime(rt, a, b)
		},
		Meet: func(a, b Value) Value {
			return meetRuntime(rt, a, b)
		},
		Widen: func(prev, next Value) Value {
			return widenRuntime(rt, prev, next)
		},
	}
}

func Equal(reg *axis.Registry, a, b Value) bool {
	return equalRuntime(mustRuntime(reg), a, b)
}

func equalRuntime(rt *registryRuntime, a, b Value) bool {
	rt.validateValue(a)
	rt.validateValue(b)
	if a.n == b.n {
		return true
	}
	if a.n != nil && b.n != nil && a.n.hash != 0 && b.n.hash != 0 && a.n.hash != b.n.hash {
		return false
	}
	if ShapeOf(a) != ShapeOf(b) {
		return false
	}
	if !presence.Equal(PresenceOf(a), PresenceOf(b)) {
		return false
	}
	aSlots, bSlots := a.slotsView(), b.slotsView()
	if len(aSlots) != len(bSlots) {
		return false
	}
	for i := range aSlots {
		if aSlots[i].ordinal != bSlots[i].ordinal {
			return false
		}
		spec := rt.axisOrdinal(aSlots[i].ordinal)
		if !spec.spec.EqualAny(aSlots[i].value, bSlots[i].value) {
			return false
		}
	}
	return true
}

func LessOrEq(reg *axis.Registry, a, b Value) bool {
	return lessOrEqRuntime(mustRuntime(reg), a, b)
}

func lessOrEqRuntime(rt *registryRuntime, a, b Value) bool {
	rt.validateValue(a)
	rt.validateValue(b)
	if a.n == b.n {
		return true
	}
	if !shapeLessOrEq(ShapeOf(a), ShapeOf(b)) {
		return false
	}
	presenceSpec := presence.Spec()
	if !presenceSpec.LessOrEq(PresenceOf(a), PresenceOf(b)) {
		return false
	}
	// Slots are sorted and omit Top. Merge the two sparse lanes instead of
	// scanning every registered axis: absent/absent is Top/Top, a present left
	// value is always <= an absent right Top, and an absent left Top cannot be
	// <= a present non-Top right value. This keeps the hot comparison path out
	// of erased dispatch unless both values actually constrain the same axis.
	aSlots, bSlots := a.slotsView(), b.slotsView()
	ai, bi := 0, 0
	for ai < len(aSlots) {
		if bi == len(bSlots) || aSlots[ai].ordinal < bSlots[bi].ordinal {
			ai++
			continue
		}
		if bSlots[bi].ordinal < aSlots[ai].ordinal {
			return false
		}
		spec := rt.axisOrdinal(aSlots[ai].ordinal)
		if !spec.spec.LessOrEqAny(aSlots[ai].value, bSlots[bi].value) {
			return false
		}
		ai++
		bi++
	}
	return bi == len(bSlots)
}

func Join(reg *axis.Registry, a, b Value) Value {
	return joinRuntime(mustRuntime(reg), a, b)
}

func joinRuntime(rt *registryRuntime, a, b Value) Value {
	return mergeRuntime(rt, a, b, axisRuntimeAxis.joinAxis, shapeJoin, presence.Join)
}

func (spec axisRuntimeAxis) joinAxis(va, vb any) any  { return spec.spec.JoinAny(va, vb) }
func (spec axisRuntimeAxis) widenAxis(va, vb any) any { return spec.spec.WidenAny(va, vb) }

// mergeRuntime is the shared product join/widen: it merges every canonical axis
// with axisMerge, the shape with shapeMerge, and presence with presenceMerge,
// dropping axes that merge to top. Missing operands (nil carrier) make the
// result top.
func mergeRuntime(
	rt *registryRuntime,
	a, b Value,
	axisMerge func(axisRuntimeAxis, any, any) any,
	shapeMerge func(Shape, Shape) Shape,
	presenceMerge func(presence.Value, presence.Value) presence.Value,
) Value {
	rt.validateValue(a)
	rt.validateValue(b)
	if a.n == b.n {
		return a
	}
	if a.n == nil || b.n == nil {
		return Top()
	}
	var small [8]slot
	slots := small[:0]
	aSlots, bSlots := a.slotsView(), b.slotsView()
	for ai, bi := 0, 0; ai < len(aSlots) && bi < len(bSlots); {
		if aSlots[ai].ordinal < bSlots[bi].ordinal {
			ai++ // value merge Top is Top
			continue
		}
		if bSlots[bi].ordinal < aSlots[ai].ordinal {
			bi++ // Top merge value is Top
			continue
		}
		spec := rt.axisOrdinal(aSlots[ai].ordinal)
		value := axisMerge(spec, aSlots[ai].value, bSlots[bi].value)
		if !spec.spec.IsTopAny(value) {
			slots = append(slots, slot{ordinal: spec.ordinal, value: value})
		}
		ai++
		bi++
	}
	return internConstructedRuntime(rt,
		shapeMerge(ShapeOf(a), ShapeOf(b)),
		presenceMerge(PresenceOf(a), PresenceOf(b)),
		slots,
	)
}

func Meet(reg *axis.Registry, a, b Value) Value {
	return meetRuntime(mustRuntime(reg), a, b)
}

func meetRuntime(rt *registryRuntime, a, b Value) Value {
	rt.validateValue(a)
	rt.validateValue(b)
	if a.n == b.n {
		return a
	}
	if a.n == nil {
		return b
	}
	if b.n == nil {
		return a
	}
	var small [8]slot
	slots := small[:0]
	aSlots, bSlots := a.slotsView(), b.slotsView()
	for ai, bi := 0, 0; ai < len(aSlots) || bi < len(bSlots); {
		switch {
		case bi == len(bSlots) || (ai < len(aSlots) && aSlots[ai].ordinal < bSlots[bi].ordinal):
			slots = append(slots, aSlots[ai]) // value meet Top is value
			ai++
		case ai == len(aSlots) || bSlots[bi].ordinal < aSlots[ai].ordinal:
			slots = append(slots, bSlots[bi]) // Top meet value is value
			bi++
		default:
			spec := rt.axisOrdinal(aSlots[ai].ordinal)
			value := spec.spec.MeetAny(aSlots[ai].value, bSlots[bi].value)
			if !spec.spec.IsTopAny(value) {
				slots = append(slots, slot{ordinal: spec.ordinal, value: value})
			}
			ai++
			bi++
		}
	}
	return internConstructedRuntime(rt,
		shapeMeet(ShapeOf(a), ShapeOf(b)),
		presence.Meet(PresenceOf(a), PresenceOf(b)),
		slots,
	)
}

func Widen(reg *axis.Registry, prev, next Value) Value {
	return widenRuntime(mustRuntime(reg), prev, next)
}

func widenRuntime(rt *registryRuntime, prev, next Value) Value {
	return mergeRuntime(rt, prev, next, axisRuntimeAxis.widenAxis, shapeWiden, presence.Widen)
}
