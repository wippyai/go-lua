package exportmanifest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/normalize"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestMergeRecordMembersUnionsConflictingFieldEvidence(t *testing.T) {
	summary := typetable.RebuildRecord(typ.RecordParts{
		Fields: []typ.Field{{
			Name:     "state",
			Type:     typ.String,
			Optional: true,
			Readonly: true,
		}},
	})
	source := typetable.RebuildRecord(typ.RecordParts{
		Fields: []typ.Field{{
			Name: "state",
			Type: typ.Number,
		}},
	})

	got, ok := mergeRecordMembers(summary, source)
	if !ok {
		t.Fatal("mergeRecordMembers failed")
	}
	record, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("mergeRecordMembers = %T, want record", got)
	}
	field, ok := fieldByName(record, "state")
	if !ok {
		t.Fatalf("merged fields = %#v, want state", record.Fields)
	}
	wantType := normalize.UnionForEvidence(typ.String, typ.Number)
	if !typ.TypeEquals(field.Type, wantType) {
		t.Fatalf("state type = %v, want %v", field.Type, wantType)
	}
	if !field.Optional {
		t.Fatalf("state optional = false, want true because one evidence source permits absence")
	}
	if field.Readonly {
		t.Fatalf("state readonly = true, want false because one evidence source is mutable")
	}
}

func TestMergeRecordMembersKeepsNarrowerSourceFieldEvidence(t *testing.T) {
	summary := typetable.RebuildRecord(typ.RecordParts{
		Fields: []typ.Field{{Name: "value", Type: typ.Number}},
	})
	source := typetable.RebuildRecord(typ.RecordParts{
		Fields: []typ.Field{{Name: "value", Type: typ.LiteralInt(1)}},
	})

	got, ok := mergeRecordMembers(summary, source)
	if !ok {
		t.Fatal("mergeRecordMembers failed")
	}
	record, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("mergeRecordMembers = %T, want record", got)
	}
	field, ok := fieldByName(record, "value")
	if !ok {
		t.Fatalf("merged fields = %#v, want value", record.Fields)
	}
	if !typ.TypeEquals(field.Type, typ.LiteralInt(1)) {
		t.Fatalf("value type = %v, want literal 1", field.Type)
	}
}

func TestMergeRecordMembersUnionsConflictingStaticMemberEvidence(t *testing.T) {
	summary := typetable.RebuildRecord(typ.RecordParts{
		StaticMembers: []typ.StaticMember{{
			Kind:     typ.StaticMemberStringIndex,
			Name:     "state",
			Type:     typ.String,
			Optional: true,
			Readonly: true,
		}},
	})
	source := typetable.RebuildRecord(typ.RecordParts{
		StaticMembers: []typ.StaticMember{{
			Kind: typ.StaticMemberStringIndex,
			Name: "state",
			Type: typ.Number,
		}},
	})

	got, ok := mergeRecordMembers(summary, source)
	if !ok {
		t.Fatal("mergeRecordMembers failed")
	}
	record, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("mergeRecordMembers = %T, want record", got)
	}
	member, ok := staticMemberByStringKey(record, "state")
	if !ok {
		t.Fatalf("merged static members = %#v, want state", record.StaticMembers)
	}
	wantType := normalize.UnionForEvidence(typ.String, typ.Number)
	if !typ.TypeEquals(member.Type, wantType) {
		t.Fatalf("state static type = %v, want %v", member.Type, wantType)
	}
	if !member.Optional {
		t.Fatalf("state static optional = false, want true because one evidence source permits absence")
	}
	if member.Readonly {
		t.Fatalf("state static readonly = true, want false because one evidence source is mutable")
	}
}

func fieldByName(record *typ.Record, name string) (typ.Field, bool) {
	for _, field := range record.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return typ.Field{}, false
}

func staticMemberByStringKey(record *typ.Record, name string) (typ.StaticMember, bool) {
	for _, member := range record.StaticMembers {
		if member.Kind == typ.StaticMemberStringIndex && member.Name == name {
			return member, true
		}
	}
	return typ.StaticMember{}, false
}
