package access

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestStaticPathAcceptsLiteralDynamicKey(t *testing.T) {
	t.Parallel()

	loc := Location{
		Root:     cfg.SymbolID(7),
		RootName: "rows",
		Steps: []Step{
			{Kind: StepStaticMember, Member: value.MemberField("by_id")},
			{Kind: StepDynamicIndex, Key: product.FromType(typ.LiteralString("a"))},
			{Kind: StepStaticMember, Member: value.MemberField("name")},
		},
	}

	path, ok := loc.StaticPath()
	if !ok {
		t.Fatal("StaticPath() failed for literal dynamic key")
	}
	want := constraint.NewPath(cfg.SymbolID(7), "rows").
		Field("by_id").
		IndexStr("a").
		Field("name")
	if !path.Equal(want) {
		t.Fatalf("path = %v, want %v", path, want)
	}
}

func TestStaticPrefixStopsAtOpaqueDynamicKey(t *testing.T) {
	t.Parallel()

	loc := Location{
		Root:     cfg.SymbolID(8),
		RootName: "rows",
		Steps: []Step{
			{Kind: StepStaticMember, Member: value.MemberField("by_id")},
			{Kind: StepDynamicIndex, Key: product.FromType(typ.Unknown)},
			{Kind: StepStaticMember, Member: value.MemberField("name")},
		},
	}

	if _, ok := loc.StaticPath(); ok {
		t.Fatal("StaticPath() succeeded for opaque dynamic key")
	}
	prefix, ok := loc.StaticPrefixPath()
	if !ok {
		t.Fatal("StaticPrefixPath() failed")
	}
	want := constraint.NewPath(cfg.SymbolID(8), "rows").Field("by_id")
	if !prefix.Equal(want) {
		t.Fatalf("prefix = %v, want %v", prefix, want)
	}
}

func TestWriteFootprintCarriesPresentElementMember(t *testing.T) {
	t.Parallel()

	written := product.FromType(typ.String)
	loc := Location{
		Root:     cfg.SymbolID(9),
		RootName: "rows",
		Steps: []Step{
			{Kind: StepStaticMember, Member: value.MemberField("items")},
			{Kind: StepDynamicIndex, Key: product.FromType(typ.Unknown)},
			{Kind: StepStaticMember, Member: value.MemberField("name")},
			{Kind: StepStaticMember, Member: value.MemberField("first")},
		},
	}

	footprint, ok := loc.WriteFootprint(true, written)
	if !ok {
		t.Fatal("WriteFootprint() failed")
	}
	wantWrite := constraint.NewPath(cfg.SymbolID(9), "rows").Field("items")
	if !footprint.WritePath.Equal(wantWrite) {
		t.Fatalf("WritePath = %v, want %v", footprint.WritePath, wantWrite)
	}
	if !footprint.PresentElementWrite {
		t.Fatal("PresentElementWrite = false, want true")
	}
	if !footprint.HasPresentElementArrayPath || !footprint.PresentElementArrayPath.Equal(wantWrite) {
		t.Fatalf("PresentElementArrayPath = %v, want %v", footprint.PresentElementArrayPath, wantWrite)
	}
	wantMember := []constraint.Segment{
		{Kind: constraint.SegmentField, Name: "name"},
		{Kind: constraint.SegmentField, Name: "first"},
	}
	if len(footprint.PresentElementMember) != len(wantMember) {
		t.Fatalf("member len = %d, want %d", len(footprint.PresentElementMember), len(wantMember))
	}
	for i := range wantMember {
		if footprint.PresentElementMember[i] != wantMember[i] {
			t.Fatalf("member[%d] = %v, want %v", i, footprint.PresentElementMember[i], wantMember[i])
		}
	}
	if !product.Domain.Equal(footprint.Written, written) {
		t.Fatal("Written was not preserved")
	}
}

func TestWriteFootprintFinalDynamicHasNoMemberFootprint(t *testing.T) {
	t.Parallel()

	loc := Location{
		Root:     cfg.SymbolID(10),
		RootName: "rows",
		Steps: []Step{
			{Kind: StepStaticMember, Member: value.MemberField("items")},
			{Kind: StepDynamicIndex, Key: product.FromType(typ.Unknown)},
		},
	}

	footprint, ok := loc.WriteFootprint(true, product.AbstractValue{})
	if !ok {
		t.Fatal("WriteFootprint() failed")
	}
	if !footprint.PresentElementWrite {
		t.Fatal("PresentElementWrite = false, want true")
	}
	if footprint.HasPresentElementArrayPath || len(footprint.PresentElementMember) != 0 {
		t.Fatalf("unexpected member footprint: %#v", footprint)
	}
}

func TestFinalDynamicIndexTargetPathProjectsTable(t *testing.T) {
	t.Parallel()

	loc := Location{
		Root:     cfg.SymbolID(11),
		RootName: "rows",
		Steps: []Step{
			{Kind: StepStaticMember, Member: value.MemberField("items")},
			{Kind: StepDynamicIndex, Key: product.FromType(typ.LiteralString("bucket"))},
			{Kind: StepStaticMember, Member: value.MemberField("by_id")},
			{Kind: StepDynamicIndex, Key: product.FromType(typ.Unknown)},
		},
	}

	path, ok := loc.FinalDynamicIndexTargetPath()
	if !ok {
		t.Fatal("FinalDynamicIndexTargetPath() failed")
	}
	want := constraint.NewPath(cfg.SymbolID(11), "rows").
		Field("items").
		IndexStr("bucket").
		Field("by_id")
	if !path.Equal(want) {
		t.Fatalf("path = %v, want %v", path, want)
	}
}

func TestFinalDynamicIndexTargetPathRejectsStaticFinalStep(t *testing.T) {
	t.Parallel()

	loc := Location{
		Root:     cfg.SymbolID(12),
		RootName: "rows",
		Steps: []Step{
			{Kind: StepStaticMember, Member: value.MemberField("items")},
			{Kind: StepStaticMember, Member: value.MemberField("name")},
		},
	}

	if _, ok := loc.FinalDynamicIndexTargetPath(); ok {
		t.Fatal("FinalDynamicIndexTargetPath() succeeded for non-dynamic final step")
	}
}
