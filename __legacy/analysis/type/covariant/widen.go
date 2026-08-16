// Package covariant owns callback-free type witness widening for mutable-view
// exposure transactions.
package covariant

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

// WidenRecord rebuilds a covariantly exposed ancestor record and returns the
// top-level field segments whose precise descendants must be invalidated.
func WidenRecord(sourceWitness, contract typ.Type, segments []segment.Segment) (typ.Type, [][]segment.Segment, bool) {
	sourceRecord, ok := unwrap.Alias(sourceWitness).(*typ.Record)
	if !ok || sourceRecord == nil {
		return nil, nil, false
	}
	targetRecord, ok := targetRecord(sourceRecord, segments, contract)
	if !ok || targetRecord == nil {
		return nil, nil, false
	}
	var leaves [][]segment.Segment
	widened, changed := widenRecord(sourceRecord, targetRecord, nil, &leaves, make(map[[2]typ.Type]bool))
	if !changed {
		return nil, nil, false
	}
	return widened, topSegments(leaves), true
}

func targetRecord(source *typ.Record, segments []segment.Segment, contract typ.Type) (*typ.Record, bool) {
	if len(segments) == 0 {
		target, ok := unwrap.Alias(contract).(*typ.Record)
		return target, ok && target != nil
	}
	return spliceFieldType(source, segments, contract)
}

func spliceFieldType(source *typ.Record, segments []segment.Segment, replacement typ.Type) (*typ.Record, bool) {
	if len(segments) == 0 || source == nil || segments[0].Kind != segment.SegmentField {
		return nil, false
	}
	index := -1
	for i := range source.Fields {
		if source.Fields[i].Name == segments[0].Name {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, false
	}
	fields := append([]typ.Field(nil), source.Fields...)
	if len(segments) == 1 {
		fields[index].Type = replacement
	} else {
		inner, ok := unwrap.Alias(fields[index].Type).(*typ.Record)
		if !ok || inner == nil {
			return nil, false
		}
		spliced, ok := spliceFieldType(inner, segments[1:], replacement)
		if !ok {
			return nil, false
		}
		fields[index].Type = spliced
	}
	return rebuild(source, fields), true
}

func widenRecord(source, target *typ.Record, prefix []segment.Segment, leaves *[][]segment.Segment, visited map[[2]typ.Type]bool) (*typ.Record, bool) {
	pair := [2]typ.Type{source, target}
	if visited[pair] {
		return source, false
	}
	visited[pair] = true
	fields := append([]typ.Field(nil), source.Fields...)
	changed := false
	for index, sourceField := range fields {
		targetField, ok := fieldByName(target, sourceField.Name)
		if !ok || sourceField.Type == nil || targetField.Type == nil ||
			typ.IsAny(sourceField.Type) || typ.IsUnknown(sourceField.Type) || typ.IsAny(targetField.Type) || typ.IsUnknown(targetField.Type) {
			continue
		}
		fieldSegments := appendSegment(prefix, sourceField.Name)
		if sourceRecord, ok := recordPayload(sourceField); ok {
			if targetRecord, ok := recordPayload(targetField); ok {
				widened, innerChanged := widenRecord(sourceRecord, targetRecord, fieldSegments, leaves, visited)
				if innerChanged {
					fields[index].Type = widened
					changed = true
				}
				continue
			}
		}
		if subtype.IsSubtype(sourceField.Type, targetField.Type) && !subtype.IsSubtype(targetField.Type, sourceField.Type) {
			fields[index].Type = targetField.Type
			*leaves = append(*leaves, fieldSegments)
			changed = true
		}
	}
	if !changed {
		return source, false
	}
	return rebuild(source, fields), true
}

func rebuild(source *typ.Record, fields []typ.Field) *typ.Record {
	return typ.RebuildRecord(typ.RecordParts{
		Fields: fields, StaticMembers: source.StaticMembers, Metatable: source.Metatable,
		MapKey: source.MapKey, MapValue: source.MapValue, Open: source.Open,
	})
}

func topSegments(leaves [][]segment.Segment) [][]segment.Segment {
	seen := make(map[string]struct{}, len(leaves))
	var out [][]segment.Segment
	for _, leaf := range leaves {
		if len(leaf) == 0 {
			continue
		}
		if _, duplicate := seen[leaf[0].Name]; duplicate {
			continue
		}
		seen[leaf[0].Name] = struct{}{}
		out = append(out, append([]segment.Segment(nil), leaf[:1]...))
	}
	return out
}

func appendSegment(prefix []segment.Segment, name string) []segment.Segment {
	next := make([]segment.Segment, len(prefix)+1)
	copy(next, prefix)
	next[len(prefix)] = segment.Segment{Kind: segment.SegmentField, Name: name}
	return next
}

func recordPayload(field typ.Field) (*typ.Record, bool) {
	record, ok := unwrap.Alias(field.Type).(*typ.Record)
	return record, ok && record != nil
}

func fieldByName(record *typ.Record, name string) (typ.Field, bool) {
	for _, field := range record.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return typ.Field{}, false
}
