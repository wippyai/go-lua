package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// HeapTableIdentitySkeletonFactor is the exact root/member-value-less
// component of the HeapTableIdentity lane. It retains object existence,
// static-member key presence, dynamic-index facts, and shape proofs. Object
// roots and static member values are owned exclusively by their independent
// factors; dynamic-index facts remain semantic skeleton coordinates.
//
// The type is sealed to one ProductDomain and one run-local KeySpace. Its
// representation is deliberately opaque so callers cannot forge key-presence
// facts or publish a partially materialized stable object.
type HeapTableIdentitySkeletonFactor struct {
	seal    *productDomainSeal
	lane    ProductLane
	keys    *keyspace.KeySpace
	top     bool
	objects map[identity.Term]heapTableIdentityObjectSkeleton
}

type heapTableIdentityObjectSkeleton struct {
	bottom bool

	staticKeys           []keyspace.Key
	dynamicIndexFacts    map[dynamicindex.Key]dynamicindex.Fact
	dynamicIndexFactsTop bool
	stableShape          bool
	prefixStableShape    bool
}

// HeapObjectRootFactor is one sealed (identity, value) coordinate of the
// HeapTableIdentity lane. Every finite non-bottom object has exactly one root
// factor; the skeleton deliberately retains no root value.
type HeapObjectRootFactor struct {
	seal  *productDomainSeal
	lane  ProductLane
	keys  *keyspace.KeySpace
	id    identity.Term
	value product.Value
}

// HeapObjectRootSlot is a sealed object-root coordinate with no value. Fresh
// constructors return a slot so an object cannot be published until its root
// is explicitly bound.
type HeapObjectRootSlot struct {
	seal *productDomainSeal
	lane ProductLane
	keys *keyspace.KeySpace
	id   identity.Term
}

// HeapStaticMemberFactor is one sealed (identity, key, value) coordinate of
// the HeapTableIdentity lane. Identity and Key are immutable coordinates;
// WithHeapStaticMemberValue is the only way to replace Value while retaining
// the ProductDomain and KeySpace ownership proof.
type HeapStaticMemberFactor struct {
	seal  *productDomainSeal
	lane  ProductLane
	keys  *keyspace.KeySpace
	id    identity.Term
	key   keyspace.Key
	value product.Value
}

// ReadHeapTableObjectTermFactor observes one symbolic or concrete object root
// directly from the registered heap-identity carrier. Missing identities are
// the carrier bottom; a top carrier returns the object top.
func (d ProductDomain) ReadHeapTableObjectTermFactor(factor LaneFactor, term identity.Term) (heapidentity.TableObject, error) {
	if !term.Valid() {
		return heapidentity.BottomObject(d.reg), ErrInvalidLaneFactor
	}
	if _, err := d.validateHeapTableIdentityFactor(factor); err != nil {
		return heapidentity.BottomObject(d.reg), err
	}
	lane := typedLaneFactorValue[heapTableIdentityLane](factor.payload)
	return lane.readTerm(d.reg, term), nil
}

// HeapStaticMemberSlot is a sealed member coordinate with no value. Fresh
// constructors return slots so an uninitialized Bottom placeholder can never
// be mistaken for a complete, publishable member factor.
type HeapStaticMemberSlot struct {
	seal *productDomainSeal
	lane ProductLane
	keys *keyspace.KeySpace
	id   identity.Term
	key  keyspace.Key
}

// HeapTableConstructorConfig is the complete non-value shape of one freshly
// materialized table object. MemberSuffixes are normalized to the exact
// primary and field-canonical mirror key inventory owned by the skeleton.
type HeapTableConstructorConfig struct {
	Identity          identity.ID
	MemberSuffixes    [][]segment.Segment
	StableShape       bool
	PrefixStableShape bool
}

// Identity returns the heap object coordinate.
func (f HeapObjectRootFactor) Identity() identity.ID {
	id, _ := f.id.Concrete()
	return id
}

// IdentityTerm returns the complete relational heap coordinate.
func (f HeapObjectRootFactor) IdentityTerm() identity.Term { return f.id }

// Value returns the object's root product-lattice value.
func (f HeapObjectRootFactor) Value() product.Value { return f.value }

// Slot returns the factor's immutable sealed coordinate.
func (f HeapObjectRootFactor) Slot() HeapObjectRootSlot {
	return HeapObjectRootSlot{seal: f.seal, lane: f.lane, keys: f.keys, id: f.id}
}

// Identity returns the heap object coordinate.
func (s HeapObjectRootSlot) Identity() identity.ID {
	id, _ := s.id.Concrete()
	return id
}

// IdentityTerm returns the complete relational heap coordinate.
func (s HeapObjectRootSlot) IdentityTerm() identity.Term { return s.id }

// Identity returns the heap object coordinate.
func (f HeapStaticMemberFactor) Identity() identity.ID {
	id, _ := f.id.Concrete()
	return id
}

// IdentityTerm returns the complete relational heap coordinate.
func (f HeapStaticMemberFactor) IdentityTerm() identity.Term { return f.id }

// Key returns the rootless static-member key coordinate.
func (f HeapStaticMemberFactor) Key() keyspace.Key { return f.key }

// Value returns the member's product-lattice value.
func (f HeapStaticMemberFactor) Value() product.Value { return f.value }

// Slot returns the factor's immutable sealed coordinate.
func (f HeapStaticMemberFactor) Slot() HeapStaticMemberSlot {
	return HeapStaticMemberSlot{seal: f.seal, lane: f.lane, keys: f.keys, id: f.id, key: f.key}
}

// Identity returns the heap object coordinate.
func (s HeapStaticMemberSlot) Identity() identity.ID {
	id, _ := s.id.Concrete()
	return id
}

// IdentityTerm returns the complete relational heap coordinate.
func (s HeapStaticMemberSlot) IdentityTerm() identity.Term { return s.id }

// Key returns the rootless static-member key coordinate.
func (s HeapStaticMemberSlot) Key() keyspace.Key { return s.key }

// ImportHeapTableIdentitySkeleton re-seals an exact heap skeleton into this
// ProductDomain without composing its independent root or member values. The
// keyspace must be shared: run-local keys are opaque coordinates and cannot be
// translated by spelling at this boundary.
func (d ProductDomain) ImportHeapTableIdentitySkeleton(source HeapTableIdentitySkeletonFactor, keys *keyspace.KeySpace) (HeapTableIdentitySkeletonFactor, error) {
	lane, ok := d.ProductLane(LaneHeapTableIdentity)
	if !ok || keys == nil || !keys.Valid() || source.seal == nil || source.lane.seal != source.seal ||
		source.lane.id != LaneHeapTableIdentity || source.keys != keys {
		return HeapTableIdentitySkeletonFactor{}, fmt.Errorf("%w: incompatible heap skeleton import", ErrInvalidLaneFactor)
	}
	out := HeapTableIdentitySkeletonFactor{seal: d.seal, lane: lane, keys: keys, top: source.top}
	if source.top {
		return out, nil
	}
	out.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton, len(source.objects))
	for id, object := range source.objects {
		clone := object
		clone.staticKeys = append([]keyspace.Key(nil), object.staticKeys...)
		if object.dynamicIndexFacts != nil {
			clone.dynamicIndexFacts = make(map[dynamicindex.Key]dynamicindex.Fact, len(object.dynamicIndexFacts))
			for key, fact := range object.dynamicIndexFacts {
				if !product.BelongsToRegistry(d.reg, fact.KeyValue) || !product.BelongsToRegistry(d.reg, fact.Value) {
					return HeapTableIdentitySkeletonFactor{}, fmt.Errorf("%w: heap skeleton import has a foreign dynamic fact", ErrInvalidLaneFactor)
				}
				clone.dynamicIndexFacts[key] = fact
			}
		}
		out.objects[id] = clone
	}
	if _, err := d.validateHeapTableIdentitySkeleton(out, keys); err != nil {
		return HeapTableIdentitySkeletonFactor{}, err
	}
	return out, nil
}

// ImportHeapObjectRootSlot re-seals one value-less object-root coordinate into
// this ProductDomain. The run-local keyspace is part of the heap factorization
// authority and must be shared exactly; root values are bound separately.
func (d ProductDomain) ImportHeapObjectRootSlot(source HeapObjectRootSlot, keys *keyspace.KeySpace) (HeapObjectRootSlot, error) {
	lane, ok := d.ProductLane(LaneHeapTableIdentity)
	if !ok || keys == nil || !keys.Valid() || source.seal == nil || source.lane.seal != source.seal ||
		source.lane.id != LaneHeapTableIdentity || source.keys != keys || !source.id.Valid() {
		return HeapObjectRootSlot{}, fmt.Errorf("%w: incompatible heap root-slot import", ErrInvalidLaneFactor)
	}
	return HeapObjectRootSlot{seal: d.seal, lane: lane, keys: keys, id: source.id}, nil
}

// ImportHeapStaticMemberSlot re-seals one value-less member coordinate into
// this ProductDomain. Values remain separate decision roots and are bound only
// by strict heap publication.
func (d ProductDomain) ImportHeapStaticMemberSlot(source HeapStaticMemberSlot, keys *keyspace.KeySpace) (HeapStaticMemberSlot, error) {
	lane, ok := d.ProductLane(LaneHeapTableIdentity)
	if !ok || keys == nil || !keys.Valid() || source.seal == nil || source.lane.seal != source.seal ||
		source.lane.id != LaneHeapTableIdentity || source.keys != keys || !source.id.Valid() {
		return HeapStaticMemberSlot{}, fmt.Errorf("%w: incompatible heap member-slot import", ErrInvalidLaneFactor)
	}
	return HeapStaticMemberSlot{seal: d.seal, lane: lane, keys: keys, id: source.id, key: source.key}, nil
}

// DecomposeHeapTableIdentity transposes one exact HeapTableIdentity lane into
// its root/member-value-less metadata/key-presence skeleton and independently
// variable object-root and member values. Roots are sorted by identity;
// members are sorted by identity and then structural key spelling.
func (d ProductDomain) DecomposeHeapTableIdentity(
	factor LaneFactor,
	keys *keyspace.KeySpace,
) (HeapTableIdentitySkeletonFactor, []HeapObjectRootFactor, []HeapStaticMemberFactor, error) {
	runtime, err := d.validateHeapTableIdentityFactor(factor)
	if err != nil {
		return HeapTableIdentitySkeletonFactor{}, nil, nil, err
	}
	if keys == nil || !keys.Valid() {
		return HeapTableIdentitySkeletonFactor{}, nil, nil, fmt.Errorf("%w: heap factorization requires a valid keyspace", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[heapTableIdentityLane](factor.payload)
	skeleton := HeapTableIdentitySkeletonFactor{
		seal: d.seal, lane: runtime.lane, keys: keys, top: lane.top,
	}
	if lane.top {
		return skeleton, nil, nil, nil
	}
	if len(lane.values) != 0 {
		skeleton.objects = make(map[identity.Term]heapTableIdentityObjectSkeleton, len(lane.values))
	}
	roots := make([]HeapObjectRootFactor, 0, len(lane.values))
	members := make([]HeapStaticMemberFactor, 0)
	for id, object := range lane.values {
		objectSkeleton := heapTableIdentityObjectSkeleton{
			bottom:               object.IsBottom(),
			dynamicIndexFactsTop: object.DynamicIndexFactsTop(),
			stableShape:          object.StableShape(),
			prefixStableShape:    object.PrefixStableShape(),
		}
		if !object.IsBottom() {
			roots = append(roots, HeapObjectRootFactor{
				seal: d.seal, lane: runtime.lane, keys: keys,
				id: id, value: object.Root(),
			})
			objectSkeleton.dynamicIndexFacts = object.DynamicIndexFacts()
			valid := true
			object.VisitStaticMembers(func(key keyspace.Key, value product.Value) bool {
				if _, ok := keys.SuffixSegmentsView(key); !ok {
					valid = false
					return false
				}
				objectSkeleton.staticKeys = append(objectSkeleton.staticKeys, key)
				members = append(members, HeapStaticMemberFactor{
					seal: d.seal, lane: runtime.lane, keys: keys,
					id: id, key: key, value: value,
				})
				return true
			})
			if !valid {
				return HeapTableIdentitySkeletonFactor{}, nil, nil, fmt.Errorf("%w: heap object %v has a foreign or non-suffix static key", ErrInvalidLaneFactor, id)
			}
			sort.Slice(objectSkeleton.staticKeys, func(i, j int) bool {
				return keys.Less(objectSkeleton.staticKeys[i], objectSkeleton.staticKeys[j])
			})
		}
		skeleton.objects[id] = objectSkeleton
	}
	sort.Slice(roots, func(i, j int) bool { return identityTermLess(roots[i].id, roots[j].id) })
	sort.Slice(members, func(i, j int) bool {
		if members[i].id != members[j].id {
			return identityTermLess(members[i].id, members[j].id)
		}
		return keys.Less(members[i].key, members[j].key)
	})
	return skeleton, roots, members, nil
}

// ComposeHeapTableIdentity is the exact inverse of
// DecomposeHeapTableIdentity. It validates the complete root and member
// inventories before constructing any lane value, so duplicate, missing,
// extra, foreign-domain, and foreign-keyspace coordinates fail without
// publishing a partial object.
func (d ProductDomain) ComposeHeapTableIdentity(
	skeleton HeapTableIdentitySkeletonFactor,
	roots []HeapObjectRootFactor,
	members []HeapStaticMemberFactor,
	keys *keyspace.KeySpace,
) (LaneFactor, error) {
	runtime, err := d.validateHeapTableIdentitySkeleton(skeleton, keys)
	if err != nil {
		return LaneFactor{}, err
	}
	if skeleton.top {
		if len(roots) != 0 || len(members) != 0 {
			return LaneFactor{}, fmt.Errorf("%w: top heap skeleton cannot carry finite roots or members", ErrInvalidLaneFactor)
		}
		lane := heapTableIdentityLane{top: true}
		return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[heapTableIdentityLane]{value: lane}}, nil
	}

	rootValues := make(map[identity.Term]product.Value, len(roots))
	for index, root := range roots {
		if err := d.validateHeapObjectRoot(root, keys); err != nil {
			return LaneFactor{}, fmt.Errorf("%w: root %d: %v", ErrInvalidLaneFactor, index, err)
		}
		object, exists := skeleton.objects[root.id]
		if !exists || object.bottom {
			return LaneFactor{}, fmt.Errorf("%w: root %d is absent from heap skeleton", ErrInvalidLaneFactor, index)
		}
		if _, duplicate := rootValues[root.id]; duplicate {
			return LaneFactor{}, fmt.Errorf("%w: duplicate heap object root factor", ErrInvalidLaneFactor)
		}
		rootValues[root.id] = root.value
	}

	valuesByObject := make(map[identity.Term]map[keyspace.Key]product.Value, len(skeleton.objects))
	for index, member := range members {
		if err := d.validateHeapStaticMember(member, keys); err != nil {
			return LaneFactor{}, fmt.Errorf("%w: member %d: %v", ErrInvalidLaneFactor, index, err)
		}
		object, exists := skeleton.objects[member.id]
		if !exists || object.bottom || !sortedHeapKeyContains(keys, object.staticKeys, member.key) {
			return LaneFactor{}, fmt.Errorf("%w: member %d is absent from heap skeleton", ErrInvalidLaneFactor, index)
		}
		objectValues := valuesByObject[member.id]
		if objectValues == nil {
			objectValues = make(map[keyspace.Key]product.Value, len(object.staticKeys))
			valuesByObject[member.id] = objectValues
		}
		if _, duplicate := objectValues[member.key]; duplicate {
			return LaneFactor{}, fmt.Errorf("%w: duplicate heap member factor", ErrInvalidLaneFactor)
		}
		objectValues[member.key] = member.value
	}

	objects := make(map[identity.Term]heapidentity.TableObject, len(skeleton.objects))
	for id, objectSkeleton := range skeleton.objects {
		if objectSkeleton.bottom {
			if len(objectSkeleton.staticKeys) != 0 || len(valuesByObject[id]) != 0 {
				return LaneFactor{}, fmt.Errorf("%w: bottom heap object has static members", ErrInvalidLaneFactor)
			}
			objects[id] = heapidentity.BottomObject(d.reg)
			continue
		}
		root, hasRoot := rootValues[id]
		if !hasRoot {
			return LaneFactor{}, fmt.Errorf("%w: heap object %v has no root factor", ErrInvalidLaneFactor, id)
		}
		staticValues := valuesByObject[id]
		if len(staticValues) != len(objectSkeleton.staticKeys) {
			return LaneFactor{}, fmt.Errorf("%w: heap object %v has %d members, want %d", ErrInvalidLaneFactor, id, len(staticValues), len(objectSkeleton.staticKeys))
		}
		object, buildErr := recomposeHeapTableIdentityObject(d.reg, keys, objectSkeleton, root, staticValues)
		if buildErr != nil {
			return LaneFactor{}, fmt.Errorf("%w: heap object %v: %v", ErrInvalidLaneFactor, id, buildErr)
		}
		objects[id] = object
	}
	laneDomain := heapTermMapDomain(d.reg)
	lane := heapTableIdentityLaneFromMap(laneDomain, objects)
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[heapTableIdentityLane]{value: lane}}, nil
}

// WithHeapObjectRootValue replaces only an object-root terminal while
// preserving its unforgeable identity, ProductDomain, lane, and KeySpace
// ownership.
func (d ProductDomain) WithHeapObjectRootValue(
	factor HeapObjectRootFactor,
	value product.Value,
) (HeapObjectRootFactor, error) {
	if err := d.validateHeapObjectRoot(factor, factor.keys); err != nil {
		return HeapObjectRootFactor{}, err
	}
	if !product.BelongsToRegistry(d.reg, value) {
		return HeapObjectRootFactor{}, fmt.Errorf("%w: foreign heap object root value", ErrInvalidLaneFactor)
	}
	factor.value = value
	return factor, nil
}

// BindHeapObjectRootValue binds a product value to a sealed constructor slot,
// producing the only root representation accepted by Compose.
func (d ProductDomain) BindHeapObjectRootValue(
	slot HeapObjectRootSlot,
	value product.Value,
) (HeapObjectRootFactor, error) {
	if err := d.validateHeapObjectRootSlot(slot, slot.keys); err != nil {
		return HeapObjectRootFactor{}, err
	}
	if !product.BelongsToRegistry(d.reg, value) {
		return HeapObjectRootFactor{}, fmt.Errorf("%w: foreign heap object root value", ErrInvalidLaneFactor)
	}
	return HeapObjectRootFactor{
		seal: slot.seal, lane: slot.lane, keys: slot.keys,
		id: slot.id, value: value,
	}, nil
}

// WithHeapStaticMemberValue replaces only a member terminal while preserving
// its unforgeable identity, key, ProductDomain, lane, and KeySpace ownership.
func (d ProductDomain) WithHeapStaticMemberValue(
	factor HeapStaticMemberFactor,
	value product.Value,
) (HeapStaticMemberFactor, error) {
	if err := d.validateHeapStaticMember(factor, factor.keys); err != nil {
		return HeapStaticMemberFactor{}, err
	}
	if !product.BelongsToRegistry(d.reg, value) {
		return HeapStaticMemberFactor{}, fmt.Errorf("%w: foreign heap member value", ErrInvalidLaneFactor)
	}
	factor.value = value
	return factor, nil
}

// BindHeapStaticMemberValue binds a product value to a sealed constructor
// slot, producing the only member representation accepted by Compose.
func (d ProductDomain) BindHeapStaticMemberValue(
	slot HeapStaticMemberSlot,
	value product.Value,
) (HeapStaticMemberFactor, error) {
	if err := d.validateHeapStaticMemberSlot(slot, slot.keys); err != nil {
		return HeapStaticMemberFactor{}, err
	}
	if !product.BelongsToRegistry(d.reg, value) {
		return HeapStaticMemberFactor{}, fmt.Errorf("%w: foreign heap member value", ErrInvalidLaneFactor)
	}
	return HeapStaticMemberFactor{
		seal: slot.seal, lane: slot.lane, keys: slot.keys,
		id: slot.id, key: slot.key, value: value,
	}, nil
}

// InstallHeapTableConstructor atomically replaces one object's skeleton with
// a fresh constructor shape and returns one root slot plus one member slot for
// every exact static key, including field-canonical mirrors. It never reads or
// aligns the replaced object's prior values. Slots carry no value and the
// object cannot be published until every slot is explicitly bound.
func (d ProductDomain) InstallHeapTableConstructor(
	skeleton HeapTableIdentitySkeletonFactor,
	config HeapTableConstructorConfig,
) (HeapTableIdentitySkeletonFactor, HeapObjectRootSlot, []HeapStaticMemberSlot, error) {
	return d.installHeapTableTermConstructor(skeleton, identity.ConcreteTerm(config.Identity), config)
}

func (d ProductDomain) installHeapTableTermConstructor(
	skeleton HeapTableIdentitySkeletonFactor,
	term identity.Term,
	config HeapTableConstructorConfig,
) (HeapTableIdentitySkeletonFactor, HeapObjectRootSlot, []HeapStaticMemberSlot, error) {
	if _, err := d.validateHeapTableIdentitySkeleton(skeleton, skeleton.keys); err != nil {
		return HeapTableIdentitySkeletonFactor{}, HeapObjectRootSlot{}, nil, err
	}
	if skeleton.top || !term.Valid() {
		return HeapTableIdentitySkeletonFactor{}, HeapObjectRootSlot{}, nil, fmt.Errorf("%w: constructor requires finite skeleton and identity", ErrInvalidLaneFactor)
	}
	keySet := make(map[keyspace.Key]struct{}, len(config.MemberSuffixes)*2)
	for index, suffix := range config.MemberSuffixes {
		primary, ok := heapidentity.StaticMemberSuffixKey(skeleton.keys, suffix)
		if !ok {
			return HeapTableIdentitySkeletonFactor{}, HeapObjectRootSlot{}, nil, fmt.Errorf("%w: constructor member %d has invalid suffix", ErrInvalidLaneFactor, index)
		}
		keySet[primary] = struct{}{}
		if canonical, mirrored := heapidentity.FieldCanonicalStaticMemberSuffixKey(skeleton.keys, suffix); mirrored {
			keySet[canonical] = struct{}{}
		}
	}
	staticKeys := make([]keyspace.Key, 0, len(keySet))
	for key := range keySet {
		staticKeys = append(staticKeys, key)
	}
	sort.Slice(staticKeys, func(i, j int) bool { return skeleton.keys.Less(staticKeys[i], staticKeys[j]) })

	out := HeapTableIdentitySkeletonFactor{
		seal: skeleton.seal, lane: skeleton.lane, keys: skeleton.keys,
		objects: make(map[identity.Term]heapTableIdentityObjectSkeleton, len(skeleton.objects)+1),
	}
	for id, object := range skeleton.objects {
		if id != term {
			out.objects[id] = object
		}
	}
	prefixStable := config.PrefixStableShape || config.StableShape
	out.objects[term] = heapTableIdentityObjectSkeleton{
		staticKeys:        staticKeys,
		stableShape:       config.StableShape,
		prefixStableShape: prefixStable,
	}
	slots := make([]HeapStaticMemberSlot, len(staticKeys))
	for index, key := range staticKeys {
		slots[index] = HeapStaticMemberSlot{
			seal: d.seal, lane: skeleton.lane, keys: skeleton.keys,
			id: term, key: key,
		}
	}
	rootSlot := HeapObjectRootSlot{
		seal: d.seal, lane: skeleton.lane, keys: skeleton.keys, id: term,
	}
	return out, rootSlot, slots, nil
}

// HeapTableIdentitySkeletonObjectRootDefault returns the semantic value of a
// non-explicit object-root coordinate:
//
//   - absent object or Object Bottom: Value Bottom;
//   - heap Top: Value Top;
//   - finite non-bottom object: explicit is true and callers must use its
//     independent HeapObjectRootFactor.
//
// This is the quotient boundary used when guarded branches have different
// object inventories. It is exact for the outer heap map lattice.
func (d ProductDomain) HeapTableIdentitySkeletonObjectRootDefault(
	skeleton HeapTableIdentitySkeletonFactor,
	id identity.ID,
) (product.Value, bool, error) {
	return d.heapTableIdentitySkeletonObjectRootTermDefault(skeleton, identity.ConcreteTerm(id))
}

func (d ProductDomain) heapTableIdentitySkeletonObjectRootTermDefault(
	skeleton HeapTableIdentitySkeletonFactor,
	id identity.Term,
) (product.Value, bool, error) {
	if _, err := d.validateHeapTableIdentitySkeleton(skeleton, skeleton.keys); err != nil {
		return product.Value{}, false, err
	}
	if !id.Valid() {
		return product.Value{}, false, fmt.Errorf("%w: object-root query has empty identity", ErrInvalidLaneFactor)
	}
	if skeleton.top {
		return product.Top(), false, nil
	}
	object, ok := skeleton.objects[id]
	if !ok || object.bottom {
		return product.Bottom(d.reg), false, nil
	}
	return product.Value{}, true, nil
}

// HeapTableIdentitySkeletonStaticMemberDefault returns the semantic value of a
// non-explicit member coordinate. There are three distinct cases:
//
//   - absent object or Object Bottom: Value Bottom;
//   - present object with absent MustMap key, or heap Top: Value Top;
//   - explicit key: present is true and callers must use its member factor.
//
// This is the quotient boundary used when guarded branches have different
// member inventories; it prevents both absence-as-Bottom underapproximation
// and heap-object absence-as-Top overapproximation.
func (d ProductDomain) HeapTableIdentitySkeletonStaticMemberDefault(
	skeleton HeapTableIdentitySkeletonFactor,
	id identity.ID,
	key keyspace.Key,
) (product.Value, bool, error) {
	return d.heapTableIdentitySkeletonStaticMemberTermDefault(skeleton, identity.ConcreteTerm(id), key)
}

func (d ProductDomain) heapTableIdentitySkeletonStaticMemberTermDefault(
	skeleton HeapTableIdentitySkeletonFactor,
	id identity.Term,
	key keyspace.Key,
) (product.Value, bool, error) {
	if _, err := d.validateHeapTableIdentitySkeleton(skeleton, skeleton.keys); err != nil {
		return product.Value{}, false, err
	}
	if _, ok := skeleton.keys.SuffixSegmentsView(key); !ok {
		return product.Value{}, false, fmt.Errorf("%w: static-member query has foreign or non-suffix key", ErrInvalidLaneFactor)
	}
	if skeleton.top {
		return product.Top(), false, nil
	}
	object, ok := skeleton.objects[id]
	if !ok || object.bottom {
		return product.Bottom(d.reg), false, nil
	}
	if !sortedHeapKeyContains(skeleton.keys, object.staticKeys, key) {
		return product.Top(), false, nil
	}
	return product.Value{}, true, nil
}

// HeapTableIdentitySkeletonBottom returns the finite empty-map skeleton, the
// bottom element of the HeapTableIdentity map lane.
func (d ProductDomain) HeapTableIdentitySkeletonBottom(keys *keyspace.KeySpace) (HeapTableIdentitySkeletonFactor, error) {
	lane, ok := d.ProductLane(LaneHeapTableIdentity)
	if !ok || keys == nil || !keys.Valid() {
		return HeapTableIdentitySkeletonFactor{}, fmt.Errorf("%w: heap skeleton bottom requires heap lane and valid keyspace", ErrInvalidLaneFactor)
	}
	return HeapTableIdentitySkeletonFactor{seal: d.seal, lane: lane, keys: keys}, nil
}

// HeapTableIdentitySkeletonEqual reports semantic equality excluding object
// roots and member values, which are compared by their independent factors.
func (d ProductDomain) HeapTableIdentitySkeletonEqual(left, right HeapTableIdentitySkeletonFactor) (bool, error) {
	if _, err := d.validateHeapTableIdentitySkeletonPair(left, right); err != nil {
		return false, err
	}
	return d.heapTableIdentitySkeletonLessOrEq(left, right) && d.heapTableIdentitySkeletonLessOrEq(right, left), nil
}

// HeapTableIdentitySkeletonLessOrEq reports the exact metadata/key-presence
// order induced by the existing HeapTableIdentity lane.
func (d ProductDomain) HeapTableIdentitySkeletonLessOrEq(left, right HeapTableIdentitySkeletonFactor) (bool, error) {
	if _, err := d.validateHeapTableIdentitySkeletonPair(left, right); err != nil {
		return false, err
	}
	return d.heapTableIdentitySkeletonLessOrEq(left, right), nil
}

func (d ProductDomain) heapTableIdentitySkeletonLessOrEq(left, right HeapTableIdentitySkeletonFactor) bool {
	return heapCoordinateSkeletonLessOrEq(d.reg, heapCoordinateSkeletonFromLegacy(left), heapCoordinateSkeletonFromLegacy(right))
}

func (d ProductDomain) heapObjectSkeletonLessOrEq(keys *keyspace.KeySpace, left, right heapTableIdentityObjectSkeleton) bool {
	return heapObjectSkeletonLessOrEq(d.reg, keys, left, right)
}

func heapObjectSkeletonLessOrEq(reg *axis.Registry, keys *keyspace.KeySpace, left, right heapTableIdentityObjectSkeleton) bool {
	if left.bottom {
		return true
	}
	if right.bottom {
		return false
	}
	// MustMap join intersects keys: proving more keys is more precise.
	for _, key := range right.staticKeys {
		if !sortedHeapKeyContains(keys, left.staticKeys, key) {
			return false
		}
	}
	leftDynamic := left.dynamicIndexFacts
	rightDynamic := right.dynamicIndexFacts
	dynamicDomain := dynamicindex.MapDomain(reg)
	if left.dynamicIndexFactsTop {
		leftDynamic = dynamicDomain.Top()
	}
	if right.dynamicIndexFactsTop {
		rightDynamic = dynamicDomain.Top()
	}
	return dynamicDomain.LessOrEq(leftDynamic, rightDynamic) &&
		(left.stableShape || !right.stableShape) &&
		(left.prefixStableShape || !right.prefixStableShape)
}

// HeapTableIdentitySkeletonJoin returns the exact least upper bound of the
// skeleton coordinates. Object roots and member values must be joined
// independently.
func (d ProductDomain) HeapTableIdentitySkeletonJoin(left, right HeapTableIdentitySkeletonFactor) (HeapTableIdentitySkeletonFactor, error) {
	return d.joinHeapTableIdentitySkeleton(left, right, false)
}

// HeapTableIdentitySkeletonWiden applies the exact widening of the
// skeleton coordinates. Static key presence has the same intersection rule
// as Join; dynamic facts use their registered widening. Roots are widened
// independently through HeapObjectRootFactor.
func (d ProductDomain) HeapTableIdentitySkeletonWiden(previous, next HeapTableIdentitySkeletonFactor) (HeapTableIdentitySkeletonFactor, error) {
	return d.joinHeapTableIdentitySkeleton(previous, next, true)
}

// HeapTableIdentitySkeletonNarrow preserves previous exactly. The registered
// HeapTableIdentity lane has no narrowing operator, so ProductDomain.LaneNarrow
// uses this same keep-previous rule for the complete lane.
func (d ProductDomain) HeapTableIdentitySkeletonNarrow(previous, next HeapTableIdentitySkeletonFactor) (HeapTableIdentitySkeletonFactor, error) {
	if _, err := d.validateHeapTableIdentitySkeletonPair(previous, next); err != nil {
		return HeapTableIdentitySkeletonFactor{}, err
	}
	return previous, nil
}

// HeapTableIdentitySkeletonMeet returns the exact greatest lower bound of the
// skeleton coordinates. Static key presence is unioned: an absent MustMap
// coordinate denotes Top, so conditioning with a present branch preserves the
// present key and its independently met member value.
func (d ProductDomain) HeapTableIdentitySkeletonMeet(left, right HeapTableIdentitySkeletonFactor) (HeapTableIdentitySkeletonFactor, error) {
	if _, err := d.validateHeapTableIdentitySkeletonPair(left, right); err != nil {
		return HeapTableIdentitySkeletonFactor{}, err
	}
	result := meetHeapCoordinateSkeleton(d.reg, heapCoordinateSkeletonFromLegacy(left), heapCoordinateSkeletonFromLegacy(right))
	return legacyHeapCoordinateSkeleton(left, result), nil
}

func (d ProductDomain) joinHeapTableIdentitySkeleton(
	left, right HeapTableIdentitySkeletonFactor,
	widen bool,
) (HeapTableIdentitySkeletonFactor, error) {
	if _, err := d.validateHeapTableIdentitySkeletonPair(left, right); err != nil {
		return HeapTableIdentitySkeletonFactor{}, err
	}
	result := joinHeapCoordinateSkeleton(d.reg, heapCoordinateSkeletonFromLegacy(left), heapCoordinateSkeletonFromLegacy(right), widen)
	return legacyHeapCoordinateSkeleton(left, result), nil
}

func (d ProductDomain) joinHeapObjectSkeleton(
	keys *keyspace.KeySpace,
	left, right heapTableIdentityObjectSkeleton,
	widen bool,
) heapTableIdentityObjectSkeleton {
	return joinHeapObjectSkeleton(d.reg, keys, left, right, widen)
}

func joinHeapObjectSkeleton(
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	left, right heapTableIdentityObjectSkeleton,
	widen bool,
) heapTableIdentityObjectSkeleton {
	dynamicDomain := dynamicindex.MapDomain(reg)
	leftDynamic, rightDynamic := left.dynamicIndexFacts, right.dynamicIndexFacts
	if left.dynamicIndexFactsTop {
		leftDynamic = dynamicDomain.Top()
	}
	if right.dynamicIndexFactsTop {
		rightDynamic = dynamicDomain.Top()
	}
	dynamic := dynamicDomain.Join(leftDynamic, rightDynamic)
	if widen {
		dynamic = dynamicDomain.Widen(leftDynamic, rightDynamic)
	}
	dynamicTop := dynamicDomain.Equal(dynamic, dynamicDomain.Top())
	if dynamicTop {
		dynamic = nil
	}
	return heapTableIdentityObjectSkeleton{
		staticKeys:           intersectSortedHeapKeys(keys, left.staticKeys, right.staticKeys),
		dynamicIndexFacts:    dynamic,
		dynamicIndexFactsTop: dynamicTop,
		stableShape:          left.stableShape && right.stableShape,
		prefixStableShape:    left.prefixStableShape && right.prefixStableShape,
	}
}

func (d ProductDomain) meetHeapObjectSkeleton(
	keys *keyspace.KeySpace,
	left, right heapTableIdentityObjectSkeleton,
) heapTableIdentityObjectSkeleton {
	return meetHeapObjectSkeleton(d.reg, keys, left, right)
}

func meetHeapObjectSkeleton(
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	left, right heapTableIdentityObjectSkeleton,
) heapTableIdentityObjectSkeleton {
	dynamicDomain := dynamicindex.MapDomain(reg)
	leftDynamic, rightDynamic := left.dynamicIndexFacts, right.dynamicIndexFacts
	if left.dynamicIndexFactsTop {
		leftDynamic = dynamicDomain.Top()
	}
	if right.dynamicIndexFactsTop {
		rightDynamic = dynamicDomain.Top()
	}
	dynamic := meetHeapDynamicIndexFacts(reg, dynamicDomain, leftDynamic, rightDynamic)
	dynamicTop := dynamicDomain.Equal(dynamic, dynamicDomain.Top())
	if dynamicTop {
		dynamic = nil
	}
	return heapTableIdentityObjectSkeleton{
		staticKeys:           unionSortedHeapKeys(keys, left.staticKeys, right.staticKeys),
		dynamicIndexFacts:    dynamic,
		dynamicIndexFactsTop: dynamicTop,
		stableShape:          left.stableShape || right.stableShape,
		prefixStableShape:    left.prefixStableShape || right.prefixStableShape,
	}
}

func meetHeapDynamicIndexFacts(
	reg *axis.Registry,
	domain lattice.Lattice[map[dynamicindex.Key]dynamicindex.Fact],
	left, right map[dynamicindex.Key]dynamicindex.Fact,
) map[dynamicindex.Key]dynamicindex.Fact {
	if domain.Equal(left, domain.Top()) {
		return right
	}
	if domain.Equal(right, domain.Top()) {
		return left
	}
	factDomain := dynamicindex.Domain(reg)
	var out map[dynamicindex.Key]dynamicindex.Fact
	for key, leftFact := range left {
		rightFact, ok := right[key]
		if !ok {
			continue
		}
		fact := dynamicindex.Fact{
			KeyPresence: presence.Meet(leftFact.KeyPresence, rightFact.KeyPresence),
			KeyValue:    product.Meet(reg, leftFact.KeyValue, rightFact.KeyValue),
			Value:       product.Meet(reg, leftFact.Value, rightFact.Value),
			Admission:   meetDynamicIndexAdmission(leftFact.Admission, rightFact.Admission),
		}
		if factDomain.Equal(fact, factDomain.Bottom()) {
			continue
		}
		if out == nil {
			out = make(map[dynamicindex.Key]dynamicindex.Fact, min(len(left), len(right)))
		}
		out[key] = fact
	}
	return out
}

func meetDynamicIndexAdmission(left, right dynamicindex.Admission) dynamicindex.Admission {
	if left == right {
		return left
	}
	if left == dynamicindex.AdmissionUnknown {
		return right
	}
	if right == dynamicindex.AdmissionUnknown {
		return left
	}
	return dynamicindex.AdmissionBottom
}

// VisitHeapTableIdentitySkeletonValueDependencies visits product values still
// retained by the root/member-value-less skeleton: dynamic-index key/value
// facts. Object roots and static members are visited through their independent
// factors.
func (d ProductDomain) VisitHeapTableIdentitySkeletonValueDependencies(
	skeleton HeapTableIdentitySkeletonFactor,
	visit func(product.Value),
) error {
	if _, err := d.validateHeapTableIdentitySkeleton(skeleton, skeleton.keys); err != nil {
		return err
	}
	if visit == nil {
		return fmt.Errorf("%w: heap skeleton value visitor is nil", ErrInvalidLaneFactor)
	}
	if skeleton.top {
		return nil
	}
	ids := sortedHeapSkeletonIdentities(skeleton.objects)
	for _, id := range ids {
		object := skeleton.objects[id]
		if object.bottom {
			continue
		}
		dynamicKeys := sortedHeapDynamicKeys(skeleton.keys, object.dynamicIndexFacts)
		for _, key := range dynamicKeys {
			fact := object.dynamicIndexFacts[key]
			visit(fact.KeyValue)
			visit(fact.Value)
		}
	}
	return nil
}

// HeapTableIdentitySkeletonFingerprint returns a deterministic semantic digest
// of every skeleton coordinate, excluding independent object-root and
// static-member values.
func (d ProductDomain) HeapTableIdentitySkeletonFingerprint(
	config FingerprintConfig,
	skeleton HeapTableIdentitySkeletonFactor,
) (uint64, error) {
	if _, err := d.validateHeapTableIdentitySkeleton(skeleton, config.KeySpace); err != nil {
		return 0, err
	}
	if config.Registry != nil && config.Registry != d.reg {
		return 0, fmt.Errorf("%w: fingerprint registry does not own heap skeleton", ErrInvalidLaneFactor)
	}
	if config.Lanes != nil && (len(config.Lanes) != 1 || config.Lanes[0] != LaneHeapTableIdentity) {
		return 0, fmt.Errorf("%w: fingerprint lanes do not name heap skeleton", ErrInvalidLaneFactor)
	}
	config.Registry = d.reg
	config.Lanes = nil
	w := newFingerprintWriter(config)
	w.string("schema", "go-lua.heap-table-identity-skeleton/v2")
	w.bool("top", skeleton.top)
	ids := sortedHeapSkeletonIdentities(skeleton.objects)
	w.int64("object-count", int64(len(ids)))
	for _, id := range ids {
		object := skeleton.objects[id]
		w.identityTerm("identity", id)
		w.bool("object-bottom", object.bottom)
		if object.bottom {
			continue
		}
		w.int64("static-key-count", int64(len(object.staticKeys)))
		for _, key := range object.staticKeys {
			w.pathKey("static-key", key)
		}
		w.bool("dynamic-top", object.dynamicIndexFactsTop)
		dynamicKeys := sortedHeapDynamicKeys(skeleton.keys, object.dynamicIndexFacts)
		w.int64("dynamic-count", int64(len(dynamicKeys)))
		for _, key := range dynamicKeys {
			fingerprintDynamicIndexKey(w, key)
			fingerprintDynamicIndexFact(w, object.dynamicIndexFacts[key])
		}
		w.bool("stable-shape", object.stableShape)
		w.bool("prefix-stable-shape", object.prefixStableShape)
	}
	if err := w.err(); err != nil {
		return 0, err
	}
	return w.sum64(), nil
}

func (d ProductDomain) validateHeapTableIdentityFactor(factor LaneFactor) (*productLaneRuntime, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil {
		return nil, err
	}
	if runtime.lane.id != LaneHeapTableIdentity {
		return nil, fmt.Errorf("%w: got lane %q, want %q", ErrInvalidLaneFactor, runtime.lane.id, LaneHeapTableIdentity)
	}
	return runtime, nil
}

func (d ProductDomain) validateHeapTableIdentitySkeleton(
	skeleton HeapTableIdentitySkeletonFactor,
	keys *keyspace.KeySpace,
) (*productLaneRuntime, error) {
	if skeleton.seal == nil || skeleton.seal != d.seal || skeleton.keys == nil || keys == nil || skeleton.keys != keys || !keys.Valid() {
		return nil, fmt.Errorf("%w: foreign heap skeleton domain or keyspace", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateLane(skeleton.lane)
	if err != nil || runtime.lane.id != LaneHeapTableIdentity {
		return nil, fmt.Errorf("%w: invalid heap skeleton lane", ErrInvalidLaneFactor)
	}
	return runtime, nil
}

func (d ProductDomain) validateHeapTableIdentitySkeletonPair(
	left, right HeapTableIdentitySkeletonFactor,
) (*productLaneRuntime, error) {
	runtime, err := d.validateHeapTableIdentitySkeleton(left, left.keys)
	if err != nil {
		return nil, err
	}
	if _, err := d.validateHeapTableIdentitySkeleton(right, left.keys); err != nil {
		return nil, err
	}
	return runtime, nil
}

func (d ProductDomain) validateHeapObjectRoot(factor HeapObjectRootFactor, keys *keyspace.KeySpace) error {
	if err := d.validateHeapObjectRootSlot(factor.Slot(), keys); err != nil {
		return err
	}
	if !product.BelongsToRegistry(d.reg, factor.value) {
		return fmt.Errorf("%w: foreign heap object root value", ErrInvalidLaneFactor)
	}
	return nil
}

func (d ProductDomain) validateHeapObjectRootSlot(slot HeapObjectRootSlot, keys *keyspace.KeySpace) error {
	if slot.seal == nil || slot.seal != d.seal || slot.keys == nil || keys == nil || slot.keys != keys || !keys.Valid() {
		return fmt.Errorf("%w: foreign heap object root domain or keyspace", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateLane(slot.lane)
	if err != nil || runtime.lane.id != LaneHeapTableIdentity || !slot.id.Valid() {
		return fmt.Errorf("%w: invalid heap object root coordinate", ErrInvalidLaneFactor)
	}
	return nil
}

func (d ProductDomain) validateHeapStaticMember(factor HeapStaticMemberFactor, keys *keyspace.KeySpace) error {
	if err := d.validateHeapStaticMemberSlot(factor.Slot(), keys); err != nil {
		return err
	}
	if !product.BelongsToRegistry(d.reg, factor.value) {
		return fmt.Errorf("%w: foreign heap member value", ErrInvalidLaneFactor)
	}
	return nil
}

func (d ProductDomain) validateHeapStaticMemberSlot(slot HeapStaticMemberSlot, keys *keyspace.KeySpace) error {
	if slot.seal == nil || slot.seal != d.seal || slot.keys == nil || keys == nil || slot.keys != keys || !keys.Valid() {
		return fmt.Errorf("%w: foreign heap member domain or keyspace", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateLane(slot.lane)
	if err != nil || runtime.lane.id != LaneHeapTableIdentity || !slot.id.Valid() {
		return fmt.Errorf("%w: invalid heap member coordinate", ErrInvalidLaneFactor)
	}
	if _, ok := keys.SuffixSegmentsView(slot.key); !ok {
		return fmt.Errorf("%w: heap member key is foreign or not rootless", ErrInvalidLaneFactor)
	}
	return nil
}

func recomposeHeapTableIdentityObject(
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	skeleton heapTableIdentityObjectSkeleton,
	root product.Value,
	staticValues map[keyspace.Key]product.Value,
) (heapidentity.TableObject, error) {
	if !skeleton.dynamicIndexFactsTop {
		return heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:              root,
			StaticMembers:     staticValues,
			DynamicIndexFacts: skeleton.dynamicIndexFacts,
			StableShape:       skeleton.stableShape,
			PrefixStableShape: skeleton.prefixStableShape,
		}), nil
	}

	// Dynamic-top objects cannot be built through TableObjectConfig. Start at
	// exact dynamic top, then restore static members. String-index spellings are
	// installed before their field-canonical mirrors so an existing mirror's
	// independently stored value is the final write.
	object := heapidentity.TopObject().WithRoot(root)
	for pass := 0; pass < 2; pass++ {
		for _, key := range skeleton.staticKeys {
			segments, ok := keys.SuffixSegmentsView(key)
			if !ok {
				return heapidentity.TableObject{}, fmt.Errorf("foreign static-member key")
			}
			hasStringIndex := heapSegmentsHaveStringIndex(segments)
			if (pass == 0) != hasStringIndex {
				continue
			}
			var written bool
			object, written = object.WithStaticMember(reg, keys, segments, staticValues[key])
			if !written {
				return heapidentity.TableObject{}, fmt.Errorf("invalid static-member key")
			}
		}
	}
	if len(object.StaticMembers()) != len(staticValues) {
		return heapidentity.TableObject{}, fmt.Errorf("non-canonical string-index mirror inventory")
	}
	if skeleton.prefixStableShape {
		object = object.WithPrefixStableShape()
	}
	if skeleton.stableShape {
		object = object.WithStableShape()
	}
	return object, nil
}

func heapSegmentsHaveStringIndex(segments []segment.Segment) bool {
	for _, current := range segments {
		if current.Kind == segment.SegmentIndexString {
			return true
		}
	}
	return false
}

func sortedHeapSkeletonIdentities(objects map[identity.Term]heapTableIdentityObjectSkeleton) []identity.Term {
	ids := make([]identity.Term, 0, len(objects))
	for id := range objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return identityTermLess(ids[i], ids[j]) })
	return ids
}

func sortedHeapDynamicKeys(keys *keyspace.KeySpace, values map[dynamicindex.Key]dynamicindex.Fact) []dynamicindex.Key {
	out := make([]dynamicindex.Key, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		left, right := keys.FormatReadOnly(out[i].Table), keys.FormatReadOnly(out[j].Table)
		if left != right {
			return left < right
		}
		return out[i].Site < out[j].Site
	})
	return out
}

func sortedHeapKeyContains(keys *keyspace.KeySpace, values []keyspace.Key, target keyspace.Key) bool {
	index := sort.Search(len(values), func(i int) bool { return !keys.Less(values[i], target) })
	return index < len(values) && values[index] == target
}

func intersectSortedHeapKeys(keys *keyspace.KeySpace, left, right []keyspace.Key) []keyspace.Key {
	out := make([]keyspace.Key, 0, min(len(left), len(right)))
	for i, j := 0, 0; i < len(left) && j < len(right); {
		switch {
		case left[i] == right[j]:
			out = append(out, left[i])
			i++
			j++
		case keys.Less(left[i], right[j]):
			i++
		default:
			j++
		}
	}
	return out
}

func unionSortedHeapKeys(keys *keyspace.KeySpace, left, right []keyspace.Key) []keyspace.Key {
	out := make([]keyspace.Key, 0, len(left)+len(right))
	for i, j := 0, 0; i < len(left) || j < len(right); {
		switch {
		case i == len(left):
			out = append(out, right[j:]...)
			return out
		case j == len(right):
			out = append(out, left[i:]...)
			return out
		case left[i] == right[j]:
			out = append(out, left[i])
			i++
			j++
		case keys.Less(left[i], right[j]):
			out = append(out, left[i])
			i++
		default:
			out = append(out, right[j])
			j++
		}
	}
	return out
}
