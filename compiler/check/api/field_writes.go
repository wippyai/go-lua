package api

import (
	"sort"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

// FieldWriteKey locates a field written through a target variable.
//
// Path is the static segment path from the target to the written table, in
// the format of constraint.FormatSegments; it is empty when the written table
// is the target itself. Field is the written field name, or
// flow.IndexerWriteField for writes by dynamic keys, whose written type is the
// map {[K]: V} they add to the table.
//
//	t.x = v       -> {Path: "", Field: "x"}
//	t.a.b.x = v   -> {Path: ".a.b", Field: "x"}
//	t.a[k] = v    -> {Path: ".a", Field: flow.IndexerWriteField}
type FieldWriteKey struct {
	Path  string
	Field string
}

// FieldWriteSet maps the fields written through one target to their types.
type FieldWriteSet = map[FieldWriteKey]typ.Type

// NewFieldWriteKey returns the key of field in the table at segments below the target.
func NewFieldWriteKey(segments []constraint.Segment, field string) FieldWriteKey {
	return FieldWriteKey{Path: constraint.FormatSegments(segments), Field: field}
}

// Segments returns the segment path from the target to the written table.
func (k FieldWriteKey) Segments() []constraint.Segment {
	return pathkey.ParseSuffix(k.Path)
}

// IsIndexer reports whether k records writes by dynamic keys.
func (k FieldWriteKey) IsIndexer() bool {
	return k.Field == flow.IndexerWriteField
}

// Under returns k relocated below prefix, the path from a new target to k's target.
func (k FieldWriteKey) Under(prefix []constraint.Segment) FieldWriteKey {
	if len(prefix) == 0 {
		return k
	}
	return FieldWriteKey{Path: constraint.FormatSegments(prefix) + k.Path, Field: k.Field}
}

// SortedFieldWriteKeys returns the keys of set ordered by path, then field.
func SortedFieldWriteKeys[T any](set map[FieldWriteKey]T) []FieldWriteKey {
	if len(set) == 0 {
		return nil
	}
	keys := make([]FieldWriteKey, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Path != keys[j].Path {
			return keys[i].Path < keys[j].Path
		}
		return keys[i].Field < keys[j].Field
	})
	return keys
}

// JoinFieldWrite joins two types written at key. Writes by dynamic keys join
// the maps they add component-wise, so the result stays one map; writes of a
// named field join as a union.
func JoinFieldWrite(key FieldWriteKey, prev, next typ.Type) typ.Type {
	if prev == nil {
		return next
	}
	if next == nil {
		return prev
	}
	if key.IsIndexer() {
		pm, pok := prev.(*typ.Map)
		nm, nok := next.(*typ.Map)
		if pok && nok {
			return typ.NewMap(typ.JoinPreferNonSoft(pm.Key, nm.Key), typ.JoinPreferNonSoft(pm.Value, nm.Value))
		}
	}
	return typ.NewUnion(prev, next)
}

// NewIndexerWrite returns the map {[K]: V} that writes of val by keys of type
// key add to a table. Storing nil removes an entry, so V is the non-nil part
// of val; a write of nil alone adds nothing and yields nil.
func NewIndexerWrite(key, val typ.Type) typ.Type {
	if val != nil {
		val = narrow.RemoveNil(val)
		if val.Kind() == kind.Never {
			return nil
		}
	}
	return typ.NewMap(key, val)
}
