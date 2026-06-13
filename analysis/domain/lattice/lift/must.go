package lift

import "github.com/wippyai/go-lua/analysis/domain/lattice"

// MustMapLane is a finite must-fact map carrier.
//
// Bottom is explicit. Top is the reachable empty map. The order is
// information-must order: more required keys is lower, so joins and widens keep
// only keys present in both operands and combine their values with the element
// lattice operation.
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
	return lattice.Lattice[MustMapLane[K, V]]{
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
			return finiteMapEqual(a.values, b.values, elem.Equal)
		},
		LessOrEq: func(a, b MustMapLane[K, V]) bool {
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
				return b.Clone()
			}
			if b.bottom {
				return a.Clone()
			}
			return MustMapLane[K, V]{values: finiteMustMapJoin(a.values, b.values, elem.Join)}
		},
		Widen: func(prev, next MustMapLane[K, V]) MustMapLane[K, V] {
			if prev.bottom {
				return next.Clone()
			}
			if next.bottom {
				return prev.Clone()
			}
			return MustMapLane[K, V]{values: finiteMustMapJoin(prev.values, next.values, elem.Widen)}
		},
	}
}

// MustSetLane is a finite must-fact set carrier.
//
// Bottom is explicit. Top is the reachable empty set. The order is
// information-must order: more required facts is lower, so joins and widens are
// finite set intersection.
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
			return finiteSetEqual(a.values, b.values)
		},
		LessOrEq: func(a, b MustSetLane[T]) bool {
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
				return b.Clone()
			}
			if b.bottom {
				return a.Clone()
			}
			return MustSetLane[T]{values: finiteSetIntersection(a.values, b.values)}
		},
		Widen: func(prev, next MustSetLane[T]) MustSetLane[T] {
			if prev.bottom {
				return next.Clone()
			}
			if next.bottom {
				return prev.Clone()
			}
			return MustSetLane[T]{values: finiteSetIntersection(prev.values, next.values)}
		},
	}
}

func cloneMap[K comparable, V any](in map[K]V) map[K]V {
	if len(in) == 0 {
		return nil
	}
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
	out := make(map[K]V)
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
	out := make(map[T]struct{})
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
