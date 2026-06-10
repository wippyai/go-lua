package product

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// Shape is the explicit placeholder for the shape component of Value. Hard
// shape widening will replace this two-point shell without changing the axis
// product APIs.
type Shape uint8

const (
	ShapeBottom Shape = iota
	ShapeTop
)

func (s Shape) IsBottom() bool {
	return s == ShapeBottom
}

func (s Shape) IsTop() bool {
	return s == ShapeTop
}

func (s Shape) String() string {
	switch s {
	case ShapeBottom:
		return "shape-bottom"
	case ShapeTop:
		return "shape-top"
	default:
		return "shape-invalid"
	}
}

// Value is the abstract-value product:
//
//	Shape x Presence x RegisteredAxes
//
// Registered-axis slots are sparse. An omitted sparse axis denotes that axis's
// Top value. Presence is a mandatory core lane, not a sparse registered axis.
type Value struct {
	n *node
}

type node struct {
	shape    Shape
	presence presence.Value
	slots    []slot
	hash     uint64
}

type slot struct {
	key   string
	value any
}

func Top() Value {
	return Value{}
}

func Bottom(reg *axis.Registry) Value {
	reg = registryOrDefault(reg)
	slots := make([]slot, 0, len(reg.Specs()))
	for _, spec := range reg.Specs() {
		slots = append(slots, slot{key: spec.ID(), value: spec.BottomAny()})
	}
	return intern(reg, ShapeBottom, presence.Bottom(), slots)
}

func New(reg *axis.Registry, shape Shape) Value {
	return intern(registryOrDefault(reg), shape, presence.Top(), nil)
}

func NewWithPresence(reg *axis.Registry, shape Shape, p presence.Value) Value {
	return intern(registryOrDefault(reg), shape, p, nil)
}

func ShapeOf(v Value) Shape {
	if v.n == nil {
		return ShapeTop
	}
	return v.n.shape
}

func WithShape(reg *axis.Registry, v Value, shape Shape) Value {
	return intern(registryOrDefault(reg), shape, PresenceOf(v), copySlots(v))
}

func PresenceOf(v Value) presence.Value {
	if v.n == nil {
		return presence.Top()
	}
	return v.n.presence
}

func WithPresence(reg *axis.Registry, v Value, p presence.Value) Value {
	return intern(registryOrDefault(reg), ShapeOf(v), p, copySlots(v))
}

// Get reads a typed axis value. If the axis is omitted, Get returns the axis
// Top value.
func Get[T any](reg *axis.Registry, v Value, key axis.Key[T]) T {
	reg = registryOrDefault(reg)
	if key.ID() == presence.Key.ID() {
		panic("product: presence is a core lane; use PresenceOf")
	}
	spec, ok := axis.Lookup[T](reg, key)
	if !ok {
		panic(fmt.Sprintf("product: unregistered axis %q", key.ID()))
	}
	if raw, ok := lookupSlot(v, key.ID()); ok {
		tv, ok := raw.(T)
		if !ok {
			panic(fmt.Sprintf("product: axis %q has value type %T, want typed key value", key.ID(), raw))
		}
		return tv
	}
	return spec.Top()
}

// Set returns v with key set to value. Setting an axis to Top canonicalizes the
// slot back to omission.
func Set[T any](reg *axis.Registry, v Value, key axis.Key[T], value T) Value {
	reg = registryOrDefault(reg)
	if key.ID() == presence.Key.ID() {
		panic("product: presence is a core lane; use WithPresence")
	}
	spec, ok := axis.Lookup[T](reg, key)
	if !ok {
		panic(fmt.Sprintf("product: unregistered axis %q", key.ID()))
	}
	slots := copySlots(v)
	if spec.Equal(value, spec.Top()) {
		slots = deleteSlot(slots, key.ID())
	} else {
		slots = upsertSlot(slots, key.ID(), value)
	}
	return intern(reg, ShapeOf(v), PresenceOf(v), slots)
}

func lookupSlot(v Value, key string) (any, bool) {
	if v.n == nil {
		return nil, false
	}
	for _, slot := range v.n.slots {
		if slot.key == key {
			return slot.value, true
		}
	}
	return nil, false
}

func copySlots(v Value) []slot {
	if v.n == nil || len(v.n.slots) == 0 {
		return nil
	}
	out := make([]slot, len(v.n.slots))
	copy(out, v.n.slots)
	return out
}

func deleteSlot(slots []slot, key string) []slot {
	for i, candidate := range slots {
		if candidate.key != key {
			continue
		}
		out := make([]slot, 0, len(slots)-1)
		out = append(out, slots[:i]...)
		out = append(out, slots[i+1:]...)
		return out
	}
	return slots
}

func upsertSlot(slots []slot, key string, value any) []slot {
	for i := range slots {
		if slots[i].key == key {
			slots[i].value = value
			return slots
		}
	}
	return append(slots, slot{key: key, value: value})
}

func shapeLessOrEq(a, b Shape) bool {
	return a == b || a == ShapeBottom || b == ShapeTop
}

func shapeJoin(a, b Shape) Shape {
	if a == ShapeTop || b == ShapeTop {
		return ShapeTop
	}
	return ShapeBottom
}

func shapeWiden(prev, next Shape) Shape {
	return shapeJoin(prev, next)
}
