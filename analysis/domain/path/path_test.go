package path

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestPathPlaceholderIndex_OverflowRejected(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflowRoot := "$" + strconv.FormatInt(int64(maxInt), 10) + "0"
	p := Path{Root: overflowRoot}

	if got := p.PlaceholderIndex(); got != -1 {
		t.Fatalf("PlaceholderIndex() = %d, want -1 for overflow input", got)
	}
	if p.IsPlaceholder() {
		t.Fatal("IsPlaceholder() should be false for overflow placeholder root")
	}
}

func TestPathStringKeyHash(t *testing.T) {
	p := Path{
		Root: "x",
		Segments: []segment.Segment{
			{Kind: segment.SegmentField, Name: "y"},
			{Kind: segment.SegmentIndexString, Name: "z"},
			{Kind: segment.SegmentIndexInt, Index: 3},
		},
	}
	display := "x.y[z][3]"
	key := `x.y["z"][3]`

	if p.String() != display {
		t.Fatalf("expected %q, got %q", display, p.String())
	}

	if p.Key() != PathKey(key) {
		t.Fatalf("expected key %q, got %q", key, p.Key())
	}

	p2 := Path{Root: "x", Segments: []segment.Segment{
		{Kind: segment.SegmentField, Name: "y"},
		{Kind: segment.SegmentIndexString, Name: "z"},
		{Kind: segment.SegmentIndexInt, Index: 3},
	}}
	if p.Hash() != p2.Hash() {
		t.Fatalf("expected stable hash, got %d vs %d", p.Hash(), p2.Hash())
	}
}

func TestPathLessAndAppend(t *testing.T) {
	a := Path{Root: "a"}
	b := Path{Root: "b"}

	if !a.Less(b) {
		t.Fatalf("expected a < b")
	}

	seg := segment.Segment{Kind: segment.SegmentField, Name: "field"}

	ap := a.Append(seg)
	if ap.Root != "a" || len(ap.Segments) != 1 {
		t.Fatalf("expected appended path, got %#v", ap)
	}

	if ap.Segments[0] != seg {
		t.Fatalf("expected same segment, got %#v", ap.Segments[0])
	}
}

func TestPathDirectFieldName(t *testing.T) {
	tests := []struct {
		name string
		path Path
		want string
		ok   bool
	}{
		{
			name: "dot field",
			path: Path{Root: "x", Symbol: 1, Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "f"}}},
			want: "f",
			ok:   true,
		},
		{
			name: "string index",
			path: Path{Root: "x", Symbol: 1, Segments: []segment.Segment{{Kind: segment.SegmentIndexString, Name: "f"}}},
			want: "f",
			ok:   true,
		},
		{
			name: "nested field rejected",
			path: Path{Root: "x", Symbol: 1, Segments: []segment.Segment{
				{Kind: segment.SegmentField, Name: "a"},
				{Kind: segment.SegmentField, Name: "b"},
			}},
		},
		{
			name: "integer index rejected",
			path: Path{Root: "x", Symbol: 1, Segments: []segment.Segment{{Kind: segment.SegmentIndexInt, Index: 1}}},
		},
		{
			name: "empty name rejected",
			path: Path{Root: "x", Symbol: 1, Segments: []segment.Segment{{Kind: segment.SegmentField}}},
		},
		{
			name: "root only rejected",
			path: Path{Root: "x", Symbol: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.path.DirectFieldName()
			if got != tt.want || ok != tt.ok {
				t.Fatalf("DirectFieldName() = %q/%v, want %q/%v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPathDirectIntIndex(t *testing.T) {
	tests := []struct {
		name string
		path Path
		want int
		ok   bool
	}{
		{
			name: "integer index",
			path: NewPath(1, "x").IndexInt(7),
			want: 7,
			ok:   true,
		},
		{
			name: "field rejected",
			path: NewPath(1, "x").Field("f"),
		},
		{
			name: "nested integer index rejected",
			path: NewPath(1, "x").Field("items").IndexInt(7),
		},
		{
			name: "root only rejected",
			path: NewPath(1, "x"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.path.DirectIntIndex()
			if got != tt.want || ok != tt.ok {
				t.Fatalf("DirectIntIndex() = %d/%v, want %d/%v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestPathCloneCopiesSegments(t *testing.T) {
	p := NewPath(1, "x").Field("a").IndexStr("b")
	clone := p.Clone()

	if !clone.Equal(p) {
		t.Fatalf("Clone() = %v, want equal to %v", clone, p)
	}
	clone.Segments[0].Name = "mutated"
	if got := p.Segments[0].Name; got != "a" {
		t.Fatalf("Clone() shares segment storage: original first segment = %q", got)
	}
}

func TestPathClonePreservesRootOnlyPath(t *testing.T) {
	p := NewPath(1, "x")
	clone := p.Clone()

	if !clone.Equal(p) {
		t.Fatalf("Clone() = %v, want equal to %v", clone, p)
	}
	if clone.Segments != nil {
		t.Fatalf("Clone() root-only Segments = %#v, want nil", clone.Segments)
	}
}

func TestPathRootOnlyDropsSuffixWithoutAliasing(t *testing.T) {
	p := NewPath(1, "x").Field("a").IndexStr("b")
	p.Version = 3

	root := p.RootOnly()
	if root.Root != "x" || root.Symbol != 1 || root.Version != 3 || len(root.Segments) != 0 {
		t.Fatalf("RootOnly() = %#v, want root identity without suffix", root)
	}
	root.Segments = append(root.Segments, segment.Segment{Kind: segment.SegmentField, Name: "mutated"})
	if got := p.Key(); got != `sym1@3.a["b"]` {
		t.Fatalf("RootOnly returned aliased suffix storage, original key now %q", got)
	}
}

func TestPathAppendSegmentsCopiesOnceAndDoesNotAliasInputs(t *testing.T) {
	base := NewPath(1, "x").Field("a")
	suffix := []segment.Segment{
		{Kind: segment.SegmentField, Name: "b"},
		{Kind: segment.SegmentIndexString, Name: "c"},
	}

	got := base.AppendSegments(suffix)
	if got.Key() != `sym1.a.b["c"]` {
		t.Fatalf("AppendSegments() key = %q, want sym1.a.b[\"c\"]", got.Key())
	}

	base.Segments[0].Name = "mutated-base"
	suffix[0].Name = "mutated-suffix"
	if got.Key() != `sym1.a.b["c"]` {
		t.Fatalf("AppendSegments() returned aliased storage: key now %q", got.Key())
	}
}

func TestPathAppendSegmentsPreservesRootOnlyCloneSemantics(t *testing.T) {
	base := NewPath(1, "x").Field("a")
	got := base.AppendSegments(nil)

	if !got.Equal(base) {
		t.Fatalf("AppendSegments(nil) = %#v, want %#v", got, base)
	}
	got.Segments[0].Name = "mutated"
	if base.Segments[0].Name != "a" {
		t.Fatalf("AppendSegments(nil) shares storage with base: %#v", base.Segments)
	}
}

func TestPathPrefixPredicatesUseCanonicalRootIdentity(t *testing.T) {
	root := NewPath(symbol.ID(10), "old").Field("items")
	sameSymbolDifferentDisplay := NewPath(symbol.ID(10), "new").Field("items").Field("name")
	differentVersion := sameSymbolDifferentDisplay
	differentVersion.Version = 2
	rootVersion := root
	rootVersion.Version = 2

	if !sameSymbolDifferentDisplay.HasPrefix(root) {
		t.Fatalf("HasPrefix rejected same symbol/version with different display roots")
	}
	if sameSymbolDifferentDisplay.HasPrefix(rootVersion) {
		t.Fatalf("HasPrefix accepted same symbol with different version")
	}
	if !sameSymbolDifferentDisplay.HasStrictPrefix(root) {
		t.Fatalf("HasStrictPrefix rejected proper ancestor")
	}
	if !sameSymbolDifferentDisplay.Overlaps(root) || !root.Overlaps(sameSymbolDifferentDisplay) {
		t.Fatalf("Overlaps should be symmetric for ancestor/descendant paths")
	}
	if !differentVersion.SameRoot(rootVersion) {
		t.Fatalf("SameRoot rejected matching symbol/version")
	}
}

func TestPathSuffixAfterCopiesRemainder(t *testing.T) {
	base := NewPath(symbol.ID(11), "root").Field("a")
	candidate := base.Field("b").IndexStr("c")

	suffix, ok := candidate.SuffixAfter(base)
	if !ok {
		t.Fatal("SuffixAfter rejected valid prefix")
	}
	if got := segment.FormatSegments(suffix); got != `.b["c"]` {
		t.Fatalf("SuffixAfter = %q, want .b[\"c\"]", got)
	}
	suffix[0].Name = "mutated"
	if got := candidate.Key(); got != `sym11.a.b["c"]` {
		t.Fatalf("SuffixAfter returned aliased suffix, candidate now %q", got)
	}
}

func TestPathSubstitute(t *testing.T) {
	placeholder := Path{Root: "$0"}
	arg := Path{Root: "vol"}

	result, ok := placeholder.Substitute([]Path{arg})
	if !ok {
		t.Fatal("Substitute failed")
	}
	if result.Root != "vol" {
		t.Fatalf("expected Root='vol', got %q", result.Root)
	}
}

func TestPathKeyUsesFormatSegments(t *testing.T) {
	segs := []segment.Segment{
		{Kind: segment.SegmentField, Name: "foo"},
		{Kind: segment.SegmentIndexString, Name: "bar"},
		{Kind: segment.SegmentIndexInt, Index: 1},
	}

	p := Path{Symbol: 42, Segments: segs}
	expected := "sym42" + segment.FormatSegments(segs)
	got := string(p.Key())
	if got != expected {
		t.Fatalf("Path.Key() should use FormatSegments: expected %q, got %q", expected, got)
	}
}

func TestPathKeyPlaceholderUsesCanonicalSegments(t *testing.T) {
	pStr := Path{
		Root: "$0",
		Segments: []segment.Segment{
			{Kind: segment.SegmentIndexString, Name: "1"},
		},
	}
	pInt := Path{
		Root: "$0",
		Segments: []segment.Segment{
			{Kind: segment.SegmentIndexInt, Index: 1},
		},
	}

	if got, want := string(pStr.Key()), `$0["1"]`; got != want {
		t.Fatalf("string index placeholder key: got %q, want %q", got, want)
	}
	if got, want := string(pInt.Key()), `$0[1]`; got != want {
		t.Fatalf("int index placeholder key: got %q, want %q", got, want)
	}
	if pStr.Key() == pInt.Key() {
		t.Fatalf("placeholder keys must not collide: %q", pStr.Key())
	}
}

func TestNewPath(t *testing.T) {
	p := NewPath(42, "x")
	if p.Symbol != 42 {
		t.Errorf("expected Symbol=42, got %d", p.Symbol)
	}
	if p.Root != "x" {
		t.Errorf("expected Root='x', got %q", p.Root)
	}
	if len(p.Segments) != 0 {
		t.Errorf("expected no segments, got %d", len(p.Segments))
	}
}

func TestNewPlaceholder(t *testing.T) {
	tests := []struct {
		index    int
		expected string
	}{
		{0, "$0"},
		{1, "$1"},
		{10, "$10"},
	}
	for _, tc := range tests {
		p := NewPlaceholder(tc.index)
		if p.Root != tc.expected {
			t.Errorf("NewPlaceholder(%d): expected Root=%q, got %q", tc.index, tc.expected, p.Root)
		}
		if p.Symbol != 0 {
			t.Errorf("NewPlaceholder(%d): expected Symbol=0, got %d", tc.index, p.Symbol)
		}
		if !p.IsPlaceholder() {
			t.Errorf("NewPlaceholder(%d): expected IsPlaceholder()=true", tc.index)
		}
		if p.PlaceholderIndex() != tc.index {
			t.Errorf("NewPlaceholder(%d): PlaceholderIndex()=%d", tc.index, p.PlaceholderIndex())
		}
	}
}

func TestNewPlaceholder_NegativeIndex(t *testing.T) {
	p := NewPlaceholder(-1)
	if !p.IsEmpty() {
		t.Fatalf("NewPlaceholder(-1) should return empty path, got %+v", p)
	}
	if p.IsPlaceholder() {
		t.Fatal("empty path must not be placeholder")
	}
}

func TestPathFluentBuilders(t *testing.T) {
	p := NewPath(1, "obj").Field("data").IndexStr("key").IndexInt(0)

	if p.String() != "obj.data[key][0]" {
		t.Errorf("expected 'obj.data[key][0]', got %q", p.String())
	}
	if len(p.Segments) != 3 {
		t.Errorf("expected 3 segments, got %d", len(p.Segments))
	}
	if p.Segments[0].Kind != segment.SegmentField || p.Segments[0].Name != "data" {
		t.Errorf("segment 0: expected field 'data', got %+v", p.Segments[0])
	}
	if p.Segments[1].Kind != segment.SegmentIndexString || p.Segments[1].Name != "key" {
		t.Errorf("segment 1: expected index string 'key', got %+v", p.Segments[1])
	}
	if p.Segments[2].Kind != segment.SegmentIndexInt || p.Segments[2].Index != 0 {
		t.Errorf("segment 2: expected index int 0, got %+v", p.Segments[2])
	}
}

func TestPathParent(t *testing.T) {
	p := NewPath(1, "x").Field("a").Field("b")
	parent := p.Parent()

	if parent.String() != "x.a" {
		t.Errorf("expected 'x.a', got %q", parent.String())
	}

	grandparent := parent.Parent()
	if grandparent.String() != "x" {
		t.Errorf("expected 'x', got %q", grandparent.String())
	}

	empty := grandparent.Parent()
	if !empty.IsEmpty() {
		t.Errorf("expected empty path, got %q", empty.String())
	}
}

func TestPathLastSegment(t *testing.T) {
	p := NewPath(1, "x").Field("y").IndexInt(5)

	seg, ok := p.LastSegment()
	if !ok {
		t.Fatal("expected LastSegment to return ok=true")
	}
	if seg.Kind != segment.SegmentIndexInt || seg.Index != 5 {
		t.Errorf("expected index int 5, got %+v", seg)
	}

	root := NewPath(1, "x")
	_, ok = root.LastSegment()
	if ok {
		t.Error("expected LastSegment to return ok=false for root path")
	}
}

func TestPathLessComprehensive(t *testing.T) {
	tests := []struct {
		name   string
		a      Path
		b      Path
		expect bool
	}{
		{
			"symbol comparison a < b",
			NewPath(1, "x"),
			NewPath(2, "x"),
			true,
		},
		{
			"symbol comparison a > b",
			NewPath(2, "x"),
			NewPath(1, "x"),
			false,
		},
		{
			"root comparison a < b",
			Path{Root: "a"},
			Path{Root: "b"},
			true,
		},
		{
			"root comparison a > b",
			Path{Root: "b"},
			Path{Root: "a"},
			false,
		},
		{
			"segment length a < b",
			NewPath(1, "x"),
			NewPath(1, "x").Field("y"),
			true,
		},
		{
			"segment length a > b",
			NewPath(1, "x").Field("y"),
			NewPath(1, "x"),
			false,
		},
		{
			"segment kind comparison",
			NewPath(1, "x").Field("y"),
			NewPath(1, "x").IndexInt(0),
			true,
		},
		{
			"segment name comparison a < b",
			NewPath(1, "x").Field("a"),
			NewPath(1, "x").Field("b"),
			true,
		},
		{
			"segment name comparison a > b",
			NewPath(1, "x").Field("b"),
			NewPath(1, "x").Field("a"),
			false,
		},
		{
			"segment index comparison a < b",
			NewPath(1, "x").IndexInt(1),
			NewPath(1, "x").IndexInt(2),
			true,
		},
		{
			"segment index comparison a > b",
			NewPath(1, "x").IndexInt(2),
			NewPath(1, "x").IndexInt(1),
			false,
		},
		{
			"equal paths",
			NewPath(1, "x").Field("y"),
			NewPath(1, "x").Field("y"),
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Less(tc.b); got != tc.expect {
				t.Errorf("Less() = %v, want %v", got, tc.expect)
			}
		})
	}
}

func TestPathHasSymbol(t *testing.T) {
	withSym := NewPath(1, "x")
	if !withSym.HasSymbol() {
		t.Error("Path with Symbol should return HasSymbol() = true")
	}

	withoutSym := Path{Root: "x"}
	if withoutSym.HasSymbol() {
		t.Error("Path without Symbol should return HasSymbol() = false")
	}
}

func TestPathDisplayRoot(t *testing.T) {
	resolver := func(id symbol.ID) string {
		if id == 1 {
			return "myResolvedVar"
		}
		return ""
	}

	p := NewPath(1, "myVar")
	if got := p.DisplayRoot(resolver); got != "myResolvedVar" {
		t.Errorf("DisplayRoot() = %q, want %q", got, "myResolvedVar")
	}

	sym := NewPath(42, "original")
	if got := sym.DisplayRoot(nil); got != "original" {
		t.Errorf("DisplayRoot(nil) = %q, want %q", got, "original")
	}

	empty := Path{}
	if got := empty.DisplayRoot(nil); got != "" {
		t.Errorf("DisplayRoot() = %q, want empty", got)
	}
}

func TestPathValidateSymbol(t *testing.T) {
	valid := NewPath(1, "x")
	if msg := valid.ValidateSymbol(); msg != "" {
		t.Errorf("ValidateSymbol() returned error for valid path: %v", msg)
	}

	invalid := Path{Root: "x"}
	if msg := invalid.ValidateSymbol(); msg == "" {
		t.Error("ValidateSymbol() should return error for path without symbol")
	}

	empty := Path{}
	if msg := empty.ValidateSymbol(); msg != "" {
		t.Errorf("ValidateSymbol() should return empty for empty path: %v", msg)
	}

	placeholder := NewPlaceholder(0)
	if msg := placeholder.ValidateSymbol(); msg != "" {
		t.Errorf("ValidateSymbol() should return empty for placeholder: %v", msg)
	}
}

func TestPathEqual(t *testing.T) {
	a := NewPath(1, "x").Field("y")
	b := NewPath(1, "x").Field("y")
	c := NewPath(1, "x").Field("z")
	d := NewPath(2, "x").Field("y")
	e := Path{Root: "x"}.Field("y")
	f := Path{Root: "x"}.Field("y")
	g := Path{Root: "x"}.Field("z")

	if !a.Equal(b) {
		t.Error("Equal paths should be equal")
	}
	if a.Equal(c) {
		t.Error("Paths with different fields should not be equal")
	}
	if a.Equal(d) {
		t.Error("Paths with different symbols should not be equal")
	}
	if !e.Equal(f) {
		t.Error("Paths with same root should be equal")
	}
	if e.Equal(g) {
		t.Error("Paths with different fields should not be equal")
	}
	if a.Equal(e) {
		t.Error("Path with symbol should not equal path without symbol")
	}
}

// TestPathParentSliceAliasing tests for slice aliasing bugs in Parent().
// If Parent() shares the backing array with the original path's segments,
// modifications to one could corrupt the other.
func TestPathParentSliceAliasing(t *testing.T) {
	// Create a path with extra capacity in the segments slice
	segments := make([]segment.Segment, 2, 4)
	segments[0] = segment.Segment{Kind: segment.SegmentField, Name: "a"}
	segments[1] = segment.Segment{Kind: segment.SegmentField, Name: "b"}
	original := Path{Root: "x", Symbol: 1, Segments: segments}

	parent := original.Parent()

	// Now append to parent's segments
	// If they share backing array, this would corrupt original
	if cap(parent.Segments) > len(parent.Segments) {
		// There's room in the backing array - this append would corrupt original
		// if slice aliasing exists
		extended := append(parent.Segments, segment.Segment{Kind: segment.SegmentField, Name: "CORRUPTED"})
		_ = extended

		// Check if original is corrupted
		if len(original.Segments) == 2 && original.Segments[1].Name != "b" {
			t.Error("BUG: Parent() slice aliasing corrupted original path")
		}
	}

	// Verify parent has correct content
	if len(parent.Segments) != 1 {
		t.Errorf("Parent should have 1 segment, got %d", len(parent.Segments))
	}
	if parent.Segments[0].Name != "a" {
		t.Errorf("Parent segment should be 'a', got %q", parent.Segments[0].Name)
	}
}
