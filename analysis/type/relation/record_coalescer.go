package relation

import (
	luatable "github.com/wippyai/go-lua/analysis/lua/table"
	"github.com/wippyai/go-lua/analysis/type/coalesce"
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

type recordJoinResult struct {
	t  Type
	ok bool
}

func (c *productCoalescer) recordJoin(key returnJoinKey) (Type, bool, bool) {
	if c == nil || c.records == nil {
		return nil, false, false
	}
	cached, ok := c.records[key]
	return cached.t, cached.ok, ok
}

func (c *productCoalescer) cacheRecordJoin(key returnJoinKey, t Type, ok bool) {
	if c.records == nil {
		c.records = make(map[returnJoinKey]recordJoinResult)
	}
	c.records[key] = recordJoinResult{t: t, ok: ok}
}

func (c *productCoalescer) recordPolicy(slotJoin SlotJoinFunc) coalesce.RecordPolicy {
	return coalesce.RecordPolicy{
		SlotJoin:      c.slotJoinOrDefault(slotJoin),
		KeyJoin:       JoinPreferNonSoft,
		Discriminants: c.discriminantDetector(),
	}
}

// JoinCompatibleRecords joins two record types into a single record when they
// are structurally compatible for safe optional-field widening.
//
// This preserves discriminated unions by refusing joins when required literal
// fields conflict across the two records.
func JoinCompatibleRecords(a, b Type) (Type, bool) {
	return JoinCompatibleRecordsWithSlotJoin(a, b, nil)
}

// JoinCompatibleRecordsWithSlotJoin joins two record types using slotJoin for
// nested field/static/container slots. A nil slotJoin preserves JoinReturnSlot
// behavior.
func JoinCompatibleRecordsWithSlotJoin(a, b Type, slotJoin SlotJoinFunc) (Type, bool) {
	state := newReturnJoinState()
	return state.product.joinCompatibleRecordsWithSlotJoin(a, b, state.slotJoinOrDefault(slotJoin))
}

// JoinClosedCompatibleRecordSet joins a compatible set of closed, non-map
// records in one pass. It is the bulk form of JoinCompatibleRecords for large
// record unions where repeated pair joins would rebuild the same optional-field
// product many times.
func JoinClosedCompatibleRecordSet(records []*Record) (Type, bool) {
	return JoinClosedCompatibleRecordSetWithSlotJoin(records, nil)
}

// JoinClosedCompatibleRecordSetWithSlotJoin joins a compatible set of closed,
// non-map records using slotJoin for repeated field/static member slots. A nil
// slotJoin preserves JoinReturnSlot behavior.
func JoinClosedCompatibleRecordSetWithSlotJoin(records []*Record, slotJoin SlotJoinFunc) (Type, bool) {
	state := newReturnJoinState()
	return state.product.joinClosedCompatibleRecordSetWithSlotJoin(records, state.slotJoinOrDefault(slotJoin))
}

func (c *productCoalescer) joinClosedCompatibleRecordSetWithSlotJoin(records []*Record, slotJoin SlotJoinFunc) (Type, bool) {
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	return coalesce.JoinClosedCompatibleRecordSet(records, state.recordPolicy(slotJoin))
}

// CoalesceCompatibleRecords merges compatible record alternatives before a
// union is constructed. It is the canonical batch form for return-slot and flow
// joins that would otherwise repeatedly build large transient unions.
func CoalesceCompatibleRecords(types []Type) []Type {
	return CoalesceCompatibleRecordsWithSlotJoin(types, nil)
}

// CoalesceCompatibleRecordsWithSlotJoin merges compatible record alternatives
// using slotJoin for nested field/static/container slots. A nil slotJoin
// preserves JoinReturnSlot behavior.
func CoalesceCompatibleRecordsWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	state := newReturnJoinState()
	return state.product.coalesceCompatibleRecordTypesWithSlotJoin(types, state.slotJoinOrDefault(slotJoin))
}

// CoalesceCompatibleRecordAlternatives canonicalizes compatible record members
// inside one union or optional union without changing non-record alternatives.
func CoalesceCompatibleRecordAlternatives(t Type) Type {
	return CoalesceCompatibleRecordAlternativesWithSlotJoin(t, nil)
}

// CoalesceCompatibleRecordAlternativesWithSlotJoin canonicalizes compatible
// record members inside one union or optional union using slotJoin for nested
// slots. A nil slotJoin preserves JoinReturnSlot behavior.
func CoalesceCompatibleRecordAlternativesWithSlotJoin(t Type, slotJoin SlotJoinFunc) Type {
	state := newReturnJoinState()
	return state.product.coalesceCompatibleRecordAlternativesWithSlotJoin(t, state.slotJoinOrDefault(slotJoin))
}

func (c *productCoalescer) coalesceCompatibleRecordAlternativesWithSlotJoin(t Type, slotJoin SlotJoinFunc) Type {
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	switch v := UnwrapAnnotated(t).(type) {
	case *Optional:
		inner := state.coalesceCompatibleRecordAlternativesWithSlotJoin(v.Inner, slotJoin)
		if SameNodeOrAcyclicEqual(inner, v.Inner) {
			return t
		}
		return NewOptional(inner)
	case *Union:
		if len(v.Members) < 2 {
			return t
		}
		members := make([]Type, len(v.Members))
		copy(members, v.Members)
		coalesced := state.coalesceCompatibleRecordTypesWithSlotJoin(members, slotJoin)
		if sameTypeSlice(members, coalesced) {
			return t
		}
		return NormalizeUnionForJoin(coalesced...)
	default:
		return t
	}
}

func (c *productCoalescer) coalesceCompatibleRecordTypesWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	if len(types) < 2 {
		return types
	}
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	if fast, ok, complete := state.coalesceClosedCompatibleRecordsWithSlotJoin(types, slotJoin); ok {
		if complete {
			return fast
		}
		types = fast
	}
	if fast, ok := state.coalesceCompatibleRecordGroupsWithSlotJoin(types, slotJoin); ok {
		return fast
	}
	return state.coalesceCompatibleRecordsPairwiseWithSlotJoin(types, slotJoin)
}

type compatibleRecordGroup struct {
	indices []int
	records []*Record
	hasMap  bool
}

func (c *productCoalescer) coalesceCompatibleRecordGroupsWithSlotJoin(types []Type, slotJoin SlotJoinFunc) ([]Type, bool) {
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	groups := make([]*compatibleRecordGroup, 0, len(types))
	for i, t := range types {
		rec := unaliasRecord(t)
		if rec == nil {
			continue
		}
		var group *compatibleRecordGroup
		for _, candidate := range groups {
			if candidate.hasMap == rec.HasMapComponent() && coalesce.RecordMergesIntoGroup(rec, candidate.records, state.discriminantDetector()) {
				group = candidate
				break
			}
		}
		if group == nil {
			group = &compatibleRecordGroup{hasMap: rec.HasMapComponent()}
			groups = append(groups, group)
		}
		group.indices = append(group.indices, i)
		group.records = append(group.records, rec)
	}

	changed := false
	mergedAt := make(map[int]Type)
	skip := make(map[int]bool)
	for _, group := range groups {
		if len(group.records) < 2 {
			continue
		}
		merged, ok := state.joinCompatibleRecordSetWithSlotJoin(group.records, slotJoin)
		if !ok {
			return nil, false
		}
		changed = true
		mergedAt[group.indices[0]] = merged
		for _, idx := range group.indices[1:] {
			skip[idx] = true
		}
	}
	if !changed {
		return nil, false
	}

	out := make([]Type, 0, len(types)-len(skip))
	for i, t := range types {
		if merged, ok := mergedAt[i]; ok {
			out = append(out, merged)
			continue
		}
		if skip[i] {
			continue
		}
		out = append(out, t)
	}
	return out, true
}

func (c *productCoalescer) joinCompatibleRecordSet(records []*Record) (Type, bool) {
	return c.joinCompatibleRecordSetWithSlotJoin(records, c.slotJoinOrDefault(nil))
}

func (c *productCoalescer) joinCompatibleRecordSetWithSlotJoin(records []*Record, slotJoin SlotJoinFunc) (Type, bool) {
	if len(records) == 0 {
		return nil, false
	}
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	current := Type(records[0])
	for _, rec := range records[1:] {
		merged, ok := state.joinCompatibleRecordsWithSlotJoin(current, rec, slotJoin)
		if !ok {
			return nil, false
		}
		current = merged
	}
	return current, true
}

func (c *productCoalescer) coalesceCompatibleRecordsPairwiseWithSlotJoin(types []Type, slotJoin SlotJoinFunc) []Type {
	if len(types) < 2 {
		return types
	}
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	used := make([]bool, len(types))
	out := make([]Type, 0, len(types))
	for i := 0; i < len(types); i++ {
		if used[i] {
			continue
		}
		current := types[i]
		currentRecord := unaliasRecord(current)
		if currentRecord == nil {
			out = append(out, current)
			continue
		}
		for j := i + 1; j < len(types); j++ {
			if used[j] {
				continue
			}
			candidateRecord := unaliasRecord(types[j])
			if candidateRecord == nil {
				continue
			}
			merged, ok := state.joinCompatibleRecordsWithSlotJoin(currentRecord, candidateRecord, slotJoin)
			if !ok {
				continue
			}
			current = merged
			currentRecord = unaliasRecord(merged)
			if currentRecord == nil {
				break
			}
			used[j] = true
		}
		out = append(out, current)
	}
	return out
}

func (c *productCoalescer) coalesceClosedCompatibleRecordsWithSlotJoin(types []Type, slotJoin SlotJoinFunc) ([]Type, bool, bool) {
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	return coalesce.CoalesceClosedCompatibleRecords(types, state.recordPolicy(slotJoin))
}

func (c *productCoalescer) joinCompatibleRecordsWithSlotJoin(a, b Type, slotJoin SlotJoinFunc) (Type, bool) {
	state := c
	if state == nil {
		state = newProductCoalescer()
	}
	slotJoin = state.slotJoinOrDefault(slotJoin)
	ar := unaliasRecord(a)
	br := unaliasRecord(b)
	if ar == nil || br == nil {
		return nil, false
	}
	if state.sameJoinInput(ar, br) {
		return ar, true
	}
	key := state.joinKey(ar, br)
	if cached, cachedOK, found := state.recordJoin(key); found {
		return cached, cachedOK
	}

	// Keep discriminated unions intact when required literal tags conflict.
	if state.discriminantDetector().RecordsConflict(ar, br) {
		state.cacheRecordJoin(key, nil, false)
		return nil, false
	}

	// Mixing map and non-map record slots can be semantically distinct.
	if ar.HasMapComponent() != br.HasMapComponent() {
		state.cacheRecordJoin(key, nil, false)
		return nil, false
	}
	if !coalesce.CompatibleRecordMetatables(ar, br) {
		state.cacheRecordJoin(key, nil, false)
		return nil, false
	}

	open := ar.Open || br.Open
	metatable := Type(nil)
	if ar.Metatable != nil && br.Metatable != nil && SameNodeOrAcyclicEqual(ar.Metatable, br.Metatable) {
		metatable = ar.Metatable
	}
	mapKey := Type(nil)
	mapValue := Type(nil)
	if ar.HasMapComponent() && br.HasMapComponent() {
		mapKey = JoinPreferNonSoft(ar.MapKey, br.MapKey)
		mapValue = JoinPreferNonSoft(ar.MapValue, br.MapValue)
	}

	fields := make([]Field, 0, len(ar.Fields)+len(br.Fields))
	i, j := 0, 0
	for i < len(ar.Fields) || j < len(br.Fields) {
		switch {
		case j >= len(br.Fields):
			fields = append(fields, state.mergeRecordField(ar.Fields[i].Name, ar.Fields[i], true, Field{}, false, ar, br, slotJoin))
			i++
		case i >= len(ar.Fields):
			fields = append(fields, state.mergeRecordField(br.Fields[j].Name, Field{}, false, br.Fields[j], true, ar, br, slotJoin))
			j++
		case ar.Fields[i].Name == br.Fields[j].Name:
			fields = append(fields, state.mergeRecordField(ar.Fields[i].Name, ar.Fields[i], true, br.Fields[j], true, ar, br, slotJoin))
			i++
			j++
		case ar.Fields[i].Name < br.Fields[j].Name:
			fields = append(fields, state.mergeRecordField(ar.Fields[i].Name, ar.Fields[i], true, Field{}, false, ar, br, slotJoin))
			i++
		default:
			fields = append(fields, state.mergeRecordField(br.Fields[j].Name, Field{}, false, br.Fields[j], true, ar, br, slotJoin))
			j++
		}
	}

	staticMembers := state.mergeRecordStaticMembers(ar, br, slotJoin)
	merged := luatable.RebuildRecord(RecordParts{
		Fields:        fields,
		StaticMembers: staticMembers,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          open,
		AssumeSorted:  true,
	})
	state.cacheRecordJoin(key, merged, true)
	return merged, true
}

func (c *productCoalescer) mergeRecordField(name string, fa Field, oka bool, fb Field, okb bool, ar, br *Record, slotJoin SlotJoinFunc) Field {
	slotJoin = c.slotJoinOrDefault(slotJoin)
	fieldType := Type(nil)
	optional := true
	readonly := false
	switch {
	case oka && okb:
		// Keep field-level merge on the caller's slot policy so
		// empty-collection paths and nil/unknown interactions stay consistent
		// with the surrounding join.
		fieldType = coalesce.JoinRecordFieldSlot(fa.Type, fb.Type, c.recordPolicy(slotJoin))
		optional = fa.Optional || fb.Optional
		readonly = fa.Readonly && fb.Readonly
	case oka:
		fieldType = fa.Type
		optional = true
		readonly = fa.Readonly
		if tail, ok := luatable.RecordTailFieldType(br, name); ok {
			fieldType, optional = normalizeMergedRecordField(slotJoin(fa.Type, tail))
			if luatable.RecordMapTailMayContainFieldName(br, name) {
				optional = true
			}
			readonly = false
		}
	case okb:
		fieldType = fb.Type
		optional = true
		readonly = fb.Readonly
		if tail, ok := luatable.RecordTailFieldType(ar, name); ok {
			fieldType, optional = normalizeMergedRecordField(slotJoin(tail, fb.Type))
			if luatable.RecordMapTailMayContainFieldName(ar, name) {
				optional = true
			}
			readonly = false
		}
	}
	return Field{Name: name, Type: fieldType, Optional: optional, Readonly: readonly}
}

func (c *productCoalescer) mergeRecordStaticMembers(ar, br *Record, slotJoin SlotJoinFunc) []StaticMember {
	slotJoin = c.slotJoinOrDefault(slotJoin)
	staticMembers := make([]StaticMember, 0, len(ar.StaticMembers)+len(br.StaticMembers))
	i, j := 0, 0
	for i < len(ar.StaticMembers) || j < len(br.StaticMembers) {
		switch {
		case j >= len(br.StaticMembers):
			staticMembers = append(staticMembers, c.mergeRecordStaticMember(ar.StaticMembers[i], true, StaticMember{}, false, ar, br, slotJoin))
			i++
		case i >= len(ar.StaticMembers):
			staticMembers = append(staticMembers, c.mergeRecordStaticMember(StaticMember{}, false, br.StaticMembers[j], true, ar, br, slotJoin))
			j++
		default:
			cmp := CompareStaticMembers(ar.StaticMembers[i], br.StaticMembers[j])
			switch {
			case cmp == 0:
				staticMembers = append(staticMembers, c.mergeRecordStaticMember(ar.StaticMembers[i], true, br.StaticMembers[j], true, ar, br, slotJoin))
				i++
				j++
			case cmp < 0:
				staticMembers = append(staticMembers, c.mergeRecordStaticMember(ar.StaticMembers[i], true, StaticMember{}, false, ar, br, slotJoin))
				i++
			default:
				staticMembers = append(staticMembers, c.mergeRecordStaticMember(StaticMember{}, false, br.StaticMembers[j], true, ar, br, slotJoin))
				j++
			}
		}
	}
	return staticMembers
}

func (c *productCoalescer) mergeRecordStaticMember(ma StaticMember, oka bool, mb StaticMember, okb bool, ar, br *Record, slotJoin SlotJoinFunc) StaticMember {
	slotJoin = c.slotJoinOrDefault(slotJoin)
	member := ma
	memberType := Type(nil)
	optional := true
	readonly := false
	switch {
	case oka && okb:
		memberType = coalesce.JoinRecordFieldSlot(ma.Type, mb.Type, c.recordPolicy(slotJoin))
		optional = ma.Optional || mb.Optional
		readonly = ma.Readonly && mb.Readonly
	case oka:
		memberType = ma.Type
		optional = true
		readonly = ma.Readonly
		if tail, ok := luatable.RecordTailStaticMemberType(br, ma); ok {
			memberType, optional = normalizeMergedRecordField(slotJoin(ma.Type, tail))
			if luatable.RecordMapTailMayContainStaticMember(br, ma) {
				optional = true
			}
			readonly = false
		}
	case okb:
		member = mb
		memberType = mb.Type
		optional = true
		readonly = mb.Readonly
		if tail, ok := luatable.RecordTailStaticMemberType(ar, mb); ok {
			memberType, optional = normalizeMergedRecordField(slotJoin(tail, mb.Type))
			if luatable.RecordMapTailMayContainStaticMember(ar, mb) {
				optional = true
			}
			readonly = false
		}
	}
	member.Type = memberType
	member.Optional = optional
	member.Readonly = readonly
	return member
}

// JoinUnionFieldSlot merges the per-member results of reading one field across a
// union. When preserveLiteral is set the field is the union's structural
// discriminant, so distinct literal alternatives are kept for path-sensitive
// narrowing; otherwise ordinary literal data fields widen to their scalar base so
// many-member unions do not explode into giant literal unions on read. The caller
// owns the discriminant decision because it alone holds the union context.
func JoinUnionFieldSlot(a, b Type, preserveLiteral bool) Type {
	coalescer := newProductCoalescer()
	slotJoin := coalescer.slotJoinOrDefault(nil)
	if preserveLiteral {
		if joined, ok := coalesce.JoinFieldContainerSlot(a, b, coalescer.recordPolicy(slotJoin)); ok {
			return joined
		}
		return slotJoin(a, b)
	}
	return coalesce.JoinRecordFieldSlot(a, b, coalescer.recordPolicy(slotJoin))
}

func normalizeMergedRecordField(t Type) (Type, bool) {
	if inner, optional := luatable.SplitNilableField(t); optional {
		return inner, true
	}
	return t, false
}

func unaliasRecord(t Type) *Record {
	return coalesce.UnaliasRecord(t)
}

// RecordsConflictOnLiteralDiscriminant reports whether two records are discriminated
// variants kept distinct by a shared required literal field rather than coalesced.
// It is the structural, name-free discriminant test shared by the return-slot join
// and the value-domain shape join.
func RecordsConflictOnLiteralDiscriminant(a, b *Record) bool {
	return discriminant.RecordsConflict(a, b)
}
