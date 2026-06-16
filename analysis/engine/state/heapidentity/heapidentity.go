// Package heapidentity owns identity-keyed heap table object semantics.
package heapidentity

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
)

type TableObject struct {
	bottom bool

	root                 product.Value
	staticMembers        map[pathdom.PathKey]product.Value
	dynamicIndexFacts    map[dynamicindex.Key]dynamicindex.Fact
	dynamicIndexFactsTop bool
}

// TableObjectConfig carries finite heap table object facts.
type TableObjectConfig struct {
	Root              product.Value
	StaticMembers     map[pathdom.PathKey]product.Value
	DynamicIndexFacts map[dynamicindex.Key]dynamicindex.Fact
}

// NewTableObject creates a finite heap table object and takes defensive copies
// of map-backed lanes.
func NewTableObject(config TableObjectConfig) TableObject {
	return TableObject{
		root:              config.Root,
		staticMembers:     clonePathMap(config.StaticMembers),
		dynamicIndexFacts: dynamicindex.CloneMap(config.DynamicIndexFacts),
	}
}

// Root returns the object's root product value.
func (o TableObject) Root() product.Value { return o.root }

// StaticMember reads a proven static member fact.
func (o TableObject) StaticMember(key pathdom.PathKey) (product.Value, bool) {
	value, ok := o.staticMembers[key]
	return value, ok
}

// StaticMembers returns a defensive copy of finite static member facts.
func (o TableObject) StaticMembers() map[pathdom.PathKey]product.Value {
	return clonePathMap(o.staticMembers)
}

// DynamicIndexFact reads a finite dynamic-index fact.
func (o TableObject) DynamicIndexFact(key dynamicindex.Key) (dynamicindex.Fact, bool) {
	value, ok := o.dynamicIndexFacts[key]
	return value, ok
}

// DynamicIndexFacts returns a defensive copy of finite dynamic-index facts.
func (o TableObject) DynamicIndexFacts() map[dynamicindex.Key]dynamicindex.Fact {
	return dynamicindex.CloneMap(o.dynamicIndexFacts)
}

// StaticMemberSuffixKey returns the canonical heap static-member key for a
// relative suffix. It intentionally encodes only the suffix segments so
// rootless member facts do not collapse to an empty path key.
func StaticMemberSuffixKey(segments []segment.Segment) (pathdom.PathKey, bool) {
	return address.RelativeStaticMemberSuffixKey(segments)
}

func ObjectDomain(reg *axis.Registry) lattice.Lattice[TableObject] {
	valueDomain := product.Domain(reg)
	staticDomain := lift.MustMap[pathdom.PathKey, product.Value](valueDomain)
	dynamicDomain := dynamicindex.MapDomain(reg)
	return lattice.Lattice[TableObject]{
		Bottom: func() TableObject { return BottomObject(reg) },
		Top:    TopObject,
		Equal: func(a, b TableObject) bool {
			if a.bottom || b.bottom {
				return a.bottom && b.bottom
			}
			return valueDomain.Equal(a.root, b.root) &&
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
				return valueDomain.LessOrEq(a.root, b.root) &&
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
				valueDomain.Join(a.root, b.root),
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
				valueDomain.Widen(prev.root, next.root),
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
		root:   product.Bottom(reg),
	}
}

func TopObject() TableObject {
	return TableObject{
		root:                 product.Top(),
		dynamicIndexFactsTop: true,
	}
}

func CloneObject(in TableObject) TableObject {
	out := in
	out.staticMembers = clonePathMap(in.staticMembers)
	out.dynamicIndexFacts = dynamicindex.CloneMap(in.dynamicIndexFacts)
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

func staticLane(object TableObject) lift.MustMapLane[pathdom.PathKey, product.Value] {
	return lift.MustMapValues(object.staticMembers)
}

func dynamicLane(
	object TableObject,
	domain lattice.Lattice[map[dynamicindex.Key]dynamicindex.Fact],
) map[dynamicindex.Key]dynamicindex.Fact {
	if object.dynamicIndexFactsTop {
		return domain.Top()
	}
	return object.dynamicIndexFacts
}

func objectFromLanes(
	root product.Value,
	static lift.MustMapLane[pathdom.PathKey, product.Value],
	dynamic map[dynamicindex.Key]dynamicindex.Fact,
	dynamicDomain lattice.Lattice[map[dynamicindex.Key]dynamicindex.Fact],
) TableObject {
	object := TableObject{
		root:          root,
		staticMembers: clonePathMap(static.Values()),
	}
	if dynamicDomain.Equal(dynamic, dynamicDomain.Top()) {
		object.dynamicIndexFactsTop = true
	} else {
		object.dynamicIndexFacts = dynamicindex.CloneMap(dynamic)
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
