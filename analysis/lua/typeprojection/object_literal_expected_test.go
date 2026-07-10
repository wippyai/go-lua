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

func TestReachesRecordAcceptsInstantiatedRecord(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param},
		typetable.NewRecord().Field("value", param).Build())

	if !ReachesRecord(typ.Instantiate(box, typ.String)) {
		t.Fatal("ReachesRecord(instantiated record) = false, want true")
	}
}

func TestReachesTableContractAcceptsOptionalRecordAndUnion(t *testing.T) {
	rec := typetable.NewRecord().Field("id", typ.String).Build()
	if !ReachesTableContract(typeexpr.Optional(rec)) {
		t.Fatal("ReachesTableContract(optional record) = false, want true")
	}
	if !ReachesTableContract(typeexpr.Union(typ.String, rec)) {
		t.Fatal("ReachesTableContract(union with record) = false, want true")
	}
	if ReachesTableContract(typeexpr.Union(typ.String, typ.Number)) {
		t.Fatal("ReachesTableContract(scalar union) = true, want false")
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

func TestExpectedRecordSegmentDistinguishesDotFieldAndStaticStringMember(t *testing.T) {
	rec := typetable.NewRecord().
		Field("kind", typ.LiteralString("dot")).
		StaticStringIndex("kind", typ.LiteralString("index")).
		Build()

	got, ok := ExpectedRecordSegment(rec, segment.Segment{Kind: segment.SegmentField, Name: "kind"})
	if !ok || !typ.TypeEquals(got, typ.LiteralString("dot")) {
		t.Fatalf("dot segment = %v/%v, want dot literal", got, ok)
	}
	got, ok = ExpectedRecordSegment(rec, segment.Segment{Kind: segment.SegmentIndexString, Name: "kind"})
	if !ok || !typ.TypeEquals(got, typ.LiteralString("index")) {
		t.Fatalf("string-index segment = %v/%v, want index literal", got, ok)
	}
}

func TestExpectedTypeAtSegmentsTraversesOptionalStaticAndUnionMembers(t *testing.T) {
	inner := typetable.NewRecord().
		OptField("name", typ.String).
		StaticIntIndex(2, typ.Integer).
		Build()
	left := typetable.NewRecord().
		Field("payload", inner).
		Build()
	right := typetable.NewRecord().
		Field("payload", inner).
		Build()

	got, ok := ExpectedTypeAtSegments(typeexpr.Union(left, right), []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
		{Kind: segment.SegmentField, Name: "name"},
	})
	if !ok || !typ.TypeEquals(got, typeexpr.Optional(typ.String)) {
		t.Fatalf("nested optional field = %v/%v, want string?", got, ok)
	}
	got, ok = ExpectedTypeAtSegments(left, []segment.Segment{
		{Kind: segment.SegmentField, Name: "payload"},
		{Kind: segment.SegmentIndexInt, Index: 2},
	})
	if !ok || !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("nested static int = %v/%v, want integer", got, ok)
	}
}

func TestMissingRequiredRecordFieldUsesContextualRecordContract(t *testing.T) {
	rec := typetable.NewRecord().
		Field("id", typ.String).
		OptField("nickname", typ.String).
		Build()

	got, ok := MissingRequiredRecordField(rec, func(name string) bool {
		return name == "nickname"
	})
	if !ok || got != "id" {
		t.Fatalf("missing field = %q/%v, want id", got, ok)
	}
	got, ok = MissingRequiredRecordField(rec, func(name string) bool {
		return name == "id"
	})
	if ok || got != "" {
		t.Fatalf("missing field with id present = %q/%v, want none", got, ok)
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

func TestAdoptExpectedSegmentTypePreservesStaticMemberKind(t *testing.T) {
	rec := typetable.NewRecord().
		Field("kind", typ.LiteralString("dot")).
		StaticStringIndex("kind", typ.LiteralString("index")).
		Build()

	got, ok := AdoptExpectedSegmentType(rec, segment.Segment{Kind: segment.SegmentIndexString, Name: "kind"}, typ.LiteralString("index"))
	if !ok || !typ.TypeEquals(got, typ.LiteralString("index")) {
		t.Fatalf("string-index adoption = %v/%v, want index literal", got, ok)
	}
	if got, ok := AdoptExpectedSegmentType(rec, segment.Segment{Kind: segment.SegmentIndexString, Name: "kind"}, typ.LiteralString("dot")); ok || got != nil {
		t.Fatalf("string-index adopted dot field = %v/%v, want rejection", got, ok)
	}
}
