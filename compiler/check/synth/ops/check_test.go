package ops

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCheckTable_NoExpected(t *testing.T) {
	fields := []FieldDef{{Name: "x", Type: typ.Integer}}

	result := CheckTable(fields, nil, nil)
	if result.Type == nil {
		t.Error("should synthesize type")
	}

	if len(result.Errors) > 0 {
		t.Error("should not have errors in synthesis mode")
	}
}

func TestCheckTable_ExpectedAny(t *testing.T) {
	fields := []FieldDef{{Name: "x", Type: typ.Integer}}

	result := CheckTable(fields, nil, typ.Any)
	if len(result.Errors) > 0 {
		t.Error("any should accept any table")
	}
}

func TestCheckTable_ExpectedReadonlyMapUsesReadViewContract(t *testing.T) {
	fields := []FieldDef{{Name: "x", Type: typ.Integer}}
	expected := typ.NewReadonlyMap(typ.String, typ.Number)

	result := CheckTable(fields, nil, expected)
	if len(result.Errors) > 0 {
		t.Fatalf("readonly map should accept compatible present entries: %v", result.Errors)
	}
	if !typ.TypeEquals(result.Type, expected) {
		t.Fatalf("CheckTable readonly map type = %v, want %v", result.Type, expected)
	}

	bad := CheckTable(fields, nil, typ.NewReadonlyMap(typ.Number, typ.Number))
	if len(bad.Errors) == 0 {
		t.Fatal("readonly map should reject incompatible literal field key")
	}
}

func TestCheckTable_ExpectedRecord_Match(t *testing.T) {
	fields := []FieldDef{
		{Name: "x", Type: typ.Integer},
		{Name: "y", Type: typ.String},
	}
	expected := &typ.Record{
		Fields: []typ.Field{
			{Name: "x", Type: typ.Integer},
			{Name: "y", Type: typ.String},
		},
	}

	result := CheckTable(fields, nil, expected)
	if len(result.Errors) > 0 {
		t.Errorf("should match expected: %v", result.Errors)
	}
}

func TestCheckTable_ExpectedRecord_MissingField(t *testing.T) {
	fields := []FieldDef{{Name: "x", Type: typ.Integer}}
	expected := &typ.Record{
		Fields: []typ.Field{
			{Name: "x", Type: typ.Integer},
			{Name: "y", Type: typ.String},
		},
	}

	result := CheckTable(fields, nil, expected)
	if len(result.Errors) == 0 {
		t.Error("should report missing required field")
	}
}

func TestCheckTable_ExpectedRecord_OptionalField(t *testing.T) {
	fields := []FieldDef{{Name: "x", Type: typ.Integer}}
	expected := &typ.Record{
		Fields: []typ.Field{
			{Name: "x", Type: typ.Integer},
			{Name: "y", Type: typ.String, Optional: true},
		},
	}

	result := CheckTable(fields, nil, expected)
	if len(result.Errors) > 0 {
		t.Errorf("optional field should not be required: %v", result.Errors)
	}
}

func TestCheckTable_ExpectedRecord_OptionalFieldAcceptsOptionalValue(t *testing.T) {
	fields := []FieldDef{
		{Name: "x", Type: typ.Integer},
		{Name: "y", Type: typ.NewOptional(typ.String)},
	}
	expected := &typ.Record{
		Fields: []typ.Field{
			{Name: "x", Type: typ.Integer},
			{Name: "y", Type: typ.String, Optional: true},
		},
	}

	result := CheckTable(fields, nil, expected)
	if len(result.Errors) > 0 {
		t.Errorf("optional field should accept optional value expression: %v", result.Errors)
	}
}

func TestCheckTable_ExpectedRecord_ExtraField(t *testing.T) {
	fields := []FieldDef{
		{Name: "x", Type: typ.Integer},
		{Name: "z", Type: typ.Boolean},
	}
	expected := &typ.Record{
		Fields: []typ.Field{{Name: "x", Type: typ.Integer}},
	}
	result := CheckTable(fields, nil, expected)
	hasExtraError := false

	for _, err := range result.Errors {
		if err.Message == "unexpected field" {
			hasExtraError = true
			break
		}
	}

	if !hasExtraError {
		t.Error("should report unexpected field")
	}
}

func TestCheckTable_ExpectedArray(t *testing.T) {
	expected := &typ.Array{Element: typ.Integer}

	result := CheckTable(nil, []typ.Type{typ.Integer, typ.Integer}, expected)
	if len(result.Errors) > 0 {
		t.Errorf("should match array: %v", result.Errors)
	}
}

func TestCheckTable_ExpectedArray_TypeMismatch(t *testing.T) {
	expected := &typ.Array{Element: typ.Integer}

	result := CheckTable(nil, []typ.Type{typ.Integer, typ.String}, expected)
	if len(result.Errors) == 0 {
		t.Error("should report element type mismatch")
	}
}

func TestCheckTable_ExpectedTuple(t *testing.T) {
	expected := typ.NewTuple(typ.Integer, typ.String)

	result := CheckTable(nil, []typ.Type{typ.Integer, typ.String}, expected)
	if len(result.Errors) > 0 {
		t.Errorf("should match tuple: %v", result.Errors)
	}
}

func TestCheckTable_ExpectedTuple_TooFew(t *testing.T) {
	expected := typ.NewTuple(typ.Integer, typ.String, typ.Boolean)

	result := CheckTable(nil, []typ.Type{typ.Integer}, expected)
	if len(result.Errors) == 0 {
		t.Error("should report not enough elements")
	}
}

func TestCheckTable_ExpectedMap(t *testing.T) {
	expected := &typ.Map{Key: typ.String, Value: typ.Integer}
	fields := []FieldDef{
		{Name: "a", Type: typ.Integer},
		{Name: "b", Type: typ.Integer},
	}

	result := CheckTable(fields, nil, expected)
	if len(result.Errors) > 0 {
		t.Errorf("should match map: %v", result.Errors)
	}
}

func TestCheckTableEntries_BracketStringDoesNotSatisfyRecordField(t *testing.T) {
	expected := typ.NewRecord().Field("name", typ.String).Build()
	entries := []EntryDef{{
		Key:  constraint.Segment{Kind: constraint.SegmentIndexString, Name: "name"},
		Type: typ.String,
	}}

	result := CheckTableEntries(entries, nil, expected)
	if len(result.Errors) == 0 {
		t.Fatal("bracket string entry should not satisfy dot record field")
	}
}

func TestCheckTableEntries_StaticMembersUseStructuralKeys(t *testing.T) {
	expected := typ.NewRecord().
		StaticStringIndex("name", typ.String).
		StaticIntIndex(1, typ.Integer).
		Build()
	entries := []EntryDef{
		{Key: constraint.Segment{Kind: constraint.SegmentIndexString, Name: "name"}, Type: typ.String},
		{Key: constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1}, Type: typ.Integer},
	}

	result := CheckTableEntries(entries, nil, expected)
	if len(result.Errors) > 0 {
		t.Fatalf("static members should accept structural bracket entries: %v", result.Errors)
	}
}

func TestCheckTableEntries_BracketEntriesUseMapAndArraySlots(t *testing.T) {
	mapResult := CheckTableEntries(
		[]EntryDef{{Key: constraint.Segment{Kind: constraint.SegmentIndexString, Name: "name"}, Type: typ.Integer}},
		nil,
		typ.NewMap(typ.String, typ.Integer),
	)
	if len(mapResult.Errors) > 0 {
		t.Fatalf("string bracket entry should use map slot: %v", mapResult.Errors)
	}

	arrayResult := CheckTableEntries(
		[]EntryDef{{Key: constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1}, Type: typ.Integer}},
		nil,
		typ.NewArray(typ.Integer),
	)
	if len(arrayResult.Errors) > 0 {
		t.Fatalf("int bracket entry should use array slot: %v", arrayResult.Errors)
	}
}

func TestCheckTable_ExpectedAlias(t *testing.T) {
	rec := &typ.Record{Fields: []typ.Field{{Name: "x", Type: typ.Integer}}}
	alias := &typ.Alias{Name: "MyRec", Target: rec}
	fields := []FieldDef{{Name: "x", Type: typ.Integer}}

	result := CheckTable(fields, nil, alias)
	if len(result.Errors) > 0 {
		t.Errorf("should unwrap alias: %v", result.Errors)
	}
}

func TestCheckTable_ExpectedOptional(t *testing.T) {
	rec := &typ.Record{Fields: []typ.Field{{Name: "x", Type: typ.Integer}}}
	opt := typ.NewOptional(rec)
	fields := []FieldDef{{Name: "x", Type: typ.Integer}}

	result := CheckTable(fields, nil, opt)
	if len(result.Errors) > 0 {
		t.Errorf("should handle optional: %v", result.Errors)
	}
}

func TestCheckTable_NilFieldTypeDoesNotPanic(t *testing.T) {
	fields := []FieldDef{{Name: "x", Type: nil}}
	result := CheckTable(fields, nil, nil)
	if result.Type == nil {
		t.Error("expected synthesized type for nil field type")
	}
}

func TestCheckTable_NilArrayElementDoesNotPanic(t *testing.T) {
	result := CheckTable(nil, []typ.Type{nil}, nil)
	if result.Type == nil {
		t.Error("expected synthesized type for nil array element")
	}
}

func TestCheckError_Fields(t *testing.T) {
	err := CheckError{
		Message:  "test error",
		Expected: typ.Integer,
		Got:      typ.String,
		Field:    "x",
	}
	if err.Message != "test error" {
		t.Error("wrong message")
	}

	if err.Expected != typ.Integer {
		t.Error("wrong expected")
	}

	if err.Got != typ.String {
		t.Error("wrong got")
	}

	if err.Field != "x" {
		t.Error("wrong field")
	}
}

func TestExpectedTableElementType_Array(t *testing.T) {
	expected := typ.NewArray(typ.String)
	got := ExpectedTableElementType(expected, 0)
	if got != typ.String {
		t.Fatalf("got %v, want string", got)
	}
}

func TestExpectedTableElementType_TupleUsesIndex(t *testing.T) {
	expected := typ.NewTuple(typ.String, typ.Integer)
	if got := ExpectedTableElementType(expected, 0); got != typ.String {
		t.Fatalf("index 0 got %v, want string", got)
	}
	if got := ExpectedTableElementType(expected, 1); got != typ.Integer {
		t.Fatalf("index 1 got %v, want integer", got)
	}
}

func TestExpectedTableElementType_UnionCollectsMembers(t *testing.T) {
	expected := typ.NewUnion(
		typ.NewArray(typ.String),
		typ.NewTuple(typ.Integer, typ.Boolean),
	)
	got := ExpectedTableElementType(expected, 0)
	if !typ.TypeEquals(got, typ.NewUnion(typ.String, typ.Integer)) {
		t.Fatalf("got %v, want string|integer", got)
	}
}

func TestExpectedTableElementType_NumericMap(t *testing.T) {
	expected := typ.NewMap(typ.Integer, typ.Boolean)
	got := ExpectedTableElementType(expected, 0)
	if got != typ.Boolean {
		t.Fatalf("got %v, want boolean", got)
	}
}

func TestExpectedTableFieldType_StringMapUsesWriteSlot(t *testing.T) {
	expected := typ.NewMap(typ.String, typ.Boolean)
	got := ExpectedTableFieldType(expected, "ready")
	if got != typ.Boolean {
		t.Fatalf("got %v, want boolean", got)
	}
}

func TestExpectedTableFieldType_RecordMapComponentUsesLiteralKey(t *testing.T) {
	expected := typ.NewRecord().
		Field("fixed", typ.String).
		MapComponent(typ.String, typ.Integer).
		Build()

	if got := ExpectedTableFieldType(expected, "fixed"); got != typ.String {
		t.Fatalf("fixed field got %v, want string", got)
	}
	if got := ExpectedTableFieldType(expected, "dynamic"); got != typ.Integer {
		t.Fatalf("dynamic field got %v, want integer", got)
	}
}

func TestExpectedTableFieldType_UnionCollectsPossibleSlots(t *testing.T) {
	expected := typ.NewUnion(
		typ.NewRecord().Field("value", typ.String).Build(),
		typ.NewRecord().Field("value", typ.Integer).Build(),
	)
	got := ExpectedTableFieldType(expected, "value")
	if !typ.TypeEquals(got, typ.NewUnion(typ.String, typ.Integer)) {
		t.Fatalf("got %v, want string|integer", got)
	}
}
