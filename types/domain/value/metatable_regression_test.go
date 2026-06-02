package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// classFull is a class observed with its full method surface.
func classFull() *typ.Record {
	return typ.NewRecord().
		Field("type", typ.Func().Param("self", typ.Unknown).Returns(typ.Unknown).Build()).
		Field("all", typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()).
		Build()
}

// classEmpty is the same class observed early, before methods are attached.
func classEmpty() *typ.Record {
	return typ.NewRecord().Build()
}

func TestMetatableConvergence_MergesSplitAndSelfClassViews(t *testing.T) {
	// View A: methods live in the __index prototype (split view).
	proto := typ.NewRecord().
		Field("type", typ.Func().Param("self", typ.Unknown).Returns(typ.Unknown).Build()).
		Field("all", typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()).
		Build()
	viewSplit := typ.NewRecord().Field(indexFieldName, proto).Build()
	// View B: methods live at the top (self view), __index points at unknown.
	viewSelf := typ.NewRecord().
		Field("type", typ.Func().Param("self", typ.Unknown).Returns(typ.Unknown).Build()).
		Field("all", typ.Func().Param("self", typ.Unknown).Returns(typ.Number).Build()).
		Field(indexFieldName, typ.Unknown).
		Build()

	for _, tc := range []struct {
		name           string
		existing, cand typ.Type
	}{
		{"split_then_self", viewSplit, viewSelf},
		{"self_then_split", viewSelf, viewSplit},
	} {
		merged := MergeForConvergence(tc.existing, tc.cand)
		rec := requireRecordType(t, tc.name, merged)
		requireRecordField(t, tc.name, rec, "type", false)
		requireRecordField(t, tc.name, rec, "all", false)

		indexField := requireRecordField(t, tc.name, rec, indexFieldName, false)
		indexRec := requireRecordType(t, tc.name+"."+indexFieldName, indexField.Type)
		requireRecordField(t, tc.name+"."+indexFieldName, indexRec, "type", false)
		requireRecordField(t, tc.name+"."+indexFieldName, indexRec, "all", false)
	}
}

const indexFieldName = "__index"

func TestMetatableConvergence_PreservesMethodSurfaceAcrossEmptyView(t *testing.T) {
	full := classFull()
	empty := classEmpty()

	for _, tc := range []struct {
		name           string
		existing, cand typ.Type
	}{
		{"empty_then_full", empty, full},
		{"full_then_empty", full, empty},
	} {
		merged := MergeForConvergence(tc.existing, tc.cand)
		rec := requireRecordType(t, tc.name, merged)
		requireRecordField(t, tc.name, rec, "type", false)
		requireRecordField(t, tc.name, rec, "all", false)
	}
}

func requireRecordType(t *testing.T, label string, ty typ.Type) *typ.Record {
	t.Helper()
	rec, ok := ty.(*typ.Record)
	if !ok {
		t.Fatalf("[%s] type is not a record: %s", label, typ.FormatShort(ty))
	}
	return rec
}

func requireRecordField(t *testing.T, label string, rec *typ.Record, name string, wantOptional bool) typ.Field {
	t.Helper()
	field := rec.GetField(name)
	if field == nil {
		t.Fatalf("[%s] missing field %q in %s", label, name, typ.FormatShort(rec))
	}
	if field.Optional != wantOptional {
		t.Fatalf("[%s] field %q optional=%v, want %v in %s", label, name, field.Optional, wantOptional, typ.FormatShort(rec))
	}
	return *field
}
