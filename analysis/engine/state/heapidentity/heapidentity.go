// Package heapidentity owns identity-keyed heap table object semantics.
package heapidentity

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/dynamicindex"
)

type TableObject struct {
	bottom bool

	Root                 product.Value
	StaticMembers        map[pathdom.PathKey]product.Value
	DynamicIndexFacts    map[dynamicindex.Key]dynamicindex.Fact
	dynamicIndexFactsTop bool
}

func ObjectDomain(reg *axis.Registry) lattice.Lattice[TableObject] {
	valueDomain := product.Domain(reg)
	staticDomain := mustMapDomain[pathdom.PathKey, product.Value](valueDomain)
	dynamicDomain := dynamicindex.MapDomain(reg)
	return lattice.Lattice[TableObject]{
		Bottom: func() TableObject { return BottomObject(reg) },
		Top:    TopObject,
		Equal: func(a, b TableObject) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			return valueDomain.Equal(a.Root, b.Root) &&
				staticDomain.Equal(staticLane(a), staticLane(b)) &&
				dynamicDomain.Equal(dynamicLane(a, dynamicDomain), dynamicLane(b, dynamicDomain))
		},
		LessOrEq: func(a, b TableObject) bool {
			switch {
			case a.bottom:
				return true
			case b.bottom:
				return false
			default:
				return valueDomain.LessOrEq(a.Root, b.Root) &&
					staticDomain.LessOrEq(staticLane(a), staticLane(b)) &&
					dynamicDomain.LessOrEq(dynamicLane(a, dynamicDomain), dynamicLane(b, dynamicDomain))
			}
		},
		Join: func(a, b TableObject) TableObject {
			if a.bottom {
				return CloneObject(b)
			}
			if b.bottom {
				return CloneObject(a)
			}
			static := staticDomain.Join(staticLane(a), staticLane(b))
			dynamic := dynamicDomain.Join(dynamicLane(a, dynamicDomain), dynamicLane(b, dynamicDomain))
			return objectFromLanes(
				valueDomain.Join(a.Root, b.Root),
				static,
				dynamic,
				dynamicDomain,
			)
		},
		Widen: func(prev, next TableObject) TableObject {
			if prev.bottom {
				return CloneObject(next)
			}
			if next.bottom {
				return CloneObject(prev)
			}
			static := staticDomain.Widen(staticLane(prev), staticLane(next))
			dynamic := dynamicDomain.Widen(dynamicLane(prev, dynamicDomain), dynamicLane(next, dynamicDomain))
			return objectFromLanes(
				valueDomain.Widen(prev.Root, next.Root),
				static,
				dynamic,
				dynamicDomain,
			)
		},
	}
}

func MapDomain(reg *axis.Registry) lattice.Lattice[map[identity.ID]TableObject] {
	return lift.Map[identity.ID, TableObject](ObjectDomain(reg))
}

func BottomObject(reg *axis.Registry) TableObject {
	return TableObject{
		bottom: true,
		Root:   product.Bottom(reg),
	}
}

func TopObject() TableObject {
	return TableObject{
		Root:                 product.Top(),
		dynamicIndexFactsTop: true,
	}
}

func CloneObject(in TableObject) TableObject {
	out := in
	out.StaticMembers = clonePathMap(in.StaticMembers)
	out.DynamicIndexFacts = dynamicindex.CloneMap(in.DynamicIndexFacts)
	return out
}

func CloneMap(in map[identity.ID]TableObject) map[identity.ID]TableObject {
	if len(in) == 0 {
		return nil
	}
	out := make(map[identity.ID]TableObject, len(in))
	for k, v := range in {
		out[k] = CloneObject(v)
	}
	return out
}

func DeleteEntry(
	in map[identity.ID]TableObject,
	id identity.ID,
) (map[identity.ID]TableObject, bool) {
	if _, ok := in[id]; !ok {
		return in, false
	}
	out := make(map[identity.ID]TableObject, len(in)-1)
	for k, v := range in {
		if k != id {
			out[k] = CloneObject(v)
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

func staticLane(object TableObject) mustMapLane[pathdom.PathKey, product.Value] {
	return mustMapLane[pathdom.PathKey, product.Value]{values: object.StaticMembers}
}

func dynamicLane(
	object TableObject,
	domain lattice.Lattice[map[dynamicindex.Key]dynamicindex.Fact],
) map[dynamicindex.Key]dynamicindex.Fact {
	if object.dynamicIndexFactsTop {
		return domain.Top()
	}
	return object.DynamicIndexFacts
}

func objectFromLanes(
	root product.Value,
	static mustMapLane[pathdom.PathKey, product.Value],
	dynamic map[dynamicindex.Key]dynamicindex.Fact,
	dynamicDomain lattice.Lattice[map[dynamicindex.Key]dynamicindex.Fact],
) TableObject {
	object := TableObject{
		Root:          root,
		StaticMembers: clonePathMap(static.values),
	}
	if dynamicDomain.Equal(dynamic, dynamicDomain.Top()) {
		object.dynamicIndexFactsTop = true
	} else {
		object.DynamicIndexFacts = dynamicindex.CloneMap(dynamic)
	}
	return object
}

func clonePathMap(in map[pathdom.PathKey]product.Value) map[pathdom.PathKey]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey]product.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type mustMapLane[K comparable, V any] struct {
	bottom bool
	values map[K]V
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
