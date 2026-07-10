package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/typenarrow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/kind"
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

func applyBranchPathRelation(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	relation factflow.BranchPathRelation,
) state.State {
	switch relation.Kind() {
	case factflow.BranchPathRelationEqual:
		return applyBranchPathEquality(typeValues, ctx, resolver, projectPath, out, relation.LeftPath(), relation.RightPath())
	case factflow.BranchPathRelationNotEqual:
		return applyBranchPathInequality(typeValues, ctx, resolver, projectPath, out, relation.LeftPath(), relation.RightPath())
	case factflow.BranchPathRelationTypeMatch:
		return applyBranchTypeComparison(typeValues, ctx, resolver, projectPath, out, relation.LeftPath(), relation.RightPath(), true)
	case factflow.BranchPathRelationTypeUnmatch:
		return applyBranchTypeComparison(typeValues, ctx, resolver, projectPath, out, relation.LeftPath(), relation.RightPath(), false)
	default:
		return out
	}
}

// applyBranchTypeComparison narrows subjectPath by the runtime type named by
// namePath's value, resolved at the branch point. When namePath resolves to a
// single string-literal type its value names the runtime kind to match (or
// exclude). A non-literal type name (general string, a union of names, or an
// unresolved value) leaves the subject unchanged, preserving soundness.
func applyBranchTypeComparison(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	subjectPath pathdom.Path,
	namePath pathdom.Path,
	match bool,
) state.State {
	resolved, ok := resolvePathValueAtCached(typeValues, ctx.Registry, resolver, ctx.Edge.From, out, namePath, projectPath)
	if !ok {
		return out
	}
	nameType, ok := typevalue.TypeOf(ctx.Registry, resolved.value)
	if !ok {
		return out
	}
	tag, ok := typenarrow.RuntimeKindTagForType(nameType)
	if !ok {
		return out
	}
	refinement := typenarrow.UnmatchRefinement(ctx.Registry, tag)
	if match {
		refinement = typenarrow.MatchRefinement(ctx.Registry, tag)
	}
	return applyBranchRefinementCached(typeValues, ctx, resolver, projectPath, out, subjectPath, refinement)
}

func applyBranchPathEquality(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	if selected, ok := applyChannelSelectCaseEquality(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, leftPath, rightPath); ok {
		return selected
	}
	return applyPathEqualityAtCached(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, leftPath, rightPath)
}

func applyPathEqualityAt(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	return applyPathEqualityAtCached(nil, reg, resolver, projectPath, point, out, leftPath, rightPath)
}

func applyPathEqualityAtCached(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	if leftKey, leftOK := factKeyspaceKeyAt(resolver, point, leftPath); leftOK {
		if rightKey, rightOK := factKeyspaceKeyAt(resolver, point, rightPath); rightOK {
			out = closeBranchProofsAcrossEquality(resolver.KeySpace(), out, leftKey, rightKey)
		}
	}
	out = applyPathOriginRelation(typeValues, reg, resolver, projectPath, point, out, leftPath, rightPath, true)
	if stateIsBottom(reg, out) {
		return out
	}
	out = applyPathOriginRelation(typeValues, reg, resolver, projectPath, point, out, rightPath, leftPath, true)
	if stateIsBottom(reg, out) {
		return out
	}
	left, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, leftPath, projectPath)
	if !ok {
		return out
	}
	right, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, rightPath, projectPath)
	if !ok {
		return out
	}
	meet := product.Meet(reg, left.value, right.value)
	if product.Equal(reg, meet, product.Bottom(reg)) {
		return unreachableState(reg)
	}
	if written, ok := left.write(reg, out, meet); ok {
		out = written
	}
	if written, ok := right.write(reg, out, meet); ok {
		out = written
	}
	return out
}

func applyBranchPathInequality(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	if selected, ok := applyChannelSelectCaseInequality(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, leftPath, rightPath); ok {
		return selected
	}
	out = applyPathOriginRelation(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, leftPath, rightPath, false)
	out = applyPathOriginRelation(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, rightPath, leftPath, false)
	return out
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
	if projected, ok := projectPathDynamicIndexValue(reg, resolver, point, out, targetPath); ok {
		return projected, true
	}
	if projected, ok := projectPathHeapStaticMemberValue(reg, resolver, point, out, targetPath); ok {
		return projected, true
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
	exact, _ := keyTypeDefinitelyMatchesSegment(keyType, seg, 0)
	return exact
}

func keyTypeDefinitelyMatchesSegment(t typ.Type, seg segment.Segment, depth int) (exact bool, definitelyNot bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false, false
	}
	t = keyProofTransparentType(t, depth)
	switch tt := t.(type) {
	case nil:
		return false, false
	case *typ.Literal:
		return literalKeyDefinitelyMatchesSegment(tt, seg)
	case *typ.Optional:
		// Optional keys include nil, so the key cannot definitely address a
		// concrete string/int slot even if the payload is exact.
		return false, false
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false, false
		}
		allExact := true
		allNot := true
		for _, member := range tt.Members {
			memberExact, memberNot := keyTypeDefinitelyMatchesSegment(member, seg, depth+1)
			allExact = allExact && memberExact
			allNot = allNot && memberNot
		}
		return allExact, allNot
	case *typ.Intersection:
		foundExact := false
		for _, member := range tt.Members {
			memberExact, memberNot := keyTypeDefinitelyMatchesSegment(member, seg, depth+1)
			if memberNot {
				return false, true
			}
			foundExact = foundExact || memberExact
		}
		return foundExact, false
	default:
		switch tt.Kind() {
		case kind.String:
			return false, seg.Kind == segment.SegmentIndexInt
		case kind.Integer, kind.Number:
			return false, seg.Kind == segment.SegmentField || seg.Kind == segment.SegmentIndexString
		case kind.Any, kind.Unknown, kind.Never:
			return false, false
		default:
			return false, true
		}
	}
}

func keyProofTransparentType(t typ.Type, depth int) typ.Type {
	for i := depth; i <= typ.DefaultRecursionDepth; i++ {
		switch tt := t.(type) {
		case *typ.Annotated:
			if tt.Inner == nil || tt.Inner == t {
				return t
			}
			t = tt.Inner
		case *typ.Alias:
			next := tt.UnaliasedTarget()
			if next == nil || next == t {
				return next
			}
			t = next
		default:
			return t
		}
	}
	return nil
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
	if product.Equal(reg, root, product.Bottom(reg)) {
		return product.Value{}, false
	}
	origin := product.Get(reg, root, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		return product.Value{}, false
	}
	if projectPath != nil {
		if rootType, ok := typeValues.TypeFromVariantOrigin(origin.Family(), origin.CasesRef()); ok {
			if projected, ok := projectPath(rootType, targetPath); ok {
				value := projectedPathValue(reg, typeValues, projected)
				if family, cases, ok := variant.ProjectOrigin(origin.Family(), origin.CasesRef(), targetPath.Segments); ok {
					value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(family, cases))
				}
				return value, true
			}
		}
	}
	family, cases, ok := variant.ProjectOrigin(origin.Family(), origin.CasesRef(), targetPath.Segments)
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

func applyPathOriginRelation(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	parentPath pathdom.Path,
	constraintPath pathdom.Path,
	equal bool,
) state.State {
	if parentPath.Symbol == 0 || len(parentPath.Segments) == 0 {
		return out
	}
	constraint, ok := resolvePathValueAt(reg, resolver, point, out, constraintPath, projectPath)
	if !ok {
		return out
	}
	slot := key.SymbolValue(parentPath.Symbol)
	root := out.ReadValue(reg, slot)
	if product.Equal(reg, root, product.Bottom(reg)) {
		return out
	}
	rootOrigin, ok := typevalue.VariantOriginOfValue(reg, typeValues, root)
	if !ok {
		return out
	}
	cases, ok := narrowOriginCasesByPathConstraint(typeValues, reg, rootOrigin, parentPath.Segments, constraint.value, equal)
	if !ok {
		return out
	}
	if len(cases) == 0 {
		return unreachableState(reg)
	}
	narrowed := variantorigin.Of(rootOrigin.Family(), cases)
	rootPath := parentPath.RootOnly()
	out = invalidateRootDescendantsAt(resolver, point, out, rootPath)
	return out.WriteValue(reg, slot, product.Set(reg, root, variantorigin.Key, narrowed))
}

func narrowOriginCasesByPathConstraint(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	rootOrigin variantorigin.Value,
	suffix []segment.Segment,
	constraint product.Value,
	equal bool,
) ([]int, bool) {
	if constraintOrigin, ok := typevalue.VariantOriginOfValue(reg, typeValues, constraint); ok {
		return variant.NarrowOriginByPath(
			rootOrigin.Family(),
			rootOrigin.CasesRef(),
			suffix,
			constraintOrigin.Family(),
			constraintOrigin.CasesRef(),
			equal,
		)
	}
	if constraintType, ok := typevalue.TypeOf(reg, constraint); ok {
		return variant.NarrowOriginByPathType(
			rootOrigin.Family(),
			rootOrigin.CasesRef(),
			suffix,
			constraintType,
			equal,
		)
	}
	return nil, false
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
