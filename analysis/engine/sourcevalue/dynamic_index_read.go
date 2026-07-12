package sourcevalue

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ReadBoundDynamicIndexValue performs the flow-sensitive part of a dynamic
// table read after its table and key operands have already been resolved.
// It is a syntax-free read kernel available to concrete and symbolic call
// boundaries. Missing path/heap evidence fails closed; this function never
// fabricates a type-only answer for a mutable table.
func ReadBoundDynamicIndexValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	ks *keyspace.KeySpace,
	resolver *visibility.Resolver,
	point cfg.Point,
	tablePath pathdom.Path,
	tableValue product.Value,
	keyValue product.Value,
	in state.State,
) (product.Value, bool) {
	if reg == nil || ks == nil || tablePath.IsEmpty() {
		return product.Value{}, false
	}
	if resolver != nil {
		ownerPath := tablePath
		if len(tablePath.Segments) != 0 {
			ownerPath = tablePath.ParentView()
		}
		owner, ok := ReadPathValue(reg, resolver, point, ownerPath, in)
		if !ok || product.Equal(reg, product.Meet(reg, owner, tableValue), product.Bottom(reg)) {
			return product.Value{}, false
		}
		projectedTable, ok := ReadPathValue(reg, resolver, point, tablePath, in)
		if !ok {
			return product.Value{}, false
		}
		tableValue = projectedTable
	}
	seg, hasExactSegment := typevalue.ExactScalarKeySegment(reg, typeValues, keyValue)

	// Prefer the visibility-scoped path fact. It carries branch refinements
	// which may be newer than the identity-owned heap snapshot.
	if hasExactSegment && resolver != nil {
		if value, ok := ReadPathValue(reg, resolver, point, tablePath.Append(seg), in); ok {
			return value, true
		}
	}
	if hasExactSegment {
		if value, ok := HeapMemberFromValue(reg, ks, in, tableValue, []segment.Segment{seg}); ok {
			return value, true
		}
	}

	id, ok := product.Get(reg, tableValue, identity.Key).ID()
	if !ok {
		return product.Value{}, false
	}
	object := in.ReadHeapTableObject(reg, id)
	if object.IsBottom() || object.DynamicIndexFactsTop() {
		return product.Value{}, false
	}
	rootID, rootOK := product.Get(reg, object.Root(), identity.Key).ID()
	if !rootOK || rootID != id || product.Equal(reg, product.Meet(reg, object.Root(), tableValue), product.Bottom(reg)) {
		return product.Value{}, false
	}

	valueDomain := product.Domain(reg)
	joined := product.Bottom(reg)
	found := false
	if !hasExactSegment {
		// A broad key may name any finite static member. Enumerate in canonical
		// keyspace order and retain nil because it may also name no member.
		if !object.StableShape() {
			return product.Value{}, false
		}
		members := object.StaticMembers()
		memberKeys := make([]keyspace.Key, 0, len(members))
		for memberKey := range members {
			memberKeys = append(memberKeys, memberKey)
		}
		sort.Slice(memberKeys, func(i, j int) bool { return ks.Less(memberKeys[i], memberKeys[j]) })
		for _, memberKey := range memberKeys {
			segments, ok := ks.SuffixSegmentsView(memberKey)
			if !ok || len(segments) != 1 {
				continue
			}
			candidateKey, ok := scalarSegmentValue(reg, segments[0])
			if !ok || valueDomain.Equal(valueDomain.Meet(candidateKey, keyValue), valueDomain.Bottom()) {
				continue
			}
			value := members[memberKey]
			if valueDomain.Equal(value, valueDomain.Bottom()) {
				continue
			}
			if !found {
				joined = value
				found = true
			} else {
				joined = valueDomain.Join(joined, value)
			}
		}
	}
	facts := object.DynamicIndexFacts()
	keys := make([]dynamicindex.Key, 0, len(facts))
	for key := range facts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Table != keys[j].Table {
			return ks.Less(keys[i].Table, keys[j].Table)
		}
		return keys[i].Site < keys[j].Site
	})
	for _, factKey := range keys {
		fact := facts[factKey]
		if fact.Admission == dynamicindex.AdmissionRejected ||
			valueDomain.Equal(fact.KeyValue, valueDomain.Bottom()) ||
			valueDomain.Equal(fact.Value, valueDomain.Bottom()) ||
			valueDomain.Equal(valueDomain.Meet(fact.KeyValue, keyValue), valueDomain.Bottom()) {
			continue
		}
		if !found {
			joined = fact.Value
			found = true
		} else {
			joined = valueDomain.Join(joined, fact.Value)
		}
	}
	if found {
		if !hasExactSegment {
			joined = valueDomain.Join(joined, typevalue.Nil(reg))
		}
		return joined, true
	}
	if !hasExactSegment && object.StableShape() {
		return typevalue.Nil(reg), true
	}
	// Absence is only exact for a final-shape object whose finite dynamic fact
	// map is known complete. A prefix-stable or mutable object cannot prove it.
	if hasExactSegment && object.StableShape() {
		return typevalue.Nil(reg), true
	}
	return product.Value{}, false
}

func scalarSegmentValue(reg *axis.Registry, seg segment.Segment) (product.Value, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return typevalue.LiteralString(reg, seg.Name), true
	case segment.SegmentIndexInt:
		return typevalue.LiteralInt(reg, int64(seg.Index)), true
	default:
		return product.Value{}, false
	}
}
