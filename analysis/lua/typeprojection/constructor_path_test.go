package typeprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
)

func TestConstructorPathFromSegmentsMapsStaticLuaSegments(t *testing.T) {
	got, ok := ConstructorPathFromSegments([]segment.Segment{
		{Kind: segment.SegmentField, Name: "field"},
		{Kind: segment.SegmentIndexString, Name: "member"},
		{Kind: segment.SegmentIndexInt, Index: 3},
	})
	if !ok {
		t.Fatal("ConstructorPathFromSegments returned false")
	}
	want := []typetable.ConstructorKey{
		{Kind: typetable.ConstructorField, Name: "field"},
		{Kind: typetable.ConstructorStringIndex, Name: "member"},
		{Kind: typetable.ConstructorIntIndex, Index: 3},
	}
	if len(got) != len(want) {
		t.Fatalf("constructor path length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("constructor key[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestConstructorPathFromSegmentsRejectsDynamicOrEmptySegments(t *testing.T) {
	if got, ok := ConstructorPathFromSegments(nil); ok || got != nil {
		t.Fatalf("empty constructor path = %#v/%v, want rejection", got, ok)
	}
	if got, ok := ConstructorPathFromSegments([]segment.Segment{{Kind: segment.SegmentKind(99)}}); ok || got != nil {
		t.Fatalf("dynamic constructor path = %#v/%v, want rejection", got, ok)
	}
	if got, ok := ConstructorPathFromSegments([]segment.Segment{{Kind: segment.SegmentField}}); ok || got != nil {
		t.Fatalf("empty field constructor path = %#v/%v, want rejection", got, ok)
	}
}

func TestConstructorPathFromSegmentReaderMapsWithoutBorrowingSlice(t *testing.T) {
	segments := []segment.Segment{
		{Kind: segment.SegmentField, Name: "field"},
		{Kind: segment.SegmentIndexString, Name: "member"},
	}
	got, ok := ConstructorPathFromSegmentReader(len(segments), func(i int) (segment.Segment, bool) {
		if i < 0 || i >= len(segments) {
			return segment.Segment{}, false
		}
		return segments[i], true
	})
	if !ok {
		t.Fatal("ConstructorPathFromSegmentReader returned false")
	}
	want := []typetable.ConstructorKey{
		{Kind: typetable.ConstructorField, Name: "field"},
		{Kind: typetable.ConstructorStringIndex, Name: "member"},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("constructor key[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
	segments[0].Name = "mutated"
	if got[0].Name != "field" {
		t.Fatalf("constructor keys retained borrowed segment storage: %#v", got)
	}
}
