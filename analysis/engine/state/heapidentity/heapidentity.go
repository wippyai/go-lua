// Package heapidentity owns identity-keyed heap table object semantics.
package heapidentity

import (
	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/lattice/lift"
	"github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/internal/registrycache"
)

type TableObject struct {
	bottom bool

	root                 product.Value
	staticMembers        map[keyspace.Key]product.Value
	dynamicIndexFacts    map[dynamicindex.Key]dynamicindex.Fact
	dynamicIndexFactsTop bool
}

// TableObjectConfig carries finite heap table object facts.
type TableObjectConfig struct {
	Root              product.Value
	StaticMembers     map[keyspace.Key]product.Value
	DynamicIndexFacts map[dynamicindex.Key]dynamicindex.Fact
}

var objectDomainCache registrycache.Cache[lattice.Lattice[TableObject]]
var mapDomainCache registrycache.Cache[lattice.Lattice[map[identity.ID]TableObject]]

// NewTableObject creates a finite heap table object and takes defensive copies
// of map-backed lanes.
func NewTableObject(config TableObjectConfig) TableObject {
	return TableObject{
		root:              config.Root,
		staticMembers:     clonePathMap(config.StaticMembers),
		dynamicIndexFacts: dynamicindex.CloneMap(config.DynamicIndexFacts),
	}
}

// NewOwnedStaticTableObject creates a finite heap table object and takes
// ownership of staticMembers. Callers must not mutate staticMembers after this
// call. Use NewTableObject unless the map was freshly built for this object.
func NewOwnedStaticTableObject(root product.Value, staticMembers map[keyspace.Key]product.Value) TableObject {
	if len(staticMembers) == 0 {
		staticMembers = nil
	}
	return TableObject{
		root:          root,
		staticMembers: staticMembers,
	}
}

// Root returns the object's root product value.
func (o TableObject) Root() product.Value { return o.root }

// StaticMember reads a proven static member fact.
func (o TableObject) StaticMember(key keyspace.Key) (product.Value, bool) {
	value, ok := o.staticMembers[key]
	return value, ok
}

// StaticMembers returns a defensive copy of finite static member facts.
func (o TableObject) StaticMembers() map[keyspace.Key]product.Value {
	return clonePathMap(o.staticMembers)
}

// WithoutStaticMemberSubtree returns an object with any static-member fact at
// prefix or below removed.
func (o TableObject) WithoutStaticMemberSubtree(ks *keyspace.KeySpace, prefix []segment.Segment) (TableObject, bool) {
	return o.withoutStaticMembersMatching(ks, prefix, false)
}

// WithoutStaticMemberDescendants returns an object with static-member facts
// strictly below prefix removed while preserving a fact exactly at prefix.
func (o TableObject) WithoutStaticMemberDescendants(ks *keyspace.KeySpace, prefix []segment.Segment) (TableObject, bool) {
	return o.withoutStaticMembersMatching(ks, prefix, true)
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
func StaticMemberSuffixKey(ks *keyspace.KeySpace, segments []segment.Segment) (keyspace.Key, bool) {
	return ks.FromRootlessSuffix(segments)
}

// FieldCanonicalStaticMemberSuffixKey returns the equivalent rootless suffix key
// with static string indexes rewritten to field spelling. It returns false when
// no string-index segment is present.
func FieldCanonicalStaticMemberSuffixKey(ks *keyspace.KeySpace, segments []segment.Segment) (keyspace.Key, bool) {
	canonical, changed := address.FieldCanonicalSegments(segments)
	if !changed {
		return keyspace.Key{}, false
	}
	return ks.FromRootlessSuffix(canonical)
}

func (o TableObject) withoutStaticMembersMatching(ks *keyspace.KeySpace, prefix []segment.Segment, descendantsOnly bool) (TableObject, bool) {
	if o.bottom || len(o.staticMembers) == 0 {
		return o, false
	}
	out := CloneObject(o)
	changed := false
	for key := range out.staticMembers {
		segments, ok := ks.SuffixSegments(key)
		if !ok {
			continue
		}
		matches := address.SegmentsHasPrefix(segments, prefix)
		if descendantsOnly {
			matches = address.SegmentsHasStrictPrefix(segments, prefix)
		}
		if matches {
			delete(out.staticMembers, key)
			changed = true
		}
	}
	if !changed {
		return o, false
	}
	if len(out.staticMembers) == 0 {
		out.staticMembers = nil
	}
	return out, true
}

// Rekey re-interns the rootless static-member keys and dynamic-index table keys
// from one keyspace into another so an object built under one analysis's keyspace
// can be consumed under another's. It is a no-op when from == to or either
// keyspace is nil.
func (o TableObject) Rekey(from, to *keyspace.KeySpace) TableObject {
	if from == nil || to == nil || from == to || o.bottom {
		return o
	}
	if len(o.staticMembers) == 0 && len(o.dynamicIndexFacts) == 0 {
		return o
	}
	out := CloneObject(o)
	if len(o.staticMembers) != 0 {
		rekeyed := make(map[keyspace.Key]product.Value, len(o.staticMembers))
		for key, value := range o.staticMembers {
			segments, ok := from.SuffixSegments(key)
			if !ok {
				continue
			}
			next, ok := to.FromRootlessSuffix(segments)
			if !ok {
				continue
			}
			rekeyed[next] = value
		}
		if len(rekeyed) == 0 {
			rekeyed = nil
		}
		out.staticMembers = rekeyed
	}
	return out
}

func ObjectDomain(reg *axis.Registry) lattice.Lattice[TableObject] {
	return objectDomainCache.Get(reg, func() lattice.Lattice[TableObject] {
		valueDomain := product.Domain(reg)
		staticDomain := lift.MustMap[keyspace.Key, product.Value](valueDomain)
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
	})
}

func MapDomain(reg *axis.Registry) lattice.Lattice[map[identity.ID]TableObject] {
	return mapDomainCache.Get(reg, func() lattice.Lattice[map[identity.ID]TableObject] {
		return lift.Map[identity.ID, TableObject](ObjectDomain(reg))
	})
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
		out[k] = v
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
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil, true
	}
	return out, true
}

func staticLane(object TableObject) lift.MustMapLane[keyspace.Key, product.Value] {
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
	static lift.MustMapLane[keyspace.Key, product.Value],
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

func clonePathMap(in map[keyspace.Key]product.Value) map[keyspace.Key]product.Value {
	if len(in) == 0 {
		return nil
	}
	out := make(map[keyspace.Key]product.Value, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
