package path

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

func TestCanonicalKeyGrammarRoundTripsEveryRootForm(t *testing.T) {
	tests := []struct {
		name string
		path Path
		key  PathKey
	}{
		{name: "stable symbol", path: Path{Symbol: 7}, key: "sym7"},
		{name: "local symbol", path: Path{Symbol: 7, Version: 3, Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "item"}}}, key: "sym7@3.item"},
		{name: "named", path: Path{Root: "a.b", Segments: []segment.Segment{{Kind: segment.SegmentIndexInt, Index: 1}}}, key: "n3:a.b[1]"},
		{name: "placeholder", path: NewPlaceholder(12).Field("item"), key: "n3:$12.item"},
		{name: "return slot", path: Path{Root: "ret[2]", Segments: []segment.Segment{{Kind: segment.SegmentIndexString, Name: "ok"}}}, key: `n6:ret[2]["ok"]`},
		{name: "rootless suffix", path: Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "item"}}}, key: "n0:.item"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatKey(tc.path); got != tc.key {
				t.Fatalf("FormatKey() = %q, want %q", got, tc.key)
			}
			got, ok := ParseKey(tc.key)
			if !ok || !reflect.DeepEqual(got, tc.path) {
				t.Fatalf("ParseKey(%q) = %#v/%v, want %#v/true", tc.key, got, ok, tc.path)
			}
			got.Segments = append(got.Segments, segment.Segment{Kind: segment.SegmentField, Name: "mutated"})
			again, _ := ParseKey(tc.key)
			if !reflect.DeepEqual(again, tc.path) {
				t.Fatalf("ParseKey(%q) exposed interned segment storage: %#v", tc.key, again)
			}
		})
	}
}

func TestCanonicalKeyGrammarRejectsDisplacedSpellings(t *testing.T) {
	for _, key := range []PathKey{
		"",
		"s7",
		"$0.item",
		"ret[1].item",
		"global.item",
		".item",
		"n00:",
		"n02:$0",
		"n3:$0",
		"n0:",
		"sym0",
		"sym07",
		"sym7@0",
		"sym7@03",
		"sym7.",
	} {
		if got, ok := ParseKey(key); ok || !got.IsEmpty() || len(got.Segments) != 0 {
			t.Fatalf("ParseKey(%q) = %#v/%v, want rejected", key, got, ok)
		}
	}
}

func TestCanonicalKeyFormatterRejectsInvalidStructure(t *testing.T) {
	for _, path := range []Path{
		{},
		{Symbol: 1, Version: -1},
		{Root: "x", Version: 1},
		{Root: "x", Segments: []segment.Segment{{Kind: 255}}},
		{Root: "x", Segments: []segment.Segment{{Kind: segment.SegmentField}}},
		{Root: "x", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "a.b"}}},
	} {
		if got := FormatKey(path); got != "" {
			t.Fatalf("FormatKey(%#v) = %q, want rejected", path, got)
		}
	}
}
