package place

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestPlaceFromStaticPathRoundTrip(t *testing.T) {
	t.Parallel()

	wantPath := constraint.NewPath(cfg.SymbolID(12), "entry")
	wantPath.Segments = append(wantPath.Segments,
		constraint.Segment{Kind: constraint.SegmentField, Name: "items"},
		constraint.Segment{Kind: constraint.SegmentIndexString, Name: "active"},
		constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 3},
	)

	p, ok := FromStaticPath(wantPath)
	if !ok {
		t.Fatalf("FromStaticPath(%v) = false", wantPath)
	}
	got, ok := p.StaticPath()
	if !ok {
		t.Fatalf("StaticPath() unexpectedly false")
	}
	if !got.Equal(wantPath) {
		t.Fatalf("StaticPath() = %#v, want %#v", got, wantPath)
	}
	if string(got.Key()) == "" {
		t.Fatal("StaticPath().Key() must be non-empty")
	}
	if p.Root != wantPath.Symbol || p.RootName != wantPath.Root {
		t.Fatalf("place root = (%d,%q), want (%d,%q)", p.Root, p.RootName, wantPath.Symbol, wantPath.Root)
	}
	p2, ok := FromStaticPath(got)
	if !ok {
		t.Fatal("FromStaticPath(lossless path) unexpectedly failed")
	}
	if !reflect.DeepEqual(p, p2) {
		t.Fatalf("FromStaticPath(StaticPath) = %#v, want %#v", p2, p)
	}
}

func TestPlaceStaticPrefixAndStringFormatting(t *testing.T) {
	t.Parallel()

	p := Place{
		Root:     99,
		RootName: "users",
		Steps: []Step{
			{Kind: StepStaticMember, Member: value.MemberField("byName")},
			{Kind: StepDynamicIndex, Key: product.FromType(typ.String)},
			{Kind: StepStaticMember, Member: value.MemberField("id")},
		},
	}

	prefix, ok := p.StaticPrefixPath()
	if !ok {
		t.Fatal("StaticPrefixPath() unexpectedly false")
	}
	if got, want := prefix.String(), "users.byName"; got != want {
		t.Fatalf("StaticPrefixPath() = %q, want %q", got, want)
	}

	if _, ok := p.StaticPath(); ok {
		t.Fatal("StaticPath() unexpectedly true for unresolved dynamic index")
	}

	s := p.String()
	if s != "users.byName[?].id" {
		t.Fatalf("String() = %q, want users.byName[?].id", s)
	}
	if got, want := p.String(), p.String(); got != want {
		t.Fatalf("String() nondeterministic: %q != %q", got, want)
	}
}

func TestPlaceDynamicLiteralIndexConvertsToSegment(t *testing.T) {
	t.Parallel()

	p := Place{
		Root:     5,
		RootName: "cache",
		Steps: []Step{
			{Kind: StepDynamicIndex, Key: product.FromType(typ.LiteralString("user"))},
		},
	}
	path, ok := p.StaticPath()
	if !ok {
		t.Fatal("StaticPath() unexpectedly false for literal dynamic key")
	}
	if got, want := path.String(), `cache[user]`; got != want {
		t.Fatalf("StaticPath() = %q, want %q", got, want)
	}
	if got, want := string(path.Key()), `sym5["user"]`; got != want {
		t.Fatalf("StaticPath().Key() = %q, want %q", got, want)
	}
	if got, want := path.Segments[0].Kind, constraint.SegmentIndexString; got != want {
		t.Fatalf("segment kind = %#v, want %#v", got, want)
	}
}
