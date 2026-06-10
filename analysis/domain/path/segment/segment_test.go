package segment

import "testing"

func TestFormatSegments(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentField, Name: "foo"},
		{Kind: SegmentIndexString, Name: "bar"},
		{Kind: SegmentIndexInt, Index: 1},
	}

	expected := `.foo["bar"][1]`
	result := FormatSegments(segs)
	if result != expected {
		t.Fatalf("FormatSegments: expected %q, got %q", expected, result)
	}
}

func TestFormatSegmentsEscapesStringIndex(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentIndexString, Name: `a"b\c`},
	}
	expected := `["a\"b\\c"]`
	if got := FormatSegments(segs); got != expected {
		t.Fatalf("FormatSegments escaped index: expected %q, got %q", expected, got)
	}
}

func TestParseFormattedSegmentsRoundTripsCanonicalSuffix(t *testing.T) {
	want := []Segment{
		{Kind: SegmentField, Name: "meta"},
		{Kind: SegmentIndexString, Name: `a"b\c`},
		{Kind: SegmentIndexInt, Index: -2},
	}
	suffix := FormatSegments(want)
	got, ok := ParseFormattedSegments(suffix)
	if !ok {
		t.Fatalf("ParseFormattedSegments(%q) failed", suffix)
	}
	if len(got) != len(want) {
		t.Fatalf("segment len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestParseFormattedSegmentsRejectsNonCanonicalStringIndex(t *testing.T) {
	if got, ok := ParseFormattedSegments("[bad]"); ok || got != nil {
		t.Fatalf("ParseFormattedSegments accepted non-canonical string index: %#v/%v", got, ok)
	}
	if ValidFormattedSegments("[bad]") {
		t.Fatalf("ValidFormattedSegments accepted non-canonical string index")
	}
}

func TestParseFormattedSegmentsReturnsDefensiveCopy(t *testing.T) {
	first, ok := ParseFormattedSegments(".field")
	if !ok || len(first) != 1 {
		t.Fatalf("ParseFormattedSegments(.field) = %#v/%v", first, ok)
	}
	first[0].Name = "mutated"
	second, ok := ParseFormattedSegments(".field")
	if !ok || len(second) != 1 || second[0].Name != "field" {
		t.Fatalf("cached formatted segments were mutated: %#v/%v", second, ok)
	}
}
