package typepath

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypeAtSegmentsProjectsNestedFields(t *testing.T) {
	base := typ.NewRecord().
		Field("node", typ.NewRecord().
			Field("id", typ.String).
			Build()).
		Build()

	got := Strict(base, []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "node"},
		{Kind: constraint.SegmentField, Name: "id"},
	})
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("projected type = %v, want string", got)
	}
}

func TestTypeAtSegmentsProjectsStringIndex(t *testing.T) {
	base := typ.NewMap(typ.String, typ.Number)

	got := Strict(base, []constraint.Segment{
		{Kind: constraint.SegmentIndexString, Name: "count"},
	})
	want := typ.NewOptional(typ.Number)
	if !typ.TypeEquals(got, want) {
		t.Fatalf("projected type = %v, want %v", got, want)
	}
}

func TestTypeAtSegmentsMissingFieldPolicy(t *testing.T) {
	base := typ.NewRecord().Build()
	segments := []constraint.Segment{{Kind: constraint.SegmentField, Name: "missing"}}

	if got := Strict(base, segments); got != nil {
		t.Fatalf("strict missing field = %v, want nil", got)
	}
	if got := TypeAtSegments(base, segments, Options{MissingFieldAsNil: true}); !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("observation missing field = %v, want nil", got)
	}
}
