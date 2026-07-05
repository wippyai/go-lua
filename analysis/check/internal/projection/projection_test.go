package projection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestWithStaticMemberWitnessBuildsNestedRecordWitness(t *testing.T) {
	reg := standard.Registry()
	root := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	member := typevalue.FromType(reg, typ.String)

	got := WithStaticMemberWitness(reg, root, []StaticMemberValue{
		{
			Suffix: []segment.Segment{
				{Kind: segment.SegmentField, Name: "config"},
				{Kind: segment.SegmentField, Name: "name"},
			},
			Value: member,
		},
	}, func(value product.Value) (typ.Type, bool) {
		return typevalue.TypeOf(reg, value)
	})

	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || gotType == nil {
		t.Fatalf("TypeOf = %v/%v, want structural witness", gotType, ok)
	}
	want := typetable.NewRecord().
		Field("config", typetable.NewRecord().Field("name", typ.String).Build()).
		Build()
	if !typ.TypeEquals(gotType, want) {
		t.Fatalf("projected witness = %v, want %v", gotType, want)
	}
}

func TestWithStaticMemberWitnessPreservesExistingMembers(t *testing.T) {
	reg := standard.Registry()
	rootType := typetable.NewRecord().Field("id", typ.String).Build()
	root := typevalue.WithWitness(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), rootType)
	member := typevalue.FromType(reg, typ.String)

	got := WithStaticMemberWitness(reg, root, []StaticMemberValue{
		{Suffix: []segment.Segment{{Kind: segment.SegmentField, Name: "config"}}, Value: member},
	}, func(value product.Value) (typ.Type, bool) {
		return typevalue.TypeOf(reg, value)
	})

	gotType, ok := typevalue.TypeOf(reg, got)
	want := typetable.NewRecord().
		Field("id", typ.String).
		Field("config", typ.String).
		Build()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("projected witness = %v/%v, want %v", gotType, ok, want)
	}
}

func TestWithDeclaredContractPreservingPresenceKeepsSourcePresence(t *testing.T) {
	reg := standard.Registry()
	value := product.WithPresence(reg, typevalue.FromType(reg, typ.String), presence.Maybe())
	declared := typevalue.FromType(reg, typ.String)

	got := WithDeclaredContractPreservingPresence(reg, value, declared)

	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Maybe()) {
		t.Fatalf("presence = %s, want source maybe", gotPresence)
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.MaterializeOptional(typ.String)) {
		t.Fatalf("type = %v/%v, want optional string from source presence", gotType, ok)
	}
}
