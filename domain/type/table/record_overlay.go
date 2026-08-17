package table

import (
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

// OverlayRecordMembers overlays a partial record witness onto an existing
// record shape, preserving siblings that the witness does not mention. It is
// used when flow facts prove a nested member more precisely without replacing
// the whole aggregate contract.
func OverlayRecordMembers(existing, overlay typ.Type) (typ.Type, bool) {
	existingRecord, ok := unwrap.Alias(existing).(*typ.Record)
	if !ok || existingRecord == nil {
		return nil, false
	}
	overlayRecord, ok := unwrap.Alias(overlay).(*typ.Record)
	if !ok || overlayRecord == nil {
		return nil, false
	}
	return overlayRecordMembers(existingRecord, overlayRecord), true
}

func overlayRecordMemberType(existing, replacement typ.Type) typ.Type {
	if existing == nil || replacement == nil {
		return replacement
	}
	if typ.SameNodeOrRecursiveIdentityEqual(existing, replacement) {
		return existing
	}
	if emptyRecordWitness(replacement) && declaredContainerType(existing) {
		return existing
	}
	existingRecord, existingOK := unwrap.Alias(existing).(*typ.Record)
	replacementRecord, replacementOK := unwrap.Alias(replacement).(*typ.Record)
	if !existingOK || existingRecord == nil || !replacementOK || replacementRecord == nil {
		return replacement
	}
	return overlayRecordMembers(existingRecord, replacementRecord)
}

func declaredContainerType(t typ.Type) bool {
	switch unwrap.Alias(t).(type) {
	case *typ.Array, *typ.Tuple, *typ.Map, *typ.ReadonlyMap:
		return true
	default:
		return false
	}
}

func emptyRecordWitness(t typ.Type) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil {
		return false
	}
	return len(rec.Fields) == 0 &&
		len(rec.StaticMembers) == 0 &&
		rec.Metatable == nil &&
		rec.MapKey == nil &&
		rec.MapValue == nil &&
		!rec.Open
}

func overlayRecordMembers(existingRecord, overlayRecord *typ.Record) typ.Type {
	fields := make([]typ.Field, 0, len(existingRecord.Fields)+len(overlayRecord.Fields))
	overlayFields := make(map[string]typ.Field, len(overlayRecord.Fields))
	for _, field := range overlayRecord.Fields {
		overlayFields[field.Name] = field
	}
	overlayStringMembers := make(map[string]typ.StaticMember)
	for _, member := range overlayRecord.StaticMembers {
		if member.Kind == typ.StaticMemberStringIndex && member.Name != "" {
			overlayStringMembers[member.Name] = member
		}
	}
	consumedStringMembers := make(map[string]struct{})
	seenFields := make(map[string]struct{}, len(existingRecord.Fields)+len(overlayRecord.Fields))
	for _, field := range existingRecord.Fields {
		if replacement, ok := overlayFields[field.Name]; ok {
			replacement.Type = overlayRecordMemberType(field.Type, replacement.Type)
			fields = append(fields, replacement)
		} else if replacement, ok := overlayStringMembers[field.Name]; ok {
			field.Type = overlayRecordMemberType(field.Type, replacement.Type)
			fields = append(fields, field)
			consumedStringMembers[field.Name] = struct{}{}
		} else {
			fields = append(fields, field)
		}
		seenFields[field.Name] = struct{}{}
	}
	for _, field := range overlayRecord.Fields {
		if _, seen := seenFields[field.Name]; !seen {
			fields = append(fields, field)
		}
	}

	members := overlayStaticMembers(existingRecord.StaticMembers, overlayRecord.StaticMembers, consumedStringMembers)
	metatable := existingRecord.Metatable
	if overlayRecord.Metatable != nil {
		metatable = overlayRecord.Metatable
	}
	mapKey := existingRecord.MapKey
	mapValue := existingRecord.MapValue
	if overlayRecord.MapKey != nil || overlayRecord.MapValue != nil {
		mapKey = overlayRecord.MapKey
		mapValue = overlayRecord.MapValue
	}
	rebuilt := RebuildRecord(typ.RecordParts{
		Fields:        fields,
		StaticMembers: members,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          existingRecord.Open || overlayRecord.Open,
	})
	if typ.SameNodeOrRecursiveIdentityEqual(existingRecord, rebuilt) {
		return existingRecord
	}
	return rebuilt
}

func overlayStaticMembers(existing []typ.StaticMember, overlay []typ.StaticMember, consumedStringMembers map[string]struct{}) []typ.StaticMember {
	out := make([]typ.StaticMember, 0, len(existing)+len(overlay))
	replacements := make(map[staticMemberKey]typ.StaticMember, len(overlay))
	for _, member := range overlay {
		if member.Kind == typ.StaticMemberStringIndex {
			if _, consumed := consumedStringMembers[member.Name]; consumed {
				continue
			}
		}
		replacements[staticMemberKeyOf(member)] = member
	}
	seen := make(map[staticMemberKey]struct{}, len(existing)+len(overlay))
	for _, member := range existing {
		key := staticMemberKeyOf(member)
		if replacement, ok := replacements[key]; ok {
			replacement.Type = overlayRecordMemberType(member.Type, replacement.Type)
			out = append(out, replacement)
		} else {
			out = append(out, member)
		}
		seen[key] = struct{}{}
	}
	for _, member := range overlay {
		if member.Kind == typ.StaticMemberStringIndex {
			if _, consumed := consumedStringMembers[member.Name]; consumed {
				continue
			}
		}
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
