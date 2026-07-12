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
	stableShape          bool
	prefixStableShape    bool
}

// TableObjectConfig carries finite heap table object facts.
type TableObjectConfig struct {
	Root              product.Value
	StaticMembers     map[keyspace.Key]product.Value
	DynamicIndexFacts map[dynamicindex.Key]dynamicindex.Fact
	StableShape       bool
	PrefixStableShape bool
}

var objectDomainCache registrycache.Cache[lattice.Lattice[TableObject]]
var mapDomainCache registrycache.Cache[lattice.Lattice[map[identity.ID]TableObject]]

// NewTableObject creates a finite heap table object and takes defensive copies
// of map-backed lanes.
func NewTableObject(config TableObjectConfig) TableObject {
	prefixStableShape := config.PrefixStableShape || config.StableShape
	return TableObject{
		root:              config.Root,
		staticMembers:     clonePathMap(config.StaticMembers),
		dynamicIndexFacts: dynamicindex.CloneMap(config.DynamicIndexFacts),
		stableShape:       config.StableShape,
		prefixStableShape: prefixStableShape,
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
		root:              root,
		staticMembers:     staticMembers,
		prefixStableShape: true,
	}
}

// Root returns the object's root product value.
func (o TableObject) Root() product.Value { return o.root }

// MapValues rewrites every product value stored by the object while preserving
// its structural and lattice metadata. The returned object never aliases a
// mutable member map with o when any value changes.
func (o TableObject) MapValues(reg *axis.Registry, mapValue func(product.Value) product.Value) TableObject {
	if o.bottom || reg == nil || mapValue == nil {
		return o
	}
	changed := false
	staticCloned := false
	dynamicCloned := false
	root := mapValue(o.root)
	changed = changed || !product.Equal(reg, root, o.root)
	staticMembers := o.staticMembers
	for key, value := range o.staticMembers {
		next := mapValue(value)
		if product.Equal(reg, next, value) {
			continue
		}
		if !staticCloned {
			staticMembers = clonePathMap(o.staticMembers)
			staticCloned = true
		}
		staticMembers[key] = next
		changed = true
	}
	dynamicFacts := o.dynamicIndexFacts
	for key, fact := range o.dynamicIndexFacts {
		next := fact
		next.KeyValue = mapValue(fact.KeyValue)
		next.Value = mapValue(fact.Value)
		if product.Equal(reg, next.KeyValue, fact.KeyValue) && product.Equal(reg, next.Value, fact.Value) {
			continue
		}
		if !dynamicCloned {
			dynamicFacts = dynamicindex.CloneMap(o.dynamicIndexFacts)
			dynamicCloned = true
		}
		dynamicFacts[key] = next
		changed = true
	}
	if !changed {
		return o
	}
	out := o
	out.root = root
	out.staticMembers = staticMembers
	out.dynamicIndexFacts = dynamicFacts
	return out
}

// IsBottom reports whether this object is the unreachable heap-object value.
func (o TableObject) IsBottom() bool { return o.bottom }

// WithRoot returns object with its root value replaced, preserving finite
// member lanes.
func (o TableObject) WithRoot(root product.Value) TableObject {
	out := o
	out.root = root
	return out
}

// StableShape reports whether this heap object came from a boundary that proved
// no structural mutation can add or remove root fields in its producing world.
func (o TableObject) StableShape() bool { return !o.bottom && o.stableShape }

// PrefixStableShape reports whether this heap object has a monotone static
// field prefix whose field set is represented by its static-member lane.
func (o TableObject) PrefixStableShape() bool { return !o.bottom && o.prefixStableShape }

// WithStableShape returns object with its final-shape marker recorded.
func (o TableObject) WithStableShape() TableObject {
	if o.bottom {
		return o
	}
	out := o
	out.stableShape = true
	out.prefixStableShape = true
	return out
}

// WithPrefixStableShape returns object with its prefix-stable marker recorded.
func (o TableObject) WithPrefixStableShape() TableObject {
	if o.bottom {
		return o
	}
	out := o
	out.prefixStableShape = true
	return out
}

// WithoutStableShape returns object with any final-shape marker removed.
func (o TableObject) WithoutStableShape() TableObject {
	if o.bottom || !o.stableShape {
		return o
	}
	out := o
	out.stableShape = false
	return out
}

// WithoutPrefixStableShape returns object with any prefix-stable marker removed.
func (o TableObject) WithoutPrefixStableShape() TableObject {
	if o.bottom || !o.prefixStableShape {
		return o
	}
	out := o
	out.prefixStableShape = false
	return out
}

// StaticMember reads a proven static member fact.
func (o TableObject) StaticMember(key keyspace.Key) (product.Value, bool) {
	value, ok := o.staticMembers[key]
	return value, ok
}

// StaticMembers returns a defensive copy of finite static member facts.
func (o TableObject) StaticMembers() map[keyspace.Key]product.Value {
	return clonePathMap(o.staticMembers)
}

// WithStaticMember returns an object with a proven static member written at the
// rootless suffix. Static string indexes are mirrored to field spelling so
// `t["id"]` and `t.id` stay equivalent inside the heap identity lane.
func (o TableObject) WithStaticMember(reg *axis.Registry, ks *keyspace.KeySpace, suffix []segment.Segment, value product.Value) (TableObject, bool) {
	return o.withStaticMember(reg, ks, suffix, value, false)
}

// WithJoinedStaticMember returns an object with a static member value joined
// into any existing slot witness. It is for stack-local retype-tolerant writes
// where the fixed slot survives and the field type widens.
func (o TableObject) WithJoinedStaticMember(reg *axis.Registry, ks *keyspace.KeySpace, suffix []segment.Segment, value product.Value) (TableObject, bool) {
	return o.withStaticMember(reg, ks, suffix, value, true)
}

func (o TableObject) withStaticMember(reg *axis.Registry, ks *keyspace.KeySpace, suffix []segment.Segment, value product.Value, joinExisting bool) (TableObject, bool) {
	if o.bottom {
		return o, false
	}
	key, ok := StaticMemberSuffixKey(ks, suffix)
	if !ok {
		return o, false
	}
	valueDomain := product.Domain(reg)
	primaryValue := value
	if joinExisting {
		if existing, ok := o.staticMembers[key]; ok {
			primaryValue = valueDomain.Join(existing, primaryValue)
		}
	}
	canonical, hasCanonical := FieldCanonicalStaticMemberSuffixKey(ks, suffix)
	canonicalValue := primaryValue
	if hasCanonical && joinExisting {
		if existing, ok := o.staticMembers[canonical]; ok {
			canonicalValue = valueDomain.Join(existing, canonicalValue)
		}
	}
	// Summary facts are applied repeatedly while interprocedural proofs
	// converge. Preserve the immutable object when the write is already fully
	// represented; cloning both its member map and the enclosing heap map would
	// otherwise turn a semantic no-op into two persistent-map allocations.
	primaryCurrent, primaryPresent := o.staticMembers[key]
	changed := o.stableShape || !primaryPresent || !valueDomain.Equal(primaryCurrent, primaryValue)
	if hasCanonical {
		canonicalCurrent, canonicalPresent := o.staticMembers[canonical]
		changed = changed || !canonicalPresent || !valueDomain.Equal(canonicalCurrent, canonicalValue)
	}
	if !changed {
		return o, true
	}
	out := CloneObject(o)
	if out.staticMembers == nil {
		out.staticMembers = make(map[keyspace.Key]product.Value, 1)
	}
	out.stableShape = false
	out.staticMembers[key] = primaryValue
	if hasCanonical {
		out.staticMembers[canonical] = canonicalValue
	}
	return out, true
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

// WithoutDynamicIndexFactSubtree returns an object with dynamic-index facts for
// tables at prefix or below removed.
func (o TableObject) WithoutDynamicIndexFactSubtree(ks *keyspace.KeySpace, prefix []segment.Segment) (TableObject, bool) {
	return o.withoutDynamicIndexFactsMatching(ks, prefix, false)
}

// WithoutDynamicIndexFactDescendants returns an object with dynamic-index facts
// strictly below prefix removed while preserving facts for the exact table.
func (o TableObject) WithoutDynamicIndexFactDescendants(ks *keyspace.KeySpace, prefix []segment.Segment) (TableObject, bool) {
	return o.withoutDynamicIndexFactsMatching(ks, prefix, true)
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

// DynamicIndexFactsTop reports whether dynamic-index facts are unknown rather
// than represented by the finite map returned by DynamicIndexFacts.
func (o TableObject) DynamicIndexFactsTop() bool { return o.dynamicIndexFactsTop }

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
		segments, ok := ks.SuffixSegmentsView(key)
		if !ok {
			continue
		}
		matches := dynamicFactSegmentsMatchInvalidation(segments, prefix, false)
		if descendantsOnly {
			matches = dynamicFactSegmentsMatchInvalidation(segments, prefix, true)
		}
		if matches {
			delete(out.staticMembers, key)
			changed = true
		}
	}
	if !changed {
		return o, false
	}
	out.stableShape = false
	out.prefixStableShape = false
	if len(out.staticMembers) == 0 {
		out.staticMembers = nil
	}
	return out, true
}

func (o TableObject) withoutDynamicIndexFactsMatching(ks *keyspace.KeySpace, prefix []segment.Segment, descendantsOnly bool) (TableObject, bool) {
	if o.bottom || len(o.dynamicIndexFacts) == 0 || o.dynamicIndexFactsTop {
		return o, false
	}
	out := CloneObject(o)
	changed := false
	for key := range out.dynamicIndexFacts {
		if key.Table.Kind == keyspace.KindInvalid {
			continue
		}
		segments := ks.Segments(key.Table)
		matches := dynamicFactSegmentsMatchInvalidation(segments, prefix, false)
		if descendantsOnly {
			matches = dynamicFactSegmentsMatchInvalidation(segments, prefix, true)
		}
		if matches {
			delete(out.dynamicIndexFacts, key)
			changed = true
		}
	}
	if !changed {
		return o, false
	}
	out.stableShape = false
	out.prefixStableShape = false
	if len(out.dynamicIndexFacts) == 0 {
		out.dynamicIndexFacts = nil
	}
	return out, true
}

func dynamicFactSegmentsMatchInvalidation(segments, prefix []segment.Segment, strict bool) bool {
	if strict {
		if address.SegmentsHasStrictPrefix(segments, prefix) {
			return true
		}
	} else if address.SegmentsHasPrefix(segments, prefix) {
		return true
	}
	for start := 1; start < len(segments); start++ {
		suffix := segments[start:]
		if strict {
			if address.SegmentsHasStrictPrefix(suffix, prefix) {
				return true
			}
			continue
		}
		if address.SegmentsHasPrefix(suffix, prefix) {
			return true
		}
	}
	return false
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
			segments, ok := from.SuffixSegmentsView(key)
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
	return objectDomainCache.GetFor(reg, objectDomainForRegistry)
}

func objectDomainForRegistry(reg *axis.Registry) lattice.Lattice[TableObject] {
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
				dynamicDomain.Equal(dynamicLane(a, dynamicDomain), dynamicLane(b, dynamicDomain)) &&
				a.stableShape == b.stableShape &&
				a.prefixStableShape == b.prefixStableShape
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
					dynamicDomain.LessOrEq(dynamicLane(a, dynamicDomain), dynamicLane(b, dynamicDomain)) &&
					stableShapeLessOrEq(a.stableShape, b.stableShape) &&
					stableShapeLessOrEq(a.prefixStableShape, b.prefixStableShape)
			}
		},
		Join: func(a, b TableObject) TableObject {
			if a.bottom {
				return b
			}
			if b.bottom {
				return a
			}
			static := staticDomain.Join(staticLane(a), staticLane(b))
			dynamic := dynamicDomain.Join(dynamicLane(a, dynamicDomain), dynamicLane(b, dynamicDomain))
			return objectFromLanes(
				valueDomain.Join(a.root, b.root),
				static,
				dynamic,
				dynamicDomain,
				a.stableShape && b.stableShape,
				a.prefixStableShape && b.prefixStableShape,
			)
		},
		Widen: func(prev, next TableObject) TableObject {
			if prev.bottom {
				return next
			}
			if next.bottom {
				return prev
			}
			static := staticDomain.Widen(staticLane(prev), staticLane(next))
			dynamic := dynamicDomain.Widen(dynamicLane(prev, dynamicDomain), dynamicLane(next, dynamicDomain))
			return objectFromLanes(
				valueDomain.Widen(prev.root, next.root),
				static,
				dynamic,
				dynamicDomain,
				prev.stableShape && next.stableShape,
				prev.prefixStableShape && next.prefixStableShape,
			)
		},
	}
}

func MapDomain(reg *axis.Registry) lattice.Lattice[map[identity.ID]TableObject] {
	return mapDomainCache.GetFor(reg, mapDomainForRegistry)
}

func mapDomainForRegistry(reg *axis.Registry) lattice.Lattice[map[identity.ID]TableObject] {
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
	stableShape bool,
	prefixStableShape bool,
) TableObject {
	object := TableObject{
		root:              root,
		staticMembers:     clonePathMap(static.Values()),
		stableShape:       stableShape,
		prefixStableShape: prefixStableShape,
	}
	if dynamicDomain.Equal(dynamic, dynamicDomain.Top()) {
		object.dynamicIndexFactsTop = true
	} else {
		object.dynamicIndexFacts = dynamicindex.CloneMap(dynamic)
	}
	return object
}

func stableShapeLessOrEq(a, b bool) bool {
	return a || !b
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
