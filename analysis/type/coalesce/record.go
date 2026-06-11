package coalesce

import (
	"github.com/wippyai/go-lua/analysis/type/discriminant"
	"github.com/wippyai/go-lua/analysis/type/identity"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// SlotJoinFunc joins two nested product slots. Callers own the surrounding
// fixed-point/recursive orchestration and inject the slot policy here.
type SlotJoinFunc func(a, b typ.Type) typ.Type

// RecordPolicy carries the caller-owned pieces needed by record coalescing
// without depending on relation's return-join state.
type RecordPolicy struct {
	SlotJoin      SlotJoinFunc
	KeyJoin       SlotJoinFunc
	Discriminants *discriminant.Detector
}

func (p RecordPolicy) slotJoin() SlotJoinFunc {
	if p.SlotJoin != nil {
		return p.SlotJoin
	}
	return func(a, b typ.Type) typ.Type {
		return normalize.UnionForJoin(a, b)
	}
}

func (p RecordPolicy) keyJoin() SlotJoinFunc {
	if p.KeyJoin != nil {
		return p.KeyJoin
	}
	return p.slotJoin()
}

func (p RecordPolicy) detector() *discriminant.Detector {
	if p.Discriminants != nil {
		return p.Discriminants
	}
	return discriminant.NewDetector()
}

// JoinRecordFieldSlot merges one record field/static-member slot under the
// caller's nested slot policy.
func JoinRecordFieldSlot(a, b typ.Type, policy RecordPolicy) typ.Type {
	slotJoin := policy.slotJoin()
	if joined, ok := JoinFieldContainerSlot(a, b, policy); ok {
		return joined
	}
	if widened, ok := joinNonDiscriminantField(a, b); ok {
		return widened
	}
	return slotJoin(a, b)
}

// JoinFieldContainerSlot merges homogeneous container slots pointwise.
func JoinFieldContainerSlot(a, b typ.Type, policy RecordPolicy) (typ.Type, bool) {
	slotJoin := policy.slotJoin()
	keyJoin := policy.keyJoin()
	a = unwrap.Annotated(a)
	b = unwrap.Annotated(b)
	switch av := a.(type) {
	case *typ.Array:
		bv, ok := b.(*typ.Array)
		if !ok {
			return nil, false
		}
		return typ.NewArray(slotJoin(av.Element, bv.Element)), true
	case *typ.Map:
		bv, ok := b.(*typ.Map)
		if !ok {
			return nil, false
		}
		return typetable.NewMap(keyJoin(av.Key, bv.Key), slotJoin(av.Value, bv.Value)), true
	case *typ.ReadonlyMap:
		bv, ok := b.(*typ.ReadonlyMap)
		if !ok {
			return nil, false
		}
		return typetable.NewReadonlyMap(keyJoin(av.Key, bv.Key), slotJoin(av.Value, bv.Value)), true
	case *typ.Tuple:
		bv, ok := b.(*typ.Tuple)
		if !ok || len(av.Elements) != len(bv.Elements) {
			return nil, false
		}
		elements := make([]typ.Type, len(av.Elements))
		for i := range av.Elements {
			elements[i] = slotJoin(av.Elements[i], bv.Elements[i])
		}
		return typ.NewTuple(elements...), true
	default:
		return nil, false
	}
}

type closedRecordGroup struct {
	indices []int
	records []*typ.Record
}

type staticMemberJoinKey struct {
	kind  typ.StaticMemberKind
	name  string
	index int64
}

type staticMemberAcc struct {
	memberType typ.Type
	count      int
	optional   bool
	readonly   bool
	kind       typ.StaticMemberKind
	name       string
	index      int64
}

// CoalesceClosedCompatibleRecords merges eligible closed record alternatives in
// one pass. The third result reports whether every input was eligible.
func CoalesceClosedCompatibleRecords(types []typ.Type, policy RecordPolicy) ([]typ.Type, bool, bool) {
	groups := make([]*closedRecordGroup, 0, len(types))
	changed := false
	ineligible := false
	eligibleCount := 0
	for i, t := range types {
		rec := unwrap.RecordUnaliased(t)
		if rec == nil {
			ineligible = true
			continue
		}
		if rec.Open || rec.HasMapComponent() || rec.Metatable != nil || inspect.ContainsRecursive(rec) {
			ineligible = true
			continue
		}
		eligibleCount++
		var group *closedRecordGroup
		for _, candidate := range groups {
			if RecordMergesIntoGroup(rec, candidate.records, policy.Discriminants) {
				group = candidate
				break
			}
		}
		if group == nil {
			group = &closedRecordGroup{}
			groups = append(groups, group)
		} else {
			changed = true
		}
		group.indices = append(group.indices, i)
		group.records = append(group.records, rec)
	}
	if eligibleCount == 0 {
		return nil, false, false
	}
	if !changed {
		if !ineligible {
			return types, true, true
		}
		return nil, false, false
	}

	mergedAt := make(map[int]typ.Type)
	skip := make(map[int]bool)
	for _, group := range groups {
		if len(group.records) == 1 {
			continue
		}
		merged, ok := JoinClosedCompatibleRecordSet(group.records, policy)
		if !ok {
			return nil, false, false
		}
		mergedAt[group.indices[0]] = merged
		for _, idx := range group.indices[1:] {
			skip[idx] = true
		}
	}
	out := make([]typ.Type, 0, len(types))
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
	return out, true, !ineligible
}

// JoinClosedCompatibleRecordSet joins a compatible set of closed, non-map
// records into one record.
func JoinClosedCompatibleRecordSet(records []*typ.Record, policy RecordPolicy) (typ.Type, bool) {
	if len(records) == 0 {
		return nil, false
	}
	if len(records) == 1 {
		return records[0], true
	}
	for _, rec := range records {
		if rec == nil || rec.Open || rec.HasMapComponent() || rec.Metatable != nil || inspect.ContainsRecursive(rec) {
			return nil, false
		}
	}
	if policy.detector().ClosedRecordSetConflicts(records) {
		return nil, false
	}

	type fieldAcc struct {
		fieldType typ.Type
		count     int
		optional  bool
		readonly  bool
	}
	fields := make(map[string]*fieldAcc)
	staticMembers := make(map[staticMemberJoinKey]*staticMemberAcc)
	for _, rec := range records {
		for _, field := range rec.Fields {
			acc := fields[field.Name]
			if acc == nil {
				fields[field.Name] = &fieldAcc{
					fieldType: field.Type,
					count:     1,
					optional:  field.Optional,
					readonly:  field.Readonly,
				}
				continue
			}
			acc.fieldType = JoinRecordFieldSlot(acc.fieldType, field.Type, policy)
			acc.count++
			acc.optional = acc.optional || field.Optional
			acc.readonly = acc.readonly && field.Readonly
		}
		for _, member := range rec.StaticMembers {
			key := staticMemberKey(member)
			acc := staticMembers[key]
			if acc == nil {
				staticMembers[key] = &staticMemberAcc{
					memberType: member.Type,
					count:      1,
					optional:   member.Optional,
					readonly:   member.Readonly,
					kind:       member.Kind,
					name:       member.Name,
					index:      member.Index,
				}
				continue
			}
			acc.memberType = JoinRecordFieldSlot(acc.memberType, member.Type, policy)
			acc.count++
			acc.optional = acc.optional || member.Optional
			acc.readonly = acc.readonly && member.Readonly
		}
	}

	mergedFields := make([]typ.Field, 0, len(fields))
	for name, acc := range fields {
		mergedFields = append(mergedFields, typ.Field{
			Name:     name,
			Type:     acc.fieldType,
			Optional: acc.optional || acc.count < len(records),
			Readonly: acc.readonly,
		})
	}
	mergedStaticMembers := make([]typ.StaticMember, 0, len(staticMembers))
	for _, acc := range staticMembers {
		mergedStaticMembers = append(mergedStaticMembers, typ.StaticMember{
			Kind:     acc.kind,
			Name:     acc.name,
			Index:    acc.index,
			Type:     acc.memberType,
			Optional: acc.optional || acc.count < len(records),
			Readonly: acc.readonly,
		})
	}
	return typetable.RebuildRecord(typ.RecordParts{
		Fields:        mergedFields,
		StaticMembers: mergedStaticMembers,
	}), true
}

// RecordMergesIntoGroup reports whether rec belongs with every record already
// grouped, rather than forming a discriminated variant.
func RecordMergesIntoGroup(rec *typ.Record, group []*typ.Record, detector *discriminant.Detector) bool {
	if detector == nil {
		detector = discriminant.NewDetector()
	}
	for _, member := range group {
		if !CompatibleRecordMetatables(rec, member) {
			return false
		}
		if detector.RecordsConflict(rec, member) {
			return false
		}
	}
	return true
}

// CompatibleRecordMetatables reports whether two records may merge without
// dropping distinct metatable behavior.
func CompatibleRecordMetatables(a, b *typ.Record) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Metatable == nil || b.Metatable == nil {
		return a.Metatable == nil && b.Metatable == nil
	}
	return identity.SameNodeOrAcyclicEqual(a.Metatable, b.Metatable)
}

func staticMemberKey(member typ.StaticMember) staticMemberJoinKey {
	return staticMemberJoinKey{kind: member.Kind, name: member.Name, index: member.Index}
}
