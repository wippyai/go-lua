package segment

import (
	"strings"
	"testing"
)

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

func TestFormattedLenAndWriterMatchFormatSegments(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentField, Name: "meta"},
		{Kind: SegmentIndexString, Name: `a"b\c`},
		{Kind: SegmentIndexInt, Index: -123},
	}
	want := FormatSegments(segs)
	if got := FormattedLen(segs); got != len(want) {
		t.Fatalf("FormattedLen = %d, want %d for %q", got, len(want), want)
	}
	var b strings.Builder
	b.Grow(FormattedLen(segs))
	WriteFormattedSegments(&b, segs)
	if got := b.String(); got != want {
		t.Fatalf("WriteFormattedSegments = %q, want %q", got, want)
	}
}

func TestDirectFieldName(t *testing.T) {
	if got, ok := DirectFieldName([]Segment{{Kind: SegmentField, Name: "id"}}); !ok || got != "id" {
		t.Fatalf("DirectFieldName(field) = %q/%v, want id/true", got, ok)
	}
	if _, ok := DirectFieldName(nil); ok {
		t.Fatal("DirectFieldName(nil) returned ok")
	}
	if _, ok := DirectFieldName([]Segment{{Kind: SegmentField, Name: "a"}, {Kind: SegmentField, Name: "b"}}); ok {
		t.Fatal("DirectFieldName(two fields) returned ok")
	}
	if _, ok := DirectFieldName([]Segment{{Kind: SegmentIndexString, Name: "id"}}); ok {
		t.Fatal("DirectFieldName(string index) returned ok")
	}
	if _, ok := DirectFieldName([]Segment{{Kind: SegmentIndexInt, Index: 1}}); ok {
		t.Fatal("DirectFieldName(integer index) returned ok")
	}
}

func TestFormatSegmentsAllocatesOnlyResultString(t *testing.T) {
	segs := []Segment{
		{Kind: SegmentField, Name: "meta"},
		{Kind: SegmentIndexString, Name: `a"b\c`},
		{Kind: SegmentIndexInt, Index: -123},
	}
	var got string
	allocs := testing.AllocsPerRun(1000, func() {
		got = FormatSegments(segs)
	})
	if got != `.meta["a\"b\\c"][-123]` {
		t.Fatalf("FormatSegments = %q", got)
	}
	if allocs > 1 {
		t.Fatalf("FormatSegments allocations/run = %.1f, want only result string", allocs)
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
		t.Fatalf("repeated parse shared mutable storage: %#v/%v", second, ok)
	}
}

func TestParseFormattedSegmentsDoesNotShareStorage(t *testing.T) {
	first, ok := ParseFormattedSegments(`.field["key"]`)
	if !ok || len(first) != 2 {
		t.Fatalf("ParseFormattedSegments = %#v/%v", first, ok)
	}
	first[0].Name = "mutated"

	second, ok := ParseFormattedSegments(`.field["key"]`)
	if !ok || len(second) != 2 || second[0].Name != "field" || second[1].Name != "key" {
		t.Fatalf("repeated parse shared mutable storage: %#v/%v", second, ok)
	}
}

func TestParseFormattedSegmentsAllocatesOnlyOwnedResult(t *testing.T) {
	var got []Segment
	allocs := testing.AllocsPerRun(1000, func() {
		got, _ = ParseFormattedSegments(".field")
	})
	if len(got) != 1 || got[0] != (Segment{Kind: SegmentField, Name: "field"}) {
		t.Fatalf("ParseFormattedSegments = %#v", got)
	}
	if allocs > 1 {
		t.Fatalf("ParseFormattedSegments allocations/run = %.1f, want one owned segment slice", allocs)
	}
}

func TestValidFormattedSegmentsDoesNotAllocate(t *testing.T) {
	const suffix = `.field["a\\b"][42]`
	if !ValidFormattedSegments(suffix) {
		t.Fatalf("ValidFormattedSegments(%q) rejected canonical suffix", suffix)
	}
	if ValidFormattedSegments(`[999999999999999999999999999999999999]`) {
		t.Fatal("ValidFormattedSegments accepted an overflowing integer index")
	}
	allocs := testing.AllocsPerRun(1000, func() {
		if !ValidFormattedSegments(suffix) {
			t.Fatal("ValidFormattedSegments rejected canonical suffix during allocation check")
		}
	})
	if allocs != 0 {
		t.Fatalf("ValidFormattedSegments allocations/run = %.1f, want zero", allocs)
	}
}

func TestValidFormattedSegmentsMatchesParser(t *testing.T) {
	for _, suffix := range []string{
		"",
		`.field["key"][1]`,
		`.field["a\\b"][-2]`,
		".",
		`["unterminated]`,
		`["bad\q"]`,
		"[1x]",
		"[999999999999999999999999999999999999]",
	} {
		_, parsed := ParseFormattedSegments(suffix)
		if got := ValidFormattedSegments(suffix); got != parsed {
			t.Fatalf("ValidFormattedSegments(%q) = %v, ParseFormattedSegments = %v", suffix, got, parsed)
		}
	}
}
