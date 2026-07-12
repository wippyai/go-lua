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
// table read after its path-root owner and key operands have been resolved.
// It is a syntax-free read kernel available to concrete and symbolic call
// boundaries. Missing concrete evidence uses the same sound type-index
// projection as the concrete reader and never claims membership/presence.
//
// tableValue is the owner at tablePath's root when tablePath has suffixes. For
// a root-only tablePath it is the table itself. Suffix projection uses exact
// path/heap evidence first and the concrete reader's sound RuntimeIndex type
// fallback second.
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
	return readBoundDynamicIndexValue(reg, typeValues, ks, resolver, point, tablePath, tableValue, keyValue, in, true)
}

// ReadBoundDynamicTableValue performs a dynamic read when tableValue is
// already the value at tablePath. The path remains authoritative for exact
// flow evidence, but is never projected from tableValue a second time.
func ReadBoundDynamicTableValue(
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
	return readBoundDynamicIndexValue(reg, typeValues, ks, resolver, point, tablePath, tableValue, keyValue, in, false)
}

func readBoundDynamicIndexValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	ks *keyspace.KeySpace,
	resolver *visibility.Resolver,
	point cfg.Point,
	tablePath pathdom.Path,
	tableValue product.Value,
	keyValue product.Value,
	in state.State,
	projectPath bool,
) (product.Value, bool) {
	if reg == nil || ks == nil || tablePath.IsEmpty() {
		return product.Value{}, false
	}
	if projectPath && len(tablePath.Segments) != 0 {
		ownerPath := tablePath
		ownerPath = tablePath.ParentView()
		if resolver != nil {
			if owner, ok := ReadPathValue(reg, resolver, point, ownerPath, in); ok &&
				product.Equal(reg, product.Meet(reg, owner, tableValue), product.Bottom(reg)) {
				return product.Value{}, false
			}
		}
		if resolver != nil {
			if projectedTable, ok := ReadPathValue(reg, resolver, point, tablePath, in); ok {
				tableValue = projectedTable
			} else if projectedTable, ok := projectBoundTablePath(reg, typeValues, ks, in, tableValue, tablePath.Segments); ok {
				tableValue = projectedTable
			} else {
				return product.Value{}, false
			}
		} else if projectedTable, ok := projectBoundTablePath(reg, typeValues, ks, in, tableValue, tablePath.Segments); ok {
			tableValue = projectedTable
		} else {
			return product.Value{}, false
		}
	} else if projectPath && resolver != nil {
		projectedTable, ok := ReadPathValue(reg, resolver, point, tablePath, in)
		if !ok || product.Equal(reg, product.Meet(reg, projectedTable, tableValue), product.Bottom(reg)) {
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
		return runtimeDynamicIndexValue(reg, typeValues, tableValue, keyValue)
	}
	object := in.ReadHeapTableObject(reg, id)
	if object.IsBottom() || object.DynamicIndexFactsTop() {
		return runtimeDynamicIndexValue(reg, typeValues, tableValue, keyValue)
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
			return runtimeDynamicIndexValue(reg, typeValues, tableValue, keyValue)
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
	return runtimeDynamicIndexValue(reg, typeValues, tableValue, keyValue)
}

func projectBoundTablePath(reg *axis.Registry, typeValues *typevalue.Cache, ks *keyspace.KeySpace, in state.State, root product.Value, suffix []segment.Segment) (product.Value, bool) {
	if projected, ok := HeapMemberFromValue(reg, ks, in, root, suffix); ok {
		return projected, true
	}
	value := root
	for _, seg := range suffix {
		keyValue, ok := scalarSegmentValue(reg, seg)
		if !ok {
			return product.Value{}, false
		}
		value, ok = runtimeDynamicIndexValue(reg, typeValues, value, keyValue)
		if !ok {
			return product.Value{}, false
		}
	}
	return value, true
}

func runtimeDynamicIndexValue(reg *axis.Registry, typeValues *typevalue.Cache, tableValue, keyValue product.Value) (product.Value, bool) {
	value, ok := typeValues.RuntimeIndex(reg, tableValue, keyValue)
	if !ok {
		return product.Value{}, false
	}
	return InheritTopOriginEvidence(reg, value, tableValue), true
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
