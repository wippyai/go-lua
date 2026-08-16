package discriminant

import (
	"testing"

	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

func TestRecordsConflictOnRequiredLiteral(t *testing.T) {
	a := typetable.NewRecord().
		Field("kind", typ.LiteralString("a")).
		Field("x", typ.Number).
		Build()
	b := typetable.NewRecord().
		Field("kind", typ.LiteralString("b")).
		Field("y", typ.String).
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
		Field("status_code", typ.LiteralInt(401)).
		Field("message", typ.LiteralString("invalid key")).
		Field("ok", typ.Boolean).
		Build()
	b := typetable.NewRecord().
		Field("status_code", typ.LiteralInt(400)).
		Field("message", typ.LiteralString("invalid model")).
		Field("ok", typ.Boolean).
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
		Field("__tag", typ.LiteralString("left")).
		Field("value", typ.Number).
		Build()
	rightPayload := typetable.NewRecord().
		Field("__tag", typ.LiteralString("right")).
		Field("value", typ.Number).
		Build()
	a := typetable.NewRecord().
		Field("kind", typ.LiteralString("event")).
		Field("payload_kind", typ.LiteralString("left")).
		Field("payload", leftPayload).
		Build()
	b := typetable.NewRecord().
		Field("kind", typ.LiteralString("event")).
		Field("payload_kind", typ.LiteralString("right")).
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
	chanInt := typ.NewAlias("__test_ChanInt", typetable.NewRecord().
		Field("__tag", typ.LiteralString("int")).
		Build())
	errCase := typetable.NewRecord().
		Field("channel", chanInt).
		Field("value", typetable.NewRecord().Field("error", typ.String).Build()).
		Build()

	d := NewDetector()
	tags := d.RequiredTags(errCase)
	if tags["channel.__tag"] != typ.EqualityHash(typ.LiteralString("int")) {
		t.Fatalf("nested channel tag was not summarized: %v", tags)
	}
}

func TestRequiredTagsRecursiveCycleSummarizesFiniteTags(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("kind", typ.LiteralString("node")).
			Field("next", self).
			Build()
	})

	d := NewDetector()
	tags := d.RequiredTags(node)
	if tags["kind"] != typ.EqualityHash(typ.LiteralString("node")) {
		t.Fatalf("recursive top-level tag was not summarized: %v", tags)
	}
	if _, ok := tags["next.kind"]; ok {
		t.Fatalf("recursive discriminant summary unfolded through self: %v", tags)
	}
}

func TestClosedRecordSetConflict(t *testing.T) {
	conflicting := []*typ.Record{
		typetable.NewRecord().
			Field("kind", typ.LiteralString("a")).
			Field("x", typ.Number).
			Build(),
		typetable.NewRecord().
			Field("kind", typ.LiteralString("b")).
			Field("y", typ.String).
			Build(),
	}
	d := NewDetector()
	if !d.ClosedRecordSetConflicts(conflicting) {
		t.Fatal("closed record set with required literal variants did not conflict")
	}

	clean := []*typ.Record{
		typetable.NewRecord().
			Field("from", typ.Func().Param("self", typ.Self).Returns(typ.Self).Build()).
			Field("where", typ.Func().Param("self", typ.Self).Param("clause", typ.String).Returns(typ.Self).Build()).
			Build(),
		typetable.NewRecord().
			Field("from", typ.Func().Param("self", typ.Self).Returns(typ.Self).Build()).
			Field("where", typ.Func().Param("self", typ.Self).Param("clause", typ.String).Returns(typ.Self).Build()).
			Field("limit", typ.Number).
			Build(),
	}
	if d.ClosedRecordSetConflicts(clean) {
		t.Fatal("records without required literal discriminants reported a conflict")
	}
}

func TestRequiredTagsReturnsDefensiveCopy(t *testing.T) {
	rec := typetable.NewRecord().
		Field("kind", typ.LiteralString("event")).
		Build()

	d := NewDetector()
	tags := d.RequiredTags(rec)
	tags["kind"] = 0
	tags["extra"] = typ.EqualityHash(typ.LiteralString("mutated"))

	got := d.RequiredTags(rec)
	if got["kind"] != typ.EqualityHash(typ.LiteralString("event")) {
		t.Fatalf("cached tag was mutated through returned map: %v", got)
	}
	if _, ok := got["extra"]; ok {
		t.Fatalf("cached tags include caller mutation: %v", got)
	}
}

func TestRequiredTagsDoesNotCacheEmptySummaries(t *testing.T) {
	plain := typetable.NewRecord().
		Field("value", typ.String).
		Build()
	tagged := typetable.NewRecord().
		Field("kind", typ.LiteralString("event")).
		Build()

	d := NewDetector()
	if tags := d.RequiredTags(plain); tags != nil {
		t.Fatalf("plain record tags = %v, want nil", tags)
	}
	if _, ok := d.tags[plain]; ok {
		t.Fatal("empty required-tag summary was retained in detector cache")
	}
	if tags := d.RequiredTags(tagged); tags["kind"] != typ.EqualityHash(typ.LiteralString("event")) {
		t.Fatalf("tagged record tags = %v, want kind tag", tags)
	}
	if _, ok := d.tags[tagged]; !ok {
		t.Fatal("non-empty required-tag summary was not cached")
	}
}

func TestRequiredTagsAcyclicTypesDoNotAllocateCycleGuard(t *testing.T) {
	plain := typetable.NewRecord().
		Field("value", typ.String).
		Build()
	tagged := typetable.NewRecord().
		Field("kind", typ.LiteralString("event")).
		Field("payload", typetable.NewRecord().Field("id", typ.String).Build()).
		Build()

	d := NewDetector()
	if tags := d.RequiredTags(plain); tags != nil {
		t.Fatalf("plain record tags = %v, want nil", tags)
	}
	if tags := d.RequiredTags(tagged); tags["kind"] != typ.EqualityHash(typ.LiteralString("event")) {
		t.Fatalf("tagged record tags = %v, want kind tag", tags)
	}
	if d.active != nil {
		t.Fatalf("acyclic required-tag extraction allocated cycle guard: %#v", d.active)
	}
}

func TestRequiredTagsRecursiveTypesStillUseCycleGuard(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("kind", typ.LiteralString("node")).
			Field("next", self).
			Build()
	})

	d := NewDetector()
	tags := d.RequiredTags(node)
	if tags["kind"] != typ.EqualityHash(typ.LiteralString("node")) {
		t.Fatalf("recursive tags = %v, want top-level kind", tags)
	}
	if d.active == nil {
		t.Fatal("recursive required-tag extraction did not allocate cycle guard")
	}
	if len(d.active) != 0 {
		t.Fatalf("recursive cycle guard retained active entries: %#v", d.active)
	}
}

func BenchmarkRequiredTagsAcyclicTagless(b *testing.B) {
	plain := typetable.NewRecord().
		Field("value", typ.String).
		Field("nested", typetable.NewRecord().Field("id", typ.String).Build()).
		Build()
	d := NewDetector()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if tags := d.RequiredTags(plain); tags != nil {
			b.Fatalf("plain record tags = %v, want nil", tags)
		}
	}
}

func TestStaticMemberTagPathAndConflict(t *testing.T) {
	a := typetable.NewRecord().
		StaticStringIndex("kind", typ.LiteralString("a")).
		Field("payload", typ.Number).
		Build()
	b := typetable.NewRecord().
		StaticStringIndex("kind", typ.LiteralString("b")).
		Field("payload", typ.Number).
		Build()

	d := NewDetector()
	tags := d.RequiredTags(a)
	if tags[`["kind"]`] != typ.EqualityHash(typ.LiteralString("a")) {
		t.Fatalf("static string member tag path was not extracted: %v", tags)
	}
	if !d.RecordsConflict(a, b) {
		t.Fatal("static member literal tag did not report a record conflict")
	}
}

func TestCommonUnionTagsKeepsOnlyIdenticalTags(t *testing.T) {
	left := typetable.NewRecord().
		Field("kind", typ.LiteralString("event")).
		Field("side", typ.LiteralString("left")).
		Build()
	right := typetable.NewRecord().
		Field("kind", typ.LiteralString("event")).
		Field("side", typ.LiteralString("right")).
		Build()

	tags := NewDetector().RequiredTags(typeexpr.Union(left, right))
	if tags["kind"] != typ.EqualityHash(typ.LiteralString("event")) {
		t.Fatalf("common identical union tag missing: %v", tags)
	}
	if _, ok := tags["side"]; ok {
		t.Fatalf("differing union tag was retained: %v", tags)
	}
}

func TestPresenceConflictIgnoresOptionalAndLiteralFields(t *testing.T) {
	a := typetable.NewRecord().
		Field("kind", typ.LiteralString("a")).
		OptField("payload", typ.Number).
		Build()
	b := typetable.NewRecord().
		Field("kind", typ.LiteralString("b")).
		OptField("other", typ.String).
		Build()

	if NewDetector().RecordsPresenceConflict(a, b) {
		t.Fatal("optional or literal fields reported a presence conflict")
	}
}

func TestNilDetectorReceiverBehavior(t *testing.T) {
	rec := typetable.NewRecord().
		Field("kind", typ.LiteralString("event")).
		Build()

	var d *Detector
	if tags := d.RequiredTags(nil); tags != nil {
		t.Fatalf("nil RequiredTags result = %v, want nil", tags)
	}
	if !d.HasRequiredTag(rec) {
		t.Fatal("nil detector receiver did not collect required tags")
	}
	if !d.ClosedRecordSetConflicts([]*typ.Record{
		typetable.NewRecord().Field("kind", typ.LiteralString("a")).Build(),
		typetable.NewRecord().Field("kind", typ.LiteralString("b")).Build(),
	}) {
		t.Fatal("nil detector receiver did not detect closed-set tag conflict")
	}
	if d.RecordsPresenceConflict(nil, rec) {
		t.Fatal("nil record reported a presence conflict")
	}
}
