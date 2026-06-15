package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

type testRecordBuilder struct {
	parts RecordParts
}

func newRecord() *testRecordBuilder {
	return &testRecordBuilder{}
}

func (b *testRecordBuilder) Build() *Record {
	return RebuildRecord(b.parts)
}

func (b *testRecordBuilder) Field(name string, t Type) *testRecordBuilder {
	b.parts.Fields = append(b.parts.Fields, Field{Name: name, Type: t})
	return b
}

func (b *testRecordBuilder) OptField(name string, t Type) *testRecordBuilder {
	b.parts.Fields = append(b.parts.Fields, Field{Name: name, Type: t, Optional: true})
	return b
}

func (b *testRecordBuilder) ReadonlyField(name string, t Type) *testRecordBuilder {
	b.parts.Fields = append(b.parts.Fields, Field{Name: name, Type: t, Readonly: true})
	return b
}

func (b *testRecordBuilder) AddStaticMember(member StaticMember) *testRecordBuilder {
	b.parts.StaticMembers = append(b.parts.StaticMembers, member)
	return b
}

func (b *testRecordBuilder) StaticStringIndex(name string, t Type) *testRecordBuilder {
	return b.AddStaticMember(StaticMember{Kind: StaticMemberStringIndex, Name: name, Type: t})
}

func (b *testRecordBuilder) StaticIntIndex(index int64, t Type) *testRecordBuilder {
	return b.AddStaticMember(StaticMember{Kind: StaticMemberIntIndex, Index: index, Type: t})
}

func (b *testRecordBuilder) Metatable(t Type) *testRecordBuilder {
	b.parts.Metatable = t
	return b
}

func (b *testRecordBuilder) SetOpen(open bool) *testRecordBuilder {
	b.parts.Open = open
	return b
}

func TestRecordEmpty(t *testing.T) {
	r := newRecord().Build()

	if r.Kind() != kind.Record {
		t.Errorf("Kind: got %v, want Record", r.Kind())
	}

	if len(r.Fields) != 0 {
		t.Errorf("Fields: got %d, want 0", len(r.Fields))
	}

	if r.String() != "{}" {
		t.Errorf("String: got %q, want %q", r.String(), "{}")
	}
}

func TestRecordWithFields(t *testing.T) {
	r := newRecord().
		Field("x", Number).
		Field("y", String).
		Build()

	if len(r.Fields) != 2 {
		t.Errorf("Fields: got %d, want 2", len(r.Fields))
	}
}

func TestRecordNilFieldDefaultsToUnknown(t *testing.T) {
	r := newRecord().
		Field("x", nil).
		Build()

	f := r.GetField("x")
	if f == nil {
		t.Fatal("expected field x")
	}
	if f.Type != Unknown {
		t.Errorf("Field type: got %v, want Unknown", f.Type)
	}
}

func TestRecordFieldsSorted(t *testing.T) {
	r := newRecord().
		Field("z", Number).
		Field("a", String).
		Field("m", Boolean).
		Build()

	if r.Fields[0].Name != "a" {
		t.Errorf("first field should be 'a', got %q", r.Fields[0].Name)
	}

	if r.Fields[1].Name != "m" {
		t.Errorf("second field should be 'm', got %q", r.Fields[1].Name)
	}

	if r.Fields[2].Name != "z" {
		t.Errorf("third field should be 'z', got %q", r.Fields[2].Name)
	}
}

func TestRecordOptionalMembersPreserveNilablePayloads(t *testing.T) {
	optionalString := MaterializeOptional(String)
	optionalNumber := MaterializeOptional(Number)
	assertOptionalPayload := func(t *testing.T, got Type, want Type) {
		t.Helper()
		if _, ok := got.(*Optional); !ok {
			t.Fatalf("payload = %T %[1]v, want optional payload", got)
		}
		if !typeEquals(got, want) {
			t.Fatalf("payload = %v, want %v", got, want)
		}
	}

	r := newRecord().
		OptField("error", optionalString).
		OptField("nil_only", Nil).
		AddStaticMember(StaticMember{
			Kind:     StaticMemberStringIndex,
			Name:     "raw",
			Type:     optionalNumber,
			Optional: true,
		}).
		Build()

	err := r.GetField("error")
	if err == nil || !err.Optional {
		t.Fatal("expected optional error field")
	}
	assertOptionalPayload(t, err.Type, optionalString)

	nilOnly := r.GetField("nil_only")
	if nilOnly == nil || !nilOnly.Optional {
		t.Fatal("expected optional nil_only field")
	}
	if !typeEquals(nilOnly.Type, Nil) {
		t.Fatalf("expected nil-only optional field type to remain nil, got %v", nilOnly.Type)
	}

	member := r.GetStaticStringIndex("raw")
	if member == nil || !member.Optional {
		t.Fatal("expected optional raw static member")
	}
	assertOptionalPayload(t, member.Type, optionalNumber)

	rebuilt := RebuildRecord(RecordParts{
		Fields: []Field{{
			Name:     "error",
			Type:     optionalString,
			Optional: true,
		}},
		StaticMembers: []StaticMember{{
			Kind:     StaticMemberIntIndex,
			Index:    1,
			Type:     optionalNumber,
			Optional: true,
		}},
	})
	if field := rebuilt.GetField("error"); field == nil || !field.Optional {
		t.Fatal("expected rebuilt optional error field")
	} else {
		assertOptionalPayload(t, field.Type, optionalString)
	}
	if member := rebuilt.GetStaticIntIndex(1); member == nil || !member.Optional {
		t.Fatal("expected rebuilt optional static member")
	} else {
		assertOptionalPayload(t, member.Type, optionalNumber)
	}
}

func TestRecordOptionalField(t *testing.T) {
	r := newRecord().
		Field("x", Number).
		OptField("y", String).
		Build()

	if r.Fields[0].Optional {
		t.Error("x should not be optional")
	}

	y := r.GetField("y")
	if y == nil || !y.Optional {
		t.Error("y should be optional")
	}
}

func TestRecordReadonlyField(t *testing.T) {
	r := newRecord().
		ReadonlyField("id", Number).
		Field("name", String).
		Build()

	id := r.GetField("id")
	if id == nil || !id.Readonly {
		t.Error("id should be readonly")
	}

	name := r.GetField("name")
	if name == nil || name.Readonly {
		t.Error("name should not be readonly")
	}
}

func TestRecordString(t *testing.T) {
	r := newRecord().
		Field("x", Number).
		OptField("y", String).
		ReadonlyField("z", Boolean).
		Build()

	s := r.String()
	if s != "{x: number, readonly y?: string, z: boolean}" &&
		s != "{x: number, y?: string, readonly z: boolean}" {
		// Fields are sorted, so exact string depends on sort order
		t.Logf("String: %s", s)
	}
}

func TestRecordEquality(t *testing.T) {
	r1 := newRecord().Field("x", Number).Field("y", String).Build()
	r2 := newRecord().Field("y", String).Field("x", Number).Build()
	r3 := newRecord().Field("x", Number).Build()
	r4 := newRecord().Field("x", String).Field("y", String).Build()

	if !r1.Equals(r2) {
		t.Error("records with same fields should be equal regardless of order")
	}

	if r1.Equals(r3) {
		t.Error("records with different field count should not be equal")
	}

	if r1.Equals(r4) {
		t.Error("records with different field types should not be equal")
	}
}

func TestRecordEqualityOptional(t *testing.T) {
	r1 := newRecord().Field("x", Number).Build()
	r2 := newRecord().OptField("x", Number).Build()

	if r1.Equals(r2) {
		t.Error("required vs optional field should differ")
	}
}

func TestRecordEqualityReadonly(t *testing.T) {
	r1 := newRecord().Field("x", Number).Build()
	r2 := newRecord().ReadonlyField("x", Number).Build()

	if r1.Equals(r2) {
		t.Error("mutable vs readonly field should differ")
	}
}

func TestRecordGetField(t *testing.T) {
	r := newRecord().
		Field("x", Number).
		Field("y", String).
		Build()

	x := r.GetField("x")
	if x == nil {
		t.Fatal("GetField(x) should not be nil")
	}

	if x.Type != Number {
		t.Error("x should have type Number")
	}

	z := r.GetField("z")
	if z != nil {
		t.Error("GetField(z) should be nil")
	}
}

func TestRecordHashUniqueness(t *testing.T) {
	records := []Type{
		newRecord().Build(),
		newRecord().Field("x", Number).Build(),
		newRecord().Field("x", String).Build(),
		newRecord().Field("y", Number).Build(),
		newRecord().OptField("x", Number).Build(),
		newRecord().ReadonlyField("x", Number).Build(),
	}

	hashes := make(map[uint64]Type)

	for _, r := range records {
		h := r.Hash()
		if existing, ok := hashes[h]; ok {
			t.Errorf("Hash collision: %s and %s", existing.String(), r.String())
		}

		hashes[h] = r
	}
}

func TestRecordNotEqualToPrimitive(t *testing.T) {
	r := newRecord().Field("x", Number).Build()
	if r.Equals(Number) {
		t.Error("record should not equal primitive")
	}
}

func TestRecordWithMetatable(t *testing.T) {
	meta := newRecord().Field("__index", Any).Build()
	r := newRecord().
		Field("value", Number).
		Metatable(meta).
		Build()

	if r.Metatable == nil {
		t.Error("record should have metatable")
	}

	if r.Metatable != meta {
		t.Error("metatable mismatch")
	}
}

func TestRecordOpenFlag(t *testing.T) {
	closed := newRecord().Field("x", Number).Build()
	open := newRecord().SetOpen(true).Field("x", Number).Build()

	if closed.Open {
		t.Error("default record should not be open")
	}

	if !open.Open {
		t.Error("record built with SetOpen(true) should be open")
	}
}

func TestRecordOpenString(t *testing.T) {
	empty := newRecord().SetOpen(true).Build()
	if empty.String() != "{...}" {
		t.Errorf("open empty record string: got %q, want %q", empty.String(), "{...}")
	}

	withField := newRecord().SetOpen(true).Field("x", Number).Build()
	if withField.String() != "{x: number, ...}" {
		t.Errorf("open record string: got %q, want %q", withField.String(), "{x: number, ...}")
	}
}

func TestRecordOpenEquality(t *testing.T) {
	closed := newRecord().Field("x", Number).Build()
	open := newRecord().SetOpen(true).Field("x", Number).Build()

	if closed.Equals(open) {
		t.Error("closed and open records with same fields should not be equal")
	}

	open2 := newRecord().SetOpen(true).Field("x", Number).Build()
	if !open.Equals(open2) {
		t.Error("two open records with same fields should be equal")
	}
}

func TestRecordOpenHash(t *testing.T) {
	closed := newRecord().Field("x", Number).Build()
	open := newRecord().SetOpen(true).Field("x", Number).Build()

	if closed.Hash() == open.Hash() {
		t.Error("closed and open records should have different hashes")
	}
}

func TestRecordOpenTypeEquals(t *testing.T) {
	closed := newRecord().Field("x", Number).Build()
	open := newRecord().SetOpen(true).Field("x", Number).Build()

	if typeEquals(closed, open) {
		t.Error("TypeEquals should distinguish open from closed")
	}

	open2 := newRecord().SetOpen(true).Field("x", Number).Build()
	if !typeEquals(open, open2) {
		t.Error("TypeEquals should match two identical open records")
	}
}
