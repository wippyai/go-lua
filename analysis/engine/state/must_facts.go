package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func (s State) ReadPathStaticMember(pathKey pathdom.PathKey) (product.Value, bool) {
	if pathKey == "" || s.pathStaticMembersBottom {
		return product.Value{}, false
	}
	v, ok := s.pathStaticMembers[pathKey]
	return v, ok
}

func (s State) WritePathStaticMember(pathKey pathdom.PathKey, value product.Value) State {
	if pathKey == "" {
		return s
	}
	members := clonePathMap(s.pathStaticMembers)
	if members == nil {
		members = make(map[pathdom.PathKey]product.Value, 1)
	}
	members[pathKey] = value
	out := s.reachable()
	out.pathStaticMembers = members
	return out
}

type mustMapLane[K comparable, V any] struct {
	bottom bool
	values map[K]V
}

type mustSetLane[T comparable] struct {
	bottom bool
	values map[T]struct{}
}

func mustMapDomain[K comparable, V any](elem lattice.Lattice[V]) lattice.Lattice[mustMapLane[K, V]] {
	return lattice.Lattice[mustMapLane[K, V]]{
		Bottom: func() mustMapLane[K, V] {
			return mustMapLane[K, V]{bottom: true}
		},
		Top: func() mustMapLane[K, V] {
			return mustMapLane[K, V]{}
		},
		Equal: func(a, b mustMapLane[K, V]) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			return finiteMapEqual(a.values, b.values, elem.Equal)
		},
		LessOrEq: func(a, b mustMapLane[K, V]) bool {
			switch {
			case a.bottom:
				return true
			case b.bottom:
				return false
			default:
				return finiteMustMapLessOrEq(a.values, b.values, elem.LessOrEq)
			}
		},
		Join: func(a, b mustMapLane[K, V]) mustMapLane[K, V] {
			if a.bottom {
				return b
			}
			if b.bottom {
				return a
			}
			return mustMapLane[K, V]{values: finiteMustMapJoin(a.values, b.values, elem.Join)}
		},
		Widen: func(prev, next mustMapLane[K, V]) mustMapLane[K, V] {
			if prev.bottom {
				return next
			}
			if next.bottom {
				return prev
			}
			return mustMapLane[K, V]{values: finiteMustMapJoin(prev.values, next.values, elem.Widen)}
		},
	}
}

func mustSetDomain[T comparable]() lattice.Lattice[mustSetLane[T]] {
	return lattice.Lattice[mustSetLane[T]]{
		Bottom: func() mustSetLane[T] {
			return mustSetLane[T]{bottom: true}
		},
		Top: func() mustSetLane[T] {
			return mustSetLane[T]{}
		},
		Equal: func(a, b mustSetLane[T]) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			return finiteSetEqual(a.values, b.values)
		},
		LessOrEq: func(a, b mustSetLane[T]) bool {
			switch {
			case a.bottom:
				return true
			case b.bottom:
				return false
			default:
				return finiteMustSetLessOrEq(a.values, b.values)
			}
		},
		Join: func(a, b mustSetLane[T]) mustSetLane[T] {
			if a.bottom {
				return b
			}
			if b.bottom {
				return a
			}
			return mustSetLane[T]{values: finiteSetIntersection(a.values, b.values)}
		},
		Widen: func(prev, next mustSetLane[T]) mustSetLane[T] {
			if prev.bottom {
				return next
			}
			if next.bottom {
				return prev
			}
			return mustSetLane[T]{values: finiteSetIntersection(prev.values, next.values)}
		},
	}
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
