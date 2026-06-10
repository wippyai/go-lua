package discriminant

import (
	"testing"

	. "github.com/wippyai/go-lua/analysis/type/typ"
)

func TestRecordsConflictOnRequiredLiteral(t *testing.T) {
	a := NewRecord().
		Field("kind", LiteralString("a")).
		Field("x", Number).
		Build()
	b := NewRecord().
		Field("kind", LiteralString("b")).
		Field("y", String).
		Build()

	d := NewDetector()
	if !d.RecordsConflict(a, b) {
		t.Fatal("records with one differing required literal tag did not conflict")
	}
	differing, equal := d.SharedRequiredLiteralAxes(a, b)
	if differing != 1 || equal != 0 {
		t.Fatalf("shared literal axes = differing %d equal %d, want 1/0", differing, equal)
	}
}

func TestRecordsDoNotConflictOnMultipleDifferingLiteralsWithCleanResidual(t *testing.T) {
	a := NewRecord().
		Field("status_code", LiteralInt(401)).
		Field("message", LiteralString("invalid key")).
		Field("ok", Boolean).
		Build()
	b := NewRecord().
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
	differing, equal := d.SharedRequiredLiteralAxes(a, b)
	if differing != 2 || equal != 0 {
		t.Fatalf("shared literal axes = differing %d equal %d, want 2/0", differing, equal)
	}
}

func TestRecordsConflictOnNestedRequiredLiteral(t *testing.T) {
	leftPayload := NewRecord().
		Field("__tag", LiteralString("left")).
		Field("value", Number).
		Build()
	rightPayload := NewRecord().
		Field("__tag", LiteralString("right")).
		Field("value", Number).
		Build()
	a := NewRecord().
		Field("kind", LiteralString("event")).
		Field("payload_kind", LiteralString("left")).
		Field("payload", leftPayload).
		Build()
	b := NewRecord().
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
	chanInt := NewAlias("__test_ChanInt", NewRecord().
		Field("__tag", LiteralString("int")).
		Build())
	errCase := NewRecord().
		Field("channel", chanInt).
		Field("value", NewRecord().Field("error", String).Build()).
		Build()

	tags := RequiredTags(errCase)
	if tags["channel.__tag"] != LiteralString("int").Hash() {
		t.Fatalf("nested channel tag was not summarized: %v", tags)
	}
}

func TestRequiredTagsRecursiveCycleSummarizesFiniteTags(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("kind", LiteralString("node")).
			Field("next", self).
			Build()
	})

	tags := RequiredTags(node)
	if tags["kind"] != LiteralString("node").Hash() {
		t.Fatalf("recursive top-level tag was not summarized: %v", tags)
	}
	if _, ok := tags["next.kind"]; ok {
		t.Fatalf("recursive discriminant summary unfolded through self: %v", tags)
	}
}

func TestClosedRecordSetConflict(t *testing.T) {
	conflicting := []*Record{
		NewRecord().
			Field("kind", LiteralString("a")).
			Field("x", Number).
			Build(),
		NewRecord().
			Field("kind", LiteralString("b")).
			Field("y", String).
			Build(),
	}
	if !ClosedRecordSetConflicts(conflicting) {
		t.Fatal("closed record set with required literal variants did not conflict")
	}

	clean := []*Record{
		NewRecord().
			Field("from", Func().Param("self", Self).Returns(Self).Build()).
			Field("where", Func().Param("self", Self).Param("clause", String).Returns(Self).Build()).
			Build(),
		NewRecord().
			Field("from", Func().Param("self", Self).Returns(Self).Build()).
			Field("where", Func().Param("self", Self).Param("clause", String).Returns(Self).Build()).
			Field("limit", Number).
			Build(),
	}
	if ClosedRecordSetConflicts(clean) {
		t.Fatal("records without required literal discriminants reported a conflict")
	}
}
