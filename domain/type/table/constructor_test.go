package table

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
)

func TestConstructorBuilderBuildsNestedRecord(t *testing.T) {
	got, ok := ConstructorType([]ConstructorEntry{
		{
			Path: []ConstructorKey{
				{Kind: ConstructorField, Name: "traits"},
				{Kind: ConstructorField, Name: "payload"},
			},
			Type: typ.String,
		},
		{
			Path: []ConstructorKey{
				{Kind: ConstructorField, Name: "traits"},
				{Kind: ConstructorField, Name: "count"},
			},
			Type: typ.Integer,
		},
	})
	if !ok {
		t.Fatal("ConstructorType returned false")
	}

	want := NewRecord().
		Field("traits", NewRecord().
			Field("count", typ.Integer).
			Field("payload", typ.String).
			Build()).
		Build()
	assertConstructorType(t, got, want)
}

func TestConstructorBuilderBuildsPureIntegerRootAsTuple(t *testing.T) {
	got, ok := ConstructorType([]ConstructorEntry{
		{Path: []ConstructorKey{{Kind: ConstructorIntIndex, Index: 3}}, Type: typ.String},
		{Path: []ConstructorKey{{Kind: ConstructorIntIndex, Index: 1}}, Type: typ.Number},
	})
	if !ok {
		t.Fatal("ConstructorType returned false")
	}
	assertConstructorType(t, got, typ.NewTuple(typ.Number, typ.String))
}

func TestConstructorBuilderBuildsRecordStaticMembers(t *testing.T) {
	got, ok := ConstructorType([]ConstructorEntry{
		{Path: []ConstructorKey{{Kind: ConstructorField, Name: "name"}}, Type: typ.String},
		{Path: []ConstructorKey{{Kind: ConstructorStringIndex, Name: "raw"}}, Type: typ.Boolean},
		{Path: []ConstructorKey{{Kind: ConstructorIntIndex, Index: 7}}, Type: typ.Integer},
	})
	if !ok {
		t.Fatal("ConstructorType returned false")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("type = %T, want record", got)
	}
	if field := rec.GetField("name"); field == nil || !typ.TypeEquals(field.Type, typ.String) {
		t.Fatalf("field name = %#v, want string", field)
	}
	if member := rec.GetStaticStringIndex("raw"); member == nil || !typ.TypeEquals(member.Type, typ.Boolean) {
		t.Fatalf("static raw = %#v, want boolean", member)
	}
	if member := rec.GetStaticIntIndex(7); member == nil || !typ.TypeEquals(member.Type, typ.Integer) {
		t.Fatalf("static 7 = %#v, want integer", member)
	}
}

func TestConstructorBuilderSealedNodeDropsChildren(t *testing.T) {
	tests := []struct {
		name    string
		entries []ConstructorEntry
	}{
		{
			name: "child then sealed parent",
			entries: []ConstructorEntry{
				{
					Path: []ConstructorKey{
						{Kind: ConstructorField, Name: "payload"},
						{Kind: ConstructorField, Name: "id"},
					},
					Type: typ.String,
				},
				{
					Path:   []ConstructorKey{{Kind: ConstructorField, Name: "payload"}},
					Type:   typ.Number,
					Sealed: true,
				},
			},
		},
		{
			name: "sealed parent then child",
			entries: []ConstructorEntry{
				{
					Path:   []ConstructorKey{{Kind: ConstructorField, Name: "payload"}},
					Type:   typ.Number,
					Sealed: true,
				},
				{
					Path: []ConstructorKey{
						{Kind: ConstructorField, Name: "payload"},
						{Kind: ConstructorField, Name: "id"},
					},
					Type: typ.String,
				},
			},
		},
		{
			name: "sealed parent ignores later unsealed same path",
			entries: []ConstructorEntry{
				{
					Path:   []ConstructorKey{{Kind: ConstructorField, Name: "payload"}},
					Type:   typ.Number,
					Sealed: true,
				},
				{
					Path: []ConstructorKey{{Kind: ConstructorField, Name: "payload"}},
					Type: typ.String,
				},
			},
		},
	}
	want := NewRecord().Field("payload", typ.Number).Build()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ConstructorType(tt.entries)
			if !ok {
				t.Fatal("ConstructorType returned false")
			}
			assertConstructorType(t, got, want)
		})
	}
}

func TestConstructorBuilderRejectsInvalidEntries(t *testing.T) {
	tests := []ConstructorEntry{
		{Path: nil, Type: typ.String},
		{Path: []ConstructorKey{{Kind: ConstructorField}}, Type: typ.String},
		{Path: []ConstructorKey{{Kind: ConstructorStringIndex}}, Type: typ.String},
		{Path: []ConstructorKey{{Kind: ConstructorIntIndex, Index: -1}}, Type: typ.String},
		{Path: []ConstructorKey{{Kind: ConstructorKeyKind(99)}}, Type: typ.String},
		{Path: []ConstructorKey{{Kind: ConstructorField, Name: "x"}}},
	}
	for _, entry := range tests {
		if got, ok := ConstructorType([]ConstructorEntry{entry}); ok || got != nil {
			t.Fatalf("ConstructorType(%#v) = %v/%v, want failure", entry, got, ok)
		}
	}
}

func assertConstructorType(t *testing.T, got, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}
