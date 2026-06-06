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

func TestPlaceAssignRootValueWritesNestedStaticMember(t *testing.T) {
	t.Parallel()

	p := Place{
		Root:     7,
		RootName: "record",
		Steps: []Step{
			{Kind: StepStaticMember, Member: value.MemberField("payload")},
			{Kind: StepStaticMember, Member: value.MemberField("id")},
		},
	}
	root := product.FromType(typ.NewRecord().Build())
	want := product.FromType(typ.String)

	updated, ok := p.AssignRootValue(root, want, nil)
	if !ok {
		t.Fatal("AssignRootValue() unexpectedly false")
	}
	payload, ok := product.MemberOf(updated, value.MemberField("payload"))
	if !ok {
		t.Fatal("payload member missing")
	}
	got, ok := product.MemberOf(payload, value.MemberField("id"))
	if !ok {
		t.Fatal("payload.id member missing")
	}
	if !product.Domain.Equal(got, want) {
		t.Fatalf("payload.id = %v, want %v", got.ProjectValue(), want.ProjectValue())
	}
}

func TestPlaceAssignRootValueUsesFinalDynamicWriter(t *testing.T) {
	t.Parallel()

	key := product.FromType(typ.LiteralString("active"))
	val := product.FromType(typ.Number)
	p := Place{
		Root:     8,
		RootName: "items",
		Steps:    []Step{{Kind: StepDynamicIndex, Key: key}},
	}

	called := false
	updated, ok := p.AssignRootValue(product.FromType(typ.NewRecord().Build()), val,
		func(base product.AbstractValue, step Step, gotVal product.AbstractValue) (product.AbstractValue, bool) {
			called = true
			if step.Kind != StepDynamicIndex || !product.Domain.Equal(step.Key, key) {
				t.Fatalf("dynamic step = %#v, want key %v", step, key.ProjectValue())
			}
			if !product.Domain.Equal(gotVal, val) {
				t.Fatalf("value = %v, want %v", gotVal.ProjectValue(), val.ProjectValue())
			}
			return product.WriteIndexForeign(base, step.Key, gotVal), true
		})
	if !ok {
		t.Fatal("AssignRootValue() unexpectedly false")
	}
	if !called {
		t.Fatal("final dynamic writer was not called")
	}
	got, ok := product.IndexOf(updated, key)
	if !ok {
		t.Fatal("dynamic index missing after assignment")
	}
	if !product.Domain.Equal(got, val) {
		t.Fatalf("dynamic index = %v, want %v", got.ProjectValue(), val.ProjectValue())
	}
}

func TestPlaceUpdateRootValueCreatesMissingIntermediateRecord(t *testing.T) {
	t.Parallel()

	p := Place{
		Root:     9,
		RootName: "root",
		Steps: []Step{
			{Kind: StepStaticMember, Member: value.MemberField("child")},
			{Kind: StepStaticMember, Member: value.MemberField("count")},
		},
	}
	want := product.FromType(typ.Number)

	updated, ok := p.UpdateRootValue(product.FromType(typ.NewRecord().Build()), func(product.AbstractValue) (product.AbstractValue, bool) {
		return want, true
	})
	if !ok {
		t.Fatal("UpdateRootValue() unexpectedly false")
	}
	child, ok := product.MemberOf(updated, value.MemberField("child"))
	if !ok {
		t.Fatal("child member missing")
	}
	got, ok := product.MemberOf(child, value.MemberField("count"))
	if !ok {
		t.Fatal("child.count member missing")
	}
	if !product.Domain.Equal(got, want) {
		t.Fatalf("child.count = %v, want %v", got.ProjectValue(), want.ProjectValue())
	}
}
