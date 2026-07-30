package factapply

import (
	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// ResolvedPathValueReader is the carrier-neutral query contract for the one
// canonical resolved-path projection. Concrete State and formal tuples bind
// these four registered roles without reconstructing the 17-axis product.
type ResolvedPathValueReader interface {
	ReadRootValue(symbol.ID) (product.Value, bool)
	ReadLocalPathValue(keyspace.Key) (product.Value, bool)
	ReadDynamicIndexTable(keyspace.Key) (state.DynamicIndexTableEvidence, bool)
	ReadHeapObject(identity.Term) (heapidentity.TableObject, bool)
}

// ResolvedStructuralPath is the keyspace-native path authority shared by
// concrete resolver addresses and formal-root coordinates.
type ResolvedStructuralPath struct {
	keys     *keyspace.KeySpace
	local    keyspace.Key
	prefixes []keyspace.Key
	root     symbol.ID
	segments []segment.Segment
}

// FreezeResolvedStructuralPath seals one structural key and its finite prefix
// chain. root is syntax provenance used only for Values/variant-origin lookup.
func FreezeResolvedStructuralPath(keys *keyspace.KeySpace, local keyspace.Key, root symbol.ID) (ResolvedStructuralPath, error) {
	if keys == nil || !keys.Valid() || root == 0 {
		return ResolvedStructuralPath{}, state.ErrInvalidLaneFactor
	}
	segments, ok := keys.SegmentsView(local)
	if !ok {
		return ResolvedStructuralPath{}, state.ErrInvalidLaneFactor
	}
	base, ok := keys.StructuralRoot(local)
	if !ok {
		return ResolvedStructuralPath{}, state.ErrInvalidLaneFactor
	}
	prefixes := make([]keyspace.Key, len(segments)+1)
	prefixes[0] = base
	for index, item := range segments {
		base, ok = keys.AppendSegment(base, item)
		if !ok {
			return ResolvedStructuralPath{}, state.ErrInvalidLaneFactor
		}
		prefixes[index+1] = base
	}
	if prefixes[len(prefixes)-1] != local {
		return ResolvedStructuralPath{}, state.ErrInvalidLaneFactor
	}
	return ResolvedStructuralPath{keys: keys, local: local, prefixes: prefixes, root: root, segments: append([]segment.Segment(nil), segments...)}, nil
}

type concreteResolvedPathValueReader struct {
	domain  state.ProductDomain
	keys    *keyspace.KeySpace
	values  state.ValueLaneFactor
	path    state.LaneFactor
	dynamic state.LaneFactor
	heap    state.LaneFactor
}

func newConcreteResolvedPathValueReader(reg *axis.Registry, keys *keyspace.KeySpace, input state.State) (concreteResolvedPathValueReader, bool) {
	if reg == nil || keys == nil || !keys.Valid() {
		return concreteResolvedPathValueReader{}, false
	}
	domain := state.RegisteredProductDomain(reg)
	pathFamily, ok := domain.PathValueFamily()
	if !ok {
		return concreteResolvedPathValueReader{}, false
	}
	dynamicLane, dynamicOK := domain.ProductLane(state.LaneDynamicIndex)
	heapLane, heapOK := domain.ProductLane(state.LaneHeapTableIdentity)
	if !dynamicOK || !heapOK {
		return concreteResolvedPathValueReader{}, false
	}
	factors, err := domain.DecomposeLanes(input, []state.ProductLane{pathFamily.Lane(), dynamicLane, heapLane})
	if err != nil {
		return concreteResolvedPathValueReader{}, false
	}
	_, values := state.DecomposeValueLane(domain.Lattice(), input)
	return concreteResolvedPathValueReader{
		domain: domain, keys: keys, values: values, path: factors[0], dynamic: factors[1], heap: factors[2],
	}, true
}

func (r concreteResolvedPathValueReader) ReadRootValue(id symbol.ID) (product.Value, bool) {
	if id == 0 {
		return product.Value{}, false
	}
	if r.values.Top {
		return product.Top(), true
	}
	value, ok := r.values.Values[statekey.SymbolValue(id)]
	if !ok {
		value = product.Bottom(r.domain.Registry())
	}
	return value, true
}

func (r concreteResolvedPathValueReader) ReadLocalPathValue(path keyspace.Key) (product.Value, bool) {
	value, present, err := r.domain.ReadPathValueFactor(r.path, r.keys, path)
	return value, present && err == nil
}

func (r concreteResolvedPathValueReader) ReadDynamicIndexTable(table keyspace.Key) (state.DynamicIndexTableEvidence, bool) {
	evidence, err := r.domain.ObserveDynamicIndexTableFactor(r.dynamic, table)
	return evidence, err == nil
}

func (r concreteResolvedPathValueReader) ReadHeapObject(term identity.Term) (heapidentity.TableObject, bool) {
	object, err := r.domain.ReadHeapTableObjectTermFactor(r.heap, term)
	return object, err == nil
}

// ResolvePathAddressFactorValue is the sole recursive path projection. Its
// recursion strictly shortens the frozen suffix and queries only registered
// role cones.
func ResolvePathAddressFactorValue(reg *axis.Registry, keys *keyspace.KeySpace, reader ResolvedPathValueReader, address ResolvedPathAddress) (product.Value, bool) {
	if reg == nil || reader == nil || !address.belongsTo(keys) {
		return product.Value{}, false
	}
	path, err := FreezeResolvedStructuralPath(keys, address.local, address.path.Symbol)
	if err != nil {
		return product.Value{}, false
	}
	return ResolveStructuralPathFactorValue(reg, reader, path)
}

// ResolveStructuralPathFactorValue resolves concrete or formal paths through
// the same registered query composition.
func ResolveStructuralPathFactorValue(reg *axis.Registry, reader ResolvedPathValueReader, path ResolvedStructuralPath) (product.Value, bool) {
	if reg == nil || reader == nil || path.keys == nil || !path.keys.Valid() || len(path.prefixes) != len(path.segments)+1 {
		return product.Value{}, false
	}
	return resolveStructuralPathFactorValue(reg, reader, path)
}

func resolveStructuralPathFactorValue(reg *axis.Registry, reader ResolvedPathValueReader, path ResolvedStructuralPath) (product.Value, bool) {
	if len(path.segments) == 0 {
		return reader.ReadRootValue(path.root)
	}
	if value, ok := readResolvedLocalPathFactor(reg, path.keys, reader, path.local); ok {
		return value, true
	}
	if projected, ok := projectResolvedDynamicFactorValue(reg, reader, path); ok {
		return projected, true
	}
	if projected, ok := projectResolvedHeapStaticFactorValue(reg, reader, path); ok {
		return projected, true
	}
	root, ok := reader.ReadRootValue(path.root)
	if !ok {
		return product.Value{}, false
	}
	return projectPathOriginFromRoot(nil, reg, root, pathdom.Path{Symbol: path.root, Segments: path.segments}, nil)
}

func readResolvedLocalPathFactor(reg *axis.Registry, keys *keyspace.KeySpace, reader ResolvedPathValueReader, path keyspace.Key) (product.Value, bool) {
	value, ok := reader.ReadLocalPathValue(path)
	if ok && !product.Equal(reg, value, product.Bottom(reg)) {
		return value, true
	}
	canonical, hasCanonical := keys.FieldCanonical(path)
	if !hasCanonical {
		return product.Value{}, false
	}
	value, ok = reader.ReadLocalPathValue(canonical)
	return value, ok && !product.Equal(reg, value, product.Bottom(reg))
}

func projectResolvedDynamicFactorValue(reg *axis.Registry, reader ResolvedPathValueReader, path ResolvedStructuralPath) (product.Value, bool) {
	last := path.segments[len(path.segments)-1]
	parent, ok := resolvedStructuralPrefix(path, len(path.segments)-1)
	if !ok {
		return product.Value{}, false
	}
	mayMatch := resolvedPathFactorHasPresentProof(reg, path.keys, reader, path.local)
	evidence, observed := reader.ReadDynamicIndexTable(parent.local)
	if observed && !evidence.Top && len(evidence.Facts) != 0 {
		if joined, matched := joinMatchingDynamicIndexTableFacts(reg, evidence.Facts, last, mayMatch); matched {
			heapMayMatch := mayMatch || presence.Equal(product.PresenceOf(joined), presence.Present())
			if _, hasID := product.Get(reg, joined, identity.Key).ID(); !hasID {
				if heapProjected, heapOK := projectResolvedHeapDynamicFactorValue(reg, reader, parent, last, heapMayMatch); heapOK {
					if _, heapHasID := product.Get(reg, heapProjected, identity.Key).ID(); heapHasID {
						if merged := product.Meet(reg, joined, heapProjected); !product.Equal(reg, merged, product.Bottom(reg)) {
							return merged, true
						}
					}
				}
			}
			return joined, true
		}
	}
	return projectResolvedHeapDynamicFactorValue(reg, reader, parent, last, mayMatch)
}

func projectResolvedHeapDynamicFactorValue(reg *axis.Registry, reader ResolvedPathValueReader, parent ResolvedStructuralPath, last segment.Segment, mayMatch bool) (product.Value, bool) {
	parentValue, ok := resolveStructuralPathFactorValue(reg, reader, parent)
	if !ok {
		return product.Value{}, false
	}
	id, ok := identityvalue.ExactTerm(reg, parentValue)
	if !ok {
		if projected, projectedOK := projectResolvedHeapStaticFactorValue(reg, reader, parent); projectedOK {
			if merged := product.Meet(reg, parentValue, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
				parentValue = merged
			} else {
				parentValue = projected
			}
		} else if root, rootOK := reader.ReadRootValue(parent.root); rootOK {
			if projected, originOK := projectPathOriginFromRoot(nil, reg, root, pathdom.Path{Symbol: parent.root, Segments: parent.segments}, nil); originOK {
				parentValue = projected
			}
		}
		id, ok = identityvalue.ExactTerm(reg, parentValue)
		if !ok {
			return product.Value{}, false
		}
	}
	object, ok := reader.ReadHeapObject(id)
	if !ok {
		return product.Value{}, false
	}
	return joinMatchingHeapDynamicIndexValues(reg, object.DynamicIndexFacts(), last, mayMatch)
}

func projectResolvedHeapStaticFactorValue(reg *axis.Registry, reader ResolvedPathValueReader, path ResolvedStructuralPath) (product.Value, bool) {
	root, rootOK := reader.ReadRootValue(path.root)
	rootProjected, hasRootProjected := heapMemberFromFactorValue(reg, path.keys, reader, root, path.segments)
	parent, ok := resolvedStructuralPrefix(path, len(path.segments)-1)
	if !ok {
		return rootProjected, rootOK && hasRootProjected
	}
	parentValue, parentOK := resolveStructuralPathFactorValue(reg, reader, parent)
	if parentOK {
		if projected, projectedOK := heapMemberFromFactorValue(reg, path.keys, reader, parentValue, path.segments[len(path.segments)-1:]); projectedOK {
			if hasRootProjected {
				if merged := product.Meet(reg, rootProjected, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
					return merged, true
				}
			}
			return projected, true
		}
	}
	return rootProjected, hasRootProjected
}

func resolvedStructuralPrefix(path ResolvedStructuralPath, count int) (ResolvedStructuralPath, bool) {
	if count < 0 || count > len(path.segments) || len(path.prefixes) != len(path.segments)+1 {
		return ResolvedStructuralPath{}, false
	}
	return ResolvedStructuralPath{
		keys: path.keys, local: path.prefixes[count], prefixes: path.prefixes[:count+1], root: path.root,
		segments: path.segments[:count],
	}, true
}

func heapMemberFromFactorValue(reg *axis.Registry, keys *keyspace.KeySpace, reader ResolvedPathValueReader, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	term, ok := identityvalue.ExactTerm(reg, value)
	if !ok {
		return product.Value{}, false
	}
	object, ok := reader.ReadHeapObject(term)
	if !ok {
		return product.Value{}, false
	}
	return sourcevalue.HeapMemberFromObject(reg, keys, object, value, suffix)
}

func resolvedPathFactorHasPresentProof(reg *axis.Registry, keys *keyspace.KeySpace, reader ResolvedPathValueReader, path keyspace.Key) bool {
	for _, candidate := range appendLocalPathKeyWithStaticStringAlias(nil, keys, path) {
		value, ok := reader.ReadLocalPathValue(candidate)
		if ok && !product.Equal(reg, value, product.Bottom(reg)) && presence.Equal(product.PresenceOf(value), presence.Present()) {
			return true
		}
	}
	return false
}

func joinMatchingDynamicIndexTableFacts(reg *axis.Registry, facts []dynamicindex.Fact, last segment.Segment, mayMatch bool) (product.Value, bool) {
	domain := product.Domain(reg)
	joined := product.Bottom(reg)
	found := false
	for _, fact := range facts {
		if fact.Admission == dynamicindex.AdmissionRejected || !dynamicIndexFactCanProjectToStaticSegment(reg, fact, last, mayMatch) || domain.Equal(fact.Value, domain.Bottom()) {
			continue
		}
		if !found {
			joined, found = fact.Value, true
		} else {
			joined = domain.Join(joined, fact.Value)
		}
	}
	return joined, found
}
