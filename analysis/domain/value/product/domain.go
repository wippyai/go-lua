package product

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

func Domain(reg *axis.Registry) lattice.Lattice[Value] {
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
	for i := range rt.axes {
		spec := rt.axes[i]
		if !spec.spec.EqualAny(rt.axisValue(spec, a), rt.axisValue(spec, b)) {
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
	for i := range rt.axes {
		spec := rt.axes[i]
		if !spec.spec.LessOrEqAny(rt.axisValue(spec, a), rt.axisValue(spec, b)) {
			return false
		}
	}
	return true
}

func Join(reg *axis.Registry, a, b Value) Value {
	return joinRuntime(mustRuntime(reg), a, b)
}

func joinRuntime(rt *registryRuntime, a, b Value) Value {
	rt.validateValue(a)
	rt.validateValue(b)
	if a.n == b.n {
		return a
	}
	if a.n == nil || b.n == nil {
		return Top()
	}
	slots := make([]slot, 0, len(rt.canonicalAxes))
	for i := range rt.canonicalAxes {
		spec := rt.canonicalAxes[i]
		value := spec.spec.JoinAny(rt.axisValue(spec, a), rt.axisValue(spec, b))
		if !spec.spec.IsTopAny(value) {
			slots = append(slots, slot{key: spec.id, value: value})
		}
	}
	return internRuntime(rt,
		shapeJoin(ShapeOf(a), ShapeOf(b)),
		presence.Join(PresenceOf(a), PresenceOf(b)),
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
	slots := make([]slot, 0, len(rt.canonicalAxes))
	for i := range rt.canonicalAxes {
		spec := rt.canonicalAxes[i]
		value := spec.spec.MeetAny(rt.axisValue(spec, a), rt.axisValue(spec, b))
		if !spec.spec.IsTopAny(value) {
			slots = append(slots, slot{key: spec.id, value: value})
		}
	}
	return internRuntime(rt,
		shapeMeet(ShapeOf(a), ShapeOf(b)),
		presence.Meet(PresenceOf(a), PresenceOf(b)),
		slots,
	)
}

func Widen(reg *axis.Registry, prev, next Value) Value {
	return widenRuntime(mustRuntime(reg), prev, next)
}

func widenRuntime(rt *registryRuntime, prev, next Value) Value {
	rt.validateValue(prev)
	rt.validateValue(next)
	if prev.n == next.n {
		return prev
	}
	if prev.n == nil || next.n == nil {
		return Top()
	}
	slots := make([]slot, 0, len(rt.canonicalAxes))
	for i := range rt.canonicalAxes {
		spec := rt.canonicalAxes[i]
		value := spec.spec.WidenAny(rt.axisValue(spec, prev), rt.axisValue(spec, next))
		if !spec.spec.IsTopAny(value) {
			slots = append(slots, slot{key: spec.id, value: value})
		}
	}
	return internRuntime(rt,
		shapeWiden(ShapeOf(prev), ShapeOf(next)),
		presence.Widen(PresenceOf(prev), PresenceOf(next)),
		slots,
	)
}
