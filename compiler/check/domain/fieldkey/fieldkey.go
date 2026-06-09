// Package fieldkey owns typed keys for statically-known field/index slots.
package fieldkey

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/pathseg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// Key identifies a statically-known field/index slot. Boundary collectors may
// still speak in source names, but product/domain internals should carry this
// structural key so field and string-index segments do not collapse into an
// ad-hoc string carrier.
type Key = constraint.Segment

// Values is the product carrier for field-indexed facts.
type Values map[Key]product.AbstractValue

// FromName lowers a source field name into a structural field key.
func FromName(name string) (Key, bool) {
	if name == "" {
		return Key{}, false
	}
	return Key{Kind: constraint.SegmentField, Name: name}, true
}

// FromSegment accepts a static field/index segment as a structural field key.
func FromSegment(seg constraint.Segment) (Key, bool) {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		if seg.Kind == constraint.SegmentField && seg.Name == "" {
			return Key{}, false
		}
		return seg, true
	case constraint.SegmentIndexInt:
		return seg, true
	default:
		return Key{}, false
	}
}

// FromTableField lowers a source table-constructor field to a structural field
// key using the field's syntax bit. Dynamic and positional fields are rejected.
func FromTableField(field *ast.Field) (Key, bool) {
	seg, ok := pathseg.StaticTableFieldSegment(field)
	if !ok {
		return Key{}, false
	}
	return FromSegment(seg)
}

// FromTableFieldWithConst is FromTableField plus compile-time constant
// resolution for bracket-syntax dynamic identifiers such as `{[k] = v}`.
func FromTableFieldWithConst(field *ast.Field, constResolver func(string) *flow.ConstValue) (Key, bool) {
	seg, ok := pathseg.StaticTableFieldSegmentWithConst(field, constResolver)
	if !ok {
		return Key{}, false
	}
	return FromSegment(seg)
}

// StringKeyFromSegment projects a structural segment to the runtime Lua string
// key it writes/reads. Dot fields and bracket string indexes both address a
// string table key; integer indexes and dynamic keys are rejected. Unlike Name,
// this treats an empty bracket string as a valid Lua key.
func StringKeyFromSegment(seg constraint.Segment) (string, bool) {
	switch seg.Kind {
	case constraint.SegmentField:
		return seg.Name, seg.Name != ""
	case constraint.SegmentIndexString:
		return seg.Name, true
	default:
		return "", false
	}
}

// StringKeyFromTableField lowers a source table-constructor field to its
// runtime Lua string key. Use this for semantic table keys such as metamethod
// names or effect field paths, not for typed-record construction policy.
func StringKeyFromTableField(field *ast.Field) (string, bool) {
	key, ok := FromTableField(field)
	if !ok {
		return "", false
	}
	return StringKeyFromSegment(key)
}

// StringKeyFromTableFieldWithConst is StringKeyFromTableField plus
// compile-time constant resolution for bracket-syntax dynamic identifiers.
func StringKeyFromTableFieldWithConst(field *ast.Field, constResolver func(string) *flow.ConstValue) (string, bool) {
	key, ok := FromTableFieldWithConst(field, constResolver)
	if !ok {
		return "", false
	}
	return StringKeyFromSegment(key)
}

// RecordFieldNameFromTableField lowers only table-constructor fields that are
// record fields under source syntax. Bracket string/index fields remain
// structural index members and do not satisfy a closed typed-record constructor.
func RecordFieldNameFromTableField(field *ast.Field) (string, bool) {
	key, ok := FromTableField(field)
	if !ok || key.Kind != constraint.SegmentField {
		return "", false
	}
	return key.Name, key.Name != ""
}

// Sorted returns field keys in deterministic structural order.
func Sorted[T any](m map[Key]T) []Key {
	if len(m) == 0 {
		return nil
	}
	keys := make([]Key, 0, len(m))
	for key := range m {
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
	return keys
}

// LiftTypeMap admits name-keyed field facts into the structural product carrier.
func LiftTypeMap(fields map[string]typ.Type) Values {
	if len(fields) == 0 {
		return nil
	}
	out := make(Values, len(fields))
	for name, fieldType := range fields {
		key, ok := FromName(name)
		if !ok {
			continue
		}
		out[key] = product.FromType(fieldType)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ProjectValueMap projects structural product field facts back to string-keyed
// record-field facts for export/overlay consumers.
func ProjectValueMap(fields Values) map[string]typ.Type {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(fields))
	for _, key := range Sorted(fields) {
		name, ok := StringKeyFromSegment(key)
		if !ok {
			continue
		}
		out[name] = fields[key].ProjectValue()
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
