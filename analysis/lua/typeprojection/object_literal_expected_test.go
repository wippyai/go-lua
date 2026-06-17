package typeprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestExpectedObjectLiteralRecordSelectsUniqueDiscriminatedUnionArm(t *testing.T) {
	start := typetable.NewRecord().
		Field("kind", typ.LiteralString("start")).
		Field("payload", typ.String).
		Build()
	stop := typetable.NewRecord().
		Field("kind", typ.LiteralString("stop")).
		Field("code", typ.Integer).
		Build()

	got, ok := ExpectedObjectLiteralRecord(typeexpr.Union(start, stop), func(name string) (typ.Type, bool) {
		if name == "kind" {
			return typ.LiteralString("stop"), true
		}
		return nil, false
	})
	if !ok || got != stop {
		t.Fatalf("selected arm = %v/%v, want stop arm", got, ok)
	}
}

func TestExpectedObjectLiteralRecordRejectsAmbiguousDiscriminatedUnionArm(t *testing.T) {
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("left", typ.String).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("same")).
		Field("right", typ.String).
		Build()

	if got, ok := ExpectedObjectLiteralRecord(typeexpr.Union(left, right), func(name string) (typ.Type, bool) {
		if name == "kind" {
			return typ.LiteralString("same"), true
		}
		return nil, false
	}); ok || got != nil {
		t.Fatalf("ambiguous selected arm = %v/%v, want rejection", got, ok)
	}
}

func TestExpectedObjectLiteralRecordRejectsNondiscriminantUnionArm(t *testing.T) {
	first := typetable.NewRecord().
		Field("kind", typ.String).
		Field("left", typ.String).
		Build()
	second := typetable.NewRecord().
		Field("kind", typ.String).
		Field("right", typ.String).
		Build()

	if got, ok := ExpectedObjectLiteralRecord(typeexpr.Union(first, second), func(name string) (typ.Type, bool) {
		if name == "kind" {
			return typ.LiteralString("start"), true
		}
		return nil, false
	}); ok || got != nil {
		t.Fatalf("nondiscriminant selected arm = %v/%v, want rejection", got, ok)
	}
}

func TestExpectedRecordFieldDistinguishesDotFieldAndStaticStringMember(t *testing.T) {
	rec := typetable.NewRecord().
		Field("name", typ.String).
		StaticStringIndex("name", typ.Boolean).
		Build()

	got, ok := ExpectedRecordField(rec, []segment.Segment{{Kind: segment.SegmentField, Name: "name"}})
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("dot field = %v/%v, want string", got, ok)
	}
	got, ok = ExpectedRecordField(rec, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "name"}})
	if !ok || !typ.TypeEquals(got, typ.Boolean) {
		t.Fatalf("static string member = %v/%v, want boolean", got, ok)
	}
	if got, ok := ExpectedRecordField(rec, []segment.Segment{{Kind: segment.SegmentIndexInt, Index: 1}}); ok || got != nil {
		t.Fatalf("integer member = %v/%v, want rejection", got, ok)
	}
}

func TestAdoptExpectedFieldTypeUsesOnlyAdmissiblePreciseFacts(t *testing.T) {
	rec := typetable.NewRecord().Field("count", typ.Integer).Build()
	path := []segment.Segment{{Kind: segment.SegmentField, Name: "count"}}

	got, ok := AdoptExpectedFieldType(rec, path, typ.LiteralInt(1))
	if !ok || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("adopted type = %v/%v, want integer", got, ok)
	}
	if got, ok := AdoptExpectedFieldType(rec, path, typ.String); ok || got != nil {
		t.Fatalf("mismatch adopted type = %v/%v, want rejection", got, ok)
	}
	if got, ok := AdoptExpectedFieldType(rec, path, typ.Any); ok || got != nil {
		t.Fatalf("any adopted type = %v/%v, want rejection", got, ok)
	}
}
