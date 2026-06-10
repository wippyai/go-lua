package product

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

func Domain(reg *axis.Registry) lattice.Lattice[Value] {
	reg = registryOrDefault(reg)
	return lattice.Lattice[Value]{
		Bottom: func() Value { return Bottom(reg) },
		Top:    Top,
		Equal: func(a, b Value) bool {
			return Equal(reg, a, b)
		},
		LessOrEq: func(a, b Value) bool {
			return LessOrEq(reg, a, b)
		},
		Join: func(a, b Value) Value {
			return Join(reg, a, b)
		},
		Widen: func(prev, next Value) Value {
			return Widen(reg, prev, next)
		},
	}
}

func Equal(reg *axis.Registry, a, b Value) bool {
	reg = registryOrDefault(reg)
	validateValue(reg, a)
	validateValue(reg, b)
	if a.n == b.n {
		return true
	}
	if ShapeOf(a) != ShapeOf(b) {
		return false
	}
	if !presence.Equal(PresenceOf(a), PresenceOf(b)) {
		return false
	}
	for _, spec := range reg.Specs() {
		if !spec.EqualAny(axisValue(spec, a), axisValue(spec, b)) {
			return false
		}
	}
	return true
}

func LessOrEq(reg *axis.Registry, a, b Value) bool {
	reg = registryOrDefault(reg)
	validateValue(reg, a)
	validateValue(reg, b)
	if !shapeLessOrEq(ShapeOf(a), ShapeOf(b)) {
		return false
	}
	presenceSpec := presence.Spec()
	if !presenceSpec.LessOrEq(PresenceOf(a), PresenceOf(b)) {
		return false
	}
	for _, spec := range reg.Specs() {
		if !spec.LessOrEqAny(axisValue(spec, a), axisValue(spec, b)) {
			return false
		}
	}
	return true
}

func Join(reg *axis.Registry, a, b Value) Value {
	reg = registryOrDefault(reg)
	validateValue(reg, a)
	validateValue(reg, b)
	slots := make([]slot, 0, len(reg.Specs()))
	for _, spec := range reg.Specs() {
		value := spec.JoinAny(axisValue(spec, a), axisValue(spec, b))
		if !spec.IsTopAny(value) {
			slots = append(slots, slot{key: spec.ID(), value: value})
		}
	}
	return intern(reg,
		shapeJoin(ShapeOf(a), ShapeOf(b)),
		presence.Join(PresenceOf(a), PresenceOf(b)),
		slots,
	)
}

func Widen(reg *axis.Registry, prev, next Value) Value {
	reg = registryOrDefault(reg)
	validateValue(reg, prev)
	validateValue(reg, next)
	slots := make([]slot, 0, len(reg.Specs()))
	for _, spec := range reg.Specs() {
		value := spec.WidenAny(axisValue(spec, prev), axisValue(spec, next))
		if !spec.IsTopAny(value) {
			slots = append(slots, slot{key: spec.ID(), value: value})
		}
	}
	return intern(reg,
		shapeWiden(ShapeOf(prev), ShapeOf(next)),
		presence.Widen(PresenceOf(prev), PresenceOf(next)),
		slots,
	)
}

func axisValue(spec axis.ErasedSpec, v Value) any {
	if v.n != nil {
		for _, slot := range v.n.slots {
			if slot.key == spec.ID() {
				return slot.value
			}
		}
	}
	return spec.TopAny()
}

func validateValue(reg *axis.Registry, v Value) {
	if v.n == nil {
		return
	}
	for _, slot := range v.n.slots {
		if _, ok := reg.LookupErased(slot.key); !ok {
			panic("product: value contains slot outside registry: " + slot.key)
		}
	}
}
