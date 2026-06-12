package state

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

type HeapTableObject struct {
	bottom bool

	Root                 product.Value
	StaticMembers        map[pathdom.PathKey]product.Value
	DynamicIndexFacts    map[DynamicIndexKey]DynamicIndexFact
	dynamicIndexFactsTop bool
}

func (s State) ReadHeapTableObject(reg *axis.Registry, id identity.ID) HeapTableObject {
	if id == (identity.ID{}) {
		return heapTableObjectBottom(reg)
	}
	if s.heapTableIdentityTop {
		return heapTableObjectTop()
	}
	if object, ok := s.heapTableIdentity[id]; ok {
		return copyHeapTableObject(object)
	}
	return heapTableObjectBottom(reg)
}

func (s State) WriteHeapTableObject(reg *axis.Registry, id identity.ID, object HeapTableObject) State {
	if id == (identity.ID{}) {
		return s
	}
	if s.heapTableIdentityTop {
		panic("state: cannot finite-write heap table object into top heap-identity lane")
	}
	domain := heapTableObjectDomain(reg)
	if domain.Equal(object, domain.Bottom()) {
		objects, changed := deleteHeapTableObjectEntry(s.heapTableIdentity, id)
		if !changed {
			return s
		}
		out := s.reachable()
		out.heapTableIdentity = objects
		return out
	}
	objects := cloneHeapTableObjectMap(s.heapTableIdentity)
	if objects == nil {
		objects = make(map[identity.ID]HeapTableObject, 1)
	}
	objects[id] = copyHeapTableObject(object)
	out := s.reachable()
	out.heapTableIdentity = objects
	return out
}

func heapTableIdentityMapDomain(reg *axis.Registry) lattice.Lattice[map[identity.ID]HeapTableObject] {
	return lift.Map[identity.ID, HeapTableObject](heapTableObjectDomain(reg))
}

func heapTableObjectDomain(reg *axis.Registry) lattice.Lattice[HeapTableObject] {
	valueDomain := product.Domain(reg)
	staticDomain := mustMapDomain[pathdom.PathKey, product.Value](valueDomain)
	dynamicDomain := dynamicIndexMapDomain(reg)
	return lattice.Lattice[HeapTableObject]{
		Bottom: func() HeapTableObject { return heapTableObjectBottom(reg) },
		Top:    heapTableObjectTop,
		Equal: func(a, b HeapTableObject) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			return valueDomain.Equal(a.Root, b.Root) &&
				staticDomain.Equal(heapStaticLane(a), heapStaticLane(b)) &&
				dynamicDomain.Equal(heapDynamicLane(a, dynamicDomain), heapDynamicLane(b, dynamicDomain))
		},
		LessOrEq: func(a, b HeapTableObject) bool {
			switch {
			case a.bottom:
				return true
			case b.bottom:
				return false
			default:
				return valueDomain.LessOrEq(a.Root, b.Root) &&
					staticDomain.LessOrEq(heapStaticLane(a), heapStaticLane(b)) &&
					dynamicDomain.LessOrEq(heapDynamicLane(a, dynamicDomain), heapDynamicLane(b, dynamicDomain))
			}
		},
		Join: func(a, b HeapTableObject) HeapTableObject {
			if a.bottom {
				return copyHeapTableObject(b)
			}
			if b.bottom {
				return copyHeapTableObject(a)
			}
			static := staticDomain.Join(heapStaticLane(a), heapStaticLane(b))
			dynamic := dynamicDomain.Join(heapDynamicLane(a, dynamicDomain), heapDynamicLane(b, dynamicDomain))
			return heapTableObjectFromLanes(
				valueDomain.Join(a.Root, b.Root),
				static,
				dynamic,
				dynamicDomain,
			)
		},
		Widen: func(prev, next HeapTableObject) HeapTableObject {
			if prev.bottom {
				return copyHeapTableObject(next)
			}
			if next.bottom {
				return copyHeapTableObject(prev)
			}
			static := staticDomain.Widen(heapStaticLane(prev), heapStaticLane(next))
			dynamic := dynamicDomain.Widen(heapDynamicLane(prev, dynamicDomain), heapDynamicLane(next, dynamicDomain))
			return heapTableObjectFromLanes(
				valueDomain.Widen(prev.Root, next.Root),
				static,
				dynamic,
				dynamicDomain,
			)
		},
	}
}

func heapTableObjectBottom(reg *axis.Registry) HeapTableObject {
	return HeapTableObject{
		bottom: true,
		Root:   product.Bottom(reg),
	}
}

func heapTableObjectTop() HeapTableObject {
	return HeapTableObject{
		Root:                 product.Top(),
		dynamicIndexFactsTop: true,
	}
}

func heapStaticLane(object HeapTableObject) mustMapLane[pathdom.PathKey, product.Value] {
	return mustMapLane[pathdom.PathKey, product.Value]{values: object.StaticMembers}
}

func heapDynamicLane(
	object HeapTableObject,
	domain lattice.Lattice[map[DynamicIndexKey]DynamicIndexFact],
) map[DynamicIndexKey]DynamicIndexFact {
	if object.dynamicIndexFactsTop {
		return domain.Top()
	}
	return object.DynamicIndexFacts
}

func heapTableObjectFromLanes(
	root product.Value,
	static mustMapLane[pathdom.PathKey, product.Value],
	dynamic map[DynamicIndexKey]DynamicIndexFact,
	dynamicDomain lattice.Lattice[map[DynamicIndexKey]DynamicIndexFact],
) HeapTableObject {
	object := HeapTableObject{
		Root:          root,
		StaticMembers: clonePathMap(static.values),
	}
	if dynamicDomain.Equal(dynamic, dynamicDomain.Top()) {
		object.dynamicIndexFactsTop = true
	} else {
		object.DynamicIndexFacts = cloneDynamicIndexMap(dynamic)
	}
	return object
}

func cloneHeapTableObjectMap(in map[identity.ID]HeapTableObject) map[identity.ID]HeapTableObject {
	if len(in) == 0 {
		return nil
	}
	out := make(map[identity.ID]HeapTableObject, len(in))
	for k, v := range in {
		out[k] = copyHeapTableObject(v)
	}
	return out
}

func copyHeapTableObject(in HeapTableObject) HeapTableObject {
	out := in
	out.StaticMembers = clonePathMap(in.StaticMembers)
	out.DynamicIndexFacts = cloneDynamicIndexMap(in.DynamicIndexFacts)
	return out
}

func deleteHeapTableObjectEntry(
	in map[identity.ID]HeapTableObject,
	id identity.ID,
) (map[identity.ID]HeapTableObject, bool) {
	if _, ok := in[id]; !ok {
		return in, false
	}
	out := make(map[identity.ID]HeapTableObject, len(in)-1)
	for k, v := range in {
		if k != id {
			out[k] = copyHeapTableObject(v)
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}
