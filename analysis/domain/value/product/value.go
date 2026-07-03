package product

import (
	"fmt"
	"reflect"

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
	reg      *axis.Registry
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
	return mustRuntime(reg).bottomValue()
}

func NewWithPresence(reg *axis.Registry, shape Shape, p presence.Value) Value {
	return intern(reg, shape, p, nil)
}

func Absent(reg *axis.Registry) Value {
	return NewWithPresence(reg, ShapeTop, presence.Absent())
}

func ShapeOf(v Value) Shape {
	if v.n == nil {
		return ShapeTop
	}
	return v.n.shape
}

func PresenceOf(v Value) presence.Value {
	if v.n == nil {
		return presence.Top()
	}
	return v.n.presence
}

// DefinitelyPresent reports whether v is proven non-nil/present.
func DefinitelyPresent(v Value) bool {
	return presence.Equal(PresenceOf(v), presence.Present())
}

func WithPresence(reg *axis.Registry, v Value, p presence.Value) Value {
	rt := mustRuntime(reg)
	rt.validateValue(v)
	if presence.Equal(PresenceOf(v), p) {
		return v
	}
	return internRuntime(rt, ShapeOf(v), p, copySlots(v))
}

// WithCompatiblePresenceFrom applies source's concrete presence to base when
// base has no conflicting concrete presence. It returns false for unknown,
// bottom, or contradictory presence evidence.
func WithCompatiblePresenceFrom(reg *axis.Registry, base, source Value) (Value, bool) {
	sourcePresence := PresenceOf(source)
	if sourcePresence.IsBottom() || sourcePresence.IsTop() {
		return Value{}, false
	}
	basePresence := PresenceOf(base)
	if !basePresence.IsTop() && !presence.Equal(basePresence, sourcePresence) {
		return Value{}, false
	}
	return WithPresence(reg, base, sourcePresence), true
}

// Get reads a typed axis value. If the axis is omitted, Get returns the axis
// Top value.
func Get[T any](reg *axis.Registry, v Value, key axis.Key[T]) T {
	rt := mustRuntime(reg)
	rt.validateValue(v)
	if key.ID() == presence.Key.ID() {
		panic("product: presence is a core lane; use PresenceOf")
	}
	info, ok := rt.axis(key.ID())
	if !ok {
		panic(fmt.Sprintf("product: unregistered axis %q", key.ID()))
	}
	wantType := reflect.TypeFor[T]()
	if raw, ok := lookupSlot(v, key.ID()); ok {
		if gotType := reflect.TypeOf(raw); gotType != info.topType {
			panic(fmt.Sprintf("product: axis %q has value type %v, want registered axis type %v", key.ID(), gotType, info.topType))
		}
		if wantType != info.topType {
			panic(fmt.Sprintf("product: axis %q has incompatible typed key type %v, want %v", key.ID(), wantType, info.topType))
		}
		tv, ok := raw.(T)
		if !ok {
			panic(fmt.Sprintf("product: axis %q has value type %T, want typed key value", key.ID(), raw))
		}
		return tv
	}
	if wantType != info.topType {
		panic(fmt.Sprintf("product: axis %q has incompatible typed key type %v, want %v", key.ID(), wantType, info.topType))
	}
	tv, ok := info.topAny.(T)
	if !ok {
		panic(fmt.Sprintf("product: axis %q has top type %T, want typed key value", key.ID(), info.topAny))
	}
	return tv
}

// Set returns v with key set to value. Setting an axis to Top canonicalizes the
// slot back to omission.
func Set[T any](reg *axis.Registry, v Value, key axis.Key[T], value T) Value {
	rt := mustRuntime(reg)
	rt.validateValue(v)
	if key.ID() == presence.Key.ID() {
		panic("product: presence is a core lane; use WithPresence")
	}
	info, ok := rt.axis(key.ID())
	if !ok {
		panic(fmt.Sprintf("product: unregistered axis %q", key.ID()))
	}
	wantType := reflect.TypeFor[T]()
	if wantType != info.topType {
		panic(fmt.Sprintf("product: axis %q has incompatible typed key type %v, want %v", key.ID(), wantType, info.topType))
	}
	if existing, ok := lookupSlot(v, key.ID()); ok {
		if info.spec.EqualAny(existing, value) {
			return v
		}
	} else if info.spec.IsTopAny(value) {
		return v
	}
	slots := copySlots(v)
	if info.spec.IsTopAny(value) {
		slots = deleteSlot(slots, key.ID())
	} else {
		slots = upsertSlot(slots, key.ID(), value)
	}
	return internRuntime(rt, ShapeOf(v), PresenceOf(v), slots)
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

// slotsView returns the interned node's slot slice for read-only scanning. The
// returned slice must never be mutated; the batch editor copies before its first
// write.
func (v Value) slotsView() []slot {
	if v.n == nil {
		return nil
	}
	return v.n.slots
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
		if key < slots[i].key {
			out := make([]slot, 0, len(slots)+1)
			out = append(out, slots[:i]...)
			out = append(out, slot{key: key, value: value})
			out = append(out, slots[i:]...)
			return out
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

func shapeMeet(a, b Shape) Shape {
	if a == ShapeBottom || b == ShapeBottom {
		return ShapeBottom
	}
	return ShapeTop
}

func shapeWiden(prev, next Shape) Shape {
	return shapeJoin(prev, next)
}
