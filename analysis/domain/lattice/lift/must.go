package lift

import (
	"reflect"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

// MustMapLane is a finite must-fact map carrier.
//
// Bottom is explicit. Top is the reachable empty map. The order is
// information-must order: more required keys is lower, so joins and widens keep
// only keys present in both operands and combine their values with the element
// lattice operation. Values maps are persistent by convention once published to
// the lattice; operations may return an input carrier for identity/equality
// cases.
type MustMapLane[K comparable, V any] struct {
	bottom bool
	values map[K]V
}

// MustMapBottom builds an explicit-bottom must-map lane.
func MustMapBottom[K comparable, V any]() MustMapLane[K, V] {
	return MustMapLane[K, V]{bottom: true}
}

// MustMapValues builds a reachable must-map lane over values.
func MustMapValues[K comparable, V any](values map[K]V) MustMapLane[K, V] {
	return MustMapLane[K, V]{values: values}
}

// Bottom reports whether this lane is explicit bottom.
func (l MustMapLane[K, V]) Bottom() bool {
	return l.bottom
}

// Values returns the lane's finite values map.
func (l MustMapLane[K, V]) Values() map[K]V {
	return l.values
}

// Clone returns an independent copy of the finite values map.
func (l MustMapLane[K, V]) Clone() MustMapLane[K, V] {
	return MustMapLane[K, V]{
		bottom: l.bottom,
		values: cloneMap(l.values),
	}
}

// MustMap lifts an element lattice into a finite must-map lattice.
func MustMap[K comparable, V any](elem lattice.Lattice[V]) lattice.Lattice[MustMapLane[K, V]] {
	sameMapValue := func(a, b map[K]V) bool {
		return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
	}
	l := lattice.Lattice[MustMapLane[K, V]]{
		Bottom: func() MustMapLane[K, V] {
			return MustMapBottom[K, V]()
		},
		Top: func() MustMapLane[K, V] {
			return MustMapLane[K, V]{}
		},
		Equal: func(a, b MustMapLane[K, V]) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			if sameMapValue(a.values, b.values) {
				return true
			}
			return finiteMapEqual(a.values, b.values, elem.Equal)
		},
		Same: func(a, b MustMapLane[K, V]) bool {
			return a.bottom == b.bottom && sameMapValue(a.values, b.values)
		},
		LessOrEq: func(a, b MustMapLane[K, V]) bool {
			if a.bottom == b.bottom && sameMapValue(a.values, b.values) {
				return true
			}
			switch {
			case a.bottom:
				return true
			case b.bottom:
				return false
			default:
				return finiteMustMapLessOrEq(a.values, b.values, elem.LessOrEq)
			}
		},
		Join: func(a, b MustMapLane[K, V]) MustMapLane[K, V] {
			if a.bottom {
				return b
			}
			if b.bottom {
				return a
			}
			if sameMapValue(a.values, b.values) {
				return a
			}
			return MustMapLane[K, V]{values: finiteMustMapJoin(a.values, b.values, elem.Join)}
		},
		Widen: func(prev, next MustMapLane[K, V]) MustMapLane[K, V] {
			if prev.bottom {
				return next
			}
			if next.bottom {
				return prev
			}
			if sameMapValue(prev.values, next.values) {
				return prev
			}
			return MustMapLane[K, V]{values: finiteMustMapJoin(prev.values, next.values, elem.Widen)}
		},
		Narrow: func(prev, next MustMapLane[K, V]) MustMapLane[K, V] {
			if elem.Narrow == nil {
				return prev
			}
			if prev.bottom || next.bottom {
				return prev
			}
			if sameMapValue(prev.values, next.values) {
				return prev
			}
			out := make(map[K]V, len(prev.values)+len(next.values))
			for key, prevValue := range prev.values {
				nextValue, ok := next.values[key]
				if !ok {
					nextValue = elem.Top()
				}
				narrowed := elem.Narrow(prevValue, nextValue)
				if !elem.Equal(narrowed, elem.Top()) {
					out[key] = narrowed
				}
			}
			for key, nextValue := range next.values {
				if _, ok := prev.values[key]; ok {
					continue
				}
				narrowed := elem.Narrow(elem.Top(), nextValue)
				if !elem.Equal(narrowed, elem.Top()) {
					out[key] = narrowed
				}
			}
			return MustMapLane[K, V]{values: out}
		},
	}
	if elem.Meet != nil {
		l.Meet = func(a, b MustMapLane[K, V]) MustMapLane[K, V] {
			if a.bottom || b.bottom {
				return MustMapBottom[K, V]()
			}
			if sameMapValue(a.values, b.values) {
				return a
			}
			return MustMapLane[K, V]{values: finiteMustMapMeet(a.values, b.values, elem.Meet)}
		}
	}
	return l
}

// MustSetLane is a finite must-fact set carrier.
//
// Bottom is explicit. Top is the reachable empty set. The order is
// information-must order: more required facts is lower, so joins and widens are
// finite set intersection. Values sets are persistent by convention once
// published to the lattice; operations may return an input carrier for
// identity/equality cases.
type MustSetLane[T comparable] struct {
	bottom bool
	values map[T]struct{}
}

// MustSetBottom builds an explicit-bottom must-set lane.
func MustSetBottom[T comparable]() MustSetLane[T] {
	return MustSetLane[T]{bottom: true}
}

// MustSetValues builds a reachable must-set lane over values.
func MustSetValues[T comparable](values map[T]struct{}) MustSetLane[T] {
	return MustSetLane[T]{values: values}
}

// Bottom reports whether this lane is explicit bottom.
func (l MustSetLane[T]) Bottom() bool {
	return l.bottom
}

// Values returns the lane's finite values set.
func (l MustSetLane[T]) Values() map[T]struct{} {
	return l.values
}

// Clone returns an independent copy of the finite values set.
func (l MustSetLane[T]) Clone() MustSetLane[T] {
	return MustSetLane[T]{
		bottom: l.bottom,
		values: cloneSet(l.values),
	}
}

// MustSet builds a finite must-set lattice.
func MustSet[T comparable]() lattice.Lattice[MustSetLane[T]] {
	sameSetValue := func(a, b map[T]struct{}) bool {
		return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
	}
	return lattice.Lattice[MustSetLane[T]]{
		Bottom: func() MustSetLane[T] {
			return MustSetBottom[T]()
		},
		Top: func() MustSetLane[T] {
			return MustSetLane[T]{}
		},
		Equal: func(a, b MustSetLane[T]) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			if sameSetValue(a.values, b.values) {
				return true
			}
			return finiteSetEqual(a.values, b.values)
		},
		Same: func(a, b MustSetLane[T]) bool {
			return a.bottom == b.bottom && sameSetValue(a.values, b.values)
		},
		LessOrEq: func(a, b MustSetLane[T]) bool {
			if a.bottom == b.bottom && sameSetValue(a.values, b.values) {
				return true
			}
			switch {
			case a.bottom:
				return true
			case b.bottom:
				return false
			default:
				return finiteMustSetLessOrEq(a.values, b.values)
			}
		},
		Join: func(a, b MustSetLane[T]) MustSetLane[T] {
			if a.bottom {
				return b
			}
			if b.bottom {
				return a
			}
			if sameSetValue(a.values, b.values) {
				return a
			}
			return MustSetLane[T]{values: finiteSetIntersection(a.values, b.values)}
		},
		Widen: func(prev, next MustSetLane[T]) MustSetLane[T] {
			if prev.bottom {
				return next
			}
			if next.bottom {
				return prev
			}
			if sameSetValue(prev.values, next.values) {
				return prev
			}
			return MustSetLane[T]{values: finiteSetIntersection(prev.values, next.values)}
		},
		Meet: func(a, b MustSetLane[T]) MustSetLane[T] {
			if a.bottom || b.bottom {
				return MustSetBottom[T]()
			}
			if sameSetValue(a.values, b.values) {
				return a
			}
			return MustSetLane[T]{values: finiteSetUnion(a.values, b.values)}
		},
		Narrow: func(prev, next MustSetLane[T]) MustSetLane[T] {
			return prev
		},
	}
}

func cloneMap[K comparable, V any](in map[K]V) map[K]V {
	return mapedit.Clone(in)
}

func finiteMapEqual[K comparable, V any](
	a map[K]V,
	b map[K]V,
	equal func(V, V) bool,
) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || !equal(av, bv) {
			return false
		}
	}
	return true
}

func finiteMustMapLessOrEq[K comparable, V any](
	a map[K]V,
	b map[K]V,
	lessOrEq func(V, V) bool,
) bool {
	for k, bv := range b {
		av, ok := a[k]
		if !ok || !lessOrEq(av, bv) {
			return false
		}
	}
	return true
}

func finiteMustMapJoin[K comparable, V any](
	a map[K]V,
	b map[K]V,
	join func(V, V) V,
) map[K]V {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	if len(a) <= len(b) {
		out := make(map[K]V, len(a))
		for k, av := range a {
			if bv, ok := b[k]; ok {
				out[k] = join(av, bv)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	out := make(map[K]V, len(b))
	for k, bv := range b {
		if av, ok := a[k]; ok {
			out[k] = join(av, bv)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func finiteMustMapMeet[K comparable, V any](
	a map[K]V,
	b map[K]V,
	meet func(V, V) V,
) map[K]V {
	if len(a) == 0 {
		return cloneMap(b)
	}
	if len(b) == 0 {
		return cloneMap(a)
	}
	out := make(map[K]V, len(a)+len(b))
	for k, av := range a {
		if bv, ok := b[k]; ok {
			out[k] = meet(av, bv)
		} else {
			out[k] = av
		}
	}
	for k, bv := range b {
		if _, ok := a[k]; !ok {
			out[k] = bv
		}
	}
	return out
}

func cloneSet[T comparable](in map[T]struct{}) map[T]struct{} {
	if len(in) == 0 {
		return nil
	}
	out := make(map[T]struct{}, len(in))
	for k := range in {
		out[k] = struct{}{}
	}
	return out
}

func finiteSetEqual[T comparable](a, b map[T]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for v := range a {
		if _, ok := b[v]; !ok {
			return false
		}
	}
	return true
}

func finiteMustSetLessOrEq[T comparable](a, b map[T]struct{}) bool {
	for v := range b {
		if _, ok := a[v]; !ok {
			return false
		}
	}
	return true
}

func finiteSetIntersection[T comparable](a, b map[T]struct{}) map[T]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	if len(b) < len(a) {
		a, b = b, a
	}
	out := make(map[T]struct{}, len(a))
	for v := range a {
		if _, ok := b[v]; ok {
			out[v] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func finiteSetUnion[T comparable](a, b map[T]struct{}) map[T]struct{} {
	if len(a) == 0 {
		return cloneSet(b)
	}
	if len(b) == 0 {
		return cloneSet(a)
	}
	out := make(map[T]struct{}, len(a)+len(b))
	for v := range a {
		out[v] = struct{}{}
	}
	for v := range b {
		out[v] = struct{}{}
	}
	return out
}
