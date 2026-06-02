// Package ops provides type synthesis for expressions and function calls.
//
// For function call synthesis, use the two-phase approach:
//
//	infer := ops.InferCall(ctx, def)    // Phase 1: resolve callee, infer type args
//	// ... optionally re-synthesize args using infer.ExpectedArgs ...
//	infer = ops.ReInfer(ctx, def, infer) // Re-infer with updated args
//	result := ops.FinishCall(ctx, def, infer) // Phase 2: check args, compute return
//
// For simple cases, CallWithGenericInference wraps the full flow.
package ops

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

// FieldDef describes a field in a table constructor.
type FieldDef struct {
	Name     string
	Type     typ.Type
	Optional bool
}

// EntryDef describes a structurally-keyed entry in a table constructor.
// SegmentField models `{name = v}` / `.name`; SegmentIndexString and
// SegmentIndexInt model exact bracket keys (`["name"]`, `[1]`). Keeping this
// carrier structural prevents record fields and bracket indexes from collapsing
// into a boundary string before the expected type policy is known.
type EntryDef struct {
	Key      constraint.Segment
	Type     typ.Type
	Optional bool
}

func fieldDefEntries(fields []FieldDef) []EntryDef {
	if len(fields) == 0 {
		return nil
	}
	entries := make([]EntryDef, 0, len(fields))
	for _, f := range fields {
		if f.Name == "" {
			continue
		}
		entries = append(entries, EntryDef{
			Key:      constraint.Segment{Kind: constraint.SegmentField, Name: f.Name},
			Type:     f.Type,
			Optional: f.Optional,
		})
	}
	return entries
}

// tableConstructor synthesizes the structural table-constructor result.
func tableConstructor(fields []FieldDef, array []typ.Type) typ.Type {
	return tableConstructorEntries(fieldDefEntries(fields), array)
}

// tableConstructorEntries synthesizes the structural table-constructor result
// from the canonical keyed-entry carrier.
func tableConstructorEntries(entries []EntryDef, array []typ.Type) typ.Type {
	if len(entries) == 0 && len(array) == 0 {
		return typ.NewFreshEmptyRecord()
	}

	// Pure array
	if len(entries) == 0 {
		return synthesizeArray(array)
	}

	var out typ.Type = typ.NewRecord().Build()
	for i, elem := range array {
		if elem == nil {
			elem = typ.Unknown
		}
		out = applyTableEntry(out, constraint.Segment{Kind: constraint.SegmentIndexInt, Index: i + 1}, elem)
	}
	for _, entry := range entries {
		ft := entry.Type
		if ft == nil {
			ft = typ.Unknown
		}
		if entry.Optional {
			ft = typ.NewOptional(ft)
		}
		out = applyTableEntry(out, entry.Key, ft)
	}

	return out
}

func applyTableEntry(base typ.Type, key constraint.Segment, valueType typ.Type) typ.Type {
	switch key.Kind {
	case constraint.SegmentField:
		if key.Name == "" {
			return base
		}
		return typ.ExtendRecordWithField(base, key.Name, valueType)
	case constraint.SegmentIndexString:
		return value.AdmitForeignIndexedWrite(base, typ.LiteralString(key.Name), valueType)
	case constraint.SegmentIndexInt:
		return value.AdmitForeignIndexedWrite(base, typ.LiteralInt(int64(key.Index)), valueType)
	default:
		return base
	}
}

// synthesizeArray creates array type from elements.
func synthesizeArray(elements []typ.Type) typ.Type {
	if len(elements) == 0 {
		return typ.NewFreshArray()
	}

	// Union of all element types
	elemType := elements[0]
	if elemType == nil {
		elemType = typ.Unknown
	}
	for i := 1; i < len(elements); i++ {
		next := elements[i]
		if next == nil {
			next = typ.Unknown
		}
		elemType = typ.NewUnion(elemType, next)
	}

	return typ.NewArray(elemType)
}
