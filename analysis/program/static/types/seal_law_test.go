package types

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func sealTable(t *testing.T, input Input) Table {
	t.Helper()
	table, err := Build(input, ledgerCounts())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return table
}

// TestTypesPreserveExactTypedRelations proves every authored relation survives
// the seal with its scalars and its ordered child column intact.
func TestTypesPreserveExactTypedRelations(t *testing.T) {
	types := sealTable(t, ledgerInput()).View()
	union := term(keyspace.FamilyTypeUnion, 1)
	intersection := term(keyspace.FamilyTypeIntersection, 1)
	generic := term(keyspace.FamilyTypeGeneric, 1)
	array := term(keyspace.FamilyTypeArray, 1)
	mapType := term(keyspace.FamilyTypeMap, 1)
	field := term(keyspace.FamilyTypeField, 1)
	record := term(keyspace.FamilyTypeRecord, 1)

	if got := types.Primitives().Count(); got != 5 {
		t.Fatalf("primitive count = %d, want 5", got)
	}
	if kind, ok := types.Primitives().Get(primitiveTerm(2)); !ok || kind != PrimitiveNumber {
		t.Fatalf("primitive relation = (%v, %v), want number", kind, ok)
	}
	if kind, exact, bits, ok := types.Literals().Get(term(keyspace.FamilyTypeLiteral, 1)); !ok ||
		kind != keyspace.LiteralString || exact != 7 || bits != 0 {
		t.Fatalf("literal relation = (%v, %v, %d, %v)", kind, exact, bits, ok)
	}
	if inner, ok := types.Optionals().Get(term(keyspace.FamilyTypeOptional, 1)); !ok || inner != primitiveTerm(1) {
		t.Fatalf("optional relation = (%v, %v)", inner, ok)
	}
	if got, ok := types.Unions().MemberCount(union); !ok || got != 2 {
		t.Fatalf("union length = (%d, %v)", got, ok)
	}
	if got, ok := types.Unions().MemberAt(union, 1); !ok || got != primitiveTerm(2) {
		t.Fatalf("union member = (%v, %v)", got, ok)
	}
	if got, ok := types.Intersections().MemberAt(intersection, 0); !ok || got != primitiveTerm(3) {
		t.Fatalf("intersection member = (%v, %v)", got, ok)
	}
	if base, arity, ok := types.Generics().Get(generic); !ok || base != term(keyspace.FamilyTypeRef, 1) || arity != 1 {
		t.Fatalf("generic relation = (%v, %d, %v)", base, arity, ok)
	}
	if got, ok := types.Generics().ArgAt(generic, 0); !ok || got != union {
		t.Fatalf("generic argument = (%v, %v)", got, ok)
	}
	if element, readOnly, ok := types.Arrays().Get(array); !ok || element != generic || !readOnly {
		t.Fatalf("array relation = (%v, %v, %v)", element, readOnly, ok)
	}
	if key, value, readOnly, ok := types.Maps().Get(mapType); !ok ||
		key != term(keyspace.FamilyTypeRef, 2) || value != array || readOnly {
		t.Fatalf("map relation = (%v, %v, %v, %v)", key, value, readOnly, ok)
	}
	if key, typ, optionalField, ok := types.Fields().Get(field); !ok || key != 9 || typ != mapType || !optionalField {
		t.Fatalf("field relation = (%v, %v, %v, %v)", key, typ, optionalField, ok)
	}
	if readOnly, fields, ok := types.Records().Get(record); !ok || !readOnly || fields != 1 {
		t.Fatalf("record relation = (%v, %d, %v)", readOnly, fields, ok)
	}
	if got, ok := types.Records().FieldAt(record, 0); !ok || got != field {
		t.Fatalf("record field = (%v, %v)", got, ok)
	}
}

// TestTypesRejectIncompleteOrAmbiguousLocalShape proves the admissions this
// vertical owns. The containment forest spans every vertical and belongs to
// the enclosing owner's combined seal -- including what a Field's value type
// may name, which is an attach edge rather than a local row shape.
func TestTypesRejectIncompleteOrAmbiguousLocalShape(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Input)
	}{
		{"one-member union", func(in *Input) { in.Union[0].Members = in.Union[0].Members[:1] }},
		{"one-member intersection", func(in *Input) {
			in.Intersection[0].Members = in.Intersection[0].Members[:1]
		}},
		{"nonstatic union member", func(in *Input) {
			in.Union[0].Members[0] = term(keyspace.FamilyCell, 1)
		}},
		{"unknown primitive kind", func(in *Input) { in.Primitive[0].Kind = 99 }},
		{"string literal without exact key", func(in *Input) { in.Literal[0].Exact = 0 }},
		{"string literal with float payload", func(in *Input) { in.Literal[0].FloatBits = 3 }},
		{"float literal with exact key", func(in *Input) { in.Literal[1].Exact = 5 }},
		{"unknown literal kind", func(in *Input) { in.Literal[0].Kind = 99 }},
		{"nonstatic optional child", func(in *Input) { in.Optional[0].Inner = term(keyspace.FamilyCell, 1) }},
		{"generic without a reference base", func(in *Input) { in.Generic[0].Base = primitiveTerm(1) }},
		{"generic without arguments", func(in *Input) { in.Generic[0].Args = nil }},
		{"nonstatic generic argument", func(in *Input) {
			in.Generic[0].Args[0] = term(keyspace.FamilyCell, 1)
		}},
		{"nonstatic array element", func(in *Input) { in.Array[0].Element = term(keyspace.FamilyCell, 1) }},
		{"nonstatic map key", func(in *Input) { in.Map[0].Key = term(keyspace.FamilyCell, 1) }},
		{"nonstatic map value", func(in *Input) { in.Map[0].Value = term(keyspace.FamilyCell, 1) }},
		{"record member that is not a field", func(in *Input) { in.Record[0].Fields[0] = primitiveTerm(1) }},
		{"field without a key", func(in *Input) { in.Field[0].Key = 0 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := ledgerInput()
			test.edit(&input)
			if _, err := Build(input, ledgerCounts()); err == nil {
				t.Fatal("Build() accepted an invalid type relation")
			}
		})
	}
}

// TestTypesCopyFencesBoundsAndQueriesDoNotAllocate proves the seal takes a copy
// of every column, each read is total, and hot queries allocate nothing.
func TestTypesCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := ledgerInput()
	table := sealTable(t, input)
	input.Union[0].Members[0] = 0
	input.Generic[0].Args[0] = 0
	input.Record[0].Fields[0] = 0
	input.Primitive[0].Kind = PrimitiveNever

	types := table.View()
	union := term(keyspace.FamilyTypeUnion, 1)
	generic := term(keyspace.FamilyTypeGeneric, 1)
	record := term(keyspace.FamilyTypeRecord, 1)
	if got, ok := types.Unions().MemberAt(union, 0); !ok || got == 0 {
		t.Fatalf("union copy fence = (%v, %v)", got, ok)
	}
	if got, ok := types.Generics().ArgAt(generic, 0); !ok || got == 0 {
		t.Fatalf("generic copy fence = (%v, %v)", got, ok)
	}
	if got, ok := types.Records().FieldAt(record, 0); !ok || got == 0 {
		t.Fatalf("record copy fence = (%v, %v)", got, ok)
	}
	if kind, ok := types.Primitives().Get(primitiveTerm(1)); !ok || kind != PrimitiveNil {
		t.Fatalf("primitive copy fence = (%v, %v)", kind, ok)
	}
	for _, index := range []int{-1, 2} {
		if _, ok := types.Unions().MemberAt(union, index); ok {
			t.Fatalf("MemberAt(%d) accepted out-of-bounds index", index)
		}
	}
	if _, ok := types.Primitives().Get(primitiveTerm(9)); ok {
		t.Fatal("Primitives.Get accepted unknown term")
	}
	if _, ok := types.Primitives().Get(union); ok {
		t.Fatal("Primitives.Get accepted foreign family")
	}
	if _, ok := types.Records().At(1); ok {
		t.Fatal("Records.At accepted out-of-range index")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		types.Primitives().Get(primitiveTerm(1))
		types.Unions().MemberCount(union)
		types.Unions().MemberAt(union, 0)
		types.Generics().Get(generic)
		types.Generics().ArgAt(generic, 0)
		types.Records().Get(record)
		types.Records().FieldAt(record, 0)
	}); allocations != 0 {
		t.Fatalf("type queries allocated %.2f times", allocations)
	}
}

// TestGenericAcceptsReferencesOwnedTypeRef proves a Generic base is admitted
// only as a TypeRef, the relation the References vertical owns.
func TestGenericAcceptsReferencesOwnedTypeRef(t *testing.T) {
	if _, err := Build(ledgerInput(), ledgerCounts()); err != nil {
		t.Fatalf("Build() rejected a References-owned generic base: %v", err)
	}
	for _, base := range []keyspace.Term{
		primitiveTerm(1), term(keyspace.FamilyTypeAlias, 1), term(keyspace.FamilyTypeRef, 9), 0,
	} {
		input := ledgerInput()
		input.Generic[0].Base = base
		if _, err := Build(input, ledgerCounts()); err == nil {
			t.Fatalf("Build() accepted generic base %v", base)
		}
	}
}

// TestDecoderRetainsTypedForestRows proves the decoded rows map each wire
// field back to the relation it names.
func TestDecoderRetainsTypedForestRows(t *testing.T) {
	decoded, err := Decode(sectionReader(t, sectionBytes(t, ledgerInput())))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(decoded.Primitive) != 5 || len(decoded.Literal) != 2 || len(decoded.Optional) != 1 ||
		len(decoded.Union) != 1 || len(decoded.Intersection) != 1 || len(decoded.Generic) != 1 ||
		len(decoded.Array) != 1 || len(decoded.Map) != 1 || len(decoded.Record) != 1 || len(decoded.Field) != 2 {
		t.Fatalf("decoded relation counts = %+v", decoded)
	}
	if decoded.Primitive[1].Kind != PrimitiveNumber {
		t.Fatalf("decoded primitive = %+v", decoded.Primitive[1])
	}
	if decoded.Literal[0].Kind != keyspace.LiteralString || decoded.Literal[0].Exact != 7 ||
		decoded.Literal[1].Kind != keyspace.LiteralFloat || decoded.Literal[1].Exact != 0 {
		t.Fatalf("decoded literals = %+v", decoded.Literal)
	}
	if len(decoded.Union[0].Members) != 2 || decoded.Union[0].Members[1] != primitiveTerm(2) {
		t.Fatalf("decoded union = %+v", decoded.Union[0])
	}
	if decoded.Generic[0].Base != term(keyspace.FamilyTypeRef, 1) || len(decoded.Generic[0].Args) != 1 {
		t.Fatalf("decoded generic = %+v", decoded.Generic[0])
	}
	if !decoded.Array[0].ReadOnly || decoded.Map[0].ReadOnly {
		t.Fatalf("decoded read-only flags = %+v %+v", decoded.Array[0], decoded.Map[0])
	}
	if decoded.Field[0].Key != 9 || !decoded.Field[0].Optional {
		t.Fatalf("decoded field = %+v", decoded.Field[0])
	}
	if !decoded.Record[0].ReadOnly || len(decoded.Record[0].Fields) != 1 {
		t.Fatalf("decoded record = %+v", decoded.Record[0])
	}
}

// TestPrimitiveVocabularyAndRuntimeBoundary proves the closed primitive
// vocabulary and the exact subset a runtime type singleton can represent.
func TestPrimitiveVocabularyAndRuntimeBoundary(t *testing.T) {
	for name, want := range map[string]PrimitiveKind{
		"nil": PrimitiveNil, "boolean": PrimitiveBoolean, "number": PrimitiveNumber,
		"integer": PrimitiveInteger, "string": PrimitiveString, "function": PrimitiveFunction,
		"any": PrimitiveAny, "unknown": PrimitiveUnknown, "never": PrimitiveNever, "self": PrimitiveSelf,
	} {
		got, ok := PrimitiveKindForName(name)
		if !ok || got != want {
			t.Fatalf("PrimitiveKindForName(%q) = %v/%v, want %v", name, got, ok, want)
		}
		if !got.Valid() {
			t.Fatalf("%q produced an invalid primitive kind", name)
		}
	}
	if got, ok := PrimitiveKindForName("table"); ok || got != 0 {
		t.Fatalf("PrimitiveKindForName(table) = %v/%v, want closed refusal", got, ok)
	}
	for kind, want := range map[PrimitiveKind]bool{
		PrimitiveNil: true, PrimitiveBoolean: true, PrimitiveNumber: true, PrimitiveInteger: true,
		PrimitiveString: true, PrimitiveAny: true, PrimitiveUnknown: true, PrimitiveNever: true,
		PrimitiveFunction: false, PrimitiveSelf: false,
	} {
		if got := kind.RuntimeLoadable(); got != want {
			t.Fatalf("PrimitiveKind(%d).RuntimeLoadable() = %v, want %v", kind, got, want)
		}
	}
	if PrimitiveKind(0).Valid() || PrimitiveKind(99).Valid() {
		t.Fatal("an out-of-vocabulary primitive kind reported valid")
	}
}

// TestZeroViewFailsClosed proves an unavailable view answers nothing.
func TestZeroViewFailsClosed(t *testing.T) {
	var view View
	union := term(keyspace.FamilyTypeUnion, 1)
	if view.Available() || view.Primitives().Count() != 0 || view.Records().Count() != 0 {
		t.Fatal("zero View reported availability or rows")
	}
	if _, ok := view.Primitives().Get(primitiveTerm(1)); ok {
		t.Fatal("zero View returned a primitive")
	}
	if _, ok := view.Unions().MemberCount(union); ok {
		t.Fatal("zero View counted union members")
	}
	if _, ok := view.Unions().MemberAt(union, 0); ok {
		t.Fatal("zero View read a union member")
	}
	if _, _, ok := view.Generics().Get(term(keyspace.FamilyTypeGeneric, 1)); ok {
		t.Fatal("zero View returned a generic")
	}
}
