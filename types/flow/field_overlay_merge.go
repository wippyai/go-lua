package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
)

// fieldOverlayMerger projects point-local child-path assignments back into the
// root type. This is solver-owned transfer state: it does not decide lattice
// laws, it only rebuilds the root product from already-computed field facts.
type fieldOverlayMerger struct {
	s        *Solution
	point    cfg.Point
	baseType typ.Type
	baseKey  constraint.PathKey
	baseRoot constraint.PathKey
	fields   []mergedField
}

type assignedOverlayField struct {
	t        typ.Type
	optional bool
}

func newFieldOverlayMerger(s *Solution, p cfg.Point, baseType typ.Type, baseKey constraint.PathKey) *fieldOverlayMerger {
	return &fieldOverlayMerger{
		s:        s,
		point:    p,
		baseType: baseType,
		baseKey:  baseKey,
	}
}

func (m *fieldOverlayMerger) merge() typ.Type {
	if !m.prepareRootAndFields() {
		return m.baseType
	}
	if m.baseType == nil {
		m.baseType = typ.NewRecord().SetOpen(true).Build()
	}
	cacheKey := fieldMergeCacheKey{point: m.point, root: m.baseRoot, base: m.baseType}
	if cached, ok := m.cached(cacheKey); ok {
		return cached
	}
	m.assume(cacheKey, m.baseType)
	merged := m.mergeType(m.baseType)
	m.assume(cacheKey, merged)
	return merged
}

func (m *fieldOverlayMerger) prepareRootAndFields() bool {
	baseSym, baseVersion, _, ok := pathkey.ParseKeyUnchecked(m.baseKey)
	if !ok {
		return false
	}
	m.baseRoot = constraint.PathKey(pathkey.SymbolVersionRoot(baseSym, baseVersion))
	m.fields = m.s.fieldAssignmentsForRootAt(m.point, m.baseRoot)
	return len(m.fields) > 0
}

func (m *fieldOverlayMerger) cached(key fieldMergeCacheKey) (typ.Type, bool) {
	if m.s.fieldMergeCache == nil {
		m.s.fieldMergeCache = make(map[fieldMergeCacheKey]typ.Type)
		return nil, false
	}
	cached, ok := m.s.fieldMergeCache[key]
	return cached, ok
}

func (m *fieldOverlayMerger) assume(key fieldMergeCacheKey, t typ.Type) {
	if m.s.fieldMergeCache == nil {
		m.s.fieldMergeCache = make(map[fieldMergeCacheKey]typ.Type)
	}
	m.s.fieldMergeCache[key] = t
}

func (m *fieldOverlayMerger) mergeType(baseType typ.Type) typ.Type {
	return typ.Visit(baseType, typ.Visitor[typ.Type]{
		Alias:        m.mergeAlias(baseType),
		Recursive:    m.mergeRecursive(baseType),
		Optional:     m.mergeOptional(baseType),
		Union:        m.mergeUnion(baseType),
		Intersection: m.mergeIntersection(baseType),
		Map:          m.mergeMap,
		Record:       m.mergeRecord,
		Default:      m.mergeDefault(baseType),
	})
}

func (m *fieldOverlayMerger) mergeNested(base typ.Type) typ.Type {
	return m.s.mergeFieldAssignmentsAt(m.point, base, m.baseKey)
}

func (m *fieldOverlayMerger) mergeAlias(baseType typ.Type) func(*typ.Alias) typ.Type {
	return func(a *typ.Alias) typ.Type {
		merged := m.mergeNested(a.Target)
		if merged == nil || typ.TypeEquals(merged, a.Target) {
			return baseType
		}
		return typ.NewAlias(a.Name, merged)
	}
}

func (m *fieldOverlayMerger) mergeRecursive(baseType typ.Type) func(*typ.Recursive) typ.Type {
	return func(r *typ.Recursive) typ.Type {
		mergedBody := m.mergeNested(r.Body)
		if mergedBody == nil || typ.TypeEquals(mergedBody, r.Body) {
			return baseType
		}
		rebuilt := typ.NewRecursivePlaceholder(r.Name)
		rebuiltBody := typ.Rewrite(mergedBody, func(n typ.Type) (typ.Type, bool) {
			if typ.IsRecursiveRef(n, r) {
				return rebuilt, true
			}
			return nil, false
		})
		rebuilt.SetBody(rebuiltBody)
		return rebuilt
	}
}

func (m *fieldOverlayMerger) mergeOptional(baseType typ.Type) func(*typ.Optional) typ.Type {
	return func(o *typ.Optional) typ.Type {
		if o.Inner == nil {
			return baseType
		}
		merged := m.mergeNested(o.Inner)
		if merged == nil || typ.TypeEquals(merged, o.Inner) {
			return baseType
		}
		return typ.NewOptional(merged)
	}
}

func (m *fieldOverlayMerger) mergeUnion(baseType typ.Type) func(*typ.Union) typ.Type {
	return func(u *typ.Union) typ.Type {
		if len(u.Members) == 0 {
			return baseType
		}
		if overlay, ok := mergeRecursiveUnionFieldOverlay(u, m.fields); ok {
			return overlay
		}
		normalized := join.Types(u.Members...)
		if normalized != nil && !typ.TypeEquals(normalized, baseType) {
			return m.mergeNested(normalized)
		}
		members, changed := m.mergeMembers(u.Members)
		if !changed {
			return baseType
		}
		return join.Types(members...)
	}
}

func (m *fieldOverlayMerger) mergeIntersection(baseType typ.Type) func(*typ.Intersection) typ.Type {
	return func(i *typ.Intersection) typ.Type {
		if len(i.Members) == 0 {
			return baseType
		}
		members, changed := m.mergeMembers(i.Members)
		if !changed {
			return baseType
		}
		return typ.NewIntersection(members...)
	}
}

func (m *fieldOverlayMerger) mergeMembers(in []typ.Type) ([]typ.Type, bool) {
	members := make([]typ.Type, len(in))
	changed := false
	for idx, member := range in {
		merged := m.mergeNested(member)
		if merged == nil {
			merged = member
		}
		if !typ.TypeEquals(merged, member) {
			changed = true
		}
		members[idx] = merged
	}
	return members, changed
}

func (m *fieldOverlayMerger) mergeMap(mp *typ.Map) typ.Type {
	builder := typ.NewRecord().SetOpen(true)
	builder.MapComponent(mp.Key, mp.Value)
	addOverlayFields(builder, m.fields)
	return builder.Build()
}

func (m *fieldOverlayMerger) mergeRecord(r *typ.Record) typ.Type {
	builder := typ.NewRecord()
	if r.Open {
		builder.SetOpen(true)
	}
	assignedByKey := coalesceOverlayFields(m.fields)
	for _, f := range r.Fields {
		fieldType := f.Type
		optional := f.Optional
		key := constraint.Segment{Kind: constraint.SegmentField, Name: f.Name}
		if assigned, ok := assignedByKey[key]; ok {
			fieldType = assigned.t
			optional = assigned.optional
			delete(assignedByKey, key)
		}
		addRecordField(builder, f.Name, fieldType, optional, f.Readonly)
	}
	for _, member := range r.StaticMembers {
		memberType := member.Type
		optional := member.Optional
		key, ok := segmentFromStaticMember(member)
		if ok {
			if assigned, exists := assignedByKey[key]; exists {
				memberType = assigned.t
				optional = assigned.optional
				delete(assignedByKey, key)
			}
		}
		addRecordStaticMember(builder, member, memberType, optional)
	}
	addAssignedOverlayFields(builder, assignedByKey)
	if r.Metatable != nil {
		builder.Metatable(r.Metatable)
	}
	if r.HasMapComponent() {
		builder.MapComponent(r.MapKey, r.MapValue)
	}
	return builder.Build()
}

func (m *fieldOverlayMerger) mergeDefault(baseType typ.Type) func(typ.Type) typ.Type {
	return func(t typ.Type) typ.Type {
		if t == nil || t.Kind() == kind.Nil || t.Kind().IsPlaceholder() {
			return baseType
		}
		builder := typ.NewRecord().SetOpen(true)
		addOverlayFields(builder, m.fields)
		return builder.Build()
	}
}

func coalesceOverlayFields(fields []mergedField) map[constraint.Segment]assignedOverlayField {
	assignedByKey := make(map[constraint.Segment]assignedOverlayField, len(fields))
	for _, f := range fields {
		if !overlaySegmentCanProjectToRecord(f.Key) {
			continue
		}
		if prev, ok := assignedByKey[f.Key]; ok {
			assignedByKey[f.Key] = assignedOverlayField{
				t:        typ.JoinReturnSlot(prev.t, f.Type),
				optional: prev.optional || f.Optional,
			}
			continue
		}
		assignedByKey[f.Key] = assignedOverlayField{t: f.Type, optional: f.Optional}
	}
	return assignedByKey
}

func addOverlayFields(builder *typ.RecordBuilder, fields []mergedField) {
	for _, f := range fields {
		addOverlayField(builder, f.Key, f.Type, f.Optional)
	}
}

func addAssignedOverlayFields(builder *typ.RecordBuilder, fields map[constraint.Segment]assignedOverlayField) {
	keys := make([]constraint.Segment, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, right := keys[i], keys[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.Index < right.Index
	})
	for _, key := range keys {
		field := fields[key]
		addOverlayField(builder, key, field.t, field.optional)
	}
}

func addOverlayField(builder *typ.RecordBuilder, key constraint.Segment, fieldType typ.Type, optional bool) {
	switch key.Kind {
	case constraint.SegmentField:
		if key.Name == "" {
			return
		}
		if optional {
			builder.OptField(key.Name, fieldType)
		} else {
			builder.Field(key.Name, fieldType)
		}
	case constraint.SegmentIndexString:
		builder.AddStaticMember(typ.StaticMember{
			Kind:     typ.StaticMemberStringIndex,
			Name:     key.Name,
			Type:     fieldType,
			Optional: optional,
		})
	case constraint.SegmentIndexInt:
		builder.AddStaticMember(typ.StaticMember{
			Kind:     typ.StaticMemberIntIndex,
			Index:    int64(key.Index),
			Type:     fieldType,
			Optional: optional,
		})
	}
}

func segmentFromStaticMember(member typ.StaticMember) (constraint.Segment, bool) {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: member.Name}, true
	case typ.StaticMemberIntIndex:
		return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(member.Index)}, true
	default:
		return constraint.Segment{}, false
	}
}

func addRecordStaticMember(builder *typ.RecordBuilder, member typ.StaticMember, memberType typ.Type, optional bool) {
	member.Type = memberType
	member.Optional = optional
	builder.AddStaticMember(member)
}

func overlaySegmentCanProjectToRecord(key constraint.Segment) bool {
	switch key.Kind {
	case constraint.SegmentField:
		return key.Name != ""
	case constraint.SegmentIndexString, constraint.SegmentIndexInt:
		return true
	default:
		return false
	}
}

func addRecordField(builder *typ.RecordBuilder, name string, fieldType typ.Type, optional, readonly bool) {
	switch {
	case optional && readonly:
		builder.OptReadonlyField(name, fieldType)
	case optional:
		builder.OptField(name, fieldType)
	case readonly:
		builder.ReadonlyField(name, fieldType)
	default:
		builder.Field(name, fieldType)
	}
}
