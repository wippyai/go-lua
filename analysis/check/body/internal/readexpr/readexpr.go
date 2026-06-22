// Package readexpr adapts Lua expression paths to check-body state reads.
package readexpr

import (
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type Config struct {
	Registry   *axis.Registry
	Facts      factflow.Facts
	Visibility *visibility.Resolver
	TypeValues *typevalue.Cache
}

func Provider(config Config) sourcevalue.ExpressionValueProvider {
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	return func(point cfg.Point, expr factflow.ExprRef, _ factflow.ValueSource, in state.State) (product.Value, bool) {
		p, ok := config.Facts.ExpressionPath(expr)
		if ok {
			return Project(config, point, p, in)
		}
		dyn, ok := config.Facts.DynamicIndexExpression(expr)
		if !ok {
			return product.Value{}, false
		}
		p, ok = dynamicIndexExpressionPath(config, point, dyn, in)
		if !ok {
			return product.Value{}, false
		}
		return Project(config, point, p, in)
	}
}

func dynamicIndexExpressionPath(config Config, point cfg.Point, dyn factflow.DynamicIndexExpression, in state.State) (pathdom.Path, bool) {
	keyValue, ok := dynamicIndexExpressionKeyValue(config, point, dyn.KeySource(), in)
	if !ok {
		return pathdom.Path{}, false
	}
	name, ok := staticStringKey(config.Registry, keyValue)
	if !ok {
		return pathdom.Path{}, false
	}
	return dyn.TablePath().IndexStr(name), true
}

func staticStringKey(reg *axis.Registry, value product.Value) (string, bool) {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return "", false
	}
	lit, ok := unwrap.Alias(t).(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return "", false
	}
	name, ok := lit.Value.(string)
	return name, ok
}

func dynamicIndexExpressionKeyValue(config Config, point cfg.Point, source factflow.ValueSource, in state.State) (product.Value, bool) {
	switch source.Kind {
	case factflow.ValueSourceExpression:
		if !source.HasExpr {
			return product.Value{}, false
		}
		if p, ok := config.Facts.ExpressionPath(source.ExprRef); ok {
			return Project(config, point, p, in)
		}
		value, ok := config.Facts.ExpressionValue(source.ExprRef)
		return value, ok
	case factflow.ValueSourceNil:
		return typevalue.Nil(config.Registry), true
	default:
		return product.Value{}, false
	}
}

func Project(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	return project(config, point, p, in, true)
}

func project(config Config, point cfg.Point, p pathdom.Path, in state.State, overlayRoot bool) (product.Value, bool) {
	reg := config.Registry
	if reg == nil {
		panic("readexpr: Config.Registry is required")
	}
	if p.IsEmpty() {
		return product.Value{}, false
	}
	if len(p.Segments) == 0 {
		value, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, p, in)
		if !ok {
			return product.Value{}, false
		}
		if !overlayRoot {
			return value, true
		}
		return overlayRootStaticMemberWitness(config, point, p, in, value), true
	}

	exactPresent := product.Value{}
	hasExactPresent := false
	if exact, ok := sourcevalue.ExactPathValue(reg, config.Visibility, point, p, in); ok {
		switch gotPresence := product.PresenceOf(exact); {
		case presence.Equal(gotPresence, presence.Present()):
			exactPresent = sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, exact, presence.Present()))
			hasExactPresent = true
			if sourcevalue.HasExactIdentity(reg, exactPresent) {
				if projected, ok, _ := projectFromStructuralEvidence(config, point, p, in); ok {
					if merged := product.Meet(reg, projected, exactPresent); !product.Equal(reg, merged, product.Bottom(reg)) {
						return merged, true
					}
				}
				if parentValue, hasParent := project(config, point, p.Parent(), in, false); hasParent {
					exactPresent = sourcevalue.InheritTopOriginEvidence(reg, exactPresent, parentValue)
				}
				return exactPresent, true
			}
		case presence.Equal(gotPresence, presence.Absent()):
			return product.Absent(reg), true
		}
	}

	if !hasExactPresent {
		if dynamicProjected, ok := projectFromDynamicIndexFacts(config, point, p, in); ok {
			return dynamicProjected, true
		}
		if heapProjected, ok := projectFromHeapIdentity(config, point, p, in); ok {
			return heapProjected, true
		}
	}

	if hasExactPresent {
		if parentValue, hasParent := project(config, point, p.Parent(), in, false); hasParent {
			exactPresent = sourcevalue.InheritTopOriginEvidence(reg, exactPresent, parentValue)
		}
	}

	if projected, ok, blocked := projectFromStructuralEvidence(config, point, p, in); ok {
		if hasExactPresent {
			if merged := product.Meet(reg, projected, exactPresent); !product.Equal(reg, merged, product.Bottom(reg)) {
				return merged, true
			}
			return exactPresent, true
		}
		return dropInBoundsIndexNil(config, point, p, in, projected), true
	} else if blocked && !hasExactPresent {
		return product.Value{}, false
	}

	if hasExactPresent {
		return exactPresent, true
	}

	value, ok := unknownIndexReadValue(config, p.Segments[len(p.Segments)-1])
	if !ok {
		return product.Value{}, false
	}
	if parentValue, hasParent := project(config, point, p.Parent(), in, false); hasParent {
		value = sourcevalue.InheritTopOriginEvidence(reg, value, parentValue)
	}
	return dropInBoundsIndexNil(config, point, p, in, value), true
}

func projectFromDynamicIndexFacts(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	reg := config.Registry
	if reg == nil || config.Visibility == nil || len(p.Segments) == 0 {
		return product.Value{}, false
	}
	parent := p.Parent()
	tableStateKey, ok := config.Visibility.StateKeyAt(point, parent)
	if !ok {
		return product.Value{}, false
	}
	tableKey, ok := config.Visibility.KeySpace().FromStateKey(tableStateKey.PathKey())
	if !ok {
		return product.Value{}, false
	}
	snapshot := in.DynamicIndexFactsSnapshot()
	if snapshot.Top || len(snapshot.Facts) == 0 {
		return product.Value{}, false
	}
	last := p.Segments[len(p.Segments)-1]
	domain := product.Domain(reg)
	joined := product.Bottom(reg)
	found := false
	for key, fact := range snapshot.Facts {
		if key.Table != tableKey || fact.Admission == dynamicindex.AdmissionRejected {
			continue
		}
		if !dynamicIndexFactDefinitelyMatchesSegment(reg, fact, last) {
			continue
		}
		if domain.Equal(fact.Value, domain.Bottom()) {
			continue
		}
		if !found {
			joined = fact.Value
			found = true
			continue
		}
		joined = domain.Join(joined, fact.Value)
	}
	if !found {
		return product.Value{}, false
	}
	return joined, true
}

func dynamicIndexFactDefinitelyMatchesSegment(reg *axis.Registry, fact dynamicindex.Fact, seg segment.Segment) bool {
	keyType, ok := typevalue.TypeOf(reg, fact.KeyValue)
	if !ok {
		return false
	}
	return dynamicIndexKeyDefinitelyMatchesSegment(keyType, seg, 0)
}

func dynamicIndexKeyDefinitelyMatchesSegment(t typ.Type, seg segment.Segment, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case nil:
		return false
	case *typ.Literal:
		return literalDynamicIndexKeyMatchesSegment(tt, seg)
	case *typ.Optional:
		return false
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !dynamicIndexKeyDefinitelyMatchesSegment(member, seg, depth+1) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		for _, member := range tt.Members {
			if dynamicIndexKeyDefinitelyMatchesSegment(member, seg, depth+1) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func literalDynamicIndexKeyMatchesSegment(lit *typ.Literal, seg segment.Segment) bool {
	if lit == nil {
		return false
	}
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		if lit.Base != kind.String {
			return false
		}
		name, ok := lit.Value.(string)
		return ok && name == seg.Name
	case segment.SegmentIndexInt:
		switch lit.Base {
		case kind.Integer:
			index, ok := lit.Value.(int64)
			return ok && index == int64(seg.Index)
		case kind.Number:
			number, ok := lit.Value.(float64)
			return ok && number == float64(seg.Index)
		default:
			return false
		}
	default:
		return false
	}
}

func overlayRootStaticMemberWitness(config Config, point cfg.Point, root pathdom.Path, in state.State, value product.Value) product.Value {
	reg := config.Registry
	if config.Visibility == nil || !sourcevalue.RuntimeMayBeTable(reg, value, true) {
		return value
	}
	rootKey, ok := config.Visibility.StateKeyAt(point, root)
	if !ok {
		return value
	}
	rootLocal, ok := pathaddr.LocalPathFromKey(rootKey.PathKey())
	if !ok || len(rootLocal.Segments) != 0 {
		return value
	}
	snapshot := in.PathStaticMembersSnapshot(config.Visibility.KeySpace())
	if snapshot.Bottom || len(snapshot.Members) == 0 {
		return value
	}
	if hasSelfIndexStaticMember(reg, rootLocal, snapshot.Members, value) {
		return value
	}
	builder := newStaticMemberWitnessBuilder()
	for memberKey, memberValue := range snapshot.Members {
		if product.Equal(reg, memberValue, product.Bottom(reg)) {
			continue
		}
		memberPath, ok := pathaddr.LocalPathFromKey(memberKey)
		if !ok || memberPath.Symbol != rootLocal.Symbol || memberPath.Version != rootLocal.Version || len(memberPath.Segments) == 0 {
			continue
		}
		memberType, ok := typevalue.TypeOf(reg, memberValue)
		if !ok || memberType == nil {
			continue
		}
		builder.add(memberPath.Segments, memberType)
	}
	staticType, ok := builder.build()
	if !ok {
		return value
	}
	if existing, ok := typevalue.TypeOf(reg, value); ok && existing != nil && !typ.IsAny(existing) && !typ.IsUnknown(existing) && !typ.IsNever(existing) {
		if _, isMap := unwrap.Alias(existing).(*typ.Map); isMap {
			// A map's declared type is invariant under a conforming key write (a
			// non-conforming write is a separate assignment error), so the root
			// witness stays the declared map rather than intersecting with the
			// per-key static-member record. Individual key reads remain precise
			// through the static-member facts; preserving the map witness keeps a
			// covariant mutable-map alias from being admitted unsoundly.
			return value
		}
		if merged, ok := mergeStaticMemberWitness(existing, staticType); ok {
			staticType = merged
		} else {
			staticType = typeexpr.Intersection(existing, staticType)
		}
	}
	return typevalue.WithWitness(reg, value, staticType)
}

func hasSelfIndexStaticMember(reg *axis.Registry, root pathdom.Path, members map[pathdom.PathKey]product.Value, rootValue product.Value) bool {
	for memberKey, memberValue := range members {
		memberPath, ok := pathaddr.LocalPathFromKey(memberKey)
		if !ok || memberPath.Symbol != root.Symbol || memberPath.Version != root.Version || len(memberPath.Segments) == 0 {
			continue
		}
		last := memberPath.Segments[len(memberPath.Segments)-1]
		if (last.Kind != segment.SegmentField && last.Kind != segment.SegmentIndexString) || last.Name != "__index" {
			continue
		}
		if sameExactIdentity(reg, rootValue, memberValue) {
			return true
		}
	}
	return false
}

func sameExactIdentity(reg *axis.Registry, left product.Value, right product.Value) bool {
	leftID, leftOK := product.Get(reg, left, identity.Key).ID()
	rightID, rightOK := product.Get(reg, right, identity.Key).ID()
	return leftOK && rightOK && leftID == rightID
}

func mergeStaticMemberWitness(existing typ.Type, static typ.Type) (typ.Type, bool) {
	existingRecord, ok := unwrap.Alias(existing).(*typ.Record)
	if !ok || existingRecord == nil {
		return nil, false
	}
	staticRecord, ok := unwrap.Alias(static).(*typ.Record)
	if !ok || staticRecord == nil {
		return nil, false
	}
	return mergeStaticMemberWitnessRecordFields(existingRecord, staticRecord, 0), true
}

func mergeStaticMemberWitnessType(existing, replacement typ.Type, depth int) typ.Type {
	if existing == nil || replacement == nil || depth > typ.DefaultRecursionDepth {
		return replacement
	}
	existingRecord, existingOK := unwrap.Alias(existing).(*typ.Record)
	replacementRecord, replacementOK := unwrap.Alias(replacement).(*typ.Record)
	if !existingOK || existingRecord == nil || !replacementOK || replacementRecord == nil {
		return replacement
	}
	merged, ok := mergeStaticMemberWitnessRecord(existingRecord, replacementRecord, depth+1)
	if !ok {
		return replacement
	}
	return merged
}

func mergeStaticMemberWitnessRecord(existingRecord, staticRecord *typ.Record, depth int) (typ.Type, bool) {
	if existingRecord == nil || staticRecord == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	return mergeStaticMemberWitnessRecordFields(existingRecord, staticRecord, depth+1), true
}

func mergeStaticMemberWitnessRecordFields(existingRecord, staticRecord *typ.Record, fieldDepth int) typ.Type {
	fields := make([]typ.Field, 0, len(existingRecord.Fields)+len(staticRecord.Fields))
	staticFields := make(map[string]typ.Field, len(staticRecord.Fields))
	for _, field := range staticRecord.Fields {
		staticFields[field.Name] = field
	}
	seenFields := make(map[string]struct{}, len(existingRecord.Fields)+len(staticRecord.Fields))
	for _, field := range existingRecord.Fields {
		if replacement, ok := staticFields[field.Name]; ok {
			replacement.Type = mergeStaticMemberWitnessType(field.Type, replacement.Type, fieldDepth)
			fields = append(fields, replacement)
		} else {
			fields = append(fields, field)
		}
		seenFields[field.Name] = struct{}{}
	}
	for _, field := range staticRecord.Fields {
		if _, seen := seenFields[field.Name]; !seen {
			fields = append(fields, field)
		}
	}
	members := mergeStaticMembers(existingRecord.StaticMembers, staticRecord.StaticMembers)
	metatable := existingRecord.Metatable
	if staticRecord.Metatable != nil {
		metatable = staticRecord.Metatable
	}
	mapKey := existingRecord.MapKey
	mapValue := existingRecord.MapValue
	if staticRecord.MapKey != nil || staticRecord.MapValue != nil {
		mapKey = staticRecord.MapKey
		mapValue = staticRecord.MapValue
	}
	return typetable.RebuildRecord(typ.RecordParts{
		Fields:        fields,
		StaticMembers: members,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          existingRecord.Open || staticRecord.Open,
	})
}

func mergeStaticMembers(existing []typ.StaticMember, static []typ.StaticMember) []typ.StaticMember {
	out := make([]typ.StaticMember, 0, len(existing)+len(static))
	replacements := make(map[staticMemberKey]typ.StaticMember, len(static))
	for _, member := range static {
		replacements[staticMemberKeyOf(member)] = member
	}
	seen := make(map[staticMemberKey]struct{}, len(existing)+len(static))
	for _, member := range existing {
		key := staticMemberKeyOf(member)
		if replacement, ok := replacements[key]; ok {
			out = append(out, replacement)
		} else {
			out = append(out, member)
		}
		seen[key] = struct{}{}
	}
	for _, member := range static {
		key := staticMemberKeyOf(member)
		if _, ok := seen[key]; !ok {
			out = append(out, member)
		}
	}
	return out
}

type staticMemberKey struct {
	kind  typ.StaticMemberKind
	name  string
	index int64
}

func staticMemberKeyOf(member typ.StaticMember) staticMemberKey {
	return staticMemberKey{kind: member.Kind, name: member.Name, index: member.Index}
}

type staticMemberWitnessBuilder struct {
	root *staticMemberWitnessNode
}

type staticMemberWitnessNode struct {
	value         typ.Type
	fields        map[string]*staticMemberWitnessNode
	stringIndexes map[string]*staticMemberWitnessNode
	intIndexes    map[int64]*staticMemberWitnessNode
}

func newStaticMemberWitnessBuilder() *staticMemberWitnessBuilder {
	return &staticMemberWitnessBuilder{root: &staticMemberWitnessNode{}}
}

func (b *staticMemberWitnessBuilder) add(segs []segment.Segment, t typ.Type) {
	if b == nil || b.root == nil || len(segs) == 0 || t == nil {
		return
	}
	b.root.insert(segs, t)
}

func (b *staticMemberWitnessBuilder) build() (typ.Type, bool) {
	if b == nil || b.root == nil {
		return nil, false
	}
	return b.root.build()
}

func (n *staticMemberWitnessNode) insert(segs []segment.Segment, t typ.Type) bool {
	if n == nil || len(segs) == 0 || t == nil {
		return false
	}
	child, ok := n.child(segs[0])
	if !ok {
		return false
	}
	if len(segs) == 1 {
		child.value = t
		return true
	}
	return child.insert(segs[1:], t)
}

func (n *staticMemberWitnessNode) child(seg segment.Segment) (*staticMemberWitnessNode, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		if seg.Name == "" {
			return nil, false
		}
		if n.fields == nil {
			n.fields = make(map[string]*staticMemberWitnessNode)
		}
		if n.fields[seg.Name] == nil {
			n.fields[seg.Name] = &staticMemberWitnessNode{}
		}
		return n.fields[seg.Name], true
	case segment.SegmentIndexString:
		if seg.Name == "" {
			return nil, false
		}
		if n.stringIndexes == nil {
			n.stringIndexes = make(map[string]*staticMemberWitnessNode)
		}
		if n.stringIndexes[seg.Name] == nil {
			n.stringIndexes[seg.Name] = &staticMemberWitnessNode{}
		}
		return n.stringIndexes[seg.Name], true
	case segment.SegmentIndexInt:
		if n.intIndexes == nil {
			n.intIndexes = make(map[int64]*staticMemberWitnessNode)
		}
		index := int64(seg.Index)
		if n.intIndexes[index] == nil {
			n.intIndexes[index] = &staticMemberWitnessNode{}
		}
		return n.intIndexes[index], true
	default:
		return nil, false
	}
}

func (n *staticMemberWitnessNode) build() (typ.Type, bool) {
	if n == nil {
		return nil, false
	}
	if len(n.fields) == 0 && len(n.stringIndexes) == 0 && len(n.intIndexes) == 0 {
		return n.value, n.value != nil
	}
	builder := typetable.NewRecord()
	for _, name := range sortedStaticMemberStringKeys(n.fields) {
		t, ok := n.fields[name].build()
		if !ok {
			return nil, false
		}
		builder.Field(name, t)
	}
	for _, name := range sortedStaticMemberStringKeys(n.stringIndexes) {
		t, ok := n.stringIndexes[name].build()
		if !ok {
			return nil, false
		}
		builder.StaticStringIndex(name, t)
	}
	for _, index := range sortedStaticMemberIntKeys(n.intIndexes) {
		t, ok := n.intIndexes[index].build()
		if !ok {
			return nil, false
		}
		builder.StaticIntIndex(index, t)
	}
	record := builder.Build()
	if n.value != nil {
		return typeexpr.Intersection(n.value, record), true
	}
	return record, true
}

func sortedStaticMemberStringKeys(in map[string]*staticMemberWitnessNode) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func sortedStaticMemberIntKeys(in map[int64]*staticMemberWitnessNode) []int64 {
	out := make([]int64, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// dropInBoundsIndexNil removes the soundly-optional nil from an array element
// read when a proven length floor establishes the literal integer index is in
// range: index k >= 1 with len(array) >= k. The decision consults the
// point-local length-floor lane keyed by the array path's visible state key.
// Out-of-floor indices keep their optional nil.
func dropInBoundsIndexNil(config Config, point cfg.Point, p pathdom.Path, in state.State, value product.Value) product.Value {
	reg := config.Registry
	if config.Visibility == nil || len(p.Segments) == 0 {
		return value
	}
	last := p.Segments[len(p.Segments)-1]
	if last.Kind != segment.SegmentIndexInt || last.Index < 1 {
		return value
	}
	arrayKey, keyOK := config.Visibility.StateKeyAt(point, p.Parent())
	if !keyOK {
		return value
	}
	floor, ok := in.ReadLenFloor(config.Visibility.KeySpace(), arrayKey)
	if !ok || floor < int64(last.Index) {
		return value
	}
	if !parentHasInBoundsIndexWitness(config, point, p.Parent(), int64(last.Index), in) {
		return value
	}
	return sourcevalue.WithoutNilRuntimeKind(reg, product.WithPresence(reg, value, presence.Present()))
}

func parentHasInBoundsIndexWitness(config Config, point cfg.Point, parent pathdom.Path, index int64, in state.State) bool {
	parentValue, ok := project(config, point, parent, in, true)
	if !ok {
		return false
	}
	parentType, ok := typevalue.TypeOf(config.Registry, parentValue)
	return ok && definitelyInBoundsIndexContainerType(parentType, index, 0)
}

func definitelyInBoundsIndexContainerType(t typ.Type, index int64, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		return true
	case *typ.Tuple:
		return index >= 1 && index <= int64(len(tt.Elements))
	case *typ.Record:
		member := tt.GetStaticIntIndex(index)
		return member != nil && !member.Optional
	case *typ.Optional:
		return definitelyInBoundsIndexContainerType(tt.Inner, index, depth+1)
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !definitelyInBoundsIndexContainerType(member, index, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func unknownIndexReadValue(config Config, seg segment.Segment) (product.Value, bool) {
	reg := config.Registry
	keyType, ok := luatypeprojection.SegmentKeyType(seg)
	if !ok {
		return product.Value{}, false
	}
	projected, ok := access.RuntimeIndex(typetable.NewMap(typ.Any, typ.Unknown), keyType)
	if !ok {
		return product.Value{}, false
	}
	if typ.IsUnknown(projected) {
		return product.Top(), true
	}
	return config.TypeValues.FromType(reg, projected), true
}

func projectFromHeapIdentity(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool) {
	reg := config.Registry
	root := p.RootOnly()
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := sourcevalue.HeapMemberFromValue(reg, config.Visibility.KeySpace(), in, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}

	parent := p.Parent()
	parentValue, _ := project(config, point, parent, in, false)
	if projected, ok := sourcevalue.HeapMemberFromValue(reg, config.Visibility.KeySpace(), in, parentValue, p.Segments[len(p.Segments)-1:]); ok {
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

func projectFromStructuralEvidence(config Config, point cfg.Point, p pathdom.Path, in state.State) (product.Value, bool, bool) {
	reg := config.Registry
	root := p.RootOnly()
	rootProjected := product.Value{}
	hasRootProjected := false
	if rootValue, ok := sourcevalue.ReadPathValue(reg, config.Visibility, point, root, in); ok {
		if projected, ok := projectFromValueEvidence(config, rootValue, p.Segments); ok {
			rootProjected = projected
			hasRootProjected = true
		}
	}

	parent := p.Parent()
	parentValue, hasParent := project(config, point, parent, in, false)
	if !sourcevalue.RuntimeMayBeTable(reg, parentValue, hasParent) {
		return product.Value{}, false, true
	}
	if projected, ok := projectFromValueEvidence(config, parentValue, p.Segments[len(p.Segments)-1:]); ok {
		// The parent-relative projection observes per-segment narrowing recorded
		// on the intermediate path (e.g. a truthy guard that removed nil from an
		// optional field), so it is at least as precise as a single root-relative
		// projection across the full suffix. Meeting them keeps a narrowed
		// non-optional result instead of re-widening it with the root's optional.
		if hasRootProjected {
			if merged := product.Meet(reg, rootProjected, projected); !product.Equal(reg, merged, product.Bottom(reg)) {
				return merged, true, false
			}
		}
		return projected, true, false
	}
	if parentProjectionRejectsFinalSegment(config, parentValue, p.Segments[len(p.Segments)-1:]) {
		return product.Value{}, false, true
	}

	if hasRootProjected {
		return rootProjected, true, false
	}

	return product.Value{}, false, false
}

func projectFromValueEvidence(config Config, value product.Value, suffix []segment.Segment) (product.Value, bool) {
	reg := config.Registry
	if len(suffix) == 0 {
		return product.Value{}, false
	}
	parentType, ok := typevalue.StructuralTypeOf(reg, config.TypeValues, value, typevalue.StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	})
	if !ok {
		return product.Value{}, false
	}
	projected, ok := luatypeprojection.ApplySegments(parentType, suffix)
	if !ok {
		return product.Value{}, false
	}
	return config.TypeValues.FromTypeWithWitness(reg, projected), true
}

func parentProjectionRejectsFinalSegment(config Config, value product.Value, suffix []segment.Segment) bool {
	if len(suffix) != 1 {
		return false
	}
	parentType, ok := typevalue.StructuralTypeOf(config.Registry, config.TypeValues, value, typevalue.StructuralTypeOptions{
		ApplyPresence:     true,
		OptionalWhenMaybe: true,
	})
	if !ok || parentType == nil || typ.IsAny(parentType) || typ.IsUnknown(parentType) || typ.IsNever(parentType) {
		return false
	}
	_, ok = luatypeprojection.ApplySegments(parentType, suffix)
	return !ok
}
