package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// luaCovariantWiden rebuilds a covariantly-exposed object's witness type. It is
// the engine's injected widener for covariant mutable-view exposures: given the
// object's current witness, the exposure contract, and the source path segments
// locating the exposed sub-object under its ancestor symbol, it returns the
// ancestor witness with every strictly-wider field widened to the contract and
// the deduplicated top segment of every widened leaf so the engine can drop the
// precise per-field facts beneath it. It returns ok=false when no field widens.
//
// For a bare-root exposure (no segments) the contract is the object's own
// widened type. For a sub-path exposure the contract names only the sub-object's
// type, so it is spliced into the ancestor's witness at the sub-path before
// widening; this performs the ancestor repair that stops the sub-path's parent
// from re-projecting the narrow field type.
func luaCovariantWiden(sourceWitness, contract typ.Type, segments []segment.Segment) (typ.Type, [][]segment.Segment, bool) {
	sourceRecord, ok := unwrap.Alias(sourceWitness).(*typ.Record)
	if !ok || sourceRecord == nil {
		return nil, nil, false
	}
	targetRecord, ok := covariantTargetRecord(sourceRecord, segments, contract)
	if !ok || targetRecord == nil {
		return nil, nil, false
	}
	var leaves [][]segment.Segment
	widened, changed := widenRecordType(sourceRecord, targetRecord, nil, &leaves, make(map[[2]typ.Type]bool))
	if !changed {
		return nil, nil, false
	}
	return widened, topWidenedSegments(leaves), true
}

// covariantTargetRecord returns the widening target record for the ancestor
// symbol. For a bare-root exposure (no segments) the contract is itself the
// target record. For a sub-path exposure the contract is spliced into a copy of
// the source structure at the sub-path, so widenRecordType widens only the
// exposed sub-object's strictly-wider fields and records leaves rooted at the
// ancestor symbol.
func covariantTargetRecord(sourceRecord *typ.Record, segments []segment.Segment, contract typ.Type) (*typ.Record, bool) {
	if len(segments) == 0 {
		target, ok := unwrap.Alias(contract).(*typ.Record)
		return target, ok && target != nil
	}
	return spliceFieldType(sourceRecord, segments, contract)
}

// spliceFieldType returns a copy of source whose field at the segment path is
// replaced by replacement, preserving the rest of the source structure so the
// resulting record is identical to source except at the spliced leaf. It handles
// only field segments; an unmatched or non-record intermediate yields no splice.
func spliceFieldType(source *typ.Record, segments []segment.Segment, replacement typ.Type) (*typ.Record, bool) {
	if len(segments) == 0 || source == nil {
		return nil, false
	}
	seg := segments[0]
	if seg.Kind != segment.SegmentField {
		return nil, false
	}
	idx := -1
	for i := range source.Fields {
		if source.Fields[i].Name == seg.Name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, false
	}
	fields := make([]typ.Field, len(source.Fields))
	copy(fields, source.Fields)
	if len(segments) == 1 {
		fields[idx].Type = replacement
	} else {
		inner, ok := unwrap.Alias(fields[idx].Type).(*typ.Record)
		if !ok || inner == nil {
			return nil, false
		}
		splicedInner, ok := spliceFieldType(inner, segments[1:], replacement)
		if !ok {
			return nil, false
		}
		fields[idx].Type = splicedInner
	}
	return typ.RebuildRecord(typ.RecordParts{
		Fields:        fields,
		StaticMembers: source.StaticMembers,
		Metatable:     source.Metatable,
		MapKey:        source.MapKey,
		MapValue:      source.MapValue,
		Open:          source.Open,
	}), true
}

// topWidenedSegments returns the deduplicated first segment of every widened
// leaf path: the shallowest source field whose subtree must be dropped.
func topWidenedSegments(leaves [][]segment.Segment) [][]segment.Segment {
	seen := make(map[string]struct{}, len(leaves))
	var out [][]segment.Segment
	for _, leaf := range leaves {
		if len(leaf) == 0 {
			continue
		}
		top := leaf[:1]
		if _, ok := seen[top[0].Name]; ok {
			continue
		}
		seen[top[0].Name] = struct{}{}
		out = append(out, top)
	}
	return out
}

// widenRecordType returns a record with every shared field whose target type
// strictly widens the source field type widened to the target payload type,
// recursing into nested record fields. It records the source-relative segment
// path of every widened leaf into leaves and reports whether any field changed.
// The field's Optional flag is preserved; only the payload type widens.
func widenRecordType(source, target *typ.Record, prefix []segment.Segment, leaves *[][]segment.Segment, visited map[[2]typ.Type]bool) (*typ.Record, bool) {
	pairKey := [2]typ.Type{source, target}
	if visited[pairKey] {
		return source, false
	}
	visited[pairKey] = true

	fields := make([]typ.Field, len(source.Fields))
	copy(fields, source.Fields)
	changed := false
	for i := range fields {
		sf := fields[i]
		tf, ok := recordFieldByName(target, sf.Name)
		if !ok || sf.Type == nil || tf.Type == nil {
			continue
		}
		if typ.IsAny(sf.Type) || typ.IsUnknown(sf.Type) || typ.IsAny(tf.Type) || typ.IsUnknown(tf.Type) {
			continue
		}
		fieldSegments := appendFieldSegment(prefix, sf.Name)
		// A record-typed field refines its own leaves: recurse so the widen lands on
		// the exact widened leaf (inner.x), not the coarse container field.
		if sr, ok := recordPayload(sf); ok {
			if tr, ok := recordPayload(tf); ok {
				widenedInner, innerChanged := widenRecordType(sr, tr, fieldSegments, leaves, visited)
				if innerChanged {
					fields[i].Type = widenedInner
					changed = true
				}
				continue
			}
		}
		if covariantStrictlyWidens(sf.Type, tf.Type) {
			fields[i].Type = tf.Type
			*leaves = append(*leaves, fieldSegments)
			changed = true
		}
	}
	if !changed {
		return source, false
	}
	return typ.RebuildRecord(typ.RecordParts{
		Fields:        fields,
		StaticMembers: source.StaticMembers,
		Metatable:     source.Metatable,
		MapKey:        source.MapKey,
		MapValue:      source.MapValue,
		Open:          source.Open,
	}), true
}

func appendFieldSegment(prefix []segment.Segment, name string) []segment.Segment {
	next := make([]segment.Segment, len(prefix)+1)
	copy(next, prefix)
	next[len(prefix)] = segment.Segment{Kind: segment.SegmentField, Name: name}
	return next
}

func recordPayload(f typ.Field) (*typ.Record, bool) {
	r, ok := unwrap.Alias(f.Type).(*typ.Record)
	if !ok || r == nil {
		return nil, false
	}
	return r, true
}

func recordFieldByName(r *typ.Record, name string) (typ.Field, bool) {
	for i := range r.Fields {
		if r.Fields[i].Name == name {
			return r.Fields[i], true
		}
	}
	return typ.Field{}, false
}

func covariantStrictlyWidens(narrow, wide typ.Type) bool {
	return subtype.IsSubtype(narrow, wide) && !subtype.IsSubtype(wide, narrow)
}
