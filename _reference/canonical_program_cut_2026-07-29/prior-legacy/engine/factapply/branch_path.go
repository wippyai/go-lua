package factapply

import (
	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/internal/typegraph"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type pathValue struct {
	value  product.Value
	target pathValueTarget
}

type pathValueTargetKind uint8

const (
	pathValueTargetNone pathValueTargetKind = iota
	pathValueTargetSlot
	pathValueTargetPathKey
)

type pathValueTarget struct {
	kind    pathValueTargetKind
	slot    key.Value
	ks      *keyspace.KeySpace
	pathKey pathdom.PathKey
}

func (v pathValue) write(reg *axis.Registry, out state.State, value product.Value) (state.State, bool) {
	switch v.target.kind {
	case pathValueTargetSlot:
		return out.WriteValue(reg, v.target.slot, value), true
	case pathValueTargetPathKey:
		if v.target.ks == nil || v.target.pathKey == "" {
			return out, false
		}
		return out.WritePathKey(reg, v.target.ks, v.target.pathKey, value), true
	default:
		return out, false
	}
}

func resolvePathValueAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	projectPath PathTypeProjector,
) (pathValue, bool) {
	return resolvePathValueAtCached(nil, reg, resolver, point, out, targetPath, projectPath)
}

func resolvePathValueAtCached(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	projectPath PathTypeProjector,
) (pathValue, bool) {
	if targetPath.Symbol == 0 {
		return pathValue{}, false
	}
	if len(targetPath.Segments) == 0 {
		slot := key.SymbolValue(targetPath.Symbol)
		return pathValue{
			value: out.ReadValue(reg, slot),
			target: pathValueTarget{
				kind: pathValueTargetSlot,
				slot: slot,
			},
		}, true
	}
	if resolver == nil {
		return pathValue{}, false
	}
	pathKey := factPathKeyAt(resolver, point, targetPath)
	if pathKey == "" {
		return pathValue{}, false
	}
	ks := resolver.KeySpace()
	value := out.ReadPathKey(reg, ks, pathKey)
	if product.Equal(reg, value, product.Bottom(reg)) {
		projected, ok := projectPathFallbackValue(typeValues, reg, resolver, point, out, targetPath, projectPath)
		if ok {
			value = projected
		} else {
			return pathValue{}, false
		}
	}
	return pathValue{
		value: value,
		target: pathValueTarget{
			kind:    pathValueTargetPathKey,
			ks:      ks,
			pathKey: pathKey,
		},
	}, true
}

func projectPathFallbackValue(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	projectPath PathTypeProjector,
) (product.Value, bool) {
	if resolver != nil && targetPath.Symbol != 0 && len(targetPath.Segments) != 0 {
		root := out.ReadValue(reg, key.SymbolValue(targetPath.Symbol))
		if query, ok := sourcevalue.BindStaticPathRead(reg, typeValues, resolver.KeySpace(), resolver, point, targetPath, root); ok {
			if evidence, err := state.RegisteredProductDomain(reg).ProjectDynamicReadEvidence(query, out); err == nil {
				if projected, resolved := sourcevalue.ResolveDynamicRead(query, evidence); resolved &&
					!product.Equal(reg, projected, product.Bottom(reg)) {
					return projected, true
				}
			}
		}
	}
	if projected, ok := projectPathStructuralValueCached(typeValues, reg, out, targetPath, projectPath); ok {
		return projected, true
	}
	if projected, ok := projectPathOriginValue(typeValues, reg, out, targetPath, projectPath); ok {
		return projected, true
	}
	return product.Value{}, false
}

func projectPathHeapStaticMemberValue(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) (product.Value, bool) {
	if len(targetPath.Segments) == 0 {
		return product.Value{}, false
	}
	root := targetPath.RootOnly()
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := resolvePathValueAtCached(nil, reg, resolver, point, out, root, nil); ok {
		if projected, ok := sourcevalue.HeapMemberFromValue(reg, resolver.KeySpace(), out, rootValue.value, targetPath.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}

	parent := targetPath.ParentView()
	parentValue, _ := resolvePathValueAtCached(nil, reg, resolver, point, out, parent, nil)
	if projected, ok := sourcevalue.HeapMemberFromValue(reg, resolver.KeySpace(), out, parentValue.value, targetPath.Segments[len(targetPath.Segments)-1:]); ok {
		if hasRootProjected {
			if merged := product.Meet(reg, rootProjected, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
				return merged, true
			}
		}
		return projected, true
	}
	if hasRootProjected {
		return rootProjected, true
	}
	return product.Value{}, false
}

func projectPathDynamicIndexValue(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
) (product.Value, bool) {
	if resolver == nil || len(targetPath.Segments) == 0 {
		return product.Value{}, false
	}
	parent := targetPath.ParentView()
	if parent.IsEmpty() {
		return product.Value{}, false
	}
	tableKey, ok := factKeyspaceKeyAt(resolver, point, parent)
	if !ok {
		return product.Value{}, false
	}
	targetKey := factPathKeyAt(resolver, point, targetPath)
	mayMatchAllowed := pathKeyHasPresentProof(reg, resolver.KeySpace(), out, targetKey)
	last := targetPath.Segments[len(targetPath.Segments)-1]
	snapshot := out.DynamicIndexFactsSnapshot()
	if snapshot.Top || len(snapshot.Facts) == 0 {
		return projectPathHeapDynamicIndexValue(reg, resolver, point, out, parent, last, mayMatchAllowed)
	}
	if joined, ok := joinMatchingDynamicIndexValues(reg, snapshot.Facts, tableKey, last, mayMatchAllowed); ok {
		heapMayMatchAllowed := mayMatchAllowed || presence.Equal(product.PresenceOf(joined), presence.Present())
		if _, hasID := product.Get(reg, joined, identity.Key).ID(); !hasID {
			if heapProjected, heapOK := projectPathHeapDynamicIndexValue(reg, resolver, point, out, parent, last, heapMayMatchAllowed); heapOK {
				if _, heapHasID := product.Get(reg, heapProjected, identity.Key).ID(); heapHasID {
					if merged := product.Meet(reg, joined, heapProjected); !product.Equal(reg, merged, product.Bottom(reg)) {
						return merged, true
					}
				}
			}
		}
		return joined, true
	}
	return projectPathHeapDynamicIndexValue(reg, resolver, point, out, parent, last, mayMatchAllowed)
}

func projectPathHeapDynamicIndexValue(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	parent pathdom.Path,
	last segment.Segment,
	mayMatchAllowed bool,
) (product.Value, bool) {
	id, ok := dynamicIndexParentHeapID(reg, resolver, point, out, parent)
	if !ok {
		return product.Value{}, false
	}
	object := out.ReadHeapTableObject(reg, id)
	return joinMatchingHeapDynamicIndexValues(reg, object.DynamicIndexFacts(), last, mayMatchAllowed)
}

func dynamicIndexParentHeapID(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	parent pathdom.Path,
) (identity.ID, bool) {
	parentValue, ok := resolvePathValueAtCached(nil, reg, resolver, point, out, parent, nil)
	if !ok {
		return identity.ID{}, false
	}
	id, ok := product.Get(reg, parentValue.value, identity.Key).ID()
	if ok {
		return id, true
	}
	projected, projectedOK := projectPathHeapStaticMemberValue(reg, resolver, point, out, parent)
	if !projectedOK {
		projected, projectedOK = projectPathOriginValue(nil, reg, out, parent, nil)
	}
	if !projectedOK {
		return identity.ID{}, false
	}
	if merged := product.Meet(reg, parentValue.value, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
		parentValue.value = merged
	} else {
		parentValue.value = projected
	}
	return product.Get(reg, parentValue.value, identity.Key).ID()
}

func joinMatchingDynamicIndexValues(
	reg *axis.Registry,
	facts map[dynamicindex.Key]dynamicindex.Fact,
	tableKey keyspace.Key,
	last segment.Segment,
	mayMatchAllowed bool,
) (product.Value, bool) {
	valueDomain := product.Domain(reg)
	joined := product.Bottom(reg)
	found := false
	for key, fact := range facts {
		if key.Table != tableKey || fact.Admission == dynamicindex.AdmissionRejected {
			continue
		}
		if !dynamicIndexFactCanProjectToStaticSegment(reg, fact, last, mayMatchAllowed) {
			continue
		}
		if valueDomain.Equal(fact.Value, valueDomain.Bottom()) {
			continue
		}
		if !found {
			joined = fact.Value
			found = true
			continue
		}
		joined = valueDomain.Join(joined, fact.Value)
	}
	if !found {
		return product.Value{}, false
	}
	return joined, true
}

func joinMatchingHeapDynamicIndexValues(
	reg *axis.Registry,
	facts map[dynamicindex.Key]dynamicindex.Fact,
	last segment.Segment,
	mayMatchAllowed bool,
) (product.Value, bool) {
	valueDomain := product.Domain(reg)
	joined := product.Bottom(reg)
	found := false
	for _, fact := range facts {
		if fact.Admission == dynamicindex.AdmissionRejected {
			continue
		}
		if !dynamicIndexFactCanProjectToStaticSegment(reg, fact, last, mayMatchAllowed) {
			continue
		}
		if valueDomain.Equal(fact.Value, valueDomain.Bottom()) {
			continue
		}
		if !found {
			joined = fact.Value
			found = true
			continue
		}
		joined = valueDomain.Join(joined, fact.Value)
	}
	if !found {
		return product.Value{}, false
	}
	return joined, true
}

func pathKeyHasPresentProof(reg *axis.Registry, ks *keyspace.KeySpace, out state.State, pathKey pathdom.PathKey) bool {
	if pathKey == "" {
		return false
	}
	localKey, ok := ks.FromPathKey(pathKey)
	if !ok {
		value := out.ReadPathKey(reg, ks, pathKey)
		return !product.Equal(reg, value, product.Bottom(reg)) &&
			presence.Equal(product.PresenceOf(value), presence.Present())
	}
	for _, key := range appendLocalPathKeyWithStaticStringAlias(nil, ks, localKey) {
		value := out.ReadLocalPathKey(reg, key)
		if !product.Equal(reg, value, product.Bottom(reg)) &&
			presence.Equal(product.PresenceOf(value), presence.Present()) {
			return true
		}
	}
	return false
}

func dynamicIndexFactMayMatchSegment(reg *axis.Registry, fact dynamicindex.Fact, seg segment.Segment) bool {
	keyType, ok := typevalue.TypeOf(reg, fact.KeyValue)
	if !ok {
		return true
	}
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return typetable.MapComponentKeyMayContainString(keyType, seg.Name)
	case segment.SegmentIndexInt:
		return typetable.MapComponentKeyMayContainInt(keyType, int64(seg.Index))
	default:
		return true
	}
}

func dynamicIndexFactCanProjectToStaticSegment(reg *axis.Registry, fact dynamicindex.Fact, seg segment.Segment, mayMatchAllowed bool) bool {
	if dynamicIndexFactDefinitelyMatchesSegment(reg, fact, seg) {
		return true
	}
	return mayMatchAllowed && dynamicIndexFactMayMatchSegment(reg, fact, seg)
}

func dynamicIndexFactDefinitelyMatchesSegment(reg *axis.Registry, fact dynamicindex.Fact, seg segment.Segment) bool {
	keyType, ok := typevalue.TypeOf(reg, fact.KeyValue)
	if !ok {
		return false
	}
	exact, _ := keyTypeDefinitelyMatchesSegment(keyType, seg)
	return exact
}

func keyTypeDefinitelyMatchesSegment(t typ.Type, seg segment.Segment) (exact bool, definitelyNot bool) {
	exact, definitelyNot, productive := keyTypeDefinitelyMatchesSegmentSeen(t, seg, &typegraph.Path{})
	if !productive {
		return false, false
	}
	return exact, definitelyNot
}

func keyTypeDefinitelyMatchesSegmentSeen(t typ.Type, seg segment.Segment, active *typegraph.Path) (exact bool, definitelyNot bool, productive bool) {
	if t == nil {
		return false, false, true
	}
	if !active.Enter(t) {
		return false, false, false
	}
	defer active.Leave(t)
	switch tt := t.(type) {
	case *typ.Annotated:
		return keyTypeDefinitelyMatchesSegmentSeen(tt.Inner, seg, active)
	case *typ.Alias:
		return keyTypeDefinitelyMatchesSegmentSeen(tt.UnaliasedTarget(), seg, active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return false, false, false
		}
		return keyTypeDefinitelyMatchesSegmentSeen(expanded, seg, active)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return false, false, false
		}
		return keyTypeDefinitelyMatchesSegmentSeen(tt.Body, seg, active)
	case *typ.Literal:
		exact, definitelyNot := literalKeyDefinitelyMatchesSegment(tt, seg)
		return exact, definitelyNot, true
	case *typ.Optional:
		// Optional keys include nil, so the key cannot definitely address a
		// concrete string/int slot even if the payload is exact.
		return false, false, true
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false, false, true
		}
		allExact := true
		allNot := true
		productive := false
		for _, member := range tt.Members {
			memberExact, memberNot, memberProductive := keyTypeDefinitelyMatchesSegmentSeen(member, seg, active)
			if !memberProductive {
				continue
			}
			productive = true
			allExact = allExact && memberExact
			allNot = allNot && memberNot
		}
		return allExact, allNot, productive
	case *typ.Intersection:
		foundExact := false
		productive := false
		for _, member := range tt.Members {
			memberExact, memberNot, memberProductive := keyTypeDefinitelyMatchesSegmentSeen(member, seg, active)
			if !memberProductive {
				continue
			}
			productive = true
			if memberNot {
				return false, true, true
			}
			foundExact = foundExact || memberExact
		}
		return foundExact, false, productive
	default:
		switch tt.Kind() {
		case kind.String:
			return false, seg.Kind == segment.SegmentIndexInt, true
		case kind.Integer, kind.Number:
			return false, seg.Kind == segment.SegmentField || seg.Kind == segment.SegmentIndexString, true
		case kind.Any, kind.Unknown, kind.Never:
			return false, false, true
		default:
			return false, true, true
		}
	}
}

func literalKeyDefinitelyMatchesSegment(lit *typ.Literal, seg segment.Segment) (exact bool, definitelyNot bool) {
	if lit == nil {
		return false, false
	}
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if lit.Base != kind.String {
			return false, true
		}
		name, ok := lit.Value.(string)
		return ok && name == seg.Name, !ok || name != seg.Name
	case segment.SegmentIndexInt:
		switch lit.Base {
		case kind.Integer:
			index, ok := lit.Value.(int64)
			return ok && index == int64(seg.Index), !ok || index != int64(seg.Index)
		case kind.Number:
			number, ok := lit.Value.(float64)
			return ok && number == float64(seg.Index), !ok || number != float64(seg.Index)
		default:
			return false, true
		}
	default:
		return false, false
	}
}

func projectPathStructuralValueCached(typeValues *typevalue.Cache, reg *axis.Registry, out state.State, targetPath pathdom.Path, projectPath PathTypeProjector) (product.Value, bool) {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return product.Value{}, false
	}
	if projectPath == nil {
		return product.Value{}, false
	}
	root := out.ReadValue(reg, key.SymbolValue(targetPath.Symbol))
	if product.Equal(reg, root, product.Bottom(reg)) {
		return product.Value{}, false
	}
	rootType, ok := typevalue.StructuralTypeOf(reg, typeValues, root, typevalue.StructuralTypeOptions{
		ApplyPresence: true,
	})
	if !ok {
		return product.Value{}, false
	}
	projected, ok := projectPath(rootType, targetPath)
	if !ok {
		return product.Value{}, false
	}
	return projectedPathValue(reg, typeValues, projected), true
}

func projectPathOriginValue(typeValues *typevalue.Cache, reg *axis.Registry, out state.State, targetPath pathdom.Path, projectPath PathTypeProjector) (product.Value, bool) {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return product.Value{}, false
	}
	root := out.ReadValue(reg, key.SymbolValue(targetPath.Symbol))
	return projectPathOriginFromRoot(typeValues, reg, root, targetPath, projectPath)
}

func projectPathOriginFromRoot(typeValues *typevalue.Cache, reg *axis.Registry, root product.Value, targetPath pathdom.Path, projectPath PathTypeProjector) (product.Value, bool) {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return product.Value{}, false
	}
	return projectPathOriginFromRootSegments(typeValues, reg, root, targetPath.Segments, projectPath)
}

// projectStructuralSegments is the single language-projection boundary for
// keyspace-native structural suffixes. Root identity is intentionally absent:
// type projection depends only on the sealed member sequence, so formal and
// concrete roots execute the same law without decoding either root spelling.
func projectStructuralSegments(projectPath PathTypeProjector, root typ.Type, segments []segment.Segment) (typ.Type, bool) {
	if projectPath == nil || len(segments) == 0 {
		return nil, false
	}
	return projectPath(root, pathdom.Path{Segments: segments})
}

func projectPathOriginFromRootSegments(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	root product.Value,
	segments []segment.Segment,
	projectPath PathTypeProjector,
) (product.Value, bool) {
	if len(segments) == 0 {
		return product.Value{}, false
	}
	if product.Equal(reg, root, product.Bottom(reg)) {
		return product.Value{}, false
	}
	origin := product.Get(reg, root, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return product.Value{}, false
	}
	if projectPath != nil {
		if rootType, ok := typeValues.TypeFromVariantOriginView(origin.Family(), origin.CasesView()); ok {
			if projected, ok := projectStructuralSegments(projectPath, rootType, segments); ok {
				value := projectedPathValue(reg, typeValues, projected)
				if family, cases, ok := variant.ProjectOriginView(origin.Family(), origin.CasesView(), segments); ok {
					value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(family, cases))
				}
				return value, true
			}
		}
	}
	family, cases, ok := variant.ProjectOriginView(origin.Family(), origin.CasesView(), segments)
	if !ok {
		return product.Value{}, false
	}
	return product.Set(reg, product.Top(), variantorigin.Key, variantorigin.Of(family, cases)), true
}

func projectedPathValue(reg *axis.Registry, typeValues *typevalue.Cache, t typ.Type) product.Value {
	value := typeValues.FromTypeWithWitness(reg, t)
	if t != nil && !typevalue.ProjectionHasNil(t) {
		value = product.WithPresence(reg, value, presence.Present())
	}
	return value
}

func invalidateRootDescendantsAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	rootPath pathdom.Path,
) state.State {
	if resolver == nil || rootPath.Symbol == 0 || len(rootPath.Segments) != 0 {
		return out
	}
	invalidated, ok := invalidatePathDescendantsAt(out, resolver, point, rootPath)
	if !ok {
		return out
	}
	return invalidated
}
