package discriminant

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	. "github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRecordsConflictOnRequiredLiteral(t *testing.T) {
	a := typetable.NewRecord().
		Field("kind", LiteralString("a")).
		Field("x", Number).
		Build()
	b := typetable.NewRecord().
		Field("kind", LiteralString("b")).
		Field("y", String).
		Build()

	d := NewDetector()
	if !d.RecordsConflict(a, b) {
		t.Fatal("records with one differing required literal tag did not conflict")
	}
	differing, equal := d.sharedRequiredLiteralAxes(a, b)
	if differing != 1 || equal != 0 {
		t.Fatalf("shared literal axes = differing %d equal %d, want 1/0", differing, equal)
	}
}

func TestRecordsDoNotConflictOnMultipleDifferingLiteralsWithCleanResidual(t *testing.T) {
	a := typetable.NewRecord().
		Field("status_code", LiteralInt(401)).
		Field("message", LiteralString("invalid key")).
		Field("ok", Boolean).
		Build()
	b := typetable.NewRecord().
		Field("status_code", LiteralInt(400)).
		Field("message", LiteralString("invalid model")).
		Field("ok", Boolean).
		Build()

	d := NewDetector()
	if d.RecordsConflict(a, b) {
		t.Fatal("multiple differing literal data fields reported a discriminant conflict")
	}
	if !d.literalErasedResidualsCleanlyMergeable(a, b) {
		t.Fatal("literal-erased residuals with the same non-literal payload should merge cleanly")
	}
	differing, equal := d.sharedRequiredLiteralAxes(a, b)
	if differing != 2 || equal != 0 {
		t.Fatalf("shared literal axes = differing %d equal %d, want 2/0", differing, equal)
	}
}

func TestRecordsConflictOnNestedRequiredLiteral(t *testing.T) {
	leftPayload := typetable.NewRecord().
		Field("__tag", LiteralString("left")).
		Field("value", Number).
		Build()
	rightPayload := typetable.NewRecord().
		Field("__tag", LiteralString("right")).
		Field("value", Number).
		Build()
	a := typetable.NewRecord().
		Field("kind", LiteralString("event")).
		Field("payload_kind", LiteralString("left")).
		Field("payload", leftPayload).
		Build()
	b := typetable.NewRecord().
		Field("kind", LiteralString("event")).
		Field("payload_kind", LiteralString("right")).
		Field("payload", rightPayload).
		Build()

	d := NewDetector()
	if !d.RecordsConflict(a, b) {
		t.Fatal("nested required literal tag did not preserve the record split")
	}
	if d.literalErasedResidualsCleanlyMergeable(a, b) {
		t.Fatal("literal-erased residuals should not merge through a nested discriminant")
	}
}

func TestRequiredTagsPreservesNestedNonRecursiveTag(t *testing.T) {
	chanInt := NewAlias("__test_ChanInt", typetable.NewRecord().
		Field("__tag", LiteralString("int")).
		Build())
	errCase := typetable.NewRecord().
		Field("channel", chanInt).
		Field("value", typetable.NewRecord().Field("error", String).Build()).
		Build()

	d := NewDetector()
	tags := d.RequiredTags(errCase)
	if tags["channel.__tag"] != EqualityHash(LiteralString("int")) {
		t.Fatalf("nested channel tag was not summarized: %v", tags)
	}
}

func TestRequiredTagsRecursiveCycleSummarizesFiniteTags(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return typetable.NewRecord().
			Field("kind", LiteralString("node")).
			Field("next", self).
			Build()
	})

	d := NewDetector()
	tags := d.RequiredTags(node)
	if tags["kind"] != EqualityHash(LiteralString("node")) {
		t.Fatalf("recursive top-level tag was not summarized: %v", tags)
	}
	if _, ok := tags["next.kind"]; ok {
		t.Fatalf("recursive discriminant summary unfolded through self: %v", tags)
	}
}

func TestClosedRecordSetConflict(t *testing.T) {
	conflicting := []*Record{
		typetable.NewRecord().
			Field("kind", LiteralString("a")).
			Field("x", Number).
			Build(),
		typetable.NewRecord().
			Field("kind", LiteralString("b")).
			Field("y", String).
			Build(),
	}
	d := NewDetector()
	if !d.ClosedRecordSetConflicts(conflicting) {
		t.Fatal("closed record set with required literal variants did not conflict")
	}

	clean := []*Record{
		typetable.NewRecord().
			Field("from", Func().Param("self", Self).Returns(Self).Build()).
			Field("where", Func().Param("self", Self).Param("clause", String).Returns(Self).Build()).
			Build(),
		typetable.NewRecord().
			Field("from", Func().Param("self", Self).Returns(Self).Build()).
			Field("where", Func().Param("self", Self).Param("clause", String).Returns(Self).Build()).
			Field("limit", Number).
			Build(),
	}
	if d.ClosedRecordSetConflicts(clean) {
		t.Fatal("records without required literal discriminants reported a conflict")
	}
}

func TestNarrowByPathLiteralKeepsMatchingVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", LiteralString("dog")).
		Field("bark", Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", LiteralString("cat")).
		Field("meow", Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteral(NewUnion(dog, cat), []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, LiteralString("dog"))
	if !ok {
		t.Fatal("expected strict discriminant narrowing")
	}
	if !TypeEquals(got, dog) {
		t.Fatalf("narrowed type = %s, want dog variant %s", got, dog)
	}
}

func TestNarrowByPathLiteralReturnsNeverForImpossibleSingleVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", LiteralString("dog")).
		Field("bark", Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteral(dog, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, LiteralString("cat"))
	if !ok || got != Never {
		t.Fatalf("narrowed type = %s/%v, want never/true", got, ok)
	}
}

func TestNarrowByPathLiteralNotKeepsNonMatchingVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", LiteralString("dog")).
		Field("bark", Func().Returns().Build()).
		Build()
	cat := typetable.NewRecord().
		Field("kind", LiteralString("cat")).
		Field("meow", Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteralNot(NewUnion(dog, cat), []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, LiteralString("dog"))
	if !ok {
		t.Fatal("expected strict negative discriminant narrowing")
	}
	if !TypeEquals(got, cat) {
		t.Fatalf("narrowed type = %s, want cat variant %s", got, cat)
	}
}

func TestNarrowByPathLiteralNotReturnsNeverForMatchingSingleVariant(t *testing.T) {
	dog := typetable.NewRecord().
		Field("kind", LiteralString("dog")).
		Field("bark", Func().Returns().Build()).
		Build()

	got, ok := NarrowByPathLiteralNot(dog, []segment.Segment{
		{Kind: segment.SegmentField, Name: "kind"},
	}, LiteralString("dog"))
	if !ok || got != Never {
		t.Fatalf("narrowed type = %s/%v, want never/true", got, ok)
	}
}
